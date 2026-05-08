package models

import (
	"database/sql"
	"time"
)

type JobStatus string

const (
	JobStatusEnabled  JobStatus = "enabled"
	JobStatusDisabled JobStatus = "disabled"
)

type RunnerType string

const (
	RunnerTypeShell  RunnerType = "shell"
	RunnerTypeDocker RunnerType = "docker"
)

type Job struct {
	ID              int64      `json:"id"`
	PredefinedJobID string     `json:"predefined_job_id"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	CronExpr        string     `json:"cron_expr"`
	RunnerType      RunnerType `json:"runner_type"`
	Command         string     `json:"command"`
	WorkingDir    string     `json:"working_dir"`
	EnvVars       string     `json:"env_vars"`
	Timeout       int        `json:"timeout"`
	AllowParallel bool       `json:"allow_parallel"`
	Status        JobStatus  `json:"status"`

	// Notification settings
	NotifyOnStart   bool `json:"notify_on_start"`
	NotifyOnSuccess bool `json:"notify_on_success"`
	NotifyOnFailure bool `json:"notify_on_failure"`
	NotifyOnCancel  bool `json:"notify_on_cancel"`
	NotifyWeb       bool `json:"notify_web"`
	NotifyDiscord   bool `json:"notify_discord"`
	NotifyEmail     bool `json:"notify_email"`

	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus *string    `json:"last_run_status,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
}

type ExecutionStatus string

const (
	ExecStatusRunning  ExecutionStatus = "running"
	ExecStatusSuccess  ExecutionStatus = "success"
	ExecStatusFailed   ExecutionStatus = "failed"
	ExecStatusCanceled ExecutionStatus = "canceled"
	ExecStatusSkipped  ExecutionStatus = "skipped"
)

type Execution struct {
	ID        int64           `json:"id"`
	JobID     int64           `json:"job_id"`
	JobName   string          `json:"job_name,omitempty"`
	StartTime time.Time       `json:"start_time"`
	EndTime   sql.NullTime    `json:"end_time"`
	ExitCode  sql.NullInt64   `json:"exit_code"`
	Status    ExecutionStatus `json:"status"`
	LogPath   string          `json:"log_path"`
	Trigger   string          `json:"trigger"`
}

type Notification struct {
	ID        int64     `json:"id"`
	JobID     int64     `json:"job_id"`
	JobName   string    `json:"job_name"`
	Level     string    `json:"level"` // "info", "success", "error", "warning"
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	IsRead    bool      `json:"is_read"`
}
