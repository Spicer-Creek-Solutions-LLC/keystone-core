package statemgmt

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// MockK8sClient is a mock Kubernetes client for testing
type MockK8sClient struct {
	namespaces   map[string]*k8s.NamespaceInfo
	deployments  map[string]*k8s.DeploymentInfo  // key is "namespace/name"
	services     map[string]*k8s.ServiceInfo     // key is "namespace/name"
	configmaps   map[string]*k8s.ConfigMapInfo   // key is "namespace/name"
	secrets      map[string]*k8s.SecretInfo      // key is "namespace/name"
	ingresses    map[string]*k8s.IngressInfo     // key is "namespace/name"
	statefulsets map[string]*k8s.StatefulSetInfo // key is "namespace/name"
	daemonsets   map[string]*k8s.DaemonSetInfo   // key is "namespace/name"
	jobs         map[string]*k8s.JobInfo         // key is "namespace/name"
	cronjobs     map[string]*k8s.CronJobInfo     // key is "namespace/name"
	pvcs         map[string]*k8s.PVCInfo         // key is "namespace/name"
	hpas         map[string]*k8s.HPAInfo         // key is "namespace/name"
}

func (m *MockK8sClient) ExecInPod(opts k8s.PodExecOptions) (*k8s.PodExecResult, error) {
	return &k8s.PodExecResult{ExitCode: 0}, nil
}

func (m *MockK8sClient) ExecInPods(selector k8s.PodSelector, command []string) ([]k8s.PodExecResult, error) {
	return []k8s.PodExecResult{{ExitCode: 0}}, nil
}

func (m *MockK8sClient) GetPod(namespace, name string) (*k8s.ResourceInfo, error) {
	return &k8s.ResourceInfo{
		Kind:      "Pod",
		Namespace: namespace,
		Name:      name,
		Status:    k8s.StatusRunning,
	}, nil
}

func (m *MockK8sClient) ListPods(selector k8s.PodSelector) ([]k8s.ResourceInfo, error) {
	return []k8s.ResourceInfo{
		{
			Kind:      "Pod",
			Namespace: "default",
			Name:      "test-pod",
			Status:    k8s.StatusRunning,
		},
	}, nil
}

func (m *MockK8sClient) GetDeployment(namespace, name string) (*k8s.DeploymentInfo, error) {
	if m.deployments == nil {
		m.deployments = make(map[string]*k8s.DeploymentInfo)
	}
	key := namespace + "/" + name
	dep, ok := m.deployments[key]
	if !ok {
		return nil, fmt.Errorf("deployment %q not found", key)
	}
	return dep, nil
}

