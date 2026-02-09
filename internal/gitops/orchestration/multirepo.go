// Package orchestration provides multi-repository deployment coordination for GitOps.
package orchestration

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Repository represents a Git repository in the deployment
type Repository struct {
	// Name is the unique identifier for this repository
	Name string `json:"name"`

	// URL is the repository URL
	URL string `json:"url"`

	// Branch to deploy from
	Branch string `json:"branch"`

	// Path within the repository (for monorepos)
	Path string `json:"path,omitempty"`

	// DeploymentConfig for this repository
	DeploymentConfig *DeploymentConfig `json:"deployment_config,omitempty"`

	// Metadata for additional configuration
	Metadata map[string]string `json:"metadata,omitempty"`
}

// DeploymentConfig configures how a repository is deployed
type DeploymentConfig struct {
	// Type of deployment (argocd, flux, helm, kubectl, etc.)
	Type string `json:"type"`

	// Application name in the deployment system
	Application string `json:"application"`

	// Namespace to deploy to
	Namespace string `json:"namespace,omitempty"`

	// Cluster to deploy to
	Cluster string `json:"cluster,omitempty"`

	// Values for Helm or similar
	Values map[string]interface{} `json:"values,omitempty"`

	// Timeout for deployment
	Timeout time.Duration `json:"timeout"`

	// HealthCheckPath for health verification
	HealthCheckPath string `json:"health_check_path,omitempty"`

	// HealthCheckTimeout for health check
	HealthCheckTimeout time.Duration `json:"health_check_timeout,omitempty"`
}

// DeploymentGroup represents a group of repositories deployed together
type DeploymentGroup struct {
	// Name of the deployment group
	Name string `json:"name"`

	// Description of what this group represents
	Description string `json:"description,omitempty"`

	// Repositories in this group
	Repositories []*Repository `json:"repositories"`

	// Dependencies are groups that must be deployed first
	Dependencies []string `json:"dependencies,omitempty"`

	// Parallel indicates if repos in this group can be deployed in parallel
	Parallel bool `json:"parallel"`

	// StopOnFailure stops the entire orchestration if this group fails
	StopOnFailure bool `json:"stop_on_failure"`

	// RollbackOnFailure triggers rollback of the entire orchestration
	RollbackOnFailure bool `json:"rollback_on_failure"`

	// Verification to run after this group completes
	Verification *VerificationConfig `json:"verification,omitempty"`

	// Metadata for additional configuration
	Metadata map[string]string `json:"metadata,omitempty"`
}

// VerificationConfig configures verification after deployment
type VerificationConfig struct {
	// Type of verification (http, command, k8s, etc.)
	Type string `json:"type"`

	// Endpoint for HTTP verification
	Endpoint string `json:"endpoint,omitempty"`

	// Command for command verification
	Command string `json:"command,omitempty"`

	// Timeout for verification
	Timeout time.Duration `json:"timeout"`

	// RetryCount for failed verifications
	RetryCount int `json:"retry_count"`

	// RetryDelay between retries
	RetryDelay time.Duration `json:"retry_delay"`
}

// Plan defines the complete multi-repo deployment plan
type Plan struct {
	// Name of the orchestration plan
	Name string `json:"name"`

	// Description of the plan
	Description string `json:"description,omitempty"`

	// Version of the plan
	Version string `json:"version,omitempty"`

	// Groups in deployment order
	Groups []*DeploymentGroup `json:"groups"`

	// GlobalTimeout for the entire orchestration
	GlobalTimeout time.Duration `json:"global_timeout"`

	// ConcurrencyLimit limits parallel deployments
	ConcurrencyLimit int `json:"concurrency_limit"`

	// RequireApproval requires manual approval before starting
	RequireApproval bool `json:"require_approval"`

	// Approvers who can approve
	Approvers []string `json:"approvers,omitempty"`

	// Notifications configuration
	Notifications *NotificationConfig `json:"notifications,omitempty"`

	// Metadata for additional configuration
	Metadata map[string]string `json:"metadata,omitempty"`
}

// NotificationConfig configures notifications for orchestration events
type NotificationConfig struct {
	// OnStart notification
	OnStart bool `json:"on_start"`

	// OnGroupComplete notification
	OnGroupComplete bool `json:"on_group_complete"`

	// OnComplete notification
	OnComplete bool `json:"on_complete"`

	// OnFailure notification
	OnFailure bool `json:"on_failure"`

	// Channels to notify
	Channels []string `json:"channels,omitempty"`
}

