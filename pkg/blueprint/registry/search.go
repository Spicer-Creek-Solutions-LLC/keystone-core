package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// SearchSort represents a sort field.
type SearchSort string

const (
	SortByRelevance  SearchSort = "relevance"
	SortByName       SearchSort = "name"
	SortByDownloads  SearchSort = "downloads"
	SortByUpdated    SearchSort = "updated"
	SortByCreated    SearchSort = "created"
	SortByPopularity SearchSort = "popularity"
)

// SortOrder represents a sort order.
type SortOrder string

const (
	OrderAsc  SortOrder = "asc"
	OrderDesc SortOrder = "desc"
)

// ExtendedSearchQuery extends SearchQuery with additional filtering options.
type ExtendedSearchQuery struct {
	SearchQuery

	// Namespace filters by namespace/vendor.
	Namespace string

	// Author filters by author.
	Author string

	// VersionConstraint filters by version constraint.
	VersionConstraint string

	// Category filters by category.
	Category string

	// Sort specifies the sort field.
	Sort SearchSort

	// Order specifies the sort order.
	Order SortOrder

	// IncludePrerelease includes prerelease versions.
	IncludePrerelease bool

	// IncludeDeprecated includes deprecated blueprints.
	IncludeDeprecated bool
}

// BlueprintSearchResult represents a single blueprint in search results.
type BlueprintSearchResult struct {
	// Name is the full blueprint name (namespace/name).
	Name string `json:"name"`

	// Namespace is the blueprint namespace.
	Namespace string `json:"namespace"`

	// ShortName is the blueprint name without namespace.
	ShortName string `json:"short_name"`

	// Description is the blueprint description.
	Description string `json:"description"`

	// LatestVersion is the latest version.
	LatestVersion string `json:"latest_version"`

	// Versions is the list of available versions.
	Versions []string `json:"versions,omitempty"`

	// Tags are the blueprint tags.
	Tags []string `json:"tags,omitempty"`

	// Author is the blueprint author.
	Author string `json:"author,omitempty"`

	// Category is the blueprint category.
	Category string `json:"category,omitempty"`

	// Downloads is the download count.
	Downloads int64 `json:"downloads"`

	// Stars is the star count.
	Stars int64 `json:"stars,omitempty"`

	// CreatedAt is when the blueprint was created.
	CreatedAt time.Time `json:"created_at,omitempty"`

	// UpdatedAt is when the blueprint was last updated.
	UpdatedAt time.Time `json:"updated_at,omitempty"`

	// Deprecated indicates if the blueprint is deprecated.
	Deprecated bool `json:"deprecated,omitempty"`

	// DeprecatedMessage is the deprecation message.
	DeprecatedMessage string `json:"deprecated_message,omitempty"`

	// Verified indicates if the blueprint is verified.
	Verified bool `json:"verified,omitempty"`

	// Homepage is the blueprint homepage URL.
	Homepage string `json:"homepage,omitempty"`

	// Repository is the source repository URL.
	Repository string `json:"repository,omitempty"`

	// License is the blueprint license.
	License string `json:"license,omitempty"`

	// Score is the search relevance score.
	Score float64 `json:"score,omitempty"`
}

// ExtendedSearchResult extends SearchResult with additional metadata.
type ExtendedSearchResult struct {
	// Query is the original query.
	Query *ExtendedSearchQuery `json:"query,omitempty"`

	// Results are the search results.
	Results []*BlueprintSearchResult `json:"results"`

	// Total is the total number of matching results.
	Total int `json:"total"`

	// Offset is the current offset.
	Offset int `json:"offset"`

	// Limit is the current limit.
	Limit int `json:"limit"`

	// Took is the query time in milliseconds.
	Took int64 `json:"took_ms"`
}

// SearchClient handles blueprint search operations.
type SearchClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewSearchClient creates a new SearchClient.
func NewSearchClient(baseURL string) *SearchClient {
	return &SearchClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetHTTPClient sets a custom HTTP client.
func (c *SearchClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// Search searches for blueprints.
func (c *SearchClient) Search(ctx context.Context, query *ExtendedSearchQuery) (*ExtendedSearchResult, error) {
	if query == nil {
		query = &ExtendedSearchQuery{}
	}

	// Build query URL
	u, err := url.Parse(c.baseURL + "/-/api/search")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	params := url.Values{}
	if query.Query != "" {
		params.Set("q", query.Query)
	}
	if query.Namespace != "" {
		params.Set("namespace", query.Namespace)
	}
	if len(query.Tags) > 0 {
		params.Set("tags", strings.Join(query.Tags, ","))
	}
	if query.Author != "" {
		params.Set("author", query.Author)
	}
	if query.VersionConstraint != "" {
		params.Set("version", query.VersionConstraint)
	}
	if query.Category != "" {
		params.Set("category", query.Category)
	}
	if query.Sort != "" {
		params.Set("sort", string(query.Sort))
	}
	if query.Order != "" {
		params.Set("order", string(query.Order))
	}
	if query.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", query.Limit))
	}
	if query.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", query.Offset))
	}
	if query.IncludePrerelease {
		params.Set("prerelease", "true")
	}
	if query.IncludeDeprecated {
		params.Set("deprecated", "true")
	}

	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search failed: %s", resp.Status)
	}

	var response ExtendedSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// ListNamespaces lists all available namespaces.
