package cluster

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
	"go.etcd.io/etcd/client/v3/concurrency"
)

const (
	// leaderElectionPrefix is the etcd key prefix for leader election.
	leaderElectionPrefix = "/leader/election/"
)

// LeaderElector implements leader election using etcd.
type LeaderElector struct {
	config    *Config
	etcd      *EtcdClient
	memberID  string
	election  *concurrency.Election
	session   *concurrency.Session
	observers []LeadershipObserver
	mu        sync.RWMutex
	isLeader  bool
	leaderID  string
	stopChan  chan struct{}
	wg        sync.WaitGroup
	started   bool
}

// NewLeaderElector creates a new leader elector.
func NewLeaderElector(config *Config, etcd *EtcdClient, memberID string) (*LeaderElector, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if etcd == nil {
		return nil, fmt.Errorf("etcd client is required")
	}
	if memberID == "" {
		return nil, fmt.Errorf("member ID is required")
	}

	return &LeaderElector{
		config:   config,
		etcd:     etcd,
		memberID: memberID,
		stopChan: make(chan struct{}),
	}, nil
}

// Start starts the leader election process.
func (l *LeaderElector) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.started {
		l.mu.Unlock()
		return fmt.Errorf("leader elector already started")
	}
	l.started = true
	l.mu.Unlock()

	// Create a session for leader election with appropriate TTL
	ttl := int(l.config.ElectionTimeout.Seconds())
	if ttl < 5 {
		ttl = 15 // Default to 15 seconds
	}

	client := l.etcd.Client()
	if client == nil {
		return ErrEtcdNotConnected
	}

	session, err := concurrency.NewSession(client, concurrency.WithTTL(ttl))
	if err != nil {
		return fmt.Errorf("failed to create election session: %w", err)
	}

	l.mu.Lock()
	l.session = session
	l.election = concurrency.NewElection(session, leaderElectionPrefix+l.config.ClusterName)
	l.mu.Unlock()

	// Start watching for leader changes
	l.wg.Add(2)
	go l.watchLeader(ctx)

	// Start campaigning for leadership
	go l.campaignLoop(ctx)

	return nil
}

// Stop stops the leader election process.
func (l *LeaderElector) Stop(ctx context.Context) error {
	l.mu.Lock()
	if !l.started {
		l.mu.Unlock()
		return nil
	}
	l.started = false
	l.mu.Unlock()

	close(l.stopChan)

	// Resign if we are the leader
	if l.IsLeader() {
		_ = l.Resign(ctx) // best-effort resign on shutdown
	}

	// Close the session
	l.mu.Lock()
	if l.session != nil {
		l.session.Close()
		l.session = nil
	}
	l.mu.Unlock()

	// Wait for goroutines to finish (with timeout)
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	wait.ForSignal(done, 5*time.Second)

	return nil
}

// Campaign starts a campaign for leadership.
// Blocks until leadership is acquired or context is cancelled.
func (l *LeaderElector) Campaign(ctx context.Context) error {
	l.mu.RLock()
	election := l.election
	l.mu.RUnlock()

	if election == nil {
		return ErrLeaderElectionFailed
	}

	// Campaign for leadership
	if err := election.Campaign(ctx, l.memberID); err != nil {
		return fmt.Errorf("failed to campaign for leadership: %w", err)
	}

	// We are now the leader
	l.mu.Lock()
	wasLeader := l.isLeader
	l.isLeader = true
	previousLeader := l.leaderID
	l.leaderID = l.memberID
	l.mu.Unlock()

	if !wasLeader {
		l.notifyObservers(LeadershipEvent{
			Type:             LeadershipEventElected,
			LeaderID:         l.memberID,
			PreviousLeaderID: previousLeader,
			Timestamp:        time.Now().UTC(),
			Reason:           "won election",
		})
	}

	return nil
}

// Resign voluntarily gives up leadership.
func (l *LeaderElector) Resign(ctx context.Context) error {
	l.mu.Lock()
	if !l.isLeader {
		l.mu.Unlock()
		return nil
	}
	election := l.election
	l.mu.Unlock()

	if election == nil {
		return nil
	}

	// Resign from leadership
	if err := election.Resign(ctx); err != nil {
		return fmt.Errorf("failed to resign leadership: %w", err)
	}

	l.mu.Lock()
	l.isLeader = false
	l.mu.Unlock()

	l.notifyObservers(LeadershipEvent{
		Type:             LeadershipEventResigned,
		LeaderID:         "",
		PreviousLeaderID: l.memberID,
		Timestamp:        time.Now().UTC(),
		Reason:           "voluntary resignation",
	})

	return nil
}

// IsLeader returns true if this instance is the current leader.
func (l *LeaderElector) IsLeader() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.isLeader
}

// GetLeader returns the current leader's member ID.
func (l *LeaderElector) GetLeader(ctx context.Context) (string, error) {
	l.mu.RLock()
	election := l.election
	l.mu.RUnlock()

	if election == nil {
		return "", ErrLeaderElectionFailed
	}

	// Get the current leader
	resp, err := election.Leader(ctx)
	if err != nil {
		// Check if there's no leader
		if errors.Is(err, concurrency.ErrElectionNoLeader) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get leader: %w", err)
	}

	if len(resp.Kvs) == 0 {
		return "", nil
	}

	return string(resp.Kvs[0].Value), nil
}

