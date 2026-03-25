package cluster

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

const (
	// Fencing constants
	defaultFenceCheckInterval = 1 * time.Second
	defaultFenceTimeout       = 5 * time.Second
	defaultEpochKeyPrefix     = "/cluster/epoch/"
	defaultFenceKeyPrefix     = "/cluster/fence/"
)

// FencingMode determines how fencing operates.
type FencingMode string

// FencingMode constants define the available modes.
const (
	FencingModeStrict   FencingMode = "strict"    // Block all operations when fenced
	FencingModeReadOnly FencingMode = "read_only" // Allow reads, block writes
	FencingModeGraceful FencingMode = "graceful"  // Allow in-flight ops to complete
)

// FenceStatus represents the current fencing status.
type FenceStatus string

// FenceStatus constants define the possible statuses.
const (
	FenceStatusActive     FenceStatus = "active"     // Operating normally
	FenceStatusWarning    FenceStatus = "warning"    // Approaching fence conditions
	FenceStatusFenced     FenceStatus = "fenced"     // Fenced - operations blocked
	FenceStatusRecovering FenceStatus = "recovering" // Recovering from fence
)

// FenceReason indicates why fencing was triggered.
type FenceReason string

// FenceReason constants define the reasons.
const (
	FenceReasonNone        FenceReason = "none"
	FenceReasonLeaseLost   FenceReason = "lease_lost"
	FenceReasonQuorumLost  FenceReason = "quorum_lost"
	FenceReasonPartitioned FenceReason = "partitioned"
	FenceReasonEpochStale  FenceReason = "epoch_stale"
	FenceReasonManual      FenceReason = "manual"
	FenceReasonHealthCheck FenceReason = "health_check_failed"
)

// FenceEvent is emitted during fencing operations.
type FenceEvent struct {
	Type      FenceEventType
	Status    FenceStatus
	Reason    FenceReason
	MemberID  string
	Epoch     int64
	Timestamp time.Time
	Details   map[string]interface{}
}

// FenceEventType identifies fencing event types.
type FenceEventType string

// FenceEventFenced constants define the events.
const (
	FenceEventFenced    FenceEventType = "fenced"
	FenceEventUnfenced  FenceEventType = "unfenced"
	FenceEventEpochBump FenceEventType = "epoch_bump"
	FenceEventWarning   FenceEventType = "warning"
	FenceEventRecovery  FenceEventType = "recovery"
)

// FenceObserver is called during fence events.
type FenceObserver func(event FenceEvent)

// FenceConfig holds fencing configuration.
type FenceConfig struct {
	Mode            FencingMode
	CheckInterval   time.Duration
	FenceTimeout    time.Duration
	QuorumRequired  bool
	LeaseRequired   bool
	EpochValidation bool
	GracePeriod     time.Duration
}

// DefaultFenceConfig returns default fencing configuration.
func DefaultFenceConfig() *FenceConfig {
	return &FenceConfig{
		Mode:            FencingModeStrict,
		CheckInterval:   defaultFenceCheckInterval,
		FenceTimeout:    defaultFenceTimeout,
		QuorumRequired:  true,
		LeaseRequired:   true,
		EpochValidation: true,
		GracePeriod:     5 * time.Second,
	}
}

// FenceGuard controls operation permissions based on fencing status.
type FenceGuard struct {
	config     *FenceConfig
	etcd       *EtcdClient
	membership *MembershipManager
	health     *HealthMonitor
	localID    string

	// State
	status       FenceStatus
	reason       FenceReason
	currentEpoch int64
	leaseValid   atomic.Bool
	fencedAt     *time.Time
	observers    []FenceObserver

	// Operation tracking for graceful mode
	inFlightOps int32
	blockNewOps atomic.Bool

	mu       sync.RWMutex
	stopChan chan struct{}
	doneChan chan struct{}
	started  bool
}

