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
	"quantsim-server/ws"
)

func main() {
	enginePath := flag.String("engine", "../build/quantsim", "path to C++ engine binary")
	dbPath     := flag.String("db", "./quantsim.db", "SQLite database path")
	port       := flag.String("port", "8080", "HTTP listen port")
	tickMs     := flag.String("tick-ms", "500", "engine tick interval ms (passed to engine)")
	flag.Parse()

	// ── Database ────────────────────────────────────────────────────────────
	store, err := db.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	// ── Engine runner ────────────────────────────────────────────────────────
	eventChan := make(chan engine.Event, 500)
	runner := engine.NewRunner(
		*enginePath,
		[]string{"--tick-ms", *tickMs},
		eventChan,
	)

	// ── WebSocket hub ────────────────────────────────────────────────────────
	hub := ws.NewHub(runner)
	go hub.Run()

	// ── Context wired to OS signals ──────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	// ── Start engine (auto-restarts on crash) ────────────────────────────────
	go runner.Start(ctx)

	// ── Fan events from engine → hub + DB ───────────────────────────────────
	go func() {
		for ev := range eventChan {
			hub.Broadcast(ev.Raw)
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
	srv.Shutdown(context.Background())
	store.Close()
}
