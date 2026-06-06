// SPDX-License-Identifier: Apache-2.0

//go:build linux

package user

import (
	"context"
	"strings"
	"testing"
)

func TestRunManaged_ExitError(t *testing.T) {
	t.Parallel()
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

func TestShadowProvider_AddSurfacesUseradd(t *testing.T) {
	t.Parallel()
	// useradd refuses an empty name; the wrapped error must cite
	// "useradd" so the operator can grep for the system tool.
	p := shadowProvider{}
	err := p.Add(context.Background(), AddOptions{Name: ""})
	if err == nil {
		t.Fatal("useradd with empty name should fail")
	}
	if !strings.Contains(err.Error(), "useradd") {
		t.Errorf("err = %v, want \"useradd\" cited", err)
	}
}

func TestShadowProvider_Mod_NoChangeSkipsCall(t *testing.T) {
	t.Parallel()
	// With no scalar fields populated, Mod refuses to invoke
	// usermod (avoids a no-op subprocess). Returns nil.
	p := shadowProvider{}
	if err := p.Mod(context.Background(), ModOptions{Name: "alice"}); err != nil {
		t.Errorf("empty Mod should be a no-op nil; got %v", err)
	}
}

func TestShadowProvider_Del_SurfacesUserdel(t *testing.T) {
	t.Parallel()
	// userdel refuses an empty name; the wrapped error surfaces
	// the actual system tool's complaint.
	p := shadowProvider{}
	err := p.Del(context.Background(), "", true)
	if err == nil {
		t.Fatal("userdel with empty name should fail")
	}
	if !strings.Contains(err.Error(), "userdel") {
		t.Errorf("err = %v, want \"userdel\" cited", err)
	}
}

func TestShadowProvider_SetGroups_SurfacesUsermod(t *testing.T) {
	t.Parallel()
	// usermod on a missing user fails; we exercise the formatting
	// path without needing root.
	p := shadowProvider{}
	err := p.SetGroups(context.Background(), "zzz-no-such-user-zzz", []string{"wheel"})
	if err == nil {
		t.Fatal("usermod on missing user should fail")
	}
	if !strings.Contains(err.Error(), "usermod") {
		t.Errorf("err = %v, want \"usermod\" cited", err)
	}
}

func TestShadowProvider_SetGroups_EmptySet(t *testing.T) {
	t.Parallel()
	p := shadowProvider{}
	// Empty set still calls usermod (with --groups ""); on a
	// missing user it fails, exercising the empty-list code path.
	err := p.SetGroups(context.Background(), "zzz-no-such-user-zzz", nil)
	if err == nil {
		t.Fatal("expected usermod error")
	}
}

func TestShadowProvider_Add_AllFlags(t *testing.T) {
	t.Parallel()
	// useradd refuses (likely with EEXIST or similar) without root
	// — but the arg-building path runs, exercising every flag.
	p := shadowProvider{}
	uid := 99999
	gid := 99999
	err := p.Add(context.Background(), AddOptions{
		Name:       "zzz-test-no-such-user-zzz",
		UID:        &uid,
		GID:        &gid,
		Home:       "/tmp/zzz",
		Shell:      "/bin/sh",
		Comment:    "test",
		Groups:     []string{"wheel"},
		System:     true,
		CreateHome: false,
	})
	if err == nil {
		t.Fatal("useradd should fail without root")
	}
}

func TestShadowProvider_Mod_AllScalarFlags(t *testing.T) {
	t.Parallel()
	p := shadowProvider{}
	uid := 99999
	gid := 99999
	err := p.Mod(context.Background(), ModOptions{
		Name:    "zzz-no-such-user-zzz",
		UID:     &uid,
		GID:     &gid,
		Home:    "/tmp/zzz",
		Shell:   "/bin/sh",
		Comment: "test",
	})
	if err == nil {
		t.Fatal("usermod on missing user should fail")
	}
}

func TestShadowProvider_Add_GroupByName(t *testing.T) {
	t.Parallel()
	// Hits the Group string path of Add (not GID *int).
	p := shadowProvider{}
	err := p.Add(context.Background(), AddOptions{
		Name:  "zzz-test-no-such-user-zzz",
		Group: "wheel",
	})
	if err == nil {
		t.Fatal("useradd should fail without root")
	}
}

func TestShadowProvider_Mod_GroupByName(t *testing.T) {
	t.Parallel()
	p := shadowProvider{}
	err := p.Mod(context.Background(), ModOptions{
		Name:  "zzz-no-such-user-zzz",
		Group: "wheel",
	})
	if err == nil {
		t.Fatal("usermod on missing user should fail")
	}
}
