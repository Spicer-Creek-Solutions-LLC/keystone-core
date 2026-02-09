package versioning

import (
	"strings"
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if len(r.versions) != 0 {
		t.Error("New registry should be empty")
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	v := &Version{
		Name:       "v1",
		Major:      1,
		Minor:      0,
		Status:     StatusCurrent,
		ReleasedAt: time.Now(),
	}

	r.Register(v)

	got, ok := r.Get("v1")
	if !ok {
		t.Fatal("Version not found after register")
	}
	if got.Name != "v1" {
		t.Errorf("Name = %s, want v1", got.Name)
	}
}

func TestRegistry_Current(t *testing.T) {
	r := NewRegistry()

	// No current initially
	_, ok := r.Current()
	if ok {
		t.Error("Should have no current version initially")
	}

	// Register current version
	r.Register(&Version{
		Name:       "v1",
		Status:     StatusCurrent,
		ReleasedAt: time.Now(),
	})

	current, ok := r.Current()
	if !ok {
		t.Fatal("Should have current version")
	}
	if current.Name != "v1" {
		t.Errorf("Current = %s, want v1", current.Name)
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	now := time.Now()
	r.Register(&Version{Name: "v1", ReleasedAt: now.Add(-2 * time.Hour)})
	r.Register(&Version{Name: "v2", ReleasedAt: now.Add(-1 * time.Hour)})
	r.Register(&Version{Name: "v3", ReleasedAt: now})

	versions := r.List()
	if len(versions) != 3 {
		t.Fatalf("List returned %d versions, want 3", len(versions))
	}

	// Should be sorted newest first
	if versions[0].Name != "v3" {
		t.Errorf("First version = %s, want v3", versions[0].Name)
	}
	if versions[2].Name != "v1" {
		t.Errorf("Last version = %s, want v1", versions[2].Name)
	}
}

func TestRegistry_ListByStatus(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusRetired, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v2", Status: StatusDeprecated, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v3", Status: StatusCurrent, ReleasedAt: time.Now()})

	deprecated := r.ListByStatus(StatusDeprecated)
	if len(deprecated) != 1 {
		t.Errorf("Deprecated count = %d, want 1", len(deprecated))
	}
	if deprecated[0].Name != "v2" {
		t.Errorf("Deprecated version = %s, want v2", deprecated[0].Name)
	}
}

func TestRegistry_Supported(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusRetired, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v2", Status: StatusSupported, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v3", Status: StatusCurrent, ReleasedAt: time.Now()})

	supported := r.Supported()
	if len(supported) != 2 {
		t.Errorf("Supported count = %d, want 2", len(supported))
	}
}

func TestRegistry_Deprecate(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusSupported, ReleasedAt: time.Now()})

	notice := &DeprecationNotice{
		Reason:      "Superseded by v2",
		Replacement: "v2",
	}

	err := r.Deprecate("v1", notice)
	if err != nil {
		t.Fatalf("Deprecate failed: %v", err)
	}

	v, _ := r.Get("v1")
	if v.Status != StatusDeprecated {
		t.Errorf("Status = %s, want deprecated", v.Status)
	}
	if v.DeprecatedAt == nil {
		t.Error("DeprecatedAt should be set")
	}
	if v.DeprecationNotice == nil {
		t.Error("DeprecationNotice should be set")
	}
}

func TestRegistry_Deprecate_NotFound(t *testing.T) {
	r := NewRegistry()

	err := r.Deprecate("nonexistent", nil)
	if err == nil {
		t.Error("Should return error for nonexistent version")
	}
}

func TestRegistry_Retire(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusDeprecated, ReleasedAt: time.Now()})

	err := r.Retire("v1")
	if err != nil {
		t.Fatalf("Retire failed: %v", err)
	}

	v, _ := r.Get("v1")
	if v.Status != StatusRetired {
		t.Errorf("Status = %s, want retired", v.Status)
	}
	if v.SunsetAt == nil {
		t.Error("SunsetAt should be set")
	}
}