func (c *SearchClient) ListNamespaces(ctx context.Context) ([]string, error) {
	u := c.baseURL + "/-/api/namespaces"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list namespaces failed: %s", resp.Status)
	}

	var namespaces []string
	if err := json.NewDecoder(resp.Body).Decode(&namespaces); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return namespaces, nil
}

// ListCategories lists all available categories.
func (c *SearchClient) ListCategories(ctx context.Context) ([]string, error) {
	u := c.baseURL + "/-/api/categories"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list categories failed: %s", resp.Status)
	}

	var categories []string
	if err := json.NewDecoder(resp.Body).Decode(&categories); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return categories, nil
}

// ListPopularTags lists popular tags.
func (c *SearchClient) ListPopularTags(ctx context.Context, limit int) ([]TagCount, error) {
	u := fmt.Sprintf("%s/-/api/tags?limit=%d", c.baseURL, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list tags failed: %s", resp.Status)
	}

	var tags []TagCount
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return tags, nil
}

// TagCount represents a tag with its count.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// LocalSearcher provides local (in-memory) search capabilities.
type LocalSearcher struct {
	blueprints []*BlueprintSearchResult
}

// NewLocalSearcher creates a new LocalSearcher.
func NewLocalSearcher() *LocalSearcher {
	return &LocalSearcher{
		blueprints: make([]*BlueprintSearchResult, 0),
	}
}

// AddBlueprint adds a blueprint to the local index.
func (s *LocalSearcher) AddBlueprint(bp *BlueprintSearchResult) {
	s.blueprints = append(s.blueprints, bp)
}

// RemoveBlueprint removes a blueprint from the local index.
func (s *LocalSearcher) RemoveBlueprint(name string) {
	for i, bp := range s.blueprints {
		if bp.Name == name {
			s.blueprints = append(s.blueprints[:i], s.blueprints[i+1:]...)
			return
		}
	}
}

// Clear clears the local index.
func (s *LocalSearcher) Clear() {
	s.blueprints = make([]*BlueprintSearchResult, 0)
}

// Count returns the number of indexed blueprints.
func (s *LocalSearcher) Count() int {
	return len(s.blueprints)
}

// Search searches the local index.
func (s *LocalSearcher) Search(query *ExtendedSearchQuery) *ExtendedSearchResult {
	start := time.Now()

	if query == nil {
		query = &ExtendedSearchQuery{}
	}

	// Apply default limit
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}

	// Filter blueprints
	var filtered []*BlueprintSearchResult
	for _, bp := range s.blueprints {
		if s.matches(bp, query) {
			// Calculate score
			bp.Score = s.calculateScore(bp, query)
			filtered = append(filtered, bp)
		}
	}

	// Sort results
	s.sortResults(filtered, query)

	// Apply pagination
	total := len(filtered)
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	var results []*BlueprintSearchResult
	if offset < total {
		results = filtered[offset:end]
	}

	return &ExtendedSearchResult{
		Query:   query,
		Results: results,
		Total:   total,
		Offset:  offset,
		Limit:   limit,
		Took:    time.Since(start).Milliseconds(),
	}
}

// matches checks if a blueprint matches the query.
func (s *LocalSearcher) matches(bp *BlueprintSearchResult, query *ExtendedSearchQuery) bool {
	// Check deprecated
	if bp.Deprecated && !query.IncludeDeprecated {
		return false
	}

	// Check namespace
	if query.Namespace != "" && bp.Namespace != query.Namespace {
		return false
	}

	// Check author
	if query.Author != "" && !strings.EqualFold(bp.Author, query.Author) {
		return false
	}

	// Check category
	if query.Category != "" && !strings.EqualFold(bp.Category, query.Category) {
		return false
	}

	// Check tags (all must match)
	if len(query.Tags) > 0 {
		bpTagsMap := make(map[string]bool)
		for _, t := range bp.Tags {
			bpTagsMap[strings.ToLower(t)] = true
		}
		for _, t := range query.Tags {
			if !bpTagsMap[strings.ToLower(t)] {
				return false
			}
		}
	}

	// Check version constraint
	if query.VersionConstraint != "" {
		constraint, err := ParseConstraintSet(query.VersionConstraint)
		if err == nil {
			matchesVersion := false
			for _, vStr := range bp.Versions {
				v, err := ParseVersion(vStr)
				if err != nil {
					continue
				}
				if !query.IncludePrerelease && !v.IsStable() {
					continue
				}
				if constraint.Matches(v) {
					matchesVersion = true
					break
				}
			}
			if !matchesVersion {
				return false
			}
		}
	}

	// Check search term
	if query.Query != "" {
		term := strings.ToLower(query.Query)
		if !s.termMatches(bp, term) {
			return false
		}
	}

	return true
}

