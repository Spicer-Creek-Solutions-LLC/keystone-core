package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Client is a wrapper around Kubernetes client-go
type Client struct {
	clientset kubernetes.Interface
	config    *rest.Config
	cluster   ClusterConfig
}

// NewClientWithInterface creates a new Client with a provided kubernetes.Interface
// This is primarily used for testing with fake clients
func NewClientWithInterface(clientset kubernetes.Interface, cluster ClusterConfig) *Client {
	return &Client{
		clientset: clientset,
		cluster:   cluster,
	}
}

// NewClient creates a new Kubernetes client
func NewClient(cluster ClusterConfig) (*Client, error) {
	var config *rest.Config
	var err error

	if cluster.Kubeconfig != "" {
		// Load from kubeconfig file, respecting context override if specified
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: cluster.Kubeconfig}
		overrides := &clientcmd.ConfigOverrides{}
		if cluster.Context != "" {
			overrides.CurrentContext = cluster.Context
		}
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
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

// RestConfig returns the underlying REST config, or nil if the client was created with NewClientWithInterface.
func (c *Client) RestConfig() *rest.Config {
	return c.config
}

// Clientset returns the underlying kubernetes.Interface for use with subsystems like leader election.
func (c *Client) Clientset() kubernetes.Interface {
	return c.clientset
}

// ExecInPod executes a command in a specific pod
func (c *Client) ExecInPod(ctx context.Context, opts PodExecOptions) (*PodExecResult, error) {
	startTime := time.Now()

	// Get pod to determine container
	if opts.Container == "" {
		pod, err := c.clientset.CoreV1().Pods(opts.Namespace).Get(ctx, opts.PodName, metav1.GetOptions{})
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

	// Apply timeout if configured
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
func (c *Client) ExecInPods(ctx context.Context, selector PodSelector, command []string) ([]PodExecResult, error) {
	// List pods matching selector
	pods, err := c.ListPods(ctx, selector)
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

		result, err := c.ExecInPod(ctx, opts)
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
func (c *Client) GetPod(ctx context.Context, namespace, name string) (*ResourceInfo, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
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
			"uid":          string(pod.UID),
			"nodeName":     pod.Spec.NodeName,
			"podIP":        pod.Status.PodIP,
			"hostIP":       pod.Status.HostIP,
			"phase":        string(pod.Status.Phase),
			"containers":   len(pod.Spec.Containers),
			"restartCount": getTotalRestartCount(pod),
		},
	}, nil
}

// ListPods lists pods matching selector
func (c *Client) ListPods(ctx context.Context, selector PodSelector) ([]ResourceInfo, error) {
	namespace := selector.Namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	listOpts := metav1.ListOptions{
		LabelSelector: selector.LabelSelector,
		FieldSelector: selector.FieldSelector,
	}

	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Filter by specific names if provided
	pods := make([]ResourceInfo, 0, len(podList.Items))
	for i := range podList.Items {
		pod := &podList.Items[i]
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
				"restartCount": getTotalRestartCount(pod),
			},
		})
	}

	return pods, nil
}

// GetDeployment retrieves deployment information
func (c *Client) GetDeployment(ctx context.Context, namespace, name string) (*DeploymentInfo, error) {
	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
func (c *Client) GetService(ctx context.Context, namespace, name string) (*ServiceInfo, error) {
	service, err := c.clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
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
func (c *Client) WatchPods(ctx context.Context, selector PodSelector) (<-chan WatchEvent, error) {
	namespace := selector.Namespace
	if namespace == "" {
		namespace = corev1.NamespaceAll
	}

	listOpts := metav1.ListOptions{
		LabelSelector: selector.LabelSelector,
		FieldSelector: selector.FieldSelector,
		Watch:         true,
	}

	watcher, err := c.clientset.CoreV1().Pods(namespace).Watch(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to watch pods: %w", err)
	}

	eventChan := make(chan WatchEvent, 100)

	go func() {
		defer close(eventChan)
		defer watcher.Stop()
		for event := range watcher.ResultChan() {
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				continue
			}

			we := WatchEvent{
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

			select {
			case eventChan <- we:
			case <-ctx.Done():
				return
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
func (c *Client) GetClusterInfo(ctx context.Context) (*ClusterInfo, error) {
	version, err := c.clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	nodes, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	namespaces, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	apiServer := ""
	if c.config != nil {
		apiServer = c.config.Host
	}

	return &ClusterInfo{
		Version:    version.String(),
		Nodes:      len(nodes.Items),
		Namespaces: len(namespaces.Items),
		APIServer:  apiServer,
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
	for i := range pod.Status.ContainerStatuses {
		total += pod.Status.ContainerStatuses[i].RestartCount
	}
	return total
}

// GetNamespace retrieves namespace information
func (c *Client) GetNamespace(ctx context.Context, name string) (*NamespaceInfo, error) {
	ns, err := c.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	// Convert FinalizerName to string
	finalizers := make([]string, len(ns.Spec.Finalizers))
	for i, f := range ns.Spec.Finalizers {
		finalizers[i] = string(f)
	}

	return &NamespaceInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "Namespace",
			Name:              ns.Name,
			Labels:            ns.Labels,
			Annotations:       ns.Annotations,
			Status:            namespacePhaseToStatus(ns.Status.Phase),
			CreationTimestamp: ns.CreationTimestamp.Time,
			Metadata: map[string]interface{}{
				"uid": string(ns.UID),
			},
		},
		Phase:      string(ns.Status.Phase),
		Finalizers: finalizers,
	}, nil
}

// ListNamespaces lists all namespaces
func (c *Client) ListNamespaces(ctx context.Context) ([]NamespaceInfo, error) {
	nsList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	namespaces := make([]NamespaceInfo, 0, len(nsList.Items))
	for i := range nsList.Items {
		ns := &nsList.Items[i]
		// Convert FinalizerName to string
		finalizers := make([]string, len(ns.Spec.Finalizers))
		for j, f := range ns.Spec.Finalizers {
			finalizers[j] = string(f)
		}

		namespaces = append(namespaces, NamespaceInfo{
			ResourceInfo: ResourceInfo{
				Kind:              "Namespace",
				Name:              ns.Name,
				Labels:            ns.Labels,
				Annotations:       ns.Annotations,
				Status:            namespacePhaseToStatus(ns.Status.Phase),
				CreationTimestamp: ns.CreationTimestamp.Time,
				Metadata: map[string]interface{}{
					"uid": string(ns.UID),
				},
			},
			Phase:      string(ns.Status.Phase),
			Finalizers: finalizers,
		})
	}

	return namespaces, nil
}

// CreateNamespace creates a new namespace
func (c *Client) CreateNamespace(ctx context.Context, spec NamespaceSpec) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
	}

	_, err := c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}

// UpdateNamespace updates a namespace's labels and annotations
func (c *Client) UpdateNamespace(ctx context.Context, spec NamespaceSpec) error {
	// Get current namespace
	ns, err := c.clientset.CoreV1().Namespaces().Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get namespace for update: %w", err)
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

	_, err = c.clientset.CoreV1().Namespaces().Update(ctx, ns, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update namespace: %w", err)
	}

	return nil
}

// DeleteNamespace deletes a namespace
func (c *Client) DeleteNamespace(ctx context.Context, name string) error {
	err := c.clientset.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	return nil
}

func namespacePhaseToStatus(phase corev1.NamespacePhase) ResourceStatus {
	switch phase {
	case corev1.NamespaceActive:
		return StatusRunning
	case corev1.NamespaceTerminating:
		return StatusPending
	default:
		return StatusUnknown
	}
}

// CreateDeployment creates a new deployment
func (c *Client) CreateDeployment(ctx context.Context, namespace string, spec DeploymentSpec) error {
	// Build selector - use provided selector or fall back to labels
	selector := spec.Selector
	if len(selector) == 0 {
		selector = spec.Labels
	}
	if len(selector) == 0 {
		// Default selector
		selector = map[string]string{"app": spec.Name}
	}

	// Build pod template labels - must match selector
	podLabels := make(map[string]string)
	for k, v := range selector {
		podLabels[k] = v
	}

	replicas := spec.Replicas
	if replicas == 0 {
		replicas = 1
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  spec.Name,
							Image: spec.Image,
						},
					},
				},
			},
		},
	}

	// Add container port if specified
	if spec.ContainerPort > 0 {
		deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
			{ContainerPort: spec.ContainerPort},
		}
	}

	_, err := c.clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	return nil
}