// Status represents the status of an orchestration
type Status string

// StatusPending constants define the possible statuses.
const (
	StatusPending     Status = "pending"
	StatusApproved    Status = "approved"
	StatusInProgress  Status = "in_progress"
	StatusCompleted   Status = "completed"
	StatusFailed      Status = "failed"
	StatusRollingBack Status = "rolling_back"
	StatusRolledBack  Status = "rolled_back"
	StatusCancelled   Status = "cancelled"
	StatusPartialFail Status = "partial_failure"
)

// Result represents the result of an orchestration
type Result struct {
	// ID of the orchestration
	ID string `json:"id"`

	// Plan being executed
	Plan *Plan `json:"plan"`

	// Status of the orchestration
	Status Status `json:"status"`

	// StartTime of orchestration
	StartTime time.Time `json:"start_time"`

	// EndTime of orchestration
	EndTime time.Time `json:"end_time"`

	// Duration of orchestration
	Duration time.Duration `json:"duration"`

	// GroupResults in execution order
	GroupResults []*GroupResult `json:"group_results"`

	// ExecutionOrder shows the order groups were executed
	ExecutionOrder []string `json:"execution_order"`

	// FailedGroups lists groups that failed
	FailedGroups []string `json:"failed_groups,omitempty"`

	// Message contains details
	Message string `json:"message"`

	// Error if orchestration failed
	Error string `json:"error,omitempty"`

	// ApprovalInfo for manual approval
	ApprovalInfo *Approval `json:"approval_info,omitempty"`

	// RequestedBy who started the orchestration
	RequestedBy string `json:"requested_by"`

	// Reason for the orchestration
	Reason string `json:"reason,omitempty"`
}

// Approval contains approval information
type Approval struct {
	Required   bool      `json:"required"`
	Status     string    `json:"status"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	ApprovedAt time.Time `json:"approved_at,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

// GroupResult represents the result of deploying a group
type GroupResult struct {
	// GroupName being deployed
	GroupName string `json:"group_name"`

	// Status of the group deployment
	Status Status `json:"status"`

	// StartTime of group deployment
	StartTime time.Time `json:"start_time"`

	// EndTime of group deployment
	EndTime time.Time `json:"end_time"`

	// Duration of group deployment
	Duration time.Duration `json:"duration"`

	// RepoResults for each repository
	RepoResults []*RepoResult `json:"repo_results"`

	// VerificationResult if verification was run
	VerificationResult *VerificationResult `json:"verification_result,omitempty"`

	// Message contains details
	Message string `json:"message"`

	// Error if group failed
	Error string `json:"error,omitempty"`
}

// RepoResult represents the result of deploying a single repository
type RepoResult struct {
	// RepoName being deployed
	RepoName string `json:"repo_name"`

	// Revision deployed
	Revision string `json:"revision"`

	// PreviousRevision before deployment
	PreviousRevision string `json:"previous_revision,omitempty"`

	// Status of the deployment
	Status Status `json:"status"`

	// StartTime of deployment
	StartTime time.Time `json:"start_time"`

	// EndTime of deployment
	EndTime time.Time `json:"end_time"`

	// Duration of deployment
	Duration time.Duration `json:"duration"`

	// Message contains details
	Message string `json:"message"`

	// Error if deployment failed
	Error string `json:"error,omitempty"`
}

// VerificationResult represents verification outcome
type VerificationResult struct {
	Passed   bool          `json:"passed"`
	Message  string        `json:"message"`
	Duration time.Duration `json:"duration"`
	Attempts int           `json:"attempts"`
}

// Deployer defines the interface for deploying repositories
type Deployer interface {
	// Deploy deploys a repository
	Deploy(ctx context.Context, repo *Repository, revision string) (*RepoResult, error)

	// GetCurrentRevision gets the currently deployed revision
	GetCurrentRevision(ctx context.Context, repo *Repository) (string, error)

	// Rollback rolls back a repository deployment
	Rollback(ctx context.Context, repo *Repository, revision string) error

	// Type returns the deployer type
	Type() string
}

// Verifier defines the interface for running verifications
type Verifier interface {
	// Verify runs verification
	Verify(ctx context.Context, config *VerificationConfig) (*VerificationResult, error)

	// Type returns the verifier type
	Type() string
}

// Request represents a request to start orchestration
type Request struct {
	// PlanName to execute
	PlanName string `json:"plan_name"`

	// Revisions maps repository name to revision to deploy
	Revisions map[string]string `json:"revisions"`

	// RequestedBy who is starting the orchestration
	RequestedBy string `json:"requested_by"`

	// Reason for the orchestration
	Reason string `json:"reason"`

	// DryRun only validates without deploying
	DryRun bool `json:"dry_run"`

	// Force bypasses approval
	Force bool `json:"force"`

	// GroupsToSkip skips specific groups
	GroupsToSkip []string `json:"groups_to_skip,omitempty"`
}

// Orchestrator coordinates multi-repo deployments
type Orchestrator struct {
	deployers map[string]Deployer
	verifiers map[string]Verifier
	plans     map[string]*Plan
	results   map[string]*Result
	mu        sync.RWMutex
	resultsMu sync.RWMutex

	// Callbacks
	onGroupStart    []func(orchestrationID, groupName string)
	onGroupComplete []func(orchestrationID, groupName string, result *GroupResult)
	onComplete      []func(result *Result)

	// Counter for IDs
	counter int64
}

// NewOrchestrator creates a new orchestrator
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		deployers:       make(map[string]Deployer),
		verifiers:       make(map[string]Verifier),
		plans:           make(map[string]*Plan),
		results:         make(map[string]*Result),
		onGroupStart:    make([]func(string, string), 0),
		onGroupComplete: make([]func(string, string, *GroupResult), 0),
		onComplete:      make([]func(*Result), 0),
	}
}

