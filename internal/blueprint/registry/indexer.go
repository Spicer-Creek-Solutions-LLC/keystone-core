package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// IndexStats contains index statistics.
type IndexStats struct {
	// TotalBlueprints is the total number of indexed blueprints.
	TotalBlueprints int `json:"total_blueprints"`

	// TotalVersions is the total number of versions across all blueprints.
	TotalVersions int `json:"total_versions"`

	// Namespaces is the count of unique namespaces.
	Namespaces int `json:"namespaces"`

	// Categories maps categories to blueprint counts.
	Categories map[string]int `json:"categories"`

	// Tags maps tags to blueprint counts.
	Tags map[string]int `json:"tags"`

	// TopDownloads lists most downloaded blueprints.
	TopDownloads []string `json:"top_downloads"`

	// TopStarred lists most starred blueprints.
	TopStarred []string `json:"top_starred"`

	// RecentlyUpdated lists recently updated blueprints.
	RecentlyUpdated []string `json:"recently_updated"`

	// LastUpdated is when the index was last updated.
	LastUpdated time.Time `json:"last_updated"`
}

// Indexer manages the blueprint index.
type Indexer struct {
	mu sync.RWMutex

	// entries maps blueprint name to entry.
	entries map[string]*IndexEntry

	// namespaceIndex maps namespace to blueprint names.
	namespaceIndex map[string][]string

	// tagIndex maps tag to blueprint names.
	tagIndex map[string][]string

	// categoryIndex maps category to blueprint names.
	categoryIndex map[string][]string

	// authorIndex maps author to blueprint names.
	authorIndex map[string][]string

	// keywordIndex maps keyword to blueprint names.
	keywordIndex map[string][]string

	// invertedIndex maps tokens to blueprint names for full-text search.
	invertedIndex map[string][]string

	// lastUpdated is when the index was last modified.
	lastUpdated time.Time
}

// NewIndexer creates a new Indexer.
func NewIndexer() *Indexer {
	return &Indexer{
		entries:        make(map[string]*IndexEntry),
		namespaceIndex: make(map[string][]string),
		tagIndex:       make(map[string][]string),
		categoryIndex:  make(map[string][]string),
		authorIndex:    make(map[string][]string),
		keywordIndex:   make(map[string][]string),
		invertedIndex:  make(map[string][]string),
	}
}

// Index adds or updates a blueprint in the index.
func (i *Indexer) Index(entry *IndexEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}
	if entry.Name == "" {
		return fmt.Errorf("entry name is required")
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	// Remove old entry if exists
	if existing, ok := i.entries[entry.Name]; ok {
		i.removeFromIndices(existing)
	}

	// Update timestamp
	entry.UpdatedAt = time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = entry.UpdatedAt
	}

	// Store entry
	i.entries[entry.Name] = entry

	// Update indices
	i.addToIndices(entry)

	i.lastUpdated = time.Now().UTC()
	return nil
}

// addToIndices adds entry to all secondary indices.
func (i *Indexer) addToIndices(entry *IndexEntry) {
	// Namespace index
	if entry.Namespace != "" {
		i.namespaceIndex[entry.Namespace] = appendUnique(i.namespaceIndex[entry.Namespace], entry.Name)
	}

	// Tag index
	for _, tag := range entry.Tags {
		tag = strings.ToLower(tag)
		i.tagIndex[tag] = appendUnique(i.tagIndex[tag], entry.Name)
	}

	// Category index
	if entry.Category != "" {
		cat := strings.ToLower(entry.Category)
		i.categoryIndex[cat] = appendUnique(i.categoryIndex[cat], entry.Name)
	}

	// Author index
	if entry.Author != "" {
		author := strings.ToLower(entry.Author)
		i.authorIndex[author] = appendUnique(i.authorIndex[author], entry.Name)
	}

	// Keyword index
	for _, keyword := range entry.Keywords {
		keyword = strings.ToLower(keyword)
		i.keywordIndex[keyword] = appendUnique(i.keywordIndex[keyword], entry.Name)
	}

	// Full-text inverted index
	tokens := tokenize(entry.Name, entry.Description, entry.ShortName)
	for _, token := range tokens {
		i.invertedIndex[token] = appendUnique(i.invertedIndex[token], entry.Name)
	}
}