func (m *MockK8sClient) CreateDeployment(namespace string, spec k8s.DeploymentSpec) error {
	if m.deployments == nil {
		m.deployments = make(map[string]*k8s.DeploymentInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.deployments[key]; exists {
		return fmt.Errorf("deployment %q already exists", key)
	}
	m.deployments[key] = &k8s.DeploymentInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "Deployment",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Replicas:          spec.Replicas,
		AvailableReplicas: spec.Replicas,
		ReadyReplicas:     spec.Replicas,
		UpdatedReplicas:   spec.Replicas,
	}
	return nil
}

func (m *MockK8sClient) UpdateDeployment(namespace string, spec k8s.DeploymentSpec) error {
	if m.deployments == nil {
		m.deployments = make(map[string]*k8s.DeploymentInfo)
	}
	key := namespace + "/" + spec.Name
	dep, exists := m.deployments[key]
	if !exists {
		return fmt.Errorf("deployment %q not found", key)
	}
	// Update replicas
	dep.Replicas = spec.Replicas
	dep.AvailableReplicas = spec.Replicas
	dep.ReadyReplicas = spec.Replicas
	dep.UpdatedReplicas = spec.Replicas
	// Merge labels
	if dep.Labels == nil {
		dep.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		dep.Labels[k] = v
	}
	// Merge annotations
	if dep.Annotations == nil {
		dep.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		dep.Annotations[k] = v
	}
	return nil
}

func (m *MockK8sClient) DeleteDeployment(namespace, name string) error {
	if m.deployments == nil {
		m.deployments = make(map[string]*k8s.DeploymentInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.deployments[key]; !exists {
		return fmt.Errorf("deployment %q not found", key)
	}
	delete(m.deployments, key)
	return nil
}

func (m *MockK8sClient) ScaleDeployment(namespace, name string, replicas int32) error {
	if m.deployments == nil {
		m.deployments = make(map[string]*k8s.DeploymentInfo)
	}
	key := namespace + "/" + name
	dep, exists := m.deployments[key]
	if !exists {
		return fmt.Errorf("deployment %q not found", key)
	}
	dep.Replicas = replicas
	dep.AvailableReplicas = replicas
	dep.ReadyReplicas = replicas
	dep.UpdatedReplicas = replicas
	return nil
}

func (m *MockK8sClient) GetService(namespace, name string) (*k8s.ServiceInfo, error) {
	if m.services == nil {
		m.services = make(map[string]*k8s.ServiceInfo)
	}
	key := namespace + "/" + name
	svc, ok := m.services[key]
	if !ok {
		return nil, fmt.Errorf("service %q not found", key)
	}
	return svc, nil
}

func (m *MockK8sClient) CreateService(namespace string, spec k8s.ServiceSpec) error {
	if m.services == nil {
		m.services = make(map[string]*k8s.ServiceInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.services[key]; exists {
		return fmt.Errorf("service %q already exists", key)
	}

	// Convert ServicePortSpec to ServicePort
	ports := make([]k8s.ServicePort, len(spec.Ports))
	for i, p := range spec.Ports {
		protocol := p.Protocol
		if protocol == "" {
			protocol = "TCP"
		}
		targetPort := p.TargetPort
		if targetPort == 0 {
			targetPort = p.Port
		}
		ports[i] = k8s.ServicePort{
			Name:       p.Name,
			Protocol:   protocol,
			Port:       p.Port,
			TargetPort: targetPort,
			NodePort:   p.NodePort,
		}
	}

	serviceType := spec.Type
	if serviceType == "" {
		serviceType = "ClusterIP"
	}

	m.services[key] = &k8s.ServiceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "Service",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Type:      serviceType,
		ClusterIP: "10.0.0.1",
		Ports:     ports,
	}
	return nil
}

func (m *MockK8sClient) UpdateService(namespace string, spec k8s.ServiceSpec) error {
	if m.services == nil {
		m.services = make(map[string]*k8s.ServiceInfo)
	}
	key := namespace + "/" + spec.Name
	svc, exists := m.services[key]
	if !exists {
		return fmt.Errorf("service %q not found", key)
	}

	// Update ports if specified
	if len(spec.Ports) > 0 {
		ports := make([]k8s.ServicePort, len(spec.Ports))
		for i, p := range spec.Ports {
			protocol := p.Protocol
			if protocol == "" {
				protocol = "TCP"
			}
			targetPort := p.TargetPort
			if targetPort == 0 {
				targetPort = p.Port
			}
			ports[i] = k8s.ServicePort{
				Name:       p.Name,
				Protocol:   protocol,
				Port:       p.Port,
				TargetPort: targetPort,
				NodePort:   p.NodePort,
			}
		}
		svc.Ports = ports
	}

	// Update type if specified
	if spec.Type != "" {
		svc.Type = spec.Type
	}

	// Merge labels
	if svc.Labels == nil {
		svc.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		svc.Labels[k] = v
	}

	// Merge annotations
	if svc.Annotations == nil {
		svc.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		svc.Annotations[k] = v
	}

	return nil
}

func (m *MockK8sClient) DeleteService(namespace, name string) error {
	if m.services == nil {
		m.services = make(map[string]*k8s.ServiceInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.services[key]; !exists {
		return fmt.Errorf("service %q not found", key)
	}
	delete(m.services, key)
	return nil
}

func (m *MockK8sClient) GetConfigMap(namespace, name string) (*k8s.ConfigMapInfo, error) {
	if m.configmaps == nil {
		m.configmaps = make(map[string]*k8s.ConfigMapInfo)
	}
	key := namespace + "/" + name
	cm, ok := m.configmaps[key]
	if !ok {
		return nil, fmt.Errorf("configmap %q not found", key)
	}
	return cm, nil
}

func (m *MockK8sClient) CreateConfigMap(namespace string, spec k8s.ConfigMapSpec) error {
	if m.configmaps == nil {
		m.configmaps = make(map[string]*k8s.ConfigMapInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.configmaps[key]; exists {
		return fmt.Errorf("configmap %q already exists", key)
	}
	m.configmaps[key] = &k8s.ConfigMapInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "ConfigMap",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Data:       spec.Data,
		BinaryData: spec.BinaryData,
	}
	return nil
}

func (m *MockK8sClient) UpdateConfigMap(namespace string, spec k8s.ConfigMapSpec) error {
	if m.configmaps == nil {
		m.configmaps = make(map[string]*k8s.ConfigMapInfo)
	}
	key := namespace + "/" + spec.Name
	cm, exists := m.configmaps[key]
	if !exists {
		return fmt.Errorf("configmap %q not found", key)
	}
	// Merge labels
	if cm.Labels == nil {
		cm.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		cm.Labels[k] = v
	}
	// Merge annotations
	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		cm.Annotations[k] = v
	}
	// Replace data
	if spec.Data != nil {
		cm.Data = spec.Data
	}
	if spec.BinaryData != nil {
		cm.BinaryData = spec.BinaryData
	}
	return nil
}

func (m *MockK8sClient) DeleteConfigMap(namespace, name string) error {
	if m.configmaps == nil {
		m.configmaps = make(map[string]*k8s.ConfigMapInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.configmaps[key]; !exists {
		return fmt.Errorf("configmap %q not found", key)
	}
	delete(m.configmaps, key)
	return nil
}

func (m *MockK8sClient) GetSecret(namespace, name string) (*k8s.SecretInfo, error) {
	if m.secrets == nil {
		m.secrets = make(map[string]*k8s.SecretInfo)
	}
	key := namespace + "/" + name
	secret, ok := m.secrets[key]
	if !ok {
		return nil, fmt.Errorf("secret %q not found", key)
	}
	return secret, nil
}

func (m *MockK8sClient) CreateSecret(namespace string, spec k8s.SecretSpec) error {
	if m.secrets == nil {
		m.secrets = make(map[string]*k8s.SecretInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.secrets[key]; exists {
		return fmt.Errorf("secret %q already exists", key)
	}

	secretType := spec.Type
	if secretType == "" {
		secretType = "Opaque"
	}

	// Merge Data and StringData
	data := make(map[string][]byte)
	for k, v := range spec.Data {
		data[k] = v
	}
	for k, v := range spec.StringData {
		data[k] = []byte(v)
	}

	m.secrets[key] = &k8s.SecretInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "Secret",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Type: secretType,
		Data: data,
	}
	return nil
}

func (m *MockK8sClient) UpdateSecret(namespace string, spec k8s.SecretSpec) error {
	if m.secrets == nil {
		m.secrets = make(map[string]*k8s.SecretInfo)
	}
	key := namespace + "/" + spec.Name
	secret, exists := m.secrets[key]
	if !exists {
		return fmt.Errorf("secret %q not found", key)
	}

	// Merge labels
	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		secret.Labels[k] = v
	}

	// Merge annotations
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		secret.Annotations[k] = v
	}

	// Update type if specified
	if spec.Type != "" {
		secret.Type = spec.Type
	}

	// Replace data if provided
	if spec.Data != nil {
		secret.Data = spec.Data
	}

	// Merge StringData into Data
	if spec.StringData != nil {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		for k, v := range spec.StringData {
			secret.Data[k] = []byte(v)
		}
	}

	return nil
}

func (m *MockK8sClient) DeleteSecret(namespace, name string) error {
	if m.secrets == nil {
		m.secrets = make(map[string]*k8s.SecretInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.secrets[key]; !exists {
		return fmt.Errorf("secret %q not found", key)
	}
	delete(m.secrets, key)
	return nil
}

func (m *MockK8sClient) GetIngress(namespace, name string) (*k8s.IngressInfo, error) {
	if m.ingresses == nil {
		m.ingresses = make(map[string]*k8s.IngressInfo)
	}
	key := namespace + "/" + name
	ingress, ok := m.ingresses[key]
	if !ok {
		return nil, fmt.Errorf("ingress %q not found", key)
	}
	return ingress, nil
}

func (m *MockK8sClient) CreateIngress(namespace string, spec k8s.IngressSpec) error {
	if m.ingresses == nil {
		m.ingresses = make(map[string]*k8s.IngressInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.ingresses[key]; exists {
		return fmt.Errorf("ingress %q already exists", key)
	}

	m.ingresses[key] = &k8s.IngressInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "Ingress",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		IngressClassName:    spec.IngressClassName,
		Rules:               spec.Rules,
		TLS:                 spec.TLS,
		DefaultBackend:      spec.DefaultBackend,
		LoadBalancerIngress: []string{},
	}
	return nil
}

func (m *MockK8sClient) UpdateIngress(namespace string, spec k8s.IngressSpec) error {
	if m.ingresses == nil {
		m.ingresses = make(map[string]*k8s.IngressInfo)
	}
	key := namespace + "/" + spec.Name
	ingress, exists := m.ingresses[key]
	if !exists {
		return fmt.Errorf("ingress %q not found", key)
	}

	// Merge labels
	if ingress.Labels == nil {
		ingress.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		ingress.Labels[k] = v
	}

	// Merge annotations
	if ingress.Annotations == nil {
		ingress.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		ingress.Annotations[k] = v
	}

	// Update ingress class if specified
	if spec.IngressClassName != "" {
		ingress.IngressClassName = spec.IngressClassName
	}

	// Update rules if specified
	if len(spec.Rules) > 0 {
		ingress.Rules = spec.Rules
	}

	// Update TLS if specified
	if len(spec.TLS) > 0 {
		ingress.TLS = spec.TLS
	}

	// Update default backend if specified
	if spec.DefaultBackend != nil {
		ingress.DefaultBackend = spec.DefaultBackend
	}

	return nil
}

func (m *MockK8sClient) DeleteIngress(namespace, name string) error {
	if m.ingresses == nil {
		m.ingresses = make(map[string]*k8s.IngressInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.ingresses[key]; !exists {
		return fmt.Errorf("ingress %q not found", key)
	}
	delete(m.ingresses, key)
	return nil
}

func (m *MockK8sClient) GetStatefulSet(namespace, name string) (*k8s.StatefulSetInfo, error) {
	if m.statefulsets == nil {
		m.statefulsets = make(map[string]*k8s.StatefulSetInfo)
	}
	key := namespace + "/" + name
	sts, ok := m.statefulsets[key]
	if !ok {
		return nil, fmt.Errorf("statefulset %q not found", key)
	}
	return sts, nil
}

func (m *MockK8sClient) CreateStatefulSet(namespace string, spec k8s.StatefulSetSpec) error {
	if m.statefulsets == nil {
		m.statefulsets = make(map[string]*k8s.StatefulSetInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.statefulsets[key]; exists {
		return fmt.Errorf("statefulset %q already exists", key)
	}

	replicas := spec.Replicas
	if replicas == 0 {
		replicas = 1
	}

	podManagementPolicy := spec.PodManagementPolicy
	if podManagementPolicy == "" {
		podManagementPolicy = "OrderedReady"
	}

	updateStrategy := spec.UpdateStrategy
	if updateStrategy == "" {
		updateStrategy = "RollingUpdate"
	}

	m.statefulsets[key] = &k8s.StatefulSetInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "StatefulSet",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Replicas:            replicas,
		ReadyReplicas:       replicas,
		CurrentReplicas:     replicas,
		UpdatedReplicas:     replicas,
		CurrentRevision:     "rev-1",
		UpdateRevision:      "rev-1",
		ServiceName:         spec.ServiceName,
		PodManagementPolicy: podManagementPolicy,
		UpdateStrategy:      updateStrategy,
	}
	return nil
}

func (m *MockK8sClient) UpdateStatefulSet(namespace string, spec k8s.StatefulSetSpec) error {
	if m.statefulsets == nil {
		m.statefulsets = make(map[string]*k8s.StatefulSetInfo)
	}
	key := namespace + "/" + spec.Name
	sts, exists := m.statefulsets[key]
	if !exists {
		return fmt.Errorf("statefulset %q not found", key)
	}

	// Update replicas if specified
	if spec.Replicas > 0 {
		sts.Replicas = spec.Replicas
		sts.ReadyReplicas = spec.Replicas
		sts.CurrentReplicas = spec.Replicas
		sts.UpdatedReplicas = spec.Replicas
	}

	// Update labels
	if spec.Labels != nil {
		if sts.Labels == nil {
			sts.Labels = make(map[string]string)
		}
		for k, v := range spec.Labels {
			sts.Labels[k] = v
		}
	}

	// Update annotations
	if spec.Annotations != nil {
		if sts.Annotations == nil {
			sts.Annotations = make(map[string]string)
		}
		for k, v := range spec.Annotations {
			sts.Annotations[k] = v
		}
	}

	// Update update strategy
	if spec.UpdateStrategy != "" {
		sts.UpdateStrategy = spec.UpdateStrategy
	}

	return nil
}

func (m *MockK8sClient) DeleteStatefulSet(namespace, name string) error {
	if m.statefulsets == nil {
		m.statefulsets = make(map[string]*k8s.StatefulSetInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.statefulsets[key]; !exists {
		return fmt.Errorf("statefulset %q not found", key)
	}
	delete(m.statefulsets, key)
	return nil
}

func (m *MockK8sClient) ScaleStatefulSet(namespace, name string, replicas int32) error {
	if m.statefulsets == nil {
		m.statefulsets = make(map[string]*k8s.StatefulSetInfo)
	}
	key := namespace + "/" + name
	sts, exists := m.statefulsets[key]
	if !exists {
		return fmt.Errorf("statefulset %q not found", key)
	}
	sts.Replicas = replicas
	sts.ReadyReplicas = replicas
	sts.CurrentReplicas = replicas
	sts.UpdatedReplicas = replicas
	return nil
}

// DaemonSet mock methods

func (m *MockK8sClient) GetDaemonSet(namespace, name string) (*k8s.DaemonSetInfo, error) {
	if m.daemonsets == nil {
		m.daemonsets = make(map[string]*k8s.DaemonSetInfo)
	}
	key := namespace + "/" + name
	ds, ok := m.daemonsets[key]
	if !ok {
		return nil, fmt.Errorf("daemonset %q not found", key)
	}
	return ds, nil
}

func (m *MockK8sClient) CreateDaemonSet(namespace string, spec k8s.DaemonSetSpec) error {
	if m.daemonsets == nil {
		m.daemonsets = make(map[string]*k8s.DaemonSetInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.daemonsets[key]; exists {
		return fmt.Errorf("daemonset %q already exists", key)
	}
	m.daemonsets[key] = &k8s.DaemonSetInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "DaemonSet",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		DesiredNumberScheduled: 3,
		CurrentNumberScheduled: 3,
		NumberReady:            3,
		NumberAvailable:        3,
		UpdateStrategy:         spec.UpdateStrategy,
	}
	return nil
}

func (m *MockK8sClient) UpdateDaemonSet(namespace string, spec k8s.DaemonSetSpec) error {
	if m.daemonsets == nil {
		m.daemonsets = make(map[string]*k8s.DaemonSetInfo)
	}
	key := namespace + "/" + spec.Name
	ds, exists := m.daemonsets[key]
	if !exists {
		return fmt.Errorf("daemonset %q not found", key)
	}
	if spec.UpdateStrategy != "" {
		ds.UpdateStrategy = spec.UpdateStrategy
	}
	if ds.Labels == nil {
		ds.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		ds.Labels[k] = v
	}
	if ds.Annotations == nil {
		ds.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		ds.Annotations[k] = v
	}
	return nil
}

func (m *MockK8sClient) DeleteDaemonSet(namespace, name string) error {
	if m.daemonsets == nil {
		m.daemonsets = make(map[string]*k8s.DaemonSetInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.daemonsets[key]; !exists {
		return fmt.Errorf("daemonset %q not found", key)
	}
	delete(m.daemonsets, key)
	return nil
}

// Job mock methods

func (m *MockK8sClient) GetJob(namespace, name string) (*k8s.JobInfo, error) {
	if m.jobs == nil {
		m.jobs = make(map[string]*k8s.JobInfo)
	}
	key := namespace + "/" + name
	job, ok := m.jobs[key]
	if !ok {
		return nil, fmt.Errorf("job %q not found", key)
	}
	return job, nil
}

func (m *MockK8sClient) CreateJob(namespace string, spec k8s.JobSpec) error {
	if m.jobs == nil {
		m.jobs = make(map[string]*k8s.JobInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.jobs[key]; exists {
		return fmt.Errorf("job %q already exists", key)
	}
	now := time.Now()
	m.jobs[key] = &k8s.JobInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "Job",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: now,
		},
		Active:       1,
		Succeeded:    0,
		Failed:       0,
		Completions:  spec.Completions,
		Parallelism:  spec.Parallelism,
		BackoffLimit: spec.BackoffLimit,
		StartTime:    &now,
	}
	return nil
}

func (m *MockK8sClient) DeleteJob(namespace, name string) error {
	if m.jobs == nil {
		m.jobs = make(map[string]*k8s.JobInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.jobs[key]; !exists {
		return fmt.Errorf("job %q not found", key)
	}
	delete(m.jobs, key)
	return nil
}

// CronJob mock methods

func (m *MockK8sClient) GetCronJob(namespace, name string) (*k8s.CronJobInfo, error) {
	if m.cronjobs == nil {
		m.cronjobs = make(map[string]*k8s.CronJobInfo)
	}
	key := namespace + "/" + name
	cj, ok := m.cronjobs[key]
	if !ok {
		return nil, fmt.Errorf("cronjob %q not found", key)
	}
	return cj, nil
}

func (m *MockK8sClient) CreateCronJob(namespace string, spec k8s.CronJobSpec) error {
	if m.cronjobs == nil {
		m.cronjobs = make(map[string]*k8s.CronJobInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.cronjobs[key]; exists {
		return fmt.Errorf("cronjob %q already exists", key)
	}
	m.cronjobs[key] = &k8s.CronJobInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "CronJob",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Schedule:          spec.Schedule,
		Suspend:           spec.Suspend,
		ConcurrencyPolicy: spec.ConcurrencyPolicy,
		ActiveJobs:        0,
	}
	return nil
}

func (m *MockK8sClient) UpdateCronJob(namespace string, spec k8s.CronJobSpec) error {
	if m.cronjobs == nil {
		m.cronjobs = make(map[string]*k8s.CronJobInfo)
	}
	key := namespace + "/" + spec.Name
	cj, exists := m.cronjobs[key]
	if !exists {
		return fmt.Errorf("cronjob %q not found", key)
	}
	if spec.Schedule != "" {
		cj.Schedule = spec.Schedule
	}
	cj.Suspend = spec.Suspend
	if spec.ConcurrencyPolicy != "" {
		cj.ConcurrencyPolicy = spec.ConcurrencyPolicy
	}
	if cj.Labels == nil {
		cj.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		cj.Labels[k] = v
	}
	if cj.Annotations == nil {
		cj.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		cj.Annotations[k] = v
	}
	return nil
}

func (m *MockK8sClient) DeleteCronJob(namespace, name string) error {
	if m.cronjobs == nil {
		m.cronjobs = make(map[string]*k8s.CronJobInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.cronjobs[key]; !exists {
		return fmt.Errorf("cronjob %q not found", key)
	}
	delete(m.cronjobs, key)
	return nil
}

// PVC mock methods

func (m *MockK8sClient) GetPVC(namespace, name string) (*k8s.PVCInfo, error) {
	if m.pvcs == nil {
		m.pvcs = make(map[string]*k8s.PVCInfo)
	}
	key := namespace + "/" + name
	pvc, ok := m.pvcs[key]
	if !ok {
		return nil, fmt.Errorf("pvc %q not found", key)
	}
	return pvc, nil
}

func (m *MockK8sClient) CreatePVC(namespace string, spec k8s.PVCSpec) error {
	if m.pvcs == nil {
		m.pvcs = make(map[string]*k8s.PVCInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.pvcs[key]; exists {
		return fmt.Errorf("pvc %q already exists", key)
	}
	m.pvcs[key] = &k8s.PVCInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "PersistentVolumeClaim",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Phase:            "Bound",
		StorageClassName: spec.StorageClassName,
		VolumeName:       "pv-" + spec.Name,
		AccessModes:      spec.AccessModes,
		RequestedStorage: spec.StorageSize,
		AllocatedStorage: spec.StorageSize,
	}
	return nil
}

func (m *MockK8sClient) UpdatePVC(namespace string, spec k8s.PVCSpec) error {
	if m.pvcs == nil {
		m.pvcs = make(map[string]*k8s.PVCInfo)
	}
	key := namespace + "/" + spec.Name
	pvc, exists := m.pvcs[key]
	if !exists {
		return fmt.Errorf("pvc %q not found", key)
	}
	if spec.StorageSize != "" {
		pvc.RequestedStorage = spec.StorageSize
		pvc.AllocatedStorage = spec.StorageSize
	}
	if pvc.Labels == nil {
		pvc.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		pvc.Labels[k] = v
	}
	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		pvc.Annotations[k] = v
	}
	return nil
}

func (m *MockK8sClient) DeletePVC(namespace, name string) error {
	if m.pvcs == nil {
		m.pvcs = make(map[string]*k8s.PVCInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.pvcs[key]; !exists {
		return fmt.Errorf("pvc %q not found", key)
	}
	delete(m.pvcs, key)
	return nil
}

// HPA mock methods

func (m *MockK8sClient) GetHPA(namespace, name string) (*k8s.HPAInfo, error) {
	if m.hpas == nil {
		m.hpas = make(map[string]*k8s.HPAInfo)
	}
	key := namespace + "/" + name
	hpa, ok := m.hpas[key]
	if !ok {
		return nil, fmt.Errorf("hpa %q not found", key)
	}
	return hpa, nil
}

func (m *MockK8sClient) CreateHPA(namespace string, spec k8s.HPASpec) error {
	if m.hpas == nil {
		m.hpas = make(map[string]*k8s.HPAInfo)
	}
	key := namespace + "/" + spec.Name
	if _, exists := m.hpas[key]; exists {
		return fmt.Errorf("hpa %q already exists", key)
	}
	m.hpas[key] = &k8s.HPAInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "HorizontalPodAutoscaler",
			Namespace:         namespace,
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		MinReplicas:           spec.MinReplicas,
		MaxReplicas:           spec.MaxReplicas,
		CurrentReplicas:       spec.MinReplicas,
		DesiredReplicas:       spec.MinReplicas,
		TargetKind:            spec.TargetKind,
		TargetName:            spec.TargetName,
		TargetCPUUtilization:  spec.TargetCPUUtilization,
	}
	return nil
}

func (m *MockK8sClient) UpdateHPA(namespace string, spec k8s.HPASpec) error {
	if m.hpas == nil {
		m.hpas = make(map[string]*k8s.HPAInfo)
	}
	key := namespace + "/" + spec.Name
	hpa, exists := m.hpas[key]
	if !exists {
		return fmt.Errorf("hpa %q not found", key)
	}
	if spec.MinReplicas > 0 {
		hpa.MinReplicas = spec.MinReplicas
	}
	if spec.MaxReplicas > 0 {
		hpa.MaxReplicas = spec.MaxReplicas
	}
	if spec.TargetCPUUtilization != nil {
		hpa.TargetCPUUtilization = spec.TargetCPUUtilization
	}
	if hpa.Labels == nil {
		hpa.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		hpa.Labels[k] = v
	}
	if hpa.Annotations == nil {
		hpa.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		hpa.Annotations[k] = v
	}
	return nil
}

func (m *MockK8sClient) DeleteHPA(namespace, name string) error {
	if m.hpas == nil {
		m.hpas = make(map[string]*k8s.HPAInfo)
	}
	key := namespace + "/" + name
	if _, exists := m.hpas[key]; !exists {
		return fmt.Errorf("hpa %q not found", key)
	}
	delete(m.hpas, key)
	return nil
}

func (m *MockK8sClient) WatchPods(selector k8s.PodSelector) (<-chan k8s.WatchEvent, error) {
	ch := make(chan k8s.WatchEvent)
	close(ch)
	return ch, nil
}

func (m *MockK8sClient) CreateResource(namespace string, manifest []byte) error {
	return nil
}

func (m *MockK8sClient) UpdateResource(namespace string, manifest []byte) error {
	return nil
}

func (m *MockK8sClient) DeleteResource(namespace, kind, name string) error {
	return nil
}

func (m *MockK8sClient) GetClusterInfo() (*k8s.ClusterInfo, error) {
	return &k8s.ClusterInfo{
		Version:    "v1.26.0",
		Nodes:      3,
		Namespaces: 5,
		APIServer:  "https://127.0.0.1:6443",
	}, nil
}

func (m *MockK8sClient) GetNamespace(name string) (*k8s.NamespaceInfo, error) {
	if m.namespaces == nil {
		m.namespaces = make(map[string]*k8s.NamespaceInfo)
		// Add default namespaces
		m.namespaces["default"] = &k8s.NamespaceInfo{
			ResourceInfo: k8s.ResourceInfo{
				Kind:              "Namespace",
				Name:              "default",
				Labels:            map[string]string{},
				Annotations:       map[string]string{},
				Status:            k8s.StatusRunning,
				CreationTimestamp: time.Now(),
			},
			Phase: "Active",
		}
		m.namespaces["kube-system"] = &k8s.NamespaceInfo{
			ResourceInfo: k8s.ResourceInfo{
				Kind:              "Namespace",
				Name:              "kube-system",
				Labels:            map[string]string{},
				Annotations:       map[string]string{},
				Status:            k8s.StatusRunning,
				CreationTimestamp: time.Now(),
			},
			Phase: "Active",
		}
	}

	ns, ok := m.namespaces[name]
	if !ok {
		return nil, fmt.Errorf("namespace %q not found", name)
	}
	return ns, nil
}

func (m *MockK8sClient) ListNamespaces() ([]k8s.NamespaceInfo, error) {
	if m.namespaces == nil {
		m.namespaces = make(map[string]*k8s.NamespaceInfo)
	}
	result := make([]k8s.NamespaceInfo, 0, len(m.namespaces))
	for _, ns := range m.namespaces {
		result = append(result, *ns)
	}
	return result, nil
}

func (m *MockK8sClient) CreateNamespace(spec k8s.NamespaceSpec) error {
	if m.namespaces == nil {
		m.namespaces = make(map[string]*k8s.NamespaceInfo)
	}
	if _, exists := m.namespaces[spec.Name]; exists {
		return fmt.Errorf("namespace %q already exists", spec.Name)
	}
	m.namespaces[spec.Name] = &k8s.NamespaceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "Namespace",
			Name:              spec.Name,
			Labels:            spec.Labels,
			Annotations:       spec.Annotations,
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Phase: "Active",
	}
	return nil
}

func (m *MockK8sClient) UpdateNamespace(spec k8s.NamespaceSpec) error {
	if m.namespaces == nil {
		m.namespaces = make(map[string]*k8s.NamespaceInfo)
	}
	ns, exists := m.namespaces[spec.Name]
	if !exists {
		return fmt.Errorf("namespace %q not found", spec.Name)
	}
	// Merge labels
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		ns.Labels[k] = v
	}
	// Merge annotations
	if ns.Annotations == nil {
		ns.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		ns.Annotations[k] = v
	}
	return nil
}

func (m *MockK8sClient) DeleteNamespace(name string) error {
	if m.namespaces == nil {
		m.namespaces = make(map[string]*k8s.NamespaceInfo)
	}
	if _, exists := m.namespaces[name]; !exists {
		return fmt.Errorf("namespace %q not found", name)
	}
	delete(m.namespaces, name)
	return nil
}

func TestK8sNamespaceModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	if module.Name() != "k8s_namespace" {
		t.Errorf("expected module name 'k8s_namespace', got '%s'", module.Name())
	}

	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(validStates))
	}
}

func TestK8sNamespaceCheck(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	// Test checking default namespace (exists)
	decl := &StateDeclaration{
		ID:         "default",
		State:      "present",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("expected default namespace to be present")
	}

	if result.CurrentState != "present" {
		t.Errorf("expected current state 'present', got '%s'", result.CurrentState)
	}

	if !result.Matches {
		t.Error("expected state to match")
	}
}

func TestK8sNamespaceApply(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	decl := &StateDeclaration{
		ID:         "default",
		State:      "present",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
}

func TestK8sNamespaceCreate(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	// Create a new namespace
	decl := &StateDeclaration{
		ID:     "my-new-namespace",
		State:  "present",
		Module: "k8s_namespace",
		Parameters: map[string]interface{}{
			"labels": map[string]interface{}{
				"env": "test",
				"app": "myapp",
			},
			"annotations": map[string]interface{}{
				"description": "Test namespace",
			},
		},
	}

	// First check should show namespace is absent
	checkResult, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if checkResult.Present {
		t.Error("expected namespace to be absent before creation")
	}
	if checkResult.Matches {
		t.Error("expected state to not match")
	}

	// Apply should create the namespace
	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["created"] != true {
		t.Error("expected 'created' change to be true")
	}

	// Second check should show namespace exists
	checkResult, err = module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check after create failed: %v", err)
	}
	if !checkResult.Present {
		t.Error("expected namespace to be present after creation")
	}
	if !checkResult.Matches {
		t.Errorf("expected state to match after creation, diff: %v", checkResult.Diff)
	}
}

func TestK8sNamespaceDelete(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	// First create the namespace
	createDecl := &StateDeclaration{
		ID:         "ns-to-delete",
		State:      "present",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}
	_, err := module.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Create for delete test failed: %v", err)
	}

	// Now delete it
	deleteDecl := &StateDeclaration{
		ID:         "ns-to-delete",
		State:      "absent",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Apply(context.Background(), deleteDecl)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["deleted"] != true {
		t.Error("expected 'deleted' change to be true")
	}

	// Verify namespace is gone
	checkResult, err := module.Check(context.Background(), deleteDecl)
	if err != nil {
		t.Fatalf("Check after delete failed: %v", err)
	}
	if checkResult.Present {
		t.Error("expected namespace to be absent after deletion")
	}
}

func TestK8sNamespaceUpdateLabels(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	// Create namespace
	createDecl := &StateDeclaration{
		ID:     "ns-with-labels",
		State:  "present",
		Module: "k8s_namespace",
		Parameters: map[string]interface{}{
			"labels": map[string]interface{}{
				"env": "dev",
			},
		},
	}
	_, err := module.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update with new labels
	updateDecl := &StateDeclaration{
		ID:     "ns-with-labels",
		State:  "present",
		Module: "k8s_namespace",
		Parameters: map[string]interface{}{
			"labels": map[string]interface{}{
				"env":  "prod",
				"team": "platform",
			},
		},
	}

	// Check should show labels don't match
	checkResult, err := module.Check(context.Background(), updateDecl)
	if err != nil {
		t.Fatalf("Check before update failed: %v", err)
	}
	if checkResult.Matches {
		t.Error("expected state to not match before label update")
	}

	// Apply should update labels
	result, err := module.Apply(context.Background(), updateDecl)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["updated"] != true {
		t.Error("expected 'updated' change to be true")
	}
}

func TestK8sNamespaceIdempotent(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	// Apply to default namespace (already exists)
	decl := &StateDeclaration{
		ID:         "default",
		State:      "present",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}

	// First apply
	result1, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("First apply failed: %v", err)
	}
	if !result1.Success {
		t.Errorf("expected success, got failure: %s", result1.Comment)
	}

	// Second apply should be idempotent (no changes)
	result2, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
	if !result2.Success {
		t.Errorf("expected success, got failure: %s", result2.Comment)
	}
	// No changes should be recorded for idempotent operation
	if len(result2.Changes) > 0 {
		t.Errorf("expected no changes for idempotent operation, got: %v", result2.Changes)
	}
}

func TestK8sNamespaceAbsentAlready(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	// Try to delete a namespace that doesn't exist
	decl := &StateDeclaration{
		ID:         "nonexistent-namespace",
		State:      "absent",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes since namespace was already absent
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes since namespace was already absent, got: %v", result.Changes)
	}
}

func TestK8sDeploymentModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	if module.Name() != "k8s_deployment" {
		t.Errorf("expected module name 'k8s_deployment', got '%s'", module.Name())
	}

	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(validStates))
	}
}

func TestK8sDeploymentCheck(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	// First create a deployment to check
	client.deployments = make(map[string]*k8s.DeploymentInfo)
	client.deployments["default/test-deployment"] = &k8s.DeploymentInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:        "Deployment",
			Namespace:   "default",
			Name:        "test-deployment",
			Labels:      map[string]string{"app": "test"},
			Annotations: map[string]string{},
			Status:      k8s.StatusRunning,
		},
		Replicas:          3,
		AvailableReplicas: 3,
		ReadyReplicas:     3,
		UpdatedReplicas:   3,
	}

	decl := &StateDeclaration{
		ID:     "test-deployment",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(3),
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("expected deployment to be present")
	}

	if result.CurrentState != "present" {
		t.Errorf("expected current state 'present', got '%s'", result.CurrentState)
	}

	if !result.Matches {
		t.Errorf("expected state to match, diff: %v", result.Diff)
	}
}

func TestK8sDeploymentCheckAbsent(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	decl := &StateDeclaration{
		ID:     "nonexistent-deployment",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected deployment to be absent")
	}

	if result.CurrentState != "absent" {
		t.Errorf("expected current state 'absent', got '%s'", result.CurrentState)
	}

	if result.Matches {
		t.Error("expected state to not match")
	}
}

func TestK8sDeploymentCreate(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	decl := &StateDeclaration{
		ID:     "new-deployment",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(3),
			"image":     "nginx:latest",
			"labels": map[string]interface{}{
				"app": "nginx",
			},
			"annotations": map[string]interface{}{
				"description": "Test deployment",
			},
			"container_port": int32(80),
			"selector": map[string]interface{}{
				"app": "nginx",
			},
		},
	}

	// First check should show deployment is absent
	checkResult, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if checkResult.Present {
		t.Error("expected deployment to be absent before creation")
	}

	// Apply should create the deployment
	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["created"] != true {
		t.Error("expected 'created' change to be true")
	}
	if result.Changes["image"] != "nginx:latest" {
		t.Errorf("expected image 'nginx:latest', got '%v'", result.Changes["image"])
	}

	// Second check should show deployment exists
	checkResult, err = module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check after create failed: %v", err)
	}
	if !checkResult.Present {
		t.Error("expected deployment to be present after creation")
	}
	if !checkResult.Matches {
		t.Errorf("expected state to match after creation, diff: %v", checkResult.Diff)
	}
}

func TestK8sDeploymentCreateRequiresImage(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	decl := &StateDeclaration{
		ID:     "new-deployment-no-image",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(3),
			// No image specified
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err == nil {
		t.Fatal("expected error for missing image")
	}
	if result.Success {
		t.Error("expected failure for missing image")
	}
}

func TestK8sDeploymentDelete(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	// First create the deployment
	createDecl := &StateDeclaration{
		ID:     "dep-to-delete",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(1),
			"image":     "nginx:latest",
		},
	}
	_, err := module.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Create for delete test failed: %v", err)
	}

	// Now delete it
	deleteDecl := &StateDeclaration{
		ID:     "dep-to-delete",
		State:  "absent",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), deleteDecl)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["deleted"] != true {
		t.Error("expected 'deleted' change to be true")
	}

	// Verify deployment is gone
	checkResult, err := module.Check(context.Background(), deleteDecl)
	if err != nil {
		t.Fatalf("Check after delete failed: %v", err)
	}
	if checkResult.Present {
		t.Error("expected deployment to be absent after deletion")
	}
}

func TestK8sDeploymentUpdateReplicas(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	// Create deployment with 2 replicas
	createDecl := &StateDeclaration{
		ID:     "dep-scale",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(2),
			"image":     "nginx:latest",
		},
	}
	_, err := module.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update to 5 replicas
	updateDecl := &StateDeclaration{
		ID:     "dep-scale",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(5),
		},
	}

	// Check should show replicas don't match
	checkResult, err := module.Check(context.Background(), updateDecl)
	if err != nil {
		t.Fatalf("Check before update failed: %v", err)
	}
	if checkResult.Matches {
		t.Error("expected state to not match before replica update")
	}

	// Apply should update replicas
	result, err := module.Apply(context.Background(), updateDecl)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["updated"] != true {
		t.Error("expected 'updated' change to be true")
	}
}

func TestK8sDeploymentUpdateLabels(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	// Create deployment with initial labels
	createDecl := &StateDeclaration{
		ID:     "dep-labels",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(1),
			"image":     "nginx:latest",
			"labels": map[string]interface{}{
				"env": "dev",
			},
		},
	}
	_, err := module.Apply(context.Background(), createDecl)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update with new labels
	updateDecl := &StateDeclaration{
		ID:     "dep-labels",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(1),
			"labels": map[string]interface{}{
				"env":  "prod",
				"team": "platform",
			},
		},
	}

	// Check should show labels don't match
	checkResult, err := module.Check(context.Background(), updateDecl)
	if err != nil {
		t.Fatalf("Check before update failed: %v", err)
	}
	if checkResult.Matches {
		t.Error("expected state to not match before label update")
	}

	// Apply should update labels
	result, err := module.Apply(context.Background(), updateDecl)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["updated"] != true {
		t.Error("expected 'updated' change to be true")
	}
}

func TestK8sDeploymentIdempotent(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	// Create deployment
	decl := &StateDeclaration{
		ID:     "dep-idempotent",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(3),
			"image":     "nginx:latest",
			"labels": map[string]interface{}{
				"app": "nginx",
			},
		},
	}

	// First apply creates
	result1, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("First apply failed: %v", err)
	}
	if !result1.Success {
		t.Errorf("expected success, got failure: %s", result1.Comment)
	}
	if result1.Changes["created"] != true {
		t.Error("expected 'created' change on first apply")
	}

	// Second apply should be idempotent (no changes)
	result2, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
	if !result2.Success {
		t.Errorf("expected success, got failure: %s", result2.Comment)
	}
	// No changes should be recorded for idempotent operation
	if len(result2.Changes) > 0 {
		t.Errorf("expected no changes for idempotent operation, got: %v", result2.Changes)
	}
}

func TestK8sDeploymentAbsentAlready(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	// Try to delete a deployment that doesn't exist
	decl := &StateDeclaration{
		ID:     "nonexistent-deployment",
		State:  "absent",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes since deployment was already absent
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes since deployment was already absent, got: %v", result.Changes)
	}
}

func TestGetSelectorLabels(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected map[string]string
	}{
		{
			name: "with selector",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"selector": map[string]interface{}{
						"app":  "nginx",
						"tier": "frontend",
					},
				},
			},
			expected: map[string]string{"app": "nginx", "tier": "frontend"},
		},
		{
			name: "no selector",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSelectorLabels(tt.decl)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("expected selector[%s]='%s', got '%s'", k, v, result[k])
				}
			}
		})
	}
}

