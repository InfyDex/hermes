package notifier_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/config"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/notifier"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestNotifierWebOnly(t *testing.T) {
	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{}, "http://localhost", "test")

	job := &models.Job{
		ID:              1,
		Name:            "notify-job",
		NotifyOnSuccess: true,
		NotifyWeb:       true,
	}
	exec := &models.Execution{
		ID:        1,
		StartTime: time.Now().UTC(),
		EndTime:   sql.NullTime{Time: time.Now().UTC(), Valid: true},
	}

	n.Notify(job, exec, notifier.EventSuccess)

	notifs, err := db.GetUnreadNotifications()
	if err != nil || len(notifs) != 1 {
		t.Fatalf("notifications = %d, err=%v", len(notifs), err)
	}
}

func TestNotifierDiscordWebhook(t *testing.T) {
	var posted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{DiscordWebhookURL: server.URL}, "http://localhost", "prod")

	job := &models.Job{
		ID:              1,
		Name:            "discord-job",
		NotifyOnFailure: true,
		NotifyDiscord:   true,
	}
	exec := &models.Execution{ID: 2}

	n.Notify(job, exec, notifier.EventFailure)
	time.Sleep(100 * time.Millisecond)
	if !posted {
		t.Fatal("expected discord webhook POST")
	}
}

func TestNotifierDisabledEvent(t *testing.T) {
	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{}, "", "")

	job := &models.Job{ID: 1, Name: "quiet", NotifyWeb: true}
	n.Notify(job, nil, notifier.EventStart)

	notifs, _ := db.GetUnreadNotifications()
	if len(notifs) != 0 {
		t.Fatalf("expected no notifications, got %d", len(notifs))
	}
}

func TestSystemNotify(t *testing.T) {
	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{}, "", "")

	n.SystemNotify("Boot", "Hermes started")

	notifs, err := db.GetUnreadNotifications()
	if err != nil || len(notifs) != 1 {
		t.Fatalf("notifications = %d, err=%v", len(notifs), err)
	}
}

func TestNotifierEmailPath(t *testing.T) {
	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{
		SMTPHost: "127.0.0.1",
		SMTPPort: 1,
		SMTPUser: "user@example.com",
	}, "http://localhost", "")

	job := &models.Job{
		ID:            1,
		Name:          "email-job",
		NotifyOnStart: true,
		NotifyEmail:   true,
	}
	n.Notify(job, &models.Execution{ID: 1}, notifier.EventStart)
	time.Sleep(50 * time.Millisecond)
}

func TestSystemNotifyDiscord(t *testing.T) {
	var posted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posted = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{DiscordWebhookURL: server.URL}, "", "prod")
	n.SystemNotify("System Alert", "disk full")
	time.Sleep(100 * time.Millisecond)
	if !posted {
		t.Fatal("expected system discord webhook")
	}
}

func TestSystemNotifyEmail(t *testing.T) {
	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{
		SMTPHost: "127.0.0.1",
		SMTPPort: 1,
		SMTPUser: "user@example.com",
	}, "", "")
	n.SystemNotify("Email Alert", "check logs")
	time.Sleep(50 * time.Millisecond)
}

func TestNotifierAllChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	db := testutil.TestDB(t)
	n := notifier.New(db, &config.NotifyConfig{
		DiscordWebhookURL: server.URL,
		SMTPHost:          "127.0.0.1",
		SMTPPort:          1,
		SMTPUser:          "user@example.com",
	}, "http://localhost", "srv")

	job := &models.Job{
		ID:             1,
		Name:           "all-channels",
		NotifyOnCancel: true,
		NotifyWeb:      true,
		NotifyDiscord:  true,
		NotifyEmail:    true,
	}
	n.Notify(job, &models.Execution{ID: 3}, notifier.EventCancel)
	time.Sleep(100 * time.Millisecond)
}
