package engine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"
)

// Event is one line of output from the C++ engine.
type Event struct {
	Raw  []byte // original JSON line, broadcast as-is
	Type string // "tick", "fill", "reject", or "" for unrecognised lines
}

// Runner manages the C++ engine subprocess and its stdio pipes.
type Runner struct {
	binaryPath string
	args       []string
	eventChan  chan<- Event
	stdinPipe  io.WriteCloser
	mu         sync.Mutex // guards stdinPipe
}

// NewRunner creates a Runner. Call Start in a goroutine to launch the process.
func NewRunner(binaryPath string, args []string, eventChan chan<- Event) *Runner {
	return &Runner{
		binaryPath: binaryPath,
		args:       args,
		eventChan:  eventChan,
	}
}

// Start launches the C++ process and restarts it on exit. Blocks until ctx
// is cancelled. Must be called in its own goroutine.
func (r *Runner) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		log.Printf("engine: starting %s %v", r.binaryPath, r.args)
		if err := r.run(ctx); err != nil {
			log.Printf("engine: exited with error: %v", err)
		} else {
			log.Printf("engine: exited cleanly")
		}

		// Don't restart if context was cancelled.
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
			log.Printf("engine: restarting")
		}
	}
}

// run starts one instance of the engine process and blocks until it exits.
func (r *Runner) run(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, r.binaryPath, r.args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	log.Printf("engine: pid %d running", cmd.Process.Pid)

	r.mu.Lock()
	r.stdinPipe = stdin
	r.mu.Unlock()

	// Log engine's stderr without blocking the stdout reader.
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			log.Printf("[engine stderr] %s", sc.Text())
		}
	}()

	// Read stdout line by line and forward to the event channel.
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB max line
	for sc.Scan() {
		raw := sc.Bytes()
		// Make a copy — scanner reuses its buffer.
		cp := make([]byte, len(raw))
		copy(cp, raw)

		ev := Event{Raw: cp, Type: extractType(cp)}
		select {
		case r.eventChan <- ev:
		default:
			// Channel full — drop rather than block the stdout reader.
		}
	}

	r.mu.Lock()
	r.stdinPipe = nil
	r.mu.Unlock()

	return cmd.Wait()
}

// SendOrder writes one order JSON line to the engine's stdin. Thread-safe.
func (r *Runner) SendOrder(orderJSON []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stdinPipe == nil {
		return fmt.Errorf("engine not running")
	}

	// Append newline — the C++ stdin reader expects newline-delimited JSON.
	buf := make([]byte, len(orderJSON)+1)
	copy(buf, orderJSON)
	buf[len(orderJSON)] = '\n'

	_, err := r.stdinPipe.Write(buf)
	return err
}

// extractType reads the "type" field without a full JSON parse.
func extractType(data []byte) string {
	switch {
	case bytes.Contains(data, []byte(`"type":"tick"`)):
		return "tick"
	case bytes.Contains(data, []byte(`"type":"fill"`)):
		return "fill"
	case bytes.Contains(data, []byte(`"type":"reject"`)):
		return "reject"
	default:
		return ""
	}
}
