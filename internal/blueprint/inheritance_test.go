package blueprint

import (
	"reflect"
	"strings"
	"testing"
)

func TestParameterSource_String(t *testing.T) {
	tests := []struct {
		source   ParameterSource
		expected string
	}{
		{SourceSchemaDefault, "schema_default"},
		{SourceVarsDefaults, "vars_defaults"},
		{SourcePlatformOverride, "platform_override"},
		{SourceParentBlueprint, "parent_blueprint"},
		{SourceIncludedBlueprint, "included_blueprint"},
		{SourceEnvironment, "environment"},
		{SourceUserProvided, "user_provided"},
		{SourceComputed, "computed"},
		{ParameterSource(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.source.String(); got != tt.expected {
				t.Errorf("ParameterSource.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParameterSource_Priority(t *testing.T) {
	// User values should have highest priority
	if SourceUserProvided.Priority() <= SourceSchemaDefault.Priority() {
		t.Error("SourceUserProvided should have higher priority than SourceSchemaDefault")
	}

	// Schema defaults should have lowest priority
	if SourceSchemaDefault.Priority() >= SourceVarsDefaults.Priority() {
		t.Error("SourceSchemaDefault should have lower priority than SourceVarsDefaults")
	}

	// Platform override > vars defaults
	if SourcePlatformOverride.Priority() <= SourceVarsDefaults.Priority() {
		t.Error("SourcePlatformOverride should have higher priority than SourceVarsDefaults")
	}
}

func TestNewParameterLayer(t *testing.T) {
	layer := NewParameterLayer(SourceUserProvided, "test")

	if layer.Source != SourceUserProvided {
		t.Errorf("Source = %v, want %v", layer.Source, SourceUserProvided)
	}
	if layer.Name != "test" {
		t.Errorf("Name = %v, want %v", layer.Name, "test")
	}
	if layer.Values == nil {
		t.Error("Values should not be nil")
	}
	if layer.MergeStrategies == nil {
		t.Error("MergeStrategies should not be nil")
	}
	if layer.Locked == nil {
		t.Error("Locked should not be nil")
	}
}

func TestParameterLayer_Set(t *testing.T) {
	layer := NewParameterLayer(SourceUserProvided, "test")

	layer.Set("port", 8080)
	layer.Set("host", "localhost")

	if layer.Values["port"] != 8080 {
		t.Errorf("port = %v, want 8080", layer.Values["port"])
	}
	if layer.Values["host"] != "localhost" {
		t.Errorf("host = %v, want localhost", layer.Values["host"])
	}
}

func TestParameterLayer_SetWithStrategy(t *testing.T) {
	layer := NewParameterLayer(SourceUserProvided, "test")

	layer.SetWithStrategy("tags", []interface{}{"a", "b"}, MergeStrategyAppend)

	if layer.Values["tags"] == nil {
		t.Error("tags should be set")
	}
	if layer.MergeStrategies["tags"] != MergeStrategyAppend {
		t.Errorf("MergeStrategy = %v, want %v", layer.MergeStrategies["tags"], MergeStrategyAppend)
	}
}

func TestParameterLayer_Lock(t *testing.T) {
	layer := NewParameterLayer(SourceParentBlueprint, "parent")

	layer.Set("locked_param", "value")
	layer.Lock("locked_param")

	if !layer.Locked["locked_param"] {
		t.Error("locked_param should be locked")
	}
}

func TestParameterResolver_Basic(t *testing.T) {
	resolver := NewParameterResolver()

	// Add schema defaults layer
	schemaLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	schemaLayer.Set("port", 8080)
	schemaLayer.Set("host", "localhost")
	resolver.AddLayer(schemaLayer)

	// Add user values layer
	userLayer := NewParameterLayer(SourceUserProvided, "user")
	userLayer.Set("port", 9090)
	resolver.AddLayer(userLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// User value should override schema default
	if result["port"] != 9090 {
		t.Errorf("port = %v, want 9090", result["port"])
	}
	// Schema default should be used when no override
	if result["host"] != "localhost" {
		t.Errorf("host = %v, want localhost", result["host"])
	}
}

func TestParameterResolver_MergeStrategyMerge(t *testing.T) {
	resolver := NewParameterResolver()

	// Base layer with object
	baseLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	baseLayer.Set("database.host", "localhost")
	baseLayer.Set("database.port", 5432)
	resolver.AddLayer(baseLayer)

	// Override layer with partial object
	overrideLayer := NewParameterLayer(SourceUserProvided, "user")
	overrideLayer.Set("database.port", 5433)
	overrideLayer.Set("database.ssl", true)
	resolver.AddLayer(overrideLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	db, ok := result["database"].(map[string]interface{})
	if !ok {
		t.Fatalf("database should be a map, got %T", result["database"])
	}

	if db["host"] != "localhost" {
		t.Errorf("database.host = %v, want localhost", db["host"])
	}
	if db["port"] != 5433 {
		t.Errorf("database.port = %v, want 5433", db["port"])
	}
	if db["ssl"] != true {
		t.Errorf("database.ssl = %v, want true", db["ssl"])
	}
}

func TestParameterResolver_MergeStrategyAppend(t *testing.T) {
	resolver := NewParameterResolver()

	// Base layer with array
	baseLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	baseLayer.SetWithStrategy("tags", []interface{}{"a", "b"}, MergeStrategyAppend)
	resolver.AddLayer(baseLayer)

	// Override layer with additional values
	overrideLayer := NewParameterLayer(SourceUserProvided, "user")
	overrideLayer.SetWithStrategy("tags", []interface{}{"c", "d"}, MergeStrategyAppend)
	resolver.AddLayer(overrideLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	tags, ok := result["tags"].([]interface{})
	if !ok {
		t.Fatalf("tags should be an array, got %T", result["tags"])
	}

	expected := []interface{}{"a", "b", "c", "d"}
	if !reflect.DeepEqual(tags, expected) {
		t.Errorf("tags = %v, want %v", tags, expected)
	}
}

func TestParameterResolver_MergeStrategyPrepend(t *testing.T) {
	resolver := NewParameterResolver()

	// Base layer with array
	baseLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	baseLayer.SetWithStrategy("list", []interface{}{"b", "c"}, MergeStrategyPrepend)
	resolver.AddLayer(baseLayer)

	// Override layer - prepend new values
	overrideLayer := NewParameterLayer(SourceUserProvided, "user")
	overrideLayer.SetWithStrategy("list", []interface{}{"a"}, MergeStrategyPrepend)
	resolver.AddLayer(overrideLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	list, ok := result["list"].([]interface{})
	if !ok {
		t.Fatalf("list should be an array, got %T", result["list"])
	}

	expected := []interface{}{"a", "b", "c"}
	if !reflect.DeepEqual(list, expected) {
		t.Errorf("list = %v, want %v", list, expected)
	}
}

func TestParameterResolver_MergeStrategyUnion(t *testing.T) {
	resolver := NewParameterResolver()

	// Base layer with array
	baseLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	baseLayer.SetWithStrategy("features", []interface{}{"a", "b", "c"}, MergeStrategyUnion)
	resolver.AddLayer(baseLayer)

	// Override layer with overlapping values
	overrideLayer := NewParameterLayer(SourceUserProvided, "user")
	overrideLayer.SetWithStrategy("features", []interface{}{"b", "c", "d"}, MergeStrategyUnion)
	resolver.AddLayer(overrideLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	features, ok := result["features"].([]interface{})
	if !ok {
		t.Fatalf("features should be an array, got %T", result["features"])
	}

	// Should have a, b, c, d (no duplicates)
	if len(features) != 4 {
		t.Errorf("features length = %d, want 4", len(features))
	}

	// Check all values present
	hasA, hasB, hasC, hasD := false, false, false, false
	for _, f := range features {
		switch f {
		case "a":
			hasA = true
		case "b":
			hasB = true
		case "c":
			hasC = true
		case "d":
			hasD = true
		}
	}
	if !hasA || !hasB || !hasC || !hasD {
		t.Errorf("features missing values, got %v", features)
	}
}

func TestParameterResolver_LockedParameter(t *testing.T) {
	resolver := NewParameterResolver()

	// Parent locks a parameter
	parentLayer := NewParameterLayer(SourceParentBlueprint, "parent")
	parentLayer.Set("locked_value", "from_parent")
	parentLayer.Lock("locked_value")
	resolver.AddLayer(parentLayer)

	// User tries to override
	userLayer := NewParameterLayer(SourceUserProvided, "user")
	userLayer.Set("locked_value", "from_user")
	resolver.AddLayer(userLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Locked value should not be overridden
	if result["locked_value"] != "from_parent" {
		t.Errorf("locked_value = %v, want from_parent", result["locked_value"])
	}
}

func TestParameterResolver_Provenance(t *testing.T) {
	resolver := NewParameterResolver()

	schemaLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	schemaLayer.Set("inherited", "schema_default")
	resolver.AddLayer(schemaLayer)

	userLayer := NewParameterLayer(SourceUserProvided, "user")
	userLayer.Set("overridden", "user_value")
	resolver.AddLayer(userLayer)

	_, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	provenance := resolver.GetProvenance()

	// Check inherited value provenance
	if pv, ok := provenance["inherited"]; ok {
		if pv.Source != SourceSchemaDefault {
			t.Errorf("inherited source = %v, want %v", pv.Source, SourceSchemaDefault)
		}
	} else {
		t.Error("provenance missing for inherited")
	}

	// Check overridden value provenance
	if pv, ok := provenance["overridden"]; ok {
		if pv.Source != SourceUserProvided {
			t.Errorf("overridden source = %v, want %v", pv.Source, SourceUserProvided)
		}
	} else {
		t.Error("provenance missing for overridden")
	}
}

func TestParameterResolver_AddSchemaDefaults(t *testing.T) {
	resolver := NewParameterResolver()

	schemas := map[string]ParameterSchema{
		"port": {
			Type:    "integer",
			Default: 8080,
		},
		"database": {
			Type: "object",
			Properties: map[string]ParameterSchema{
				"host": {
					Type:    "string",
					Default: "localhost",
				},
			},
		},
	}

	resolver.AddSchemaDefaults(schemas)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if result["port"] != 8080 {
		t.Errorf("port = %v, want 8080", result["port"])
	}

	db, ok := result["database"].(map[string]interface{})
	if !ok {
		t.Fatalf("database should be a map, got %T", result["database"])
	}
	if db["host"] != "localhost" {
		t.Errorf("database.host = %v, want localhost", db["host"])
	}
}

func TestParameterResolver_MergeDirectives(t *testing.T) {
	resolver := NewParameterResolver()

	// Layer with merge directive in value
	layer := NewParameterLayer(SourceUserProvided, "user")
	resolver.flattenValues("", map[string]interface{}{
		"tags": map[string]interface{}{
			"$merge": "union",
			"$value": []interface{}{"a", "b"},
		},
	}, layer)
	resolver.AddLayer(layer)

	if layer.MergeStrategies["tags"] != MergeStrategyUnion {
		t.Errorf("MergeStrategy = %v, want %v", layer.MergeStrategies["tags"], MergeStrategyUnion)
	}
	if !reflect.DeepEqual(layer.Values["tags"], []interface{}{"a", "b"}) {
		t.Errorf("tags = %v, want [a b]", layer.Values["tags"])
	}
}

func TestParameterResolver_LockedDirective(t *testing.T) {
	resolver := NewParameterResolver()

	layer := NewParameterLayer(SourceParentBlueprint, "parent")
	resolver.flattenValues("", map[string]interface{}{
		"api_key": map[string]interface{}{
			"$locked": true,
			"$value":  "secret-key",
		},
	}, layer)
	resolver.AddLayer(layer)

	if !layer.Locked["api_key"] {
		t.Error("api_key should be locked")
	}
	if layer.Values["api_key"] != "secret-key" {
		t.Errorf("api_key = %v, want secret-key", layer.Values["api_key"])
	}
}

func TestInheritanceChain_ResolveParameters(t *testing.T) {
	chain := NewInheritanceChain()

	// Parent blueprint
	parent := &Blueprint{
		Metadata: Metadata{Name: "parent-bp"},
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:    "integer",
				Default: 8080,
			},
			"env": {
				Type:    "string",
				Default: "development",
			},
		},
	}

	// Child blueprint
	child := &Blueprint{
		Metadata: Metadata{Name: "child-bp"},
		Parameters: map[string]ParameterSchema{
			"port": {
				Type:    "integer",
				Default: 9090,
			},
			"debug": {
				Type:    "boolean",
				Default: false,
			},
		},
	}

	chain.AddParent(parent)
	chain.AddChild(child)

	// User provides some values
	userParams := map[string]interface{}{
		"env":   "production",
		"debug": true,
	}

	result, err := chain.ResolveParameters(userParams)
	if err != nil {
		t.Fatalf("ResolveParameters() error = %v", err)
	}

	// port: child default (9090) should override parent default (8080)
	if result["port"] != 9090 {
		t.Errorf("port = %v, want 9090", result["port"])
	}

	// env: user value should override parent default
	if result["env"] != "production" {
		t.Errorf("env = %v, want production", result["env"])
	}

	// debug: user value should override child default
	if result["debug"] != true {
		t.Errorf("debug = %v, want true", result["debug"])
	}
}

func TestValidateInheritance_CircularDetection(t *testing.T) {
	bp := &Blueprint{
		Metadata: Metadata{Name: "bp-a"},
	}

	parents := []*Blueprint{
		{Metadata: Metadata{Name: "bp-b"}},
		{Metadata: Metadata{Name: "bp-a"}}, // Circular reference
	}

	err := ValidateInheritance(bp, parents)
	if err == nil {
		t.Error("expected circular inheritance error")
	}
	if err != nil && !strings.Contains(err.Error(), "circular inheritance") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInheritance_TypeMismatch(t *testing.T) {
	child := &Blueprint{
		Metadata: Metadata{Name: "child"},
		Parameters: map[string]ParameterSchema{
			"port": {Type: "string"}, // Different type than parent
		},
	}

	parents := []*Blueprint{
		{
			Metadata: Metadata{Name: "parent"},
			Parameters: map[string]ParameterSchema{
				"port": {Type: "integer"},
			},
		},
	}

	err := ValidateInheritance(child, parents)
	if err == nil {
		t.Error("expected type mismatch error")
	}
	if err != nil && !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInheritance_RequiredToOptional(t *testing.T) {
	child := &Blueprint{
		Metadata: Metadata{Name: "child"},
		Parameters: map[string]ParameterSchema{
			"required_param": {Type: "string", Required: false},
		},
	}

	parents := []*Blueprint{
		{
			Metadata: Metadata{Name: "parent"},
			Parameters: map[string]ParameterSchema{
				"required_param": {Type: "string", Required: true},
			},
		},
	}

	err := ValidateInheritance(child, parents)
	if err == nil {
		t.Error("expected required-to-optional error")
	}
	if err != nil && !strings.Contains(err.Error(), "cannot make required parameter optional") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInheritance_Valid(t *testing.T) {
	child := &Blueprint{
		Metadata: Metadata{Name: "child"},
		Parameters: map[string]ParameterSchema{
			"port": {Type: "integer", Required: true},
		},
	}

	parents := []*Blueprint{
		{
			Metadata: Metadata{Name: "parent"},
			Parameters: map[string]ParameterSchema{
				"port": {Type: "integer", Required: false},
			},
		},
	}

	err := ValidateInheritance(child, parents)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParameterResolver_NestedMerge(t *testing.T) {
	resolver := NewParameterResolver()

	// Deep nested object in base layer
	baseLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	baseLayer.Set("config.server.http.port", 80)
	baseLayer.Set("config.server.http.host", "0.0.0.0")
	baseLayer.Set("config.server.grpc.port", 9000)
	resolver.AddLayer(baseLayer)

	// Override just one nested value
	overrideLayer := NewParameterLayer(SourceUserProvided, "user")
	overrideLayer.Set("config.server.http.port", 8080)
	resolver.AddLayer(overrideLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Navigate to check values
	config, _ := result["config"].(map[string]interface{})
	server, _ := config["server"].(map[string]interface{})
	http, _ := server["http"].(map[string]interface{})
	grpc, _ := server["grpc"].(map[string]interface{})

	if http["port"] != 8080 {
		t.Errorf("config.server.http.port = %v, want 8080", http["port"])
	}
	if http["host"] != "0.0.0.0" {
		t.Errorf("config.server.http.host = %v, want 0.0.0.0", http["host"])
	}
	if grpc["port"] != 9000 {
		t.Errorf("config.server.grpc.port = %v, want 9000", grpc["port"])
	}
}

func TestParameterResolver_MultipleLayers(t *testing.T) {
	resolver := NewParameterResolver()

	// Layer 1: Schema defaults
	schemaLayer := NewParameterLayer(SourceSchemaDefault, "schema")
	schemaLayer.Set("port", 80)
	schemaLayer.Set("host", "localhost")
	schemaLayer.Set("timeout", 30)
	resolver.AddLayer(schemaLayer)

	// Layer 2: Vars defaults
	varsLayer := NewParameterLayer(SourceVarsDefaults, "defaults.yaml")
	varsLayer.Set("timeout", 60)
	resolver.AddLayer(varsLayer)

	// Layer 3: Platform override
	platformLayer := NewParameterLayer(SourcePlatformOverride, "debian")
	platformLayer.Set("port", 8080)
	resolver.AddLayer(platformLayer)

	// Layer 4: Environment
	envLayer := NewParameterLayer(SourceEnvironment, "production")
	envLayer.Set("host", "0.0.0.0")
	resolver.AddLayer(envLayer)

	// Layer 5: User provided
	userLayer := NewParameterLayer(SourceUserProvided, "user")
	userLayer.Set("timeout", 120)
	resolver.AddLayer(userLayer)

	result, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Check that highest priority wins for each
	if result["port"] != 8080 { // platform override
		t.Errorf("port = %v, want 8080", result["port"])
	}
	if result["host"] != "0.0.0.0" { // environment
		t.Errorf("host = %v, want 0.0.0.0", result["host"])
	}
	if result["timeout"] != 120 { // user provided
		t.Errorf("timeout = %v, want 120", result["timeout"])
	}
}
