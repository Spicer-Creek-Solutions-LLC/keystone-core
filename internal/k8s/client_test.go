package k8s

import (
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func TestPodStatusToResourceStatus(t *testing.T) {
	tests := []struct {
		name     string
		phase    corev1.PodPhase
		expected ResourceStatus
	}{
		{"Running", corev1.PodRunning, StatusRunning},
		{"Pending", corev1.PodPending, StatusPending},
		{"Succeeded", corev1.PodSucceeded, StatusSucceeded},
		{"Failed", corev1.PodFailed, StatusFailed},
		{"Unknown", corev1.PodUnknown, StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := podStatusToResourceStatus(tt.phase)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestNamespacePhaseToStatus(t *testing.T) {
	tests := []struct {
		name     string
		phase    corev1.NamespacePhase
		expected ResourceStatus
	}{
		{"Active", corev1.NamespaceActive, StatusRunning},
		{"Terminating", corev1.NamespaceTerminating, StatusPending},
		{"Unknown", corev1.NamespacePhase("Unknown"), StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := namespacePhaseToStatus(tt.phase)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestResourceQuantityFromString(t *testing.T) {
	tests := []struct {
		name    string
		size    string
		wantErr bool
	}{
		{"valid Gi", "10Gi", false},
		{"valid Mi", "500Mi", false},
		{"valid Ki", "1024Ki", false},
		{"valid bytes", "1000000", false},
		{"valid G", "1G", false},
		{"invalid", "invalid", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resourceQuantityFromString(tt.size)
			if (err != nil) != tt.wantErr {
				t.Errorf("resourceQuantityFromString(%s) error = %v, wantErr %v", tt.size, err, tt.wantErr)
			}
		})
	}
}

func TestGetTotalRestartCount(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{RestartCount: 5},
				{RestartCount: 3},
				{RestartCount: 2},
			},
		},
	}

	expected := int32(10)
	result := getTotalRestartCount(pod)

	if result != expected {
		t.Errorf("expected %d, got %d", expected, result)
	}
}

func TestClusterConfig(t *testing.T) {
	config := ClusterConfig{
		Name:       "test-cluster",
		Kubeconfig: "/path/to/kubeconfig",
		Context:    "test-context",
		Namespace:  "default",
		Timeout:    30 * time.Second,
	}

	if config.Name != "test-cluster" {
		t.Errorf("expected name 'test-cluster', got '%s'", config.Name)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", config.Timeout)
	}
}

func TestPodSelector(t *testing.T) {
	selector := PodSelector{
		Namespace:     "default",
		LabelSelector: "app=nginx",
		FieldSelector: "status.phase=Running",
		Container:     "nginx",
		MaxPods:       10,
	}

	if selector.LabelSelector != "app=nginx" {
		t.Errorf("expected label selector 'app=nginx', got '%s'", selector.LabelSelector)
	}

	if selector.MaxPods != 10 {
		t.Errorf("expected max pods 10, got %d", selector.MaxPods)
	}
}

func TestResourceStatus(t *testing.T) {
	statuses := []ResourceStatus{
		StatusRunning,
		StatusPending,
		StatusSucceeded,
		StatusFailed,
		StatusUnknown,
	}

	expected := []string{
		"Running",
		"Pending",
		"Succeeded",
		"Failed",
		"Unknown",
	}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("expected status '%s', got '%s'", expected[i], status)
		}
	}
}

func TestExecutionMode(t *testing.T) {
	modes := []ExecutionMode{
		ExecModePod,
		ExecModeJob,
		ExecModeNode,
	}

	expected := []string{
		"pod",
		"job",
		"node",
	}

	for i, mode := range modes {
		if string(mode) != expected[i] {
			t.Errorf("expected mode '%s', got '%s'", expected[i], mode)
		}
	}
}

func TestOperatorConfig(t *testing.T) {
	config := OperatorConfig{
		Namespace:               "kscore-system",
		LeaderElection:          true,
		LeaderElectionID:        "kscore-operator",
		MetricsAddr:             ":8080",
		ProbeAddr:               ":8081",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 3,
	}

	if config.Namespace != "kscore-system" {
		t.Errorf("expected namespace 'kscore-system', got '%s'", config.Namespace)
	}

	if !config.LeaderElection {
		t.Error("expected leader election enabled")
	}

	if config.MaxConcurrentReconciles != 3 {
		t.Errorf("expected 3 concurrent reconciles, got %d", config.MaxConcurrentReconciles)
	}
}

func TestDeploymentInfo(t *testing.T) {
	info := DeploymentInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "nginx",
			Labels:    map[string]string{"app": "nginx"},
			Status:    StatusRunning,
		},
		Replicas:          3,
		AvailableReplicas: 3,
		ReadyReplicas:     3,
		UpdatedReplicas:   3,
	}

	if info.Kind != "Deployment" {
		t.Errorf("expected kind 'Deployment', got '%s'", info.Kind)
	}

	if info.Replicas != 3 {
		t.Errorf("expected 3 replicas, got %d", info.Replicas)
	}

	if info.AvailableReplicas != info.Replicas {
		t.Error("expected all replicas to be available")
	}
}