// UpdateDeployment updates a deployment's labels, annotations, and replicas
func (c *Client) UpdateDeployment(ctx context.Context, namespace string, spec DeploymentSpec) error {
	// Get current deployment
	deployment, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment for update: %w", err)
	}

	// Update replicas if specified
	if spec.Replicas > 0 {
		deployment.Spec.Replicas = &spec.Replicas
	}

	// Merge labels
	if deployment.Labels == nil {
		deployment.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		deployment.Labels[k] = v
	}

	// Merge annotations
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		deployment.Annotations[k] = v
	}

	// Update image if specified
	if spec.Image != "" && len(deployment.Spec.Template.Spec.Containers) > 0 {
		deployment.Spec.Template.Spec.Containers[0].Image = spec.Image
	}

	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
}

// DeleteDeployment deletes a deployment
func (c *Client) DeleteDeployment(ctx context.Context, namespace, name string) error {
	err := c.clientset.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// ScaleDeployment scales a deployment to specified replicas
func (c *Client) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	scale, err := c.clientset.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment scale: %w", err)
	}

	scale.Spec.Replicas = replicas

	_, err = c.clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to scale deployment: %w", err)
	}

	return nil
}

// CreateService creates a new Kubernetes service
func (c *Client) CreateService(ctx context.Context, namespace string, spec ServiceSpec) error {
	// Build ports
	ports := make([]corev1.ServicePort, len(spec.Ports))
	for i, p := range spec.Ports {
		protocol := corev1.ProtocolTCP
		if p.Protocol != "" {
			protocol = corev1.Protocol(p.Protocol)
		}
		targetPort := p.TargetPort
		if targetPort == 0 {
			targetPort = p.Port
		}
		ports[i] = corev1.ServicePort{
			Name:       p.Name,
			Protocol:   protocol,
			Port:       p.Port,
			TargetPort: intstr.FromInt32(targetPort),
			NodePort:   p.NodePort,
		}
	}

	// Build service type
	serviceType := corev1.ServiceTypeClusterIP
	if spec.Type != "" {
		serviceType = corev1.ServiceType(spec.Type)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:         serviceType,
			Selector:     spec.Selector,
			Ports:        ports,
			ClusterIP:    spec.ClusterIP,
			ExternalName: spec.ExternalName,
		},
	}

	_, err := c.clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return nil
}

// UpdateService updates an existing Kubernetes service
func (c *Client) UpdateService(ctx context.Context, namespace string, spec ServiceSpec) error {
	// Get existing service
	service, err := c.clientset.CoreV1().Services(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Update ports if specified
	if len(spec.Ports) > 0 {
		ports := make([]corev1.ServicePort, len(spec.Ports))
		for i, p := range spec.Ports {
			protocol := corev1.ProtocolTCP
			if p.Protocol != "" {
				protocol = corev1.Protocol(p.Protocol)
			}
			targetPort := p.TargetPort
			if targetPort == 0 {
				targetPort = p.Port
			}
			ports[i] = corev1.ServicePort{
				Name:       p.Name,
				Protocol:   protocol,
				Port:       p.Port,
				TargetPort: intstr.FromInt32(targetPort),
				NodePort:   p.NodePort,
			}
		}
		service.Spec.Ports = ports
	}

	// Update selector if specified
	if spec.Selector != nil {
		service.Spec.Selector = spec.Selector
	}

	// Merge labels
	if service.Labels == nil {
		service.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		service.Labels[k] = v
	}

	// Merge annotations
	if service.Annotations == nil {
		service.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		service.Annotations[k] = v
	}

	// Update type if specified (note: not all type changes are allowed)
	if spec.Type != "" {
		service.Spec.Type = corev1.ServiceType(spec.Type)
	}

	_, err = c.clientset.CoreV1().Services(namespace).Update(ctx, service, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update service: %w", err)
	}

	return nil
}

// DeleteService deletes a Kubernetes service
func (c *Client) DeleteService(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	return nil
}

// GetConfigMap retrieves configmap information
func (c *Client) GetConfigMap(ctx context.Context, namespace, name string) (*ConfigMapInfo, error) {
	cm, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &ConfigMapInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "ConfigMap",
			Namespace:         cm.Namespace,
			Name:              cm.Name,
			Labels:            cm.Labels,
			Annotations:       cm.Annotations,
			CreationTimestamp: cm.CreationTimestamp.Time,
		},
		Data:       cm.Data,
		BinaryData: cm.BinaryData,
	}, nil
}

// CreateConfigMap creates a new configmap
func (c *Client) CreateConfigMap(ctx context.Context, namespace string, spec ConfigMapSpec) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Data:       spec.Data,
		BinaryData: spec.BinaryData,
	}

	_, err := c.clientset.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create configmap: %w", err)
	}
	return nil
}

