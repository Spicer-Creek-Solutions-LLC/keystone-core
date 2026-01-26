package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewIndexer(t *testing.T) {
	idx := NewIndexer()
	if idx == nil {
		t.Fatal("NewIndexer returned nil")
	}
	if idx.Count() != 0 {
		t.Errorf("expected empty indexer, got %d entries", idx.Count())
	}
}

func TestIndexer_Index(t *testing.T) {
	idx := NewIndexer()

	entry := &IndexEntry{
		Name:          "myorg/web-stack",
		Namespace:     "myorg",
		ShortName:     "web-stack",
		LatestVersion: "1.0.0",
		AllVersions:   []string{"1.0.0", "0.9.0"},
		Description:   "A complete web application stack",
		Author:        "MyOrg Team",
		Tags:          []string{"web", "nginx", "postgres"},
		Category:      "infrastructure",
		Downloads:     100,
		Stars:         10,
	}

	err := idx.Index(entry)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	if idx.Count() != 1 {
		t.Errorf("expected 1 entry, got %d", idx.Count())
	}

	// Retrieve entry
	got, ok := idx.Get("myorg/web-stack")
	if !ok {
		t.Fatal("failed to get indexed entry")
	}
	if got.Name != entry.Name {
		t.Errorf("expected name %s, got %s", entry.Name, got.Name)
	}
	if got.Downloads != 100 {
		t.Errorf("expected downloads 100, got %d", got.Downloads)
	}
}

func TestIndexer_Index_Errors(t *testing.T) {
	idx := NewIndexer()

	// Nil entry
	err := idx.Index(nil)
	if err == nil {
		t.Error("expected error for nil entry")
	}

	// Empty name
	err = idx.Index(&IndexEntry{})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestIndexer_Index_Update(t *testing.T) {
	idx := NewIndexer()

	entry := &IndexEntry{
		Name:        "myorg/web-stack",
		Namespace:   "myorg",
		ShortName:   "web-stack",
		Description: "Original description",
		Tags:        []string{"web"},
	}

	err := idx.Index(entry)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	// Update with new description and tags
	updatedEntry := &IndexEntry{
		Name:        "myorg/web-stack",
		Namespace:   "myorg",
		ShortName:   "web-stack",
		Description: "Updated description",
		Tags:        []string{"web", "nginx"},
	}

	err = idx.Index(updatedEntry)
	if err != nil {
		t.Fatalf("Index update failed: %v", err)
	}

	if idx.Count() != 1 {
		t.Errorf("expected 1 entry after update, got %d", idx.Count())
	}

	got, _ := idx.Get("myorg/web-stack")
	if got.Description != "Updated description" {
		t.Errorf("expected updated description, got %s", got.Description)
	}
}

func TestIndexer_Remove(t *testing.T) {
	idx := NewIndexer()

	entry := &IndexEntry{
		Name:      "myorg/web-stack",
		Namespace: "myorg",
		ShortName: "web-stack",
		Tags:      []string{"web"},
	}

	err := idx.Index(entry)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	err = idx.Remove("myorg/web-stack")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if idx.Count() != 0 {
		t.Errorf("expected 0 entries after remove, got %d", idx.Count())
	}

	_, ok := idx.Get("myorg/web-stack")
	if ok {
		t.Error("entry should not exist after remove")
	}

	// Remove non-existent
	err = idx.Remove("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent entry")
	}
}

