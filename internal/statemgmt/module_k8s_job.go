package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sJobModule implements Kubernetes job management
type K8sJobModule struct {
	*K8sBaseModule
}

// NewK8sJobModule creates a new Kubernetes job module
func NewK8sJobModule(client k8s.ClientInterface) *K8sJobModule {
	return &K8sJobModule{
		K8sBaseModule: NewK8sBaseModule("k8s_job", []string{"present", "absent", "completed"}, client),
	}
}

// Check checks the current state of a Kubernetes job
func (m *K8sJobModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("job name is required")
	}

	// Get job from cluster
	job, err := m.client.GetJob(ctx, namespace, name)
	if err != nil {
		// Check if job doesn't exist (not found error)
		if strings.Contains(err.Error(), "not found") {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (decl.State == "absent")
			if !result.Matches {
				result.Diff["state"] = map[string]string{
					"current": "absent",
					"desired": decl.State,
				}
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to check job: %w", err)
	}

	result.Present = true
	result.Metadata["namespace"] = job.Namespace
	result.Metadata["active"] = job.Active
	result.Metadata["succeeded"] = job.Succeeded
	result.Metadata["failed"] = job.Failed
	result.Metadata["completions"] = job.Completions
	result.Metadata["parallelism"] = job.Parallelism
	result.Metadata["backoff_limit"] = job.BackoffLimit
	if job.StartTime != nil {
		result.Metadata["start_time"] = job.StartTime.Format(time.RFC3339)
	}
	if job.CompletionTime != nil {
		result.Metadata["completion_time"] = job.CompletionTime.Format(time.RFC3339)
	}
	result.Metadata["status"] = string(job.Status)

	// Determine current state based on job status
	switch {
	case job.CompletionTime != nil && job.Succeeded >= job.Completions:
		result.CurrentState = "completed"
	case job.Failed > 0 && job.Active == 0:
		result.CurrentState = "failed"
	default:
		result.CurrentState = "present"
	}

	// Check if state matches desired
	switch decl.State {
	case "absent":
		result.Matches = false
		result.Diff["state"] = map[string]string{
			"current": result.CurrentState,
			"desired": "absent",
		}
	case "completed":
		result.Matches = (result.CurrentState == "completed")
		if !result.Matches {
			result.Diff["state"] = map[string]string{
				"current": result.CurrentState,
				"desired": "completed",
			}
		}
	case "present":
		result.Matches = true
		// Job exists, that's what we wanted
	default:
		result.Matches = false
	}

	return result, nil
}

// Apply applies the job state
func (m *K8sJobModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		StartTime: startTime,
		Changes:   make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		result.Success = false
		result.Comment = "Job name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("job name is required")
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.Duration = time.Since(startTime)
		return result, err
	}

	// If already in desired state, nothing to do
	if checkResult.Matches {
		result.Success = true
		result.Comment = fmt.Sprintf("Job %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		if !checkResult.Present {
			spec := k8s.JobSpec{
				Name:          name,
				Labels:        getLabels(decl),
				Annotations:   getAnnotations(decl),
				Image:         getStringParameter(decl, "image", ""),
				Command:       getCommandParameter(decl),
				Args:          getArgsParameter(decl),
				Completions:   getInt32Parameter(decl, "completions", 1),
				Parallelism:   getInt32Parameter(decl, "parallelism", 1),
				BackoffLimit:  getInt32Parameter(decl, "backoff_limit", 6),
				RestartPolicy: getStringParameter(decl, "restart_policy", "Never"),
			}

			// Validate required fields for creation
			if spec.Image == "" {
				result.Success = false
				result.Comment = "Image is required to create a job"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("image is required to create a job")
			}

			// Create job
			if err := m.client.CreateJob(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create job: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["image"] = spec.Image
			result.Changes["completions"] = spec.Completions
			result.Changes["parallelism"] = spec.Parallelism
			result.Comment = fmt.Sprintf("Created job %s/%s", namespace, name)
			result.Success = true
		} else {
			// Job already exists - jobs are immutable, can't update
			result.Success = true
			result.Comment = fmt.Sprintf("Job %s/%s already exists (jobs are immutable)", namespace, name)
		}

	case "completed":
		if !checkResult.Present {
			// Need to create the job first
			spec := k8s.JobSpec{
				Name:          name,
				Labels:        getLabels(decl),
				Annotations:   getAnnotations(decl),
				Image:         getStringParameter(decl, "image", ""),
				Command:       getCommandParameter(decl),
				Args:          getArgsParameter(decl),
				Completions:   getInt32Parameter(decl, "completions", 1),
				Parallelism:   getInt32Parameter(decl, "parallelism", 1),
				BackoffLimit:  getInt32Parameter(decl, "backoff_limit", 6),
				RestartPolicy: getStringParameter(decl, "restart_policy", "Never"),
			}

			if spec.Image == "" {
				result.Success = false
				result.Comment = "Image is required to create a job"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("image is required to create a job")
			}

			if err := m.client.CreateJob(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create job: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Comment = fmt.Sprintf("Created job %s/%s, waiting for completion", namespace, name)
		}
		// Job exists but not completed yet - this is a transient state
		// The job will eventually complete or fail
		result.Success = true
		if checkResult.CurrentState == "failed" {
			result.Comment = fmt.Sprintf("Job %s/%s has failed", namespace, name)
		} else {
			result.Comment = fmt.Sprintf("Job %s/%s is running, not yet completed", namespace, name)
		}

	case "absent":
		if checkResult.Present {
			// Delete job
			if err := m.client.DeleteJob(ctx, namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete job: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted job %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("Job %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for job", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the job is in the desired state
func (m *K8sJobModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getCommandParameter extracts command array from state declaration
func getCommandParameter(decl *StateDeclaration) []string {
	cmdRaw, ok := decl.Parameters["command"]
	if !ok {
		return nil
	}

	cmdSlice, ok := cmdRaw.([]interface{})
	if !ok {
		// Try single string
		if cmdStr, ok := cmdRaw.(string); ok {
			return []string{cmdStr}
		}
		return nil
	}

	commands := make([]string, 0, len(cmdSlice))
	for _, c := range cmdSlice {
		if s, ok := c.(string); ok {
			commands = append(commands, s)
		}
	}
	return commands
}

// getArgsParameter extracts args array from state declaration
func getArgsParameter(decl *StateDeclaration) []string {
	argsRaw, ok := decl.Parameters["args"]
	if !ok {
		return nil
	}

	argsSlice, ok := argsRaw.([]interface{})
	if !ok {
		// Try single string
		if argStr, ok := argsRaw.(string); ok {
			return []string{argStr}
		}
		return nil
	}

	args := make([]string, 0, len(argsSlice))
	for _, a := range argsSlice {
		if s, ok := a.(string); ok {
			args = append(args, s)
		}
	}
	return args
}
