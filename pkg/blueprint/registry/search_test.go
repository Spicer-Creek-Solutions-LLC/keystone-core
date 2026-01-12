package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSearchClient(t *testing.T) {
	client := NewSearchClient("https://registry.example.com")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.baseURL != "https://registry.example.com" {
		t.Errorf("baseURL = %s, want https://registry.example.com", client.baseURL)
	}
}

func TestSearchClient_Search(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/api/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Check query parameters
		q := r.URL.Query()
		if q.Get("q") != "nginx" {
			t.Errorf("q = %s, want nginx", q.Get("q"))
		}

		response := ExtendedSearchResult{
			Results: []*BlueprintSearchResult{
				{
					Name:          "official/nginx",
					Namespace:     "official",
					ShortName:     "nginx",
					Description:   "NGINX web server",
					LatestVersion: "1.0.0",
					Downloads:     1000,
				},
			},
			Total:  1,
			Offset: 0,
			Limit:  20,
			Took:   5,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewSearchClient(server.URL)

	resp, err := client.Search(context.Background(), &ExtendedSearchQuery{
		SearchQuery: SearchQuery{Query: "nginx"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if resp.Total != 1 {
		t.Errorf("Total = %d, want 1", resp.Total)
	}
	if len(resp.Results) != 1 {
		t.Errorf("len(Results) = %d, want 1", len(resp.Results))
	}
	if resp.Results[0].Name != "official/nginx" {
		t.Errorf("Name = %s, want official/nginx", resp.Results[0].Name)
	}
}

func TestSearchClient_ListNamespaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/api/namespaces" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		namespaces := []string{"official", "community", "enterprise"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(namespaces)
	}))
	defer server.Close()

	client := NewSearchClient(server.URL)

	namespaces, err := client.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces failed: %v", err)
	}

	if len(namespaces) != 3 {
		t.Errorf("len(namespaces) = %d, want 3", len(namespaces))
	}
}

func TestSearchClient_ListCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/api/categories" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		categories := []string{"web", "database", "monitoring"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(categories)
	}))
	defer server.Close()

	client := NewSearchClient(server.URL)

	categories, err := client.ListCategories(context.Background())
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}

	if len(categories) != 3 {
		t.Errorf("len(categories) = %d, want 3", len(categories))
	}
}

func TestSearchClient_ListPopularTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/-/api/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		tags := []TagCount{
			{Tag: "linux", Count: 100},
			{Tag: "web", Count: 80},
			{Tag: "docker", Count: 60},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tags)
	}))
	defer server.Close()

	client := NewSearchClient(server.URL)

	tags, err := client.ListPopularTags(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListPopularTags failed: %v", err)
	}

	if len(tags) != 3 {
		t.Errorf("len(tags) = %d, want 3", len(tags))
	}
	if tags[0].Tag != "linux" {
		t.Errorf("tags[0].Tag = %s, want linux", tags[0].Tag)
	}
	if tags[0].Count != 100 {
		t.Errorf("tags[0].Count = %d, want 100", tags[0].Count)
	}
}

func TestLocalSearcher_Basic(t *testing.T) {
	searcher := NewLocalSearcher()

	if searcher.Count() != 0 {
		t.Errorf("Count() = %d, want 0", searcher.Count())
	}

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:          "official/nginx",
		Namespace:     "official",
		ShortName:     "nginx",
		Description:   "NGINX web server blueprint",
		LatestVersion: "1.0.0",
		Versions:      []string{"1.0.0", "0.9.0"},
		Tags:          []string{"web", "linux"},
		Author:        "admin",
		Downloads:     1000,
	})

	if searcher.Count() != 1 {
		t.Errorf("Count() = %d, want 1", searcher.Count())
	}

	searcher.RemoveBlueprint("official/nginx")
	if searcher.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after remove", searcher.Count())
	}
}