func TestStatefulSetInfo(t *testing.T) {
	info := StatefulSetInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "StatefulSet",
			Namespace: "default",
			Name:      "redis",
			Status:    StatusRunning,
		},
		Replicas:            3,
		ReadyReplicas:       3,
		CurrentReplicas:     3,
		UpdatedReplicas:     3,
		CurrentRevision:     "redis-7f9b8c6d5",
		UpdateRevision:      "redis-7f9b8c6d5",
		ServiceName:         "redis-headless",
		PodManagementPolicy: "OrderedReady",
		UpdateStrategy:      "RollingUpdate",
	}

	if info.Kind != "StatefulSet" {
		t.Errorf("expected kind 'StatefulSet', got '%s'", info.Kind)
	}

	if info.ServiceName != "redis-headless" {
		t.Errorf("expected service name 'redis-headless', got '%s'", info.ServiceName)
	}

	if info.PodManagementPolicy != "OrderedReady" {
		t.Errorf("expected pod management policy 'OrderedReady', got '%s'", info.PodManagementPolicy)
	}
}

func TestDaemonSetInfo(t *testing.T) {
	info := DaemonSetInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "DaemonSet",
			Namespace: "kube-system",
			Name:      "fluentd",
			Status:    StatusRunning,
		},
		DesiredNumberScheduled: 5,
		CurrentNumberScheduled: 5,
		NumberReady:            5,
		NumberAvailable:        5,
		NumberMisscheduled:     0,
		UpdatedNumberScheduled: 5,
		UpdateStrategy:         "RollingUpdate",
	}

	if info.Kind != "DaemonSet" {
		t.Errorf("expected kind 'DaemonSet', got '%s'", info.Kind)
	}

	if info.DesiredNumberScheduled != 5 {
		t.Errorf("expected 5 desired scheduled, got %d", info.DesiredNumberScheduled)
	}

	if info.NumberMisscheduled != 0 {
		t.Errorf("expected 0 misscheduled, got %d", info.NumberMisscheduled)
	}
}

func TestJobInfo(t *testing.T) {
	startTime := time.Now().Add(-5 * time.Minute)
	completionTime := time.Now()

	info := JobInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "Job",
			Namespace: "default",
			Name:      "backup-job",
			Status:    StatusSucceeded,
		},
		Active:         0,
		Succeeded:      1,
		Failed:         0,
		Completions:    1,
		Parallelism:    1,
		BackoffLimit:   3,
		StartTime:      &startTime,
		CompletionTime: &completionTime,
	}

	if info.Kind != "Job" {
		t.Errorf("expected kind 'Job', got '%s'", info.Kind)
	}

	if info.Status != StatusSucceeded {
		t.Errorf("expected status 'Succeeded', got '%s'", info.Status)
	}

	if info.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", info.Succeeded)
	}

	if info.StartTime == nil {
		t.Error("expected start time to be set")
	}
}

func TestCronJobInfo(t *testing.T) {
	lastScheduleTime := time.Now().Add(-1 * time.Hour)

	info := CronJobInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "CronJob",
			Namespace: "default",
			Name:      "hourly-backup",
			Status:    StatusRunning,
		},
		Schedule:           "0 * * * *",
		Suspend:            false,
		ConcurrencyPolicy:  "Forbid",
		ActiveJobs:         0,
		LastScheduleTime:   &lastScheduleTime,
		LastSuccessfulTime: &lastScheduleTime,
	}

	if info.Schedule != "0 * * * *" {
		t.Errorf("expected schedule '0 * * * *', got '%s'", info.Schedule)
	}

	if info.Suspend {
		t.Error("expected cronjob not to be suspended")
	}

	if info.ConcurrencyPolicy != "Forbid" {
		t.Errorf("expected concurrency policy 'Forbid', got '%s'", info.ConcurrencyPolicy)
	}
}