// removeFromIndices removes entry from all secondary indices.
func (i *Indexer) removeFromIndices(entry *IndexEntry) {
	// Namespace index
	i.namespaceIndex[entry.Namespace] = removeFromSlice(i.namespaceIndex[entry.Namespace], entry.Name)

	// Tag index
	for _, tag := range entry.Tags {
		tag = strings.ToLower(tag)
		i.tagIndex[tag] = removeFromSlice(i.tagIndex[tag], entry.Name)
	}

	// Category index
	if entry.Category != "" {
		cat := strings.ToLower(entry.Category)
		i.categoryIndex[cat] = removeFromSlice(i.categoryIndex[cat], entry.Name)
	}

	// Author index
	if entry.Author != "" {
		author := strings.ToLower(entry.Author)
		i.authorIndex[author] = removeFromSlice(i.authorIndex[author], entry.Name)
	}

	// Keyword index
	for _, keyword := range entry.Keywords {
		keyword = strings.ToLower(keyword)
		i.keywordIndex[keyword] = removeFromSlice(i.keywordIndex[keyword], entry.Name)
	}

	// Full-text inverted index
	tokens := tokenize(entry.Name, entry.Description, entry.ShortName)
	for _, token := range tokens {
		i.invertedIndex[token] = removeFromSlice(i.invertedIndex[token], entry.Name)
	}
}

// Remove removes a blueprint from the index.
func (i *Indexer) Remove(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("blueprint not found: %s", name)
	}

	i.removeFromIndices(entry)
	delete(i.entries, name)
	i.lastUpdated = time.Now().UTC()
	return nil
}

// Get returns a blueprint entry by name.
func (i *Indexer) Get(name string) (*IndexEntry, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	entry, ok := i.entries[name]
	if !ok {
		return nil, false
	}
	return entry, true
}

// Search performs a full-text search across the index.
func (i *Indexer) Search(query string) []*IndexEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if query == "" {
		// Return all entries
		results := make([]*IndexEntry, 0, len(i.entries))
		for _, entry := range i.entries {
			results = append(results, entry)
		}
		return results
	}

	// Tokenize query
	tokens := tokenize(query)

	// Find matching entries using inverted index
	matches := make(map[string]int) // name -> match count
	for _, token := range tokens {
		if names, ok := i.invertedIndex[token]; ok {
			for _, name := range names {
				matches[name]++
			}
		}
	}

	// Also search in keywords, tags, and categories
	queryLower := strings.ToLower(query)
	if names, ok := i.tagIndex[queryLower]; ok {
		for _, name := range names {
			matches[name] += 2 // Boost tag matches
		}
	}
	if names, ok := i.categoryIndex[queryLower]; ok {
		for _, name := range names {
			matches[name] += 2 // Boost category matches
		}
	}
	if names, ok := i.keywordIndex[queryLower]; ok {
		for _, name := range names {
			matches[name] += 2 // Boost keyword matches
		}
	}

	// Sort by match count
	type scored struct {
		name  string
		score int
	}
	scoredResults := make([]scored, 0, len(matches))
	for name, count := range matches {
		scoredResults = append(scoredResults, scored{name, count})
	}
	sort.Slice(scoredResults, func(a, b int) bool {
		return scoredResults[a].score > scoredResults[b].score
	})

	// Build result list
	results := make([]*IndexEntry, 0, len(scoredResults))
	for _, sr := range scoredResults {
		if entry, ok := i.entries[sr.name]; ok {
			results = append(results, entry)
		}
	}

	return results
}

// ListByNamespace returns blueprints in a namespace.
func (i *Indexer) ListByNamespace(namespace string) []*IndexEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()

	names := i.namespaceIndex[namespace]
	results := make([]*IndexEntry, 0, len(names))
	for _, name := range names {
		if entry, ok := i.entries[name]; ok {
			results = append(results, entry)
		}
	}
	return results
}

// ListByTag returns blueprints with a specific tag.
func (i *Indexer) ListByTag(tag string) []*IndexEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()

	tag = strings.ToLower(tag)
	names := i.tagIndex[tag]
	results := make([]*IndexEntry, 0, len(names))
	for _, name := range names {
		if entry, ok := i.entries[name]; ok {
			results = append(results, entry)
		}
	}
	return results
}

// ListByCategory returns blueprints in a category.
func (i *Indexer) ListByCategory(category string) []*IndexEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()

	category = strings.ToLower(category)
	names := i.categoryIndex[category]
	results := make([]*IndexEntry, 0, len(names))
	for _, name := range names {
		if entry, ok := i.entries[name]; ok {
			results = append(results, entry)
		}
	}
	return results
}

// ListByAuthor returns blueprints by an author.
func (i *Indexer) ListByAuthor(author string) []*IndexEntry {
	i.mu.RLock()
	defer i.mu.RUnlock()

	author = strings.ToLower(author)
	names := i.authorIndex[author]
	results := make([]*IndexEntry, 0, len(names))
	for _, name := range names {
		if entry, ok := i.entries[name]; ok {
			results = append(results, entry)
		}
	}
	return results
}

