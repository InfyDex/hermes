package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/executor"
	"github.com/hermes-scheduler/hermes/internal/fleet"
	"github.com/hermes-scheduler/hermes/internal/notifier"
	"github.com/hermes-scheduler/hermes/internal/runners"
	"github.com/hermes-scheduler/hermes/internal/scheduler"
)

// WebDeps holds components used by web handler tests.
type WebDeps struct {
	DB    *database.DB
	Exec  *executor.Executor
	Sched *scheduler.Scheduler
}

const SessionSecret = "test-session-secret-must-be-at-least-32-bytes"

func SetTestEnv(t *testing.T) func() {
	t.Helper()
	prevSecret := os.Getenv("HERMES_SESSION_SECRET")
	os.Setenv("HERMES_SESSION_SECRET", SessionSecret)
	return func() {
		if prevSecret == "" {
			os.Unsetenv("HERMES_SESSION_SECRET")
		} else {
			os.Setenv("HERMES_SESSION_SECRET", prevSecret)
		}
	}
}

func TestDB(t *testing.T) *database.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("database.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAuthConfig() *config.AuthConfig {
	return &config.AuthConfig{Username: "admin", Password: "secret"}
}

func TestServerConfig() config.ServerConfig {
	return config.ServerConfig{DomainURL: "http://localhost:4376", ServerName: "test"}
}

func TestSessionConfig() *config.SessionConfig {
	return &config.SessionConfig{
		Secret:      SessionSecret,
		TTL:         time.Hour,
		RememberTTL: 24 * time.Hour,
	}
}

func TestSessionStore(t *testing.T) *auth.SessionStore {
	t.Helper()
	store, err := auth.NewSessionStore(TestSessionConfig())
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	return store
}

func TestRegistry() *runners.Registry {
	reg := runners.NewRegistry()
	reg.Register(runners.NewShellRunner())
	reg.Register(runners.NewDockerRunner())
	return reg
}

func TestFleetManager(t *testing.T, db *database.DB, notif *notifier.Notifier) *fleet.Manager {
	t.Helper()
	mgr, err := fleet.New(db, notif, config.ServerConfig{DomainURL: "http://localhost:4376"}, config.FleetConfig{NodeID: "test-node"})
	if err != nil {
		t.Fatalf("fleet.New: %v", err)
	}
	return mgr
}

func TestStack(t *testing.T) (*database.DB, *executor.Executor, *scheduler.Scheduler, *notifier.Notifier) {
	t.Helper()
	db := TestDB(t)
	logsDir := filepath.Join(t.TempDir(), "logs")
	reg := TestRegistry()
	notif := notifier.New(db, &config.NotifyConfig{}, "http://localhost", "test")
	exec := executor.New(db, reg, logsDir, notif)
	sched := scheduler.New(db, exec)
	return db, exec, sched, notif
}