func TestPodExecOptions(t *testing.T) {
	opts := PodExecOptions{
		Namespace: "default",
		PodName:   "nginx-abc123",
		Container: "nginx",
		Command:   []string{"sh", "-c", "echo hello"},
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
		Timeout:   30 * time.Second,
	}

	if opts.Namespace != "default" {
		t.Errorf("expected namespace 'default', got '%s'", opts.Namespace)
	}

	if len(opts.Command) != 3 {
		t.Errorf("expected 3 command args, got %d", len(opts.Command))
	}

	if !opts.Stdout || !opts.Stderr {
		t.Error("expected stdout and stderr to be enabled")
	}

	if opts.TTY {
		t.Error("expected TTY to be disabled")
	}
}

func TestPodExecResult(t *testing.T) {
	result := PodExecResult{
		ExitCode: 0,
		Stdout:   "hello world\n",
		Stderr:   "",
		Error:    nil,
		Duration: 150 * time.Millisecond,
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if result.Stdout != "hello world\n" {
		t.Errorf("expected stdout 'hello world\\n', got '%s'", result.Stdout)
	}

	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}

	if result.Duration != 150*time.Millisecond {
		t.Errorf("expected duration 150ms, got %v", result.Duration)
	}
}

func TestResourceInfo(t *testing.T) {
	now := time.Now()
	info := ResourceInfo{
		Kind:              "Pod",
		Namespace:         "default",
		Name:              "nginx-12345",
		Labels:            map[string]string{"app": "nginx", "tier": "frontend"},
		Annotations:       map[string]string{"description": "Web server"},
		Status:            StatusRunning,
		CreationTimestamp: now,
		Metadata:          map[string]interface{}{"version": "1.19"},
	}

	if info.Kind != "Pod" {
		t.Errorf("expected kind 'Pod', got '%s'", info.Kind)
	}

	if len(info.Labels) != 2 {
		t.Errorf("expected 2 labels, got %d", len(info.Labels))
	}

	if info.Labels["app"] != "nginx" {
		t.Errorf("expected label 'app=nginx', got '%s'", info.Labels["app"])
	}

	if info.Status != StatusRunning {
		t.Errorf("expected status 'Running', got '%s'", info.Status)
	}
}

func TestServiceInfo(t *testing.T) {
	info := ServiceInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "Service",
			Namespace: "default",
			Name:      "nginx-svc",
			Status:    StatusRunning,
		},
		Type:        "ClusterIP",
		ClusterIP:   "10.96.0.100",
		ExternalIPs: []string{},
		Ports: []ServicePort{
			{Name: "http", Protocol: "TCP", Port: 80, TargetPort: 8080},
			{Name: "https", Protocol: "TCP", Port: 443, TargetPort: 8443},
		},
	}

	if info.Type != "ClusterIP" {
		t.Errorf("expected service type 'ClusterIP', got '%s'", info.Type)
	}

	if info.ClusterIP != "10.96.0.100" {
		t.Errorf("expected cluster IP '10.96.0.100', got '%s'", info.ClusterIP)
	}

	if len(info.Ports) != 2 {
		t.Errorf("expected 2 ports, got %d", len(info.Ports))
	}
}

func TestServicePort(t *testing.T) {
	port := ServicePort{
		Name:       "http",
		Protocol:   "TCP",
		Port:       80,
		TargetPort: 8080,
		NodePort:   30080,
	}

	if port.Name != "http" {
		t.Errorf("expected port name 'http', got '%s'", port.Name)
	}

	if port.Protocol != "TCP" {
		t.Errorf("expected protocol 'TCP', got '%s'", port.Protocol)
	}

	if port.Port != 80 {
		t.Errorf("expected port 80, got %d", port.Port)
	}

	if port.TargetPort != 8080 {
		t.Errorf("expected target port 8080, got %d", port.TargetPort)
	}

	if port.NodePort != 30080 {
		t.Errorf("expected node port 30080, got %d", port.NodePort)
	}
}

