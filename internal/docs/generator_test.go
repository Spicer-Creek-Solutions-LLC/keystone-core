package docs

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNewGenerator(t *testing.T) {
	g := NewGenerator(nil)
	if g == nil {
		t.Fatal("Expected non-nil generator")
	}
	if g.config.Format != FormatMarkdown {
		t.Errorf("Default format = %s, want markdown", g.config.Format)
	}
}

func TestNewGenerator_WithConfig(t *testing.T) {
	config := &GeneratorConfig{
		Title:          "Test Docs",
		Format:         FormatHTML,
		IncludePrivate: true,
		IncludeSource:  true,
		Metadata: map[string]string{
			"version": "1.0.0",
		},
	}

	g := NewGenerator(config)
	if g.config.Format != FormatHTML {
		t.Errorf("Format = %s, want html", g.config.Format)
	}
	if !g.config.IncludePrivate {
		t.Error("IncludePrivate should be true")
	}
}

func TestModuleInfo(t *testing.T) {
	info := &ModuleInfo{
		Name:        "testpkg",
		Package:     "testpkg",
		ImportPath:  "github.com/example/testpkg",
		Description: "A test package",
		Types: []TypeInfo{
			{
				Name:        "TestType",
				Kind:        "struct",
				Description: "A test type",
				Fields: []FieldInfo{
					{Name: "ID", Type: "int", Description: "The ID"},
					{Name: "Name", Type: "string", Description: "The name"},
				},
			},
		},
		Functions: []FunctionInfo{
			{
				Name:        "NewTestType",
				Description: "Creates a new TestType",
				Parameters: []ParamInfo{
					{Name: "id", Type: "int"},
					{Name: "name", Type: "string"},
				},
				Returns: []ParamInfo{
					{Type: "*TestType"},
				},
			},
		},
	}

	if info.Name != "testpkg" {
		t.Errorf("Name = %s, want testpkg", info.Name)
	}
	if len(info.Types) != 1 {
		t.Errorf("Types count = %d, want 1", len(info.Types))
	}
	if len(info.Functions) != 1 {
		t.Errorf("Functions count = %d, want 1", len(info.Functions))
	}
}

func TestTypeInfo(t *testing.T) {
	ti := TypeInfo{
		Name:        "Server",
		Kind:        "struct",
		Description: "Server represents an HTTP server",
		Fields: []FieldInfo{
			{Name: "Host", Type: "string", Required: true},
			{Name: "Port", Type: "int", Required: true},
			{Name: "TLS", Type: "bool", Required: false},
		},
		Methods: []MethodInfo{
			{
				Name:        "Start",
				Receiver:    "Server",
				Description: "Start starts the server",
			},
			{
				Name:        "Stop",
				Receiver:    "Server",
				Description: "Stop stops the server",
			},
		},
	}

	if ti.Kind != "struct" {
		t.Errorf("Kind = %s, want struct", ti.Kind)
	}
	if len(ti.Fields) != 3 {
		t.Errorf("Fields count = %d, want 3", len(ti.Fields))
	}
	if len(ti.Methods) != 2 {
		t.Errorf("Methods count = %d, want 2", len(ti.Methods))
	}
}

func TestFieldInfo(t *testing.T) {
	tests := []struct {
		name     string
		field    FieldInfo
		wantReq  bool
		wantDep  bool
	}{
		{
			name: "required field",
			field: FieldInfo{
				Name:     "ID",
				Type:     "string",
				Tag:      `json:"id"`,
				Required: true,
			},
			wantReq: true,
		},
		{
			name: "optional field",
			field: FieldInfo{
				Name:     "Name",
				Type:     "string",
				Tag:      `json:"name,omitempty"`,
				Required: false,
			},
			wantReq: false,
		},
		{
			name: "deprecated field",
			field: FieldInfo{
				Name:        "OldField",
				Type:        "string",
				Description: "Deprecated: use NewField instead",
				Deprecated:  true,
			},
			wantDep: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field.Required != tt.wantReq {
				t.Errorf("Required = %v, want %v", tt.field.Required, tt.wantReq)
			}
			if tt.field.Deprecated != tt.wantDep {
				t.Errorf("Deprecated = %v, want %v", tt.field.Deprecated, tt.wantDep)
			}
		})
	}
}

