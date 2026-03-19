package models

// PredefinedJob contains the locked configuration for built-in jobs
type PredefinedJob struct {
	ID          string
	Name        string
	Description string
	RunnerType  RunnerType
	Command     string
	WorkingDir  string
	ScriptPath  string
}

// PredefinedJobsRegistry stores all predefined job templates
var PredefinedJobsRegistry = map[string]PredefinedJob{
	"docker_cleanup": {
		ID:          "docker_cleanup",
		Name:        "Docker System Cleanup",
		Description: "Automatically cleans up unused docker containers, networks, images, and volumes.",
		RunnerType:  RunnerTypeShell,
		Command:     "/app/scripts/docker-cleanup.sh",
		WorkingDir:  "/app",
		ScriptPath:  "/app/scripts/docker-cleanup.sh",
	},
}

// ApplyPredefinedOverrides applies the locked fields of a predefined job onto a Job struct.
// It returns true if the job matched a predefined template.
func (j *Job) ApplyPredefinedOverrides(customScriptPath string) bool {
	if j.PredefinedJobID == "" {
		return false
	}

	if pj, ok := PredefinedJobsRegistry[j.PredefinedJobID]; ok {
		j.RunnerType = pj.RunnerType
		j.WorkingDir = pj.WorkingDir
		
		if customScriptPath != "" {
			j.Command = customScriptPath
		} else {
			j.Command = pj.Command
		}
		return true
	}
	return false
}