func TestLocalSearcher_Search(t *testing.T) {
	searcher := NewLocalSearcher()

	now := time.Now()

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:          "official/nginx",
		Namespace:     "official",
		ShortName:     "nginx",
		Description:   "NGINX web server blueprint",
		LatestVersion: "1.0.0",
		Versions:      []string{"1.0.0", "0.9.0"},
		Tags:          []string{"web", "linux"},
		Author:        "admin",
		Category:      "web",
		Downloads:     1000,
		CreatedAt:     now.Add(-24 * time.Hour),
		UpdatedAt:     now,
	})

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:          "community/postgres",
		Namespace:     "community",
		ShortName:     "postgres",
		Description:   "PostgreSQL database blueprint",
		LatestVersion: "2.0.0",
		Versions:      []string{"2.0.0", "1.5.0"},
		Tags:          []string{"database", "linux"},
		Author:        "user1",
		Category:      "database",
		Downloads:     500,
		CreatedAt:     now.Add(-48 * time.Hour),
		UpdatedAt:     now.Add(-12 * time.Hour),
	})

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:          "official/redis",
		Namespace:     "official",
		ShortName:     "redis",
		Description:   "Redis cache blueprint",
		LatestVersion: "1.0.0",
		Versions:      []string{"1.0.0"},
		Tags:          []string{"cache", "linux"},
		Author:        "admin",
		Category:      "database",
		Downloads:     750,
		Deprecated:    true,
		CreatedAt:     now.Add(-72 * time.Hour),
		UpdatedAt:     now.Add(-24 * time.Hour),
	})

	tests := []struct {
		name     string
		query    *ExtendedSearchQuery
		wantLen  int
		wantName string
	}{
		{
			name:     "empty query",
			query:    &ExtendedSearchQuery{},
			wantLen:  2, // redis is deprecated
			wantName: "official/nginx",
		},
		{
			name:     "search by term",
			query:    &ExtendedSearchQuery{SearchQuery: SearchQuery{Query: "nginx"}},
			wantLen:  1,
			wantName: "official/nginx",
		},
		{
			name:     "search by namespace",
			query:    &ExtendedSearchQuery{Namespace: "official"},
			wantLen:  1, // redis is deprecated
			wantName: "official/nginx",
		},
		{
			name:     "search by tag",
			query:    &ExtendedSearchQuery{SearchQuery: SearchQuery{Tags: []string{"database"}}},
			wantLen:  1,
			wantName: "community/postgres",
		},
		{
			name:     "search by author",
			query:    &ExtendedSearchQuery{Author: "admin"},
			wantLen:  1, // redis is deprecated
			wantName: "official/nginx",
		},
		{
			name:     "search by category",
			query:    &ExtendedSearchQuery{Category: "database"},
			wantLen:  1, // redis is deprecated
			wantName: "community/postgres",
		},
		{
			name:     "include deprecated",
			query:    &ExtendedSearchQuery{IncludeDeprecated: true},
			wantLen:  3,
			wantName: "official/nginx",
		},
		{
			name:     "search deprecated by namespace",
			query:    &ExtendedSearchQuery{Namespace: "official", IncludeDeprecated: true},
			wantLen:  2,
			wantName: "official/nginx",
		},
		{
			name:    "no matches",
			query:   &ExtendedSearchQuery{SearchQuery: SearchQuery{Query: "nonexistent"}},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := searcher.Search(tt.query)

			if len(resp.Results) != tt.wantLen {
				t.Errorf("len(Results) = %d, want %d", len(resp.Results), tt.wantLen)
			}
			if tt.wantLen > 0 && resp.Results[0].Name != tt.wantName {
				t.Errorf("Results[0].Name = %s, want %s", resp.Results[0].Name, tt.wantName)
			}
		})
	}
}

func TestLocalSearcher_Search_Sorting(t *testing.T) {
	searcher := NewLocalSearcher()

	now := time.Now()

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:      "a/first",
		Namespace: "a",
		ShortName: "first",
		Downloads: 100,
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now,
	})

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:      "b/second",
		Namespace: "b",
		ShortName: "second",
		Downloads: 500,
		CreatedAt: now.Add(-48 * time.Hour),
		UpdatedAt: now.Add(-12 * time.Hour),
	})

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:      "c/third",
		Namespace: "c",
		ShortName: "third",
		Downloads: 300,
		CreatedAt: now.Add(-72 * time.Hour),
		UpdatedAt: now.Add(-24 * time.Hour),
	})

	tests := []struct {
		name      string
		sort      SearchSort
		order     SortOrder
		wantFirst string
	}{
		{
			name:      "sort by name asc",
			sort:      SortByName,
			order:     OrderAsc,
			wantFirst: "a/first",
		},
		{
			name:      "sort by name desc",
			sort:      SortByName,
			order:     OrderDesc,
			wantFirst: "c/third",
		},
		{
			name:      "sort by downloads desc",
			sort:      SortByDownloads,
			order:     OrderDesc,
			wantFirst: "b/second",
		},
		{
			name:      "sort by downloads asc",
			sort:      SortByDownloads,
			order:     OrderAsc,
			wantFirst: "a/first",
		},
		{
			name:      "sort by created asc",
			sort:      SortByCreated,
			order:     OrderAsc,
			wantFirst: "c/third",
		},
		{
			name:      "sort by updated desc",
			sort:      SortByUpdated,
			order:     OrderDesc,
			wantFirst: "a/first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := searcher.Search(&ExtendedSearchQuery{
				Sort:  tt.sort,
				Order: tt.order,
			})

			if len(resp.Results) == 0 {
				t.Fatal("expected results")
			}
			if resp.Results[0].Name != tt.wantFirst {
				t.Errorf("first result = %s, want %s", resp.Results[0].Name, tt.wantFirst)
			}
		})
	}
}

