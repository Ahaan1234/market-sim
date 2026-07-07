package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"

	"quantsim-server/engine"
)

const (
	maxConcurrentSandboxes = 5
	maxScriptBytes         = 50 * 1024
	autoKillAfter          = 24 * time.Hour
	sandboxImageName       = "quantsim-sandbox:latest"
	scriptBasePath         = "/tmp/quantsim-scripts"
)

var traderIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

type sandboxEntry struct {
	info      SandboxInfo
	scriptDir string
	cancel    context.CancelFunc
}

// Manager spawns and supervises Docker sandbox containers for trader scripts.
type Manager struct {
	dockerClient *dockerclient.Client
	sandboxes    map[string]*sandboxEntry
	mu           sync.RWMutex
	relay        *Relay
	eventChan    chan<- engine.Event
}

// NewManager creates a Manager connected to the local Docker daemon.
func NewManager(relay *Relay, eventChan chan<- engine.Event) (*Manager, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Manager{
		dockerClient: cli,
		sandboxes:    make(map[string]*sandboxEntry),
		relay:        relay,
		eventChan:    eventChan,
	}, nil
}

// Spawn launches a new sandbox container running scriptContent for traderID.
func (m *Manager) Spawn(traderID, scriptContent string) error {
	if !traderIDRegex.MatchString(traderID) {
		return fmt.Errorf("invalid_trader_id")
	}
	if len(scriptContent) > maxScriptBytes {
		return fmt.Errorf("script_too_large")
	}

	m.mu.Lock()
	if _, exists := m.sandboxes[traderID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("duplicate_trader_id")
	}

	// Count active sandboxes before reserving the slot.
	active := 0
	for _, e := range m.sandboxes {
		if e.info.Status == StatusRunning || e.info.Status == StatusSpawning {
			active++
		}
	}
	if active >= maxConcurrentSandboxes {
		m.mu.Unlock()
		return fmt.Errorf("too_many_sandboxes")
	}

	ctx, cancel := context.WithCancel(context.Background())
	entry := &sandboxEntry{
		info: SandboxInfo{
			TraderID:   traderID,
			Status:     StatusSpawning,
			StartedAt:  time.Now(),
			ScriptSize: len(scriptContent),
		},
		cancel: cancel,
	}
	m.sandboxes[traderID] = entry
	m.mu.Unlock()

	// Write script to host filesystem.
	scriptDir := filepath.Join(scriptBasePath, traderID)
	scriptPath := filepath.Join(scriptDir, "trader.py")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		m.setError(traderID, cancel, fmt.Sprintf("mkdir script: %v", err))
		return fmt.Errorf("mkdir script dir: %w", err)
	}
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0444); err != nil {
		m.setError(traderID, cancel, fmt.Sprintf("write script: %v", err))
		return fmt.Errorf("write script: %w", err)
	}
	entry.scriptDir = scriptDir

	// Create the container with an open stdin: the relay bridges engine
	// events → container stdin and container stdout → engine orders.
	cfg := &container.Config{
		Image:     sandboxImageName,
		OpenStdin: true,
		Env: []string{
			"TRADER_ID=" + traderID,
		},
		Labels: map[string]string{
			"quantsim-sandbox": "true",
			"trader-id":        traderID,
		},
	}
	hostCfg := &container.HostConfig{
		NetworkMode:    "none",
		ReadonlyRootfs: true,
		AutoRemove:     true,
		Resources: container.Resources{
			Memory:   128 << 20,
			NanoCPUs: 500_000_000,
		},
		Tmpfs: map[string]string{
			"/tmp": "rw,size=16m",
		},
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeBind,
				Source:   scriptPath,
				Target:   "/script/trader.py",
				ReadOnly: true,
			},
		},
	}

	resp, err := m.dockerClient.ContainerCreate(ctx, cfg, hostCfg, nil, nil, "")
	if err != nil {
		m.setError(traderID, cancel, fmt.Sprintf("container create: %v", err))
		return fmt.Errorf("container create: %w", err)
	}

	// Attach before starting so no early output is missed.
	attach, err := m.dockerClient.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		m.setError(traderID, cancel, fmt.Sprintf("container attach: %v", err))
		return fmt.Errorf("container attach: %w", err)
	}
	go m.relay.HandleSandbox(ctx, traderID, attach.Conn, attach.Reader)

	if err := m.dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		m.setError(traderID, cancel, fmt.Sprintf("container start: %v", err))
		return fmt.Errorf("container start: %w", err)
	}

	m.mu.Lock()
	entry.info.ContainerID = resp.ID
	entry.info.Status = StatusRunning
	m.mu.Unlock()

	go m.monitor(ctx, cancel, traderID, resp.ID)
	return nil
}