// UpdateConfigMap updates a configmap
func (c *Client) UpdateConfigMap(ctx context.Context, namespace string, spec ConfigMapSpec) error {
	// Get existing configmap
	cm, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get configmap for update: %w", err)
	}

	// Update fields
	if spec.Labels != nil {
		if cm.Labels == nil {
			cm.Labels = make(map[string]string)
		}
		for k, v := range spec.Labels {
			cm.Labels[k] = v
		}
	}

	if spec.Annotations != nil {
		if cm.Annotations == nil {
			cm.Annotations = make(map[string]string)
		}
		for k, v := range spec.Annotations {
			cm.Annotations[k] = v
		}
	}

	// Replace data entirely if provided
	if spec.Data != nil {
		cm.Data = spec.Data
	}

	if spec.BinaryData != nil {
		cm.BinaryData = spec.BinaryData
	}

	_, err = c.clientset.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update configmap: %w", err)
	}
	return nil
}

// DeleteConfigMap deletes a configmap
func (c *Client) DeleteConfigMap(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete configmap: %w", err)
	}
	return nil
}

// GetSecret retrieves secret information
func (c *Client) GetSecret(ctx context.Context, namespace, name string) (*SecretInfo, error) {
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &SecretInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "Secret",
			Namespace:         secret.Namespace,
			Name:              secret.Name,
			Labels:            secret.Labels,
			Annotations:       secret.Annotations,
			CreationTimestamp: secret.CreationTimestamp.Time,
		},
		Type: string(secret.Type),
		Data: secret.Data,
	}, nil
}

// CreateSecret creates a new secret
func (c *Client) CreateSecret(ctx context.Context, namespace string, spec SecretSpec) error {
	secretType := corev1.SecretTypeOpaque
	if spec.Type != "" {
		secretType = corev1.SecretType(spec.Type)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Type:       secretType,
		Data:       spec.Data,
		StringData: spec.StringData,
	}

	_, err := c.clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}
	return nil
}

// UpdateSecret updates a secret
func (c *Client) UpdateSecret(ctx context.Context, namespace string, spec SecretSpec) error {
	// Get existing secret
	secret, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret for update: %w", err)
	}

	// Update labels
	if spec.Labels != nil {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		for k, v := range spec.Labels {
			secret.Labels[k] = v
		}
	}

	// Update annotations
	if spec.Annotations != nil {
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		for k, v := range spec.Annotations {
			secret.Annotations[k] = v
		}
	}

	// Update type if specified (note: changing type may not always be allowed)
	if spec.Type != "" {
		secret.Type = corev1.SecretType(spec.Type)
	}

	// Replace data entirely if provided
	if spec.Data != nil {
		secret.Data = spec.Data
	}

	// Handle StringData - convert to Data
	if spec.StringData != nil {
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		for k, v := range spec.StringData {
			secret.Data[k] = []byte(v)
		}
	}

	_, err = c.clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}
	return nil
}

// DeleteSecret deletes a secret
func (c *Client) DeleteSecret(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}

// GetIngress retrieves ingress information
func (c *Client) GetIngress(ctx context.Context, namespace, name string) (*IngressInfo, error) {
	ingress, err := c.clientset.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Convert rules
	rules := make([]IngressRule, len(ingress.Spec.Rules))
	for i, rule := range ingress.Spec.Rules {
		paths := make([]IngressPath, 0)
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				pathType := "ImplementationSpecific"
				if path.PathType != nil {
					pathType = string(*path.PathType)
				}
				backend := IngressBackend{}
				if path.Backend.Service != nil {
					backend.ServiceName = path.Backend.Service.Name
					if path.Backend.Service.Port.Number != 0 {
						backend.ServicePort = path.Backend.Service.Port.Number
					}
				}
				paths = append(paths, IngressPath{
					Path:     path.Path,
					PathType: pathType,
					Backend:  backend,
				})
			}
		}
		rules[i] = IngressRule{
			Host:  rule.Host,
			Paths: paths,
		}
	}

	// Convert TLS
	tls := make([]IngressTLS, len(ingress.Spec.TLS))
	for i, t := range ingress.Spec.TLS {
		tls[i] = IngressTLS{
			Hosts:      t.Hosts,
			SecretName: t.SecretName,
		}
	}

	// Convert default backend
	var defaultBackend *IngressBackend
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		defaultBackend = &IngressBackend{
			ServiceName: ingress.Spec.DefaultBackend.Service.Name,
		}
		if ingress.Spec.DefaultBackend.Service.Port.Number != 0 {
			defaultBackend.ServicePort = ingress.Spec.DefaultBackend.Service.Port.Number
		}
	}

	// Get load balancer ingress IPs/hostnames
	lbIngress := make([]string, 0)
	for _, lb := range ingress.Status.LoadBalancer.Ingress {
		if lb.IP != "" {
			lbIngress = append(lbIngress, lb.IP)
		} else if lb.Hostname != "" {
			lbIngress = append(lbIngress, lb.Hostname)
		}
	}

	ingressClassName := ""
	if ingress.Spec.IngressClassName != nil {
		ingressClassName = *ingress.Spec.IngressClassName
	}

	return &IngressInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "Ingress",
			Namespace:         ingress.Namespace,
			Name:              ingress.Name,
			Labels:            ingress.Labels,
			Annotations:       ingress.Annotations,
			CreationTimestamp: ingress.CreationTimestamp.Time,
		},
		IngressClassName:    ingressClassName,
		Rules:               rules,
		TLS:                 tls,
		DefaultBackend:      defaultBackend,
		LoadBalancerIngress: lbIngress,
	}, nil
}

// CreateIngress creates a new ingress
func (c *Client) CreateIngress(ctx context.Context, namespace string, spec IngressSpec) error {
	// Build rules
	rules := make([]networkingv1.IngressRule, len(spec.Rules))
	for i, rule := range spec.Rules {
		paths := make([]networkingv1.HTTPIngressPath, len(rule.Paths))
		for j, path := range rule.Paths {
			pathType := networkingv1.PathTypeImplementationSpecific
			switch path.PathType {
			case "Exact":
				pathType = networkingv1.PathTypeExact
			case "Prefix":
				pathType = networkingv1.PathTypePrefix
			}
			paths[j] = networkingv1.HTTPIngressPath{
				Path:     path.Path,
				PathType: &pathType,
				Backend: networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: path.Backend.ServiceName,
						Port: networkingv1.ServiceBackendPort{
							Number: path.Backend.ServicePort,
						},
					},
				},
			}
		}
		rules[i] = networkingv1.IngressRule{
			Host: rule.Host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: paths,
				},
			},
		}
	}

	// Build TLS
	tls := make([]networkingv1.IngressTLS, len(spec.TLS))
	for i, t := range spec.TLS {
		tls[i] = networkingv1.IngressTLS{
			Hosts:      t.Hosts,
			SecretName: t.SecretName,
		}
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			Rules: rules,
			TLS:   tls,
		},
	}

	// Set ingress class if specified
	if spec.IngressClassName != "" {
		ingress.Spec.IngressClassName = &spec.IngressClassName
	}

	// Set default backend if specified
	if spec.DefaultBackend != nil {
		ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: spec.DefaultBackend.ServiceName,
				Port: networkingv1.ServiceBackendPort{
					Number: spec.DefaultBackend.ServicePort,
				},
			},
		}
	}

	_, err := c.clientset.NetworkingV1().Ingresses(namespace).Create(ctx, ingress, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ingress: %w", err)
	}
	return nil
}

