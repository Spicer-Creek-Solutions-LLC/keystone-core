// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/plugin"
)

// writeExec writes an executable shell script.
func writeExec(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiscovery(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeExec(t, d1, "kscore-module", `echo module`)
	writeExec(t, d1, "kscore-shadow", `echo first`)
	writeExec(t, d2, "kscore-shadow", `echo second`) // first PATH wins
	writeExec(t, d2, "kscore-other", `echo other`)
	// Noise that must be ignored:
	writeExec(t, d1, "notkscore-x", `echo x`) // wrong prefix
	if err := os.WriteFile(filepath.Join(d1, "kscore-noexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	} // not executable
	if err := os.MkdirAll(filepath.Join(d1, "kscore-dir"), 0o755); err != nil {
		t.Fatal(err)
	} // a directory
	if err := os.WriteFile(filepath.Join(d1, "kscore-"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	} // empty name after prefix

	t.Setenv("PATH", d1+string(os.PathListSeparator)+d2)
	d := plugin.New("")

	got := d.Discover()
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.Name
	}
	want := []string{"module", "other", "shadow"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("Discover names = %v, want %v", names, want)
	}
	sh, ok := d.Lookup("shadow")
	if !ok || filepath.Dir(sh.Path) != d1 {
		t.Fatalf("shadow = %+v (want from %s — first PATH wins)", sh, d1)
	}
	if _, ok := d.Lookup("nope"); ok {
		t.Fatal("Lookup(nope) should be false")
	}

	// Cache: a new binary is invisible until Refresh.
	writeExec(t, d2, "kscore-late", `echo late`)
	if _, ok := d.Lookup("late"); ok {
		t.Fatal("cache should hide a newly-added plugin")
	}
	d.Refresh()
	if _, ok := d.Lookup("late"); !ok {
		t.Fatal("Refresh should pick up the new plugin")
	}
}

func TestExecutor(t *testing.T) {
	dir := t.TempDir()
	args := writeExec(t, dir, "kscore-args", `echo "got:$1:$2"`)
	cat := writeExec(t, dir, "kscore-cat", `cat`)
	fail := writeExec(t, dir, "kscore-fail", `exit 3`)
	sleep := writeExec(t, dir, "kscore-sleep", `sleep 30`)

	var e plugin.Executor
	var out, errb bytes.Buffer

	code, err := e.Execute(context.Background(), plugin.Plugin{Name: "args", Path: args},
		[]string{"a", "b"}, nil, &out, &errb)
	if err != nil || code != 0 || strings.TrimSpace(out.String()) != "got:a:b" {
		t.Fatalf("args = %d %q %v", code, out.String(), err)
	}

	out.Reset()
	code, err = e.Execute(context.Background(), plugin.Plugin{Name: "cat", Path: cat},
		nil, strings.NewReader("piped-stdin"), &out, &errb)
	if err != nil || code != 0 || out.String() != "piped-stdin" {
		t.Fatalf("stdin pipe = %d %q %v", code, out.String(), err)
	}

	code, err = e.Execute(context.Background(), plugin.Plugin{Name: "fail", Path: fail},
		nil, nil, &out, &errb)
	if err != nil || code != 3 {
		t.Fatalf("exit code = %d, %v (want 3, nil — plugin chose the code)", code, err)
	}

	if code, err := e.Execute(context.Background(),
		plugin.Plugin{Name: "missing", Path: filepath.Join(dir, "kscore-nope")},
		nil, nil, &out, &errb); code != -1 || err == nil {
		t.Fatalf("spawn failure = %d, %v (want -1, err)", code, err)
	}
	if _, err := (plugin.Executor{}).Execute(context.Background(), plugin.Plugin{}, nil, nil, &out, &errb); err == nil {
		t.Fatal("empty path: want ErrPluginNotFound")
	}

	// ctx cancellation kills the child promptly.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = e.Execute(ctx, plugin.Plugin{Name: "sleep", Path: sleep}, nil, nil, &out, &errb)
	if time.Since(start) > 5*time.Second {
		t.Fatal("ctx cancel did not kill the child")
	}
}
