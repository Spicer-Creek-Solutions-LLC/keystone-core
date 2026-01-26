package state

import (
	"context"
	"testing"
	"time"
)

func TestNewDriftDetector_NilManager(t *testing.T) {
	if _, err := NewDriftDetector(nil, nil, DefaultDriftConfig()); err == nil {
		t.Fatal("Expected error for nil manager")
	}
}

func TestDriftDetector_HashAndDiffs(t *testing.T) {
	manager := newTestManager(t, "device-1")
	detector, err := NewDriftDetector(manager, nil, DefaultDriftConfig())
	if err != nil {
		t.Fatalf("NewDriftDetector failed: %v", err)
	}

	hash1 := detector.hashConfig("line1\nline2\n")
	hash2 := detector.hashConfig("line1\nline2\n")
	if hash1 != hash2 {
		t.Fatalf("Expected identical hashes, got %s and %s", hash1, hash2)
	}

	diffs := detector.computeDiffs("password abc\ninterface eth0\n", "password xyz\ninterface eth0\nvlan 10\n")
	if len(diffs) == 0 {
		t.Fatal("Expected diffs")
	}

	severity := detector.calculateSeverity(diffs)
	if severity != DriftSeverityCritical && severity != DriftSeverityHigh && severity != DriftSeverityMedium {
		t.Fatalf("Unexpected severity: %s", severity)
	}
}

func TestDriftDetector_CaptureBaselineAndCheck(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{
		exitCodes: map[string]int{
			"show running-config": 0,
		},
		stdout: map[string]string{
			"show running-config": "hostname device\n",
		},
	})

	detector, err := NewDriftDetector(manager, nil, DefaultDriftConfig())
	if err != nil {
		t.Fatalf("NewDriftDetector failed: %v", err)
	}

	baseline, err := detector.CaptureBaseline(context.Background(), "device-1")
	if err != nil {
		t.Fatalf("CaptureBaseline failed: %v", err)
	}
	if baseline.Hash == "" {
		t.Fatal("Expected baseline hash")
	}

	report, err := detector.CheckDrift(context.Background(), "device-1")
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}
	if report.HasDrift {
		t.Fatal("Expected no drift when config matches baseline")
	}
}

func TestDriftDetector_CheckDrift_NoBaseline(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{})

	detector, err := NewDriftDetector(manager, nil, DefaultDriftConfig())
	if err != nil {
		t.Fatalf("NewDriftDetector failed: %v", err)
	}

	report, err := detector.CheckDrift(context.Background(), "device-1")
	if err == nil {
		t.Fatal("Expected error for missing baseline")
	}
	if report.Error == "" {
		t.Fatal("Expected report error")
	}
}

func TestInMemoryBaselineStore(t *testing.T) {
	store := NewInMemoryBaselineStore()
	baseline := &ConfigBaseline{
		DeviceID:  "device-1",
		Timestamp: time.Now(),
		Hash:      "hash",
		Config:    "config",
	}

	if err := store.Save("device-1", baseline); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load("device-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got == nil || got.Hash != "hash" {
		t.Fatalf("Unexpected baseline: %+v", got)
	}

	ids, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(ids) != 1 || ids[0] != "device-1" {
		t.Fatalf("Unexpected list result: %v", ids)
	}

	if err := store.Delete("device-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if got, _ = store.Load("device-1"); got != nil {
		t.Fatalf("Expected baseline to be deleted")
	}
}

