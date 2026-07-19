package executor_test

import (
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func TestExecutorRunSuccess(t *testing.T) {
	db, exec, _, _ := testutil.TestStack(t)
	job := &models.Job{
		Name:       "success-job",
		CronExpr:   "0 * * * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "echo done",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
	if err := db.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	exec.Run(job, "manual")
	time.Sleep(200 * time.Millisecond)

	execs, err := db.ListExecutions(job.ID, 1)
	if err != nil || len(execs) != 1 {
		t.Fatalf("executions = %d, err=%v", len(execs), err)
	}
	if execs[0].Status != models.ExecStatusSuccess {
		t.Fatalf("status = %q", execs[0].Status)
	}
}

func TestExecutorRunFailure(t *testing.T) {
	db, exec, _, _ := testutil.TestStack(t)
	job := &models.Job{
		Name:       "fail-job",
		CronExpr:   "0 * * * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "exit 1",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)
	exec.Run(job, "manual")
	time.Sleep(200 * time.Millisecond)

	execs, _ := db.ListExecutions(job.ID, 1)
	if len(execs) != 1 || execs[0].Status != models.ExecStatusFailed {
		t.Fatalf("executions = %+v", execs)
	}
}

func TestExecutorParallelLock(t *testing.T) {
	db, exec, _, _ := testutil.TestStack(t)
	job := &models.Job{
		Name:          "lock-job",
		CronExpr:      "0 * * * * *",
		RunnerType:    models.RunnerTypeShell,
		Command:       "sleep 1",
		EnvVars:       "{}",
		Status:        models.JobStatusEnabled,
		AllowParallel: false,
	}
	_ = db.CreateJob(job)

	go exec.Run(job, "manual")
	time.Sleep(50 * time.Millisecond)
	exec.Run(job, "manual")
	time.Sleep(100 * time.Millisecond)

	execs, _ := db.ListExecutions(job.ID, 10)
	if len(execs) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(execs))
	}
}

func TestExecutorUnknownRunner(t *testing.T) {
	_, exec, _, _ := testutil.TestStack(t)
	job := &models.Job{
		ID:         1,
		RunnerType: models.RunnerType("unknown"),
		Command:    "echo",
	}
	exec.Run(job, "manual")
}

func TestExecutorCancel(t *testing.T) {
	db, exec, _, _ := testutil.TestStack(t)
	job := &models.Job{
		Name:       "cancel-job",
		CronExpr:   "0 * * * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "sleep 5",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)

	go exec.Run(job, "manual")
	time.Sleep(100 * time.Millisecond)
	execs, _ := db.ListExecutions(job.ID, 1)
	if len(execs) != 1 {
		t.Fatal("expected running execution")
	}
	if !exec.Cancel(execs[0].ID) {
		t.Fatal("expected cancel to succeed")
	}
	if exec.Cancel(9999) {
		t.Fatal("expected cancel unknown to fail")
	}
	time.Sleep(200 * time.Millisecond)
}

func TestExecutorTimeout(t *testing.T) {
	db, exec, _, _ := testutil.TestStack(t)
	job := &models.Job{
		Name:       "timeout-job",
		CronExpr:   "0 * * * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "sleep 5",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
		Timeout:    1,
	}
	_ = db.CreateJob(job)
	exec.Run(job, "manual")
	time.Sleep(1500 * time.Millisecond)

	execs, _ := db.ListExecutions(job.ID, 1)
	if len(execs) != 1 || execs[0].Status != models.ExecStatusFailed {
		t.Fatalf("executions = %+v", execs)
	}
}

func TestIsJobRunning(t *testing.T) {
	db, exec, _, _ := testutil.TestStack(t)
	job := &models.Job{
		Name:       "running-job",
		CronExpr:   "0 * * * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "sleep 1",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)

	if exec.IsJobRunning(job.ID) {
		t.Fatal("expected not running initially")
	}
	go exec.Run(job, "manual")
	time.Sleep(50 * time.Millisecond)
	if !exec.IsJobRunning(job.ID) {
		t.Fatal("expected running")
	}
	time.Sleep(1200 * time.Millisecond)
}