// termMatches checks if the search term matches the blueprint.
func (s *LocalSearcher) termMatches(bp *BlueprintSearchResult, term string) bool {
	// Match against name
	if strings.Contains(strings.ToLower(bp.Name), term) {
		return true
	}
	if strings.Contains(strings.ToLower(bp.ShortName), term) {
		return true
	}

	// Match against description
	if strings.Contains(strings.ToLower(bp.Description), term) {
		return true
	}

	// Match against tags
	for _, tag := range bp.Tags {
		if strings.Contains(strings.ToLower(tag), term) {
			return true
		}
	}

	// Match against author
	if strings.Contains(strings.ToLower(bp.Author), term) {
		return true
	}

	return false
}

// calculateScore calculates the relevance score.
func (s *LocalSearcher) calculateScore(bp *BlueprintSearchResult, query *ExtendedSearchQuery) float64 {
	score := 1.0

	if query.Query != "" {
		term := strings.ToLower(query.Query)

		// Exact name match is highest
		if strings.EqualFold(bp.ShortName, query.Query) {
			score += 100
		} else if strings.EqualFold(bp.Name, query.Query) {
			score += 90
		} else if strings.HasPrefix(strings.ToLower(bp.ShortName), term) {
			score += 50
		} else if strings.Contains(strings.ToLower(bp.ShortName), term) {
			score += 30
		} else if strings.Contains(strings.ToLower(bp.Name), term) {
			score += 20
		} else if strings.Contains(strings.ToLower(bp.Description), term) {
			score += 10
		}
	}

	// Boost verified blueprints
	if bp.Verified {
		score *= 1.5
	}

	// Boost by popularity
	if bp.Downloads > 0 {
		score += float64(bp.Downloads) / 1000
	}
	if bp.Stars > 0 {
		score += float64(bp.Stars) / 10
	}

	return score
}

// sortResults sorts the results based on query.
func (s *LocalSearcher) sortResults(results []*BlueprintSearchResult, query *ExtendedSearchQuery) {
	sortField := query.Sort
	if sortField == "" {
		sortField = SortByRelevance
	}

	order := query.Order
	if order == "" {
		order = OrderDesc
	}

	sort.Slice(results, func(i, j int) bool {
		var less bool

		switch sortField {
		case SortByName:
			less = strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
		case SortByDownloads:
			less = results[i].Downloads < results[j].Downloads
		case SortByUpdated:
			less = results[i].UpdatedAt.Before(results[j].UpdatedAt)
		case SortByCreated:
			less = results[i].CreatedAt.Before(results[j].CreatedAt)
		case SortByPopularity:
			less = results[i].Downloads+results[i].Stars*10 < results[j].Downloads+results[j].Stars*10
		default: // SortByRelevance
			less = results[i].Score < results[j].Score
		}

		if order == OrderDesc {
			return !less
		}
		return less
	})
}

// SearchFilter is a function that filters search results.
type SearchFilter func(*BlueprintSearchResult) bool

// FilterByNamespace returns a filter for namespace.
func FilterByNamespace(namespace string) SearchFilter {
	return func(bp *BlueprintSearchResult) bool {
		return bp.Namespace == namespace
	}
}

// FilterByTag returns a filter for tag.
func FilterByTag(tag string) SearchFilter {
	tag = strings.ToLower(tag)
	return func(bp *BlueprintSearchResult) bool {
		for _, t := range bp.Tags {
			if strings.ToLower(t) == tag {
				return true
			}
		}
		return false
	}
}

// FilterByAuthor returns a filter for author.
func FilterByAuthor(author string) SearchFilter {
	author = strings.ToLower(author)
	return func(bp *BlueprintSearchResult) bool {
		return strings.ToLower(bp.Author) == author
	}
}

// FilterByVerified returns a filter for verified blueprints.
func FilterByVerified(verified bool) SearchFilter {
	return func(bp *BlueprintSearchResult) bool {
		return bp.Verified == verified
	}
}

// FilterByPattern returns a filter that matches name against a pattern.
func FilterByPattern(pattern string) SearchFilter {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return func(bp *BlueprintSearchResult) bool { return false }
	}
	return func(bp *BlueprintSearchResult) bool {
		return re.MatchString(bp.Name) || re.MatchString(bp.ShortName)
	}
}

// ApplyFilters applies multiple filters to results.
func ApplyFilters(results []*BlueprintSearchResult, filters ...SearchFilter) []*BlueprintSearchResult {
	if len(filters) == 0 {
		return results
	}

	var filtered []*BlueprintSearchResult
	for _, bp := range results {
		match := true
		for _, filter := range filters {
			if !filter(bp) {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, bp)
		}
	}

	return filtered
}
