package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/k8s"
)

// K8sCronJobModule implements Kubernetes cronjob management
type K8sCronJobModule struct {
	*K8sBaseModule
}

// NewK8sCronJobModule creates a new Kubernetes cronjob module
func NewK8sCronJobModule(client k8s.ClientInterface) *K8sCronJobModule {
	return &K8sCronJobModule{
		K8sBaseModule: NewK8sBaseModule("k8s_cronjob", []string{"present", "absent", "suspended"}, client),
	}
}

// Check checks the current state of a Kubernetes cronjob
func (m *K8sCronJobModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("cronjob name is required")
	}

	// Get cronjob from cluster
	cj, err := m.client.GetCronJob(namespace, name)
	if err != nil {
		// Check if cronjob doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check cronjob: %w", err)
	}

	result.Present = true
	result.Metadata["namespace"] = cj.Namespace
	result.Metadata["schedule"] = cj.Schedule
	result.Metadata["suspend"] = cj.Suspend
	result.Metadata["concurrency_policy"] = cj.ConcurrencyPolicy
	result.Metadata["active_jobs"] = cj.ActiveJobs
	if cj.LastScheduleTime != nil {
		result.Metadata["last_schedule_time"] = cj.LastScheduleTime.Format(time.RFC3339)
	}
	if cj.LastSuccessfulTime != nil {
		result.Metadata["last_successful_time"] = cj.LastSuccessfulTime.Format(time.RFC3339)
	}
	result.Metadata["status"] = string(cj.Status)

	// Determine current state
	if cj.Suspend {
		result.CurrentState = "suspended"
	} else {
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
	case "suspended":
		result.Matches = cj.Suspend
		if !result.Matches {
			result.Diff["suspend"] = map[string]interface{}{
				"current": false,
				"desired": true,
			}
		}
	case "present":
		result.Matches = true

		// Check schedule
		desiredSchedule := getStringParameter(decl, "schedule", "")
		if desiredSchedule != "" && cj.Schedule != desiredSchedule {
			result.Matches = false
			result.Diff["schedule"] = map[string]string{
				"current": cj.Schedule,
				"desired": desiredSchedule,
			}
		}

		// Check concurrency policy
		desiredConcurrencyPolicy := getStringParameter(decl, "concurrency_policy", "")
		if desiredConcurrencyPolicy != "" && cj.ConcurrencyPolicy != desiredConcurrencyPolicy {
			result.Matches = false
			result.Diff["concurrency_policy"] = map[string]string{
				"current": cj.ConcurrencyPolicy,
				"desired": desiredConcurrencyPolicy,
			}
		}

		// Check if suspended when it shouldn't be
		if cj.Suspend {
			result.Matches = false
			result.Diff["suspend"] = map[string]interface{}{
				"current": true,
				"desired": false,
			}
		}

		// Check labels
		desiredLabels := getLabels(decl)
		if desiredLabels != nil && len(desiredLabels) > 0 {
			if !compareLabels(cj.Labels, desiredLabels) {
				result.Matches = false
				result.Diff["labels"] = map[string]interface{}{
					"current": cj.Labels,
					"desired": desiredLabels,
				}
			}
		}

		// Check annotations
		desiredAnnotations := getAnnotations(decl)
		if desiredAnnotations != nil && len(desiredAnnotations) > 0 {
			if !compareAnnotations(cj.Annotations, desiredAnnotations) {
				result.Matches = false
				result.Diff["annotations"] = map[string]interface{}{
					"current": cj.Annotations,
					"desired": desiredAnnotations,
				}
			}
		}
	default:
		result.Matches = false
	}

	return result, nil
}

// Apply applies the cronjob state
func (m *K8sCronJobModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "CronJob name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("cronjob name is required")
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
		result.Comment = fmt.Sprintf("CronJob %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.CronJobSpec{
			Name:              name,
			Labels:            getLabels(decl),
			Annotations:       getAnnotations(decl),
			Schedule:          getStringParameter(decl, "schedule", ""),
			Image:             getStringParameter(decl, "image", ""),
			Command:           getCommandParameter(decl),
			Args:              getArgsParameter(decl),
			ConcurrencyPolicy: getStringParameter(decl, "concurrency_policy", "Allow"),
			Suspend:           false, // present means not suspended
			RestartPolicy:     getStringParameter(decl, "restart_policy", "Never"),
		}

		if !checkResult.Present {
			// Validate required fields for creation
			if spec.Schedule == "" {
				result.Success = false
				result.Comment = "Schedule is required to create a cronjob"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("schedule is required to create a cronjob")
			}
			if spec.Image == "" {
				result.Success = false
				result.Comment = "Image is required to create a cronjob"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("image is required to create a cronjob")
			}

			// Create cronjob
			if err := m.client.CreateCronJob(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create cronjob: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["schedule"] = spec.Schedule
			result.Changes["image"] = spec.Image
			result.Changes["concurrency_policy"] = spec.ConcurrencyPolicy
			result.Comment = fmt.Sprintf("Created cronjob %s/%s", namespace, name)
		} else {
			// Update cronjob
			if err := m.client.UpdateCronJob(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update cronjob: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if scheduleDiff, ok := checkResult.Diff["schedule"]; ok {
				result.Changes["schedule_updated"] = scheduleDiff
			}
			if concurrencyDiff, ok := checkResult.Diff["concurrency_policy"]; ok {
				result.Changes["concurrency_policy_updated"] = concurrencyDiff
			}
			if suspendDiff, ok := checkResult.Diff["suspend"]; ok {
				result.Changes["suspend_updated"] = suspendDiff
			}
			result.Comment = fmt.Sprintf("Updated cronjob %s/%s", namespace, name)
		}
		result.Success = true

	case "suspended":
		if !checkResult.Present {
			// Need to create the cronjob first
			spec := k8s.CronJobSpec{
				Name:              name,
				Labels:            getLabels(decl),
				Annotations:       getAnnotations(decl),
				Schedule:          getStringParameter(decl, "schedule", ""),
				Image:             getStringParameter(decl, "image", ""),
				Command:           getCommandParameter(decl),
				Args:              getArgsParameter(decl),
				ConcurrencyPolicy: getStringParameter(decl, "concurrency_policy", "Allow"),
				Suspend:           true,
				RestartPolicy:     getStringParameter(decl, "restart_policy", "Never"),
			}

			if spec.Schedule == "" {
				result.Success = false
				result.Comment = "Schedule is required to create a cronjob"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("schedule is required to create a cronjob")
			}
			if spec.Image == "" {
				result.Success = false
				result.Comment = "Image is required to create a cronjob"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("image is required to create a cronjob")
			}

			if err := m.client.CreateCronJob(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create cronjob: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["suspended"] = true
			result.Comment = fmt.Sprintf("Created suspended cronjob %s/%s", namespace, name)
		} else {
			// Update to suspend
			spec := k8s.CronJobSpec{
				Name:    name,
				Suspend: true,
			}
			if err := m.client.UpdateCronJob(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to suspend cronjob: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["suspended"] = true
			result.Comment = fmt.Sprintf("Suspended cronjob %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete cronjob
			if err := m.client.DeleteCronJob(namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete cronjob: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted cronjob %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("CronJob %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for cronjob", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the cronjob is in the desired state
func (m *K8sCronJobModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}
