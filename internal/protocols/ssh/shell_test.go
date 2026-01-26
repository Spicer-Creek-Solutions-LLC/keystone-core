package ssh

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

type idleReader struct{}

func (idleReader) Read(p []byte) (int, error) {
	return 0, nil
}

func TestWaitForReadPause_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := wait.ForContext(ctx, 10*time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestShell_readUntil_ContextDeadline(t *testing.T) {
	shell := &Shell{stdout: idleReader{}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := shell.readUntil(ctx, "never", 2*time.Second)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded error, got %v", err)
	}
}
