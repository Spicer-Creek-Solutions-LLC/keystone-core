package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunner_Run(t *testing.T) {
	buf := new(bytes.Buffer)
	runner := NewRunner(buf)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "bootstrap completed") {
		t.Fatalf("expected completion message, got: %s", output)
	}
}

func TestRunner_RollbackOnFailure(t *testing.T) {
	buf := new(bytes.Buffer)
	runner := NewRunner(buf)
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				_, err := state.Output.Write([]byte("rollback detect\n"))
				return err
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("boom")
			},
			Rollback: func(ctx context.Context, state *State) error {
				_, err := state.Output.Write([]byte("rollback configure\n"))
				return err
			},
		},
	}

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from failing phase")
	}

	output := buf.String()
	if !strings.Contains(output, "rollback detect") {
		t.Fatalf("expected rollback output, got: %s", output)
	}
}
