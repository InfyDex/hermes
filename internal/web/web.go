package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/executor"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/scheduler"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Web struct {
	db        *database.DB
	scheduler *scheduler.Scheduler
	executor  *executor.Executor
	templates map[string]*template.Template
}

func New(db *database.DB, sched *scheduler.Scheduler, exec *executor.Executor) *Web {
	w := &Web{
		db:        db,
		scheduler: sched,
		executor:  exec,
		templates: make(map[string]*template.Template),
	}
	w.loadTemplates()
	return w
}

func (w *Web) loadTemplates() {
	funcMap := template.FuncMap{
		"formatTime": func(t *time.Time) string {
			if t == nil {
				return "-"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"statusClass": func(s *string) string {
			if s == nil {
				return ""
			}
			switch *s {
			case "success":
				return "success"
			case "failed":
				return "failed"
			case "running":
				return "running"
			case "canceled":
				return "canceled"
			default:
				return "disabled"
			}
		},
		"execStatusClass": func(s models.ExecutionStatus) string {
			switch s {
			case models.ExecStatusSuccess:
				return "success"
			case models.ExecStatusFailed:
				return "failed"
			case models.ExecStatusRunning:
				return "running"
			case models.ExecStatusCanceled:
				return "canceled"
			default:
				return "disabled"
			}
		},
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"duration": func(start, end time.Time) string {
			d := end.Sub(start)
			if d < time.Second {
				return fmt.Sprintf("%dms", d.Milliseconds())
			}
			if d < time.Minute {
				return fmt.Sprintf("%.1fs", d.Seconds())
			}
			return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
		},
	}

	pages := []string{"dashboard", "job_form", "job_detail", "logs"}
	for _, page := range pages {
		t, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+page+".html")
		if err != nil {
			log.Fatalf("Failed to parse template %s: %v", page, err)
		}
		w.templates[page] = t
	}
}

func (w *Web) render(wr http.ResponseWriter, name string, data interface{}) {
	t, ok := w.templates[name]
	if !ok {
		http.Error(wr, "template not found", http.StatusInternalServerError)
		return
	}
	wr.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(wr, "layout", data); err != nil {
		log.Printf("Template error: %v", err)
	}
}

func (w *Web) RegisterRoutes(r *mux.Router) {
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.HandleFunc("/", w.dashboard).Methods("GET")
	r.HandleFunc("/jobs/new", w.newJob).Methods("GET")
	r.HandleFunc("/jobs/new", w.createJob).Methods("POST")
	r.HandleFunc("/jobs/{id}", w.jobDetail).Methods("GET")
	r.HandleFunc("/jobs/{id}/edit", w.editJob).Methods("GET")
	r.HandleFunc("/jobs/{id}/edit", w.updateJob).Methods("POST")
	r.HandleFunc("/jobs/{id}/run", w.runJob).Methods("POST")
	r.HandleFunc("/jobs/{id}/delete", w.deleteJob).Methods("POST")
	r.HandleFunc("/executions/{id}/logs", w.viewLogs).Methods("GET")
	r.HandleFunc("/executions/{id}/cancel", w.cancelExecution).Methods("POST")

	// Notifications API
	r.HandleFunc("/api/notifications", w.getNotifications).Methods("GET")
	r.HandleFunc("/api/notifications/read", w.markNotificationsRead).Methods("POST")
}

func (w *Web) dashboard(wr http.ResponseWriter, r *http.Request) {
	jobs, err := w.db.ListJobs()
	if err != nil {
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range jobs {
		if next := w.scheduler.NextRun(jobs[i].ID); next != nil {
			jobs[i].NextRunAt = next
		}
	}
	w.render(wr, "dashboard", map[string]interface{}{"Title": "Dashboard", "Jobs": jobs})
}

func (w *Web) newJob(wr http.ResponseWriter, r *http.Request) {
	w.render(wr, "job_form", map[string]interface{}{
		"Title": "New Job",
		"Job":   &models.Job{RunnerType: models.RunnerTypeShell, Status: models.JobStatusEnabled, EnvVars: "{}"},
	})
}

func (w *Web) createJob(wr http.ResponseWriter, r *http.Request) {
	job := w.parseJobForm(r)
	if err := w.db.CreateJob(job); err != nil {
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}
	if job.Status == models.JobStatusEnabled {
		w.scheduler.AddJob(job)
	}
	http.Redirect(wr, r, fmt.Sprintf("/jobs/%d", job.ID), http.StatusSeeOther)
}

func (w *Web) jobDetail(wr http.ResponseWriter, r *http.Request) {
	id, err := webParseID(r)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	job, err := w.db.GetJob(id)
	if err != nil || job == nil {
		http.Error(wr, "not found", http.StatusNotFound)
		return
	}
	if next := w.scheduler.NextRun(job.ID); next != nil {
		job.NextRunAt = next
	}
	execs, _ := w.db.ListExecutions(job.ID, 50)
	w.render(wr, "job_detail", map[string]interface{}{"Title": job.Name, "Job": job, "Executions": execs})
}

func (w *Web) editJob(wr http.ResponseWriter, r *http.Request) {
	id, err := webParseID(r)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	job, err := w.db.GetJob(id)
	if err != nil || job == nil {
		http.Error(wr, "not found", http.StatusNotFound)
		return
	}
	w.render(wr, "job_form", map[string]interface{}{"Title": "Edit " + job.Name, "Job": job})
}

func (w *Web) updateJob(wr http.ResponseWriter, r *http.Request) {
	id, err := webParseID(r)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	existing, err := w.db.GetJob(id)
	if err != nil || existing == nil {
		http.Error(wr, "not found", http.StatusNotFound)
		return
	}
	job := w.parseJobForm(r)
	job.ID = id
	job.CreatedAt = existing.CreatedAt
	if err := w.db.UpdateJob(job); err != nil {
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}
	if job.Status == models.JobStatusEnabled {
		w.scheduler.AddJob(job)
	} else {
		w.scheduler.RemoveJob(job.ID)
	}
	http.Redirect(wr, r, fmt.Sprintf("/jobs/%d", job.ID), http.StatusSeeOther)
}

func (w *Web) runJob(wr http.ResponseWriter, r *http.Request) {
	id, err := webParseID(r)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	job, err := w.db.GetJob(id)
	if err != nil || job == nil {
		http.Error(wr, "not found", http.StatusNotFound)
		return
	}
	go w.executor.Run(job, "manual")
	http.Redirect(wr, r, fmt.Sprintf("/jobs/%d", job.ID), http.StatusSeeOther)
}

func (w *Web) deleteJob(wr http.ResponseWriter, r *http.Request) {
	id, err := webParseID(r)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	w.scheduler.RemoveJob(id)
	w.db.DeleteJob(id)
	http.Redirect(wr, r, "/", http.StatusSeeOther)
}

func (w *Web) viewLogs(wr http.ResponseWriter, r *http.Request) {
	id, err := webParseID(r)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	exec, err := w.db.GetExecution(id)
	if err != nil || exec == nil {
		http.Error(wr, "not found", http.StatusNotFound)
		return
	}
	logs := ""
	if data, err := os.ReadFile(exec.LogPath); err == nil {
		logs = string(data)
	}
	w.render(wr, "logs", map[string]interface{}{
		"Title": fmt.Sprintf("Execution #%d", exec.ID), "Execution": exec, "Logs": logs,
	})
}

func (w *Web) cancelExecution(wr http.ResponseWriter, r *http.Request) {
	id, err := webParseID(r)
	if err != nil {
		http.Error(wr, "invalid id", http.StatusBadRequest)
		return
	}
	w.executor.Cancel(id)
	exec, err := w.db.GetExecution(id)
	if err != nil || exec == nil {
		http.Redirect(wr, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(wr, r, fmt.Sprintf("/jobs/%d", exec.JobID), http.StatusSeeOther)
}

func webParseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
}

func (w *Web) parseJobForm(r *http.Request) *models.Job {
	r.ParseForm()
	timeout, _ := strconv.Atoi(r.FormValue("timeout"))
	envVars := r.FormValue("env_vars")
	if envVars == "" {
		envVars = "{}"
	}
	return &models.Job{
		Name:            r.FormValue("name"),
		Description:     r.FormValue("description"),
		CronExpr:        r.FormValue("cron_expr"),
		RunnerType:      models.RunnerType(r.FormValue("runner_type")),
		Command:         r.FormValue("command"),
		WorkingDir:      r.FormValue("working_dir"),
		EnvVars:         envVars,
		Timeout:         timeout,
		AllowParallel:   r.FormValue("allow_parallel") == "true",
		Status:          models.JobStatus(r.FormValue("status")),
		NotifyOnStart:   r.FormValue("notify_on_start") == "true",
		NotifyOnSuccess: r.FormValue("notify_on_success") == "true",
		NotifyOnFailure: r.FormValue("notify_on_failure") == "true",
		NotifyOnCancel:  r.FormValue("notify_on_cancel") == "true",
		NotifyWeb:       r.FormValue("notify_web") == "true",
		NotifyDiscord:   r.FormValue("notify_discord") == "true",
		NotifyEmail:     r.FormValue("notify_email") == "true",
	}
}

func (w *Web) getNotifications(wr http.ResponseWriter, r *http.Request) {
	notifs, err := w.db.GetUnreadNotifications()
	if err != nil {
		http.Error(wr, "Failed to get notifications", http.StatusInternalServerError)
		return
	}

	wr.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(wr).Encode(notifs); err != nil {
		http.Error(wr, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (w *Web) markNotificationsRead(wr http.ResponseWriter, r *http.Request) {
	if err := w.db.MarkAllNotificationsRead(); err != nil {
		http.Error(wr, "Failed to mark as read", http.StatusInternalServerError)
		return
	}
	wr.WriteHeader(http.StatusOK)
}