// UpdateIngress updates an ingress
func (c *Client) UpdateIngress(ctx context.Context, namespace string, spec IngressSpec) error {
	// Get existing ingress
	ingress, err := c.clientset.NetworkingV1().Ingresses(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get ingress for update: %w", err)
	}

	// Update labels
	if spec.Labels != nil {
		if ingress.Labels == nil {
			ingress.Labels = make(map[string]string)
		}
		for k, v := range spec.Labels {
			ingress.Labels[k] = v
		}
	}

	// Update annotations
	if spec.Annotations != nil {
		if ingress.Annotations == nil {
			ingress.Annotations = make(map[string]string)
		}
		for k, v := range spec.Annotations {
			ingress.Annotations[k] = v
		}
	}

	// Update ingress class if specified
	if spec.IngressClassName != "" {
		ingress.Spec.IngressClassName = &spec.IngressClassName
	}

	// Update rules if specified
	if len(spec.Rules) > 0 {
		rules := make([]networkingv1.IngressRule, len(spec.Rules))
		for i, rule := range spec.Rules {
			paths := make([]networkingv1.HTTPIngressPath, len(rule.Paths))
			for j, path := range rule.Paths {
				pathType := networkingv1.PathTypeImplementationSpecific
				switch path.PathType {
				case "Exact":
					pathType = networkingv1.PathTypeExact
				case "Prefix":
					pathType = networkingv1.PathTypePrefix
				}
				paths[j] = networkingv1.HTTPIngressPath{
					Path:     path.Path,
					PathType: &pathType,
					Backend: networkingv1.IngressBackend{
						Service: &networkingv1.IngressServiceBackend{
							Name: path.Backend.ServiceName,
							Port: networkingv1.ServiceBackendPort{
								Number: path.Backend.ServicePort,
							},
						},
					},
				}
			}
			rules[i] = networkingv1.IngressRule{
				Host: rule.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: paths,
					},
				},
			}
		}
		ingress.Spec.Rules = rules
	}

	// Update TLS if specified
	if len(spec.TLS) > 0 {
		tls := make([]networkingv1.IngressTLS, len(spec.TLS))
		for i, t := range spec.TLS {
			tls[i] = networkingv1.IngressTLS{
				Hosts:      t.Hosts,
				SecretName: t.SecretName,
			}
		}
		ingress.Spec.TLS = tls
	}

	// Update default backend if specified
	if spec.DefaultBackend != nil {
		ingress.Spec.DefaultBackend = &networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: spec.DefaultBackend.ServiceName,
				Port: networkingv1.ServiceBackendPort{
					Number: spec.DefaultBackend.ServicePort,
				},
			},
		}
	}

	_, err = c.clientset.NetworkingV1().Ingresses(namespace).Update(ctx, ingress, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ingress: %w", err)
	}
	return nil
}

// DeleteIngress deletes an ingress
func (c *Client) DeleteIngress(ctx context.Context, namespace, name string) error {
	err := c.clientset.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete ingress: %w", err)
	}
	return nil
}

// GetStatefulSet retrieves statefulset information
func (c *Client) GetStatefulSet(ctx context.Context, namespace, name string) (*StatefulSetInfo, error) {
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset: %w", err)
	}

	status := StatusUnknown
	if sts.Status.ReadyReplicas == *sts.Spec.Replicas {
		status = StatusRunning
	} else if sts.Status.ReadyReplicas > 0 {
		status = StatusPending
	}

	// Extract update strategy
	updateStrategy := "RollingUpdate"
	if sts.Spec.UpdateStrategy.Type != "" {
		updateStrategy = string(sts.Spec.UpdateStrategy.Type)
	}

	// Extract pod management policy
	podManagementPolicy := "OrderedReady"
	if sts.Spec.PodManagementPolicy != "" {
		podManagementPolicy = string(sts.Spec.PodManagementPolicy)
	}

	return &StatefulSetInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "StatefulSet",
			Namespace:         sts.Namespace,
			Name:              sts.Name,
			Labels:            sts.Labels,
			Annotations:       sts.Annotations,
			Status:            status,
			CreationTimestamp: sts.CreationTimestamp.Time,
		},
		Replicas:            *sts.Spec.Replicas,
		ReadyReplicas:       sts.Status.ReadyReplicas,
		CurrentReplicas:     sts.Status.CurrentReplicas,
		UpdatedReplicas:     sts.Status.UpdatedReplicas,
		CurrentRevision:     sts.Status.CurrentRevision,
		UpdateRevision:      sts.Status.UpdateRevision,
		ServiceName:         sts.Spec.ServiceName,
		PodManagementPolicy: podManagementPolicy,
		UpdateStrategy:      updateStrategy,
	}, nil
}

// CreateStatefulSet creates a new statefulset
func (c *Client) CreateStatefulSet(ctx context.Context, namespace string, spec StatefulSetSpec) error {
	// Build selector - use provided selector or fall back to labels
	selector := spec.Selector
	if len(selector) == 0 {
		selector = spec.Labels
	}
	if len(selector) == 0 {
		// Default selector
		selector = map[string]string{"app": spec.Name}
	}

	// Build pod template labels - must match selector
	podLabels := make(map[string]string)
	for k, v := range selector {
		podLabels[k] = v
	}

	replicas := spec.Replicas
	if replicas == 0 {
		replicas = 1
	}

	// Set pod management policy
	podManagementPolicy := appsv1.OrderedReadyPodManagement
	if spec.PodManagementPolicy == "Parallel" {
		podManagementPolicy = appsv1.ParallelPodManagement
	}

	// Set update strategy
	updateStrategy := appsv1.RollingUpdateStatefulSetStrategyType
	if spec.UpdateStrategy == "OnDelete" {
		updateStrategy = appsv1.OnDeleteStatefulSetStrategyType
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            &replicas,
			ServiceName:         spec.ServiceName,
			PodManagementPolicy: podManagementPolicy,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: updateStrategy,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  spec.Name,
							Image: spec.Image,
						},
					},
				},
			},
		},
	}

	// Add container port if specified
	if spec.ContainerPort > 0 {
		sts.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
			{ContainerPort: spec.ContainerPort},
		}
	}

	// Add volume claim templates if specified
	if len(spec.VolumeClaimTemplates) > 0 {
		pvcs := make([]corev1.PersistentVolumeClaim, len(spec.VolumeClaimTemplates))
		for i, vct := range spec.VolumeClaimTemplates {
			accessModes := make([]corev1.PersistentVolumeAccessMode, len(vct.AccessModes))
			for j, am := range vct.AccessModes {
				switch am {
				case "ReadWriteOnce":
					accessModes[j] = corev1.ReadWriteOnce
				case "ReadOnlyMany":
					accessModes[j] = corev1.ReadOnlyMany
				case "ReadWriteMany":
					accessModes[j] = corev1.ReadWriteMany
				case "ReadWriteOncePod":
					accessModes[j] = corev1.ReadWriteOncePod
				default:
					accessModes[j] = corev1.ReadWriteOnce
				}
			}

			pvc := corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: vct.Name,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: accessModes,
				},
			}

			// Set storage class if specified
			if vct.StorageClassName != "" {
				pvc.Spec.StorageClassName = &vct.StorageClassName
			}

			// Parse and set storage size
			if vct.StorageSize != "" {
				quantity, err := resourceQuantityFromString(vct.StorageSize)
				if err == nil {
					pvc.Spec.Resources.Requests = corev1.ResourceList{
						corev1.ResourceStorage: quantity,
					}
				}
			}

			pvcs[i] = pvc
		}
		sts.Spec.VolumeClaimTemplates = pvcs
	}

	_, err := c.clientset.AppsV1().StatefulSets(namespace).Create(ctx, sts, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create statefulset: %w", err)
	}

	return nil
}

