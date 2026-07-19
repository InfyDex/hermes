package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/api"
	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/database"
	"github.com/hermes-scheduler/hermes/internal/executor"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/scheduler"
	"github.com/hermes-scheduler/hermes/internal/testutil"
)

func newTestAPI(t *testing.T) (*mux.Router, *database.DB, *executor.Executor) {
	t.Helper()
	db, exec, sched, _ := testutil.TestStack(t)
	_ = sched.Start()
	t.Cleanup(func() { sched.Stop() })

	apiHandler := api.New(db, sched, exec)
	router := mux.NewRouter()
	router.Use(auth.BasicAuthMiddleware(testutil.TestAuthConfig()))
	apiHandler.RegisterRoutes(router)
	return router, db, exec
}

func authRequest(method, target string, body []byte) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.SetBasicAuth("admin", "secret")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestAPIJobLifecycle(t *testing.T) {
	router, _, _ := newTestAPI(t)

	createBody, _ := json.Marshal(map[string]string{
		"name":      "api-job",
		"cron_expr": "0 0 0 * * *",
		"command":   "echo api",
	})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, "/jobs", createBody))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}

	var job models.Job
	if err := json.NewDecoder(rr.Body).Decode(&job); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, "/jobs", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, fmt.Sprintf("/jobs/%d", job.ID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d", rr.Code)
	}

	updateBody, _ := json.Marshal(map[string]string{
		"name":      "api-job-updated",
		"cron_expr": "0 0 12 * * *",
		"command":   "echo updated",
		"status":    string(models.JobStatusEnabled),
	})
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPut, fmt.Sprintf("/jobs/%d", job.ID), updateBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, fmt.Sprintf("/jobs/%d/run", job.ID), nil))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("run status=%d", rr.Code)
	}
	time.Sleep(150 * time.Millisecond)

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, fmt.Sprintf("/jobs/%d/executions", job.ID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("executions status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodDelete, fmt.Sprintf("/jobs/%d", job.ID), nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rr.Code)
	}
}

func TestAPIValidationErrors(t *testing.T) {
	router, _, _ := newTestAPI(t)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, "/jobs", []byte(`{}`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("create bad status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, "/jobs/not-a-number", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad id status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, "/jobs/9999", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d", rr.Code)
	}
}

func TestAPIUnauthorized(t *testing.T) {
	db, exec, sched, _ := testutil.TestStack(t)
	_ = sched.Start()
	t.Cleanup(func() { sched.Stop() })
	apiHandler := api.New(db, sched, exec)
	router := mux.NewRouter()
	router.Use(auth.BasicAuthMiddleware(testutil.TestAuthConfig()))
	apiHandler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAPIExecutionLogsAndCancel(t *testing.T) {
	router, db, exec := newTestAPI(t)

	job := &models.Job{
		Name:       "log-job",
		CronExpr:   "0 * * * * *",
		RunnerType: models.RunnerTypeShell,
		Command:    "echo logline",
		EnvVars:    "{}",
		Status:     models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)
	exec.Run(job, "manual")
	time.Sleep(200 * time.Millisecond)

	execs, _ := db.ListExecutions(job.ID, 1)
	if len(execs) != 1 {
		t.Fatal("expected execution")
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, fmt.Sprintf("/executions/%d/logs", execs[0].ID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("logs status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, "/executions/9999/cancel", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cancel missing status=%d", rr.Code)
	}
}

func TestAPIListExecutionsWithLimit(t *testing.T) {
	router, db, _ := newTestAPI(t)
	job := &models.Job{
		Name: "limit-job", CronExpr: "0 * * * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, fmt.Sprintf("/jobs/%d/executions?limit=10", job.ID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAPIListExecutionsInvalidLimit(t *testing.T) {
	router, db, _ := newTestAPI(t)
	job := &models.Job{
		Name: "limit-job", CronExpr: "0 * * * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, fmt.Sprintf("/jobs/%d/executions?limit=999", job.ID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAPIGetJobWithNextRun(t *testing.T) {
	router, db, _, sched := newTestAPIWithSched(t)
	_ = sched.Start()
	t.Cleanup(func() { sched.Stop() })

	job := &models.Job{
		Name: "next-run", CronExpr: "0 0 0 * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)
	_ = sched.AddJob(job)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, fmt.Sprintf("/jobs/%d", job.ID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func newTestAPIWithSched(t *testing.T) (*mux.Router, *database.DB, *executor.Executor, *scheduler.Scheduler) {
	t.Helper()
	db, exec, sched, _ := testutil.TestStack(t)
	apiHandler := api.New(db, sched, exec)
	router := mux.NewRouter()
	router.Use(auth.BasicAuthMiddleware(testutil.TestAuthConfig()))
	apiHandler.RegisterRoutes(router)
	return router, db, exec, sched
}

func TestAPICancelRunningExecution(t *testing.T) {
	router, db, exec := newTestAPI(t)

	job := &models.Job{
		Name: "cancel-api", CronExpr: "0 * * * * *", RunnerType: models.RunnerTypeShell,
		Command: "sleep 3", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)
	go exec.Run(job, "manual")
	time.Sleep(100 * time.Millisecond)

	execs, _ := db.ListExecutions(job.ID, 1)
	if len(execs) != 1 {
		t.Fatal("expected execution")
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, fmt.Sprintf("/executions/%d/cancel", execs[0].ID), nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("cancel status=%d", rr.Code)
	}
}

func TestAPIUpdateJobInvalidJSON(t *testing.T) {
	router, db, _ := newTestAPI(t)
	job := &models.Job{
		Name: "json-job", CronExpr: "0 0 0 * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPut, fmt.Sprintf("/jobs/%d", job.ID), []byte(`{`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAPIUpdateJobDisable(t *testing.T) {
	router, db, _ := newTestAPI(t)

	job := &models.Job{
		Name: "disable-me", CronExpr: "0 0 0 * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = db.CreateJob(job)

	updateBody, _ := json.Marshal(map[string]string{
		"name":      "disable-me",
		"cron_expr": "0 0 0 * * *",
		"command":   "echo",
		"status":    string(models.JobStatusDisabled),
	})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPut, fmt.Sprintf("/jobs/%d", job.ID), updateBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAPIInvalidJSON(t *testing.T) {
	router, _, _ := newTestAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, "/jobs", []byte(`not json`)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAPIUpdateJobNotFound(t *testing.T) {
	router, _, _ := newTestAPI(t)
	body, _ := json.Marshal(map[string]string{
		"name": "x", "cron_expr": "0 0 0 * * *", "command": "echo",
	})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPut, "/jobs/9999", body))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAPIGetExecutionLogsNotFound(t *testing.T) {
	router, _, _ := newTestAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodGet, "/executions/9999/logs", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestAPIRunJobNotFound(t *testing.T) {
	router, _, _ := newTestAPI(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authRequest(http.MethodPost, "/jobs/9999/run", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}
