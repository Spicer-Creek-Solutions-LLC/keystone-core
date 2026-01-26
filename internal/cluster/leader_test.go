package cluster

import (
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestNewLeaderElector(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		etcd     *EtcdClient
		memberID string
		wantErr  bool
	}{
		{
			name:     "nil config",
			config:   nil,
			etcd:     &EtcdClient{},
			memberID: "member-1",
			wantErr:  true,
		},
		{
			name:     "nil etcd",
			config:   &Config{ClusterName: "test"},
			etcd:     nil,
			memberID: "member-1",
			wantErr:  true,
		},
		{
			name:     "empty member ID",
			config:   &Config{ClusterName: "test"},
			etcd:     &EtcdClient{},
			memberID: "",
			wantErr:  true,
		},
		{
			name:     "valid config",
			config:   &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second},
			etcd:     &EtcdClient{},
			memberID: "member-1",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			le, err := NewLeaderElector(tt.config, tt.etcd, tt.memberID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLeaderElector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && le == nil {
				t.Error("NewLeaderElector() returned nil without error")
			}
			if !tt.wantErr && le != nil {
				if le.MemberID() != tt.memberID {
					t.Errorf("MemberID() = %v, want %v", le.MemberID(), tt.memberID)
				}
			}
		})
	}
}

func TestLeaderElector_IsLeader(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, err := NewLeaderElector(config, &EtcdClient{}, "member-1")
	if err != nil {
		t.Fatalf("Failed to create leader elector: %v", err)
	}

	// Initially not a leader
	if le.IsLeader() {
		t.Error("IsLeader() should be false initially")
	}

	// Manually set as leader
	le.mu.Lock()
	le.isLeader = true
	le.mu.Unlock()

	if !le.IsLeader() {
		t.Error("IsLeader() should be true after setting")
	}
}

func TestLeaderElector_GetLeaderID(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, err := NewLeaderElector(config, &EtcdClient{}, "member-1")
	if err != nil {
		t.Fatalf("Failed to create leader elector: %v", err)
	}

	// Initially empty
	if le.GetLeaderID() != "" {
		t.Error("GetLeaderID() should be empty initially")
	}

	// Set leader ID
	le.mu.Lock()
	le.leaderID = "member-2"
	le.mu.Unlock()

	if le.GetLeaderID() != "member-2" {
		t.Errorf("GetLeaderID() = %v, want member-2", le.GetLeaderID())
	}
}

func TestLeaderElector_Observers(t *testing.T) {
	config := &Config{ClusterName: "test", ElectionTimeout: 15 * time.Second}
	le, err := NewLeaderElector(config, &EtcdClient{}, "member-1")
	if err != nil {
		t.Fatalf("Failed to create leader elector: %v", err)
	}

	var mu sync.Mutex
	called := false
	var receivedEvent LeadershipEvent

	observer := func(event LeadershipEvent) {
		mu.Lock()
		defer mu.Unlock()
		called = true
		receivedEvent = event
	}

	le.AddObserver(observer)

	// Notify observers
	testEvent := LeadershipEvent{
		Type:      LeadershipEventElected,
		LeaderID:  "member-1",
		Timestamp: time.Now(),
	}

	le.notifyObservers(testEvent)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		return called, nil
	}); err != nil {
		t.Fatalf("Observer was not called: %v", err)
	}

	mu.Lock()
	wasCalled := called
	eventType := receivedEvent.Type
	leaderID := receivedEvent.LeaderID
	mu.Unlock()

	if !wasCalled {
		t.Error("Observer was not called")
	}

	if eventType != LeadershipEventElected {
		t.Errorf("Event type = %v, want %v", eventType, LeadershipEventElected)
	}

	if leaderID != "member-1" {
		t.Errorf("LeaderID = %v, want member-1", leaderID)
	}
}

func TestLeadershipEventType_String(t *testing.T) {
	tests := []struct {
		eventType LeadershipEventType
		want      string
	}{
		{LeadershipEventElected, "elected"},
		{LeadershipEventResigned, "resigned"},
		{LeadershipEventLost, "lost"},
		{LeadershipEventTransferred, "transferred"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if got := string(tt.eventType); got != tt.want {
				t.Errorf("LeadershipEventType = %v, want %v", got, tt.want)
			}
		})
	}
}