// GetLeaderID returns the cached leader ID (does not query etcd).
func (l *LeaderElector) GetLeaderID() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.leaderID
}

// TransferLeadership transfers leadership to another member.
func (l *LeaderElector) TransferLeadership(ctx context.Context, targetID string) error {
	if !l.IsLeader() {
		return ErrNotLeader
	}

	// Resign to allow the target to become leader
	// Note: etcd's election doesn't have direct transfer, so we resign
	// and the target will win the next election
	if err := l.Resign(ctx); err != nil {
		return fmt.Errorf("failed to resign for transfer: %w", err)
	}

	l.notifyObservers(LeadershipEvent{
		Type:             LeadershipEventTransferred,
		LeaderID:         targetID,
		PreviousLeaderID: l.memberID,
		Timestamp:        time.Now().UTC(),
		Reason:           "leadership transfer requested",
	})

	return nil
}

// AddObserver adds an observer for leadership changes.
func (l *LeaderElector) AddObserver(observer LeadershipObserver) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.observers = append(l.observers, observer)
}

// RemoveObserver removes a leadership observer.
func (l *LeaderElector) RemoveObserver(observer LeadershipObserver) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for i, o := range l.observers {
		if &o == &observer {
			l.observers = append(l.observers[:i], l.observers[i+1:]...)
			return
		}
	}
}

// notifyObservers notifies all observers of a leadership event with panic recovery.
func (l *LeaderElector) notifyObservers(event LeadershipEvent) {
	l.mu.RLock()
	observers := make([]LeadershipObserver, len(l.observers))
	copy(observers, l.observers)
	l.mu.RUnlock()

	safeDispatchObservers(observers, event, func(o LeadershipObserver, e any) {
		o(e.(LeadershipEvent))
	})
}

// campaignLoop continuously campaigns for leadership.
func (l *LeaderElector) campaignLoop(ctx context.Context) {
	defer l.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		default:
		}

		// Try to become leader
		if err := l.Campaign(ctx); err != nil {
			// Campaign failed or was cancelled
			if wait.ForContextOrSignal(ctx, l.stopChan, time.Second) {
				// Retry after a short delay
				continue
			}
			return
		}

		// We became leader, wait until we lose leadership
		l.mu.RLock()
		session := l.session
		l.mu.RUnlock()

		if session == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		case <-session.Done():
			// Session expired, we lost leadership
			l.mu.Lock()
			wasLeader := l.isLeader
			l.isLeader = false
			l.mu.Unlock()

			if wasLeader {
				l.notifyObservers(LeadershipEvent{
					Type:             LeadershipEventLost,
					LeaderID:         "",
					PreviousLeaderID: l.memberID,
					Timestamp:        time.Now().UTC(),
					Reason:           "session expired",
				})
			}

			// Recreate session and retry
			l.mu.Lock()
			client := l.etcd.Client()
			if client == nil {
				l.mu.Unlock()
				return
			}

			ttl := int(l.config.ElectionTimeout.Seconds())
			if ttl < 5 {
				ttl = 15
			}

			newSession, err := concurrency.NewSession(client, concurrency.WithTTL(ttl))
			if err != nil {
				l.mu.Unlock()
				continue
			}

			l.session = newSession
			l.election = concurrency.NewElection(newSession, leaderElectionPrefix+l.config.ClusterName)
			l.mu.Unlock()
		}
	}
}

// watchLeader watches for leader changes.
func (l *LeaderElector) watchLeader(ctx context.Context) {
	defer l.wg.Done()

	l.mu.RLock()
	election := l.election
	l.mu.RUnlock()

	if election == nil {
		return
	}

	observe := election.Observe(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		case resp, ok := <-observe:
			if !ok {
				return
			}
			if len(resp.Kvs) > 0 {
				newLeader := string(resp.Kvs[0].Value)
				l.mu.Lock()
				previousLeader := l.leaderID
				if newLeader != l.leaderID {
					l.leaderID = newLeader
					l.isLeader = (newLeader == l.memberID)
					l.mu.Unlock()

					if newLeader != previousLeader {
						l.notifyObservers(LeadershipEvent{
							Type:             LeadershipEventElected,
							LeaderID:         newLeader,
							PreviousLeaderID: previousLeader,
							Timestamp:        time.Now().UTC(),
							Reason:           "new leader elected",
						})
					}
				} else {
					l.mu.Unlock()
				}
			}
		}
	}
}

// MemberID returns this elector's member ID.
func (l *LeaderElector) MemberID() string {
	return l.memberID
}

// WaitForLeadership blocks until this member becomes leader or context is cancelled.
func (l *LeaderElector) WaitForLeadership(ctx context.Context) error {
	for {
		if l.IsLeader() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-l.stopChan:
			return ErrShutdown
		default:
			if wait.ForContextOrSignal(ctx, l.stopChan, 100*time.Millisecond) {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrShutdown
		}
	}
}

// WaitForAnyLeader blocks until any leader is elected or context is cancelled.
func (l *LeaderElector) WaitForAnyLeader(ctx context.Context) (string, error) {
	for {
		leader, err := l.GetLeader(ctx)
		if err != nil {
			return "", err
		}
		if leader != "" {
			return leader, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-l.stopChan:
			return "", ErrShutdown
		default:
			if wait.ForContextOrSignal(ctx, l.stopChan, 100*time.Millisecond) {
				continue
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", ErrShutdown
		}
	}
}