// NewFenceGuard creates a new fence guard.
func NewFenceGuard(
	config *FenceConfig,
	etcd *EtcdClient,
	membership *MembershipManager,
	health *HealthMonitor,
	localID string,
) (*FenceGuard, error) {
	if config == nil {
		config = DefaultFenceConfig()
	}
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}
	if localID == "" {
		return nil, fmt.Errorf("local member ID is required")
	}

	fg := &FenceGuard{
		config:     config,
		etcd:       etcd,
		membership: membership,
		health:     health,
		localID:    localID,
		status:     FenceStatusActive,
		reason:     FenceReasonNone,
		observers:  make([]FenceObserver, 0),
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
	}

	fg.leaseValid.Store(true)

	return fg, nil
}

// Start begins fence monitoring.
func (f *FenceGuard) Start(ctx context.Context) error {
	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		return fmt.Errorf("fence guard already started")
	}
	f.started = true
	f.mu.Unlock()

	// Initialize epoch
	if err := f.initializeEpoch(ctx); err != nil {
		return err
	}

	// Start monitoring loop
	go f.monitorLoop(ctx)

	return nil
}

// Stop stops fence monitoring.
func (f *FenceGuard) Stop() error {
	f.mu.Lock()
	if !f.started {
		f.mu.Unlock()
		return nil
	}
	f.started = false
	f.mu.Unlock()

	close(f.stopChan)

	wait.ForSignal(f.doneChan, 5*time.Second)

	return nil
}

// AddObserver adds a fence event observer.
func (f *FenceGuard) AddObserver(observer FenceObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observers = append(f.observers, observer)
}

// GetStatus returns the current fence status.
func (f *FenceGuard) GetStatus() FenceStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.status
}

// GetReason returns the current fence reason.
func (f *FenceGuard) GetReason() FenceReason {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.reason
}

// GetEpoch returns the current epoch.
func (f *FenceGuard) GetEpoch() int64 {
	return atomic.LoadInt64(&f.currentEpoch)
}

// IsFenced returns true if operations should be blocked.
func (f *FenceGuard) IsFenced() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.status == FenceStatusFenced
}

// CanWrite returns true if write operations are allowed.
func (f *FenceGuard) CanWrite() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.status == FenceStatusFenced {
		return false
	}

	if f.status == FenceStatusWarning && f.config.Mode == FencingModeReadOnly {
		return false
	}

	return true
}

// CanRead returns true if read operations are allowed.
func (f *FenceGuard) CanRead() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.status == FenceStatusFenced && f.config.Mode == FencingModeStrict {
		return false
	}

	return true
}

// AcquireOperation attempts to acquire permission for an operation.
// Returns a release function if permitted, nil if fenced.
func (f *FenceGuard) AcquireOperation(ctx context.Context, isWrite bool) (func(), error) {
	// Check if new operations are blocked
	if f.blockNewOps.Load() {
		return nil, ErrFenced
	}

	// Check permissions
	if isWrite && !f.CanWrite() {
		return nil, ErrFenced
	}

	if !isWrite && !f.CanRead() {
		return nil, ErrFenced
	}

	// Track operation
	atomic.AddInt32(&f.inFlightOps, 1)

	release := func() {
		atomic.AddInt32(&f.inFlightOps, -1)
	}

	return release, nil
}

// Fence manually triggers fencing.
func (f *FenceGuard) Fence(reason FenceReason) {
	f.setFenced(reason)
}

// Unfence removes fencing if conditions allow.
func (f *FenceGuard) Unfence(ctx context.Context) error {
	// Verify we can unfence
	if f.config.QuorumRequired {
		if f.health != nil && !f.health.HasQuorum() {
			return fmt.Errorf("cannot unfence: no quorum")
		}
	}

	if f.config.LeaseRequired && !f.leaseValid.Load() {
		return fmt.Errorf("cannot unfence: lease invalid")
	}

	f.mu.Lock()
	oldStatus := f.status
	f.status = FenceStatusActive
	f.reason = FenceReasonNone
	f.fencedAt = nil
	f.blockNewOps.Store(false)
	f.mu.Unlock()

	if oldStatus == FenceStatusFenced {
		f.notifyObservers(FenceEvent{
			Type:      FenceEventUnfenced,
			Status:    FenceStatusActive,
			Reason:    FenceReasonNone,
			MemberID:  f.localID,
			Epoch:     f.GetEpoch(),
			Timestamp: time.Now(),
		})
	}

	return nil
}

