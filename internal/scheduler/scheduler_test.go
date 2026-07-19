package scheduler_test

import (
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestSchedulerAddRemoveNextRun(t *testing.T) {
	db, _, sched, _ := testutil.TestStack(t)
	t.Cleanup(func() { sched.Stop() })

	job := &models.Job{
		Name:       "cron-job",
		CronExpr:   "0 0 0 * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "echo sched",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
	if err := db.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if err := sched.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := sched.AddJob(job); err != nil {
		t.Fatalf("AddJob: %v", err)
	}
	if next := sched.NextRun(job.ID); next == nil {
		t.Fatal("expected next run")
	}

	sched.RemoveJob(job.ID)
	if next := sched.NextRun(job.ID); next != nil {
		t.Fatal("expected no next run after remove")
	}
}

func TestSchedulerDisabledJobSkipped(t *testing.T) {
	db, _, sched, _ := testutil.TestStack(t)
	t.Cleanup(func() { sched.Stop() })

	job := &models.Job{
		Name:       "disabled-job",
		CronExpr:   "0 0 0 * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "echo",
		EnvVars:    "{}",
		Status:     models.JobStatusDisabled,
	}
	_ = db.CreateJob(job)
	_ = sched.Start()
	if err := sched.AddJob(job); err != nil {
		t.Fatalf("AddJob: %v", err)
	}
	if next := sched.NextRun(job.ID); next != nil {
		t.Fatal("expected disabled job not scheduled")
	}
}

func TestSchedulerInvalidCron(t *testing.T) {
	_, _, sched, _ := testutil.TestStack(t)
	t.Cleanup(func() { sched.Stop() })
	_ = sched.Start()

	job := &models.Job{
		ID:         1,
		Name:       "bad-cron",
		CronExpr:   "not a cron",
		RunnerType: models.RunnerTypeShell,
		Command:    "echo",
		Status:     models.JobStatusEnabled,
	}
	if err := sched.AddJob(job); err == nil {
		t.Fatal("expected invalid cron error")
	}
}

func TestSchedulerRescheduleJob(t *testing.T) {
	db, _, sched, _ := testutil.TestStack(t)
	t.Cleanup(func() { sched.Stop() })
	_ = sched.Start()

	job := &models.Job{
		Name:       "resched",
		CronExpr:   "0 0 0 * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "echo",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)
	if err := sched.AddJob(job); err != nil {
		t.Fatalf("AddJob: %v", err)
	}
	job.CronExpr = "0 0 12 * * *"
	if err := sched.AddJob(job); err != nil {
		t.Fatalf("re-AddJob: %v", err)
	}
	if sched.NextRun(job.ID) == nil {
		t.Fatal("expected next run after reschedule")
	}
	time.Sleep(10 * time.Millisecond)
}