// UpdateStatefulSet updates a statefulset's labels, annotations, and replicas
func (c *Client) UpdateStatefulSet(ctx context.Context, namespace string, spec StatefulSetSpec) error {
	// Get current statefulset
	sts, err := c.clientset.AppsV1().StatefulSets(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get statefulset for update: %w", err)
	}

	// Update replicas if specified
	if spec.Replicas > 0 {
		sts.Spec.Replicas = &spec.Replicas
	}

	// Merge labels
	if sts.Labels == nil {
		sts.Labels = make(map[string]string)
	}
	for k, v := range spec.Labels {
		sts.Labels[k] = v
	}

	// Merge annotations
	if sts.Annotations == nil {
		sts.Annotations = make(map[string]string)
	}
	for k, v := range spec.Annotations {
		sts.Annotations[k] = v
	}

	// Update image if specified
	if spec.Image != "" && len(sts.Spec.Template.Spec.Containers) > 0 {
		sts.Spec.Template.Spec.Containers[0].Image = spec.Image
	}

	// Update update strategy if specified
	if spec.UpdateStrategy != "" {
		switch spec.UpdateStrategy {
		case "OnDelete":
			sts.Spec.UpdateStrategy.Type = appsv1.OnDeleteStatefulSetStrategyType
		default:
			sts.Spec.UpdateStrategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
		}
	}

	_, err = c.clientset.AppsV1().StatefulSets(namespace).Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update statefulset: %w", err)
	}
	return nil
}

// DeleteStatefulSet deletes a statefulset
func (c *Client) DeleteStatefulSet(ctx context.Context, namespace, name string) error {
	err := c.clientset.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete statefulset: %w", err)
	}
	return nil
}

// ScaleStatefulSet scales a statefulset to specified replicas
func (c *Client) ScaleStatefulSet(ctx context.Context, namespace, name string, replicas int32) error {
	scale, err := c.clientset.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get statefulset scale: %w", err)
	}

	scale.Spec.Replicas = replicas

	_, err = c.clientset.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to scale statefulset: %w", err)
	}

	return nil
}

// ==================== DaemonSet Methods ====================

// GetDaemonSet retrieves daemonset information
func (c *Client) GetDaemonSet(ctx context.Context, namespace, name string) (*DaemonSetInfo, error) {
	ds, err := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get daemonset: %w", err)
	}

	status := StatusUnknown
	if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled {
		status = StatusRunning
	} else if ds.Status.NumberReady > 0 {
		status = StatusPending
	}

	updateStrategy := "RollingUpdate"
	if ds.Spec.UpdateStrategy.Type != "" {
		updateStrategy = string(ds.Spec.UpdateStrategy.Type)
	}

	return &DaemonSetInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "DaemonSet",
			Namespace:         ds.Namespace,
			Name:              ds.Name,
			Labels:            ds.Labels,
			Annotations:       ds.Annotations,
			Status:            status,
			CreationTimestamp: ds.CreationTimestamp.Time,
		},
		DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
		CurrentNumberScheduled: ds.Status.CurrentNumberScheduled,
		NumberReady:            ds.Status.NumberReady,
		NumberAvailable:        ds.Status.NumberAvailable,
		NumberMisscheduled:     ds.Status.NumberMisscheduled,
		UpdatedNumberScheduled: ds.Status.UpdatedNumberScheduled,
		UpdateStrategy:         updateStrategy,
	}, nil
}

// CreateDaemonSet creates a new daemonset
func (c *Client) CreateDaemonSet(ctx context.Context, namespace string, spec DaemonSetSpec) error {
	selector := spec.Selector
	if len(selector) == 0 {
		selector = spec.Labels
	}
	if len(selector) == 0 {
		selector = map[string]string{"app": spec.Name}
	}

	podLabels := make(map[string]string)
	for k, v := range selector {
		podLabels[k] = v
	}

	updateStrategy := appsv1.RollingUpdateDaemonSetStrategyType
	if spec.UpdateStrategy == "OnDelete" {
		updateStrategy = appsv1.OnDeleteDaemonSetStrategyType
	}

	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: updateStrategy,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  spec.Name,
							Image: spec.Image,
						},
					},
				},
			},
		},
	}

	if spec.ContainerPort > 0 {
		ds.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{
			{ContainerPort: spec.ContainerPort},
		}
	}

	if spec.NodeSelector != nil {
		ds.Spec.Template.Spec.NodeSelector = spec.NodeSelector
	}

	_, err := c.clientset.AppsV1().DaemonSets(namespace).Create(ctx, ds, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create daemonset: %w", err)
	}
	return nil
}

// UpdateDaemonSet updates a daemonset
func (c *Client) UpdateDaemonSet(ctx context.Context, namespace string, spec DaemonSetSpec) error {
	ds, err := c.clientset.AppsV1().DaemonSets(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get daemonset for update: %w", err)
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

	if spec.Image != "" && len(ds.Spec.Template.Spec.Containers) > 0 {
		ds.Spec.Template.Spec.Containers[0].Image = spec.Image
	}

	if spec.UpdateStrategy != "" {
		switch spec.UpdateStrategy {
		case "OnDelete":
			ds.Spec.UpdateStrategy.Type = appsv1.OnDeleteDaemonSetStrategyType
		default:
			ds.Spec.UpdateStrategy.Type = appsv1.RollingUpdateDaemonSetStrategyType
		}
	}

	_, err = c.clientset.AppsV1().DaemonSets(namespace).Update(ctx, ds, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update daemonset: %w", err)
	}
	return nil
}

// DeleteDaemonSet deletes a daemonset
func (c *Client) DeleteDaemonSet(ctx context.Context, namespace, name string) error {
	err := c.clientset.AppsV1().DaemonSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete daemonset: %w", err)
	}
	return nil
}

