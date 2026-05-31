package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"quantsim-server/engine"
)

// Relay fans engine events (ticks, fills, rejects) to connected sandbox processes
// over Unix domain sockets. Each sandbox process connects via its own socket.
type Relay struct {
	runner   *engine.Runner
	tickSub  chan []byte // receives all raw engine events from main fan-out
	mu       sync.RWMutex
	lastTick []byte
	conns    map[string]chan []byte // trader_id → per-connection outbound channel
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

// fanOut reads from tickSub and routes events to registered sandbox connections.
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

// ListenForSandbox opens a Unix socket at socketPath and accepts connections
// from the sandbox container identified by traderID. Blocks until ctx is done.
func (r *Relay) ListenForSandbox(ctx context.Context, traderID, socketPath string) {
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Printf("relay[%s]: listen %s: %v", traderID, socketPath, err)
		return
	}

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("relay[%s]: accept: %v", traderID, err)
				return
			}
		}
		go r.handleConn(ctx, traderID, conn)
	}
}

func (r *Relay) handleConn(ctx context.Context, traderID string, conn net.Conn) {
	defer conn.Close()

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
		close(outCh)
	}()

	// Send last known tick immediately so the sandbox has market context.
	r.mu.RLock()
	lt := r.lastTick
	r.mu.RUnlock()
	if lt != nil {
		outCh <- lt
	}

	// Writer goroutine: drain outCh → conn.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-outCh:
				if !ok {
					return
				}
				conn.Write(append(msg, '\n'))
			}
		}
	}()

	// Rate-limit state (per connection).
	var (
		rlMu      sync.Mutex
		rlCount   int
		rlWindow  time.Time
	)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		raw := scanner.Bytes()

		var hdr struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &hdr); err != nil || hdr.Type != "order" {
			continue
		}

		// Sliding 1-second window rate limit.
		now := time.Now()
		rlMu.Lock()
		if now.After(rlWindow) {
			rlCount = 0
			rlWindow = now.Add(time.Second)
		}
		rlCount++
		limited := rlCount > 10
		rlMu.Unlock()

		if limited {
			reject := []byte(`{"type":"reject","reason":"rate_limited"}`)
			select {
			case outCh <- reject:
			default:
			}
			continue
		}

		cp := make([]byte, len(raw))
		copy(cp, raw)
		if err := r.runner.SendOrder(cp); err != nil {
			log.Printf("relay[%s]: SendOrder: %v", traderID, err)
		}
	}
}
