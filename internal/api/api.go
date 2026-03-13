package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/executor"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/scheduler"
)

type API struct {
	db        *database.DB
	scheduler *scheduler.Scheduler
	executor  *executor.Executor
}

func New(db *database.DB, sched *scheduler.Scheduler, exec *executor.Executor) *API {
	return &API{db: db, scheduler: sched, executor: exec}
}

func (a *API) RegisterRoutes(r *mux.Router) {
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/jobs", a.listJobs).Methods("GET")
	api.HandleFunc("/jobs", a.createJob).Methods("POST")
	api.HandleFunc("/jobs/{id}", a.getJob).Methods("GET")
	api.HandleFunc("/jobs/{id}", a.updateJob).Methods("PUT")
	api.HandleFunc("/jobs/{id}", a.deleteJob).Methods("DELETE")
	api.HandleFunc("/jobs/{id}/run", a.runJob).Methods("POST")
	api.HandleFunc("/jobs/{id}/executions", a.listExecutions).Methods("GET")
	api.HandleFunc("/executions/{id}/cancel", a.cancelExecution).Methods("POST")
	api.HandleFunc("/executions/{id}/logs", a.getExecutionLogs).Methods("GET")
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.db.ListJobs()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []models.Job{}
	}
	jsonResponse(w, jobs, http.StatusOK)
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	job, err := a.db.GetJob(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if next := a.scheduler.NextRun(job.ID); next != nil {
		job.NextRunAt = next
	}
	jsonResponse(w, job, http.StatusOK)
}

func (a *API) createJob(w http.ResponseWriter, r *http.Request) {
	var job models.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if job.Name == "" || job.CronExpr == "" || job.Command == "" {
		jsonError(w, "name, cron_expr, and command are required", http.StatusBadRequest)
		return
	}
	if job.RunnerType == "" {
		job.RunnerType = models.RunnerTypeShell
	}
	if job.Status == "" {
		job.Status = models.JobStatusEnabled
	}
	if job.EnvVars == "" {
		job.EnvVars = "{}"
	}
	if err := a.db.CreateJob(&job); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job.Status == models.JobStatusEnabled {
		if err := a.scheduler.AddJob(&job); err != nil {
			log.Printf("Failed to schedule job %d: %v", job.ID, err)
		}
	}
	jsonResponse(w, job, http.StatusCreated)
}

func (a *API) updateJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	existing, err := a.db.GetJob(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	var job models.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	job.ID = id
	job.CreatedAt = existing.CreatedAt
	if job.EnvVars == "" {
		job.EnvVars = "{}"
	}
	if err := a.db.UpdateJob(&job); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job.Status == models.JobStatusEnabled {
		if err := a.scheduler.AddJob(&job); err != nil {
			log.Printf("Failed to reschedule job %d: %v", job.ID, err)
		}
	} else {
		a.scheduler.RemoveJob(job.ID)
	}
	jsonResponse(w, job, http.StatusOK)
}

func (a *API) deleteJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	a.scheduler.RemoveJob(id)
	if err := a.db.DeleteJob(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) runJob(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	job, err := a.db.GetJob(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	go a.executor.Run(job, "manual")
	jsonResponse(w, map[string]string{"status": "started"}, http.StatusAccepted)
}

func (a *API) listExecutions(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	execs, err := a.db.ListExecutions(id, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if execs == nil {
		execs = []models.Execution{}
	}
	jsonResponse(w, execs, http.StatusOK)
}

func (a *API) cancelExecution(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if a.executor.Cancel(id) {
		jsonResponse(w, map[string]string{"status": "canceled"}, http.StatusOK)
	} else {
		jsonError(w, "execution not running", http.StatusNotFound)
	}
}

func (a *API) getExecutionLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	exec, err := a.db.GetExecution(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if exec == nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	data, err := os.ReadFile(exec.LogPath)
	if err != nil {
		jsonError(w, "log file not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
}

func jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, message string, status int) {
	jsonResponse(w, map[string]string{"error": message}, status)
}