func TestDriftDetector_CheckAllDrift(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{
		exitCodes: map[string]int{
			"show running-config": 0,
		},
		stdout: map[string]string{
			"show running-config": "hostname device\n",
		},
	})

	store := NewInMemoryBaselineStore()
	if err := store.Save("device-1", &ConfigBaseline{
		DeviceID:  "device-1",
		Timestamp: time.Now(),
		Hash:      "hash",
		Config:    "hostname device\n",
	}); err != nil {
		t.Fatalf("Save baseline failed: %v", err)
	}

	detector, err := NewDriftDetector(manager, store, DefaultDriftConfig())
	if err != nil {
		t.Fatalf("NewDriftDetector failed: %v", err)
	}

	reports, err := detector.CheckAllDrift(context.Background())
	if err != nil {
		t.Fatalf("CheckAllDrift failed: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("Expected 1 report, got %d", len(reports))
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	if !containsIgnoreCase("Enable password", "password") {
		t.Fatal("Expected case-insensitive match")
	}
	if containsIgnoreCase("hello", "world") {
		t.Fatal("Expected no match")
	}
}

func TestSplitLines(t *testing.T) {
	lines := splitLines("one\r\ntwo\n\nthree")
	if len(lines) != 3 {
		t.Fatalf("Expected 3 lines, got %d", len(lines))
	}
}

func TestDriftDetector_GetDeviceConfig_Failure(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{
		exitCodes: map[string]int{
			"show running-config": 1,
			"show configuration":  1,
		},
	})

	detector, err := NewDriftDetector(manager, nil, DefaultDriftConfig())
	if err != nil {
		t.Fatalf("NewDriftDetector failed: %v", err)
	}

	device, err := manager.Registry().Get(context.Background(), "device-1")
	if err != nil {
		t.Fatalf("Get device failed: %v", err)
	}

	if _, err := detector.getDeviceConfig(context.Background(), device, manager.Executor()); err == nil {
		t.Fatal("Expected error when config cannot be retrieved")
	}
}

func TestDriftDetector_ClassifyLineSeverity(t *testing.T) {
	manager := newTestManager(t, "device-1")
	detector, err := NewDriftDetector(manager, nil, DefaultDriftConfig())
	if err != nil {
		t.Fatalf("NewDriftDetector failed: %v", err)
	}

	if detector.classifyLineSeverity("enable secret foo") != DriftSeverityCritical {
		t.Fatal("Expected critical severity")
	}
	if detector.classifyLineSeverity("ip route 0.0.0.0/0") != DriftSeverityHigh {
		t.Fatal("Expected high severity")
	}
	if detector.classifyLineSeverity("interface eth0") != DriftSeverityMedium {
		t.Fatal("Expected medium severity")
	}
	if detector.classifyLineSeverity("hostname device") != DriftSeverityLow {
		t.Fatal("Expected low severity")
	}
}

func TestDriftDetector_EmitEvent(t *testing.T) {
	manager := newTestManager(t, "device-1")
	manager.SetExecutor(&fakeExecutor{
		exitCodes: map[string]int{
			"show running-config": 0,
		},
		stdout: map[string]string{
			"show running-config": "hostname device\n",
		},
	})

	store := NewInMemoryBaselineStore()
	if err := store.Save("device-1", &ConfigBaseline{
		DeviceID:  "device-1",
		Timestamp: time.Now(),
		Hash:      "old-hash",
		Config:    "hostname old\n",
	}); err != nil {
		t.Fatalf("Save baseline failed: %v", err)
	}

	detector, err := NewDriftDetector(manager, store, DefaultDriftConfig())
	if err != nil {
		t.Fatalf("NewDriftDetector failed: %v", err)
	}

	emitter := &captureEmitter{}
	detector.SetEventEmitter(emitter)

	report, err := detector.CheckDrift(context.Background(), "device-1")
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}
	if !report.HasDrift {
		t.Fatal("Expected drift to be detected")
	}
	if len(emitter.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(emitter.events))
	}
}

func TestFileBaselineStore_NoOps(t *testing.T) {
	store := NewFileBaselineStore(t.TempDir())
	if err := store.Save("device", &ConfigBaseline{}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := store.Load("device"); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if err := store.Delete("device"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if _, err := store.List(); err != nil {
		t.Fatalf("List failed: %v", err)
	}
}
