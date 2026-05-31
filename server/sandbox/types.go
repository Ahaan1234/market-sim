package sandbox

import "time"

type Status string

const (
	StatusSpawning Status = "spawning"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
	StatusStopped  Status = "stopped"
)

type SandboxInfo struct {
	TraderID    string    `json:"trader_id"`
	ContainerID string    `json:"container_id"`
	Status      Status    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	ErrorMsg    string    `json:"error_msg,omitempty"`
	ScriptSize  int       `json:"script_size"`
}