func TestGetInt32Parameter(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected int32
	}{
		{"int", int(42), 42},
		{"int32", int32(42), 42},
		{"int64", int64(42), 42},
		{"float64", float64(42), 42},
		{"missing", nil, 10}, // Uses default value
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{
				Parameters: map[string]interface{}{},
			}
			if tt.value != nil {
				decl.Parameters["test"] = tt.value
			}

			result := getInt32Parameter(decl, "test", 10)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetNamespace(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected string
	}{
		{
			name: "explicit namespace",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"namespace": "custom",
				},
			},
			expected: "custom",
		},
		{
			name: "default namespace",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getNamespace(tt.decl)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestCompareLabels(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string]string
		desired  map[string]string
		expected bool
	}{
		{
			name:     "identical labels",
			current:  map[string]string{"app": "test", "version": "1.0"},
			desired:  map[string]string{"app": "test", "version": "1.0"},
			expected: true,
		},
		{
			name:     "different labels",
			current:  map[string]string{"app": "test"},
			desired:  map[string]string{"app": "test", "version": "1.0"},
			expected: false,
		},
		{
			name:     "empty labels",
			current:  map[string]string{},
			desired:  map[string]string{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareLabels(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// ==================== K8s Service Module Tests ====================

func TestK8sServiceModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sServiceModule(client)

	if module.Name() != "k8s_service" {
		t.Errorf("expected module name 'k8s_service', got '%s'", module.Name())
	}
}

func TestK8sServiceCheck_NotFound(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "my-service",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "ClusterIP",
			"ports": []interface{}{
				map[string]interface{}{
					"port":        int(80),
					"target_port": int(8080),
					"protocol":    "TCP",
				},
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Present {
		t.Error("expected service to not be present")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected current state 'absent', got '%s'", result.CurrentState)
	}
	if result.Matches {
		t.Error("expected Matches to be false for non-existent service")
	}
}

func TestK8sServiceCheck_Found(t *testing.T) {
	client := &MockK8sClient{
		services: make(map[string]*k8s.ServiceInfo),
	}
	// Pre-populate a service
	client.services["default/my-service"] = &k8s.ServiceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "my-service",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Type:      "ClusterIP",
		ClusterIP: "10.0.0.100",
		Ports: []k8s.ServicePort{
			{Port: 80, TargetPort: 8080, Protocol: "TCP"},
		},
	}

	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "my-service",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "ClusterIP",
			"ports": []interface{}{
				map[string]interface{}{
					"port":        int(80),
					"target_port": int(8080),
					"protocol":    "TCP",
				},
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Present {
		t.Error("expected service to be present")
	}
	if result.CurrentState != "present" {
		t.Errorf("expected current state 'present', got '%s'", result.CurrentState)
	}
	if !result.Matches {
		t.Errorf("expected Matches to be true, diff: %v", result.Diff)
	}
}

func TestK8sServiceCreate(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "new-service",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "ClusterIP",
			"selector": map[string]interface{}{
				"app": "myapp",
			},
			"ports": []interface{}{
				map[string]interface{}{
					"name":        "http",
					"port":        int(80),
					"target_port": int(8080),
					"protocol":    "TCP",
				},
			},
			"labels": map[string]interface{}{
				"env": "test",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["created"] != true {
		t.Error("expected 'created' change to be true")
	}

	// Verify service was created
	svc, exists := client.services["default/new-service"]
	if !exists {
		t.Fatal("service was not created in mock client")
	}
	if svc.Type != "ClusterIP" {
		t.Errorf("expected type ClusterIP, got %s", svc.Type)
	}
}

func TestK8sServiceCreate_MissingPorts(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "bad-service",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "ClusterIP",
			// No ports defined
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err == nil {
		t.Fatal("expected error for missing ports")
	}
	if result.Success {
		t.Error("expected failure for missing ports")
	}
}

func TestK8sServiceDelete(t *testing.T) {
	client := &MockK8sClient{
		services: make(map[string]*k8s.ServiceInfo),
	}
	// Pre-populate a service
	client.services["default/delete-me"] = &k8s.ServiceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "delete-me",
			Namespace: "default",
		},
		Type: "ClusterIP",
		Ports: []k8s.ServicePort{
			{Port: 80, TargetPort: 80, Protocol: "TCP"},
		},
	}

	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "delete-me",
		State: "absent",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["deleted"] != true {
		t.Error("expected 'deleted' change to be true")
	}

	// Verify service was deleted
	if _, exists := client.services["default/delete-me"]; exists {
		t.Error("service should have been deleted")
	}
}

func TestK8sServiceUpdate(t *testing.T) {
	client := &MockK8sClient{
		services: make(map[string]*k8s.ServiceInfo),
	}
	// Pre-populate a service with old config
	client.services["default/update-me"] = &k8s.ServiceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "update-me",
			Namespace: "default",
			Labels:    map[string]string{"app": "old"},
		},
		Type: "ClusterIP",
		Ports: []k8s.ServicePort{
			{Port: 80, TargetPort: 80, Protocol: "TCP"},
		},
	}

	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "update-me",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "NodePort", // Changed type
			"ports": []interface{}{
				map[string]interface{}{
					"port":        int(80),
					"target_port": int(8080), // Changed target port
					"protocol":    "TCP",
				},
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["updated"] != true {
		t.Error("expected 'updated' change to be true")
	}

	// Verify service was updated
	svc := client.services["default/update-me"]
	if svc.Type != "NodePort" {
		t.Errorf("expected type NodePort, got %s", svc.Type)
	}
}

func TestK8sServiceIdempotent(t *testing.T) {
	client := &MockK8sClient{
		services: make(map[string]*k8s.ServiceInfo),
	}
	// Pre-populate a service matching desired state
	client.services["default/existing"] = &k8s.ServiceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "existing",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Type: "ClusterIP",
		Ports: []k8s.ServicePort{
			{Port: 80, TargetPort: 8080, Protocol: "TCP"},
		},
	}

	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "existing",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "ClusterIP",
			"ports": []interface{}{
				map[string]interface{}{
					"port":        int(80),
					"target_port": int(8080),
					"protocol":    "TCP",
				},
			},
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes should be made
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes for idempotent apply, got: %v", result.Changes)
	}
}

