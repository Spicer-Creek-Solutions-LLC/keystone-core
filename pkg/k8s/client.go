package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Client is a wrapper around Kubernetes client-go
type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
	cluster   ClusterConfig
}

// NewClient creates a new Kubernetes client
func NewClient(cluster ClusterConfig) (*Client, error) {
	var config *rest.Config
	var err error

	if cluster.Kubeconfig != "" {
		// Load from kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", cluster.Kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}
	} else {
		// Use in-cluster configuration
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load in-cluster config: %w", err)
		}
	}

	// Apply timeout if specified
	if cluster.Timeout > 0 {
		config.Timeout = cluster.Timeout
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &Client{
		clientset: clientset,
		config:    config,
		cluster:   cluster,
	}, nil
}

// ExecInPod executes a command in a specific pod
func (c *Client) ExecInPod(opts PodExecOptions) (*PodExecResult, error) {
	startTime := time.Now()

	// Get pod to determine container
	if opts.Container == "" {
		pod, err := c.clientset.CoreV1().Pods(opts.Namespace).Get(context.Background(), opts.PodName, metav1.GetOptions{})
		if err != nil {
			return &PodExecResult{Error: err}, err
		}
		if len(pod.Spec.Containers) > 0 {
			opts.Container = pod.Spec.Containers[0].Name
		}
	}

	// Create exec request
	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(opts.PodName).
		Namespace(opts.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: opts.Container,
			Command:   opts.Command,
			Stdin:     opts.Stdin,
			Stdout:    opts.Stdout,
			Stderr:    opts.Stderr,
			TTY:       opts.TTY,
		}, scheme.ParameterCodec)

	// Create executor
	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return &PodExecResult{Error: err}, err
	}

	// Buffers for output
	var stdout, stderr bytes.Buffer

	// Create context with timeout
	ctx := context.Background()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Execute command
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    opts.TTY,
	})

	duration := time.Since(startTime)

	result := &PodExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		result.Error = err
		result.ExitCode = 1
		return result, err
	}

	result.ExitCode = 0
	return result, nil
}

// ExecInPods executes a command in multiple pods matching selector
func (c *Client) ExecInPods(selector PodSelector, command []string) ([]PodExecResult, error) {
	// List pods matching selector
	pods, err := c.ListPods(selector)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Limit number of pods if MaxPods is set
	if selector.MaxPods > 0 && len(pods) > selector.MaxPods {
		pods = pods[:selector.MaxPods]
	}

	// Execute in each pod
	results := make([]PodExecResult, 0, len(pods))
	for _, pod := range pods {
		opts := PodExecOptions{
			Namespace: pod.Namespace,
			PodName:   pod.Name,
			Container: selector.Container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}

		result, err := c.ExecInPod(opts)
		if err != nil {
			// Continue with other pods even if one fails
			result = &PodExecResult{
				ExitCode: 1,
				Error:    err,
				Stderr:   err.Error(),
			}
		}

		results = append(results, *result)
	}

	return results, nil
}

// GetPod retrieves pod information
func (c *Client) GetPod(namespace, name string) (*ResourceInfo, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	return &ResourceInfo{
		Kind:              "Pod",
		Namespace:         pod.Namespace,
		Name:              pod.Name,
		Labels:            pod.Labels,
		Annotations:       pod.Annotations,
		Status:            podStatusToResourceStatus(pod.Status.Phase),
		CreationTimestamp: pod.CreationTimestamp.Time,
		Metadata: map[string]interface{}{
			"uid":         string(pod.UID),
			"nodeName":    pod.Spec.NodeName,
			"podIP":       pod.Status.PodIP,
			"hostIP":      pod.Status.HostIP,
			"phase":       string(pod.Status.Phase),
			"containers":  len(pod.Spec.Containers),
			"restartCount": getTotalRestartCount(pod),
		},
	}, nil
}