func TestIndexer_Search(t *testing.T) {
	idx := NewIndexer()

	// Add test entries
	entries := []*IndexEntry{
		{
			Name:        "myorg/web-stack",
			Namespace:   "myorg",
			ShortName:   "web-stack",
			Description: "Complete web application stack with nginx",
			Tags:        []string{"web", "nginx"},
			Category:    "infrastructure",
		},
		{
			Name:        "myorg/database",
			Namespace:   "myorg",
			ShortName:   "database",
			Description: "PostgreSQL database setup",
			Tags:        []string{"database", "postgres"},
			Category:    "data",
		},
		{
			Name:        "acme/monitoring",
			Namespace:   "acme",
			ShortName:   "monitoring",
			Description: "Prometheus and Grafana monitoring",
			Tags:        []string{"monitoring", "prometheus"},
			Category:    "observability",
		},
	}

	for _, e := range entries {
		if err := idx.Index(e); err != nil {
			t.Fatalf("Index failed: %v", err)
		}
	}

	tests := []struct {
		name     string
		query    string
		minCount int
	}{
		{"empty query returns all", "", 3},
		{"search by name", "web-stack", 1},
		{"search by description", "nginx", 1},
		{"search by description word", "database", 1},
		{"search by tag", "monitoring", 1},
		{"no matches", "kubernetes", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := idx.Search(tt.query)
			if len(results) < tt.minCount {
				t.Errorf("expected at least %d results, got %d", tt.minCount, len(results))
			}
		})
	}
}

func TestIndexer_ListByNamespace(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "myorg/web", Namespace: "myorg", ShortName: "web"},
		{Name: "myorg/api", Namespace: "myorg", ShortName: "api"},
		{Name: "acme/app", Namespace: "acme", ShortName: "app"},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	results := idx.ListByNamespace("myorg")
	if len(results) != 2 {
		t.Errorf("expected 2 results for myorg, got %d", len(results))
	}

	results = idx.ListByNamespace("acme")
	if len(results) != 1 {
		t.Errorf("expected 1 result for acme, got %d", len(results))
	}

	results = idx.ListByNamespace("unknown")
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown, got %d", len(results))
	}
}

func TestIndexer_ListByTag(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "myorg/web", Namespace: "myorg", Tags: []string{"web", "nginx"}},
		{Name: "myorg/api", Namespace: "myorg", Tags: []string{"api", "web"}},
		{Name: "acme/app", Namespace: "acme", Tags: []string{"app"}},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	results := idx.ListByTag("web")
	if len(results) != 2 {
		t.Errorf("expected 2 results for web tag, got %d", len(results))
	}

	results = idx.ListByTag("nginx")
	if len(results) != 1 {
		t.Errorf("expected 1 result for nginx tag, got %d", len(results))
	}

	// Case insensitive
	results = idx.ListByTag("WEB")
	if len(results) != 2 {
		t.Errorf("expected 2 results for WEB tag (case insensitive), got %d", len(results))
	}
}

func TestIndexer_ListByCategory(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "myorg/web", Namespace: "myorg", Category: "infrastructure"},
		{Name: "myorg/api", Namespace: "myorg", Category: "infrastructure"},
		{Name: "acme/app", Namespace: "acme", Category: "application"},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	results := idx.ListByCategory("infrastructure")
	if len(results) != 2 {
		t.Errorf("expected 2 results for infrastructure, got %d", len(results))
	}

	results = idx.ListByCategory("application")
	if len(results) != 1 {
		t.Errorf("expected 1 result for application, got %d", len(results))
	}
}

func TestIndexer_ListByAuthor(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "myorg/web", Namespace: "myorg", Author: "John Doe"},
		{Name: "myorg/api", Namespace: "myorg", Author: "John Doe"},
		{Name: "acme/app", Namespace: "acme", Author: "Jane Smith"},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	results := idx.ListByAuthor("john doe")
	if len(results) != 2 {
		t.Errorf("expected 2 results for john doe, got %d", len(results))
	}
}

