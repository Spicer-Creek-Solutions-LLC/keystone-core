package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultInstaller_FreshWrite(t *testing.T) {
	dir := t.TempDir()
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		JoinURL:     "nats://server:4222",
		ConfigPath:  filepath.Join(dir, "agent.yaml"),
	}
	i := NewDefaultInstaller(discardLogger())
	res, err := i.Install(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !res.Created {
		t.Error("Created = false, want true on fresh write")
	}
	if res.Updated {
		t.Error("Updated = true, want false on fresh write")
	}
	if res.BytesWritten == 0 {
		t.Error("BytesWritten = 0")
	}

	body, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	bodyStr := string(body)
	for _, want := range []string{`mode: development`, `id: agent-1`, `clustername: default`, `nats://server:4222`} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("config missing %q:\n%s", want, bodyStr)
		}
	}
}

func TestDefaultInstaller_Idempotent(t *testing.T) {
	dir := t.TempDir()
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(dir, "agent.yaml"),
	}
	i := NewDefaultInstaller(discardLogger())
	if _, err := i.Install(context.Background(), cfg); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	res, err := i.Install(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if res.Created || res.Updated {
		t.Errorf("Created=%v Updated=%v on idempotent re-run; want both false",
			res.Created, res.Updated)
	}
}

func TestDefaultInstaller_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(dir, "agent.yaml"),
	}
	i := NewDefaultInstaller(discardLogger())
	if _, err := i.Install(context.Background(), cfg); err != nil {
		t.Fatalf("first Install: %v", err)
	}

	// Mutate cluster name; second Install should rewrite.
	cfg.ClusterName = "different"
	res, err := i.Install(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Install after change: %v", err)
	}
	if res.Created {
		t.Error("Created = true on update, want false")
	}
	if !res.Updated {
		t.Error("Updated = false on changed config, want true")
	}
}

func TestDefaultInstaller_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	cfg := &Configuration{
		Mode:        ModeDemo,
		ClusterName: "default",
		AgentID:     "agent-1",
		ConfigPath:  filepath.Join(dir, "agent.yaml"),
		DryRun:      true,
	}
	i := NewDefaultInstaller(discardLogger())
	if _, err := i.Install(context.Background(), cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(cfg.ConfigPath); !os.IsNotExist(err) {
		t.Errorf("dry run wrote file: %v", err)
	}
}

func TestDefaultInstaller_RejectsEmptyConfigPath(t *testing.T) {
	cfg := &Configuration{Mode: ModeDemo, ClusterName: "x", AgentID: "a", ConfigPath: ""}
	i := NewDefaultInstaller(discardLogger())
	if _, err := i.Install(context.Background(), cfg); err == nil {
		t.Error("Install with empty ConfigPath: expected error")
	}
}

func TestDefaultInstaller_RejectsNilConfig(t *testing.T) {
	i := NewDefaultInstaller(discardLogger())
	if _, err := i.Install(context.Background(), nil); err == nil {
		t.Error("Install(nil): expected error")
	}
}

func TestAtomicWriteFile_NoTempLeftOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")
	if err := atomicWriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp.") {
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
}
