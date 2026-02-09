package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
)

// Controller is the base controller interface
type Controller interface {
	// Start starts the controller
	Start(ctx context.Context) error
	// Stop stops the controller
	Stop()
}

// RemoteExecutionController reconciles RemoteExecution resources
type RemoteExecutionController struct {
	client      *Client
	config      OperatorConfig
	queue       workqueue.RateLimitingInterface //nolint:staticcheck // SA1019: workqueue.RateLimitingInterface is deprecated but requires k8s API migration
	stopCh      chan struct{}
	wg          sync.WaitGroup
	reconciling sync.Map
}

// NewRemoteExecutionController creates a new RemoteExecution controller
func NewRemoteExecutionController(client *Client, config OperatorConfig) *RemoteExecutionController {
	return &RemoteExecutionController{
		client: client,
		config: config,
		queue:  workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()), //nolint:staticcheck // SA1019: workqueue.NewRateLimitingQueue is deprecated but requires k8s API migration
		stopCh: make(chan struct{}),
	}
}

// Start starts the controller
func (c *RemoteExecutionController) Start(ctx context.Context) error {
	// Start worker goroutines
	for i := 0; i < c.config.MaxConcurrentReconciles; i++ {
		c.wg.Add(1)
		go c.worker(ctx)
	}

	// Start periodic reconciliation
	c.wg.Add(1)
	go c.periodicReconcile(ctx)

	return nil
}

// Stop stops the controller
func (c *RemoteExecutionController) Stop() {
	close(c.stopCh)
	c.queue.ShutDown()
	c.wg.Wait()
}

// worker processes items from the work queue
func (c *RemoteExecutionController) worker(ctx context.Context) {
	defer c.wg.Done()

	for c.processNextItem(ctx) {
	}
}

// processNextItem processes a single item from the queue
func (c *RemoteExecutionController) processNextItem(ctx context.Context) bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	key, ok := item.(string)
	if !ok {
		c.queue.Forget(item)
		return true
	}

	// Check if we're already reconciling this resource
	if _, loaded := c.reconciling.LoadOrStore(key, true); loaded {
		// Already reconciling, requeue
		c.queue.AddRateLimited(item)
		return true
	}
	defer c.reconciling.Delete(key)

	// Reconcile the resource
	if err := c.reconcile(ctx, key); err != nil {
		// Requeue with rate limiting
		c.queue.AddRateLimited(item)
		return true
	}

	// Success, forget the item
	c.queue.Forget(item)
	return true
}

// periodicReconcile triggers periodic reconciliation
func (c *RemoteExecutionController) periodicReconcile(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			// This would list all RemoteExecution resources and enqueue them
			// Simplified implementation - actual would use informers
		}
	}
}

// reconcile reconciles a single RemoteExecution resource
func (c *RemoteExecutionController) reconcile(ctx context.Context, key string) error {
	// In a real implementation, this would:
	// 1. Get the RemoteExecution resource from the API server
	// 2. Check if it needs execution (based on schedule or one-time)
	// 3. Execute commands in matching pods
	// 4. Update status with results

	// Simplified implementation for now
	return nil
}

// StateConfigController reconciles StateConfig resources
type StateConfigController struct {
	client      *Client
	config      OperatorConfig
	queue       workqueue.RateLimitingInterface //nolint:staticcheck // SA1019: workqueue.RateLimitingInterface is deprecated but requires k8s API migration
	stopCh      chan struct{}
	wg          sync.WaitGroup
	reconciling sync.Map
}

// NewStateConfigController creates a new StateConfig controller
func NewStateConfigController(client *Client, config OperatorConfig) *StateConfigController {
	return &StateConfigController{
		client: client,
		config: config,
		queue:  workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()), //nolint:staticcheck // SA1019: workqueue.NewRateLimitingQueue is deprecated but requires k8s API migration
		stopCh: make(chan struct{}),
	}
}

// Start starts the controller
func (c *StateConfigController) Start(ctx context.Context) error {
	// Start worker goroutines
	for i := 0; i < c.config.MaxConcurrentReconciles; i++ {
		c.wg.Add(1)
		go c.worker(ctx)
	}

	// Start periodic reconciliation
	c.wg.Add(1)
	go c.periodicReconcile(ctx)

	return nil
}

// Stop stops the controller
func (c *StateConfigController) Stop() {
	close(c.stopCh)
	c.queue.ShutDown()
	c.wg.Wait()
}

// worker processes items from the work queue
func (c *StateConfigController) worker(ctx context.Context) {
	defer c.wg.Done()

	for c.processNextItem(ctx) {
	}
}