// ValidateEpoch validates an operation's epoch.
func (f *FenceGuard) ValidateEpoch(epoch int64) error {
	if !f.config.EpochValidation {
		return nil
	}

	currentEpoch := f.GetEpoch()
	if epoch < currentEpoch {
		return fmt.Errorf("%w: operation epoch %d < current epoch %d", ErrStaleEpoch, epoch, currentEpoch)
	}

	return nil
}

// BumpEpoch increments the epoch (usually on leader election).
func (f *FenceGuard) BumpEpoch(ctx context.Context) (int64, error) {
	newEpoch := atomic.AddInt64(&f.currentEpoch, 1)

	// Persist to etcd
	clusterName := f.membership.GetClusterInfo().Name
	key := fmt.Sprintf("%s%s", defaultEpochKeyPrefix, clusterName)
	err := f.etcd.Put(ctx, key, []byte(fmt.Sprintf("%d", newEpoch)), 0)
	if err != nil {
		return 0, fmt.Errorf("failed to persist epoch: %w", err)
	}

	f.notifyObservers(FenceEvent{
		Type:      FenceEventEpochBump,
		Status:    f.GetStatus(),
		MemberID:  f.localID,
		Epoch:     newEpoch,
		Timestamp: time.Now(),
	})

	return newEpoch, nil
}

// initializeEpoch loads or initializes the epoch.
func (f *FenceGuard) initializeEpoch(ctx context.Context) error {
	clusterName := f.membership.GetClusterInfo().Name
	key := fmt.Sprintf("%s%s", defaultEpochKeyPrefix, clusterName)

	value, err := f.etcd.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to get epoch: %w", err)
	}

	if len(value) == 0 {
		// Initialize epoch
		atomic.StoreInt64(&f.currentEpoch, 1)
		err = f.etcd.Put(ctx, key, []byte("1"), 0)
		if err != nil {
			return fmt.Errorf("failed to initialize epoch: %w", err)
		}
	} else {
		var epoch int64
		if _, err := fmt.Sscanf(string(value), "%d", &epoch); err != nil {
			return fmt.Errorf("failed to parse epoch: %w", err)
		}
		atomic.StoreInt64(&f.currentEpoch, epoch)
	}

	return nil
}

// monitorLoop monitors fence conditions.
func (f *FenceGuard) monitorLoop(ctx context.Context) {
	defer close(f.doneChan)

	ticker := time.NewTicker(f.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.stopChan:
			return
		case <-ticker.C:
			f.checkFenceConditions(ctx)
		}
	}
}

// checkFenceConditions checks if fencing should be triggered.
func (f *FenceGuard) checkFenceConditions(ctx context.Context) {
	// Check quorum
	if f.config.QuorumRequired && f.health != nil {
		if !f.health.HasQuorum() {
			f.setFenced(FenceReasonQuorumLost)
			return
		}
	}

	// Check for partition
	if f.health != nil && f.health.IsPartitioned() {
		f.setFenced(FenceReasonPartitioned)
		return
	}

	// Check lease validity
	if f.config.LeaseRequired && !f.leaseValid.Load() {
		f.setFenced(FenceReasonLeaseLost)
		return
	}

	// Check etcd connectivity
	if !f.etcd.IsConnected() {
		f.setWarning(FenceReasonHealthCheck)
		return
	}

	// All checks passed - ensure we're active
	f.mu.Lock()
	if f.status == FenceStatusWarning || f.status == FenceStatusRecovering {
		f.status = FenceStatusActive
		f.reason = FenceReasonNone
	}
	f.mu.Unlock()
}

