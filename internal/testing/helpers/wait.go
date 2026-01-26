package helpers

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// WaitForCondition polls until condition returns true or the context is done.
func WaitForCondition(ctx context.Context, interval time.Duration, condition func() (bool, error)) error {
	return wait.ForCondition(ctx, interval, condition)
}

// WaitForTimeout polls until condition returns true or the timeout expires.
func WaitForTimeout(timeout, interval time.Duration, condition func() (bool, error)) error {
	return wait.ForTimeout(timeout, interval, condition)
}

// FreePort returns an available TCP port on localhost.
// The port is obtained by binding to port 0 and then closing the listener.
// Note: There's a small race window between closing and actual use.
func FreePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

// FreePorts returns n available TCP ports on localhost.
func FreePorts(t *testing.T, n int) []int {
	t.Helper()

	ports := make([]int, n)
	listeners := make([]net.Listener, n)

	// Acquire all ports first to avoid getting the same port twice
	for i := 0; i < n; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			// Clean up already acquired listeners
			for j := 0; j < i; j++ {
				listeners[j].Close()
			}
			t.Fatalf("failed to get free port: %v", err)
		}
		listeners[i] = listener
		ports[i] = listener.Addr().(*net.TCPAddr).Port
	}

	// Close all listeners
	for _, l := range listeners {
		l.Close()
	}

	return ports
}
