package runners

import (
	"context"
	"io"

	"github.com/hermes-scheduler/hermes/internal/models"
)

// Runner defines the interface for job execution backends.
type Runner interface {
	Type() models.RunnerType
	Execute(ctx context.Context, job *models.Job, output io.Writer) (exitCode int, err error)
}

// Registry holds available runners keyed by type.
type Registry struct {
	runners map[models.RunnerType]Runner
}

func NewRegistry() *Registry {
	return &Registry{runners: make(map[models.RunnerType]Runner)}
}

func (r *Registry) Register(runner Runner) {
	r.runners[runner.Type()] = runner
}

func (r *Registry) Get(t models.RunnerType) (Runner, bool) {
	runner, ok := r.runners[t]
	return runner, ok
}
