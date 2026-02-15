package saga_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/saga"
)

// ExampleUpgradeSaga demonstrates the saga pattern applied to an upgrade
// workflow with compensating transactions. Each step can share data and
// failures trigger reverse compensation.
func TestExample_UpgradeSaga_Success(t *testing.T) {
	log := saga.NewMemoryLog()
	coord := saga.New("upgrade", saga.WithLog(log))

	coord.AddStep(saga.Step{
		Name: "validate",
		Action: func(_ context.Context, data map[string]any) error {
			data["from_version"] = "1.0.0"
			data["to_version"] = "2.0.0"
			return nil
		},
	})

	coord.AddStep(saga.Step{
		Name: "backup",
		Action: func(_ context.Context, data map[string]any) error {
			data["backup_path"] = "/var/backups/kscore-1.0.0.tar.gz"
			return nil
		},
		Compensate: func(_ context.Context, data map[string]any) error {
			// Clean up backup on rollback.
			delete(data, "backup_path")
			return nil
		},
	})

	coord.AddStep(saga.Step{
		Name: "upgrade_nodes",
		Action: func(_ context.Context, data map[string]any) error {
			bp, ok := data["backup_path"].(string)
			if !ok || bp == "" {
				return errors.New("backup_path not set")
			}
			data["nodes_upgraded"] = 3
			return nil
		},
		Compensate: func(_ context.Context, data map[string]any) error {
			// Restore from backup.
			bp := data["backup_path"].(string)
			_ = bp // would call restoreFromBackup(bp)
			delete(data, "nodes_upgraded")
			return nil
		},
	})

	coord.AddStep(saga.Step{
		Name: "verify",
		Action: func(_ context.Context, data map[string]any) error {
			n, _ := data["nodes_upgraded"].(int)
			if n != 3 {
				return fmt.Errorf("expected 3 nodes upgraded, got %d", n)
			}
			return nil
		},
	})

	exec, err := coord.Execute(context.Background(), "upgrade-001", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != saga.StatusCompleted {
		t.Errorf("got status %q, want %q", exec.Status, saga.StatusCompleted)
	}
	if exec.Data["to_version"] != "2.0.0" {
		t.Errorf("to_version: got %v, want 2.0.0", exec.Data["to_version"])
	}
}

// TestExample_UpgradeSaga_FailureWithCompensation shows what happens when
// the verify step fails — upgrade_nodes and backup are compensated in reverse.
func TestExample_UpgradeSaga_FailureWithCompensation(t *testing.T) {
	var compensated []string
	log := saga.NewMemoryLog()
	coord := saga.New("upgrade", saga.WithLog(log))

	coord.AddStep(saga.Step{
		Name: "validate",
		Action: func(_ context.Context, data map[string]any) error {
			data["from_version"] = "1.0.0"
			return nil
		},
	})

	coord.AddStep(saga.Step{
		Name: "backup",
		Action: func(_ context.Context, data map[string]any) error {
			data["backup_path"] = "/var/backups/kscore-1.0.0.tar.gz"
			return nil
		},
		Compensate: func(_ context.Context, _ map[string]any) error {
			compensated = append(compensated, "backup")
			return nil
		},
	})

	coord.AddStep(saga.Step{
		Name: "upgrade_nodes",
		Action: func(_ context.Context, data map[string]any) error {
			data["nodes_upgraded"] = 3
			return nil
		},
		Compensate: func(_ context.Context, data map[string]any) error {
			compensated = append(compensated, "upgrade_nodes")
			return nil
		},
	})

	coord.AddStep(saga.Step{
		Name: "verify",
		Action: func(_ context.Context, _ map[string]any) error {
			return errors.New("health check failed: 1 node unhealthy")
		},
	})

	exec, err := coord.Execute(context.Background(), "upgrade-002", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if exec.Status != saga.StatusCompensated {
		t.Errorf("got status %q, want %q", exec.Status, saga.StatusCompensated)
	}

	// Compensation runs in reverse: upgrade_nodes first, then backup.
	if len(compensated) != 2 {
		t.Fatalf("expected 2 compensations, got %d", len(compensated))
	}
	if compensated[0] != "upgrade_nodes" {
		t.Errorf("first compensated: got %q, want %q", compensated[0], "upgrade_nodes")
	}
	if compensated[1] != "backup" {
		t.Errorf("second compensated: got %q, want %q", compensated[1], "backup")
	}

	// Verify step statuses.
	if exec.Steps[0].Status != saga.StepCompleted {
		t.Errorf("validate: got %q, want completed", exec.Steps[0].Status)
	}
	if exec.Steps[1].Status != saga.StepCompensated {
		t.Errorf("backup: got %q, want compensated", exec.Steps[1].Status)
	}
	if exec.Steps[2].Status != saga.StepCompensated {
		t.Errorf("upgrade_nodes: got %q, want compensated", exec.Steps[2].Status)
	}
	if exec.Steps[3].Status != saga.StepFailed {
		t.Errorf("verify: got %q, want failed", exec.Steps[3].Status)
	}
}
