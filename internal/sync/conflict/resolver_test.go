package conflict

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.DefaultStrategy == "" {
		t.Error("DefaultStrategy should be set")
	}
	if config.MaxHistorySize <= 0 {
		t.Error("MaxHistorySize should be positive")
	}
}

func TestVersion_Compare(t *testing.T) {
	t.Run("clock comparison", func(t *testing.T) {
		v1 := &Version{Clock: 1}
		v2 := &Version{Clock: 2}

		if v1.Compare(v2) != -1 {
			t.Error("v1 should be less than v2")
		}
		if v2.Compare(v1) != 1 {
			t.Error("v2 should be greater than v1")
		}
	})

	t.Run("same clock different timestamp", func(t *testing.T) {
		now := time.Now()
		v1 := &Version{Clock: 1, Timestamp: now.Add(-time.Hour)}
		v2 := &Version{Clock: 1, Timestamp: now}

		if v1.Compare(v2) != -1 {
			t.Error("v1 should be less than v2 (older timestamp)")
		}
	})

	t.Run("vector comparison", func(t *testing.T) {
		v1 := &Version{
			Vector: map[string]uint64{"A": 1, "B": 1},
		}
		v2 := &Version{
			Vector: map[string]uint64{"A": 2, "B": 1},
		}

		if v1.Compare(v2) != -1 {
			t.Error("v1 should be less than v2")
		}
	})

	t.Run("concurrent versions", func(t *testing.T) {
		v1 := &Version{
			Vector: map[string]uint64{"A": 2, "B": 1},
		}
		v2 := &Version{
			Vector: map[string]uint64{"A": 1, "B": 2},
		}

		if v1.Compare(v2) != 0 {
			t.Error("Concurrent versions should return 0")
		}
	})
}

func TestDocument_ComputeHash(t *testing.T) {
	doc1 := &Document{Data: []byte("hello")}
	doc2 := &Document{Data: []byte("hello")}
	doc3 := &Document{Data: []byte("world")}

	if doc1.ComputeHash() != doc2.ComputeHash() {
		t.Error("Same content should have same hash")
	}
	if doc1.ComputeHash() == doc3.ComputeHash() {
		t.Error("Different content should have different hash")
	}
}

func TestManager_Detect(t *testing.T) {
	manager := NewManager(nil)

	now := time.Now()

	t.Run("no conflict - same content", func(t *testing.T) {
		local := &Document{
			ID:      "doc1",
			Data:    []byte("same"),
			Version: &Version{Clock: 1, Timestamp: now},
		}
		remote := &Document{
			ID:      "doc1",
			Data:    []byte("same"),
			Version: &Version{Clock: 2, Timestamp: now},
		}

		conflict, err := manager.Detect(local, remote)
		if err != nil {
			t.Fatalf("Detect failed: %v", err)
		}
		if conflict != nil {
			t.Error("Should not detect conflict for same content")
		}
	})

	t.Run("no conflict - remote is newer", func(t *testing.T) {
		local := &Document{
			ID:      "doc1",
			Data:    []byte("local"),
			Version: &Version{Clock: 1, Timestamp: now.Add(-time.Hour)},
		}
		remote := &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 2, Timestamp: now},
		}

		conflict, err := manager.Detect(local, remote)
		if err != nil {
			t.Fatalf("Detect failed: %v", err)
		}
		if conflict != nil {
			t.Error("Should not detect conflict when remote is newer")
		}
	})

	t.Run("conflict - concurrent modifications", func(t *testing.T) {
		local := &Document{
			ID:   "doc1",
			Data: []byte("local"),
			Version: &Version{
				Clock:     1,
				Timestamp: now,
				Vector:    map[string]uint64{"A": 1},
			},
		}
		remote := &Document{
			ID:   "doc1",
			Data: []byte("remote"),
			Version: &Version{
				Clock:     1,
				Timestamp: now,
				Vector:    map[string]uint64{"B": 1},
			},
		}

		conflict, err := manager.Detect(local, remote)
		if err != nil {
			t.Fatalf("Detect failed: %v", err)
		}
		if conflict == nil {
			t.Error("Should detect conflict for concurrent modifications")
		}
	})
}

func TestManager_Resolve_LastWriteWins(t *testing.T) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyLastWriteWins
	manager := NewManager(config)

	now := time.Now()
	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    []byte("local"),
			Version: &Version{Clock: 1, Timestamp: now.Add(-time.Hour)},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict

	resolution, err := manager.Resolve(conflict)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if string(resolution.Document.Data) != "remote" {
		t.Error("Last write wins should choose remote (newer)")
	}
}

