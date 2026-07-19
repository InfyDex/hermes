package database_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func sampleJob(name string) *models.Job {
	return &models.Job{
		Name:       name,
		CronExpr:   "0 * * * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "echo hi",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
}

func TestJobCRUD(t *testing.T) {
	db := testutil.TestDB(t)

	job := sampleJob("test-job")
	if err := db.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.ID == 0 {
		t.Fatal("expected job ID assigned")
	}

	got, err := db.GetJob(job.ID)
	if err != nil || got == nil || got.Name != "test-job" {
		t.Fatalf("GetJob = %+v, err=%v", got, err)
	}

	missing, err := db.GetJob(9999)
	if err != nil || missing != nil {
		t.Fatalf("GetJob missing = %+v, err=%v", missing, err)
	}

	job.Name = "updated"
	if err := db.UpdateJob(job); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	got, _ = db.GetJob(job.ID)
	if got.Name != "updated" {
		t.Fatalf("name = %q", got.Name)
	}

	jobs, err := db.ListJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("ListJobs = %d, err=%v", len(jobs), err)
	}

	enabled, err := db.GetEnabledJobs()
	if err != nil || len(enabled) != 1 {
		t.Fatalf("GetEnabledJobs = %d, err=%v", len(enabled), err)
	}

	job.Status = models.JobStatusDisabled
	_ = db.UpdateJob(job)
	enabled, _ = db.GetEnabledJobs()
	if len(enabled) != 0 {
		t.Fatalf("enabled count = %d", len(enabled))
	}

	if err := db.DeleteJob(job.ID); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	jobs, _ = db.ListJobs()
	if len(jobs) != 0 {
		t.Fatalf("jobs after delete = %d", len(jobs))
	}
}

func TestExecutionCRUD(t *testing.T) {
	db := testutil.TestDB(t)
	job := sampleJob("exec-job")
	if err := db.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	exec := &models.Execution{
		JobID:     job.ID,
		StartTime: time.Now().UTC(),
		Status:    models.ExecStatusRunning,
		LogPath:   "/tmp/log.txt",
		Trigger:   "manual",
	}
	if err := db.CreateExecution(exec); err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	got, err := db.GetExecution(exec.ID)
	if err != nil || got == nil || got.JobName != job.Name {
		t.Fatalf("GetExecution = %+v, err=%v", got, err)
	}

	exec.Status = models.ExecStatusSuccess
	exec.EndTime = sql.NullTime{Time: time.Now().UTC(), Valid: true}
	exec.ExitCode = sql.NullInt64{Int64: 0, Valid: true}
	if err := db.UpdateExecution(exec); err != nil {
		t.Fatalf("UpdateExecution: %v", err)
	}

	execs, err := db.ListExecutions(job.ID, 10)
	if err != nil || len(execs) != 1 {
		t.Fatalf("ListExecutions = %d, err=%v", len(execs), err)
	}

	now := time.Now().UTC()
	if err := db.UpdateJobLastRun(job.ID, now, "success"); err != nil {
		t.Fatalf("UpdateJobLastRun: %v", err)
	}
	next := now.Add(time.Hour)
	if err := db.UpdateJobNextRun(job.ID, &next); err != nil {
		t.Fatalf("UpdateJobNextRun: %v", err)
	}
}

func TestNotifications(t *testing.T) {
	db := testutil.TestDB(t)
	job := sampleJob("notify-job")
	_ = db.CreateJob(job)

	if err := db.InsertNotification(job.ID, "info", "hello"); err != nil {
		t.Fatalf("InsertNotification: %v", err)
	}
	if err := db.InsertNotification(0, "info", "system"); err != nil {
		t.Fatalf("InsertNotification system: %v", err)
	}

	notifs, err := db.GetUnreadNotifications()
	if err != nil || len(notifs) != 2 {
		t.Fatalf("GetUnreadNotifications = %d, err=%v", len(notifs), err)
	}
	var foundSystem bool
	for _, n := range notifs {
		if n.JobName == "System" {
			foundSystem = true
		}
	}
	if !foundSystem {
		t.Fatalf("expected system notification in %+v", notifs)
	}

	if err := db.MarkAllNotificationsRead(); err != nil {
		t.Fatalf("MarkAllNotificationsRead: %v", err)
	}
	notifs, _ = db.GetUnreadNotifications()
	if len(notifs) != 0 {
		t.Fatalf("unread after mark = %d", len(notifs))
	}

	if err := db.ClearOldNotifications(30); err != nil {
		t.Fatalf("ClearOldNotifications: %v", err)
	}
}

func TestJobBooleanFields(t *testing.T) {
	db := testutil.TestDB(t)
	job := sampleJob("bool-job")
	job.AllowParallel = true
	job.NotifyOnStart = true
	job.NotifyWeb = true
	if err := db.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	got, _ := db.GetJob(job.ID)
	if !got.AllowParallel || !got.NotifyOnStart || !got.NotifyWeb {
		t.Fatalf("bools = %+v", got)
	}
}
