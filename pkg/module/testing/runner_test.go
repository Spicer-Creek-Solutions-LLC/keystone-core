// SPDX-License-Identifier: Apache-2.0

package moduletest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	star "go.starlark.net/starlark"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

func baseManifest() *manifest.Manifest {
	return &manifest.Manifest{
		Name: "acme/mod", Version: "1.0.0",
		Type: manifest.TypeStarlark, Entrypoint: "main.star",
	}
}

func newModule(t *testing.T, m *manifest.Manifest, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	my, err := manifest.MarshalManifest(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), my, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func result(t *testing.T, rep *Report, name string) Result {
	t.Helper()
	for _, r := range rep.Results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no result %q in %+v", name, rep.Results)
	return Result{}
}

func TestRun_PassFailMixed(t *testing.T) {
	dir := newModule(t, baseManifest(), map[string]string{
		"main.star": "def add(a, b):\n    return a + b\ndef main(input):\n    return {}\n",
		"mod_test.star": `
def test_pass():
    assert.eq(3, add(1, 2))

def test_truthy():
    assert.true([1])
    assert.false("")

def test_fail():
    assert.eq(1, 2, "deliberate")

test_not_a_func = 5
`,
	})

	rep, err := Run(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Passed != 2 || rep.Failed != 1 || len(rep.Results) != 3 {
		t.Fatalf("got passed=%d failed=%d n=%d, want 2/1/3", rep.Passed, rep.Failed, len(rep.Results))
	}
	if !result(t, rep, "test_pass").Passed || !result(t, rep, "test_truthy").Passed {
		t.Fatal("test_pass/test_truthy should pass")
	}
	tf := result(t, rep, "test_fail")
	if tf.Passed || !errors.Is(tf.Err, ErrAssertion) {
		t.Fatalf("test_fail: passed=%v err=%v", tf.Passed, tf.Err)
	}
	if !strings.Contains(tf.Err.Error(), "deliberate") {
		t.Fatalf("test_fail err should carry the assert message: %v", tf.Err)
	}
	// non-callable test_* global skipped.
	for _, r := range rep.Results {
		if r.Name == "test_not_a_func" {
			t.Fatal("non-callable test_* must be skipped")
		}
	}
}

func TestRun_NoTestFiles(t *testing.T) {
	dir := newModule(t, baseManifest(), map[string]string{
		"main.star": "def main(input):\n    return {}\n",
	})
	rep, err := Run(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Passed != 0 || rep.Failed != 0 || len(rep.Results) != 0 {
		t.Fatalf("want empty report, got %+v", rep)
	}
}

func TestRun_TestFileCompileErrorIsolated(t *testing.T) {
	dir := newModule(t, baseManifest(), map[string]string{
		"main.star":    "def main(input):\n    return {}\n",
		"a_test.star":  "def test_ok():\n    assert.true(True)\n",
		"b_test.star":  "this is not valid starlark @@@\n",
	})
	rep, err := Run(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result(t, rep, "test_ok").Passed {
		t.Fatal("the valid file's test should still run + pass")
	}
	cr := result(t, rep, "<compile>")
	if cr.Passed || !errors.Is(cr.Err, ErrTestFile) {
		t.Fatalf("compile failure: passed=%v err=%v", cr.Passed, cr.Err)
	}
}

func TestRun_PrintCapturedInLogs(t *testing.T) {
	dir := newModule(t, baseManifest(), map[string]string{
		"main.star": "def main(input):\n    return {}\n",
		"l_test.star": `
def test_logs():
    print("hello from test")
    assert.true(True)
`,
	})
	rep, err := Run(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := result(t, rep, "test_logs")
	if len(r.Logs) != 1 || r.Logs[0] != "hello from test" {
		t.Fatalf("logs = %#v", r.Logs)
	}
	if r.Duration <= 0 {
		t.Fatalf("duration not measured: %v", r.Duration)
	}
}

func TestRun_HardErrors(t *testing.T) {
	t.Run("missing manifest", func(t *testing.T) {
		_, err := Run(context.Background(), t.TempDir(), Options{})
		if !errors.Is(err, ErrManifest) {
			t.Fatalf("err = %v, want ErrManifest", err)
		}
	})
	t.Run("invalid manifest yaml", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(":\n: bad"), 0o600)
		if _, err := Run(context.Background(), dir, Options{}); !errors.Is(err, ErrManifest) {
			t.Fatalf("err = %v, want ErrManifest", err)
		}
	})
	t.Run("missing entrypoint file", func(t *testing.T) {
		dir := newModule(t, baseManifest(), nil) // no main.star
		if _, err := Run(context.Background(), dir, Options{}); !errors.Is(err, ErrEntrypoint) {
			t.Fatalf("err = %v, want ErrEntrypoint", err)
		}
	})
	t.Run("entrypoint compile error", func(t *testing.T) {
		dir := newModule(t, baseManifest(), map[string]string{"main.star": "@@@ not starlark\n"})
		if _, err := Run(context.Background(), dir, Options{}); !errors.Is(err, ErrEntrypoint) {
			t.Fatalf("err = %v, want ErrEntrypoint", err)
		}
	})
	t.Run("unknown capability", func(t *testing.T) {
		m := baseManifest()
		m.Capabilities = map[string]manifest.CapabilityConfig{"bogus": {}}
		dir := newModule(t, m, map[string]string{"main.star": "def main(input):\n    return {}\n"})
		// Manifest.Validate rejects unknown caps -> ErrManifest.
		if _, err := Run(context.Background(), dir, Options{}); !errors.Is(err, ErrManifest) {
			t.Fatalf("err = %v, want ErrManifest", err)
		}
	})
}

func TestRun_StepLimit(t *testing.T) {
	dir := newModule(t, baseManifest(), map[string]string{
		"main.star": "def main(input):\n    return {}\n",
		"s_test.star": `
def test_loop():
    for i in range(1000000):
        pass
`,
	})
	rep, err := Run(context.Background(), dir, Options{Config: Config{MaxSteps: 500}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := result(t, rep, "test_loop")
	if r.Passed || !errors.Is(r.Err, ErrStepLimit) {
		t.Fatalf("test_loop: passed=%v err=%v, want ErrStepLimit", r.Passed, r.Err)
	}
}

func TestRun_Timeout(t *testing.T) {
	dir := newModule(t, baseManifest(), map[string]string{
		"main.star": "def main(input):\n    return {}\n",
		"t_test.star": `
def test_slow():
    for i in range(1000000000):
        pass
`,
	})
	rep, err := Run(context.Background(), dir, Options{
		Config: Config{MaxSteps: 1 << 62, DefaultTimeout: time.Millisecond},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := result(t, rep, "test_slow")
	if r.Passed || !errors.Is(r.Err, ErrTimeout) {
		t.Fatalf("test_slow: passed=%v err=%v, want ErrTimeout", r.Passed, r.Err)
	}
}

func TestRun_ManifestTimeoutHonoured(t *testing.T) {
	m := baseManifest()
	m.Limits = manifest.Limits{Timeout: "1ms"}
	dir := newModule(t, m, map[string]string{
		"main.star": "def main(input):\n    return {}\n",
		"t_test.star": `
def test_slow():
    for i in range(1000000000):
        pass
`,
	})
	rep, err := Run(context.Background(), dir, Options{Config: Config{MaxSteps: 1 << 62}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r := result(t, rep, "test_slow"); !errors.Is(r.Err, ErrTimeout) {
		t.Fatalf("manifest Limits.Timeout not honoured: %v", r.Err)
	}
}

func TestRunOne_ContextCancelled(t *testing.T) {
	// White-box: a long-running fn with a pre-cancelled context is
	// deterministic (done is never ready) -> the ctx.Err() branch.
	thread := &star.Thread{Name: "x"}
	g, err := star.ExecFileOptions(strictOptions, thread, "x.star",
		"def loop():\n    for i in range(1000000000):\n        pass\n", nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, rerr := Config{MaxSteps: 1 << 62}.runOne(ctx, "loop", g["loop"], 0)
	if !errors.Is(rerr, context.Canceled) {
		t.Fatalf("runOne err = %v, want context.Canceled", rerr)
	}
}

func TestRun_CapabilitiesKVAndLog(t *testing.T) {
	m := baseManifest()
	m.Capabilities = map[string]manifest.CapabilityConfig{
		manifest.CapKV:  {},
		manifest.CapLog: {},
	}
	dir := newModule(t, m, map[string]string{
		"main.star": "def main(input):\n    return {}\n",
		"c_test.star": `
def test_kv():
    kv.set("k", "v")
    assert.eq("v", kv.get("k"))
    kv.delete("k")
    assert.eq(None, kv.get("k"))

def test_log():
    log.info("hello", run="test")
    assert.true(True)
`,
	})
	rep, err := Run(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Failed != 0 {
		t.Fatalf("capability tests failed: %+v", rep.Results)
	}
}

func TestRun_FSWriteScopeDeniedAndAudited(t *testing.T) {
	work := t.TempDir()
	m := baseManifest()
	m.Capabilities = map[string]manifest.CapabilityConfig{
		manifest.CapFSWrite: {Paths: []string{filepath.Join(work, "**")}},
		manifest.CapFSRead:  {Paths: []string{filepath.Join(work, "**")}},
	}
	ok := filepath.Join(work, "ok.txt")
	evil := filepath.Join(t.TempDir(), "evil.txt")
	dir := newModule(t, m, map[string]string{
		"main.star": "def main(input):\n    return {}\n",
		"f_test.star": `
def test_allowed():
    fs.write("` + ok + `", "data")
    assert.eq("data", fs.read("` + ok + `"))

def test_denied():
    assert.fails(lambda: fs.write("` + evil + `", "x"))
`,
	})

	auditFile := filepath.Join(t.TempDir(), "audit.jsonl")
	rep, err := Run(context.Background(), dir, Options{
		Audit: AuditOptions{Level: "failure", Output: auditFile},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Failed != 0 {
		t.Fatalf("fs scope tests failed: %+v", rep.Results)
	}
	if b, _ := os.ReadFile(ok); string(b) != "data" {
		t.Fatalf("allowed write not performed: %q", b)
	}
	ab, err := os.ReadFile(auditFile)
	if err != nil || !strings.Contains(string(ab), `"success":false`) {
		t.Fatalf("expected a failed fs.write audit entry, got %q (err %v)", ab, err)
	}
}