// RegisterDeployer registers a deployer for a type
func (o *Orchestrator) RegisterDeployer(deployer Deployer) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.deployers[deployer.Type()] = deployer
}

// RegisterVerifier registers a verifier for a type
func (o *Orchestrator) RegisterVerifier(verifier Verifier) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.verifiers[verifier.Type()] = verifier
}

// RegisterPlan registers an orchestration plan
func (o *Orchestrator) RegisterPlan(plan *Plan) error {
	if plan.Name == "" {
		return fmt.Errorf("plan name is required")
	}

	// Validate the plan
	if err := o.validatePlan(plan); err != nil {
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.plans[plan.Name] = plan
	return nil
}

// GetPlan retrieves a plan by name
func (o *Orchestrator) GetPlan(name string) (*Plan, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	plan, ok := o.plans[name]
	return plan, ok
}

// validatePlan validates an orchestration plan
func (o *Orchestrator) validatePlan(plan *Plan) error {
	if len(plan.Groups) == 0 {
		return fmt.Errorf("plan must have at least one group")
	}

	groupNames := make(map[string]bool)
	for _, group := range plan.Groups {
		if group.Name == "" {
			return fmt.Errorf("group name is required")
		}
		if groupNames[group.Name] {
			return fmt.Errorf("duplicate group name: %s", group.Name)
		}
		groupNames[group.Name] = true

		if len(group.Repositories) == 0 {
			return fmt.Errorf("group %s must have at least one repository", group.Name)
		}

		// Validate dependencies exist
		for _, dep := range group.Dependencies {
			if !groupNames[dep] {
				// Check if it's defined later in the list
				found := false
				for _, g := range plan.Groups {
					if g.Name == dep {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("group %s depends on unknown group: %s", group.Name, dep)
				}
			}
		}
	}

	// Check for circular dependencies
	return o.checkCircularDependencies(plan)
}

// checkCircularDependencies detects circular dependencies in the plan
func (o *Orchestrator) checkCircularDependencies(plan *Plan) error {
	// Build dependency graph
	deps := make(map[string][]string)
	for _, group := range plan.Groups {
		deps[group.Name] = group.Dependencies
	}

	// DFS to detect cycles
	visited := make(map[string]int) // 0=unvisited, 1=visiting, 2=visited

	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		if visited[name] == 1 {
			return fmt.Errorf("circular dependency detected: %v -> %s", path, name)
		}
		if visited[name] == 2 {
			return nil
		}

		visited[name] = 1
		path = append(path, name)

		for _, dep := range deps[name] {
			if err := visit(dep, path); err != nil {
				return err
			}
		}

		visited[name] = 2
		return nil
	}

	for _, group := range plan.Groups {
		if err := visit(group.Name, nil); err != nil {
			return err
		}
	}

	return nil
}

// OnGroupStart registers a callback for when a group starts
func (o *Orchestrator) OnGroupStart(callback func(orchestrationID, groupName string)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onGroupStart = append(o.onGroupStart, callback)
}

// OnGroupComplete registers a callback for when a group completes
func (o *Orchestrator) OnGroupComplete(callback func(orchestrationID, groupName string, result *GroupResult)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onGroupComplete = append(o.onGroupComplete, callback)
}

// OnComplete registers a callback for when orchestration completes
func (o *Orchestrator) OnComplete(callback func(result *Result)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.onComplete = append(o.onComplete, callback)
}

// Execute executes an orchestration plan
func (o *Orchestrator) Execute(ctx context.Context, req *Request) (*Result, error) {
	o.mu.RLock()
	plan, ok := o.plans[req.PlanName]
	o.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("plan not found: %s", req.PlanName)
	}

	// Create result
	o.counter++
	result := &Result{
		ID:             fmt.Sprintf("orch-%d-%d", time.Now().UnixNano(), o.counter),
		Plan:           plan,
		Status:         StatusPending,
		StartTime:      time.Now(),
		GroupResults:   make([]*GroupResult, 0),
		ExecutionOrder: make([]string, 0),
		RequestedBy:    req.RequestedBy,
		Reason:         req.Reason,
	}

	// Store result
	o.resultsMu.Lock()
	o.results[result.ID] = result
	o.resultsMu.Unlock()

	// Check approval
	if plan.RequireApproval && !req.Force {
		result.ApprovalInfo = &Approval{
			Required: true,
			Status:   "pending",
		}
		return result, nil
	}

	// Execute
	return o.executeOrchestration(ctx, plan, req, result)
}