func TestGenerator_GenerateMarkdown(t *testing.T) {
	g := NewGenerator(&GeneratorConfig{Format: FormatMarkdown})

	info := &ModuleInfo{
		Name:        "example",
		ImportPath:  "github.com/example/pkg",
		Description: "An example package",
		Types: []TypeInfo{
			{
				Name:        "Config",
				Kind:        "struct",
				Description: "Config holds configuration",
				Fields: []FieldInfo{
					{Name: "Host", Type: "string", Description: "The host address"},
				},
			},
		},
		Functions: []FunctionInfo{
			{
				Name:        "New",
				Description: "New creates a new instance",
			},
		},
	}

	var buf bytes.Buffer
	err := g.Generate(info, &buf)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := buf.String()

	// Check content
	if !strings.Contains(output, "# example") {
		t.Error("Missing package title")
	}
	if !strings.Contains(output, "github.com/example/pkg") {
		t.Error("Missing import path")
	}
	if !strings.Contains(output, "## Types") {
		t.Error("Missing types section")
	}
	if !strings.Contains(output, "### Config") {
		t.Error("Missing type name")
	}
	if !strings.Contains(output, "## Functions") {
		t.Error("Missing functions section")
	}
}

func TestGenerator_GenerateHTML(t *testing.T) {
	g := NewGenerator(&GeneratorConfig{Format: FormatHTML})

	info := &ModuleInfo{
		Name:        "htmlpkg",
		Description: "HTML test package",
		Types: []TypeInfo{
			{
				Name: "Widget",
				Kind: "struct",
				Fields: []FieldInfo{
					{Name: "ID", Type: "int", Deprecated: true},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := g.Generate(info, &buf)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "<!DOCTYPE html>") {
		t.Error("Missing HTML doctype")
	}
	if !strings.Contains(output, "<title>htmlpkg") {
		t.Error("Missing title")
	}
	if !strings.Contains(output, "<h3>Widget</h3>") {
		t.Error("Missing type heading")
	}
}

func TestGenerator_GenerateJSON(t *testing.T) {
	g := NewGenerator(&GeneratorConfig{Format: FormatJSON})

	info := &ModuleInfo{
		Name:        "jsonpkg",
		Package:     "jsonpkg",
		ImportPath:  "example.com/jsonpkg",
		Description: "A JSON test package",
		Types:       []TypeInfo{{Name: "T1"}, {Name: "T2"}},
		Functions:   []FunctionInfo{{Name: "F1"}},
	}

	var buf bytes.Buffer
	err := g.Generate(info, &buf)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, `"name": "jsonpkg"`) {
		t.Error("Missing name in JSON")
	}
	if !strings.Contains(output, `"typesCount": 2`) {
		t.Error("Missing types count")
	}
	if !strings.Contains(output, `"functionsCount": 1`) {
		t.Error("Missing functions count")
	}
}

func TestGenerator_GenerateString(t *testing.T) {
	g := NewGenerator(&GeneratorConfig{Format: FormatMarkdown})

	info := &ModuleInfo{
		Name:        "strpkg",
		Description: "String test",
	}

	output, err := g.GenerateString(info)
	if err != nil {
		t.Fatalf("GenerateString failed: %v", err)
	}

	if !strings.Contains(output, "# strpkg") {
		t.Error("Missing package name in output")
	}
}

func TestGenerator_UnsupportedFormat(t *testing.T) {
	g := NewGenerator(&GeneratorConfig{Format: "yaml"})

	info := &ModuleInfo{Name: "test"}
	var buf bytes.Buffer

	err := g.Generate(info, &buf)
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
}

func TestIndexGenerator(t *testing.T) {
	config := &GeneratorConfig{Title: "Test Index"}
	ig := NewIndexGenerator(config)

	modules := []*ModuleInfo{
		{
			Name:        "auth",
			Package:     "auth",
			ImportPath:  "pkg/auth",
			Description: "Authentication package provides user authentication and authorization. It supports multiple auth providers.",
			Types:       []TypeInfo{{Name: "User"}, {Name: "Token"}},
			Functions:   []FunctionInfo{{Name: "Login"}},
		},
		{
			Name:        "api",
			Package:     "api",
			ImportPath:  "pkg/api",
			Description: "API handlers",
			Functions:   []FunctionInfo{{Name: "Handle1"}, {Name: "Handle2"}},
		},
	}

	index := ig.GenerateIndex(modules)

	if index.Title != "Test Index" {
		t.Errorf("Title = %s, want Test Index", index.Title)
	}
	if len(index.Modules) != 2 {
		t.Errorf("Modules = %d, want 2", len(index.Modules))
	}

	// Check sorting (should be alphabetical)
	if index.Modules[0].Name != "api" {
		t.Errorf("First module = %s, want api (alphabetical)", index.Modules[0].Name)
	}

	// Check api entry
	apiEntry := index.Modules[0]
	if apiEntry.Functions != 2 {
		t.Errorf("API functions = %d, want 2", apiEntry.Functions)
	}

	// Check auth entry
	authEntry := index.Modules[1]
	if authEntry.Types != 2 {
		t.Errorf("Auth types = %d, want 2", authEntry.Types)
	}
}

func TestIndexGenerator_GenerateMarkdown(t *testing.T) {
	config := &GeneratorConfig{Title: "Module Index"}
	ig := NewIndexGenerator(config)

	index := &DocIndex{
		Title:       "Test Modules",
		Description: "Documentation index",
		Modules: []DocEntry{
			{Name: "pkg1", Path: "pkg/pkg1", Types: 3, Functions: 2},
			{Name: "pkg2", Path: "pkg/pkg2", Types: 1, Functions: 5},
		},
		Generated: time.Now(),
	}

	var buf bytes.Buffer
	err := ig.GenerateIndexMarkdown(index, &buf)
	if err != nil {
		t.Fatalf("GenerateIndexMarkdown failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "# Test Modules") {
		t.Error("Missing title")
	}
	if !strings.Contains(output, "| Module |") {
		t.Error("Missing table header")
	}
	if !strings.Contains(output, "[pkg1]") {
		t.Error("Missing pkg1 link")
	}
}

func TestTruncateDescription(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"Short text", 100, "Short text"},
		{"First sentence. Second sentence.", 100, "First sentence."},
		{"First paragraph.\n\nSecond paragraph.", 100, "First paragraph."},
		{strings.Repeat("x", 300), 50, strings.Repeat("x", 47) + "..."},
	}

	for _, tt := range tests {
		result := truncateDescription(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateDescription(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestExtractKeywords(t *testing.T) {
	mod := &ModuleInfo{
		Name:        "server",
		Description: "HTTP server implementation with TLS support",
		Types: []TypeInfo{
			{Name: "Server"},
			{Name: "Config"},
		},
		Functions: []FunctionInfo{
			{Name: "NewServer"},
		},
	}

	keywords := extractKeywords(mod)

	if len(keywords) == 0 {
		t.Error("Expected some keywords")
	}

	// Should include type/function names
	found := make(map[string]bool)
	for _, k := range keywords {
		found[k] = true
	}

	if !found["server"] {
		t.Error("Expected 'server' keyword")
	}
	if !found["config"] {
		t.Error("Expected 'config' keyword")
	}
}

func TestMethodInfo(t *testing.T) {
	method := MethodInfo{
		Name:        "Start",
		Receiver:    "Server",
		Description: "Start starts the server on the configured port",
		Parameters: []ParamInfo{
			{Name: "ctx", Type: "context.Context"},
		},
		Returns: []ParamInfo{
			{Type: "error"},
		},
	}

	if method.Name != "Start" {
		t.Errorf("Name = %s, want Start", method.Name)
	}
	if method.Receiver != "Server" {
		t.Errorf("Receiver = %s, want Server", method.Receiver)
	}
	if len(method.Parameters) != 1 {
		t.Errorf("Parameters = %d, want 1", len(method.Parameters))
	}
}

func TestFunctionInfo(t *testing.T) {
	fn := FunctionInfo{
		Name:        "ParseConfig",
		Description: "ParseConfig parses configuration from a file",
		Parameters: []ParamInfo{
			{Name: "path", Type: "string"},
		},
		Returns: []ParamInfo{
			{Type: "*Config"},
			{Type: "error"},
		},
		Deprecated: false,
	}

	if fn.Deprecated {
		t.Error("Function should not be deprecated")
	}
	if len(fn.Returns) != 2 {
		t.Errorf("Returns = %d, want 2", len(fn.Returns))
	}
}

func TestConstantInfo(t *testing.T) {
	c := ConstantInfo{
		Name:        "MaxConnections",
		Type:        "int",
		Value:       "100",
		Description: "Maximum number of connections",
	}

	if c.Name != "MaxConnections" {
		t.Errorf("Name = %s, want MaxConnections", c.Name)
	}
	if c.Value != "100" {
		t.Errorf("Value = %s, want 100", c.Value)
	}
}

func TestVariableInfo(t *testing.T) {
	v := VariableInfo{
		Name:        "DefaultTimeout",
		Type:        "time.Duration",
		Description: "Default timeout for operations",
	}

	if v.Type != "time.Duration" {
		t.Errorf("Type = %s, want time.Duration", v.Type)
	}
}

func TestExampleInfo(t *testing.T) {
	ex := ExampleInfo{
		Name:        "ExampleNewServer",
		Description: "Shows how to create a new server",
		Code:        "server := NewServer(\"localhost\", 8080)",
		Output:      "Server created",
	}

	if ex.Name != "ExampleNewServer" {
		t.Errorf("Name = %s, want ExampleNewServer", ex.Name)
	}
	if ex.Output != "Server created" {
		t.Errorf("Output = %s, want 'Server created'", ex.Output)
	}
}

func TestParamInfo(t *testing.T) {
	params := []ParamInfo{
		{
			Name: "port",
			Type: "int",
		},
		{
			Name: "host",
			Type: "string",
		},
	}

	if params[0].Name != "port" {
		t.Errorf("Name = %s, want port", params[0].Name)
	}
	if params[1].Type != "string" {
		t.Errorf("Type = %s, want string", params[1].Type)
	}
}

func TestOutputFormat(t *testing.T) {
	formats := []OutputFormat{FormatMarkdown, FormatHTML, FormatJSON}
	expected := []string{"markdown", "html", "json"}

	for i, f := range formats {
		if string(f) != expected[i] {
			t.Errorf("Format = %s, want %s", f, expected[i])
		}
	}
}

func TestModuleDoc(t *testing.T) {
	doc := ModuleDoc{
		Info: &ModuleInfo{
			Name: "testmod",
		},
		Content:   "# testmod\n\nDocumentation content",
		Format:    FormatMarkdown,
		Generated: time.Now(),
	}

	if doc.Format != FormatMarkdown {
		t.Errorf("Format = %s, want markdown", doc.Format)
	}
	if doc.Info.Name != "testmod" {
		t.Errorf("Info.Name = %s, want testmod", doc.Info.Name)
	}
}

func TestDocEntry(t *testing.T) {
	entry := DocEntry{
		Name:        "auth",
		Package:     "auth",
		Path:        "pkg/auth",
		Description: "Authentication module",
		Types:       5,
		Functions:   10,
		Keywords:    []string{"auth", "login", "token"},
	}

	if entry.Types != 5 {
		t.Errorf("Types = %d, want 5", entry.Types)
	}
	if len(entry.Keywords) != 3 {
		t.Errorf("Keywords = %d, want 3", len(entry.Keywords))
	}
}

func TestAPIDoc(t *testing.T) {
	api := APIDoc{
		Endpoints: []EndpointDoc{
			{
				Method:      "GET",
				Path:        "/api/users",
				Description: "List all users",
				Parameters: []APIParam{
					{Name: "limit", In: "query", Type: "int", Required: false},
				},
				Responses: []APIResponse{
					{StatusCode: 200, Description: "Success", ContentType: "application/json"},
				},
				Tags: []string{"users"},
			},
			{
				Method:      "POST",
				Path:        "/api/users",
				Description: "Create a user",
				RequestBody: &APIBody{
					ContentType: "application/json",
					Schema:      "UserCreate",
				},
				Responses: []APIResponse{
					{StatusCode: 201, Description: "Created"},
					{StatusCode: 400, Description: "Bad Request"},
				},
			},
		},
	}

	if len(api.Endpoints) != 2 {
		t.Errorf("Endpoints = %d, want 2", len(api.Endpoints))
	}

	getEndpoint := api.Endpoints[0]
	if getEndpoint.Method != "GET" {
		t.Errorf("Method = %s, want GET", getEndpoint.Method)
	}
	if len(getEndpoint.Parameters) != 1 {
		t.Errorf("Parameters = %d, want 1", len(getEndpoint.Parameters))
	}

	postEndpoint := api.Endpoints[1]
	if postEndpoint.RequestBody == nil {
		t.Error("Expected request body")
	}
	if len(postEndpoint.Responses) != 2 {
		t.Errorf("Responses = %d, want 2", len(postEndpoint.Responses))
	}
}

func TestNewBatchGenerator(t *testing.T) {
	config := &GeneratorConfig{
		Format: FormatMarkdown,
		Title:  "Batch Docs",
	}

	bg := NewBatchGenerator(config)
	if bg == nil {
		t.Fatal("Expected non-nil batch generator")
	}
	if bg.generator.config.Format != FormatMarkdown {
		t.Errorf("Format = %s, want markdown", bg.generator.config.Format)
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello\"world", "hello\\\"world"},
		{"hello\nworld", "hello\\nworld"},
		{"hello\tworld", "hello\\tworld"},
		{"hello\\world", "hello\\\\world"},
	}

	for _, tt := range tests {
		result := escapeJSON(tt.input)
		if result != tt.expected {
			t.Errorf("escapeJSON(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLabelSelectorRequirement_Interface(t *testing.T) {
	info := TypeInfo{
		Name: "Handler",
		Kind: "interface",
		Methods: []MethodInfo{
			{Name: "Handle", Description: "Handle handles a request"},
			{Name: "Close", Description: "Close closes the handler"},
		},
	}

	if info.Kind != "interface" {
		t.Errorf("Kind = %s, want interface", info.Kind)
	}
	if len(info.Methods) != 2 {
		t.Errorf("Methods = %d, want 2", len(info.Methods))
	}
}