func TestLocalSearcher_Search_Pagination(t *testing.T) {
	searcher := NewLocalSearcher()

	for i := 0; i < 50; i++ {
		searcher.AddBlueprint(&BlueprintSearchResult{
			Name:      fmt.Sprintf("ns/bp%02d", i),
			Namespace: "ns",
			ShortName: fmt.Sprintf("bp%02d", i),
		})
	}

	tests := []struct {
		name       string
		limit      int
		offset     int
		wantLen    int
		wantTotal  int
		wantOffset int
	}{
		{
			name:       "default limit",
			limit:      0,
			offset:     0,
			wantLen:    20,
			wantTotal:  50,
			wantOffset: 0,
		},
		{
			name:       "custom limit",
			limit:      10,
			offset:     0,
			wantLen:    10,
			wantTotal:  50,
			wantOffset: 0,
		},
		{
			name:       "with offset",
			limit:      10,
			offset:     40,
			wantLen:    10,
			wantTotal:  50,
			wantOffset: 40,
		},
		{
			name:       "offset past end",
			limit:      10,
			offset:     45,
			wantLen:    5,
			wantTotal:  50,
			wantOffset: 45,
		},
		{
			name:       "offset at end",
			limit:      10,
			offset:     50,
			wantLen:    0,
			wantTotal:  50,
			wantOffset: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := searcher.Search(&ExtendedSearchQuery{
				SearchQuery: SearchQuery{
					Limit:  tt.limit,
					Offset: tt.offset,
				},
			})

			if len(resp.Results) != tt.wantLen {
				t.Errorf("len(Results) = %d, want %d", len(resp.Results), tt.wantLen)
			}
			if resp.Total != tt.wantTotal {
				t.Errorf("Total = %d, want %d", resp.Total, tt.wantTotal)
			}
			if resp.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", resp.Offset, tt.wantOffset)
			}
		})
	}
}

func TestLocalSearcher_Search_VersionConstraint(t *testing.T) {
	searcher := NewLocalSearcher()

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:          "ns/v1only",
		Namespace:     "ns",
		ShortName:     "v1only",
		LatestVersion: "1.5.0",
		Versions:      []string{"1.5.0", "1.0.0"},
	})

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:          "ns/v2only",
		Namespace:     "ns",
		ShortName:     "v2only",
		LatestVersion: "2.0.0",
		Versions:      []string{"2.0.0"},
	})

	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:          "ns/both",
		Namespace:     "ns",
		ShortName:     "both",
		LatestVersion: "2.1.0",
		Versions:      []string{"2.1.0", "1.9.0"},
	})

	tests := []struct {
		name       string
		constraint string
		wantLen    int
	}{
		{
			name:       ">=2.0.0",
			constraint: ">=2.0.0",
			wantLen:    2,
		},
		{
			name:       "<2.0.0",
			constraint: "<2.0.0",
			wantLen:    2,
		},
		{
			name:       "^1.0.0",
			constraint: "^1.0.0",
			wantLen:    2,
		},
		{
			name:       "=2.0.0",
			constraint: "=2.0.0",
			wantLen:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := searcher.Search(&ExtendedSearchQuery{
				VersionConstraint: tt.constraint,
			})

			if len(resp.Results) != tt.wantLen {
				t.Errorf("len(Results) = %d, want %d", len(resp.Results), tt.wantLen)
			}
		})
	}
}

func TestLocalSearcher_Clear(t *testing.T) {
	searcher := NewLocalSearcher()

	searcher.AddBlueprint(&BlueprintSearchResult{Name: "a/b"})
	searcher.AddBlueprint(&BlueprintSearchResult{Name: "c/d"})

	if searcher.Count() != 2 {
		t.Errorf("Count() = %d, want 2", searcher.Count())
	}

	searcher.Clear()

	if searcher.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after clear", searcher.Count())
	}
}

