package statemgmt

import (
	"strings"
	"testing"
)

func TestGraphVisualizer_BasicGraph(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"package": {
				{
					ID:     "nginx",
					Module: "package",
					State:  "installed",
				},
			},
			"file": {
				{
					ID:     "/etc/nginx/nginx.conf",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"},
						},
					},
				},
			},
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"},
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
					},
				},
			},
		},
	}

	viz := NewGraphVisualizer()
	err := viz.BuildFromStateFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	graph := viz.GetGraph()

	// Check node count
	if graph.TotalNodes != 3 {
		t.Errorf("Expected 3 nodes, got %d", graph.TotalNodes)
	}

	// Check edge count (package->file, package->service, file->service)
	if graph.TotalEdges != 3 {
		t.Errorf("Expected 3 edges, got %d", graph.TotalEdges)
	}

	// Should not have cycles
	if graph.HasCycle {
		t.Error("Graph should not have cycles")
	}

	// Should have 3 levels (package -> file -> service)
	if len(graph.Levels) != 3 {
		t.Errorf("Expected 3 levels, got %d", len(graph.Levels))
	}
}

func TestGraphVisualizer_RenderDOT(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "A",
					Module: "file",
					State:  "present",
				},
				{
					ID:     "B",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "A"},
						},
					},
				},
			},
		},
	}

	output, err := VisualizeDependencies(stateFile, GraphFormatDOT)
	if err != nil {
		t.Fatalf("Failed to visualize: %v", err)
	}

	// Check for DOT format elements
	if !strings.Contains(output, "digraph StateDependencies") {
		t.Error("DOT output should contain digraph declaration")
	}

	if !strings.Contains(output, "rankdir=TB") {
		t.Error("DOT output should contain rankdir")
	}

	if !strings.Contains(output, "->") {
		t.Error("DOT output should contain edges")
	}

	if !strings.Contains(output, "subgraph cluster") {
		t.Error("DOT output should contain level subgraphs")
	}
}

func TestGraphVisualizer_RenderMermaid(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "A",
					Module: "file",
					State:  "present",
				},
				{
					ID:     "B",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "A"},
						},
					},
				},
			},
		},
	}

	output, err := VisualizeDependencies(stateFile, GraphFormatMermaid)
	if err != nil {
		t.Fatalf("Failed to visualize: %v", err)
	}

	// Check for Mermaid format elements
	if !strings.Contains(output, "```mermaid") {
		t.Error("Mermaid output should contain mermaid code fence")
	}

	if !strings.Contains(output, "flowchart TD") {
		t.Error("Mermaid output should contain flowchart declaration")
	}

	if !strings.Contains(output, "subgraph") {
		t.Error("Mermaid output should contain subgraphs")
	}

	if !strings.Contains(output, "-->") {
		t.Error("Mermaid output should contain arrows")
	}
}

func TestGraphVisualizer_RenderText(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "A",
					Module: "file",
					State:  "present",
				},
				{
					ID:     "B",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "A"},
						},
					},
				},
			},
		},
	}

	output, err := VisualizeDependencies(stateFile, GraphFormatText)
	if err != nil {
		t.Fatalf("Failed to visualize: %v", err)
	}

	// Check for text format elements
	if !strings.Contains(output, "State Dependency Graph") {
		t.Error("Text output should contain header")
	}

	if !strings.Contains(output, "Total States: 2") {
		t.Error("Text output should show total states")
	}

	if !strings.Contains(output, "Execution Order") {
		t.Error("Text output should show execution order")
	}

	if !strings.Contains(output, "Level 0") {
		t.Error("Text output should show levels")
	}
}

func TestGraphVisualizer_RenderJSON(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "A",
					Module: "file",
					State:  "present",
				},
				{
					ID:     "B",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "A"},
						},
					},
				},
			},
		},
	}

	output, err := VisualizeDependencies(stateFile, GraphFormatJSON)
	if err != nil {
		t.Fatalf("Failed to visualize: %v", err)
	}

	// Check for JSON format elements
	if !strings.Contains(output, "\"total_nodes\": 2") {
		t.Error("JSON output should contain total_nodes")
	}

	if !strings.Contains(output, "\"nodes\":") {
		t.Error("JSON output should contain nodes array")
	}

	if !strings.Contains(output, "\"edges\":") {
		t.Error("JSON output should contain edges array")
	}

	if !strings.Contains(output, "\"levels\":") {
		t.Error("JSON output should contain levels array")
	}
}

func TestGraphVisualizer_EdgeTypes(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "config",
					Module: "file",
					State:  "present",
				},
				{
					ID:     "data",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "file", ID: "config"}},
					},
				},
			},
			"service": {
				{
					ID:     "app",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Watch:   []StateReference{{Module: "file", ID: "config"}},
						Require: []StateReference{{Module: "file", ID: "data"}},
					},
				},
			},
		},
	}

	viz := NewGraphVisualizer()
	err := viz.BuildFromStateFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	graph := viz.GetGraph()

	// Check for different edge types
	hasRequire := false
	hasWatch := false
	for _, edge := range graph.Edges {
		if edge.EdgeType == "require" {
			hasRequire = true
		}
		if edge.EdgeType == "watch" {
			hasWatch = true
		}
	}

	if !hasRequire {
		t.Error("Graph should have require edges")
	}

	if !hasWatch {
		t.Error("Graph should have watch edges")
	}
}

