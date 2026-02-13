package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller is the base controller interface
type Controller interface {
	// Start starts the controller
	Start(ctx context.Context) error
	// Stop stops the controller
	Stop()
}

// RemoteExecutor abstracts the execution of a RemoteExecution resource.
// Implementations must populate exec.Status before returning.
type RemoteExecutor interface {
	ExecuteRemoteExecution(ctx context.Context, exec *RemoteExecution) error
}

// RemoteExecutionController reconciles RemoteExecution resources
type RemoteExecutionController struct {
	client      *Client
	crdClient   *CRDClient
	executor    RemoteExecutor
	config      OperatorConfig
	queue       workqueue.RateLimitingInterface //nolint:staticcheck // SA1019: workqueue.RateLimitingInterface is deprecated but requires k8s API migration
	stopCh      chan struct{}
	wg          sync.WaitGroup
	reconciling sync.Map
}

// NewRemoteExecutionController creates a new RemoteExecution controller.
// If executor is nil, the controller uses its own ExecuteRemoteExecution method.
func NewRemoteExecutionController(client *Client, crdClient *CRDClient, config OperatorConfig) *RemoteExecutionController {
	c := &RemoteExecutionController{
		client:    client,
		crdClient: crdClient,
		config:    config,
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()), //nolint:staticcheck // SA1019: workqueue.NewRateLimitingQueue is deprecated but requires k8s API migration
		stopCh:    make(chan struct{}),
	}
	c.executor = c // default: use self
	return c
}

// SetExecutor overrides the RemoteExecutor used during reconciliation.
func (c *RemoteExecutionController) SetExecutor(executor RemoteExecutor) {
	c.executor = executor
}

// Enqueue adds a resource key to the work queue.
func (c *RemoteExecutionController) Enqueue(key string) {
	c.queue.Add(key)
}

// Start starts the controller
func (c *RemoteExecutionController) Start(ctx context.Context) error {
	// Start worker goroutines
	for i := 0; i < c.config.MaxConcurrentReconciles; i++ {
		c.wg.Add(1)
		go c.worker(ctx)
	}

	// Start periodic reconciliation as safety net
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

// periodicReconcile lists all RemoteExecution resources and enqueues non-terminal ones.
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
			if c.crdClient == nil {
				continue
			}
			items, err := c.crdClient.ListRemoteExecutions(ctx, c.config.Namespace)
			if err != nil {
				continue
			}
			for i := range items {
				phase := items[i].Status.Phase
				if phase == "Succeeded" || phase == "Failed" {
					continue
				}
				key := items[i].Namespace + "/" + items[i].Name
				c.queue.Add(key)
			}
		}
	}
}

// reconcile reconciles a single RemoteExecution resource
func (c *RemoteExecutionController) reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil //nolint:nilerr // bad key should not be requeued
	}

	if c.crdClient == nil {
		return nil
	}

	exec, err := c.crdClient.GetRemoteExecution(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("get remote execution: %w", err)
	}

	// Skip terminal phases
	if exec.Status.Phase == "Succeeded" || exec.Status.Phase == "Failed" {
		return nil
	}

	// Execute via the configured executor
	if execErr := c.executor.ExecuteRemoteExecution(ctx, exec); execErr != nil {
		return execErr
	}

	// Persist status
	if updateErr := c.crdClient.UpdateRemoteExecutionStatus(ctx, namespace, name, &exec.Status); updateErr != nil {
		return fmt.Errorf("update status: %w", updateErr)
	}

	return nil
}

// StateExecutor abstracts execution of a state file.
type StateExecutor interface {
	ExecuteStateFile(ctx context.Context, sc *StateConfig) error
}

// StateDriftChecker abstracts drift detection for a StateConfig resource.
type StateDriftChecker interface {
	CheckDrift(ctx context.Context, sc *StateConfig) (bool, error)
}

// StateConfigController reconciles StateConfig resources
type StateConfigController struct {
	client       *Client
	crdClient    *CRDClient
	stateExec    StateExecutor
	driftChecker StateDriftChecker
	config       OperatorConfig
	queue        workqueue.RateLimitingInterface //nolint:staticcheck // SA1019: workqueue.RateLimitingInterface is deprecated but requires k8s API migration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	reconciling  sync.Map
}

// NewStateConfigController creates a new StateConfig controller
func NewStateConfigController(client *Client, crdClient *CRDClient, config OperatorConfig) *StateConfigController {
	return &StateConfigController{
		client:    client,
		crdClient: crdClient,
		config:    config,
		queue:     workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()), //nolint:staticcheck // SA1019: workqueue.NewRateLimitingQueue is deprecated but requires k8s API migration
		stopCh:    make(chan struct{}),
	}
}

// SetStateExecutor overrides the StateExecutor used during reconciliation.
func (c *StateConfigController) SetStateExecutor(exec StateExecutor) {
	c.stateExec = exec
}

// SetDriftChecker overrides the StateDriftChecker used during periodic reconciliation.
func (c *StateConfigController) SetDriftChecker(checker StateDriftChecker) {
	c.driftChecker = checker
}

// Enqueue adds a resource key to the work queue.
func (c *StateConfigController) Enqueue(key string) {
	c.queue.Add(key)
}