// GetStats returns index statistics.
func (i *Indexer) GetStats() *IndexStats {
	i.mu.RLock()
	defer i.mu.RUnlock()

	stats := &IndexStats{
		TotalBlueprints: len(i.entries),
		Namespaces:      len(i.namespaceIndex),
		Categories:      make(map[string]int),
		Tags:            make(map[string]int),
		LastUpdated:     i.lastUpdated,
	}

	// Count total versions
	for _, entry := range i.entries {
		stats.TotalVersions += len(entry.AllVersions)
	}

	// Category counts
	for cat, names := range i.categoryIndex {
		stats.Categories[cat] = len(names)
	}

	// Tag counts
	for tag, names := range i.tagIndex {
		stats.Tags[tag] = len(names)
	}

	// Top downloads
	byDownloads := make([]*IndexEntry, 0, len(i.entries))
	for _, entry := range i.entries {
		byDownloads = append(byDownloads, entry)
	}
	sort.Slice(byDownloads, func(a, b int) bool {
		return byDownloads[a].Downloads > byDownloads[b].Downloads
	})
	limit := 10
	if len(byDownloads) < limit {
		limit = len(byDownloads)
	}
	stats.TopDownloads = make([]string, limit)
	for j := 0; j < limit; j++ {
		stats.TopDownloads[j] = byDownloads[j].Name
	}

	// Top starred
	byStars := make([]*IndexEntry, 0, len(i.entries))
	for _, entry := range i.entries {
		byStars = append(byStars, entry)
	}
	sort.Slice(byStars, func(a, b int) bool {
		return byStars[a].Stars > byStars[b].Stars
	})
	if len(byStars) < limit {
		limit = len(byStars)
	}
	stats.TopStarred = make([]string, limit)
	for j := 0; j < limit; j++ {
		stats.TopStarred[j] = byStars[j].Name
	}

	// Recently updated
	byUpdated := make([]*IndexEntry, 0, len(i.entries))
	for _, entry := range i.entries {
		byUpdated = append(byUpdated, entry)
	}
	sort.Slice(byUpdated, func(a, b int) bool {
		return byUpdated[a].UpdatedAt.After(byUpdated[b].UpdatedAt)
	})
	if len(byUpdated) < limit {
		limit = len(byUpdated)
	}
	stats.RecentlyUpdated = make([]string, limit)
	for j := 0; j < limit; j++ {
		stats.RecentlyUpdated[j] = byUpdated[j].Name
	}

	return stats
}

// GetNamespaces returns all namespaces.
func (i *Indexer) GetNamespaces() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	namespaces := make([]string, 0, len(i.namespaceIndex))
	for ns := range i.namespaceIndex {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	return namespaces
}

// GetCategories returns all categories.
func (i *Indexer) GetCategories() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	categories := make([]string, 0, len(i.categoryIndex))
	for cat := range i.categoryIndex {
		categories = append(categories, cat)
	}
	sort.Strings(categories)
	return categories
}

// GetTags returns all tags.
func (i *Indexer) GetTags() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	tags := make([]string, 0, len(i.tagIndex))
	for tag := range i.tagIndex {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// GetPopularTags returns the most used tags.
func (i *Indexer) GetPopularTags(limit int) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	type tagCount struct {
		tag   string
		count int
	}

	counts := make([]tagCount, 0, len(i.tagIndex))
	for tag, names := range i.tagIndex {
		counts = append(counts, tagCount{tag, len(names)})
	}

	sort.Slice(counts, func(a, b int) bool {
		return counts[a].count > counts[b].count
	})

	if limit > len(counts) {
		limit = len(counts)
	}

	result := make([]string, limit)
	for j := 0; j < limit; j++ {
		result[j] = counts[j].tag
	}
	return result
}

// Count returns the number of indexed blueprints.
func (i *Indexer) Count() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.entries)
}

// Clear removes all entries from the index.
func (i *Indexer) Clear() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.entries = make(map[string]*IndexEntry)
	i.namespaceIndex = make(map[string][]string)
	i.tagIndex = make(map[string][]string)
	i.categoryIndex = make(map[string][]string)
	i.authorIndex = make(map[string][]string)
	i.keywordIndex = make(map[string][]string)
	i.invertedIndex = make(map[string][]string)
	i.lastUpdated = time.Now().UTC()
}

// IndexData is the serializable index state.
type IndexData struct {
	Entries     []*IndexEntry `json:"entries"`
	LastUpdated time.Time     `json:"last_updated"`
	Version     string        `json:"version"`
}

