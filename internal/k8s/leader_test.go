package k8s

import (
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

func TestDefaultLeaderElectionConfig(t *testing.T) {
	cfg := DefaultLeaderElectionConfig("kscore-system")

	if cfg.LeaseName != "kscore-operator" {
		t.Errorf("Expected lease name kscore-operator, got %s", cfg.LeaseName)
	}
	if cfg.LeaseNamespace != "kscore-system" {
		t.Errorf("Expected namespace kscore-system, got %s", cfg.LeaseNamespace)
	}
	if cfg.Identity == "" {
		t.Error("Expected non-empty identity")
	}
	if cfg.LeaseDuration != 15*time.Second {
		t.Errorf("Expected 15s lease duration, got %s", cfg.LeaseDuration)
	}
	if cfg.RenewDeadline != 10*time.Second {
		t.Errorf("Expected 10s renew deadline, got %s", cfg.RenewDeadline)
	}
	if cfg.RetryPeriod != 2*time.Second {
		t.Errorf("Expected 2s retry period, got %s", cfg.RetryPeriod)
	}
}

func TestLeaderElectionConfig_Validate(t *testing.T) {
	valid := LeaderElectionConfig{
		LeaseName:      "kscore-operator",
		LeaseNamespace: "kscore-system",
		Identity:       "pod-1",
		LeaseDuration:  15 * time.Second,
		RenewDeadline:  10 * time.Second,
		RetryPeriod:    2 * time.Second,
	}

	t.Run("valid_config", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Errorf("Expected valid, got error: %v", err)
		}
	})

	t.Run("empty_lease_name", func(t *testing.T) {
		cfg := valid
		cfg.LeaseName = ""
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for empty lease name")
		}
	})

	t.Run("empty_namespace", func(t *testing.T) {
		cfg := valid
		cfg.LeaseNamespace = ""
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for empty namespace")
		}
	})

	t.Run("empty_identity", func(t *testing.T) {
		cfg := valid
		cfg.Identity = ""
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for empty identity")
		}
	})

	t.Run("lease_duration_not_greater_than_renew", func(t *testing.T) {
		cfg := valid
		cfg.LeaseDuration = 10 * time.Second
		cfg.RenewDeadline = 10 * time.Second
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error when lease duration == renew deadline")
		}
	})

	t.Run("renew_not_greater_than_retry", func(t *testing.T) {
		cfg := valid
		cfg.RenewDeadline = 2 * time.Second
		cfg.RetryPeriod = 2 * time.Second
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error when renew deadline == retry period")
		}
	})
}

func TestNewLeaderElector(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := DefaultLeaderElectionConfig("kscore-system")

	le := NewLeaderElector(clientset, cfg)
	if le == nil {
		t.Fatal("Expected non-nil leader elector")
	}
	if le.IsLeader() {
		t.Error("Expected IsLeader()=false before Run")
	}
}