// ListPods lists pods matching selector
func (c *Client) ListPods(selector PodSelector) ([]ResourceInfo, error) {
	namespace := selector.Namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	listOpts := metav1.ListOptions{
		LabelSelector: selector.LabelSelector,
		FieldSelector: selector.FieldSelector,
	}

	podList, err := c.clientset.CoreV1().Pods(namespace).List(context.Background(), listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Filter by specific names if provided
	pods := make([]ResourceInfo, 0, len(podList.Items))
	for _, pod := range podList.Items {
		if len(selector.Names) > 0 {
			found := false
			for _, name := range selector.Names {
				if pod.Name == name {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		pods = append(pods, ResourceInfo{
			Kind:              "Pod",
			Namespace:         pod.Namespace,
			Name:              pod.Name,
			Labels:            pod.Labels,
			Annotations:       pod.Annotations,
			Status:            podStatusToResourceStatus(pod.Status.Phase),
			CreationTimestamp: pod.CreationTimestamp.Time,
			Metadata: map[string]interface{}{
				"nodeName":     pod.Spec.NodeName,
				"podIP":        pod.Status.PodIP,
				"phase":        string(pod.Status.Phase),
				"containers":   len(pod.Spec.Containers),
				"restartCount": getTotalRestartCount(&pod),
			},
		})
	}

	return pods, nil
}

// GetDeployment retrieves deployment information
func (c *Client) GetDeployment(namespace, name string) (*DeploymentInfo, error) {
	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	status := StatusUnknown
	if deployment.Status.AvailableReplicas == *deployment.Spec.Replicas {
		status = StatusRunning
	} else if deployment.Status.AvailableReplicas > 0 {
		status = StatusPending
	}

	return &DeploymentInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "Deployment",
			Namespace:         deployment.Namespace,
			Name:              deployment.Name,
			Labels:            deployment.Labels,
			Annotations:       deployment.Annotations,
			Status:            status,
			CreationTimestamp: deployment.CreationTimestamp.Time,
		},
		Replicas:          *deployment.Spec.Replicas,
		AvailableReplicas: deployment.Status.AvailableReplicas,
		ReadyReplicas:     deployment.Status.ReadyReplicas,
		UpdatedReplicas:   deployment.Status.UpdatedReplicas,
	}, nil
}

// GetService retrieves service information
func (c *Client) GetService(namespace, name string) (*ServiceInfo, error) {
	service, err := c.clientset.CoreV1().Services(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	ports := make([]ServicePort, len(service.Spec.Ports))
	for i, p := range service.Spec.Ports {
		ports[i] = ServicePort{
			Name:       p.Name,
			Protocol:   string(p.Protocol),
			Port:       p.Port,
			TargetPort: p.TargetPort.IntVal,
			NodePort:   p.NodePort,
		}
	}

	return &ServiceInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "Service",
			Namespace:         service.Namespace,
			Name:              service.Name,
			Labels:            service.Labels,
			Annotations:       service.Annotations,
			Status:            StatusRunning,
			CreationTimestamp: service.CreationTimestamp.Time,
		},
		Type:        string(service.Spec.Type),
		ClusterIP:   service.Spec.ClusterIP,
		ExternalIPs: service.Spec.ExternalIPs,
		Ports:       ports,
	}, nil
}

// WatchPods watches for pod events
func (c *Client) WatchPods(selector PodSelector) (<-chan WatchEvent, error) {
	namespace := selector.Namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	listOpts := metav1.ListOptions{
		LabelSelector: selector.LabelSelector,
		FieldSelector: selector.FieldSelector,
		Watch:         true,
	}

	watcher, err := c.clientset.CoreV1().Pods(namespace).Watch(context.Background(), listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to watch pods: %w", err)
	}

	eventChan := make(chan WatchEvent, 100)

	go func() {
		defer close(eventChan)
		for event := range watcher.ResultChan() {
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			eventChan <- WatchEvent{
				Type: string(event.Type),
				Resource: ResourceInfo{
					Kind:              "Pod",
					Namespace:         pod.Namespace,
					Name:              pod.Name,
					Labels:            pod.Labels,
					Annotations:       pod.Annotations,
					Status:            podStatusToResourceStatus(pod.Status.Phase),
					CreationTimestamp: pod.CreationTimestamp.Time,
				},
				Timestamp: time.Now(),
			}
		}
	}()

	return eventChan, nil
}

// CreateResource creates a Kubernetes resource from a manifest
func (c *Client) CreateResource(namespace string, manifest []byte) error {
	// This is a simplified implementation
	// In production, you'd want to use server-side apply or dynamic client
	return fmt.Errorf("not implemented")
}

// UpdateResource updates a Kubernetes resource
func (c *Client) UpdateResource(namespace string, manifest []byte) error {
	return fmt.Errorf("not implemented")
}

// DeleteResource deletes a Kubernetes resource
func (c *Client) DeleteResource(namespace, kind, name string) error {
	return fmt.Errorf("not implemented")
}

// GetClusterInfo returns information about the cluster
func (c *Client) GetClusterInfo() (*ClusterInfo, error) {
	version, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	nodes, err := c.clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	namespaces, err := c.clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	return &ClusterInfo{
		Version:    version.String(),
		Nodes:      len(nodes.Items),
		Namespaces: len(namespaces.Items),
		APIServer:  c.config.Host,
	}, nil
}

// Helper functions

func podStatusToResourceStatus(phase corev1.PodPhase) ResourceStatus {
	switch phase {
	case corev1.PodRunning:
		return StatusRunning
	case corev1.PodPending:
		return StatusPending
	case corev1.PodSucceeded:
		return StatusSucceeded
	case corev1.PodFailed:
		return StatusFailed
	default:
		return StatusUnknown
	}
}

func getTotalRestartCount(pod *corev1.Pod) int32 {
	var total int32
	for _, status := range pod.Status.ContainerStatuses {
		total += status.RestartCount
	}
	return total
}

// StreamExecOutput executes a command in a pod and streams output to provided writers
func (c *Client) StreamExecOutput(opts PodExecOptions, stdout, stderr io.Writer) error {
	// Get pod to determine container
	if opts.Container == "" {
		pod, err := c.clientset.CoreV1().Pods(opts.Namespace).Get(context.Background(), opts.PodName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if len(pod.Spec.Containers) > 0 {
			opts.Container = pod.Spec.Containers[0].Name
		}
	}

	// Create exec request
	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(opts.PodName).
		Namespace(opts.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: opts.Container,
			Command:   opts.Command,
			Stdin:     opts.Stdin,
			Stdout:    opts.Stdout,
			Stderr:    opts.Stderr,
			TTY:       opts.TTY,
		}, scheme.ParameterCodec)

	// Create executor
	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return err
	}

	// Create context with timeout
	ctx := context.Background()
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Execute command with streaming output
	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    opts.TTY,
	})
}