// setFenced sets fenced status.
func (f *FenceGuard) setFenced(reason FenceReason) {
	f.mu.Lock()
	wasActive := f.status == FenceStatusActive || f.status == FenceStatusWarning

	if f.status != FenceStatusFenced {
		f.status = FenceStatusFenced
		f.reason = reason
		now := time.Now()
		f.fencedAt = &now

		// Block new operations in graceful mode
		if f.config.Mode == FencingModeGraceful {
			f.blockNewOps.Store(true)
		}
	}
	f.mu.Unlock()

	if wasActive {
		f.notifyObservers(FenceEvent{
			Type:      FenceEventFenced,
			Status:    FenceStatusFenced,
			Reason:    reason,
			MemberID:  f.localID,
			Epoch:     f.GetEpoch(),
			Timestamp: time.Now(),
		})

		// In graceful mode, wait for in-flight operations
		if f.config.Mode == FencingModeGraceful {
			go f.waitForInFlightOps()
		}
	}
}

// setWarning sets warning status.
func (f *FenceGuard) setWarning(reason FenceReason) {
	f.mu.Lock()
	wasActive := f.status == FenceStatusActive

	if f.status == FenceStatusActive {
		f.status = FenceStatusWarning
		f.reason = reason
	}
	f.mu.Unlock()

	if wasActive {
		f.notifyObservers(FenceEvent{
			Type:      FenceEventWarning,
			Status:    FenceStatusWarning,
			Reason:    reason,
			MemberID:  f.localID,
			Epoch:     f.GetEpoch(),
			Timestamp: time.Now(),
		})
	}
}

// waitForInFlightOps waits for in-flight operations to complete.
func (f *FenceGuard) waitForInFlightOps() {
	timeout := time.NewTimer(f.config.GracePeriod)
	defer timeout.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			// Grace period expired
			return
		case <-ticker.C:
			if atomic.LoadInt32(&f.inFlightOps) == 0 {
				return
			}
		}
	}
}

// SetLeaseValid updates the lease validity status.
func (f *FenceGuard) SetLeaseValid(valid bool) {
	wasValid := f.leaseValid.Swap(valid)

	if wasValid && !valid {
		// Lease became invalid
		f.setFenced(FenceReasonLeaseLost)
	}
}

// notifyObservers notifies all fence observers with panic recovery.
func (f *FenceGuard) notifyObservers(event FenceEvent) {
	f.mu.RLock()
	observers := make([]FenceObserver, len(f.observers))
	copy(observers, f.observers)
	f.mu.RUnlock()

	safeDispatchObservers(observers, event, func(o FenceObserver, e any) {
		o(e.(FenceEvent))
	})
}

// GetStats returns fencing statistics.
func (f *FenceGuard) GetStats() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	stats := map[string]interface{}{
		"status":        f.status,
		"reason":        f.reason,
		"epoch":         f.GetEpoch(),
		"lease_valid":   f.leaseValid.Load(),
		"in_flight_ops": atomic.LoadInt32(&f.inFlightOps),
		"mode":          f.config.Mode,
	}

	if f.fencedAt != nil {
		stats["fenced_at"] = f.fencedAt
		stats["fenced_duration_ms"] = time.Since(*f.fencedAt).Milliseconds()
	}

	return stats
}

// ErrFenced is returned when an operation is blocked due to fencing.
var ErrFenced = fmt.Errorf("operation blocked: member is fenced")

// ErrStaleEpoch is returned when an operation has a stale epoch.
var ErrStaleEpoch = fmt.Errorf("stale epoch")

// FenceToken represents a token that must be validated for operations.
type FenceToken struct {
	MemberID  string
	Epoch     int64
	Timestamp time.Time
	Signature []byte
}

// NewFenceToken creates a new fence token.
func (f *FenceGuard) NewFenceToken() *FenceToken {
	return &FenceToken{
		MemberID:  f.localID,
		Epoch:     f.GetEpoch(),
		Timestamp: time.Now(),
	}
}

// ValidateFenceToken validates a fence token.
func (f *FenceGuard) ValidateFenceToken(token *FenceToken) error {
	if token == nil {
		return fmt.Errorf("nil fence token")
	}

	if token.MemberID != f.localID {
		return fmt.Errorf("fence token member mismatch")
	}

	if err := f.ValidateEpoch(token.Epoch); err != nil {
		return err
	}

	// Check token age (optional)
	if time.Since(token.Timestamp) > time.Hour {
		return fmt.Errorf("fence token expired")
	}

	return nil
}