func TestIndexer_GetStats(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{
			Name:        "myorg/web",
			Namespace:   "myorg",
			Category:    "infrastructure",
			Tags:        []string{"web"},
			AllVersions: []string{"1.0.0", "0.9.0"},
			Downloads:   100,
			Stars:       10,
		},
		{
			Name:        "myorg/api",
			Namespace:   "myorg",
			Category:    "infrastructure",
			Tags:        []string{"api"},
			AllVersions: []string{"1.0.0"},
			Downloads:   50,
			Stars:       5,
		},
		{
			Name:        "acme/app",
			Namespace:   "acme",
			Category:    "application",
			Tags:        []string{"app", "web"},
			AllVersions: []string{"2.0.0", "1.0.0", "0.5.0"},
			Downloads:   200,
			Stars:       20,
		},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	stats := idx.GetStats()
	if stats.TotalBlueprints != 3 {
		t.Errorf("expected 3 total blueprints, got %d", stats.TotalBlueprints)
	}
	if stats.TotalVersions != 6 {
		t.Errorf("expected 6 total versions, got %d", stats.TotalVersions)
	}
	if stats.Namespaces != 2 {
		t.Errorf("expected 2 namespaces, got %d", stats.Namespaces)
	}
	if stats.Categories["infrastructure"] != 2 {
		t.Errorf("expected 2 infrastructure blueprints, got %d", stats.Categories["infrastructure"])
	}
	if stats.Tags["web"] != 2 {
		t.Errorf("expected 2 web tags, got %d", stats.Tags["web"])
	}
	if len(stats.TopDownloads) != 3 {
		t.Errorf("expected 3 top downloads, got %d", len(stats.TopDownloads))
	}
	if stats.TopDownloads[0] != "acme/app" {
		t.Errorf("expected acme/app as top download, got %s", stats.TopDownloads[0])
	}
}

func TestIndexer_GetNamespaces(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "myorg/web", Namespace: "myorg"},
		{Name: "acme/app", Namespace: "acme"},
		{Name: "std/base", Namespace: "std"},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	namespaces := idx.GetNamespaces()
	if len(namespaces) != 3 {
		t.Errorf("expected 3 namespaces, got %d", len(namespaces))
	}
	// Should be sorted
	if namespaces[0] != "acme" {
		t.Errorf("expected first namespace to be acme, got %s", namespaces[0])
	}
}

func TestIndexer_GetCategories(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "myorg/web", Namespace: "myorg", Category: "infrastructure"},
		{Name: "acme/app", Namespace: "acme", Category: "application"},
		{Name: "std/base", Namespace: "std", Category: "core"},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	categories := idx.GetCategories()
	if len(categories) != 3 {
		t.Errorf("expected 3 categories, got %d", len(categories))
	}
}

func TestIndexer_GetTags(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "myorg/web", Namespace: "myorg", Tags: []string{"web", "nginx"}},
		{Name: "acme/app", Namespace: "acme", Tags: []string{"app", "web"}},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	tags := idx.GetTags()
	if len(tags) != 3 {
		t.Errorf("expected 3 unique tags, got %d", len(tags))
	}
}

func TestIndexer_GetPopularTags(t *testing.T) {
	idx := NewIndexer()

	entries := []*IndexEntry{
		{Name: "a", Namespace: "a", Tags: []string{"common", "web"}},
		{Name: "b", Namespace: "b", Tags: []string{"common", "api"}},
		{Name: "c", Namespace: "c", Tags: []string{"common", "db"}},
		{Name: "d", Namespace: "d", Tags: []string{"web"}},
	}

	for _, e := range entries {
		idx.Index(e)
	}

	popular := idx.GetPopularTags(2)
	if len(popular) != 2 {
		t.Errorf("expected 2 popular tags, got %d", len(popular))
	}
	if popular[0] != "common" {
		t.Errorf("expected most popular tag to be common, got %s", popular[0])
	}
}

func TestIndexer_Clear(t *testing.T) {
	idx := NewIndexer()

	idx.Index(&IndexEntry{Name: "a", Namespace: "a"})
	idx.Index(&IndexEntry{Name: "b", Namespace: "b"})

	if idx.Count() != 2 {
		t.Errorf("expected 2 entries, got %d", idx.Count())
	}

	idx.Clear()

	if idx.Count() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", idx.Count())
	}
}

