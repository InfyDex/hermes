package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/hermes-scheduler/hermes/internal/models"
)

type DB struct {
	conn *sql.DB
}

func New(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		predefined_job_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		cron_expr TEXT NOT NULL,
		runner_type TEXT NOT NULL DEFAULT 'shell',
		command TEXT NOT NULL,
		working_dir TEXT NOT NULL DEFAULT '',
		env_vars TEXT NOT NULL DEFAULT '{}',
		timeout INTEGER NOT NULL DEFAULT 0,
		allow_parallel INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'enabled',
		notify_on_start INTEGER NOT NULL DEFAULT 0,
		notify_on_success INTEGER NOT NULL DEFAULT 0,
		notify_on_failure INTEGER NOT NULL DEFAULT 0,
		notify_on_cancel INTEGER NOT NULL DEFAULT 0,
		notify_web INTEGER NOT NULL DEFAULT 0,
		notify_discord INTEGER NOT NULL DEFAULT 0,
		notify_email INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_run_at DATETIME,
		last_run_status TEXT,
		next_run_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		exit_code INTEGER,
		status TEXT NOT NULL DEFAULT 'running',
		log_path TEXT NOT NULL DEFAULT '',
		trigger_type TEXT NOT NULL DEFAULT 'schedule',
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id INTEGER NOT NULL,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		is_read INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_executions_job_id ON executions(job_id);
	CREATE INDEX IF NOT EXISTS idx_executions_status ON executions(status);
	CREATE INDEX IF NOT EXISTS idx_notifications_job_id ON notifications(job_id);
	CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at);
	`
	_, err := db.conn.Exec(schema)
	if err != nil {
		return err
	}

	// Migrations for existing rows
	alterStatements := []string{
		"ALTER TABLE jobs ADD COLUMN notify_on_start INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN notify_on_success INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN notify_on_failure INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN notify_on_cancel INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN notify_web INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN notify_discord INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN notify_email INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE jobs ADD COLUMN predefined_job_id TEXT NOT NULL DEFAULT ''",
	}
	for _, stmt := range alterStatements {
		db.conn.Exec(stmt) // ignore errors if columns already exist
	}

	return nil
}

func (db *DB) ListJobs() ([]models.Job, error) {
	rows, err := db.conn.Query(`
		SELECT id, predefined_job_id, name, description, cron_expr, runner_type, command,
		       working_dir, env_vars, timeout, allow_parallel, status,
		       notify_on_start, notify_on_success, notify_on_failure, notify_on_cancel, notify_web, notify_discord, notify_email,
		       created_at, updated_at, last_run_at, last_run_status, next_run_at
		FROM jobs ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (db *DB) GetJob(id int64) (*models.Job, error) {
	row := db.conn.QueryRow(`
		SELECT id, predefined_job_id, name, description, cron_expr, runner_type, command,
		       working_dir, env_vars, timeout, allow_parallel, status,
		       notify_on_start, notify_on_success, notify_on_failure, notify_on_cancel, notify_web, notify_discord, notify_email,
		       created_at, updated_at, last_run_at, last_run_status, next_run_at
		FROM jobs WHERE id = ?`, id)
	j, err := scanJobRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &j, nil
}