func TestConfigMapInfo(t *testing.T) {
	info := ConfigMapInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "ConfigMap",
			Namespace: "default",
			Name:      "app-config",
			Status:    StatusRunning,
		},
		Data: map[string]string{
			"config.yaml":  "key: value",
			"logging.yaml": "level: debug",
		},
		BinaryData: map[string][]byte{},
	}

	if len(info.Data) != 2 {
		t.Errorf("expected 2 data entries, got %d", len(info.Data))
	}

	if info.Data["config.yaml"] != "key: value" {
		t.Errorf("expected config.yaml value, got '%s'", info.Data["config.yaml"])
	}
}

func TestSecretInfo(t *testing.T) {
	info := SecretInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "Secret",
			Namespace: "default",
			Name:      "db-credentials",
			Status:    StatusRunning,
		},
		Type: "Opaque",
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret123"),
		},
	}

	if info.Type != "Opaque" {
		t.Errorf("expected secret type 'Opaque', got '%s'", info.Type)
	}

	if len(info.Data) != 2 {
		t.Errorf("expected 2 data entries, got %d", len(info.Data))
	}

	if string(info.Data["username"]) != "admin" {
		t.Errorf("expected username 'admin', got '%s'", string(info.Data["username"]))
	}
}

func TestIngressInfo(t *testing.T) {
	info := IngressInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "Ingress",
			Namespace: "default",
			Name:      "app-ingress",
			Status:    StatusRunning,
		},
		IngressClassName: "nginx",
		Rules: []IngressRule{
			{Host: "app.example.com"},
			{Host: "api.example.com"},
		},
		TLS: []IngressTLS{
			{Hosts: []string{"app.example.com"}, SecretName: "app-tls"},
		},
		LoadBalancerIngress: []string{"192.168.1.100"},
	}

	if info.IngressClassName != "nginx" {
		t.Errorf("expected ingress class 'nginx', got '%s'", info.IngressClassName)
	}

	if len(info.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(info.Rules))
	}

	if len(info.TLS) != 1 {
		t.Errorf("expected 1 TLS config, got %d", len(info.TLS))
	}
}

func TestIngressRule(t *testing.T) {
	rule := IngressRule{
		Host: "app.example.com",
		Paths: []IngressPath{
			{Path: "/", PathType: "Prefix", Backend: IngressBackend{ServiceName: "app-svc", ServicePort: 80}},
			{Path: "/api", PathType: "Prefix", Backend: IngressBackend{ServiceName: "api-svc", ServicePort: 8080}},
		},
	}

	if rule.Host != "app.example.com" {
		t.Errorf("expected host 'app.example.com', got '%s'", rule.Host)
	}

	if len(rule.Paths) != 2 {
		t.Errorf("expected 2 paths, got %d", len(rule.Paths))
	}

	if rule.Paths[0].Path != "/" {
		t.Errorf("expected path '/', got '%s'", rule.Paths[0].Path)
	}
}

func TestIngressTLS(t *testing.T) {
	tls := IngressTLS{
		Hosts:      []string{"app.example.com", "api.example.com"},
		SecretName: "app-tls-secret",
	}

	if len(tls.Hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(tls.Hosts))
	}

	if tls.SecretName != "app-tls-secret" {
		t.Errorf("expected secret name 'app-tls-secret', got '%s'", tls.SecretName)
	}
}

func TestIngressBackend(t *testing.T) {
	backend := IngressBackend{
		ServiceName: "backend-svc",
		ServicePort: 8080,
	}

	if backend.ServiceName != "backend-svc" {
		t.Errorf("expected service name 'backend-svc', got '%s'", backend.ServiceName)
	}

	if backend.ServicePort != 8080 {
		t.Errorf("expected service port 8080, got %d", backend.ServicePort)
	}
}