func TestIndexer_SaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.json")

	// Create and populate indexer
	idx := NewIndexer()
	entry := &IndexEntry{
		Name:          "myorg/web-stack",
		Namespace:     "myorg",
		ShortName:     "web-stack",
		LatestVersion: "1.0.0",
		AllVersions:   []string{"1.0.0"},
		Description:   "Test blueprint",
		Tags:          []string{"web"},
		Downloads:     100,
	}
	idx.Index(entry)

	// Save
	err := idx.Save(indexPath)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Fatal("index file was not created")
	}

	// Load into new indexer
	idx2 := NewIndexer()
	err = idx2.Load(indexPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify loaded data
	if idx2.Count() != 1 {
		t.Errorf("expected 1 entry after load, got %d", idx2.Count())
	}

	got, ok := idx2.Get("myorg/web-stack")
	if !ok {
		t.Fatal("failed to get loaded entry")
	}
	if got.Description != "Test blueprint" {
		t.Errorf("expected description 'Test blueprint', got '%s'", got.Description)
	}
	if got.Downloads != 100 {
		t.Errorf("expected downloads 100, got %d", got.Downloads)
	}

	// Verify secondary indices rebuilt
	results := idx2.ListByTag("web")
	if len(results) != 1 {
		t.Errorf("expected 1 result for web tag after load, got %d", len(results))
	}
}

func TestIndexer_IncrementDownloads(t *testing.T) {
	idx := NewIndexer()
	idx.Index(&IndexEntry{Name: "a", Namespace: "a", Downloads: 10})

	err := idx.IncrementDownloads("a")
	if err != nil {
		t.Fatalf("IncrementDownloads failed: %v", err)
	}

	entry, _ := idx.Get("a")
	if entry.Downloads != 11 {
		t.Errorf("expected downloads 11, got %d", entry.Downloads)
	}

	// Non-existent
	err = idx.IncrementDownloads("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent entry")
	}
}

func TestIndexer_IncrementDecrementStars(t *testing.T) {
	idx := NewIndexer()
	idx.Index(&IndexEntry{Name: "a", Namespace: "a", Stars: 5})

	err := idx.IncrementStars("a")
	if err != nil {
		t.Fatalf("IncrementStars failed: %v", err)
	}

	entry, _ := idx.Get("a")
	if entry.Stars != 6 {
		t.Errorf("expected stars 6, got %d", entry.Stars)
	}

	err = idx.DecrementStars("a")
	if err != nil {
		t.Fatalf("DecrementStars failed: %v", err)
	}

	entry, _ = idx.Get("a")
	if entry.Stars != 5 {
		t.Errorf("expected stars 5, got %d", entry.Stars)
	}

	// Decrement at 0
	idx.Index(&IndexEntry{Name: "b", Namespace: "b", Stars: 0})
	err = idx.DecrementStars("b")
	if err != nil {
		t.Fatalf("DecrementStars at 0 failed: %v", err)
	}
	entry, _ = idx.Get("b")
	if entry.Stars != 0 {
		t.Errorf("expected stars 0 (no negative), got %d", entry.Stars)
	}
}

func TestIndexer_MarkDeprecated(t *testing.T) {
	idx := NewIndexer()
	idx.Index(&IndexEntry{Name: "a", Namespace: "a"})

	err := idx.MarkDeprecated("a", "Use myorg/web instead")
	if err != nil {
		t.Fatalf("MarkDeprecated failed: %v", err)
	}

	entry, _ := idx.Get("a")
	if !entry.Deprecated {
		t.Error("expected entry to be deprecated")
	}
	if entry.DeprecationMessage != "Use myorg/web instead" {
		t.Errorf("expected deprecation message, got '%s'", entry.DeprecationMessage)
	}
}