func TestK8sServiceAbsentAlready(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sServiceModule(client)

	decl := &StateDeclaration{
		ID:    "nonexistent",
		State: "absent",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes since service was already absent
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes since service was already absent, got: %v", result.Changes)
	}
}

func TestK8sServiceTest(t *testing.T) {
	client := &MockK8sClient{
		services: make(map[string]*k8s.ServiceInfo),
	}
	client.services["default/test-svc"] = &k8s.ServiceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "test-svc",
			Namespace: "default",
		},
		Type: "ClusterIP",
		Ports: []k8s.ServicePort{
			{Port: 80, TargetPort: 8080, Protocol: "TCP"},
		},
	}

	module := NewK8sServiceModule(client)

	// Test matching state
	decl := &StateDeclaration{
		ID:    "test-svc",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "ClusterIP",
			"ports": []interface{}{
				map[string]interface{}{
					"port":        int(80),
					"target_port": int(8080),
					"protocol":    "TCP",
				},
			},
		},
	}

	matches, err := module.Test(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matches {
		t.Error("expected Test to return true for matching state")
	}

	// Test non-matching state
	decl.State = "absent"
	matches, err = module.Test(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matches {
		t.Error("expected Test to return false for non-matching state")
	}
}

func TestGetServicePorts(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected []k8s.ServicePort
	}{
		{
			name: "single port",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port":        int(80),
							"target_port": int(8080),
							"protocol":    "TCP",
						},
					},
				},
			},
			expected: []k8s.ServicePort{
				{Port: 80, TargetPort: 8080, Protocol: "TCP"},
			},
		},
		{
			name: "multiple ports",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"name":        "http",
							"port":        int(80),
							"target_port": int(8080),
							"protocol":    "TCP",
						},
						map[string]interface{}{
							"name":        "https",
							"port":        int(443),
							"target_port": int(8443),
							"protocol":    "TCP",
						},
					},
				},
			},
			expected: []k8s.ServicePort{
				{Name: "http", Port: 80, TargetPort: 8080, Protocol: "TCP"},
				{Name: "https", Port: 443, TargetPort: 8443, Protocol: "TCP"},
			},
		},
		{
			name: "no ports",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: nil,
		},
		{
			name: "port defaults target_port to port",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"ports": []interface{}{
						map[string]interface{}{
							"port":     int(80),
							"protocol": "TCP",
						},
					},
				},
			},
			expected: []k8s.ServicePort{
				{Port: 80, TargetPort: 80, Protocol: "TCP"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getServicePorts(tt.decl)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d ports, got %d", len(tt.expected), len(result))
			}
			for i, exp := range tt.expected {
				if result[i].Port != exp.Port {
					t.Errorf("port[%d]: expected Port %d, got %d", i, exp.Port, result[i].Port)
				}
				if result[i].TargetPort != exp.TargetPort {
					t.Errorf("port[%d]: expected TargetPort %d, got %d", i, exp.TargetPort, result[i].TargetPort)
				}
				if result[i].Protocol != exp.Protocol {
					t.Errorf("port[%d]: expected Protocol %s, got %s", i, exp.Protocol, result[i].Protocol)
				}
			}
		})
	}
}