// Start starts the controller
func (c *StateConfigController) Start(ctx context.Context) error {
	// Start worker goroutines
	for i := 0; i < c.config.MaxConcurrentReconciles; i++ {
		c.wg.Add(1)
		go c.worker(ctx)
	}

	// Start periodic reconciliation as safety net
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

// periodicReconcile lists all StateConfig resources, runs drift detection on Applied ones,
// and enqueues non-terminal or drifted resources.
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
			if c.crdClient == nil {
				continue
			}
			items, err := c.crdClient.ListStateConfigs(ctx, c.config.Namespace)
			if err != nil {
				continue
			}
			for i := range items {
				phase := items[i].Status.Phase

				// Run drift detection on Applied resources
				if phase == "Applied" && !items[i].Status.DriftDetected && c.driftChecker != nil {
					hasDrift, driftErr := c.driftChecker.CheckDrift(ctx, &items[i])
					if driftErr != nil {
						continue
					}
					if hasDrift {
						items[i].Status.DriftDetected = true
						_ = c.crdClient.UpdateStateConfigStatus(
							ctx, items[i].Namespace, items[i].Name, &items[i].Status,
						)
					} else {
						continue // no drift, skip
					}
				} else if phase == "Applied" && !items[i].Status.DriftDetected {
					continue
				}

				key := items[i].Namespace + "/" + items[i].Name
				c.queue.Add(key)
			}
		}
	}
}

// reconcile reconciles a single StateConfig resource
func (c *StateConfigController) reconcile(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return nil //nolint:nilerr // bad key should not be requeued
	}

	if c.crdClient == nil {
		return nil
	}

	sc, err := c.crdClient.GetStateConfig(ctx, namespace, name)
	if err != nil {
		return fmt.Errorf("get state config: %w", err)
	}

	// Skip Applied with no drift
	if sc.Status.Phase == "Applied" && !sc.Status.DriftDetected {
		return nil
	}

	if c.stateExec == nil {
		return nil
	}

	// Set phase to Applying
	sc.Status.Phase = "Applying"
	sc.Status.DriftDetected = false
	_ = c.crdClient.UpdateStateConfigStatus(ctx, namespace, name, &sc.Status)

	// Execute the state
	if execErr := c.stateExec.ExecuteStateFile(ctx, sc); execErr != nil {
		sc.Status.Phase = "Failed"
		sc.Status.Message = execErr.Error()
		_ = c.crdClient.UpdateStateConfigStatus(ctx, namespace, name, &sc.Status)
		return execErr
	}

	// Execution succeeded — update to Applied
	now := metav1.Now()
	sc.Status.Phase = "Applied"
	sc.Status.LastApplied = &now
	if err = c.crdClient.UpdateStateConfigStatus(ctx, namespace, name, &sc.Status); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	return nil
}

// OperatorManager manages all controllers and informers
type OperatorManager struct {
	client        *Client
	crdClient     *CRDClient
	informerMgr   *InformerManager
	leaderElector *LeaderElector
	config        OperatorConfig
	controllers   []Controller
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

// NewOperatorManager creates a new operator manager
func NewOperatorManager(client *Client, crdClient *CRDClient, config OperatorConfig) *OperatorManager {
	return &OperatorManager{
		client:      client,
		crdClient:   crdClient,
		config:      config,
		controllers: make([]Controller, 0),
		stopCh:      make(chan struct{}),
	}
}

// SetInformerManager sets the informer manager and wires event handlers to controllers.
func (m *OperatorManager) SetInformerManager(im *InformerManager) {
	m.informerMgr = im

	for _, ctrl := range m.controllers {
		switch c := ctrl.(type) {
		case *RemoteExecutionController:
			im.SetRemoteExecutionHandler(cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					if key, err := KeyFromObject(obj); err == nil {
						c.Enqueue(key)
					}
				},
				UpdateFunc: func(_, newObj interface{}) {
					if key, err := KeyFromObject(newObj); err == nil {
						c.Enqueue(key)
					}
				},
			})
		case *StateConfigController:
			im.SetStateConfigHandler(cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					if key, err := KeyFromObject(obj); err == nil {
						c.Enqueue(key)
					}
				},
				UpdateFunc: func(_, newObj interface{}) {
					if key, err := KeyFromObject(newObj); err == nil {
						c.Enqueue(key)
					}
				},
			})
		}
	}
}

// SetLeaderElector sets the leader elector for HA deployments.
// When set, controllers only start when this instance becomes leader.
func (m *OperatorManager) SetLeaderElector(le *LeaderElector) {
	m.leaderElector = le
}

// AddController adds a controller to the manager
func (m *OperatorManager) AddController(controller Controller) {
	m.controllers = append(m.controllers, controller)
}

// Start starts the operator. If a leader elector is set, controllers only
// start when this instance becomes the leader. Start is non-blocking — it
// launches goroutines and returns immediately.
func (m *OperatorManager) Start(ctx context.Context) error {
	startControllers := func(ctx context.Context) error {
		if m.informerMgr != nil {
			if err := m.informerMgr.Start(ctx); err != nil {
				return fmt.Errorf("start informers: %w", err)
			}
		}
		for _, controller := range m.controllers {
			if err := controller.Start(ctx); err != nil {
				return fmt.Errorf("failed to start controller: %w", err)
			}
		}
		return nil
	}

	if m.leaderElector != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.leaderElector.Run(ctx, func(leaderCtx context.Context) {
				if err := startControllers(leaderCtx); err != nil {
					fmt.Printf("operator: failed to start controllers: %v\n", err)
				}
			}, func() {
				m.stopControllers()
			})
		}()
		return nil
	}

	return startControllers(ctx)
}

func (m *OperatorManager) stopControllers() {
	for _, controller := range m.controllers {
		controller.Stop()
	}
	if m.informerMgr != nil {
		m.informerMgr.Stop()
	}
}

// Stop stops all controllers and informers
func (m *OperatorManager) Stop() {
	close(m.stopCh)
	m.stopControllers()
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
