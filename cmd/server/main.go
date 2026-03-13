package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/api"
	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/executor"
	"github.com/hermes-scheduler/hermes/internal/runners"
	"github.com/hermes-scheduler/hermes/internal/scheduler"
	"github.com/hermes-scheduler/hermes/internal/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.MkdirAll(cfg.Logs.Directory, 0750); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}

	db, err := database.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	registry := runners.NewRegistry()
	registry.Register(runners.NewShellRunner())
	registry.Register(runners.NewDockerRunner())

	exec := executor.New(db, registry, cfg.Logs.Directory)

	sched := scheduler.New(db, exec)
	if err := sched.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	// Clean up old notifications on boot
	if err := db.ClearOldNotifications(30); err != nil {
		log.Printf("Warning: failed to clear old notifications: %v", err)
	}

	router := mux.NewRouter()

	apiHandler := api.New(db, sched, exec)
	apiHandler.RegisterRoutes(router)

	webHandler := web.New(db, sched, exec)
	webHandler.RegisterRoutes(router)

	handler := api.BasicAuth(&cfg.Auth, router)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		sched.Stop()
		server.Close()
	}()

	log.Printf("Hermes started on http://0.0.0.0%s", addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
