package k8s

import (
	"context"
	"fmt"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// LeaderElectionConfig holds leader election parameters.
type LeaderElectionConfig struct {
	LeaseName      string
	LeaseNamespace string
	Identity       string
	LeaseDuration  time.Duration
	RenewDeadline  time.Duration
	RetryPeriod    time.Duration
}

// DefaultLeaderElectionConfig returns sensible defaults.
func DefaultLeaderElectionConfig(namespace string) LeaderElectionConfig {
	hostname, _ := os.Hostname()
	return LeaderElectionConfig{
		LeaseName:      "kscore-operator",
		LeaseNamespace: namespace,
		Identity:       hostname,
		LeaseDuration:  15 * time.Second,
		RenewDeadline:  10 * time.Second,
		RetryPeriod:    2 * time.Second,
	}
}

// LeaderElector wraps k8s lease-based leader election.
type LeaderElector struct {
	clientset kubernetes.Interface
	config    LeaderElectionConfig
	isLeader  bool
}

// NewLeaderElector creates a new leader elector.
func NewLeaderElector(clientset kubernetes.Interface, config LeaderElectionConfig) *LeaderElector {
	return &LeaderElector{
		clientset: clientset,
		config:    config,
	}
}

// IsLeader returns whether this instance currently holds the leader lease.
func (le *LeaderElector) IsLeader() bool {
	return le.isLeader
}

// Run starts the leader election loop. It calls onStartedLeading when this
// instance becomes leader and onStoppedLeading when it loses the lease.
// Run blocks until ctx is canceled.
func (le *LeaderElector) Run(ctx context.Context, onStartedLeading func(context.Context), onStoppedLeading func()) {
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      le.config.LeaseName,
			Namespace: le.config.LeaseNamespace,
		},
		Client: le.clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: le.config.Identity,
		},
	}

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   le.config.LeaseDuration,
		RenewDeadline:   le.config.RenewDeadline,
		RetryPeriod:     le.config.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				le.isLeader = true
				onStartedLeading(ctx)
			},
			OnStoppedLeading: func() {
				le.isLeader = false
				onStoppedLeading()
			},
			OnNewLeader: func(identity string) {
				// Could add logging here if needed
			},
		},
	})
}

// Validate checks the leader election configuration.
func (c LeaderElectionConfig) Validate() error {
	if c.LeaseName == "" {
		return fmt.Errorf("leader election lease name is required")
	}
	if c.LeaseNamespace == "" {
		return fmt.Errorf("leader election namespace is required")
	}
	if c.Identity == "" {
		return fmt.Errorf("leader election identity is required")
	}
	if c.LeaseDuration <= c.RenewDeadline {
		return fmt.Errorf("lease duration (%s) must be greater than renew deadline (%s)", c.LeaseDuration, c.RenewDeadline)
	}
	if c.RenewDeadline <= c.RetryPeriod {
		return fmt.Errorf("renew deadline (%s) must be greater than retry period (%s)", c.RenewDeadline, c.RetryPeriod)
	}
	return nil
}