func TestCompareServicePorts(t *testing.T) {
	tests := []struct {
		name     string
		current  []k8s.ServicePort
		desired  []k8s.ServicePort
		expected bool
	}{
		{
			name: "identical ports",
			current: []k8s.ServicePort{
				{Port: 80, TargetPort: 8080, Protocol: "TCP"},
			},
			desired: []k8s.ServicePort{
				{Port: 80, TargetPort: 8080, Protocol: "TCP"},
			},
			expected: true,
		},
		{
			name: "different target port",
			current: []k8s.ServicePort{
				{Port: 80, TargetPort: 8080, Protocol: "TCP"},
			},
			desired: []k8s.ServicePort{
				{Port: 80, TargetPort: 9090, Protocol: "TCP"},
			},
			expected: false,
		},
		{
			name: "different protocol",
			current: []k8s.ServicePort{
				{Port: 53, TargetPort: 53, Protocol: "TCP"},
			},
			desired: []k8s.ServicePort{
				{Port: 53, TargetPort: 53, Protocol: "UDP"},
			},
			expected: false,
		},
		{
			name: "different count",
			current: []k8s.ServicePort{
				{Port: 80, TargetPort: 80, Protocol: "TCP"},
			},
			desired: []k8s.ServicePort{
				{Port: 80, TargetPort: 80, Protocol: "TCP"},
				{Port: 443, TargetPort: 443, Protocol: "TCP"},
			},
			expected: false,
		},
		{
			name:     "empty ports",
			current:  []k8s.ServicePort{},
			desired:  []k8s.ServicePort{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareServicePorts(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestToInt32(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected int32
	}{
		{"int", int(42), 42},
		{"int32", int32(42), 42},
		{"int64", int64(42), 42},
		{"float64", float64(42.0), 42},
		{"string", "42", 0}, // Unsupported type returns 0
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toInt32(tt.value)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetMapStringValue(t *testing.T) {
	m := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}

	if v := getMapStringValue(m, "key1", "default"); v != "value1" {
		t.Errorf("expected 'value1', got '%s'", v)
	}
	if v := getMapStringValue(m, "key2", "default"); v != "default" {
		t.Errorf("expected 'default' for non-string value, got '%s'", v)
	}
	if v := getMapStringValue(m, "missing", "default"); v != "default" {
		t.Errorf("expected 'default' for missing key, got '%s'", v)
	}
}

// ==================== K8s ConfigMap Module Tests ====================

func TestK8sConfigMapModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sConfigMapModule(client)

	if module.Name() != "k8s_configmap" {
		t.Errorf("expected module name 'k8s_configmap', got '%s'", module.Name())
	}
}

func TestK8sConfigMapCheck_NotFound(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sConfigMapModule(client)

	decl := &StateDeclaration{
		ID:    "my-config",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"data": map[string]interface{}{
				"key1": "value1",
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Present {
		t.Error("expected configmap to not be present")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected current state 'absent', got '%s'", result.CurrentState)
	}
	if result.Matches {
		t.Error("expected Matches to be false for non-existent configmap")
	}
}

func TestK8sConfigMapCheck_Found(t *testing.T) {
	client := &MockK8sClient{
		configmaps: make(map[string]*k8s.ConfigMapInfo),
	}
	// Pre-populate a configmap
	client.configmaps["default/my-config"] = &k8s.ConfigMapInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "my-config",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	module := NewK8sConfigMapModule(client)

	decl := &StateDeclaration{
		ID:    "my-config",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"data": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Present {
		t.Error("expected configmap to be present")
	}
	if result.CurrentState != "present" {
		t.Errorf("expected current state 'present', got '%s'", result.CurrentState)
	}
	if !result.Matches {
		t.Errorf("expected Matches to be true, diff: %v", result.Diff)
	}
}

func TestK8sConfigMapCreate(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sConfigMapModule(client)

	decl := &StateDeclaration{
		ID:    "new-config",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"data": map[string]interface{}{
				"config.yaml":   "key: value",
				"settings.json": `{"debug": true}`,
			},
			"labels": map[string]interface{}{
				"app": "myapp",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["created"] != true {
		t.Error("expected 'created' change to be true")
	}

	// Verify configmap was created
	cm, exists := client.configmaps["default/new-config"]
	if !exists {
		t.Fatal("configmap was not created in mock client")
	}
	if cm.Data["config.yaml"] != "key: value" {
		t.Errorf("expected data[config.yaml]='key: value', got '%s'", cm.Data["config.yaml"])
	}
}

func TestK8sConfigMapDelete(t *testing.T) {
	client := &MockK8sClient{
		configmaps: make(map[string]*k8s.ConfigMapInfo),
	}
	// Pre-populate a configmap
	client.configmaps["default/delete-me"] = &k8s.ConfigMapInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "delete-me",
			Namespace: "default",
		},
		Data: map[string]string{"key": "value"},
	}

	module := NewK8sConfigMapModule(client)

	decl := &StateDeclaration{
		ID:    "delete-me",
		State: "absent",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["deleted"] != true {
		t.Error("expected 'deleted' change to be true")
	}

	// Verify configmap was deleted
	if _, exists := client.configmaps["default/delete-me"]; exists {
		t.Error("configmap should have been deleted")
	}
}

func TestK8sConfigMapUpdate(t *testing.T) {
	client := &MockK8sClient{
		configmaps: make(map[string]*k8s.ConfigMapInfo),
	}
	// Pre-populate a configmap with old data
	client.configmaps["default/update-me"] = &k8s.ConfigMapInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "update-me",
			Namespace: "default",
		},
		Data: map[string]string{
			"old_key": "old_value",
		},
	}

	module := NewK8sConfigMapModule(client)

	decl := &StateDeclaration{
		ID:    "update-me",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"data": map[string]interface{}{
				"new_key": "new_value",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["updated"] != true {
		t.Error("expected 'updated' change to be true")
	}

	// Verify configmap was updated
	cm := client.configmaps["default/update-me"]
	if cm.Data["new_key"] != "new_value" {
		t.Errorf("expected data[new_key]='new_value', got '%s'", cm.Data["new_key"])
	}
}

func TestK8sConfigMapIdempotent(t *testing.T) {
	client := &MockK8sClient{
		configmaps: make(map[string]*k8s.ConfigMapInfo),
	}
	// Pre-populate a configmap matching desired state
	client.configmaps["default/existing"] = &k8s.ConfigMapInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "existing",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Data: map[string]string{
			"config": "value",
		},
	}

	module := NewK8sConfigMapModule(client)

	decl := &StateDeclaration{
		ID:    "existing",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"data": map[string]interface{}{
				"config": "value",
			},
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes should be made
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes for idempotent apply, got: %v", result.Changes)
	}
}

func TestK8sConfigMapAbsentAlready(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sConfigMapModule(client)

	decl := &StateDeclaration{
		ID:    "nonexistent",
		State: "absent",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes since configmap was already absent
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes since configmap was already absent, got: %v", result.Changes)
	}
}

func TestK8sConfigMapTest(t *testing.T) {
	client := &MockK8sClient{
		configmaps: make(map[string]*k8s.ConfigMapInfo),
	}
	client.configmaps["default/test-cm"] = &k8s.ConfigMapInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	module := NewK8sConfigMapModule(client)

	// Test matching state
	decl := &StateDeclaration{
		ID:    "test-cm",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	matches, err := module.Test(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matches {
		t.Error("expected Test to return true for matching state")
	}

	// Test non-matching state
	decl.State = "absent"
	matches, err = module.Test(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matches {
		t.Error("expected Test to return false for non-matching state")
	}
}

func TestGetConfigMapData(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected map[string]string
	}{
		{
			name: "with data",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"data": map[string]interface{}{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			expected: map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name: "no data",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: nil,
		},
		{
			name: "empty data",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"data": map[string]interface{}{},
				},
			},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getConfigMapData(tt.decl)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("expected data[%s]='%s', got '%s'", k, v, result[k])
				}
			}
		})
	}
}

func TestGetMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected int
	}{
		{
			name:     "with keys",
			input:    map[string]string{"a": "1", "b": "2", "c": "3"},
			expected: 3,
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: 0,
		},
		{
			name:     "nil map",
			input:    nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMapKeys(tt.input)
			if tt.expected == 0 && result != nil && len(result) > 0 {
				t.Errorf("expected empty/nil, got %v", result)
			} else if len(result) != tt.expected {
				t.Errorf("expected %d keys, got %d", tt.expected, len(result))
			}
		})
	}
}

// ==================== K8s Secret Module Tests ====================

func TestK8sSecretModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sSecretModule(client)

	if module.Name() != "k8s_secret" {
		t.Errorf("expected module name 'k8s_secret', got '%s'", module.Name())
	}

	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(validStates))
	}
}

func TestK8sSecretCheck_NotFound(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "my-secret",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"string_data": map[string]interface{}{
				"password": "secret123",
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Present {
		t.Error("expected secret to not be present")
	}
	if result.CurrentState != "absent" {
		t.Errorf("expected current state 'absent', got '%s'", result.CurrentState)
	}
	if result.Matches {
		t.Error("expected Matches to be false for non-existent secret")
	}
}

func TestK8sSecretCheck_Found(t *testing.T) {
	client := &MockK8sClient{
		secrets: make(map[string]*k8s.SecretInfo),
	}
	// Pre-populate a secret
	client.secrets["default/my-secret"] = &k8s.SecretInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "my-secret",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret123"),
		},
	}

	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "my-secret",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "Opaque",
			"string_data": map[string]interface{}{
				"username": "admin",
				"password": "secret123",
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Present {
		t.Error("expected secret to be present")
	}
	if result.CurrentState != "present" {
		t.Errorf("expected current state 'present', got '%s'", result.CurrentState)
	}
	if !result.Matches {
		t.Errorf("expected Matches to be true, diff: %v", result.Diff)
	}
}

func TestK8sSecretCreate(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "new-secret",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "Opaque",
			"string_data": map[string]interface{}{
				"api_key":    "abc123xyz",
				"api_secret": "supersecret",
			},
			"labels": map[string]interface{}{
				"app": "myapp",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["created"] != true {
		t.Error("expected 'created' change to be true")
	}

	// Verify secret was created
	secret, exists := client.secrets["default/new-secret"]
	if !exists {
		t.Fatal("secret was not created in mock client")
	}
	if secret.Type != "Opaque" {
		t.Errorf("expected type Opaque, got %s", secret.Type)
	}
	if string(secret.Data["api_key"]) != "abc123xyz" {
		t.Errorf("expected data[api_key]='abc123xyz', got '%s'", string(secret.Data["api_key"]))
	}
}

func TestK8sSecretCreate_TLSType(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "tls-secret",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "kubernetes.io/tls",
			"string_data": map[string]interface{}{
				"tls.crt": "-----BEGIN CERTIFICATE-----\nMIIC...",
				"tls.key": "-----BEGIN PRIVATE KEY-----\nMIIE...",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}

	// Verify secret type
	secret := client.secrets["default/tls-secret"]
	if secret.Type != "kubernetes.io/tls" {
		t.Errorf("expected type kubernetes.io/tls, got %s", secret.Type)
	}
}

func TestK8sSecretDelete(t *testing.T) {
	client := &MockK8sClient{
		secrets: make(map[string]*k8s.SecretInfo),
	}
	// Pre-populate a secret
	client.secrets["default/delete-me"] = &k8s.SecretInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "delete-me",
			Namespace: "default",
		},
		Type: "Opaque",
		Data: map[string][]byte{"key": []byte("value")},
	}

	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "delete-me",
		State: "absent",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["deleted"] != true {
		t.Error("expected 'deleted' change to be true")
	}

	// Verify secret was deleted
	if _, exists := client.secrets["default/delete-me"]; exists {
		t.Error("secret should have been deleted")
	}
}

