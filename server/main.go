package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"quantsim-server/api"
	"quantsim-server/db"
	"quantsim-server/engine"
	"quantsim-server/sandbox"
	"quantsim-server/ws"
)

func main() {
	enginePath := flag.String("engine", "../build/quantsim", "path to C++ engine binary")
	dbPath     := flag.String("db", "./quantsim.db", "SQLite database path")
	port       := flag.String("port", "8080", "HTTP listen port")
	tickMs     := flag.Int("tick-ms", 500, "engine tick interval ms (passed to engine)")
	makers     := flag.Int("makers", 10, "number of maker agents")
	takers     := flag.Int("takers", 20, "number of taker agents")
	whales     := flag.Int("whales", 2, "number of whale agents")
	flag.Parse()

	initialCfg := api.EngineConfig{
		Makers: *makers, Takers: *takers, Whales: *whales, TickMs: *tickMs,
	}

	// ── Database ────────────────────────────────────────────────────────────
	store, err := db.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// ── Engine runner ────────────────────────────────────────────────────────
	eventChan := make(chan engine.Event, 500)
	runner := engine.NewRunner(
		*enginePath,
		initialCfg.Args(),
		eventChan,
	)

	// ── WebSocket hub ────────────────────────────────────────────────────────
	hub := ws.NewHub(runner)
	go hub.Run()

	// ── Sandbox relay + manager ──────────────────────────────────────────────
	relay := sandbox.NewRelay(runner)

	sandboxMgr, err := sandbox.NewManager(relay, eventChan)
	if err != nil {
		log.Fatalf("sandbox: %v", err)
	}

	// ── Context wired to OS signals ──────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	// ── Start engine (auto-restarts on crash) ────────────────────────────────
	go runner.Start(ctx)

	// ── Fan events from engine → hub + relay + DB ────────────────────────────
	go func() {
		for ev := range eventChan {
			hub.Broadcast(ev.Raw)
			select {
			case relay.TickSub() <- ev.Raw:
			default:
			}
			if ev.Type == "tick" {
				if err := store.StoreTick(ev.Raw); err != nil {
					log.Printf("db: StoreTick: %v", err)
				}
			}
		}
	}()

	// ── HTTP routes ──────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS)
	mux.HandleFunc("/api/history", api.HistoryHandler(store))
	mux.HandleFunc("/api/config", api.ConfigHandler(runner, initialCfg))
	mux.HandleFunc("/api/traders", api.TraderHandler(sandboxMgr))
	mux.HandleFunc("/api/traders/", api.TraderHandler(sandboxMgr))
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	srv := &http.Server{Addr: ":" + *port, Handler: mux}
	go func() {
		log.Printf("server: listening on :%s", *port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server: %v", err)
		}
	}()

	// ── Wait for shutdown signal ─────────────────────────────────────────────
	<-ctx.Done()
	log.Println("server: shutting down")
	sandboxMgr.KillAll()
	srv.Shutdown(context.Background())
	store.Close()
}