func TestRegistry_SetCurrent(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusCurrent, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v2", Status: StatusSupported, ReleasedAt: time.Now()})

	err := r.SetCurrent("v2")
	if err != nil {
		t.Fatalf("SetCurrent failed: %v", err)
	}

	// v2 should be current
	v2, _ := r.Get("v2")
	if v2.Status != StatusCurrent {
		t.Errorf("v2 status = %s, want current", v2.Status)
	}

	// v1 should be demoted to supported
	v1, _ := r.Get("v1")
	if v1.Status != StatusSupported {
		t.Errorf("v1 status = %s, want supported", v1.Status)
	}

	// Current should return v2
	current, _ := r.Current()
	if current.Name != "v2" {
		t.Errorf("Current = %s, want v2", current.Name)
	}
}

func TestRegistry_SetCurrent_NotFound(t *testing.T) {
	r := NewRegistry()

	err := r.SetCurrent("nonexistent")
	if err == nil {
		t.Error("Should return error for nonexistent version")
	}
}

func TestRegistry_IsDeprecated(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusDeprecated, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v2", Status: StatusCurrent, ReleasedAt: time.Now()})

	if !r.IsDeprecated("v1") {
		t.Error("v1 should be deprecated")
	}
	if r.IsDeprecated("v2") {
		t.Error("v2 should not be deprecated")
	}
	if r.IsDeprecated("nonexistent") {
		t.Error("nonexistent should return false")
	}
}

func TestRegistry_IsRetired(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusRetired, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v2", Status: StatusCurrent, ReleasedAt: time.Now()})

	if !r.IsRetired("v1") {
		t.Error("v1 should be retired")
	}
	if r.IsRetired("v2") {
		t.Error("v2 should not be retired")
	}
}

func TestRegistry_GetDeprecationNotice(t *testing.T) {
	r := NewRegistry()

	notice := &DeprecationNotice{
		Reason:      "Test reason",
		Replacement: "v2",
	}

	r.Register(&Version{
		Name:              "v1",
		Status:            StatusDeprecated,
		ReleasedAt:        time.Now(),
		DeprecationNotice: notice,
	})

	got := r.GetDeprecationNotice("v1")
	if got == nil {
		t.Fatal("Should return notice")
	}
	if got.Reason != "Test reason" {
		t.Errorf("Reason = %s", got.Reason)
	}

	// Non-deprecated version
	r.Register(&Version{Name: "v2", Status: StatusCurrent, ReleasedAt: time.Now()})
	if r.GetDeprecationNotice("v2") != nil {
		t.Error("Should return nil for non-deprecated version")
	}
}

func TestRegistry_CheckSunset(t *testing.T) {
	r := NewRegistry()

	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	r.Register(&Version{Name: "v1", Status: StatusDeprecated, ReleasedAt: time.Now(), SunsetAt: &past})
	r.Register(&Version{Name: "v2", Status: StatusDeprecated, ReleasedAt: time.Now(), SunsetAt: &future})
	r.Register(&Version{Name: "v3", Status: StatusRetired, ReleasedAt: time.Now(), SunsetAt: &past}) // Already retired

	expired := r.CheckSunset()
	if len(expired) != 1 {
		t.Errorf("Expired count = %d, want 1", len(expired))
	}
	if expired[0].Name != "v1" {
		t.Errorf("Expired version = %s, want v1", expired[0].Name)
	}
}

func TestRegistry_VersionHistory(t *testing.T) {
	r := NewRegistry()

	r.Register(&Version{Name: "v1", Status: StatusRetired, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v2", Status: StatusDeprecated, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v3", Status: StatusSupported, ReleasedAt: time.Now()})
	r.Register(&Version{Name: "v4", Status: StatusCurrent, ReleasedAt: time.Now()})

	history := r.VersionHistory()

	if len(history.Versions) != 4 {
		t.Errorf("Version count = %d, want 4", len(history.Versions))
	}
	if history.Current != "v4" {
		t.Errorf("Current = %s, want v4", history.Current)
	}
	if history.CurrentCount != 1 {
		t.Errorf("CurrentCount = %d, want 1", history.CurrentCount)
	}
	if history.SupportedCount != 1 {
		t.Errorf("SupportedCount = %d, want 1", history.SupportedCount)
	}
	if history.DeprecatedCount != 1 {
		t.Errorf("DeprecatedCount = %d, want 1", history.DeprecatedCount)
	}
	if history.RetiredCount != 1 {
		t.Errorf("RetiredCount = %d, want 1", history.RetiredCount)
	}
}

