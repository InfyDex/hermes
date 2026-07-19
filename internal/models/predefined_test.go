package models_test

import (
	"testing"

	"github.com/hermes-scheduler/hermes/internal/models"
)

func TestApplyPredefinedOverridesMatch(t *testing.T) {
	job := &models.Job{PredefinedJobID: "docker_cleanup", Command: "ignored"}
	if !job.ApplyPredefinedOverrides("") {
		t.Fatal("expected match")
	}
	if job.RunnerType != models.RunnerTypeShell {
		t.Fatalf("runner = %q", job.RunnerType)
	}
	if job.Command != "/app/scripts/docker-cleanup.sh" {
		t.Fatalf("command = %q", job.Command)
	}
}

func TestApplyPredefinedOverridesCustomPath(t *testing.T) {
	job := &models.Job{PredefinedJobID: "docker_cleanup"}
	job.ApplyPredefinedOverrides("/data/scripts/job_1_script.sh")
	if job.Command != "/data/scripts/job_1_script.sh" {
		t.Fatalf("command = %q", job.Command)
	}
}

func TestApplyPredefinedOverridesNoMatch(t *testing.T) {
	job := &models.Job{PredefinedJobID: "unknown", Command: "echo hi"}
	if job.ApplyPredefinedOverrides("") {
		t.Fatal("expected no match")
	}
	if job.Command != "echo hi" {
		t.Fatalf("command = %q", job.Command)
	}
}

func TestApplyPredefinedOverridesEmptyID(t *testing.T) {
	job := &models.Job{Command: "echo hi"}
	if job.ApplyPredefinedOverrides("") {
		t.Fatal("expected false for empty predefined id")
	}
}
