package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/api"
	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/executor"
	"github.com/hermes-scheduler/hermes/internal/notifier"
	"github.com/hermes-scheduler/hermes/internal/runners"
	"github.com/hermes-scheduler/hermes/internal/scheduler"
	"github.com/hermes-scheduler/hermes/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Auth.Username == "admin" && cfg.Auth.Password == "admin" {
		log.Println("Warning: using default admin credentials; set HERMES_USERNAME and HERMES_PASSWORD")
	}

	sessionStore, err := auth.NewSessionStore(&cfg.Session)
	if err != nil {
		log.Fatalf("Failed to initialize session store: %v", err)
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

	notif := notifier.New(db, &cfg.Notify, cfg.Server.DomainURL, cfg.Server.ServerName)
	exec := executor.New(db, registry, cfg.Logs.Directory, notif)

	sched := scheduler.New(db, exec)
	if err := sched.Start(); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	if err := db.ClearOldNotifications(30); err != nil {
		log.Printf("Warning: failed to clear old notifications: %v", err)
	}

	jobs, err := db.ListJobs()
	jobCount := 0
	if err == nil {
		for _, j := range jobs {
			if j.Status == "enabled" {
				jobCount++
			}
		}
	}
	notif.SystemNotify("Hermes Started", fmt.Sprintf("Hermes is ready. %d jobs are scheduled.", jobCount))

	limiter := auth.NewLoginRateLimiter()
	root := mux.NewRouter()

	apiHandler := api.New(db, sched, exec)
	webHandler := web.New(db, sched, exec, sessionStore, limiter, &cfg.Auth, cfg.Server.TrustProxy)

	webHandler.RegisterPublicRoutes(root)
	apiRouter := root.PathPrefix("/api").Subrouter()
	apiRouter.Use(auth.BasicAuthMiddleware(&cfg.Auth))
	apiHandler.RegisterRoutes(apiRouter)

	webRouter := root.NewRoute().MatcherFunc(func(r *http.Request, _ *mux.RouteMatch) bool {
		path := r.URL.Path
		return path != "/login" && !strings.HasPrefix(path, "/static/") && !strings.HasPrefix(path, "/api/")
	}).Subrouter()
	webRouter.Use(auth.SessionMiddleware(sessionStore))
	webHandler.RegisterRoutes(webRouter)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: root,
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