// processNextItem processes a single item from the queue
func (c *StateConfigController) processNextItem(ctx context.Context) bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	key, ok := item.(string)
	if !ok {
		c.queue.Forget(item)
		return false
	}

	// Check if we're already reconciling this resource
	if _, loaded := c.reconciling.LoadOrStore(key, true); loaded {
		// Already reconciling, requeue
		c.queue.AddRateLimited(item)
		return true
	}
	defer c.reconciling.Delete(key)

	// Reconcile the resource
	if err := c.reconcile(ctx, key); err != nil {
		// Requeue with rate limiting
		c.queue.AddRateLimited(item)
		return true
	}

	// Success, forget the item
	c.queue.Forget(item)
	return true
}

// periodicReconcile triggers periodic reconciliation
func (c *StateConfigController) periodicReconcile(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			// This would list all StateConfig resources and enqueue them
			// Simplified implementation - actual would use informers
		}
	}
}

// reconcile reconciles a single StateConfig resource
func (c *StateConfigController) reconcile(ctx context.Context, key string) error {
	// In a real implementation, this would:
	// 1. Get the StateConfig resource from the API server
	// 2. Check for drift
	// 3. Apply state declarations to matching pods
	// 4. Update status with results

	// Simplified implementation for now
	return nil
}

// OperatorManager manages all controllers
type OperatorManager struct {
	client      *Client
	config      OperatorConfig
	controllers []Controller
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

// NewOperatorManager creates a new operator manager
func NewOperatorManager(client *Client, config OperatorConfig) *OperatorManager {
	return &OperatorManager{
		client:      client,
		config:      config,
		controllers: make([]Controller, 0),
		stopCh:      make(chan struct{}),
	}
}

// AddController adds a controller to the manager
func (m *OperatorManager) AddController(controller Controller) {
	m.controllers = append(m.controllers, controller)
}

// Start starts all controllers
func (m *OperatorManager) Start(ctx context.Context) error {
	for _, controller := range m.controllers {
		if err := controller.Start(ctx); err != nil {
			return fmt.Errorf("failed to start controller: %w", err)
		}
	}

	// Wait for context cancellation
	<-ctx.Done()
	return ctx.Err()
}

// Stop stops all controllers
func (m *OperatorManager) Stop() {
	close(m.stopCh)
	for _, controller := range m.controllers {
		controller.Stop()
	}
	m.wg.Wait()
}

// ExecuteRemoteExecution executes a RemoteExecution resource
func (c *RemoteExecutionController) ExecuteRemoteExecution(ctx context.Context, exec *RemoteExecution) error {
	// Update status to Running
	now := metav1.Now()
	exec.Status.Phase = "Running"
	exec.Status.StartTime = &now

	// List matching pods
	pods, err := c.client.ListPods(exec.Spec.Target) //nolint:contextcheck // ListPods API doesn't take context
	if err != nil {
		exec.Status.Phase = "Failed"
		exec.Status.Message = fmt.Sprintf("Failed to list pods: %v", err)
		return err
	}

	exec.Status.PodsExecuted = len(pods)

	// Execute command in each pod
	results := make([]PodExecutionResult, 0, len(pods))
	succeeded := 0
	failed := 0

	for _, pod := range pods {
		opts := PodExecOptions{
			Namespace: pod.Namespace,
			PodName:   pod.Name,
			Container: exec.Spec.Target.Container,
			Command:   exec.Spec.Command,
			Stdout:    true,
			Stderr:    true,
			Timeout:   exec.Spec.Timeout.Duration,
		}

		result, err := c.client.ExecInPod(opts) //nolint:contextcheck // ExecInPod API doesn't take context

		podResult := PodExecutionResult{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
			Duration:  metav1.Duration{Duration: result.Duration},
		}

		if err != nil || result.ExitCode != 0 {
			failed++
			podResult.ExitCode = result.ExitCode
			podResult.Error = result.Stderr
			if err != nil {
				podResult.Error = err.Error()
			}
		} else {
			succeeded++
			podResult.ExitCode = 0
			podResult.Output = result.Stdout
		}

		results = append(results, podResult)
	}

	// Update status
	exec.Status.PodsSucceeded = succeeded
	exec.Status.PodsFailed = failed
	exec.Status.Results = results

	if failed > 0 {
		exec.Status.Phase = "Failed"
		exec.Status.Message = fmt.Sprintf("%d/%d pods failed", failed, len(pods))
	} else {
		exec.Status.Phase = "Succeeded"
		exec.Status.Message = fmt.Sprintf("Successfully executed on %d pods", succeeded)
	}

	completionTime := metav1.Now()
	exec.Status.CompletionTime = &completionTime

	return nil
}
