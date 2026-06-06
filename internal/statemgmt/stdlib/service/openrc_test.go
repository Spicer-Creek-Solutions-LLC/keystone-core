// SPDX-License-Identifier: Apache-2.0

//go:build linux

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Shared helpers (capturingRunner) live in systemd_test.go — same
// package.

// ---- rcUpdateShowHasService --------------------------------------

func TestRcUpdateShowHasService(t *testing.T) {
	t.Parallel()
	out := "            sysklogd | boot\n               nginx | default\n"
	cases := []struct {
		name string
		want bool
	}{
		{"nginx", true},    // listed (default runlevel)
		{"sysklogd", true}, // listed (boot runlevel; "|"-stripped name still matches)
		{"sshd", false},    // not listed
		{"ngin", false},    // prefix of a listed name must not match
	}
	for _, c := range cases {
		if got := rcUpdateShowHasService(out, c.name); got != c.want {
			t.Errorf("rcUpdateShowHasService(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRcUpdateShowHasService_NoPipe(t *testing.T) {
	t.Parallel()
	// A line with no "|" is taken verbatim as the service name.
	if !rcUpdateShowHasService("nginx\n", "nginx") {
		t.Error("bare-name line should match")
	}
}

func TestRcUpdateShowHasService_Empty(t *testing.T) {
	t.Parallel()
	if rcUpdateShowHasService("", "nginx") {
		t.Error("empty output should not match")
	}
}

// ---- openrcProvider arg formation --------------------------------

func newOpenrcForTest(r commandRunner, q openrcQueryFn) *openrcProvider {
	return &openrcProvider{
		rcService: "/sbin/rc-service",
		rcUpdate:  "/sbin/rc-update",
		runner:    r,
		query:     q,
	}
}

func TestOpenrcProvider_MutatingArgFormation(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newOpenrcForTest(cr.run, nil)
	ctx := context.Background()
	if err := p.Start(ctx, "nginx"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop(ctx, "nginx"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := p.Enable(ctx, "nginx"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if err := p.Disable(ctx, "nginx"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	want := [][]string{
		{"/sbin/rc-service", "nginx", "start"},
		{"/sbin/rc-service", "nginx", "stop"},
		{"/sbin/rc-update", "add", "nginx", "default"},
		{"/sbin/rc-update", "del", "nginx", "default"},
	}
	if len(cr.calls) != len(want) {
		t.Fatalf("calls = %d, want %d (%v)", len(cr.calls), len(want), cr.calls)
	}
	for i, w := range want {
		if strings.Join(cr.calls[i], " ") != strings.Join(w, " ") {
			t.Errorf("call %d = %v, want %v", i, cr.calls[i], w)
		}
	}
}

func TestOpenrcProvider_MutatingErrorPropagates(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{err: errors.New("rc-service: nginx failed to start")}
	p := newOpenrcForTest(cr.run, nil)
	if err := p.Start(context.Background(), "nginx"); err == nil || !strings.Contains(err.Error(), "failed to start") {
		t.Errorf("err = %v, want runner's underlying error", err)
	}
}

// ---- openrcProvider Lookup orchestration -------------------------

// queryResponder dispatches on the joined args so one fake can answer
// the three distinct Lookup queries.
type queryResponse struct {
	out  string
	code int
	err  error
}

func responder(t *testing.T, m map[string]queryResponse) (openrcQueryFn, *int) {
	t.Helper()
	calls := 0
	fn := func(_ context.Context, _ string, args []string) (string, int, error) {
		calls++
		key := strings.Join(args, " ")
		r, ok := m[key]
		if !ok {
			t.Errorf("unexpected query args %q", key)
			return "", 0, nil
		}
		return r.out, r.code, r.err
	}
	return fn, &calls
}

func TestOpenrcProvider_Lookup_RunningEnabled(t *testing.T) {
	t.Parallel()
	q, _ := responder(t, map[string]queryResponse{
		"--exists nginx": {code: 0},
		"nginx status":   {code: 0}, // started
		"show default":   {out: "               nginx | default\n", code: 0},
	})
	info, err := newOpenrcForTest(nil, q).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Exists || !info.Active || !info.Enabled {
		t.Errorf("got %+v, want all true", info)
	}
}

func TestOpenrcProvider_Lookup_StoppedDisabled(t *testing.T) {
	t.Parallel()
	q, _ := responder(t, map[string]queryResponse{
		"--exists nginx": {code: 0},
		"nginx status":   {code: 3}, // stopped (LSB exit 3)
		"show default":   {out: "               sshd | default\n", code: 0},
	})
	info, err := newOpenrcForTest(nil, q).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Exists {
		t.Error("Exists should be true")
	}
	if info.Active {
		t.Error("Active should be false for a stopped service")
	}
	if info.Enabled {
		t.Error("Enabled should be false when not in the default runlevel")
	}
}

func TestOpenrcProvider_Lookup_NotExists_ShortCircuits(t *testing.T) {
	t.Parallel()
	q, calls := responder(t, map[string]queryResponse{
		"--exists nginx": {code: 1}, // absent
	})
	info, err := newOpenrcForTest(nil, q).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Exists {
		t.Error("Exists should be false")
	}
	if *calls != 1 {
		t.Errorf("expected only the --exists query, got %d calls", *calls)
	}
}

func TestOpenrcProvider_Lookup_QueryErrorSurfaces(t *testing.T) {
	t.Parallel()
	// A genuine failure on each of the three query stages must surface.
	stages := []map[string]queryResponse{
		{"--exists nginx": {err: errors.New("rc-service missing")}},
		{"--exists nginx": {code: 0}, "nginx status": {err: errors.New("status blew up")}},
		{"--exists nginx": {code: 0}, "nginx status": {code: 0}, "show default": {err: errors.New("rc-update blew up")}},
	}
	for i, m := range stages {
		q, _ := responder(t, m)
		if _, err := newOpenrcForTest(nil, q).Lookup("nginx"); err == nil {
			t.Errorf("stage %d: expected error to surface", i)
		}
	}
}

// ---- realOpenrcQuery (binary side) -------------------------------

func writeStub(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rc-stub")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatalf("write stub: %v", err)
	}
	return path
}

// retryETXTBSY runs fn, retrying while it fails with ETXTBSY. That is
// the known write-then-exec race (golang/go#22315): when one parallel
// test execs a just-written stub while a sibling test forks, the fork's
// child briefly inherits the stub's still-open write fd, so the exec
// sees the file as busy. The real rc-service/rc-update binaries are
// never written-then-exec'd, so this retry is test-only and doesn't
// mask production bugs.
func retryETXTBSY(t *testing.T, fn func() error) error {
	t.Helper()
	var err error
	for i := 0; i < 100; i++ {
		if err = fn(); !errors.Is(err, syscall.ETXTBSY) {
			return err
		}
		time.Sleep(time.Millisecond)
	}
	return err
}

func TestRealOpenrcQuery_Success(t *testing.T) {
	t.Parallel()
	stub := writeStub(t, "echo 'nginx | default'\nexit 0")
	var out string
	var code int
	err := retryETXTBSY(t, func() error {
		var e error
		out, code, e = realOpenrcQuery(context.Background(), stub, []string{"show", "default"})
		return e
	})
	if err != nil {
		t.Fatalf("realOpenrcQuery: %v", err)
	}
	if code != 0 || !strings.Contains(out, "nginx") {
		t.Errorf("out=%q code=%d, want nginx + code 0", out, code)
	}
}

func TestRealOpenrcQuery_NonZeroExitNotError(t *testing.T) {
	t.Parallel()
	// A stopped service's exit 3 is a signal, not a Go error.
	stub := writeStub(t, "exit 3")
	var code int
	err := retryETXTBSY(t, func() error {
		var e error
		_, code, e = realOpenrcQuery(context.Background(), stub, []string{"nginx", "status"})
		return e
	})
	if err != nil {
		t.Fatalf("non-zero exit must not be an error: %v", err)
	}
	if code != 3 {
		t.Errorf("code = %d, want 3", code)
	}
}

func TestRealOpenrcQuery_BinaryNotFound(t *testing.T) {
	t.Parallel()
	_, _, err := realOpenrcQuery(context.Background(), "/no/such/rc-service", []string{"--exists", "nginx"})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}
