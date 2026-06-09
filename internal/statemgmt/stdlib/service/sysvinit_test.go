// SPDX-License-Identifier: Apache-2.0

//go:build linux

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeSysvFS returns fixed answers for the two filesystem checks.
type fakeSysvFS struct{ exists, enabled bool }

func (f fakeSysvFS) initScriptExists(string) bool   { return f.exists }
func (f fakeSysvFS) enabledViaSymlinks(string) bool { return f.enabled }

func newSysvForTest(mode sysvEnableMode, r commandRunner, q sysvQueryFn, fs sysvFS) *sysvinitProvider {
	bin := "/sbin/chkconfig"
	if mode == sysvUpdateRcd {
		bin = "/usr/sbin/update-rc.d"
	}
	return &sysvinitProvider{
		serviceBin: "/usr/sbin/service", enableBin: bin, mode: mode,
		runner: r, query: q, fs: fs,
	}
}

func assertCalls(t *testing.T, got, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("calls = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if !sliceEq(got[i], want[i]) {
			t.Errorf("call %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// ---- chkconfigEnabled parser -------------------------------------

func TestChkconfigEnabled(t *testing.T) {
	t.Parallel()
	out := "httpd          \t0:off\t1:off\t2:on\t3:on\t4:on\t5:on\t6:off\n" +
		"crond          \t0:off\t1:off\t2:off\t3:off\t4:off\t5:off\t6:off\n"
	cases := []struct {
		name string
		want bool
	}{
		{"httpd", true},  // on in 2-5
		{"crond", false}, // off everywhere
		{"sshd", false},  // not listed
		{"http", false},  // prefix of httpd must not match
	}
	for _, c := range cases {
		if got := chkconfigEnabled(out, c.name); got != c.want {
			t.Errorf("chkconfigEnabled(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// ---- arg formation -----------------------------------------------

func TestSysvinit_ArgFormation_Chkconfig(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newSysvForTest(sysvChkconfig, cr.run, nil, fakeSysvFS{})
	ctx := context.Background()
	for _, op := range []func(context.Context, string) error{p.Start, p.Stop, p.Enable, p.Disable} {
		if err := op(ctx, "nginx"); err != nil {
			t.Fatalf("op: %v", err)
		}
	}
	assertCalls(t, cr.calls, [][]string{
		{"/usr/sbin/service", "nginx", "start"},
		{"/usr/sbin/service", "nginx", "stop"},
		{"/sbin/chkconfig", "nginx", "on"},
		{"/sbin/chkconfig", "nginx", "off"},
	})
}

func TestSysvinit_ArgFormation_UpdateRcd(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newSysvForTest(sysvUpdateRcd, cr.run, nil, fakeSysvFS{})
	ctx := context.Background()
	if err := p.Enable(ctx, "nginx"); err != nil {
		t.Fatal(err)
	}
	if err := p.Disable(ctx, "nginx"); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, cr.calls, [][]string{
		{"/usr/sbin/update-rc.d", "nginx", "enable"},
		{"/usr/sbin/update-rc.d", "nginx", "disable"},
	})
}

// ---- Lookup orchestration ----------------------------------------

func TestSysvinit_Lookup_NotExists(t *testing.T) {
	t.Parallel()
	// No init script → not present; no queries run.
	info, err := newSysvForTest(sysvChkconfig, nil, nil, fakeSysvFS{exists: false}).Lookup("nginx")
	if err != nil {
		t.Fatal(err)
	}
	if info.Exists || info.Active || info.Enabled {
		t.Errorf("got %+v, want all false", info)
	}
}

func TestSysvinit_Lookup_Chkconfig_RunningEnabled(t *testing.T) {
	t.Parallel()
	q, _ := responder(t, map[string]queryResponse{
		"nginx status": {code: 0}, // running
		"--list nginx": {out: "nginx          \t0:off\t1:off\t2:on\t3:on\t4:on\t5:on\t6:off\n", code: 0},
	})
	info, err := newSysvForTest(sysvChkconfig, nil, sysvQueryFn(q), fakeSysvFS{exists: true}).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Exists || !info.Active || !info.Enabled {
		t.Errorf("got %+v, want all true", info)
	}
}

func TestSysvinit_Lookup_Chkconfig_StoppedUnregistered(t *testing.T) {
	t.Parallel()
	q, _ := responder(t, map[string]queryResponse{
		"nginx status": {code: 3}, // stopped (LSB exit 3)
		"--list nginx": {code: 1}, // not chkconfig-managed → enabled=false
	})
	info, err := newSysvForTest(sysvChkconfig, nil, sysvQueryFn(q), fakeSysvFS{exists: true}).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Exists || info.Active || info.Enabled {
		t.Errorf("got %+v, want exists+!active+!enabled", info)
	}
}

func TestSysvinit_Lookup_UpdateRcd_Enabled(t *testing.T) {
	t.Parallel()
	// update-rc.d mode: enabled comes from the symlink scan (fs), not a
	// query — only `service status` is queried.
	q, _ := responder(t, map[string]queryResponse{
		"nginx status": {code: 0}, // running
	})
	info, err := newSysvForTest(sysvUpdateRcd, nil, sysvQueryFn(q), fakeSysvFS{exists: true, enabled: true}).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Exists || !info.Active || !info.Enabled {
		t.Errorf("got %+v, want all true", info)
	}
}

// ---- realSysvFS over a tempdir -----------------------------------

func TestRealSysvFS(t *testing.T) {
	dir := t.TempDir()
	initd := filepath.Join(dir, "init.d")
	if err := os.MkdirAll(initd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(initd, "nginx"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rc3 := filepath.Join(dir, "rc3.d")
	if err := os.MkdirAll(rc3, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../init.d/nginx", filepath.Join(rc3, "S01nginx")); err != nil {
		t.Fatal(err)
	}
	fs := realSysvFS{initdDir: initd, etcDir: dir}

	if !fs.initScriptExists("nginx") {
		t.Error("nginx init script should exist")
	}
	if fs.initScriptExists("absent") {
		t.Error("absent init script should not exist")
	}
	if !fs.enabledViaSymlinks("nginx") {
		t.Error("nginx S-symlink should be found")
	}
	if fs.enabledViaSymlinks("other") {
		t.Error("other should not be enabled")
	}
}
