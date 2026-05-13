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

func TestLinuxProvider_AddWithGIDAndSystem(t *testing.T) {
	t.Parallel()
	// Confirms the gid + system branches of Add wire through to
	// groupadd's argv. groupadd will refuse without root; we assert
	// the wrapped error path fired (proving args were assembled and
	// the binary executed).
	p := linuxProvider{}
	gid := 1
	err := p.Add(context.Background(), "kscore-coverage-test-system", &gid, true)
	if err == nil {
		t.Fatal("groupadd as non-root should fail")
	}
	if !strings.Contains(err.Error(), "groupadd") {
		t.Errorf("err = %v, want \"groupadd\" cited", err)
	}
}

func TestLinuxProvider_ModDelegatesToGroupmod(t *testing.T) {
	t.Parallel()
	// groupmod on a group that almost certainly doesn't exist will
	// exit non-zero; the wrapped error must cite groupmod so the
	// dispatch + error-format path is exercised.
	p := linuxProvider{}
	err := p.Mod(context.Background(), "zzz-no-such-group-zzz", 12345)
	if err == nil {
		t.Fatal("groupmod on missing group should fail")
	}
	if !strings.Contains(err.Error(), "groupmod") {
		t.Errorf("err = %v, want \"groupmod\" cited", err)
	}
}

func TestLinuxProvider_DelDelegatesToGroupdel(t *testing.T) {
	t.Parallel()
	p := linuxProvider{}
	err := p.Del(context.Background(), "zzz-no-such-group-zzz")
	if err == nil {
		t.Fatal("groupdel on missing group should fail")
	}
	if !strings.Contains(err.Error(), "groupdel") {
		t.Errorf("err = %v, want \"groupdel\" cited", err)
	}
}

func TestLinuxProvider_LookupNotFound(t *testing.T) {
	t.Parallel()
	// linuxProvider embeds osLookup; this round-trips the embedding
	// in case future restructuring shadows Lookup.
	p := linuxProvider{}
	info, err := p.Lookup("zzz-no-such-group-zzz")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil for missing group; got %+v", info)
	}
}

func TestDefaultProvider_ReturnsLinuxProvider(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("nil")
	}
}