// ==================== Job Methods ====================

// GetJob retrieves job information
func (c *Client) GetJob(ctx context.Context, namespace, name string) (*JobInfo, error) {
	job, err := c.clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	status := StatusUnknown
	switch {
	case job.Status.Succeeded > 0:
		status = StatusRunning // Completed successfully
	case job.Status.Failed > 0:
		status = StatusFailed
	case job.Status.Active > 0:
		status = StatusPending // Running
	}

	info := &JobInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "Job",
			Namespace:         job.Namespace,
			Name:              job.Name,
			Labels:            job.Labels,
			Annotations:       job.Annotations,
			Status:            status,
			CreationTimestamp: job.CreationTimestamp.Time,
		},
		Active:    job.Status.Active,
		Succeeded: job.Status.Succeeded,
		Failed:    job.Status.Failed,
	}

	if job.Spec.Completions != nil {
		info.Completions = *job.Spec.Completions
	}
	if job.Spec.Parallelism != nil {
		info.Parallelism = *job.Spec.Parallelism
	}
	if job.Spec.BackoffLimit != nil {
		info.BackoffLimit = *job.Spec.BackoffLimit
	}
	if job.Status.StartTime != nil {
		t := job.Status.StartTime.Time
		info.StartTime = &t
	}
	if job.Status.CompletionTime != nil {
		t := job.Status.CompletionTime.Time
		info.CompletionTime = &t
	}

	return info, nil
}

// CreateJob creates a new job
func (c *Client) CreateJob(ctx context.Context, namespace string, spec JobSpec) error {
	completions := spec.Completions
	if completions == 0 {
		completions = 1
	}
	parallelism := spec.Parallelism
	if parallelism == 0 {
		parallelism = 1
	}
	backoffLimit := spec.BackoffLimit
	if backoffLimit == 0 {
		backoffLimit = 6
	}

	restartPolicy := corev1.RestartPolicyNever
	if spec.RestartPolicy == "OnFailure" {
		restartPolicy = corev1.RestartPolicyOnFailure
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: batchv1.JobSpec{
			Completions:  &completions,
			Parallelism:  &parallelism,
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: restartPolicy,
					Containers: []corev1.Container{
						{
							Name:    spec.Name,
							Image:   spec.Image,
							Command: spec.Command,
							Args:    spec.Args,
						},
					},
				},
			},
		},
	}

	if spec.ActiveDeadlineSeconds != nil {
		job.Spec.ActiveDeadlineSeconds = spec.ActiveDeadlineSeconds
	}
	if spec.TTLSecondsAfterFinished != nil {
		job.Spec.TTLSecondsAfterFinished = spec.TTLSecondsAfterFinished
	}

	_, err := c.clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}
	return nil
}

// DeleteJob deletes a job
func (c *Client) DeleteJob(ctx context.Context, namespace, name string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	err := c.clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	if err != nil {
		return fmt.Errorf("failed to delete job: %w", err)
	}
	return nil
}

// ==================== CronJob Methods ====================

// GetCronJob retrieves cronjob information
func (c *Client) GetCronJob(ctx context.Context, namespace, name string) (*CronJobInfo, error) {
	cj, err := c.clientset.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cronjob: %w", err)
	}

	status := StatusRunning
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		status = StatusPending // Suspended
	}

	info := &CronJobInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "CronJob",
			Namespace:         cj.Namespace,
			Name:              cj.Name,
			Labels:            cj.Labels,
			Annotations:       cj.Annotations,
			Status:            status,
			CreationTimestamp: cj.CreationTimestamp.Time,
		},
		Schedule:          cj.Spec.Schedule,
		ConcurrencyPolicy: string(cj.Spec.ConcurrencyPolicy),
		//nolint:gosec // G115: number of active jobs is small, fits in int32
		ActiveJobs:        int32(len(cj.Status.Active)),
	}

	if cj.Spec.Suspend != nil {
		info.Suspend = *cj.Spec.Suspend
	}
	if cj.Status.LastScheduleTime != nil {
		t := cj.Status.LastScheduleTime.Time
		info.LastScheduleTime = &t
	}
	if cj.Status.LastSuccessfulTime != nil {
		t := cj.Status.LastSuccessfulTime.Time
		info.LastSuccessfulTime = &t
	}

	return info, nil
}

// CreateCronJob creates a new cronjob
func (c *Client) CreateCronJob(ctx context.Context, namespace string, spec CronJobSpec) error {
	concurrencyPolicy := batchv1.AllowConcurrent
	switch spec.ConcurrencyPolicy {
	case "Forbid":
		concurrencyPolicy = batchv1.ForbidConcurrent
	case "Replace":
		concurrencyPolicy = batchv1.ReplaceConcurrent
	}

	restartPolicy := corev1.RestartPolicyNever
	if spec.RestartPolicy == "OnFailure" {
		restartPolicy = corev1.RestartPolicyOnFailure
	}

	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          spec.Schedule,
			Suspend:           &spec.Suspend,
			ConcurrencyPolicy: concurrencyPolicy,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: restartPolicy,
							Containers: []corev1.Container{
								{
									Name:    spec.Name,
									Image:   spec.Image,
									Command: spec.Command,
									Args:    spec.Args,
								},
							},
						},
					},
				},
			},
		},
	}

	if spec.SuccessfulJobsHistoryLimit != nil {
		cj.Spec.SuccessfulJobsHistoryLimit = spec.SuccessfulJobsHistoryLimit
	}
	if spec.FailedJobsHistoryLimit != nil {
		cj.Spec.FailedJobsHistoryLimit = spec.FailedJobsHistoryLimit
	}
	if spec.StartingDeadlineSeconds != nil {
		cj.Spec.StartingDeadlineSeconds = spec.StartingDeadlineSeconds
	}

	_, err := c.clientset.BatchV1().CronJobs(namespace).Create(ctx, cj, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create cronjob: %w", err)
	}
	return nil
}

// UpdateCronJob updates a cronjob
func (c *Client) UpdateCronJob(ctx context.Context, namespace string, spec CronJobSpec) error {
	cj, err := c.clientset.BatchV1().CronJobs(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get cronjob for update: %w", err)
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

	if spec.Schedule != "" {
		cj.Spec.Schedule = spec.Schedule
	}

	cj.Spec.Suspend = &spec.Suspend

	if spec.ConcurrencyPolicy != "" {
		switch spec.ConcurrencyPolicy {
		case "Forbid":
			cj.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
		case "Replace":
			cj.Spec.ConcurrencyPolicy = batchv1.ReplaceConcurrent
		default:
			cj.Spec.ConcurrencyPolicy = batchv1.AllowConcurrent
		}
	}

	if spec.Image != "" && len(cj.Spec.JobTemplate.Spec.Template.Spec.Containers) > 0 {
		cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image = spec.Image
	}

	_, err = c.clientset.BatchV1().CronJobs(namespace).Update(ctx, cj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update cronjob: %w", err)
	}
	return nil
}

// DeleteCronJob deletes a cronjob
func (c *Client) DeleteCronJob(ctx context.Context, namespace, name string) error {
	err := c.clientset.BatchV1().CronJobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete cronjob: %w", err)
	}
	return nil
}