func (m *Manager) setError(traderID string, cancel context.CancelFunc, msg string) {
	cancel()
	m.mu.Lock()
	if e, ok := m.sandboxes[traderID]; ok {
		e.info.Status = StatusError
		e.info.ErrorMsg = msg
	}
	m.mu.Unlock()
}

func (m *Manager) monitor(ctx context.Context, cancel context.CancelFunc, traderID, containerID string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	autoKill := time.After(autoKillAfter)

	for {
		select {
		case <-ctx.Done():
			return

		case <-autoKill:
			log.Printf("sandbox[%s]: auto-kill after %v", traderID, autoKillAfter)
			m.Kill(traderID)
			return

		case <-ticker.C:
			inspect, err := m.dockerClient.ContainerInspect(context.Background(), containerID)
			if err != nil || !inspect.State.Running {
				exitCode := 0
				if err == nil && inspect.State != nil {
					exitCode = inspect.State.ExitCode
				}
				log.Printf("sandbox[%s]: container stopped (exit %d)", traderID, exitCode)
				cancel()
				m.mu.Lock()
				if e, ok := m.sandboxes[traderID]; ok {
					e.info.Status = StatusStopped
				}
				m.mu.Unlock()
				m.cleanup(traderID)
				return
			}
		}
	}
}

// Kill stops the container and cleans up resources for traderID.
func (m *Manager) Kill(traderID string) error {
	m.mu.RLock()
	entry, ok := m.sandboxes[traderID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("not_found")
	}

	entry.cancel()

	if entry.info.ContainerID != "" {
		timeout := 5
		m.dockerClient.ContainerStop(
			context.Background(),
			entry.info.ContainerID,
			container.StopOptions{Timeout: &timeout},
		)
	}

	m.mu.Lock()
	if e, ok := m.sandboxes[traderID]; ok {
		e.info.Status = StatusStopped
	}
	m.mu.Unlock()

	m.cleanup(traderID)
	return nil
}

func (m *Manager) cleanup(traderID string) {
	m.mu.RLock()
	entry, ok := m.sandboxes[traderID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	if entry.scriptDir != "" {
		os.RemoveAll(entry.scriptDir)
	}
}

// KillAll stops all running sandboxes. Called on server shutdown.
func (m *Manager) KillAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sandboxes))
	for id := range m.sandboxes {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	for _, id := range ids {
		if err := m.Kill(id); err != nil {
			log.Printf("sandbox: KillAll %s: %v", id, err)
		}
	}
}

// List returns a snapshot of all sandbox infos.
func (m *Manager) List() []SandboxInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SandboxInfo, 0, len(m.sandboxes))
	for _, e := range m.sandboxes {
		out = append(out, e.info)
	}
	return out
}

// Get returns the SandboxInfo for traderID.
func (m *Manager) Get(traderID string) (SandboxInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.sandboxes[traderID]
	if !ok {
		return SandboxInfo{}, false
	}
	return e.info, true
}

// ContainerLogs returns the last tail lines of stdout+stderr for traderID's container.
func (m *Manager) ContainerLogs(traderID, tail string) (string, error) {
	m.mu.RLock()
	entry, ok := m.sandboxes[traderID]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("not_found")
	}
	if entry.info.ContainerID == "" {
		return "", fmt.Errorf("no_container")
	}

	rc, err := m.dockerClient.ContainerLogs(
		context.Background(),
		entry.info.ContainerID,
		container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       tail,
		},
	)
	if err != nil {
		return "", fmt.Errorf("logs: %w", err)
	}
	defer rc.Close()

	var buf bytes.Buffer
	stdcopy.StdCopy(&buf, &buf, rc)
	return buf.String(), nil
}