func TestHistory_Format(t *testing.T) {
	r := NewRegistry()

	deprecatedAt := time.Now().Add(-30 * 24 * time.Hour)
	sunsetAt := time.Now().Add(30 * 24 * time.Hour)

	r.Register(&Version{
		Name:         "v1",
		Status:       StatusDeprecated,
		ReleasedAt:   time.Now().Add(-90 * 24 * time.Hour),
		Description:  "Initial version",
		DeprecatedAt: &deprecatedAt,
		SunsetAt:     &sunsetAt,
		DeprecationNotice: &DeprecationNotice{
			Reason:      "Superseded by v2",
			Replacement: "v2",
		},
		BreakingChanges: []string{"Changed auth format"},
	})

	r.Register(&Version{
		Name:        "v2",
		Status:      StatusCurrent,
		ReleasedAt:  time.Now(),
		Description: "Current version",
	})

	history := r.VersionHistory()
	output := history.Format()

	expectedParts := []string{
		"API Version History",
		"v1",
		"v2",
		"deprecated",
		"current",
		"Superseded by v2",
		"Use v2 instead",
		"Breaking changes",
		"Changed auth format",
	}

	for _, part := range expectedParts {
		if !strings.Contains(output, part) {
			t.Errorf("Format output missing: %s", part)
		}
	}
}

func TestDeprecationWarning(t *testing.T) {
	sunset := time.Now().Add(30 * 24 * time.Hour)
	notice := &DeprecationNotice{
		Reason:         "Superseded by v2",
		Replacement:    "v2",
		MigrationGuide: "https://docs.example.com/migrate",
		SunsetDate:     &sunset,
	}

	warning := DeprecationWarning("v1", notice)

	expectedParts := []string{
		"WARNING",
		"v1",
		"deprecated",
		"Superseded by v2",
		"migrate to v2",
		"removed on",
		"Migration guide",
	}

	for _, part := range expectedParts {
		if !strings.Contains(warning, part) {
			t.Errorf("Warning missing: %s\nGot: %s", part, warning)
		}
	}
}

func TestDeprecationWarning_NoNotice(t *testing.T) {
	warning := DeprecationWarning("v1", nil)

	if !strings.Contains(warning, "v1") {
		t.Error("Should mention version")
	}
	if !strings.Contains(warning, "deprecated") {
		t.Error("Should mention deprecated")
	}
}

func TestSunsetWarning(t *testing.T) {
	// Past sunset
	past := time.Now().Add(-24 * time.Hour)
	warning := SunsetWarning("v1", past)
	if !strings.Contains(warning, "ERROR") || !strings.Contains(warning, "retired") {
		t.Errorf("Past sunset warning incorrect: %s", warning)
	}

	// Today
	today := time.Now().Add(1 * time.Hour)
	warning = SunsetWarning("v1", today)
	if !strings.Contains(warning, "CRITICAL") || !strings.Contains(warning, "TODAY") {
		t.Errorf("Today sunset warning incorrect: %s", warning)
	}

	// Within a week
	week := time.Now().Add(5 * 24 * time.Hour)
	warning = SunsetWarning("v1", week)
	if !strings.Contains(warning, "CRITICAL") {
		t.Errorf("Week sunset warning incorrect: %s", warning)
	}

	// Within a month
	month := time.Now().Add(20 * 24 * time.Hour)
	warning = SunsetWarning("v1", month)
	if !strings.Contains(warning, "WARNING") {
		t.Errorf("Month sunset warning incorrect: %s", warning)
	}

	// Far future
	future := time.Now().Add(60 * 24 * time.Hour)
	warning = SunsetWarning("v1", future)
	if !strings.Contains(warning, "NOTICE") {
		t.Errorf("Future sunset warning incorrect: %s", warning)
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusCurrent, "✓"},
		{StatusSupported, "●"},
		{StatusDeprecated, "⚠"},
		{StatusRetired, "✗"},
		{StatusBeta, "β"},
		{StatusAlpha, "α"},
		{Status("unknown"), "?"},
	}

	for _, tt := range tests {
		if got := statusIcon(tt.status); got != tt.expected {
			t.Errorf("statusIcon(%s) = %s, want %s", tt.status, got, tt.expected)
		}
	}
}
