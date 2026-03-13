package scheduler

import (
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/executor"
	"github.com/hermes-scheduler/hermes/internal/models"
)

type Scheduler struct {
	cron     *cron.Cron
	db       *database.DB
	executor *executor.Executor

	mu      sync.Mutex
	entries map[int64]cron.EntryID
}

func New(db *database.DB, exec *executor.Executor) *Scheduler {
	c := cron.New(cron.WithSeconds(), cron.WithChain(cron.Recover(cron.DefaultLogger)))
	return &Scheduler{
		cron:     c,
		db:       db,
		executor: exec,
		entries:  make(map[int64]cron.EntryID),
	}
}

func (s *Scheduler) Start() error {
	jobs, err := s.db.GetEnabledJobs()
	if err != nil {
		return err
	}
	for i := range jobs {
		if err := s.addJob(&jobs[i]); err != nil {
			log.Printf("Failed to schedule job %d (%s): %v", jobs[i].ID, jobs[i].Name, err)
		}
	}
	s.cron.Start()
	log.Printf("Scheduler started with %d jobs", len(jobs))
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}

func (s *Scheduler) AddJob(job *models.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[job.ID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, job.ID)
	}
	if job.Status != models.JobStatusEnabled {
		return nil
	}
	return s.addJobLocked(job)
}

func (s *Scheduler) RemoveJob(jobID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[jobID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, jobID)
	}
}

func (s *Scheduler) addJob(job *models.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addJobLocked(job)
}

func (s *Scheduler) addJobLocked(job *models.Job) error {
	jobCopy := *job
	entryID, err := s.cron.AddFunc(jobCopy.CronExpr, func() {
		current, err := s.db.GetJob(jobCopy.ID)
		if err != nil || current == nil || current.Status != models.JobStatusEnabled {
			return
		}
		s.executor.Run(current, "schedule")
	})
	if err != nil {
		return err
	}
	s.entries[job.ID] = entryID
	s.updateNextRun(job.ID, entryID)
	log.Printf("Scheduled job %d (%s): %s", job.ID, job.Name, job.CronExpr)
	return nil
}

func (s *Scheduler) updateNextRun(jobID int64, entryID cron.EntryID) {
	entry := s.cron.Entry(entryID)
	if !entry.Next.IsZero() {
		next := entry.Next.UTC()
		s.db.UpdateJobNextRun(jobID, &next)
	}
}

func (s *Scheduler) NextRun(jobID int64) *time.Time {
	s.mu.Lock()
	entryID, ok := s.entries[jobID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	entry := s.cron.Entry(entryID)
	if entry.Next.IsZero() {
		return nil
	}
	t := entry.Next.UTC()
	return &t
}
