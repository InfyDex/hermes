package runners_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hermes-scheduler/hermes/internal/models"
	"github.com/hermes-scheduler/hermes/internal/runners"
)

func TestRegistry(t *testing.T) {
	reg := runners.NewRegistry()
	shell := runners.NewShellRunner()
	reg.Register(shell)

	got, ok := reg.Get(models.RunnerTypeShell)
	if !ok || got.Type() != models.RunnerTypeShell {
		t.Fatalf("Get shell = %v, ok=%v", got, ok)
	}
	if _, ok := reg.Get(models.RunnerType("unknown")); ok {
		t.Fatal("expected unknown runner missing")
	}
}

func TestShellRunnerSuccess(t *testing.T) {
	r := runners.NewShellRunner()
	job := &models.Job{Command: "echo hello"}
	var buf bytes.Buffer
	code, err := r.Execute(context.Background(), job, &buf)
	if err != nil || code != 0 {
		t.Fatalf("Execute: code=%d err=%v", code, err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("hello")) {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestShellRunnerFailureExitCode(t *testing.T) {
	r := runners.NewShellRunner()
	job := &models.Job{Command: "exit 7"}
	var buf bytes.Buffer
	code, err := r.Execute(context.Background(), job, &buf)
	if err != nil || code != 7 {
		t.Fatalf("Execute: code=%d err=%v", code, err)
	}
}

func TestShellRunnerWithEnv(t *testing.T) {
	r := runners.NewShellRunner()
	job := &models.Job{
		Command: "echo $MY_VAR",
		EnvVars: `{"MY_VAR":"hermes"}`,
	}
	var buf bytes.Buffer
	code, err := r.Execute(context.Background(), job, &buf)
	if err != nil || code != 0 {
		t.Fatalf("Execute: code=%d err=%v", code, err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("hermes")) {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestShellRunnerTimeout(t *testing.T) {
	r := runners.NewShellRunner()
	job := &models.Job{Command: "sleep 10"}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var buf bytes.Buffer
	code, err := r.Execute(ctx, job, &buf)
	if err == nil && code == 0 {
		t.Fatal("expected timeout or non-zero exit")
	}
}

func TestDockerRunnerEmptyCommand(t *testing.T) {
	r := runners.NewDockerRunner()
	var buf bytes.Buffer
	code, err := r.Execute(context.Background(), &models.Job{Command: ""}, &buf)
	if err != nil || code != -1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestDockerRunnerType(t *testing.T) {
	r := runners.NewDockerRunner()
	if r.Type() != models.RunnerTypeDocker {
		t.Fatalf("type = %q", r.Type())
	}
}

func TestNormalizePredefinedScript(t *testing.T) {
	dir := "/data/scripts"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Skip("cannot create /data/scripts")
	}
	path := filepath.Join(dir, "job_1_script.sh")
	t.Cleanup(func() { _ = os.Remove(path) })

	content := []byte{0xEF, 0xBB, 0xBF, '#', '!', '/', 'b', 'i', 'n', '/', 's', 'h', '\r', '\n', 'e', 'c', 'h', 'o', 'h', 'i', '\r', '\n'}
	if err := os.WriteFile(path, content, 0755); err != nil {
		t.Fatal(err)
	}

	job := &models.Job{
		PredefinedJobID: "docker_cleanup",
		Command:         path,
	}
	r := runners.NewShellRunner()
	var buf bytes.Buffer
	code, err := r.Execute(context.Background(), job, &buf)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%q", code, err, buf.String())
	}
}

func TestDockerRunnerEcho(t *testing.T) {
	r := runners.NewDockerRunner()
	job := &models.Job{Command: "echo docker-test"}
	var buf bytes.Buffer
	code, err := r.Execute(context.Background(), job, &buf)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v out=%q", code, err, buf.String())
	}
}
