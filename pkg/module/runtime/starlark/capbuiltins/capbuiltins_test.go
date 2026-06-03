// SPDX-License-Identifier: Apache-2.0

package capbuiltins_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
	"go.keystone-core.io/keystone-core/pkg/module/runtime/starlark/capbuiltins"
)

// fakeFS is a minimal capability.FSHost over the real filesystem (the
// capbuiltins test can't import internal/module's osFSHost).
type fakeFS struct{}

func (fakeFS) ReadFile(p string) ([]byte, error) { return os.ReadFile(p) }
func (fakeFS) WriteFile(p string, d []byte, m uint32) error {
	return os.WriteFile(p, d, os.FileMode(m))
}
func (fakeFS) Stat(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// TestProvider_KVLogFSReadThroughRuntime exercises the shims end-to-end
// through the real Starlark runtime: a module sets/gets a KV value,
// logs, and reads a (path-scoped) file via the host.
func TestProvider_KVLogFSReadThroughRuntime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(file, []byte("hello-from-host"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	m := &manifest.Manifest{
		Name:       "test/mod",
		Version:    "0.1.0",
		Type:       manifest.TypeStarlark,
		Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{
			manifest.CapKV:     {},
			manifest.CapLog:    {},
			manifest.CapFSRead: {Paths: []string{filepath.Join(dir, "*")}},
		},
	}

	caps, err := capability.BuildCapabilities(m, capability.Hosts{FS: fakeFS{}})
	if err != nil {
		t.Fatalf("BuildCapabilities: %v", err)
	}

	rt := srt.New(srt.Config{Builtins: capbuiltins.Provider})
	script := []byte(`
def main(input):
    kv_set("k", "v")
    val, found = kv_get("k")
    log("info", "module ran")
    content = fs_read(input["path"])
    return {"val": val, "found": found, "content": content}
`)
	inst, err := rt.Init(context.Background(), m, script, caps)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer inst.Close()

	res, err := inst.Execute(context.Background(), map[string]any{"path": file})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := res.Output["val"]; got != "v" {
		t.Errorf("val = %v, want v", got)
	}
	if got := res.Output["found"]; got != true {
		t.Errorf("found = %v, want true", got)
	}
	if got := res.Output["content"]; got != "hello-from-host" {
		t.Errorf("content = %v, want hello-from-host", got)
	}
}

// TestProvider_UngrantedCapabilityAbsent confirms a capability the
// module did not request is not in the Starlark namespace (calling it
// is a NameError, not an unscoped effect).
func TestProvider_UngrantedCapabilityAbsent(t *testing.T) {
	m := &manifest.Manifest{
		Name: "test/mod", Version: "0.1.0", Type: manifest.TypeStarlark, Entrypoint: "main.star",
		Capabilities: map[string]manifest.CapabilityConfig{manifest.CapKV: {}},
	}
	caps, err := capability.BuildCapabilities(m, capability.Hosts{})
	if err != nil {
		t.Fatalf("BuildCapabilities: %v", err)
	}
	rt := srt.New(srt.Config{Builtins: capbuiltins.Provider})
	// Module references http_get, which was never granted.
	script := []byte("def main(input):\n    return {\"x\": http_get(\"http://x\")}\n")
	if _, err := rt.Init(context.Background(), m, script, caps); err == nil {
		t.Fatal("Init succeeded; want a compile error for undefined http_get")
	}
}
