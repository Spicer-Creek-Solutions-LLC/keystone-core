package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sServiceModule implements Kubernetes service management
type K8sServiceModule struct {
	*K8sBaseModule
}

// NewK8sServiceModule creates a new Kubernetes service module
func NewK8sServiceModule(client k8s.ClientInterface) *K8sServiceModule {
	return &K8sServiceModule{
		K8sBaseModule: NewK8sBaseModule("k8s_service", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes service
func (m *K8sServiceModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("service name is required")
	}

	// Get service from cluster
	service, err := m.client.GetService(ctx, namespace, name)
	if err != nil {
		// Check if service doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check service: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = service.Namespace
	result.Metadata["type"] = service.Type
	result.Metadata["clusterIP"] = service.ClusterIP
	result.Metadata["ports"] = service.Ports

	// Check if state matches desired
	if decl.State == "absent" {
		result.Matches = false
		result.Diff["state"] = map[string]string{
			"current": "present",
			"desired": "absent",
		}
		return result, nil
	}

	// For "present" state, check additional properties
	result.Matches = true

	// Check service type
	desiredType := getStringParameter(decl, "type", "ClusterIP")
	if service.Type != desiredType {
		result.Matches = false
		result.Diff["type"] = map[string]interface{}{
			"current": service.Type,
			"desired": desiredType,
		}
	}

	// Check ports
	desiredPorts := getServicePorts(decl)
	if len(desiredPorts) > 0 {
		if !compareServicePorts(service.Ports, desiredPorts) {
			result.Matches = false
			result.Diff["ports"] = map[string]interface{}{
				"current": service.Ports,
				"desired": desiredPorts,
			}
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if len(desiredLabels) > 0 {
		if !compareLabels(service.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": service.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if len(desiredAnnotations) > 0 {
		if !compareAnnotations(service.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": service.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the service state
func (m *K8sServiceModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "Service name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("service name is required")
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
		result.Comment = fmt.Sprintf("Service %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.ServiceSpec{
			Name:        name,
			Type:        getStringParameter(decl, "type", "ClusterIP"),
			Selector:    getSelectorLabels(decl),
			Ports:       convertToServicePortSpecs(getServicePorts(decl)),
			Labels:      getLabels(decl),
			Annotations: getAnnotations(decl),
			ClusterIP:   getStringParameter(decl, "cluster_ip", ""),
		}

		if !checkResult.Present {
			// Validate required fields for creation
			if len(spec.Ports) == 0 {
				result.Success = false
				result.Comment = "At least one port is required to create a service"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("at least one port is required to create a service")
			}

			// Create service
			if err := m.client.CreateService(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create service: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["type"] = spec.Type
			result.Changes["ports"] = spec.Ports
			result.Comment = fmt.Sprintf("Created service %s/%s", namespace, name)
		} else {
			// Update service
			if err := m.client.UpdateService(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update service: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if typeDiff, ok := checkResult.Diff["type"]; ok {
				result.Changes["type_updated"] = typeDiff
			}
			if portsDiff, ok := checkResult.Diff["ports"]; ok {
				result.Changes["ports_updated"] = portsDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated service %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete service
			if err := m.client.DeleteService(ctx, namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete service: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted service %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("Service %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for service", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the service is in the desired state
func (m *K8sServiceModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getServicePorts extracts service ports from state declaration
func getServicePorts(decl *StateDeclaration) []k8s.ServicePort {
	portsRaw, ok := decl.Parameters["ports"]
	if !ok {
		return nil
	}

	portsSlice, ok := portsRaw.([]interface{})
	if !ok {
		return nil
	}

	ports := make([]k8s.ServicePort, 0, len(portsSlice))
	for _, p := range portsSlice {
		portMap, ok := p.(map[string]interface{})
		if !ok {
			continue
		}

		port := k8s.ServicePort{
			Name:     getMapStringValue(portMap, "name", ""),
			Protocol: getMapStringValue(portMap, "protocol", "TCP"),
		}

		if portVal, ok := portMap["port"]; ok {
			port.Port = toInt32(portVal)
		}
		if targetPortVal, ok := portMap["target_port"]; ok {
			port.TargetPort = toInt32(targetPortVal)
		} else {
			port.TargetPort = port.Port
		}
		if nodePortVal, ok := portMap["node_port"]; ok {
			port.NodePort = toInt32(nodePortVal)
		}

		ports = append(ports, port)
	}

	return ports
}

// convertToServicePortSpecs converts ServicePort to ServicePortSpec
func convertToServicePortSpecs(ports []k8s.ServicePort) []k8s.ServicePortSpec {
	if ports == nil {
		return nil
	}
	specs := make([]k8s.ServicePortSpec, len(ports))
	for i, p := range ports {
		specs[i] = k8s.ServicePortSpec(p)
	}
	return specs
}

// compareServicePorts compares two service port slices
func compareServicePorts(current, desired []k8s.ServicePort) bool {
	if len(current) != len(desired) {
		return false
	}

	// Build a map of current ports by port number for comparison
	currentByPort := make(map[int32]k8s.ServicePort)
	for _, p := range current {
		currentByPort[p.Port] = p
	}

	for _, d := range desired {
		c, exists := currentByPort[d.Port]
		if !exists {
			return false
		}
		// Compare relevant fields (protocol and target port)
		if c.Protocol != d.Protocol && d.Protocol != "" {
			return false
		}
		if c.TargetPort != d.TargetPort && d.TargetPort != 0 {
			return false
		}
	}

	return true
}

// getMapStringValue gets a string value from a map
func getMapStringValue(m map[string]interface{}, key, defaultValue string) string {
	if v, ok := m[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return defaultValue
}

// toInt32 converts an interface to int32
func toInt32(v interface{}) int32 {
	switch val := v.(type) {
	case int:
		return int32(val) //nolint:gosec // G115: k8s service params are small ints
	case int32:
		return val
	case int64:
		return int32(val) //nolint:gosec // G115: k8s service params are small ints
	case float64:
		return int32(val) //nolint:gosec // G115: k8s service params are small ints
	default:
		return 0
	}
}
