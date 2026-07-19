package web_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"

	"github.com/hermes-scheduler/hermes/internal/auth"
	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/testutil"
	"github.com/hermes-scheduler/hermes/internal/web"
)

func newTestWeb(t *testing.T) (*mux.Router, *auth.SessionStore, *testutil.WebDeps) {
	t.Helper()
	db, exec, sched, _ := testutil.TestStack(t)
	_ = sched.Start()
	t.Cleanup(func() { sched.Stop() })

	store := testutil.TestSessionStore(t)
	limiter := auth.NewLoginRateLimiter()
	w := web.New(db, sched, exec, store, limiter, testutil.TestAuthConfig(), false)

	root := mux.NewRouter()
	w.RegisterPublicRoutes(root)

	webRouter := root.NewRoute().Subrouter()
	webRouter.Use(auth.SessionMiddleware(store))
	w.RegisterRoutes(webRouter)

	return root, store, &testutil.WebDeps{DB: db, Exec: exec, Sched: sched}
}

func authenticatedRequest(t *testing.T, store *auth.SessionStore, method, target string, body string) *http.Request {
	t.Helper()
	saveRR := httptest.NewRecorder()
	session, err := store.NewSession("admin", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := store.Save(saveRR, session, false); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	for _, c := range saveRR.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

func TestLoginPage(t *testing.T) {
	router, _, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/login", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Sign In") {
		t.Fatal("expected login page")
	}
}

func TestLoginSubmitSuccess(t *testing.T) {
	router, store, _ := newTestWeb(t)

	loginRR := httptest.NewRecorder()
	router.ServeHTTP(loginRR, httptest.NewRequest(http.MethodGet, "/login", nil))
	csrf := extractCSRFFromBody(t, loginRR.Body.String())

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range loginRR.Result().Cookies() {
		req.AddCookie(c)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	req2 := authenticatedRequest(t, store, http.MethodGet, "/", "")
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d", rr2.Code)
	}
}

func TestLoginSubmitInvalidCredentials(t *testing.T) {
	router, _, _ := newTestWeb(t)
	loginRR := httptest.NewRecorder()
	router.ServeHTTP(loginRR, httptest.NewRequest(http.MethodGet, "/login", nil))
	csrf := extractCSRFFromBody(t, loginRR.Body.String())

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "wrong")
	form.Set("csrf_token", csrf)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range loginRR.Result().Cookies() {
		req.AddCookie(c)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Invalid username or password") {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestDashboardRequiresAuth(t *testing.T) {
	router, _, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCreateJobViaWeb(t *testing.T) {
	router, store, deps := newTestWeb(t)

	form := url.Values{}
	form.Set("name", "web-job")
	form.Set("cron_expr", "0 0 0 * * *")
	form.Set("command", "echo web")
	form.Set("runner_type", string(models.RunnerTypeShell))
	form.Set("status", string(models.JobStatusEnabled))
	form.Set("env_vars", "{}")

	req := authenticatedRequest(t, store, http.MethodPost, "/jobs/new", form.Encode())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}

	jobs, err := deps.DB.ListJobs()
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%d err=%v", len(jobs), err)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, fmt.Sprintf("/jobs/%d", jobs[0].ID), ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("detail status=%d", rr.Code)
	}
}

func TestToggleAndDeleteJob(t *testing.T) {
	router, store, deps := newTestWeb(t)
	job := &models.Job{
		Name: "toggle-job", CronExpr: "0 0 0 * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = deps.DB.CreateJob(job)
	deps.Sched.AddJob(job)

	req := authenticatedRequest(t, store, http.MethodPost, fmt.Sprintf("/jobs/%d/toggle", job.ID), "")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("toggle status=%d", rr.Code)
	}

	req = authenticatedRequest(t, store, http.MethodPost, fmt.Sprintf("/jobs/%d/delete", job.ID), "")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("delete status=%d", rr.Code)
	}
}

func TestNewJobPage(t *testing.T) {
	router, store, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, "/jobs/new", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestCreatePredefinedJob(t *testing.T) {
	router, store, deps := newTestWeb(t)

	form := url.Values{}
	form.Set("predefined_job_id", "docker_cleanup")
	form.Set("name", "docker-clean")
	form.Set("cron_expr", "0 0 2 * * *")
	form.Set("command", "/app/scripts/docker-cleanup.sh")
	form.Set("runner_type", string(models.RunnerTypeShell))
	form.Set("status", string(models.JobStatusEnabled))
	form.Set("env_vars", "{}")
	form.Set("script_content", "#!/bin/sh\necho cleanup\n")

	req := authenticatedRequest(t, store, http.MethodPost, "/jobs/new", form.Encode())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d body=%s", rr.Code, rr.Body.String())
	}

	jobs, _ := deps.DB.ListJobs()
	if len(jobs) != 1 || jobs[0].PredefinedJobID != "docker_cleanup" {
		t.Fatalf("jobs=%+v", jobs)
	}
}