// executeOrchestration executes the orchestration
func (o *Orchestrator) executeOrchestration(ctx context.Context, plan *Plan, req *Request, result *Result) (*Result, error) {
	result.Status = StatusInProgress

	// Set up timeout
	if plan.GlobalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, plan.GlobalTimeout)
		defer cancel()
	}

	// Determine execution order (topological sort)
	order, err := o.topologicalSort(plan)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	// Create skip set
	skipSet := make(map[string]bool)
	for _, name := range req.GroupsToSkip {
		skipSet[name] = true
	}

	// Track completed groups
	completed := make(map[string]bool)

	// Execute groups in order
	for _, groupName := range order {
		// Check context
		if ctx.Err() != nil {
			result.Status = StatusFailed
			result.Error = "orchestration cancelled or timed out"
			break
		}

		// Skip if requested
		if skipSet[groupName] {
			continue
		}

		// Find group
		var group *DeploymentGroup
		for _, g := range plan.Groups {
			if g.Name == groupName {
				group = g
				break
			}
		}
		if group == nil {
			continue
		}

		// Wait for dependencies
		for _, dep := range group.Dependencies {
			if !completed[dep] && !skipSet[dep] {
				// Dependency not completed - this shouldn't happen with proper sort
				result.Status = StatusFailed
				result.Error = fmt.Sprintf("dependency %s not completed for group %s", dep, groupName)
				break
			}
		}

		// Notify callbacks
		for _, cb := range o.onGroupStart {
			cb(result.ID, groupName)
		}

		// Execute group
		var groupResult *GroupResult
		if req.DryRun {
			groupResult = &GroupResult{
				GroupName: groupName,
				Status:    StatusCompleted,
				StartTime: time.Now(),
				EndTime:   time.Now(),
				Message:   "Dry run - no actual deployment",
			}
		} else {
			groupResult = o.executeGroup(ctx, group, req.Revisions)
		}

		result.GroupResults = append(result.GroupResults, groupResult)
		result.ExecutionOrder = append(result.ExecutionOrder, groupName)

		// Notify callbacks
		for _, cb := range o.onGroupComplete {
			cb(result.ID, groupName, groupResult)
		}

		// Handle failure
		if groupResult.Status == StatusFailed {
			result.FailedGroups = append(result.FailedGroups, groupName)

			if group.StopOnFailure {
				result.Status = StatusFailed
				result.Message = fmt.Sprintf("Orchestration stopped due to failure in group: %s", groupName)
				break
			}

			if group.RollbackOnFailure {
				result.Status = StatusRollingBack
				// Trigger rollback of completed groups
				o.rollbackCompletedGroups(ctx, plan, completed, req.Revisions)
				result.Status = StatusRolledBack
				result.Message = fmt.Sprintf("Rolled back due to failure in group: %s", groupName)
				break
			}
		}

		completed[groupName] = true
	}

	// Finalize result
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if result.Status == StatusInProgress {
		if len(result.FailedGroups) > 0 {
			result.Status = StatusPartialFail
			result.Message = fmt.Sprintf("Completed with %d failed groups", len(result.FailedGroups))
		} else {
			result.Status = StatusCompleted
			result.Message = "Orchestration completed successfully"
		}
	}

	// Notify completion callbacks
	for _, cb := range o.onComplete {
		cb(result)
	}

	return result, nil
}

