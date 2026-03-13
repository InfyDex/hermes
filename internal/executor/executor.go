package executor

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/runners"
)

type Executor struct {
	db       *database.DB
	registry *runners.Registry
	logsDir  string

	mu       sync.Mutex
	running  map[int64]context.CancelFunc // execution ID -> cancel
	jobLocks map[int64]bool               // job ID -> is running
}

func New(db *database.DB, registry *runners.Registry, logsDir string) *Executor {
	os.MkdirAll(logsDir, 0750)
	return &Executor{
		db:       db,
		registry: registry,
		logsDir:  logsDir,
		running:  make(map[int64]context.CancelFunc),
		jobLocks: make(map[int64]bool),
	}
}

// Run executes a job. trigger is "schedule" or "manual".
func (e *Executor) Run(job *models.Job, trigger string) {
	if !job.AllowParallel {
		e.mu.Lock()
		if e.jobLocks[job.ID] {
			e.mu.Unlock()
			log.Printf("Job %d (%s) already running, skipping", job.ID, job.Name)
			return
		}
		e.jobLocks[job.ID] = true
		e.mu.Unlock()
		defer func() {
			e.mu.Lock()
			delete(e.jobLocks, job.ID)
			e.mu.Unlock()
		}()
	}

	runner, ok := e.registry.Get(job.RunnerType)
	if !ok {
		log.Printf("No runner for type %s (job %d)", job.RunnerType, job.ID)
		return
	}

	now := time.Now().UTC()
	logFileName := fmt.Sprintf("job_%d_%s.log", job.ID, now.Format("20060102_150405"))
	logPath := filepath.Join(e.logsDir, logFileName)
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Printf("Failed to create log file: %v", err)
		return
	}

	execution := &models.Execution{
		JobID:     job.ID,
		StartTime: now,
		Status:    models.ExecStatusRunning,
		LogPath:   logPath,
		Trigger:   trigger,
	}
	if err := e.db.CreateExecution(execution); err != nil {
		log.Printf("Failed to create execution: %v", err)
		logFile.Close()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	if job.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(job.Timeout)*time.Second)
	}

	e.mu.Lock()
	e.running[execution.ID] = cancel
	e.mu.Unlock()

	defer func() {
		cancel()
		logFile.Close()
		e.mu.Lock()
		delete(e.running, execution.ID)
		e.mu.Unlock()
	}()

	log.Printf("Starting job %d (%s), execution %d", job.ID, job.Name, execution.ID)
	exitCode, runErr := runner.Execute(ctx, job, logFile)

	endTime := time.Now().UTC()
	execution.EndTime = sql.NullTime{Time: endTime, Valid: true}
	execution.ExitCode = sql.NullInt64{Int64: int64(exitCode), Valid: true}

	switch {
	case ctx.Err() == context.Canceled:
		execution.Status = models.ExecStatusCanceled
	case ctx.Err() == context.DeadlineExceeded:
		execution.Status = models.ExecStatusFailed
		fmt.Fprintf(logFile, "\n[hermes] Job timed out after %d seconds\n", job.Timeout)
	case runErr != nil || exitCode != 0:
		execution.Status = models.ExecStatusFailed
		if runErr != nil {
			fmt.Fprintf(logFile, "\n[hermes] Execution error: %v\n", runErr)
		}
	default:
		execution.Status = models.ExecStatusSuccess
	}

	e.db.UpdateExecution(execution)
	e.db.UpdateJobLastRun(job.ID, execution.StartTime, string(execution.Status))
	log.Printf("Job %d execution %d: %s (exit %d)", job.ID, execution.ID, execution.Status, exitCode)
}

func (e *Executor) Cancel(executionID int64) bool {
	e.mu.Lock()
	cancel, ok := e.running[executionID]
	e.mu.Unlock()
	if ok {
		cancel()
		return true
	}
	return false
}

func (e *Executor) IsJobRunning(jobID int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.jobLocks[jobID]
}