func TestDeletePredefinedJob(t *testing.T) {
	router, store, deps := newTestWeb(t)
	job := &models.Job{
		Name: "del-predef", CronExpr: "0 0 0 * * *", RunnerType: models.RunnerTypeShell,
		PredefinedJobID: "docker_cleanup", Command: "/data/scripts/job_99_script.sh",
		EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = deps.DB.CreateJob(job)

	req := authenticatedRequest(t, store, http.MethodPost, fmt.Sprintf("/jobs/%d/delete", job.ID), "")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("delete status=%d", rr.Code)
	}
}

func TestCancelExecutionViaWeb(t *testing.T) {
	router, store, deps := newTestWeb(t)
	job := &models.Job{
		Name: "cancel-web", CronExpr: "0 * * * * *", RunnerType: models.RunnerTypeShell,
		Command: "sleep 3", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = deps.DB.CreateJob(job)
	go deps.Exec.Run(job, "manual")
	time.Sleep(100 * time.Millisecond)

	execs, _ := deps.DB.ListExecutions(job.ID, 1)
	if len(execs) != 1 {
		t.Fatal("expected execution")
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodPost, fmt.Sprintf("/executions/%d/cancel", execs[0].ID), ""))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("cancel status=%d", rr.Code)
	}
	time.Sleep(200 * time.Millisecond)
}

func TestJobDetailNotFound(t *testing.T) {
	router, store, _ := newTestWeb(t)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, "/jobs/9999", ""))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestLoginRedirectWhenAuthenticated(t *testing.T) {
	router, store, _ := newTestWeb(t)
	saveRR := httptest.NewRecorder()
	session, _ := store.NewSession("admin", false)
	_ = store.Save(saveRR, session, false)
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	for _, c := range saveRR.Result().Cookies() {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestLoginCSRFInvalid(t *testing.T) {
	router, _, _ := newTestWeb(t)
	loginRR := httptest.NewRecorder()
	router.ServeHTTP(loginRR, httptest.NewRequest(http.MethodGet, "/login", nil))

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")
	form.Set("csrf_token", "wrong-token")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range loginRR.Result().Cookies() {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Invalid username or password") {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestNotificationsEndpoints(t *testing.T) {
	router, store, deps := newTestWeb(t)
	_ = deps.DB.InsertNotification(0, "info", "hello")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, "/notifications", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("get notifications status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodPost, "/notifications/read", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("mark read status=%d", rr.Code)
	}
}

func TestRunJobViaWeb(t *testing.T) {
	router, store, deps := newTestWeb(t)
	job := &models.Job{
		Name: "run-web", CronExpr: "0 * * * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo run", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = deps.DB.CreateJob(job)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodPost, fmt.Sprintf("/jobs/%d/run", job.ID), ""))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("run status=%d", rr.Code)
	}
	time.Sleep(150 * time.Millisecond)
}

func TestEditJobPage(t *testing.T) {
	router, store, deps := newTestWeb(t)
	job := &models.Job{
		Name: "edit-job", CronExpr: "0 0 0 * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = deps.DB.CreateJob(job)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, fmt.Sprintf("/jobs/%d/edit", job.ID), ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("edit status=%d", rr.Code)
	}

	form := url.Values{}
	form.Set("name", "edited")
	form.Set("cron_expr", "0 0 12 * * *")
	form.Set("command", "echo edited")
	form.Set("runner_type", string(models.RunnerTypeShell))
	form.Set("status", string(models.JobStatusEnabled))
	form.Set("env_vars", "{}")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodPost, fmt.Sprintf("/jobs/%d/edit", job.ID), form.Encode()))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("update status=%d", rr.Code)
	}
}

func TestViewExecutionLogs(t *testing.T) {
	router, store, deps := newTestWeb(t)
	job := &models.Job{
		Name: "logs-job", CronExpr: "0 * * * * *", RunnerType: models.RunnerTypeShell,
		Command: "echo logtest", EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = deps.DB.CreateJob(job)
	deps.Exec.Run(job, "manual")
	time.Sleep(200 * time.Millisecond)

	execs, _ := deps.DB.ListExecutions(job.ID, 1)
	if len(execs) != 1 {
		t.Fatal("expected execution")
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, fmt.Sprintf("/executions/%d/logs", execs[0].ID), ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("view logs status=%d", rr.Code)
	}

	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodGet, fmt.Sprintf("/executions/%d/logs/stream", execs[0].ID), ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("stream logs status=%d", rr.Code)
	}
}