// executeGroup executes a single deployment group
func (o *Orchestrator) executeGroup(ctx context.Context, group *DeploymentGroup, revisions map[string]string) *GroupResult {
	result := &GroupResult{
		GroupName:   group.Name,
		Status:      StatusInProgress,
		StartTime:   time.Now(),
		RepoResults: make([]*RepoResult, 0),
	}

	if group.Parallel {
		result.RepoResults = o.executeReposParallel(ctx, group.Repositories, revisions)
	} else {
		result.RepoResults = o.executeReposSequential(ctx, group.Repositories, revisions)
	}

	// Check for failures
	hasFailure := false
	for _, rr := range result.RepoResults {
		if rr.Status == StatusFailed {
			hasFailure = true
			break
		}
	}

	// Run verification if configured and no failures
	if !hasFailure && group.Verification != nil {
		o.mu.RLock()
		verifier, ok := o.verifiers[group.Verification.Type]
		o.mu.RUnlock()

		if ok {
			verifyResult, err := verifier.Verify(ctx, group.Verification)
			if err != nil {
				result.VerificationResult = &VerificationResult{
					Passed:  false,
					Message: err.Error(),
				}
				hasFailure = true
			} else {
				result.VerificationResult = verifyResult
				if !verifyResult.Passed {
					hasFailure = true
				}
			}
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if hasFailure {
		result.Status = StatusFailed
		result.Message = "Group deployment failed"
	} else {
		result.Status = StatusCompleted
		result.Message = "Group deployed successfully"
	}

	return result
}

// executeReposParallel deploys repositories in parallel
func (o *Orchestrator) executeReposParallel(ctx context.Context, repos []*Repository, revisions map[string]string) []*RepoResult {
	results := make([]*RepoResult, len(repos))
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r *Repository) {
			defer wg.Done()
			results[idx] = o.executeRepo(ctx, r, revisions)
		}(i, repo)
	}

	wg.Wait()
	return results
}

// executeReposSequential deploys repositories sequentially
func (o *Orchestrator) executeReposSequential(ctx context.Context, repos []*Repository, revisions map[string]string) []*RepoResult {
	results := make([]*RepoResult, 0, len(repos))

	for _, repo := range repos {
		if ctx.Err() != nil {
			break
		}
		results = append(results, o.executeRepo(ctx, repo, revisions))
	}

	return results
}

// executeRepo deploys a single repository
func (o *Orchestrator) executeRepo(ctx context.Context, repo *Repository, revisions map[string]string) *RepoResult {
	result := &RepoResult{
		RepoName:  repo.Name,
		StartTime: time.Now(),
	}

	// Get revision
	revision := revisions[repo.Name]
	if revision == "" {
		revision = "HEAD"
	}
	result.Revision = revision

	// Get deployer
	deployerType := "default"
	if repo.DeploymentConfig != nil && repo.DeploymentConfig.Type != "" {
		deployerType = repo.DeploymentConfig.Type
	}

	o.mu.RLock()
	deployer, ok := o.deployers[deployerType]
	o.mu.RUnlock()

	if !ok {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("no deployer registered for type: %s", deployerType)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result
	}

	// Get current revision for rollback
	currentRev, _ := deployer.GetCurrentRevision(ctx, repo)
	result.PreviousRevision = currentRev

	// Deploy
	deployResult, err := deployer.Deploy(ctx, repo, revision)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		result.Message = "Deployment failed"
	} else {
		result.Status = deployResult.Status
		result.Message = deployResult.Message
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result
}

// rollbackCompletedGroups rolls back all completed groups
func (o *Orchestrator) rollbackCompletedGroups(ctx context.Context, plan *Plan, completed map[string]bool, revisions map[string]string) {
	// Rollback in reverse order
	for i := len(plan.Groups) - 1; i >= 0; i-- {
		group := plan.Groups[i]
		if !completed[group.Name] {
			continue
		}

		for _, repo := range group.Repositories {
			deployerType := "default"
			if repo.DeploymentConfig != nil && repo.DeploymentConfig.Type != "" {
				deployerType = repo.DeploymentConfig.Type
			}

			o.mu.RLock()
			deployer, ok := o.deployers[deployerType]
			o.mu.RUnlock()

			if ok {
				// Rollback to previous revision (empty means let deployer figure it out)
				deployer.Rollback(ctx, repo, "")
			}
		}
	}
}