func TestPVCInfo(t *testing.T) {
	info := PVCInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "PersistentVolumeClaim",
			Namespace: "default",
			Name:      "data-pvc",
			Status:    StatusRunning,
		},
		StorageClassName: "standard",
		AccessModes:      []string{"ReadWriteOnce"},
		RequestedStorage: "10Gi",
		AllocatedStorage: "10Gi",
		VolumeName:       "pv-data-001",
		Phase:            "Bound",
	}

	if info.StorageClassName != "standard" {
		t.Errorf("expected storage class 'standard', got '%s'", info.StorageClassName)
	}

	if info.Phase != "Bound" {
		t.Errorf("expected phase 'Bound', got '%s'", info.Phase)
	}

	if info.RequestedStorage != "10Gi" {
		t.Errorf("expected requested storage '10Gi', got '%s'", info.RequestedStorage)
	}

	if info.VolumeName != "pv-data-001" {
		t.Errorf("expected volume name 'pv-data-001', got '%s'", info.VolumeName)
	}
}

func TestHPAInfo(t *testing.T) {
	cpuUtil := int32(50)
	info := HPAInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "HorizontalPodAutoscaler",
			Namespace: "default",
			Name:      "nginx-hpa",
			Status:    StatusRunning,
		},
		MinReplicas:           2,
		MaxReplicas:           10,
		CurrentReplicas:       3,
		DesiredReplicas:       3,
		TargetKind:            "Deployment",
		TargetName:            "nginx",
		CurrentCPUUtilization: &cpuUtil,
	}

	if info.MinReplicas != 2 {
		t.Errorf("expected min replicas 2, got %d", info.MinReplicas)
	}

	if info.MaxReplicas != 10 {
		t.Errorf("expected max replicas 10, got %d", info.MaxReplicas)
	}

	if info.TargetKind != "Deployment" {
		t.Errorf("expected target kind 'Deployment', got '%s'", info.TargetKind)
	}

	if info.TargetName != "nginx" {
		t.Errorf("expected target name 'nginx', got '%s'", info.TargetName)
	}

	if info.CurrentCPUUtilization == nil || *info.CurrentCPUUtilization != 50 {
		t.Error("expected current CPU utilization 50")
	}
}

func TestNamespaceInfo(t *testing.T) {
	info := NamespaceInfo{
		ResourceInfo: ResourceInfo{
			Kind:   "Namespace",
			Name:   "production",
			Status: StatusRunning,
		},
		Phase:      "Active",
		Finalizers: []string{"kubernetes"},
	}

	if info.Name != "production" {
		t.Errorf("expected name 'production', got '%s'", info.Name)
	}

	if info.Phase != "Active" {
		t.Errorf("expected phase 'Active', got '%s'", info.Phase)
	}

	if len(info.Finalizers) != 1 {
		t.Errorf("expected 1 finalizer, got %d", len(info.Finalizers))
	}
}

func TestJSONMarshalResourceInfo(t *testing.T) {
	now := time.Now()
	info := ResourceInfo{
		Kind:              "Pod",
		Namespace:         "default",
		Name:              "test-pod",
		Labels:            map[string]string{"app": "test"},
		Status:            StatusRunning,
		CreationTimestamp: now,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal ResourceInfo: %v", err)
	}

	var decoded ResourceInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ResourceInfo: %v", err)
	}

	if decoded.Kind != info.Kind {
		t.Errorf("expected kind '%s', got '%s'", info.Kind, decoded.Kind)
	}

	if decoded.Namespace != info.Namespace {
		t.Errorf("expected namespace '%s', got '%s'", info.Namespace, decoded.Namespace)
	}

	if decoded.Status != info.Status {
		t.Errorf("expected status '%s', got '%s'", info.Status, decoded.Status)
	}
}

func TestJSONMarshalDeploymentInfo(t *testing.T) {
	info := DeploymentInfo{
		ResourceInfo: ResourceInfo{
			Kind:      "Deployment",
			Namespace: "default",
			Name:      "test-deploy",
			Status:    StatusRunning,
		},
		Replicas:          3,
		AvailableReplicas: 3,
		ReadyReplicas:     3,
		UpdatedReplicas:   3,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal DeploymentInfo: %v", err)
	}

	var decoded DeploymentInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal DeploymentInfo: %v", err)
	}

	if decoded.Replicas != info.Replicas {
		t.Errorf("expected replicas %d, got %d", info.Replicas, decoded.Replicas)
	}

	if decoded.AvailableReplicas != info.AvailableReplicas {
		t.Errorf("expected available replicas %d, got %d", info.AvailableReplicas, decoded.AvailableReplicas)
	}
}