func TestIndexer_MarkVerified(t *testing.T) {
	idx := NewIndexer()
	idx.Index(&IndexEntry{Name: "a", Namespace: "a"})

	err := idx.MarkVerified("a", "myorg@example.com")
	if err != nil {
		t.Fatalf("MarkVerified failed: %v", err)
	}

	entry, _ := idx.Get("a")
	if !entry.Verified {
		t.Error("expected entry to be verified")
	}
	if entry.SignatureStatus != "verified" {
		t.Errorf("expected signature status 'verified', got '%s'", entry.SignatureStatus)
	}
	if entry.SignerIdentity != "myorg@example.com" {
		t.Errorf("expected signer identity, got '%s'", entry.SignerIdentity)
	}
}

func TestIndexer_AddVersion(t *testing.T) {
	idx := NewIndexer()
	idx.Index(&IndexEntry{
		Name:          "a",
		Namespace:     "a",
		AllVersions:   []string{"1.0.0"},
		LatestVersion: "1.0.0",
	})

	err := idx.AddVersion("a", "1.1.0")
	if err != nil {
		t.Fatalf("AddVersion failed: %v", err)
	}

	entry, _ := idx.Get("a")
	if len(entry.AllVersions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(entry.AllVersions))
	}
	if entry.LatestVersion != "1.1.0" {
		t.Errorf("expected latest version 1.1.0, got %s", entry.LatestVersion)
	}

	// Add existing version (no-op)
	err = idx.AddVersion("a", "1.0.0")
	if err != nil {
		t.Fatalf("AddVersion existing failed: %v", err)
	}
	entry, _ = idx.Get("a")
	if len(entry.AllVersions) != 2 {
		t.Errorf("expected still 2 versions, got %d", len(entry.AllVersions))
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		minCount int
	}{
		{"single word", []string{"hello"}, 1},
		{"multiple words", []string{"hello world"}, 2},
		{"with separators", []string{"web-stack"}, 2},
		{"with slashes", []string{"myorg/web-stack"}, 3},
		{"short words filtered", []string{"a b cd"}, 1}, // Only cd is >= 2 chars
		{"description", []string{"A complete web application stack"}, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenize(tt.input...)
			if len(tokens) < tt.minCount {
				t.Errorf("expected at least %d tokens, got %d: %v", tt.minCount, len(tokens), tokens)
			}
		})
	}
}

func TestAppendUnique(t *testing.T) {
	slice := []string{"a", "b"}

	result := appendUnique(slice, "c")
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}

	result = appendUnique(result, "a")
	if len(result) != 3 {
		t.Errorf("expected still 3 items after duplicate, got %d", len(result))
	}
}

func TestRemoveFromSlice(t *testing.T) {
	slice := []string{"a", "b", "c"}

	result := removeFromSlice(slice, "b")
	if len(result) != 2 {
		t.Errorf("expected 2 items, got %d", len(result))
	}

	result = removeFromSlice(result, "nonexistent")
	if len(result) != 2 {
		t.Errorf("expected still 2 items after removing nonexistent, got %d", len(result))
	}
}

func TestIndexer_CreatedAtPreserved(t *testing.T) {
	idx := NewIndexer()

	originalTime := time.Now().Add(-24 * time.Hour).UTC()
	entry := &IndexEntry{
		Name:      "a",
		Namespace: "a",
		CreatedAt: originalTime,
	}

	err := idx.Index(entry)
	if err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	got, _ := idx.Get("a")
	if !got.CreatedAt.Equal(originalTime) {
		t.Errorf("expected CreatedAt to be preserved, got %v", got.CreatedAt)
	}

	// Update entry
	entry2 := &IndexEntry{
		Name:      "a",
		Namespace: "a",
		CreatedAt: originalTime,
	}
	err = idx.Index(entry2)
	if err != nil {
		t.Fatalf("Index update failed: %v", err)
	}

	got, _ = idx.Get("a")
	if !got.CreatedAt.Equal(originalTime) {
		t.Errorf("expected CreatedAt to still be preserved after update, got %v", got.CreatedAt)
	}
}