func TestK8sSecretUpdate(t *testing.T) {
	client := &MockK8sClient{
		secrets: make(map[string]*k8s.SecretInfo),
	}
	// Pre-populate a secret with old data
	client.secrets["default/update-me"] = &k8s.SecretInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "update-me",
			Namespace: "default",
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"old_key": []byte("old_value"),
		},
	}

	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "update-me",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"string_data": map[string]interface{}{
				"new_key": "new_value",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	if result.Changes["updated"] != true {
		t.Error("expected 'updated' change to be true")
	}

	// Verify secret was updated
	secret := client.secrets["default/update-me"]
	if string(secret.Data["new_key"]) != "new_value" {
		t.Errorf("expected data[new_key]='new_value', got '%s'", string(secret.Data["new_key"]))
	}
}

func TestK8sSecretIdempotent(t *testing.T) {
	client := &MockK8sClient{
		secrets: make(map[string]*k8s.SecretInfo),
	}
	// Pre-populate a secret matching desired state
	client.secrets["default/existing"] = &k8s.SecretInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "existing",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"secret": []byte("value"),
		},
	}

	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "existing",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "Opaque",
			"string_data": map[string]interface{}{
				"secret": "value",
			},
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes should be made
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes for idempotent apply, got: %v", result.Changes)
	}
}

func TestK8sSecretAbsentAlready(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sSecretModule(client)

	decl := &StateDeclaration{
		ID:    "nonexistent",
		State: "absent",
		Parameters: map[string]interface{}{
			"namespace": "default",
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
	// No changes since secret was already absent
	if len(result.Changes) > 0 {
		t.Errorf("expected no changes since secret was already absent, got: %v", result.Changes)
	}
}

func TestK8sSecretTest(t *testing.T) {
	client := &MockK8sClient{
		secrets: make(map[string]*k8s.SecretInfo),
	}
	client.secrets["default/test-secret"] = &k8s.SecretInfo{
		ResourceInfo: k8s.ResourceInfo{
			Name:      "test-secret",
			Namespace: "default",
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}

	module := NewK8sSecretModule(client)

	// Test matching state
	decl := &StateDeclaration{
		ID:    "test-secret",
		State: "present",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"type":      "Opaque",
			"string_data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	matches, err := module.Test(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matches {
		t.Error("expected Test to return true for matching state")
	}

	// Test non-matching state
	decl.State = "absent"
	matches, err = module.Test(context.Background(), decl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matches {
		t.Error("expected Test to return false for non-matching state")
	}
}

func TestGetSecretData(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected map[string][]byte
	}{
		{
			name: "with string_data",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"string_data": map[string]interface{}{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			expected: map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
		},
		{
			name: "with data (bytes)",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"data": map[string]interface{}{
						"key1": []byte("value1"),
					},
				},
			},
			expected: map[string][]byte{"key1": []byte("value1")},
		},
		{
			name: "no data",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: nil,
		},
		{
			name: "combined data and string_data",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"data": map[string]interface{}{
						"binary_key": []byte("binary_value"),
					},
					"string_data": map[string]interface{}{
						"string_key": "string_value",
					},
				},
			},
			expected: map[string][]byte{
				"binary_key": []byte("binary_value"),
				"string_key": []byte("string_value"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSecretData(tt.decl)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d entries, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if string(result[k]) != string(v) {
					t.Errorf("expected data[%s]='%s', got '%s'", k, string(v), string(result[k]))
				}
			}
		})
	}
}

func TestCompareSecretData(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string][]byte
		desired  map[string][]byte
		expected bool
	}{
		{
			name:     "identical data",
			current:  map[string][]byte{"key": []byte("value")},
			desired:  map[string][]byte{"key": []byte("value")},
			expected: true,
		},
		{
			name:     "different values",
			current:  map[string][]byte{"key": []byte("value1")},
			desired:  map[string][]byte{"key": []byte("value2")},
			expected: false,
		},
		{
			name:     "different keys",
			current:  map[string][]byte{"key1": []byte("value")},
			desired:  map[string][]byte{"key2": []byte("value")},
			expected: false,
		},
		{
			name:     "different count",
			current:  map[string][]byte{"key1": []byte("value1")},
			desired:  map[string][]byte{"key1": []byte("value1"), "key2": []byte("value2")},
			expected: false,
		},
		{
			name:     "empty data",
			current:  map[string][]byte{},
			desired:  map[string][]byte{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareSecretData(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetSecretType(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected string
	}{
		{
			name: "explicit type",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"type": "kubernetes.io/tls",
				},
			},
			expected: "kubernetes.io/tls",
		},
		{
			name: "default type",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: "Opaque",
		},
		{
			name: "dockerconfigjson type",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"type": "kubernetes.io/dockerconfigjson",
				},
			},
			expected: "kubernetes.io/dockerconfigjson",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSecretType(tt.decl)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// ============= K8s Ingress Module Tests =============

func TestK8sIngressModule(t *testing.T) {
	t.Run("Check absent ingress", func(t *testing.T) {
		client := &MockK8sClient{}
		module := NewK8sIngressModule(client)

		decl := &StateDeclaration{
			ID:    "my-ingress",
			State: "absent",
			Parameters: map[string]interface{}{
				"namespace": "default",
			},
		}

		result, err := module.Check(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Present {
			t.Error("expected ingress to not be present")
		}
		if !result.Matches {
			t.Error("expected state to match (absent ingress, want absent)")
		}
	})

	t.Run("Check present ingress", func(t *testing.T) {
		client := &MockK8sClient{
			ingresses: map[string]*k8s.IngressInfo{
				"default/my-ingress": {
					ResourceInfo: k8s.ResourceInfo{
						Kind:      "Ingress",
						Namespace: "default",
						Name:      "my-ingress",
						Labels:    map[string]string{"app": "test"},
					},
					IngressClassName: "nginx",
					Rules: []k8s.IngressRule{
						{
							Host: "example.com",
							Paths: []k8s.IngressPath{
								{
									Path:     "/",
									PathType: "Prefix",
									Backend: k8s.IngressBackend{
										ServiceName: "my-service",
										ServicePort: 80,
									},
								},
							},
						},
					},
				},
			},
		}
		module := NewK8sIngressModule(client)

		decl := &StateDeclaration{
			ID:    "my-ingress",
			State: "present",
			Parameters: map[string]interface{}{
				"namespace":     "default",
				"ingress_class": "nginx",
				"labels":        map[string]interface{}{"app": "test"},
				"rules": []interface{}{
					map[string]interface{}{
						"host": "example.com",
						"paths": []interface{}{
							map[string]interface{}{
								"path":      "/",
								"path_type": "Prefix",
								"backend": map[string]interface{}{
									"service": "my-service",
									"port":    80,
								},
							},
						},
					},
				},
			},
		}

		result, err := module.Check(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Present {
			t.Error("expected ingress to be present")
		}
		if !result.Matches {
			t.Errorf("expected state to match, diff: %v", result.Diff)
		}
	})

	t.Run("Apply create ingress", func(t *testing.T) {
		client := &MockK8sClient{}
		module := NewK8sIngressModule(client)

		decl := &StateDeclaration{
			ID:    "my-ingress",
			State: "present",
			Parameters: map[string]interface{}{
				"namespace":     "default",
				"ingress_class": "nginx",
				"labels":        map[string]interface{}{"app": "web"},
				"rules": []interface{}{
					map[string]interface{}{
						"host": "app.example.com",
						"paths": []interface{}{
							map[string]interface{}{
								"path":      "/api",
								"path_type": "Prefix",
								"backend": map[string]interface{}{
									"service": "api-service",
									"port":    8080,
								},
							},
						},
					},
				},
				"tls": []interface{}{
					map[string]interface{}{
						"hosts":       []interface{}{"app.example.com"},
						"secret_name": "tls-secret",
					},
				},
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Comment)
		}
		if result.Changes["created"] != true {
			t.Error("expected 'created' in changes")
		}

		// Verify ingress was created
		ingress, err := client.GetIngress("default", "my-ingress")
		if err != nil {
			t.Fatalf("ingress not created: %v", err)
		}
		if ingress.IngressClassName != "nginx" {
			t.Errorf("expected ingress class 'nginx', got '%s'", ingress.IngressClassName)
		}
		if len(ingress.Rules) != 1 {
			t.Errorf("expected 1 rule, got %d", len(ingress.Rules))
		}
		if len(ingress.TLS) != 1 {
			t.Errorf("expected 1 TLS config, got %d", len(ingress.TLS))
		}
	})

	t.Run("Apply update ingress", func(t *testing.T) {
		client := &MockK8sClient{
			ingresses: map[string]*k8s.IngressInfo{
				"default/my-ingress": {
					ResourceInfo: k8s.ResourceInfo{
						Kind:      "Ingress",
						Namespace: "default",
						Name:      "my-ingress",
						Labels:    map[string]string{"app": "old"},
					},
					IngressClassName: "nginx",
					Rules: []k8s.IngressRule{
						{
							Host: "old.example.com",
							Paths: []k8s.IngressPath{
								{
									Path:     "/",
									PathType: "Prefix",
									Backend: k8s.IngressBackend{
										ServiceName: "old-service",
										ServicePort: 80,
									},
								},
							},
						},
					},
				},
			},
		}
		module := NewK8sIngressModule(client)

		decl := &StateDeclaration{
			ID:    "my-ingress",
			State: "present",
			Parameters: map[string]interface{}{
				"namespace":     "default",
				"ingress_class": "nginx",
				"labels":        map[string]interface{}{"app": "new"},
				"rules": []interface{}{
					map[string]interface{}{
						"host": "new.example.com",
						"paths": []interface{}{
							map[string]interface{}{
								"path":      "/v2",
								"path_type": "Prefix",
								"backend": map[string]interface{}{
									"service": "new-service",
									"port":    8080,
								},
							},
						},
					},
				},
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Comment)
		}
		if result.Changes["updated"] != true {
			t.Error("expected 'updated' in changes")
		}

		// Verify ingress was updated
		ingress, err := client.GetIngress("default", "my-ingress")
		if err != nil {
			t.Fatalf("ingress not found: %v", err)
		}
		if ingress.Labels["app"] != "new" {
			t.Errorf("expected label app='new', got '%s'", ingress.Labels["app"])
		}
		if len(ingress.Rules) != 1 || ingress.Rules[0].Host != "new.example.com" {
			t.Error("rules not updated correctly")
		}
	})

	t.Run("Apply delete ingress", func(t *testing.T) {
		client := &MockK8sClient{
			ingresses: map[string]*k8s.IngressInfo{
				"default/my-ingress": {
					ResourceInfo: k8s.ResourceInfo{
						Kind:      "Ingress",
						Namespace: "default",
						Name:      "my-ingress",
					},
				},
			},
		}
		module := NewK8sIngressModule(client)

		decl := &StateDeclaration{
			ID:    "my-ingress",
			State: "absent",
			Parameters: map[string]interface{}{
				"namespace": "default",
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Comment)
		}
		if result.Changes["deleted"] != true {
			t.Error("expected 'deleted' in changes")
		}

		// Verify ingress was deleted
		_, err = client.GetIngress("default", "my-ingress")
		if err == nil {
			t.Error("expected ingress to be deleted")
		}
	})

	t.Run("Apply with default backend", func(t *testing.T) {
		client := &MockK8sClient{}
		module := NewK8sIngressModule(client)

		decl := &StateDeclaration{
			ID:    "default-backend-ingress",
			State: "present",
			Parameters: map[string]interface{}{
				"namespace":     "default",
				"ingress_class": "nginx",
				"default_backend": map[string]interface{}{
					"service": "default-service",
					"port":    80,
				},
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.Success {
			t.Errorf("expected success, got failure: %s", result.Comment)
		}

		ingress, err := client.GetIngress("default", "default-backend-ingress")
		if err != nil {
			t.Fatalf("ingress not created: %v", err)
		}
		if ingress.DefaultBackend == nil {
			t.Error("expected default backend to be set")
		} else {
			if ingress.DefaultBackend.ServiceName != "default-service" {
				t.Errorf("expected service 'default-service', got '%s'", ingress.DefaultBackend.ServiceName)
			}
			if ingress.DefaultBackend.ServicePort != 80 {
				t.Errorf("expected port 80, got %d", ingress.DefaultBackend.ServicePort)
			}
		}
	})

	t.Run("Test matching state", func(t *testing.T) {
		client := &MockK8sClient{
			ingresses: map[string]*k8s.IngressInfo{
				"default/my-ingress": {
					ResourceInfo: k8s.ResourceInfo{
						Kind:      "Ingress",
						Namespace: "default",
						Name:      "my-ingress",
					},
					IngressClassName: "nginx",
				},
			},
		}
		module := NewK8sIngressModule(client)

		decl := &StateDeclaration{
			ID:    "my-ingress",
			State: "present",
			Parameters: map[string]interface{}{
				"namespace":     "default",
				"ingress_class": "nginx",
			},
		}

		matches, err := module.Test(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !matches {
			t.Error("expected Test to return true for matching state")
		}

		// Test non-matching state
		decl.State = "absent"
		matches, err = module.Test(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matches {
			t.Error("expected Test to return false for non-matching state")
		}
	})
}

func TestGetIngressRules(t *testing.T) {
	tests := []struct {
		name          string
		decl          *StateDeclaration
		expectedCount int
	}{
		{
			name: "with rules",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"rules": []interface{}{
						map[string]interface{}{
							"host": "example.com",
							"paths": []interface{}{
								map[string]interface{}{
									"path":      "/",
									"path_type": "Prefix",
									"backend": map[string]interface{}{
										"service": "my-service",
										"port":    80,
									},
								},
							},
						},
					},
				},
			},
			expectedCount: 1,
		},
		{
			name: "no rules",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expectedCount: 0,
		},
		{
			name: "multiple rules",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"rules": []interface{}{
						map[string]interface{}{
							"host": "api.example.com",
							"paths": []interface{}{
								map[string]interface{}{
									"path":      "/v1",
									"path_type": "Prefix",
									"backend": map[string]interface{}{
										"service": "api-v1",
										"port":    8080,
									},
								},
							},
						},
						map[string]interface{}{
							"host": "web.example.com",
							"paths": []interface{}{
								map[string]interface{}{
									"path":      "/",
									"path_type": "Prefix",
									"backend": map[string]interface{}{
										"service": "web",
										"port":    80,
									},
								},
							},
						},
					},
				},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIngressRules(tt.decl)
			if tt.expectedCount == 0 {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != tt.expectedCount {
				t.Errorf("expected %d rules, got %d", tt.expectedCount, len(result))
			}
		})
	}
}

func TestGetIngressTLS(t *testing.T) {
	tests := []struct {
		name          string
		decl          *StateDeclaration
		expectedCount int
	}{
		{
			name: "with TLS",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"tls": []interface{}{
						map[string]interface{}{
							"hosts":       []interface{}{"example.com"},
							"secret_name": "tls-secret",
						},
					},
				},
			},
			expectedCount: 1,
		},
		{
			name: "no TLS",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expectedCount: 0,
		},
		{
			name: "multiple TLS",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"tls": []interface{}{
						map[string]interface{}{
							"hosts":       []interface{}{"api.example.com"},
							"secret_name": "api-tls",
						},
						map[string]interface{}{
							"hosts":       []interface{}{"web.example.com"},
							"secret_name": "web-tls",
						},
					},
				},
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIngressTLS(tt.decl)
			if tt.expectedCount == 0 {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != tt.expectedCount {
				t.Errorf("expected %d TLS configs, got %d", tt.expectedCount, len(result))
			}
		})
	}
}

func TestGetIngressClassName(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected string
	}{
		{
			name: "explicit ingress class",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"ingress_class": "nginx",
				},
			},
			expected: "nginx",
		},
		{
			name: "no ingress class",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: "",
		},
		{
			name: "traefik ingress class",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"ingress_class": "traefik",
				},
			},
			expected: "traefik",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getIngressClassName(tt.decl)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// ========================= STATEFULSET TESTS =========================