func (db *DB) CreateJob(j *models.Job) error {
	now := time.Now().UTC()
	j.CreatedAt = now
	j.UpdatedAt = now
	result, err := db.conn.Exec(`
		INSERT INTO jobs (predefined_job_id, name, description, cron_expr, runner_type, command,
		                   working_dir, env_vars, timeout, allow_parallel, status,
		                  notify_on_start, notify_on_success, notify_on_failure, notify_on_cancel, notify_web, notify_discord, notify_email,
		                  created_at, updated_at, next_run_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.PredefinedJobID, j.Name, j.Description, j.CronExpr, j.RunnerType, j.Command,
		j.WorkingDir, j.EnvVars, j.Timeout, j.AllowParallel, j.Status,
		j.NotifyOnStart, j.NotifyOnSuccess, j.NotifyOnFailure, j.NotifyOnCancel, j.NotifyWeb, j.NotifyDiscord, j.NotifyEmail,
		j.CreatedAt, j.UpdatedAt, j.NextRunAt)
	if err != nil {
		return err
	}
	j.ID, err = result.LastInsertId()
	return err
}

func (db *DB) UpdateJob(j *models.Job) error {
	j.UpdatedAt = time.Now().UTC()
	_, err := db.conn.Exec(`
		UPDATE jobs SET predefined_job_id=?, name=?, description=?, cron_expr=?, runner_type=?, command=?,
		               working_dir=?, env_vars=?, timeout=?, allow_parallel=?, status=?,
		                notify_on_start=?, notify_on_success=?, notify_on_failure=?, notify_on_cancel=?, notify_web=?, notify_discord=?, notify_email=?,
		                updated_at=?, next_run_at=?
		WHERE id=?`,
		j.PredefinedJobID, j.Name, j.Description, j.CronExpr, j.RunnerType, j.Command,
		j.WorkingDir, j.EnvVars, j.Timeout, j.AllowParallel, j.Status,
		j.NotifyOnStart, j.NotifyOnSuccess, j.NotifyOnFailure, j.NotifyOnCancel, j.NotifyWeb, j.NotifyDiscord, j.NotifyEmail,
		j.UpdatedAt, j.NextRunAt, j.ID)
	return err
}

func (db *DB) DeleteJob(id int64) error {
	_, err := db.conn.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

func (db *DB) UpdateJobLastRun(id int64, at time.Time, status string) error {
	_, err := db.conn.Exec(`UPDATE jobs SET last_run_at=?, last_run_status=?, updated_at=? WHERE id=?`,
		at, status, time.Now().UTC(), id)
	return err
}

func (db *DB) UpdateJobNextRun(id int64, nextRun *time.Time) error {
	_, err := db.conn.Exec(`UPDATE jobs SET next_run_at=? WHERE id=?`, nextRun, id)
	return err
}

func (db *DB) CreateExecution(e *models.Execution) error {
	result, err := db.conn.Exec(`
		INSERT INTO executions (job_id, start_time, status, log_path, trigger_type)
		VALUES (?, ?, ?, ?, ?)`,
		e.JobID, e.StartTime, e.Status, e.LogPath, e.Trigger)
	if err != nil {
		return err
	}
	e.ID, err = result.LastInsertId()
	return err
}

func (db *DB) UpdateExecution(e *models.Execution) error {
	_, err := db.conn.Exec(`UPDATE executions SET end_time=?, exit_code=?, status=? WHERE id=?`,
		e.EndTime, e.ExitCode, e.Status, e.ID)
	return err
}

func (db *DB) GetExecution(id int64) (*models.Execution, error) {
	row := db.conn.QueryRow(`
		SELECT e.id, e.job_id, j.name, e.start_time, e.end_time, e.exit_code,
		       e.status, e.log_path, e.trigger_type
		FROM executions e LEFT JOIN jobs j ON j.id = e.job_id
		WHERE e.id = ?`, id)
	var e models.Execution
	err := row.Scan(&e.ID, &e.JobID, &e.JobName, &e.StartTime, &e.EndTime,
		&e.ExitCode, &e.Status, &e.LogPath, &e.Trigger)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (db *DB) ListExecutions(jobID int64, limit int) ([]models.Execution, error) {
	rows, err := db.conn.Query(`
		SELECT e.id, e.job_id, j.name, e.start_time, e.end_time, e.exit_code,
		       e.status, e.log_path, e.trigger_type
		FROM executions e LEFT JOIN jobs j ON j.id = e.job_id
		WHERE e.job_id = ? ORDER BY e.start_time DESC LIMIT ?`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []models.Execution
	for rows.Next() {
		var e models.Execution
		if err := rows.Scan(&e.ID, &e.JobID, &e.JobName, &e.StartTime, &e.EndTime,
			&e.ExitCode, &e.Status, &e.LogPath, &e.Trigger); err != nil {
			return nil, err
		}
		execs = append(execs, e)
	}
	return execs, rows.Err()
}

func (db *DB) GetEnabledJobs() ([]models.Job, error) {
	rows, err := db.conn.Query(`
		SELECT id, predefined_job_id, name, description, cron_expr, runner_type, command,
		       working_dir, env_vars, timeout, allow_parallel, status,
		       notify_on_start, notify_on_success, notify_on_failure, notify_on_cancel, notify_web, notify_discord, notify_email,
		       created_at, updated_at, last_run_at, last_run_status, next_run_at
		FROM jobs WHERE status = 'enabled' ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []models.Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanJobFields(s scanner) (models.Job, error) {
	var j models.Job
	var allowParallel int
	var nStart, nSuccess, nFail, nCancel, nWeb, nDiscord, nEmail int

	err := s.Scan(
		&j.ID, &j.PredefinedJobID, &j.Name, &j.Description, &j.CronExpr, &j.RunnerType, &j.Command,
		&j.WorkingDir, &j.EnvVars, &j.Timeout, &allowParallel, &j.Status,
		&nStart, &nSuccess, &nFail, &nCancel, &nWeb, &nDiscord, &nEmail,
		&j.CreatedAt, &j.UpdatedAt, &j.LastRunAt, &j.LastRunStatus, &j.NextRunAt)

	j.AllowParallel = allowParallel != 0
	j.NotifyOnStart = nStart != 0
	j.NotifyOnSuccess = nSuccess != 0
	j.NotifyOnFailure = nFail != 0
	j.NotifyOnCancel = nCancel != 0
	j.NotifyWeb = nWeb != 0
	j.NotifyDiscord = nDiscord != 0
	j.NotifyEmail = nEmail != 0

	return j, err
}

func scanJob(rows *sql.Rows) (models.Job, error) {
	return scanJobFields(rows)
}

func scanJobRow(row *sql.Row) (models.Job, error) {
	return scanJobFields(row)
}

func (db *DB) InsertNotification(jobID int64, level, message string) error {
	_, err := db.conn.Exec(`INSERT INTO notifications (job_id, level, message) VALUES (?, ?, ?)`, jobID, level, message)
	return err
}

func (db *DB) GetUnreadNotifications() ([]models.Notification, error) {
	rows, err := db.conn.Query(`
		SELECT n.id, n.job_id, j.name, n.level, n.message, n.created_at, n.is_read
		FROM notifications n
		LEFT JOIN jobs j ON n.job_id = j.id
		WHERE n.is_read = 0
		ORDER BY n.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifs []models.Notification
	for rows.Next() {
		var n models.Notification
		var jobName sql.NullString
		var isRead int
		if err := rows.Scan(&n.ID, &n.JobID, &jobName, &n.Level, &n.Message, &n.CreatedAt, &isRead); err != nil {
			return nil, err
		}
		if jobName.Valid {
			n.JobName = jobName.String
		} else if n.JobID == 0 {
			n.JobName = "System"
		} else {
			n.JobName = "Deleted Job"
		}
		n.IsRead = isRead != 0
		notifs = append(notifs, n)
	}
	return notifs, rows.Err()
}

func (db *DB) MarkAllNotificationsRead() error {
	_, err := db.conn.Exec(`UPDATE notifications SET is_read = 1 WHERE is_read = 0`)
	return err
}

func (db *DB) ClearOldNotifications(days int) error {
	_, err := db.conn.Exec(`DELETE FROM notifications WHERE created_at < datetime('now', ?)`, fmt.Sprintf("-%d days", days))
	return err
}