func TestGraphVisualizer_CircularDependency(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "A",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "file", ID: "B"}},
					},
				},
				{
					ID:     "B",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "file", ID: "A"}},
					},
				},
			},
		},
	}

	viz := NewGraphVisualizer()
	err := viz.BuildFromStateFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	graph := viz.GetGraph()

	if !graph.HasCycle {
		t.Error("Graph should detect circular dependency")
	}

	// Text output should mention cycle
	output := viz.Render(GraphFormatText)
	if !strings.Contains(output, "Circular") && !strings.Contains(output, "WARNING") {
		t.Error("Text output should warn about circular dependency")
	}
}

func TestGraphVisualizer_EmptyGraph(t *testing.T) {
	stateFile := &StateFile{
		States: make(map[string][]StateDeclaration),
	}

	viz := NewGraphVisualizer()
	err := viz.BuildFromStateFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to build empty graph: %v", err)
	}

	graph := viz.GetGraph()

	if graph.TotalNodes != 0 {
		t.Errorf("Expected 0 nodes, got %d", graph.TotalNodes)
	}

	if graph.TotalEdges != 0 {
		t.Errorf("Expected 0 edges, got %d", graph.TotalEdges)
	}
}

func TestGraphVisualizer_ComplexGraph(t *testing.T) {
	// Create a more complex dependency graph
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"package": {
				{ID: "nginx", Module: "package", State: "installed"},
				{ID: "php", Module: "package", State: "installed"},
				{ID: "mysql", Module: "package", State: "installed"},
			},
			"file": {
				{
					ID:     "/etc/nginx/nginx.conf",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "package", ID: "nginx"}},
					},
				},
				{
					ID:     "/etc/php/php.ini",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "package", ID: "php"}},
					},
				},
			},
			"service": {
				{
					ID:     "nginx",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "nginx"},
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
						Watch: []StateReference{
							{Module: "file", ID: "/etc/nginx/nginx.conf"},
						},
					},
				},
				{
					ID:     "php-fpm",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "php"},
							{Module: "file", ID: "/etc/php/php.ini"},
						},
					},
				},
				{
					ID:     "mysql",
					Module: "service",
					State:  "running",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "package", ID: "mysql"},
						},
					},
				},
			},
		},
	}

	viz := NewGraphVisualizer()
	err := viz.BuildFromStateFile(stateFile)
	if err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	graph := viz.GetGraph()

	// Check counts
	if graph.TotalNodes != 8 {
		t.Errorf("Expected 8 nodes, got %d", graph.TotalNodes)
	}

	if graph.HasCycle {
		t.Error("Complex graph should not have cycles")
	}

	// Check that all formats render without error
	formats := []GraphFormat{GraphFormatDOT, GraphFormatMermaid, GraphFormatText, GraphFormatJSON}
	for _, format := range formats {
		output := viz.Render(format)
		if output == "" {
			t.Errorf("Format %s produced empty output", format)
		}
	}
}

func TestGraphVisualizer_DefaultFormat(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "A",
					Module: "file",
					State:  "present",
				},
			},
		},
	}

	viz := NewGraphVisualizer()
	if err := viz.BuildFromStateFile(stateFile); err != nil {
		t.Fatalf("Failed to build graph: %v", err)
	}

	output := viz.Render(GraphFormat("unknown"))
	if !strings.Contains(output, "State Dependency Graph") {
		t.Error("Expected default format to render text")
	}
}

func TestVisualizeDependencies_Error(t *testing.T) {
	stateFile := &StateFile{
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     "A",
					Module: "file",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{{Module: "file", ID: "missing"}},
					},
				},
			},
		},
	}

	_, err := VisualizeDependencies(stateFile, GraphFormatText)
	if err == nil {
		t.Fatal("Expected error when dependencies are missing")
	}
}

func TestGraphVisualizer_HelperFunctions(t *testing.T) {
	errMsg := "circular dependency detected: [file:A, file:B, file:A]"
	path := extractCyclePath(errMsg)
	if len(path) != 3 {
		t.Errorf("Expected cycle path length 3, got %d", len(path))
	}

	if sanitizeMermaidID("file:/tmp/test.txt") != "file__tmp_test_txt" {
		t.Error("Expected sanitizeMermaidID to replace invalid characters")
	}

	escaped := escapeJSON("line1\nline2\t\"quoted\"")
	if !strings.Contains(escaped, "\\n") || !strings.Contains(escaped, "\\t") || !strings.Contains(escaped, "\\\"") {
		t.Error("Expected escapeJSON to escape control characters")
	}

	if truncateID("short", 10) != "short" {
		t.Error("Expected truncateID to keep short strings")
	}

	if truncateID("0123456789", 5) != "01..." {
		t.Errorf("Expected truncateID to shorten, got %q", truncateID("0123456789", 5))
	}
}

func TestSanitizeMermaidID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dash", "with_dash"},
		{"with/slash", "with_slash"},
		{"with:colon", "with_colon"},
		{"/etc/nginx.conf", "_etc_nginx_conf"},
	}

	for _, tt := range tests {
		result := sanitizeMermaidID(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeMermaidID(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"this is a very long string", 10, "this is..."},
		{"exact10chr", 10, "exact10chr"},
	}

	for _, tt := range tests {
		result := truncateID(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateID(%s, %d) = %s, want %s", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}