func TestK8sStatefulSetModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sStatefulSetModule(client)

	t.Run("Name", func(t *testing.T) {
		if module.Name() != "k8s_statefulset" {
			t.Errorf("expected module name 'k8s_statefulset', got %s", module.Name())
		}
	})

	t.Run("ValidStates", func(t *testing.T) {
		states := module.ValidStates()
		if len(states) != 2 {
			t.Fatalf("expected 2 valid states, got %d", len(states))
		}
		if states[0] != "present" || states[1] != "absent" {
			t.Errorf("unexpected states: %v", states)
		}
	})

	t.Run("Check_absent_statefulset", func(t *testing.T) {
		decl := &StateDeclaration{
			ID:     "my-sts",
			Module: "k8s_statefulset",
			State:  "absent",
		}

		result, err := module.Check(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Present {
			t.Error("expected statefulset to not be present")
		}
		if !result.Matches {
			t.Error("expected state to match (absent)")
		}
	})

	t.Run("Apply_create_statefulset", func(t *testing.T) {
		decl := &StateDeclaration{
			ID:     "test-sts",
			Module: "k8s_statefulset",
			State:  "present",
			Parameters: map[string]interface{}{
				"namespace":             "default",
				"image":                 "redis:7",
				"service_name":          "redis-headless",
				"replicas":              3,
				"pod_management_policy": "Parallel",
				"update_strategy":       "RollingUpdate",
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got: %s", result.Comment)
		}
		if result.Changes["created"] != true {
			t.Error("expected created=true in changes")
		}
		if result.Changes["service_name"] != "redis-headless" {
			t.Errorf("expected service_name='redis-headless', got %v", result.Changes["service_name"])
		}
	})

	t.Run("Check_existing_statefulset_matches", func(t *testing.T) {
		decl := &StateDeclaration{
			ID:     "test-sts",
			Module: "k8s_statefulset",
			State:  "present",
			Parameters: map[string]interface{}{
				"namespace":             "default",
				"replicas":              3,
				"pod_management_policy": "Parallel",
				"update_strategy":       "RollingUpdate",
			},
		}

		result, err := module.Check(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Present {
			t.Error("expected statefulset to be present")
		}
		if !result.Matches {
			t.Errorf("expected state to match, diff: %v", result.Diff)
		}
	})

	t.Run("Apply_update_replicas", func(t *testing.T) {
		decl := &StateDeclaration{
			ID:     "test-sts",
			Module: "k8s_statefulset",
			State:  "present",
			Parameters: map[string]interface{}{
				"namespace": "default",
				"replicas":  5,
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got: %s", result.Comment)
		}
		if result.Changes["updated"] != true {
			t.Error("expected updated=true in changes")
		}
		if result.Changes["replicas_updated"] == nil {
			t.Error("expected replicas_updated in changes")
		}
	})

	t.Run("Apply_delete_statefulset", func(t *testing.T) {
		decl := &StateDeclaration{
			ID:     "test-sts",
			Module: "k8s_statefulset",
			State:  "absent",
			Parameters: map[string]interface{}{
				"namespace": "default",
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Errorf("expected success, got: %s", result.Comment)
		}
		if result.Changes["deleted"] != true {
			t.Error("expected deleted=true in changes")
		}
	})

	t.Run("Apply_create_without_image", func(t *testing.T) {
		decl := &StateDeclaration{
			ID:     "no-image-sts",
			Module: "k8s_statefulset",
			State:  "present",
			Parameters: map[string]interface{}{
				"namespace":    "default",
				"service_name": "test-headless",
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err == nil {
			t.Error("expected error when image is not specified")
		}
		if result.Success {
			t.Error("expected failure when image is not specified")
		}
	})

	t.Run("Apply_create_without_service_name", func(t *testing.T) {
		decl := &StateDeclaration{
			ID:     "no-svc-sts",
			Module: "k8s_statefulset",
			State:  "present",
			Parameters: map[string]interface{}{
				"namespace": "default",
				"image":     "redis:7",
			},
		}

		result, err := module.Apply(context.Background(), decl)
		if err == nil {
			t.Error("expected error when service_name is not specified")
		}
		if result.Success {
			t.Error("expected failure when service_name is not specified")
		}
	})
}

func TestGetVolumeClaimTemplates(t *testing.T) {
	tests := []struct {
		name          string
		decl          *StateDeclaration
		expectedCount int
		expectedName  string
	}{
		{
			name: "no volume claim templates",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expectedCount: 0,
		},
		{
			name: "single volume claim template",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"volume_claim_templates": []interface{}{
						map[string]interface{}{
							"name":          "data",
							"storage_class": "standard",
							"storage_size":  "10Gi",
							"access_modes":  []interface{}{"ReadWriteOnce"},
						},
					},
				},
			},
			expectedCount: 1,
			expectedName:  "data",
		},
		{
			name: "multiple volume claim templates",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"volume_claim_templates": []interface{}{
						map[string]interface{}{
							"name":          "data",
							"storage_class": "fast",
							"storage_size":  "100Gi",
							"access_modes":  []interface{}{"ReadWriteOnce"},
						},
						map[string]interface{}{
							"name":          "logs",
							"storage_class": "standard",
							"storage_size":  "10Gi",
							"access_mode":   "ReadWriteOnce",
						},
					},
				},
			},
			expectedCount: 2,
		},
		{
			name: "invalid volume claim template format",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"volume_claim_templates": "invalid",
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getVolumeClaimTemplates(tt.decl)
			if tt.expectedCount == 0 {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != tt.expectedCount {
				t.Errorf("expected %d templates, got %d", tt.expectedCount, len(result))
			}
			if tt.expectedName != "" && len(result) > 0 {
				if result[0].Name != tt.expectedName {
					t.Errorf("expected name '%s', got '%s'", tt.expectedName, result[0].Name)
				}
			}
		})
	}
}

// ==================== DaemonSet Module Tests ====================

func TestK8sDaemonSetModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDaemonSetModule(client)

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Name",
			testFunc: func(t *testing.T) {
				if module.Name() != "k8s_daemonset" {
					t.Errorf("expected name 'k8s_daemonset', got '%s'", module.Name())
				}
			},
		},
		{
			name: "ValidStates",
			testFunc: func(t *testing.T) {
				states := module.ValidStates()
				if len(states) != 2 {
					t.Errorf("expected 2 valid states, got %d", len(states))
				}
			},
		},
		{
			name: "Check_absent_daemonset",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "my-daemonset",
					Module: "k8s_daemonset",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Present {
					t.Error("expected daemonset to be absent")
				}
				if result.Matches {
					t.Error("expected state not to match")
				}
			},
		},
		{
			name: "Apply_create_daemonset",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "test-daemon",
					Module: "k8s_daemonset",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
						"image":     "nginx:latest",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got failure: %s", result.Comment)
				}
				if _, ok := result.Changes["created"]; !ok {
					t.Error("expected 'created' in changes")
				}
			},
		},
		{
			name: "Apply_create_without_image",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "no-image-daemon",
					Module: "k8s_daemonset",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				_, err := module.Apply(context.Background(), decl)
				if err == nil {
					t.Error("expected error for missing image")
				}
			},
		},
		{
			name: "Apply_delete_daemonset",
			testFunc: func(t *testing.T) {
				// First create a daemonset
				client.daemonsets = map[string]*k8s.DaemonSetInfo{
					"default/to-delete": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "to-delete",
							Namespace: "default",
							Status:    k8s.StatusRunning,
						},
					},
				}
				decl := &StateDeclaration{
					ID:     "to-delete",
					Module: "k8s_daemonset",
					State:  "absent",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got: %s", result.Comment)
				}
				if _, ok := result.Changes["deleted"]; !ok {
					t.Error("expected 'deleted' in changes")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// ==================== Job Module Tests ====================

func TestK8sJobModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sJobModule(client)

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Name",
			testFunc: func(t *testing.T) {
				if module.Name() != "k8s_job" {
					t.Errorf("expected name 'k8s_job', got '%s'", module.Name())
				}
			},
		},
		{
			name: "ValidStates",
			testFunc: func(t *testing.T) {
				states := module.ValidStates()
				if len(states) != 3 {
					t.Errorf("expected 3 valid states (present, absent, completed), got %d", len(states))
				}
			},
		},
		{
			name: "Check_absent_job",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "my-job",
					Module: "k8s_job",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Present {
					t.Error("expected job to be absent")
				}
			},
		},
		{
			name: "Apply_create_job",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "test-job",
					Module: "k8s_job",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
						"image":     "busybox:latest",
						"command":   []interface{}{"echo", "hello"},
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got failure: %s", result.Comment)
				}
			},
		},
		{
			name: "Apply_create_without_image",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "no-image-job",
					Module: "k8s_job",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				_, err := module.Apply(context.Background(), decl)
				if err == nil {
					t.Error("expected error for missing image")
				}
			},
		},
		{
			name: "Check_completed_job",
			testFunc: func(t *testing.T) {
				completionTime := time.Now()
				client.jobs = map[string]*k8s.JobInfo{
					"default/completed-job": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "completed-job",
							Namespace: "default",
							Status:    k8s.StatusSucceeded,
						},
						Succeeded:      1,
						Completions:    1,
						CompletionTime: &completionTime,
					},
				}
				decl := &StateDeclaration{
					ID:     "completed-job",
					Module: "k8s_job",
					State:  "completed",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Matches {
					t.Error("expected completed job to match 'completed' state")
				}
			},
		},
		{
			name: "Apply_delete_job",
			testFunc: func(t *testing.T) {
				client.jobs = map[string]*k8s.JobInfo{
					"default/to-delete-job": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "to-delete-job",
							Namespace: "default",
						},
					},
				}
				decl := &StateDeclaration{
					ID:     "to-delete-job",
					Module: "k8s_job",
					State:  "absent",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got: %s", result.Comment)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// ==================== CronJob Module Tests ====================

func TestK8sCronJobModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sCronJobModule(client)

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Name",
			testFunc: func(t *testing.T) {
				if module.Name() != "k8s_cronjob" {
					t.Errorf("expected name 'k8s_cronjob', got '%s'", module.Name())
				}
			},
		},
		{
			name: "ValidStates",
			testFunc: func(t *testing.T) {
				states := module.ValidStates()
				if len(states) != 3 {
					t.Errorf("expected 3 valid states (present, absent, suspended), got %d", len(states))
				}
			},
		},
		{
			name: "Check_absent_cronjob",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "my-cronjob",
					Module: "k8s_cronjob",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Present {
					t.Error("expected cronjob to be absent")
				}
			},
		},
		{
			name: "Apply_create_cronjob",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "test-cronjob",
					Module: "k8s_cronjob",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
						"image":     "busybox:latest",
						"schedule":  "*/5 * * * *",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got failure: %s", result.Comment)
				}
			},
		},
		{
			name: "Apply_create_without_schedule",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "no-schedule-cronjob",
					Module: "k8s_cronjob",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
						"image":     "busybox:latest",
					},
				}
				_, err := module.Apply(context.Background(), decl)
				if err == nil {
					t.Error("expected error for missing schedule")
				}
			},
		},
		{
			name: "Check_suspended_cronjob",
			testFunc: func(t *testing.T) {
				client.cronjobs = map[string]*k8s.CronJobInfo{
					"default/suspended-cron": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "suspended-cron",
							Namespace: "default",
						},
						Schedule: "0 * * * *",
						Suspend:  true,
					},
				}
				decl := &StateDeclaration{
					ID:     "suspended-cron",
					Module: "k8s_cronjob",
					State:  "suspended",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Matches {
					t.Error("expected suspended cronjob to match 'suspended' state")
				}
			},
		},
		{
			name: "Apply_suspend_cronjob",
			testFunc: func(t *testing.T) {
				client.cronjobs = map[string]*k8s.CronJobInfo{
					"default/running-cron": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "running-cron",
							Namespace: "default",
						},
						Schedule: "0 * * * *",
						Suspend:  false,
					},
				}
				decl := &StateDeclaration{
					ID:     "running-cron",
					Module: "k8s_cronjob",
					State:  "suspended",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got: %s", result.Comment)
				}
			},
		},
		{
			name: "Apply_delete_cronjob",
			testFunc: func(t *testing.T) {
				client.cronjobs = map[string]*k8s.CronJobInfo{
					"default/to-delete-cron": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "to-delete-cron",
							Namespace: "default",
						},
					},
				}
				decl := &StateDeclaration{
					ID:     "to-delete-cron",
					Module: "k8s_cronjob",
					State:  "absent",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got: %s", result.Comment)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// ==================== PVC Module Tests ====================

func TestK8sPVCModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sPVCModule(client)

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Name",
			testFunc: func(t *testing.T) {
				if module.Name() != "k8s_pvc" {
					t.Errorf("expected name 'k8s_pvc', got '%s'", module.Name())
				}
			},
		},
		{
			name: "ValidStates",
			testFunc: func(t *testing.T) {
				states := module.ValidStates()
				if len(states) != 3 {
					t.Errorf("expected 3 valid states (present, absent, bound), got %d", len(states))
				}
			},
		},
		{
			name: "Check_absent_pvc",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "my-pvc",
					Module: "k8s_pvc",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Present {
					t.Error("expected pvc to be absent")
				}
			},
		},
		{
			name: "Apply_create_pvc",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "test-pvc",
					Module: "k8s_pvc",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace":          "default",
						"storage_size":       "10Gi",
						"storage_class_name": "standard",
						"access_modes":       []interface{}{"ReadWriteOnce"},
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got failure: %s", result.Comment)
				}
			},
		},
		{
			name: "Apply_create_without_storage_size",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "no-size-pvc",
					Module: "k8s_pvc",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				_, err := module.Apply(context.Background(), decl)
				if err == nil {
					t.Error("expected error for missing storage_size")
				}
			},
		},
		{
			name: "Check_bound_pvc",
			testFunc: func(t *testing.T) {
				client.pvcs = map[string]*k8s.PVCInfo{
					"default/bound-pvc": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "bound-pvc",
							Namespace: "default",
							Status:    k8s.StatusRunning,
						},
						Phase:            "Bound",
						RequestedStorage: "10Gi",
						AccessModes:      []string{"ReadWriteOnce"},
					},
				}
				decl := &StateDeclaration{
					ID:     "bound-pvc",
					Module: "k8s_pvc",
					State:  "bound",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Matches {
					t.Error("expected bound PVC to match 'bound' state")
				}
			},
		},
		{
			name: "Apply_delete_pvc",
			testFunc: func(t *testing.T) {
				client.pvcs = map[string]*k8s.PVCInfo{
					"default/to-delete-pvc": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "to-delete-pvc",
							Namespace: "default",
						},
					},
				}
				decl := &StateDeclaration{
					ID:     "to-delete-pvc",
					Module: "k8s_pvc",
					State:  "absent",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got: %s", result.Comment)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestGetAccessModes(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected []string
	}{
		{
			name: "array of access modes",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"access_modes": []interface{}{"ReadWriteOnce", "ReadOnlyMany"},
				},
			},
			expected: []string{"ReadWriteOnce", "ReadOnlyMany"},
		},
		{
			name: "single access_mode string",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"access_mode": "ReadWriteOnce",
				},
			},
			expected: []string{"ReadWriteOnce"},
		},
		{
			name: "access_modes as string",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"access_modes": "ReadWriteMany",
				},
			},
			expected: []string{"ReadWriteMany"},
		},
		{
			name:     "no access modes",
			decl:     &StateDeclaration{Parameters: map[string]interface{}{}},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getAccessModes(tt.decl)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d modes, got %d", len(tt.expected), len(result))
			}
		})
	}
}

