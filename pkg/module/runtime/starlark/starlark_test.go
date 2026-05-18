package starlark_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	star "go.starlark.net/starlark"

	"go.keystone-core.io/keystone-core/pkg/module/loader"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
)

func mani(t *testing.T, limitTimeout string) *manifest.Manifest {
	t.Helper()
	return &manifest.Manifest{
		Name: "acme/widget", Version: "1.0.0", Type: manifest.TypeStarlark,
		Entrypoint: "main.star",
		Limits:     manifest.Limits{Timeout: limitTimeout},
	}
}

func initInst(t *testing.T, rt *srt.Runtime, src string) (loader.Instance, error) {
	t.Helper()
	return rt.Init(context.Background(), mani(t, ""), []byte(src), nil)
}

func TestExecute_HappyPath(t *testing.T) {
	rt := srt.New(srt.Config{})
	src := `
def main(input):
    print("hello from module")
    return {"sum": input["a"] + input["b"], "name": input["name"], "items": [1, 2, 3]}
`
	inst, err := initInst(t, rt, src)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	out, err := inst.Execute(context.Background(), map[string]any{
		"a": 2, "b": 3, "name": "x",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Output["sum"].(int64) != 5 || out.Output["name"] != "x" {
		t.Fatalf("output = %+v", out.Output)
	}
	items, ok := out.Output["items"].([]any)
	if !ok || len(items) != 3 || items[0].(int64) != 1 {
		t.Fatalf("items = %+v", out.Output["items"])
	}
	if len(out.Logs) != 1 || out.Logs[0] != "hello from module" {
		t.Fatalf("logs = %v", out.Logs)
	}
	if err := inst.Close(); err != nil || inst.Close() != nil {
		t.Fatalf("Close not idempotent: %v", err)
	}
}

func TestExecute_NonMapResultWrapped(t *testing.T) {
	rt := srt.New(srt.Config{})
	inst, err := initInst(t, rt, "def main(input):\n    return input['x'] * 2\n")
	if err != nil {
		t.Fatal(err)
	}
	out, err := inst.Execute(context.Background(), map[string]any{"x": 21})
	if err != nil || out.Output["result"].(int64) != 42 {
		t.Fatalf("wrapped result = %+v, %v", out.Output, err)
	}
}

func TestInit_CompileAndNoMain(t *testing.T) {
	rt := srt.New(srt.Config{})
	if _, err := initInst(t, rt, "def main(:\n  pass\n"); !errors.Is(err, srt.ErrCompile) {
		t.Fatalf("syntax error = %v, want ErrCompile", err)
	}
	if _, err := initInst(t, rt, "x = 1\n"); !errors.Is(err, srt.ErrNoMain) {
		t.Fatalf("no main = %v, want ErrNoMain", err)
	}
	if _, err := initInst(t, rt, "main = 5\n"); !errors.Is(err, srt.ErrNoMain) {
		t.Fatalf("non-callable main = %v, want ErrNoMain", err)
	}
}

func TestExecute_RuntimeError(t *testing.T) {
	rt := srt.New(srt.Config{})
	inst, err := initInst(t, rt, "def main(input):\n    return 1 // 0\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = inst.Execute(context.Background(), nil)
	if !errors.Is(err, srt.ErrExec) {
		t.Fatalf("div0 = %v, want ErrExec", err)
	}
}

func TestExecute_StepLimit(t *testing.T) {
	rt := srt.New(srt.Config{MaxSteps: 5000})
	src := `
def main(input):
    s = 0
    for x in range(100000000):
        s += x
    return {"s": s}
`
	inst, err := initInst(t, rt, src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Execute(context.Background(), nil); !errors.Is(err, srt.ErrStepLimit) {
		t.Fatalf("step limit = %v, want ErrStepLimit", err)
	}
}

func TestExecute_Timeout(t *testing.T) {
	// Huge step budget so the wall clock wins, not the step cap.
	rt := srt.New(srt.Config{MaxSteps: 1 << 60, DefaultTimeout: 50 * time.Millisecond})
	src := `
def main(input):
    s = 0
    for x in range(100000000):
        s += x
    return {"s": s}
`
	inst, err := initInst(t, rt, src)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := inst.Execute(context.Background(), nil); !errors.Is(err, srt.ErrTimeout) {
		t.Fatalf("timeout = %v, want ErrTimeout", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout did not fire promptly")
	}
}

func TestExecute_ContextCancelled(t *testing.T) {
	rt := srt.New(srt.Config{MaxSteps: 1 << 60})
	inst, err := initInst(t, rt, "def main(input):\n    s=0\n    for x in range(100000000):\n        s+=x\n    return {}\n")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	if _, err := inst.Execute(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v, want context.Canceled", err)
	}
}

func TestDeterministic_NoTimeOrRandom(t *testing.T) {
	rt := srt.New(srt.Config{})
	// `time` / `random` are not predeclared → resolve error at Init.
	for _, src := range []string{
		"def main(input):\n    return time.now()\n",
		"def main(input):\n    return random.random()\n",
	} {
		if _, err := initInst(t, rt, src); !errors.Is(err, srt.ErrCompile) {
			t.Fatalf("non-deterministic builtin = %v, want ErrCompile", err)
		}
	}
}

func TestBuiltinProvider_Seam(t *testing.T) {
	provider := func(_ map[string]any) (star.StringDict, error) {
		double := star.NewBuiltin("double", func(_ *star.Thread, _ *star.Builtin, args star.Tuple, _ []star.Tuple) (star.Value, error) {
			var n int
			if err := star.UnpackArgs("double", args, nil, "n", &n); err != nil {
				return nil, err
			}
			return star.MakeInt(n * 2), nil
		})
		return star.StringDict{"double": double}, nil
	}
	rt := srt.New(srt.Config{Builtins: provider})
	inst, err := initInst(t, rt, "def main(input):\n    return {\"v\": double(input[\"n\"])}\n")
	if err != nil {
		t.Fatalf("Init with provider: %v", err)
	}
	out, err := inst.Execute(context.Background(), map[string]any{"n": 21})
	if err != nil || out.Output["v"].(int64) != 42 {
		t.Fatalf("provider builtin = %+v, %v", out.Output, err)
	}

	// A provider error aborts Init.
	bad := srt.New(srt.Config{Builtins: func(map[string]any) (star.StringDict, error) {
		return nil, errors.New("boom")
	}})
	if _, err := initInst(t, bad, "def main(input):\n    return {}\n"); err == nil {
		t.Fatal("provider error: want Init failure")
	}
}

func TestManifestTimeoutHonoured(t *testing.T) {
	rt := srt.New(srt.Config{MaxSteps: 1 << 60})
	m := mani(t, "40ms") // manifest Limits.Timeout overrides
	inst, err := rt.Init(context.Background(), m,
		[]byte("def main(input):\n    s=0\n    for x in range(100000000):\n        s+=x\n    return {}\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.Execute(context.Background(), nil); !errors.Is(err, srt.ErrTimeout) {
		t.Fatalf("manifest timeout = %v, want ErrTimeout", err)
	}
}

func TestConversion_AllTypes(t *testing.T) {
	rt := srt.New(srt.Config{})
	inst, err := initInst(t, rt, "def main(input):\n    return input\n")
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]any{
		"nil":   nil,
		"bool":  true,
		"str":   "s",
		"f64":   1.5,
		"f32":   float32(2.5),
		"int":   7,
		"i8":    int8(8),
		"i16":   int16(16),
		"i32":   int32(32),
		"i64":   int64(64),
		"u":     uint(1),
		"u8":    uint8(2),
		"u16":   uint16(3),
		"u32":   uint32(4),
		"u64":   uint64(5),
		"list":  []any{1, "two", []any{3}},
		"nestm": map[string]any{"k": "v"},
	}
	out, err := inst.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	o := out.Output
	if o["nil"] != nil || o["bool"] != true || o["str"] != "s" ||
		o["f64"].(float64) != 1.5 || o["f32"].(float64) != 2.5 ||
		o["int"].(int64) != 7 || o["i64"].(int64) != 64 || o["u64"].(int64) != 5 ||
		o["u8"].(int64) != 2 {
		t.Fatalf("scalar round-trip wrong: %+v", o)
	}
	if l := o["list"].([]any); len(l) != 3 || l[1] != "two" || l[2].([]any)[0].(int64) != 3 {
		t.Fatalf("list round-trip: %+v", o["list"])
	}
	if o["nestm"].(map[string]any)["k"] != "v" {
		t.Fatalf("nested map: %+v", o["nestm"])
	}
}

func TestConversion_InputUnsupportedAndOutputEdges(t *testing.T) {
	rt := srt.New(srt.Config{})

	// Unsupported input type → input conversion error.
	inst, _ := initInst(t, rt, "def main(input):\n    return {}\n")
	if _, err := inst.Execute(context.Background(), map[string]any{"ch": make(chan int)}); err == nil {
		t.Fatal("unsupported input type: want error")
	}

	// Tuple return → []any (fromStarlark Tuple branch).
	ti, _ := initInst(t, rt, "def main(input):\n    return (1, 2, 3)\n")
	to, err := ti.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("tuple exec: %v", err)
	}
	if r := to.Output["result"].([]any); len(r) != 3 || r[0].(int64) != 1 {
		t.Fatalf("tuple result = %+v", to.Output)
	}

	// Non-string dict key → output conversion error.
	bi, _ := initInst(t, rt, "def main(input):\n    return {1: \"x\"}\n")
	if _, err := bi.Execute(context.Background(), nil); err == nil {
		t.Fatal("non-string dict key: want output conversion error")
	}

	// A function value → default fromStarlark fallback (its repr).
	fi, _ := initInst(t, rt, "def main(input):\n    return main\n")
	fo, err := fi.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("func-value exec: %v", err)
	}
	if _, ok := fo.Output["result"].(string); !ok {
		t.Fatalf("func value fallback = %+v", fo.Output)
	}
}

// End-to-end through the task-10 loader: register the runtime, load
// a real module zip, execute.
func TestLoaderIntegration(t *testing.T) {
	m := mani(t, "")
	manBytes, _ := manifest.MarshalManifest(m)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w1, _ := zw.Create("manifest.yaml")
	_, _ = w1.Write(manBytes)
	w2, _ := zw.Create("main.star")
	_, _ = w2.Write([]byte("def main(input):\n    return {\"echo\": input[\"v\"]}\n"))
	_ = zw.Close()
	path := filepath.Join(t.TempDir(), "m.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	rr := loader.NewRuntimeRegistry()
	rr.Register(manifest.TypeStarlark, srt.New(srt.Config{}))
	l := loader.New(loader.Config{Runtimes: rr})
	out, err := l.LoadAndExecute(context.Background(), path,
		loader.LoadOptions{SkipVerification: true}, map[string]any{"v": "hi"})
	if err != nil {
		t.Fatalf("LoadAndExecute: %v", err)
	}
	if out.Output["echo"] != "hi" {
		t.Fatalf("e2e output = %+v", out.Output)
	}
}