func TestSearchFilters(t *testing.T) {
	results := []*BlueprintSearchResult{
		{Name: "ns1/nginx", Namespace: "ns1", Tags: []string{"web"}, Author: "admin", Verified: true},
		{Name: "ns2/postgres", Namespace: "ns2", Tags: []string{"database"}, Author: "user1", Verified: false},
		{Name: "ns1/redis", Namespace: "ns1", Tags: []string{"cache", "database"}, Author: "admin", Verified: true},
	}

	tests := []struct {
		name    string
		filters []SearchFilter
		wantLen int
	}{
		{
			name:    "filter by namespace",
			filters: []SearchFilter{FilterByNamespace("ns1")},
			wantLen: 2,
		},
		{
			name:    "filter by tag",
			filters: []SearchFilter{FilterByTag("database")},
			wantLen: 2,
		},
		{
			name:    "filter by author",
			filters: []SearchFilter{FilterByAuthor("admin")},
			wantLen: 2,
		},
		{
			name:    "filter by verified",
			filters: []SearchFilter{FilterByVerified(true)},
			wantLen: 2,
		},
		{
			name:    "multiple filters",
			filters: []SearchFilter{FilterByNamespace("ns1"), FilterByTag("cache")},
			wantLen: 1,
		},
		{
			name:    "filter by pattern",
			filters: []SearchFilter{FilterByPattern("^ns1/")},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := ApplyFilters(results, tt.filters...)
			if len(filtered) != tt.wantLen {
				t.Errorf("len(filtered) = %d, want %d", len(filtered), tt.wantLen)
			}
		})
	}
}

func TestSearchSortConstants(t *testing.T) {
	// Verify sort constants
	if SortByRelevance != "relevance" {
		t.Errorf("SortByRelevance = %s, want relevance", SortByRelevance)
	}
	if SortByName != "name" {
		t.Errorf("SortByName = %s, want name", SortByName)
	}
	if SortByDownloads != "downloads" {
		t.Errorf("SortByDownloads = %s, want downloads", SortByDownloads)
	}
	if SortByUpdated != "updated" {
		t.Errorf("SortByUpdated = %s, want updated", SortByUpdated)
	}
	if SortByCreated != "created" {
		t.Errorf("SortByCreated = %s, want created", SortByCreated)
	}
	if SortByPopularity != "popularity" {
		t.Errorf("SortByPopularity = %s, want popularity", SortByPopularity)
	}
}

func TestSortOrderConstants(t *testing.T) {
	if OrderAsc != "asc" {
		t.Errorf("OrderAsc = %s, want asc", OrderAsc)
	}
	if OrderDesc != "desc" {
		t.Errorf("OrderDesc = %s, want desc", OrderDesc)
	}
}

func TestSearchClient_SetHTTPClient(t *testing.T) {
	client := NewSearchClient("https://example.com")

	customClient := &http.Client{Timeout: 60 * time.Second}
	client.SetHTTPClient(customClient)

	if client.httpClient != customClient {
		t.Error("httpClient not set")
	}
}

func TestLocalSearcher_ScoreCalculation(t *testing.T) {
	searcher := NewLocalSearcher()

	// Blueprint with exact name match should score highest
	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:      "ns/nginx",
		Namespace: "ns",
		ShortName: "nginx",
		Downloads: 100,
		Verified:  true,
	})

	// Blueprint with partial match
	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:        "ns/nginx-proxy",
		Namespace:   "ns",
		ShortName:   "nginx-proxy",
		Description: "NGINX reverse proxy",
		Downloads:   500,
		Verified:    false,
	})

	// Blueprint with only description match
	searcher.AddBlueprint(&BlueprintSearchResult{
		Name:        "ns/webserver",
		Namespace:   "ns",
		ShortName:   "webserver",
		Description: "nginx based webserver",
		Downloads:   200,
		Verified:    false,
	})

	resp := searcher.Search(&ExtendedSearchQuery{
		SearchQuery: SearchQuery{Query: "nginx"},
		Sort:        SortByRelevance,
	})

	if len(resp.Results) != 3 {
		t.Fatalf("len(Results) = %d, want 3", len(resp.Results))
	}

	// Exact match should be first (even with lower downloads) due to verification boost and name match
	if resp.Results[0].ShortName != "nginx" {
		t.Errorf("first result = %s, want nginx", resp.Results[0].ShortName)
	}
}
