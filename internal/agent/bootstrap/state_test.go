// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewState_StartsAtDetect(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	s := NewState(now)
	if s.Phase != PhaseDetect {
		t.Errorf("Phase = %q, want detect", s.Phase)
	}
	if s.Version != stateSchemaVersion {
		t.Errorf("Version = %d, want %d", s.Version, stateSchemaVersion)
	}
	if !s.StartedAt.Equal(now.UTC()) {
		t.Errorf("StartedAt = %v, want %v", s.StartedAt, now.UTC())
	}
}

func TestLoadState_AbsentReturnsNilNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-file.json")
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil for missing file", got)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	original := NewState(time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC))
	original.Phase = PhaseInstall
	original.Detection = &DetectionResult{OS: "linux", Distro: "ubuntu"}
	if err := original.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.Phase != PhaseInstall {
		t.Errorf("Phase round-trip = %q", got.Phase)
	}
	if got.Detection == nil || got.Detection.Distro != "ubuntu" {
		t.Errorf("Detection round-trip = %+v", got.Detection)
	}
}

func TestSave_AtomicNoTempLeftOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.json")
	if err := NewState(time.Now()).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
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

func TestSave_RejectsEmptyPath(t *testing.T) {
	if err := NewState(time.Now()).Save(""); err == nil {
		t.Error("Save(\"\"): expected error, got nil")
	}
}

func TestLoadState_RejectsHigherVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	body := []byte(`{"version": 999, "phase": "detect"}`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := LoadState(path)
	if err == nil {
		t.Error("LoadState: expected version error, got nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("err = %v, want containing 'version'", err)
	}
}

func TestLoadState_RejectsCorruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadState(path); err == nil {
		t.Error("LoadState corrupt: expected error")
	}
}

func TestNextPhase_Sequence(t *testing.T) {
	want := []Phase{PhaseConfigure, PhaseValidate, PhaseInstall, PhaseVerify, PhaseDone}
	cur := PhaseDetect
	for i, w := range want {
		got := nextPhase(cur)
		if got != w {
			t.Errorf("step %d: nextPhase(%q) = %q, want %q", i, cur, got, w)
		}
		cur = got
	}
	// PhaseDone is terminal — re-asking returns Done.
	if got := nextPhase(PhaseDone); got != PhaseDone {
		t.Errorf("nextPhase(done) = %q, want done (terminal)", got)
	}
}
