package runners

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"

	"github.com/hermes-scheduler/hermes/internal/models"
)

// ShellRunner executes commands via sh -c.
type ShellRunner struct{}

func NewShellRunner() *ShellRunner { return &ShellRunner{} }

func (r *ShellRunner) Type() models.RunnerType { return models.RunnerTypeShell }

func (r *ShellRunner) Execute(ctx context.Context, job *models.Job, output io.Writer) (int, error) {
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
	var envMap map[string]string
	if err := json.Unmarshal([]byte(envJSON), &envMap); err == nil {
		for k, v := range envMap {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
}