// Save persists the index to a file.
func (i *Indexer) Save(path string) error {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	//nolint:gosec // G301: index directory needs to be accessible by service user
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Build data
	data := &IndexData{
		Entries:     make([]*IndexEntry, 0, len(i.entries)),
		LastUpdated: i.lastUpdated,
		Version:     "1.0",
	}
	for _, entry := range i.entries {
		data.Entries = append(data.Entries, entry)
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	// Write to temp file first
	tmpPath := path + ".tmp"
	//nolint:gosec // G306: index files need to be readable by registry clients
	if err := os.WriteFile(tmpPath, jsonData, 0o644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	// Rename to final path
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename index file: %w", err)
	}

	return nil
}

// Load reads the index from a file.
func (i *Indexer) Load(path string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Read file
	jsonData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read index: %w", err)
	}

	// Unmarshal JSON
	var data IndexData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal index: %w", err)
	}

	// Clear existing index
	i.entries = make(map[string]*IndexEntry)
	i.namespaceIndex = make(map[string][]string)
	i.tagIndex = make(map[string][]string)
	i.categoryIndex = make(map[string][]string)
	i.authorIndex = make(map[string][]string)
	i.keywordIndex = make(map[string][]string)
	i.invertedIndex = make(map[string][]string)

	// Rebuild index
	for _, entry := range data.Entries {
		i.entries[entry.Name] = entry
		i.addToIndices(entry)
	}

	i.lastUpdated = data.LastUpdated
	return nil
}

// IncrementDownloads increments the download count for a blueprint.
func (i *Indexer) IncrementDownloads(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("blueprint not found: %s", name)
	}

	entry.Downloads++
	entry.UpdatedAt = time.Now().UTC()
	i.lastUpdated = entry.UpdatedAt
	return nil
}

// IncrementStars increments the star count for a blueprint.
func (i *Indexer) IncrementStars(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("blueprint not found: %s", name)
	}

	entry.Stars++
	entry.UpdatedAt = time.Now().UTC()
	i.lastUpdated = entry.UpdatedAt
	return nil
}

// DecrementStars decrements the star count for a blueprint.
func (i *Indexer) DecrementStars(name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("blueprint not found: %s", name)
	}

	if entry.Stars > 0 {
		entry.Stars--
	}
	entry.UpdatedAt = time.Now().UTC()
	i.lastUpdated = entry.UpdatedAt
	return nil
}

// MarkDeprecated marks a blueprint as deprecated.
func (i *Indexer) MarkDeprecated(name, message string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("blueprint not found: %s", name)
	}

	entry.Deprecated = true
	entry.DeprecationMessage = message
	entry.UpdatedAt = time.Now().UTC()
	i.lastUpdated = entry.UpdatedAt
	return nil
}

// MarkVerified marks a blueprint as verified.
func (i *Indexer) MarkVerified(name, signerIdentity string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("blueprint not found: %s", name)
	}

	entry.Verified = true
	entry.SignatureStatus = "verified"
	entry.SignerIdentity = signerIdentity
	entry.UpdatedAt = time.Now().UTC()
	i.lastUpdated = entry.UpdatedAt
	return nil
}

// AddVersion adds a new version to a blueprint.
func (i *Indexer) AddVersion(name, version string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	entry, ok := i.entries[name]
	if !ok {
		return fmt.Errorf("blueprint not found: %s", name)
	}

	// Check if version already exists
	for _, v := range entry.AllVersions {
		if v == version {
			return nil // Already exists
		}
	}

	entry.AllVersions = append(entry.AllVersions, version)

	// Update latest version
	entry.LatestVersion = findLatestVersion(entry.AllVersions)
	entry.UpdatedAt = time.Now().UTC()
	i.lastUpdated = entry.UpdatedAt
	return nil
}

// Helper functions

// appendUnique appends value to slice if not already present.
func appendUnique(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice
		}
	}
	return append(slice, value)
}

// removeFromSlice removes value from slice.
func removeFromSlice(slice []string, value string) []string {
	result := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != value {
			result = append(result, v)
		}
	}
	return result
}

// tokenize breaks text into searchable tokens.
func tokenize(texts ...string) []string {
	tokens := make(map[string]struct{})
	splitter := regexp.MustCompile(`[\s\-_/.,;:!?()[\]{}'"]+`)

	for _, text := range texts {
		text = strings.ToLower(text)
		words := splitter.Split(text, -1)
		for _, word := range words {
			word = strings.TrimSpace(word)
			if len(word) >= 2 { // Minimum token length
				tokens[word] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(tokens))
	for token := range tokens {
		result = append(result, token)
	}
	return result
}

// findLatestVersion finds the latest semantic version from a list.
func findLatestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}

	// Parse and sort versions
	sorted := SortVersions(versions)
	if len(sorted) > 0 {
		return sorted[0] // SortVersions returns newest first
	}
	return versions[0]
}
