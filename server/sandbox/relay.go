package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"
	"time"

	"github.com/docker/docker/pkg/stdcopy"

	"quantsim-server/engine"
)

// Relay fans engine events (ticks, fills, rejects) to sandbox containers and
// routes their orders back to the engine. Transport is the container's own
// stdin/stdout, attached via the Docker API — no network, no sockets, and it
// behaves identically on macOS and Linux hosts.
type Relay struct {
	runner   *engine.Runner
	tickSub  chan []byte // receives all raw engine events from main fan-out
	mu       sync.RWMutex
	lastTick []byte
	conns    map[string]chan []byte // trader_id → per-sandbox outbound channel
}

// NewRelay creates a Relay and starts its background fan-out goroutine.
func NewRelay(runner *engine.Runner) *Relay {
	r := &Relay{
		runner:  runner,
		tickSub: make(chan []byte, 256),
		conns:   make(map[string]chan []byte),
	}
	go r.fanOut()
	return r
}

// TickSub returns a send-only view of the internal subscription channel.
// main.go uses this to push raw engine events into the relay.
func (r *Relay) TickSub() chan<- []byte {
	return r.tickSub
}

// fanOut reads from tickSub and routes events to attached sandboxes.
func (r *Relay) fanOut() {
	for raw := range r.tickSub {
		cp := make([]byte, len(raw))
		copy(cp, raw)

		var hdr struct {
			Type     string `json:"type"`
			TraderID string `json:"trader_id"`
		}
		json.Unmarshal(cp, &hdr)

		switch hdr.Type {
		case "tick":
			r.mu.Lock()
			r.lastTick = cp
			for _, ch := range r.conns {
				select {
				case ch <- cp:
				default:
				}
			}
			r.mu.Unlock()

		case "fill", "reject":
			if hdr.TraderID == "" {
				continue
			}
			r.mu.RLock()
			ch, ok := r.conns[hdr.TraderID]
			r.mu.RUnlock()
			if ok {
				select {
				case ch <- cp:
				default:
				}
			}
		}
	}
}

// HandleSandbox bridges one attached container: engine events are written to
// the container's stdin, order lines read from its stdout are forwarded to
// the engine. stdin is the hijacked attach connection; output is the Docker
// multiplexed stream (stdout + stderr frames). Blocks until the container
// stops or ctx is cancelled.
func (r *Relay) HandleSandbox(ctx context.Context, traderID string, stdin io.WriteCloser, output io.Reader) {
	outCh := make(chan []byte, 64)

	r.mu.Lock()
	r.conns[traderID] = outCh
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if r.conns[traderID] == outCh {
			delete(r.conns, traderID)
		}
		r.mu.Unlock()
		stdin.Close()
	}()

	// Send last known tick immediately so the strategy has market context.
	r.mu.RLock()
	lt := r.lastTick
	r.mu.RUnlock()
	if lt != nil {
		select {
		case outCh <- lt:
		default:
		}
	}

	// Writer: drain outCh → container stdin.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-outCh:
				if !ok {
					return
				}
				if _, err := stdin.Write(append(msg, '\n')); err != nil {
					return
				}
			}
		}
	}()

	// Demultiplex the container's output: stdout carries order JSON,
	// stderr is surfaced into the server log for debugging.
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(stdoutW, stderrW, output)
		stdoutW.CloseWithError(err)
		stderrW.CloseWithError(err)
	}()
	go func() {
		sc := bufio.NewScanner(stderrR)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			log.Printf("[sandbox %s stderr] %s", traderID, sc.Text())
		}
	}()

	// Rate-limit state: sliding 1-second window.
	var (
		rlCount  int
		rlWindow time.Time
	)

	scanner := bufio.NewScanner(stdoutR)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		raw := scanner.Bytes()

		var msg struct {
			Type      string  `json:"type"`
			OrderID   string  `json:"order_id"`
			Side      string  `json:"side"`
			OrderType string  `json:"order_type"`
			Price     float64 `json:"price"`
			Qty       float64 `json:"qty"`
		}
		// Non-order lines (user print() output etc.) are simply ignored.
		if err := json.Unmarshal(raw, &msg); err != nil || msg.Type != "order" {
			continue
		}

		now := time.Now()
		if now.After(rlWindow) {
			rlCount = 0
			rlWindow = now.Add(time.Second)
		}
		rlCount++
		if rlCount > 10 {
			reject := []byte(`{"type":"reject","reason":"rate_limited"}`)
			select {
			case outCh <- reject:
			default:
			}
			continue
		}

		// Translate to engine stdin format: order_type → type. The trader_id
		// is forced to the sandbox's own ID so a script can't spoof another.
		orderType := msg.OrderType
		if orderType == "" {
			orderType = "LIMIT"
		}
		engineOrder, err := json.Marshal(map[string]interface{}{
			"trader_id": traderID,
			"order_id":  msg.OrderID,
			"side":      msg.Side,
			"type":      orderType,
			"price":     msg.Price,
			"qty":       msg.Qty,
		})
		if err != nil {
			continue
		}
		if err := r.runner.SendOrder(engineOrder); err != nil {
			log.Printf("relay[%s]: SendOrder: %v", traderID, err)
		}
	}
	<-writerDone
}
