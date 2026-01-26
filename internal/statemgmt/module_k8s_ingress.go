package statemgmt

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sIngressModule implements Kubernetes ingress management
type K8sIngressModule struct {
	*K8sBaseModule
}

// NewK8sIngressModule creates a new Kubernetes ingress module
func NewK8sIngressModule(client k8s.ClientInterface) *K8sIngressModule {
	return &K8sIngressModule{
		K8sBaseModule: NewK8sBaseModule("k8s_ingress", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes ingress
func (m *K8sIngressModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("ingress name is required")
	}

	// Get ingress from cluster
	ingress, err := m.client.GetIngress(namespace, name)
	if err != nil {
		// Check if ingress doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check ingress: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = ingress.Namespace
	result.Metadata["ingress_class"] = ingress.IngressClassName
	result.Metadata["rules_count"] = len(ingress.Rules)
	result.Metadata["tls_count"] = len(ingress.TLS)
	result.Metadata["load_balancer_ingress"] = ingress.LoadBalancerIngress

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

	// Check ingress class
	desiredIngressClass := getIngressClassName(decl)
	if desiredIngressClass != "" && ingress.IngressClassName != desiredIngressClass {
		result.Matches = false
		result.Diff["ingress_class"] = map[string]string{
			"current": ingress.IngressClassName,
			"desired": desiredIngressClass,
		}
	}

	// Check rules
	desiredRules := getIngressRules(decl)
	if desiredRules != nil && len(desiredRules) > 0 {
		if !compareIngressRules(ingress.Rules, desiredRules) {
			result.Matches = false
			result.Diff["rules"] = map[string]interface{}{
				"current_count": len(ingress.Rules),
				"desired_count": len(desiredRules),
			}
		}
	}

	// Check TLS
	desiredTLS := getIngressTLS(decl)
	if desiredTLS != nil && len(desiredTLS) > 0 {
		if !compareIngressTLS(ingress.TLS, desiredTLS) {
			result.Matches = false
			result.Diff["tls"] = map[string]interface{}{
				"current_count": len(ingress.TLS),
				"desired_count": len(desiredTLS),
			}
		}
	}

	// Check default backend
	desiredDefaultBackend := getIngressDefaultBackend(decl)
	if desiredDefaultBackend != nil {
		if !compareIngressBackend(ingress.DefaultBackend, desiredDefaultBackend) {
			result.Matches = false
			currentBackend := "none"
			if ingress.DefaultBackend != nil {
				currentBackend = fmt.Sprintf("%s:%d", ingress.DefaultBackend.ServiceName, ingress.DefaultBackend.ServicePort)
			}
			result.Diff["default_backend"] = map[string]interface{}{
				"current": currentBackend,
				"desired": fmt.Sprintf("%s:%d", desiredDefaultBackend.ServiceName, desiredDefaultBackend.ServicePort),
			}
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if desiredLabels != nil && len(desiredLabels) > 0 {
		if !compareLabels(ingress.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": ingress.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if desiredAnnotations != nil && len(desiredAnnotations) > 0 {
		if !compareAnnotations(ingress.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": ingress.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the ingress state
func (m *K8sIngressModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "Ingress name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("ingress name is required")
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
		result.Comment = fmt.Sprintf("Ingress %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.IngressSpec{
			Name:             name,
			IngressClassName: getIngressClassName(decl),
			Labels:           getLabels(decl),
			Annotations:      getAnnotations(decl),
			Rules:            getIngressRules(decl),
			TLS:              getIngressTLS(decl),
			DefaultBackend:   getIngressDefaultBackend(decl),
		}

		if !checkResult.Present {
			// Create ingress
			if err := m.client.CreateIngress(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create ingress: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["ingress_class"] = spec.IngressClassName
			result.Changes["rules_count"] = len(spec.Rules)
			result.Changes["tls_count"] = len(spec.TLS)
			result.Comment = fmt.Sprintf("Created ingress %s/%s", namespace, name)
		} else {
			// Update ingress
			if err := m.client.UpdateIngress(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update ingress: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if ingressClassDiff, ok := checkResult.Diff["ingress_class"]; ok {
				result.Changes["ingress_class_updated"] = ingressClassDiff
			}
			if rulesDiff, ok := checkResult.Diff["rules"]; ok {
				result.Changes["rules_updated"] = rulesDiff
			}
			if tlsDiff, ok := checkResult.Diff["tls"]; ok {
				result.Changes["tls_updated"] = tlsDiff
			}
			if backendDiff, ok := checkResult.Diff["default_backend"]; ok {
				result.Changes["default_backend_updated"] = backendDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated ingress %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete ingress
			if err := m.client.DeleteIngress(namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete ingress: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted ingress %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("Ingress %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for ingress", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the ingress is in the desired state
func (m *K8sIngressModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getIngressClassName extracts ingress class name from state declaration
func getIngressClassName(decl *StateDeclaration) string {
	classRaw, ok := decl.Parameters["ingress_class"]
	if !ok {
		return ""
	}
	if str, ok := classRaw.(string); ok {
		return str
	}
	return ""
}

// getIngressRules extracts ingress rules from state declaration
func getIngressRules(decl *StateDeclaration) []k8s.IngressRule {
	rulesRaw, ok := decl.Parameters["rules"]
	if !ok {
		return nil
	}

	rulesSlice, ok := rulesRaw.([]interface{})
	if !ok {
		return nil
	}

	rules := make([]k8s.IngressRule, 0, len(rulesSlice))
	for _, ruleRaw := range rulesSlice {
		ruleMap, ok := ruleRaw.(map[string]interface{})
		if !ok {
			continue
		}

		rule := k8s.IngressRule{}

		if host, ok := ruleMap["host"].(string); ok {
			rule.Host = host
		}

		if pathsRaw, ok := ruleMap["paths"].([]interface{}); ok {
			paths := make([]k8s.IngressPath, 0, len(pathsRaw))
			for _, pathRaw := range pathsRaw {
				pathMap, ok := pathRaw.(map[string]interface{})
				if !ok {
					continue
				}

				path := k8s.IngressPath{}
				if p, ok := pathMap["path"].(string); ok {
					path.Path = p
				}
				if pt, ok := pathMap["path_type"].(string); ok {
					path.PathType = pt
				}
				if backend, ok := pathMap["backend"].(map[string]interface{}); ok {
					if svc, ok := backend["service"].(string); ok {
						path.Backend.ServiceName = svc
					}
					if port, ok := backend["port"].(int); ok {
						path.Backend.ServicePort = int32(port)
					} else if port, ok := backend["port"].(float64); ok {
						path.Backend.ServicePort = int32(port)
					}
				}
				paths = append(paths, path)
			}
			rule.Paths = paths
		}

		rules = append(rules, rule)
	}

	if len(rules) == 0 {
		return nil
	}
	return rules
}

// getIngressTLS extracts TLS configuration from state declaration
func getIngressTLS(decl *StateDeclaration) []k8s.IngressTLS {
	tlsRaw, ok := decl.Parameters["tls"]
	if !ok {
		return nil
	}

	tlsSlice, ok := tlsRaw.([]interface{})
	if !ok {
		return nil
	}

	tlsList := make([]k8s.IngressTLS, 0, len(tlsSlice))
	for _, tlsRaw := range tlsSlice {
		tlsMap, ok := tlsRaw.(map[string]interface{})
		if !ok {
			continue
		}

		tls := k8s.IngressTLS{}

		if secretName, ok := tlsMap["secret_name"].(string); ok {
			tls.SecretName = secretName
		}

		if hostsRaw, ok := tlsMap["hosts"].([]interface{}); ok {
			hosts := make([]string, 0, len(hostsRaw))
			for _, hostRaw := range hostsRaw {
				if host, ok := hostRaw.(string); ok {
					hosts = append(hosts, host)
				}
			}
			tls.Hosts = hosts
		}

		tlsList = append(tlsList, tls)
	}

	if len(tlsList) == 0 {
		return nil
	}
	return tlsList
}

// getIngressDefaultBackend extracts default backend from state declaration
func getIngressDefaultBackend(decl *StateDeclaration) *k8s.IngressBackend {
	backendRaw, ok := decl.Parameters["default_backend"]
	if !ok {
		return nil
	}

	backendMap, ok := backendRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	backend := &k8s.IngressBackend{}

	if service, ok := backendMap["service"].(string); ok {
		backend.ServiceName = service
	}

	if port, ok := backendMap["port"].(int); ok {
		backend.ServicePort = int32(port)
	} else if port, ok := backendMap["port"].(float64); ok {
		backend.ServicePort = int32(port)
	}

	if backend.ServiceName == "" {
		return nil
	}

	return backend
}

// compareIngressRules compares two ingress rule slices
func compareIngressRules(current, desired []k8s.IngressRule) bool {
	if len(current) != len(desired) {
		return false
	}
	return reflect.DeepEqual(current, desired)
}

// compareIngressTLS compares two ingress TLS slices
func compareIngressTLS(current, desired []k8s.IngressTLS) bool {
	if len(current) != len(desired) {
		return false
	}
	return reflect.DeepEqual(current, desired)
}

// compareIngressBackend compares two ingress backends
func compareIngressBackend(current, desired *k8s.IngressBackend) bool {
	if current == nil && desired == nil {
		return true
	}
	if current == nil || desired == nil {
		return false
	}
	return current.ServiceName == desired.ServiceName && current.ServicePort == desired.ServicePort
}
