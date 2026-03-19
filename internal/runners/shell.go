package runners

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/hermes-scheduler/hermes/internal/models"
)

// ShellRunner executes commands via sh -c.
type ShellRunner struct{}

func NewShellRunner() *ShellRunner { return &ShellRunner{} }

func (r *ShellRunner) Type() models.RunnerType { return models.RunnerTypeShell }

func (r *ShellRunner) Execute(ctx context.Context, job *models.Job, output io.Writer) (int, error) {
	normalizePredefinedScript(job)

	cmd := exec.CommandContext(ctx, "sh", "-c", job.Command)
	if job.WorkingDir != "" {
		cmd.Dir = job.WorkingDir
	}
	applyEnv(cmd, job.EnvVars)
	cmd.Stdout = output
	cmd.Stderr = output

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

func normalizePredefinedScript(job *models.Job) {
	if job == nil || job.PredefinedJobID == "" {
		return
	}

	path := strings.TrimSpace(job.Command)
	if !strings.HasPrefix(path, "/data/scripts/job_") || !strings.HasSuffix(path, "_script.sh") {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	normalized := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	normalized = bytes.ReplaceAll(normalized, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	if bytes.Equal(data, normalized) {
		return
	}

	if err := os.WriteFile(path, normalized, 0755); err == nil {
		_ = os.Chmod(path, 0755)
	}
}

// DockerRunner executes docker CLI commands directly.
type DockerRunner struct{}

func NewDockerRunner() *DockerRunner { return &DockerRunner{} }

func (r *DockerRunner) Type() models.RunnerType { return models.RunnerTypeDocker }

func (r *DockerRunner) Execute(ctx context.Context, job *models.Job, output io.Writer) (int, error) {
	parts := strings.Fields(job.Command)
	if len(parts) == 0 {
		return -1, nil
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	if job.WorkingDir != "" {
		cmd.Dir = job.WorkingDir
	}
	applyEnv(cmd, job.EnvVars)
	cmd.Stdout = output
	cmd.Stderr = output

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

func applyEnv(cmd *exec.Cmd, envJSON string) {
	if envJSON == "" || envJSON == "{}" {
		return
	}

	// Start with the parent (system) environment to preserve PATH, HOME, etc.
	cmd.Env = os.Environ()

	var envMap map[string]string
	if err := json.Unmarshal([]byte(envJSON), &envMap); err == nil {
		for k, v := range envMap {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
}
