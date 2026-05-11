//go:build linux

package group

// This file is Linux-tagged to keep the Add/Mod/Del path's helper
// (runManaged) testable on CI without requiring root. We exercise
// the error-formatting branches against deterministic shell binaries.

import (
	"context"
	"strings"
	"testing"
)

func TestRunManaged_ExitError(t *testing.T) {
	t.Parallel()
	// /bin/false exits 1 on every Linux distro; the wrapped error
	// must surface that exit code rather than a bare "exec error."
	err := runManaged(context.Background(), "/bin/false", nil)
	if err == nil {
		t.Fatal("expected exit-1 error")
	}
	if !strings.Contains(err.Error(), "exit") {
		t.Errorf("err = %v, want \"exit\" in message", err)
	}
}

func TestRunManaged_BinaryNotFound(t *testing.T) {
	t.Parallel()
	err := runManaged(context.Background(), "/no/such/bin", []string{"arg"})
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestLinuxProvider_AddDelegatesToGroupadd(t *testing.T) {
	t.Parallel()
	// We can't actually create groups without root. But we can
	// confirm the path-resolution + error-bubbling flow by calling
	// Add with a name `groupadd` is guaranteed to reject: an empty
	// name. groupadd exits non-zero; the error must be reported
	// through runManaged's formatting.
	p := linuxProvider{}
	err := p.Add(context.Background(), "", nil, false)
	if err == nil {
		t.Fatal("groupadd with empty name should fail")
	}
	if !strings.Contains(err.Error(), "groupadd") {
		t.Errorf("err = %v, want \"groupadd\" cited", err)
	}
}