func TestManager_Resolve_FirstWriteWins(t *testing.T) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyFirstWriteWins
	manager := NewManager(config)

	now := time.Now()
	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    []byte("local"),
			Version: &Version{Clock: 1, Timestamp: now.Add(-time.Hour)},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict

	resolution, err := manager.Resolve(conflict)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if string(resolution.Document.Data) != "local" {
		t.Error("First write wins should choose local (older)")
	}
}

func TestManager_Resolve_HighestVersion(t *testing.T) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyHighestVersion
	manager := NewManager(config)

	now := time.Now()
	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    []byte("local"),
			Version: &Version{Clock: 5, Timestamp: now},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 3, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict

	resolution, err := manager.Resolve(conflict)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if string(resolution.Document.Data) != "local" {
		t.Error("Highest version should choose local (higher clock)")
	}
}

func TestManager_Resolve_Merge(t *testing.T) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyMerge
	config.MergeEnabled = true
	manager := NewManager(config)

	now := time.Now()

	localData, _ := json.Marshal(map[string]interface{}{
		"name": "test",
		"a":    1,
	})
	remoteData, _ := json.Marshal(map[string]interface{}{
		"name": "test",
		"b":    2,
	})

	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    localData,
			Version: &Version{Clock: 1, Timestamp: now},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    remoteData,
			Version: &Version{Clock: 1, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict

	resolution, err := manager.Resolve(conflict)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	var merged map[string]interface{}
	json.Unmarshal(resolution.Document.Data, &merged)

	if merged["a"] != float64(1) {
		t.Error("Merged should contain 'a' from local")
	}
	if merged["b"] != float64(2) {
		t.Error("Merged should contain 'b' from remote")
	}
}