func TestWebInvalidIDs(t *testing.T) {
	router, store, _ := newTestWeb(t)
	paths := []string{
		"/jobs/bad/edit",
		"/jobs/bad/run",
		"/jobs/bad/toggle",
		"/jobs/bad/delete",
		"/executions/bad/logs",
		"/executions/bad/logs/stream",
		"/executions/bad/cancel",
	}
	for _, path := range paths {
		method := http.MethodGet
		if strings.Contains(path, "/run") || strings.Contains(path, "/toggle") ||
			strings.Contains(path, "/delete") || strings.Contains(path, "/cancel") {
			method = http.MethodPost
		}
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, authenticatedRequest(t, store, method, path, ""))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status=%d", method, path, rr.Code)
		}
	}
}

func TestCreateJobWithAllNotifyFlags(t *testing.T) {
	router, store, deps := newTestWeb(t)

	form := url.Values{}
	form.Set("name", "notify-flags-job")
	form.Set("cron_expr", "0 0 0 * * *")
	form.Set("command", "echo notify")
	form.Set("runner_type", string(models.RunnerTypeShell))
	form.Set("status", string(models.JobStatusEnabled))
	form.Set("env_vars", `{"K":"V"}`)
	form.Set("allow_parallel", "true")
	form.Set("notify_on_start", "true")
	form.Set("notify_on_success", "true")
	form.Set("notify_on_failure", "true")
	form.Set("notify_on_cancel", "true")
	form.Set("notify_web", "true")
	form.Set("notify_discord", "true")
	form.Set("notify_email", "true")

	req := authenticatedRequest(t, store, http.MethodPost, "/jobs/new", form.Encode())
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	jobs, _ := deps.DB.ListJobs()
	if len(jobs) != 1 || !jobs[0].NotifyOnStart || !jobs[0].AllowParallel {
		t.Fatalf("job=%+v", jobs[0])
	}
}

func TestUpdatePredefinedJobWithScript(t *testing.T) {
	router, store, deps := newTestWeb(t)
	job := &models.Job{
		Name: "predef-edit", CronExpr: "0 0 0 * * *", RunnerType: models.RunnerTypeShell,
		PredefinedJobID: "docker_cleanup", Command: "/app/scripts/docker-cleanup.sh",
		EnvVars: "{}", Status: models.JobStatusEnabled,
	}
	_ = deps.DB.CreateJob(job)

	form := url.Values{}
	form.Set("predefined_job_id", "docker_cleanup")
	form.Set("name", "predef-edit")
	form.Set("cron_expr", "0 0 3 * * *")
	form.Set("command", "/app/scripts/docker-cleanup.sh")
	form.Set("runner_type", string(models.RunnerTypeShell))
	form.Set("status", string(models.JobStatusEnabled))
	form.Set("env_vars", "{}")
	form.Set("script_content", "#!/bin/sh\necho updated-script\n")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, authenticatedRequest(t, store, http.MethodPost, fmt.Sprintf("/jobs/%d/edit", job.ID), form.Encode()))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLogout(t *testing.T) {
	router, store, _ := newTestWeb(t)

	saveRR := httptest.NewRecorder()
	session, err := store.NewSession("admin", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := store.Save(saveRR, session, false); err != nil {
		t.Fatalf("Save: %v", err)
	}

	form := url.Values{}
	form.Set("csrf_token", session.CSRFToken)
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range saveRR.Result().Cookies() {
		req.AddCookie(c)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("logout status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestLogoutInvalidCSRF(t *testing.T) {
	router, store, _ := newTestWeb(t)

	saveRR := httptest.NewRecorder()
	session, err := store.NewSession("admin", false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := store.Save(saveRR, session, false); err != nil {
		t.Fatalf("Save: %v", err)
	}

	form := url.Values{}
	form.Set("csrf_token", "wrong")
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range saveRR.Result().Cookies() {
		req.AddCookie(c)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("logout status=%d", rr.Code)
	}
	_ = session
}

func extractCSRFFromBody(t *testing.T, body string) string {
	t.Helper()
	const marker = `name="csrf_token" value="`
	idx := strings.Index(body, marker)
	if idx < 0 {
		t.Fatal("csrf token not found in login page")
	}
	start := idx + len(marker)
	end := strings.Index(body[start:], `"`)
	if end < 0 {
		t.Fatal("csrf token end not found")
	}
	return body[start : start+end]
}