// ==================== PVC Methods ====================

// GetPVC retrieves persistent volume claim information
func (c *Client) GetPVC(ctx context.Context, namespace, name string) (*PVCInfo, error) {
	pvc, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pvc: %w", err)
	}

	status := StatusUnknown
	switch pvc.Status.Phase {
	case corev1.ClaimBound:
		status = StatusRunning
	case corev1.ClaimPending:
		status = StatusPending
	case corev1.ClaimLost:
		status = StatusFailed
	}

	accessModes := make([]string, len(pvc.Spec.AccessModes))
	for i, am := range pvc.Spec.AccessModes {
		accessModes[i] = string(am)
	}

	info := &PVCInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "PersistentVolumeClaim",
			Namespace:         pvc.Namespace,
			Name:              pvc.Name,
			Labels:            pvc.Labels,
			Annotations:       pvc.Annotations,
			Status:            status,
			CreationTimestamp: pvc.CreationTimestamp.Time,
		},
		Phase:       string(pvc.Status.Phase),
		VolumeName:  pvc.Spec.VolumeName,
		AccessModes: accessModes,
	}

	if pvc.Spec.StorageClassName != nil {
		info.StorageClassName = *pvc.Spec.StorageClassName
	}
	if req, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		info.RequestedStorage = req.String()
	}
	if alloc, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		info.AllocatedStorage = alloc.String()
	}

	return info, nil
}

// CreatePVC creates a new persistent volume claim
func (c *Client) CreatePVC(ctx context.Context, namespace string, spec PVCSpec) error {
	accessModes := make([]corev1.PersistentVolumeAccessMode, len(spec.AccessModes))
	for i, am := range spec.AccessModes {
		switch am {
		case "ReadWriteOnce":
			accessModes[i] = corev1.ReadWriteOnce
		case "ReadOnlyMany":
			accessModes[i] = corev1.ReadOnlyMany
		case "ReadWriteMany":
			accessModes[i] = corev1.ReadWriteMany
		case "ReadWriteOncePod":
			accessModes[i] = corev1.ReadWriteOncePod
		default:
			accessModes[i] = corev1.ReadWriteOnce
		}
	}

	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
		},
	}

	if spec.StorageClassName != "" {
		pvc.Spec.StorageClassName = &spec.StorageClassName
	}

	if spec.StorageSize != "" {
		quantity, err := resource.ParseQuantity(spec.StorageSize)
		if err != nil {
			return fmt.Errorf("invalid storage size: %w", err)
		}
		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: quantity,
		}
	}

	if spec.VolumeName != "" {
		pvc.Spec.VolumeName = spec.VolumeName
	}

	if spec.VolumeMode != "" {
		mode := corev1.PersistentVolumeFilesystem
		if spec.VolumeMode == "Block" {
			mode = corev1.PersistentVolumeBlock
		}
		pvc.Spec.VolumeMode = &mode
	}

	_, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create pvc: %w", err)
	}
	return nil
}

// UpdatePVC updates a persistent volume claim
func (c *Client) UpdatePVC(ctx context.Context, namespace string, spec PVCSpec) error {
	pvc, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pvc for update: %w", err)
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

	// Note: Storage size can be expanded but not shrunk
	if spec.StorageSize != "" {
		quantity, err := resource.ParseQuantity(spec.StorageSize)
		if err != nil {
			return fmt.Errorf("invalid storage size: %w", err)
		}
		if pvc.Spec.Resources.Requests == nil {
			pvc.Spec.Resources.Requests = corev1.ResourceList{}
		}
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = quantity
	}

	_, err = c.clientset.CoreV1().PersistentVolumeClaims(namespace).Update(ctx, pvc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update pvc: %w", err)
	}
	return nil
}

// DeletePVC deletes a persistent volume claim
func (c *Client) DeletePVC(ctx context.Context, namespace, name string) error {
	err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pvc: %w", err)
	}
	return nil
}

// ==================== HPA Methods ====================

// GetHPA retrieves horizontal pod autoscaler information
func (c *Client) GetHPA(ctx context.Context, namespace, name string) (*HPAInfo, error) {
	hpa, err := c.clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get hpa: %w", err)
	}

	status := StatusRunning

	info := &HPAInfo{
		ResourceInfo: ResourceInfo{
			Kind:              "HorizontalPodAutoscaler",
			Namespace:         hpa.Namespace,
			Name:              hpa.Name,
			Labels:            hpa.Labels,
			Annotations:       hpa.Annotations,
			Status:            status,
			CreationTimestamp: hpa.CreationTimestamp.Time,
		},
		MaxReplicas:     hpa.Spec.MaxReplicas,
		CurrentReplicas: hpa.Status.CurrentReplicas,
		DesiredReplicas: hpa.Status.DesiredReplicas,
		TargetKind:      hpa.Spec.ScaleTargetRef.Kind,
		TargetName:      hpa.Spec.ScaleTargetRef.Name,
	}

	if hpa.Spec.MinReplicas != nil {
		info.MinReplicas = *hpa.Spec.MinReplicas
	}

	// Extract CPU utilization from metrics
	for _, metric := range hpa.Spec.Metrics {
		if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
			if metric.Resource.Name == corev1.ResourceCPU && metric.Resource.Target.AverageUtilization != nil {
				info.TargetCPUUtilization = metric.Resource.Target.AverageUtilization
			}
		}
	}

	// Get current CPU utilization from status
	for _, metric := range hpa.Status.CurrentMetrics {
		if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
			if metric.Resource.Name == corev1.ResourceCPU && metric.Resource.Current.AverageUtilization != nil {
				info.CurrentCPUUtilization = metric.Resource.Current.AverageUtilization
			}
		}
	}

	return info, nil
}