func TestManager_ManualResolve(t *testing.T) {
	manager := NewManager(nil)

	now := time.Now()
	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    []byte("local"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict

	resolution, err := manager.ManualResolve("c1", "local")
	if err != nil {
		t.Fatalf("ManualResolve failed: %v", err)
	}

	if string(resolution.Document.Data) != "local" {
		t.Error("Manual resolve should choose local")
	}
	if !resolution.Manual {
		t.Error("Resolution should be marked as manual")
	}
}

func TestManager_ManualResolve_Remote(t *testing.T) {
	manager := NewManager(nil)

	now := time.Now()
	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    []byte("local"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict

	resolution, err := manager.ManualResolve("c1", "remote")
	if err != nil {
		t.Fatalf("ManualResolve failed: %v", err)
	}

	if string(resolution.Document.Data) != "remote" {
		t.Error("Manual resolve should choose remote")
	}
}

func TestManager_ListConflicts(t *testing.T) {
	manager := NewManager(nil)

	now := time.Now()
	for i := 0; i < 3; i++ {
		conflict := &Conflict{
			ID:         string(rune('a' + i)),
			DocumentID: "doc" + string(rune('0'+i)),
			Local: &Document{
				ID:      "doc" + string(rune('0'+i)),
				Version: &Version{Clock: 1, Timestamp: now},
			},
			Remote: &Document{
				ID:      "doc" + string(rune('0'+i)),
				Version: &Version{Clock: 1, Timestamp: now},
			},
		}
		manager.conflicts[conflict.ID] = conflict
	}

	conflicts := manager.ListConflicts()
	if len(conflicts) != 3 {
		t.Errorf("Expected 3 conflicts, got %d", len(conflicts))
	}
}

func TestManager_History(t *testing.T) {
	config := DefaultConfig()
	config.KeepHistory = true
	manager := NewManager(config)

	now := time.Now()
	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    []byte("local"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict
	manager.Resolve(conflict)

	history := manager.History()
	if len(history) != 1 {
		t.Errorf("Expected 1 history entry, got %d", len(history))
	}
	if !history[0].Resolved {
		t.Error("History entry should be resolved")
	}
}

func TestManager_Events(t *testing.T) {
	manager := NewManager(nil)

	var events []*Event
	var mu sync.Mutex

	manager.AddListener(func(event *Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	now := time.Now()

	// Detect conflict
	local := &Document{
		ID:   "doc1",
		Data: []byte("local"),
		Version: &Version{
			Clock:     1,
			Timestamp: now,
			Vector:    map[string]uint64{"A": 1},
		},
	}
	remote := &Document{
		ID:   "doc1",
		Data: []byte("remote"),
		Version: &Version{
			Clock:     1,
			Timestamp: now,
			Vector:    map[string]uint64{"B": 1},
		},
	}

	conflict, _ := manager.Detect(local, remote)
	manager.Resolve(conflict)

	mu.Lock()
	defer mu.Unlock()

	if len(events) < 2 {
		t.Errorf("Expected at least 2 events, got %d", len(events))
	}

	hasDetected := false
	hasResolved := false
	for _, e := range events {
		if e.Type == "conflict_detected" {
			hasDetected = true
		}
		if e.Type == "conflict_resolved" {
			hasResolved = true
		}
	}

	if !hasDetected {
		t.Error("Expected conflict_detected event")
	}
	if !hasResolved {
		t.Error("Expected conflict_resolved event")
	}
}

func TestManager_TypeStrategies(t *testing.T) {
	config := DefaultConfig()
	config.DefaultStrategy = StrategyLastWriteWins
	config.TypeStrategies = map[string]Strategy{
		"config": StrategyFirstWriteWins,
	}
	manager := NewManager(config)

	now := time.Now()

	// Conflict with type metadata
	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:       "doc1",
			Data:     []byte("local"),
			Version:  &Version{Clock: 1, Timestamp: now.Add(-time.Hour)},
			Metadata: map[string]string{"type": "config"},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    []byte("remote"),
			Version: &Version{Clock: 1, Timestamp: now},
		},
	}

	manager.conflicts["c1"] = conflict

	resolution, err := manager.Resolve(conflict)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Should use first-write-wins for "config" type
	if string(resolution.Document.Data) != "local" {
		t.Error("Type-specific strategy should use first-write-wins for config type")
	}
}

func TestCRDTResolver_Counter(t *testing.T) {
	resolver := &CRDTResolver{Type: "counter"}

	localData, _ := json.Marshal(int64(10))
	remoteData, _ := json.Marshal(int64(15))

	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    localData,
			Version: &Version{Clock: 1},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    remoteData,
			Version: &Version{Clock: 1},
		},
	}

	resolution, err := resolver.ResolveCounter(conflict)
	if err != nil {
		t.Fatalf("ResolveCounter failed: %v", err)
	}

	var value int64
	json.Unmarshal(resolution.Document.Data, &value)

	if value != 15 {
		t.Errorf("Counter should be max value (15), got %d", value)
	}
}

func TestCRDTResolver_Set(t *testing.T) {
	resolver := &CRDTResolver{Type: "set"}

	localData, _ := json.Marshal([]string{"a", "b"})
	remoteData, _ := json.Marshal([]string{"b", "c"})

	conflict := &Conflict{
		ID:         "c1",
		DocumentID: "doc1",
		Local: &Document{
			ID:      "doc1",
			Data:    localData,
			Version: &Version{Clock: 1},
		},
		Remote: &Document{
			ID:      "doc1",
			Data:    remoteData,
			Version: &Version{Clock: 1},
		},
	}

	resolution, err := resolver.ResolveSet(conflict)
	if err != nil {
		t.Fatalf("ResolveSet failed: %v", err)
	}

	var values []interface{}
	json.Unmarshal(resolution.Document.Data, &values)

	if len(values) != 3 {
		t.Errorf("Set union should have 3 elements, got %d", len(values))
	}
}

func TestThreeWayMerger(t *testing.T) {
	merger := &ThreeWayMerger{}

	base := &Document{
		ID:      "doc1",
		Data:    []byte("line1\nline2\nline3"),
		Version: &Version{Clock: 1},
	}
	local := &Document{
		ID:      "doc1",
		Data:    []byte("line1\nmodified2\nline3"),
		Version: &Version{Clock: 2},
	}
	remote := &Document{
		ID:      "doc1",
		Data:    []byte("line1\nline2\nmodified3"),
		Version: &Version{Clock: 2},
	}

	result, err := merger.Merge(base, local, remote)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	expected := "line1\nmodified2\nmodified3"
	if string(result.Data) != expected {
		t.Errorf("Merge result = %q, want %q", string(result.Data), expected)
	}
}

func TestMergeRecursive(t *testing.T) {
	local := map[string]interface{}{
		"name": "test",
		"nested": map[string]interface{}{
			"a": 1,
			"b": 2,
		},
	}
	remote := map[string]interface{}{
		"name": "changed",
		"nested": map[string]interface{}{
			"b": 3,
			"c": 4,
		},
	}

	result := mergeRecursive(local, remote)

	if result["name"] != "changed" {
		t.Error("name should be overwritten by remote")
	}

	nested := result["nested"].(map[string]interface{})
	if nested["a"] != 1 {
		t.Error("nested.a should be preserved from local")
	}
	if nested["b"] != 3 {
		t.Error("nested.b should be overwritten by remote")
	}
	if nested["c"] != 4 {
		t.Error("nested.c should be added from remote")
	}
}