// topologicalSort returns groups in dependency order
func (o *Orchestrator) topologicalSort(plan *Plan) ([]string, error) {
	// Build dependency graph
	deps := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, group := range plan.Groups {
		deps[group.Name] = group.Dependencies
		if _, ok := inDegree[group.Name]; !ok {
			inDegree[group.Name] = 0
		}
		for _, dep := range group.Dependencies {
			if _, ok := inDegree[dep]; !ok {
				inDegree[dep] = 0 // Ensure exists
			}
		}
	}

	// Calculate in-degrees
	for _, dependencies := range deps {
		for range dependencies {
			// Count how many depend on each node
		}
	}
	for name, dependencies := range deps {
		_ = name
		for _, dep := range dependencies {
			_ = dep
		}
	}

	// Kahn's algorithm
	queue := make([]string, 0)
	for _, group := range plan.Groups {
		if len(group.Dependencies) == 0 {
			queue = append(queue, group.Name)
		}
	}

	result := make([]string, 0, len(plan.Groups))
	processed := make(map[string]bool)

	for len(queue) > 0 {
		// Get next node
		name := queue[0]
		queue = queue[1:]

		if processed[name] {
			continue
		}
		processed[name] = true
		result = append(result, name)

		// Find groups that depend on this one
		for _, group := range plan.Groups {
			if processed[group.Name] {
				continue
			}

			// Check if all dependencies are processed
			allDepsProcessed := true
			for _, dep := range group.Dependencies {
				if !processed[dep] {
					allDepsProcessed = false
					break
				}
			}

			if allDepsProcessed {
				queue = append(queue, group.Name)
			}
		}
	}

	if len(result) != len(plan.Groups) {
		return nil, fmt.Errorf("could not resolve all dependencies")
	}

	return result, nil
}

// GetOrchestration retrieves an orchestration result
func (o *Orchestrator) GetOrchestration(id string) (*Result, bool) {
	o.resultsMu.RLock()
	defer o.resultsMu.RUnlock()
	result, ok := o.results[id]
	return result, ok
}

// ListOrchestrations returns all orchestration results
func (o *Orchestrator) ListOrchestrations() []*Result {
	o.resultsMu.RLock()
	defer o.resultsMu.RUnlock()

	results := make([]*Result, 0, len(o.results))
	for _, r := range o.results {
		results = append(results, r)
	}

	// Sort by start time descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].StartTime.After(results[j].StartTime)
	})

	return results
}

// CancelOrchestration cancels a running orchestration
func (o *Orchestrator) CancelOrchestration(id string) error {
	o.resultsMu.Lock()
	defer o.resultsMu.Unlock()

	result, ok := o.results[id]
	if !ok {
		return fmt.Errorf("orchestration not found: %s", id)
	}

	if result.Status != StatusInProgress && result.Status != StatusPending {
		return fmt.Errorf("orchestration is not running: %s", result.Status)
	}

	result.Status = StatusCancelled
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Message = "Orchestration cancelled"

	return nil
}

// ApproveOrchestration approves a pending orchestration
func (o *Orchestrator) ApproveOrchestration(ctx context.Context, id, approver, reason string) error {
	o.resultsMu.Lock()
	result, ok := o.results[id]
	o.resultsMu.Unlock()

	if !ok {
		return fmt.Errorf("orchestration not found: %s", id)
	}

	if result.ApprovalInfo == nil || !result.ApprovalInfo.Required {
		return fmt.Errorf("orchestration does not require approval")
	}

	if result.Status != StatusPending {
		return fmt.Errorf("orchestration is not pending: %s", result.Status)
	}

	result.ApprovalInfo.Status = "approved"
	result.ApprovalInfo.ApprovedBy = approver
	result.ApprovalInfo.ApprovedAt = time.Now()
	result.ApprovalInfo.Reason = reason
	result.Status = StatusApproved

	// Execute
	go func() {
		req := &Request{
			PlanName:    result.Plan.Name,
			Revisions:   make(map[string]string),
			RequestedBy: result.RequestedBy,
			Reason:      result.Reason,
			Force:       true,
		}
		_, _ = o.executeOrchestration(ctx, result.Plan, req, result) //nolint:errcheck // async execution
	}()

	return nil
}