// CreateHPA creates a new horizontal pod autoscaler
func (c *Client) CreateHPA(ctx context.Context, namespace string, spec HPASpec) error {
	minReplicas := spec.MinReplicas
	if minReplicas == 0 {
		minReplicas = 1
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:        spec.Name,
			Namespace:   namespace,
			Labels:      spec.Labels,
			Annotations: spec.Annotations,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       spec.TargetKind,
				Name:       spec.TargetName,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: spec.MaxReplicas,
			Metrics:     []autoscalingv2.MetricSpec{},
		},
	}

	if spec.TargetCPUUtilization != nil {
		hpa.Spec.Metrics = append(hpa.Spec.Metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: spec.TargetCPUUtilization,
				},
			},
		})
	}

	if spec.TargetMemoryUtilization != nil {
		hpa.Spec.Metrics = append(hpa.Spec.Metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: spec.TargetMemoryUtilization,
				},
			},
		})
	}

	_, err := c.clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Create(ctx, hpa, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create hpa: %w", err)
	}
	return nil
}

// UpdateHPA updates a horizontal pod autoscaler
func (c *Client) UpdateHPA(ctx context.Context, namespace string, spec HPASpec) error {
	hpa, err := c.clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get hpa for update: %w", err)
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

	if spec.MinReplicas > 0 {
		hpa.Spec.MinReplicas = &spec.MinReplicas
	}
	if spec.MaxReplicas > 0 {
		hpa.Spec.MaxReplicas = spec.MaxReplicas
	}

	// Update CPU metric if specified
	if spec.TargetCPUUtilization != nil {
		found := false
		for i, metric := range hpa.Spec.Metrics {
			if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil && metric.Resource.Name == corev1.ResourceCPU {
				hpa.Spec.Metrics[i].Resource.Target.AverageUtilization = spec.TargetCPUUtilization
				found = true
				break
			}
		}
		if !found {
			hpa.Spec.Metrics = append(hpa.Spec.Metrics, autoscalingv2.MetricSpec{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: spec.TargetCPUUtilization,
					},
				},
			})
		}
	}

	_, err = c.clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Update(ctx, hpa, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update hpa: %w", err)
	}
	return nil
}

// DeleteHPA deletes a horizontal pod autoscaler
func (c *Client) DeleteHPA(ctx context.Context, namespace, name string) error {
	err := c.clientset.AutoscalingV2().HorizontalPodAutoscalers(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete hpa: %w", err)
	}
	return nil
}

// resourceQuantityFromString parses a storage size string (e.g., "10Gi") into a resource.Quantity
func resourceQuantityFromString(size string) (resource.Quantity, error) {
	return resource.ParseQuantity(size)
}

// StreamExecOutput executes a command in a pod and streams output to provided writers
func (c *Client) StreamExecOutput(ctx context.Context, opts PodExecOptions, stdout, stderr io.Writer) error {
	// Get pod to determine container
	if opts.Container == "" {
		pod, err := c.clientset.CoreV1().Pods(opts.Namespace).Get(ctx, opts.PodName, metav1.GetOptions{})
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

	// Apply timeout if configured
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

// GetNetworkPolicy retrieves a NetworkPolicy from Kubernetes
func (c *Client) GetNetworkPolicy(ctx context.Context, namespace, name string) (*NetworkPolicy, error) {
	np, err := c.clientset.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get network policy: %w", err)
	}
	return FromK8sNetworkPolicy(np), nil
}

// ListNetworkPolicies lists NetworkPolicies in a namespace
func (c *Client) ListNetworkPolicies(ctx context.Context, namespace, labelSelector string) ([]*NetworkPolicy, error) {
	listOpts := metav1.ListOptions{}
	if labelSelector != "" {
		listOpts.LabelSelector = labelSelector
	}

	var npList *networkingv1.NetworkPolicyList
	var err error

	if namespace == "" {
		npList, err = c.clientset.NetworkingV1().NetworkPolicies(corev1.NamespaceAll).List(ctx, listOpts)
	} else {
		npList, err = c.clientset.NetworkingV1().NetworkPolicies(namespace).List(ctx, listOpts)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list network policies: %w", err)
	}

	policies := make([]*NetworkPolicy, len(npList.Items))
	for i := range npList.Items {
		policies[i] = FromK8sNetworkPolicy(&npList.Items[i])
	}
	return policies, nil
}

// CreateNetworkPolicy creates a NetworkPolicy in Kubernetes
func (c *Client) CreateNetworkPolicy(ctx context.Context, namespace string, policy *NetworkPolicy) error {
	if policy.Namespace == "" {
		policy.Namespace = namespace
	}

	k8sPolicy := ToK8sNetworkPolicy(policy)
	_, err := c.clientset.NetworkingV1().NetworkPolicies(namespace).Create(ctx, k8sPolicy, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create network policy: %w", err)
	}
	return nil
}

// UpdateNetworkPolicy updates a NetworkPolicy in Kubernetes
func (c *Client) UpdateNetworkPolicy(ctx context.Context, namespace string, policy *NetworkPolicy) error {
	if policy.Namespace == "" {
		policy.Namespace = namespace
	}

	// Get the existing policy to preserve resourceVersion
	existing, err := c.clientset.NetworkingV1().NetworkPolicies(namespace).Get(ctx, policy.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get existing network policy: %w", err)
	}

	k8sPolicy := ToK8sNetworkPolicy(policy)
	k8sPolicy.ResourceVersion = existing.ResourceVersion

	_, err = c.clientset.NetworkingV1().NetworkPolicies(namespace).Update(ctx, k8sPolicy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update network policy: %w", err)
	}
	return nil
}

// DeleteNetworkPolicy deletes a NetworkPolicy from Kubernetes
func (c *Client) DeleteNetworkPolicy(ctx context.Context, namespace, name string) error {
	err := c.clientset.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete network policy: %w", err)
	}
	return nil
}

// WatchNetworkPolicies watches for NetworkPolicy changes
func (c *Client) WatchNetworkPolicies(ctx context.Context, namespace, labelSelector string) (<-chan NetworkPolicyWatchEvent, error) {
	listOpts := metav1.ListOptions{
		Watch: true,
	}
	if labelSelector != "" {
		listOpts.LabelSelector = labelSelector
	}

	ns := namespace
	if ns == "" {
		ns = corev1.NamespaceAll
	}

	watcher, err := c.clientset.NetworkingV1().NetworkPolicies(ns).Watch(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to watch network policies: %w", err)
	}

	eventChan := make(chan NetworkPolicyWatchEvent, 100)

	go func() {
		defer close(eventChan)
		for event := range watcher.ResultChan() {
			np, ok := event.Object.(*networkingv1.NetworkPolicy)
			if !ok {
				continue
			}

			eventChan <- NetworkPolicyWatchEvent{
				Type:      string(event.Type),
				Policy:    FromK8sNetworkPolicy(np),
				Timestamp: time.Now(),
			}
		}
	}()

	return eventChan, nil
}

// NetworkPolicyWatchEvent represents a NetworkPolicy watch event
type NetworkPolicyWatchEvent struct {
	Type      string         `json:"type"` // ADDED, MODIFIED, DELETED
	Policy    *NetworkPolicy `json:"policy"`
	Timestamp time.Time      `json:"timestamp"`
}