func TestCompareAccessModes(t *testing.T) {
	tests := []struct {
		name     string
		current  []string
		desired  []string
		expected bool
	}{
		{
			name:     "matching modes",
			current:  []string{"ReadWriteOnce"},
			desired:  []string{"ReadWriteOnce"},
			expected: true,
		},
		{
			name:     "different order same modes",
			current:  []string{"ReadOnlyMany", "ReadWriteOnce"},
			desired:  []string{"ReadWriteOnce", "ReadOnlyMany"},
			expected: true,
		},
		{
			name:     "different modes",
			current:  []string{"ReadWriteOnce"},
			desired:  []string{"ReadWriteMany"},
			expected: false,
		},
		{
			name:     "different lengths",
			current:  []string{"ReadWriteOnce"},
			desired:  []string{"ReadWriteOnce", "ReadOnlyMany"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareAccessModes(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// ==================== HPA Module Tests ====================

func TestK8sHPAModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sHPAModule(client)

	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Name",
			testFunc: func(t *testing.T) {
				if module.Name() != "k8s_hpa" {
					t.Errorf("expected name 'k8s_hpa', got '%s'", module.Name())
				}
			},
		},
		{
			name: "ValidStates",
			testFunc: func(t *testing.T) {
				states := module.ValidStates()
				if len(states) != 2 {
					t.Errorf("expected 2 valid states (present, absent), got %d", len(states))
				}
			},
		},
		{
			name: "Check_absent_hpa",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "my-hpa",
					Module: "k8s_hpa",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Present {
					t.Error("expected hpa to be absent")
				}
			},
		},
		{
			name: "Apply_create_hpa",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "test-hpa",
					Module: "k8s_hpa",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace":              "default",
						"target_kind":            "Deployment",
						"target_name":            "my-deployment",
						"min_replicas":           2,
						"max_replicas":           10,
						"target_cpu_utilization": 80,
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got failure: %s", result.Comment)
				}
			},
		},
		{
			name: "Apply_create_without_target_name",
			testFunc: func(t *testing.T) {
				decl := &StateDeclaration{
					ID:     "no-target-hpa",
					Module: "k8s_hpa",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace":    "default",
						"min_replicas": 1,
						"max_replicas": 5,
					},
				}
				_, err := module.Apply(context.Background(), decl)
				if err == nil {
					t.Error("expected error for missing target_name")
				}
			},
		},
		{
			name: "Check_existing_hpa",
			testFunc: func(t *testing.T) {
				targetCPU := int32(80)
				client.hpas = map[string]*k8s.HPAInfo{
					"default/existing-hpa": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "existing-hpa",
							Namespace: "default",
							Status:    k8s.StatusRunning,
						},
						MinReplicas:          2,
						MaxReplicas:          10,
						CurrentReplicas:      3,
						DesiredReplicas:      3,
						TargetKind:           "Deployment",
						TargetName:           "my-app",
						TargetCPUUtilization: &targetCPU,
					},
				}
				decl := &StateDeclaration{
					ID:     "existing-hpa",
					Module: "k8s_hpa",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace":              "default",
						"target_kind":            "Deployment",
						"target_name":            "my-app",
						"min_replicas":           2,
						"max_replicas":           10,
						"target_cpu_utilization": 80,
					},
				}
				result, err := module.Check(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Matches {
					t.Errorf("expected HPA to match, diff: %v", result.Diff)
				}
			},
		},
		{
			name: "Apply_update_hpa",
			testFunc: func(t *testing.T) {
				targetCPU := int32(70)
				client.hpas = map[string]*k8s.HPAInfo{
					"default/update-hpa": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "update-hpa",
							Namespace: "default",
						},
						MinReplicas:          1,
						MaxReplicas:          5,
						TargetKind:           "Deployment",
						TargetName:           "my-app",
						TargetCPUUtilization: &targetCPU,
					},
				}
				decl := &StateDeclaration{
					ID:     "update-hpa",
					Module: "k8s_hpa",
					State:  "present",
					Parameters: map[string]interface{}{
						"namespace":              "default",
						"target_kind":            "Deployment",
						"target_name":            "my-app",
						"min_replicas":           2,
						"max_replicas":           10,
						"target_cpu_utilization": 80,
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got: %s", result.Comment)
				}
				if _, ok := result.Changes["updated"]; !ok {
					t.Error("expected 'updated' in changes")
				}
			},
		},
		{
			name: "Apply_delete_hpa",
			testFunc: func(t *testing.T) {
				client.hpas = map[string]*k8s.HPAInfo{
					"default/to-delete-hpa": {
						ResourceInfo: k8s.ResourceInfo{
							Name:      "to-delete-hpa",
							Namespace: "default",
						},
					},
				}
				decl := &StateDeclaration{
					ID:     "to-delete-hpa",
					Module: "k8s_hpa",
					State:  "absent",
					Parameters: map[string]interface{}{
						"namespace": "default",
					},
				}
				result, err := module.Apply(context.Background(), decl)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !result.Success {
					t.Errorf("expected success, got: %s", result.Comment)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

func TestGetInt32PointerParameter(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		param    string
		expected *int32
	}{
		{
			name: "int value",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"value": 80,
				},
			},
			param:    "value",
			expected: func() *int32 { v := int32(80); return &v }(),
		},
		{
			name: "int32 value",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"value": int32(50),
				},
			},
			param:    "value",
			expected: func() *int32 { v := int32(50); return &v }(),
		},
		{
			name: "float64 value",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"value": float64(75),
				},
			},
			param:    "value",
			expected: func() *int32 { v := int32(75); return &v }(),
		},
		{
			name:     "missing parameter",
			decl:     &StateDeclaration{Parameters: map[string]interface{}{}},
			param:    "value",
			expected: nil,
		},
		{
			name: "invalid type",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"value": "not a number",
				},
			},
			param:    "value",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getInt32PointerParameter(tt.decl, tt.param)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", *result)
				}
				return
			}
			if result == nil {
				t.Errorf("expected %d, got nil", *tt.expected)
				return
			}
			if *result != *tt.expected {
				t.Errorf("expected %d, got %d", *tt.expected, *result)
			}
		})
	}
}
