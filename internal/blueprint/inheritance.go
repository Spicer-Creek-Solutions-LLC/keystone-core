package blueprint

import (
	"fmt"
	"sort"
	"strings"
)

// MergeStrategy defines how values are combined during inheritance.
type MergeStrategy string

const (
	// MergeStrategyReplace replaces the entire value (default for scalars).
	MergeStrategyReplace MergeStrategy = "replace"

	// MergeStrategyMerge deep merges object values (default for objects).
	MergeStrategyMerge MergeStrategy = "merge"

	// MergeStrategyAppend appends array values (default for arrays).
	MergeStrategyAppend MergeStrategy = "append"

	// MergeStrategyPrepend prepends array values.
	MergeStrategyPrepend MergeStrategy = "prepend"

	// MergeStrategyUnion creates a union of unique values (for arrays).
	MergeStrategyUnion MergeStrategy = "union"
)

// ParameterSource identifies where a parameter value came from.
type ParameterSource int

const (
	// SourceSchemaDefault is the default from the parameter schema.
	SourceSchemaDefault ParameterSource = iota

	// SourceVarsDefaults is from vars/defaults.yaml.
	SourceVarsDefaults

	// SourcePlatformOverride is from platform-specific vars/platforms/*.yaml.
	SourcePlatformOverride

	// SourceParentBlueprint is from an inherited parent blueprint.
	SourceParentBlueprint

	// SourceIncludedBlueprint is from an included blueprint.
	SourceIncludedBlueprint

	// SourceEnvironment is from environment-specific configuration.
	SourceEnvironment

	// SourceUserProvided is explicitly provided by the user.
	SourceUserProvided

	// SourceComputed is computed from other values or conditions.
	SourceComputed
)

// String returns a human-readable name for the parameter source.
func (s ParameterSource) String() string {
	switch s {
	case SourceSchemaDefault:
		return "schema_default"
	case SourceVarsDefaults:
		return "vars_defaults"
	case SourcePlatformOverride:
		return "platform_override"
	case SourceParentBlueprint:
		return "parent_blueprint"
	case SourceIncludedBlueprint:
		return "included_blueprint"
	case SourceEnvironment:
		return "environment"
	case SourceUserProvided:
		return "user_provided"
	case SourceComputed:
		return "computed"
	default:
		return "unknown"
	}
}

// Priority returns the override priority (higher = takes precedence).
func (s ParameterSource) Priority() int {
	switch s {
	case SourceSchemaDefault:
		return 0
	case SourceVarsDefaults:
		return 10
	case SourcePlatformOverride:
		return 20
	case SourceParentBlueprint:
		return 30
	case SourceIncludedBlueprint:
		return 40
	case SourceEnvironment:
		return 50
	case SourceUserProvided:
		return 100
	case SourceComputed:
		return 90
	default:
		return -1
	}
}

// ParameterValue wraps a value with its source and metadata.
type ParameterValue struct {
	// Value is the actual parameter value.
	Value interface{}

	// Source identifies where this value came from.
	Source ParameterSource

	// SourceName provides additional context (e.g., parent blueprint name).
	SourceName string

	// MergeStrategy defines how this value should be merged.
	MergeStrategy MergeStrategy

	// Locked prevents this value from being overridden.
	Locked bool
}

// ParameterLayer represents a single layer of parameter values.
type ParameterLayer struct {
	// Source identifies the layer's origin.
	Source ParameterSource

	// Name provides additional context (e.g., blueprint name).
	Name string

	// Values contains the parameter values in this layer.
	Values map[string]interface{}

	// MergeStrategies defines per-parameter merge strategies.
	MergeStrategies map[string]MergeStrategy

	// Locked contains parameters that cannot be overridden.
	Locked map[string]bool
}

// NewParameterLayer creates a new parameter layer.
func NewParameterLayer(source ParameterSource, name string) *ParameterLayer {
	return &ParameterLayer{
		Source:          source,
		Name:            name,
		Values:          make(map[string]interface{}),
		MergeStrategies: make(map[string]MergeStrategy),
		Locked:          make(map[string]bool),
	}
}

// Set sets a parameter value in the layer.
func (l *ParameterLayer) Set(name string, value interface{}) {
	l.Values[name] = value
}

// SetWithStrategy sets a parameter value with a specific merge strategy.
func (l *ParameterLayer) SetWithStrategy(name string, value interface{}, strategy MergeStrategy) {
	l.Values[name] = value
	l.MergeStrategies[name] = strategy
}

// Lock prevents a parameter from being overridden by higher layers.
func (l *ParameterLayer) Lock(name string) {
	l.Locked[name] = true
}

// ParameterResolver resolves parameters through multiple inheritance layers.
type ParameterResolver struct {
	// layers contains parameter layers in order of increasing priority.
	layers []*ParameterLayer

	// schemas contains parameter schemas for type information.
	schemas map[string]ParameterSchema

	// provenance tracks where each final value came from.
	provenance map[string]ParameterValue

	// defaultMergeStrategies defines default strategies by parameter type.
	defaultMergeStrategies map[string]MergeStrategy
}

// NewParameterResolver creates a new parameter resolver.
func NewParameterResolver() *ParameterResolver {
	return &ParameterResolver{
		layers:     make([]*ParameterLayer, 0),
		schemas:    make(map[string]ParameterSchema),
		provenance: make(map[string]ParameterValue),
		defaultMergeStrategies: map[string]MergeStrategy{
			"string":  MergeStrategyReplace,
			"integer": MergeStrategyReplace,
			"number":  MergeStrategyReplace,
			"boolean": MergeStrategyReplace,
			"object":  MergeStrategyMerge,
			"array":   MergeStrategyAppend,
		},
	}
}

// SetSchemas sets the parameter schemas for type information.
func (r *ParameterResolver) SetSchemas(schemas map[string]ParameterSchema) {
	r.schemas = schemas
}

// AddLayer adds a parameter layer.
func (r *ParameterResolver) AddLayer(layer *ParameterLayer) {
	r.layers = append(r.layers, layer)
}

// AddSchemaDefaults adds schema defaults as the lowest priority layer.
func (r *ParameterResolver) AddSchemaDefaults(schemas map[string]ParameterSchema) {
	layer := NewParameterLayer(SourceSchemaDefault, "schema")
	r.extractSchemaDefaultsRecursive("", schemas, layer)
	r.AddLayer(layer)
}

func (r *ParameterResolver) extractSchemaDefaultsRecursive(prefix string, schemas map[string]ParameterSchema, layer *ParameterLayer) {
	for name, schema := range schemas {
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}

		if schema.Default != nil {
			layer.Set(fullName, schema.Default)
		}

		// Recurse into nested properties
		if schema.Type == "object" && schema.Properties != nil {
			r.extractSchemaDefaultsRecursive(fullName, schema.Properties, layer)
		}
	}
}

// AddVarsDefaults adds vars/defaults.yaml values.
func (r *ParameterResolver) AddVarsDefaults(values map[string]interface{}) {
	layer := NewParameterLayer(SourceVarsDefaults, "defaults.yaml")
	r.flattenValues("", values, layer)
	r.AddLayer(layer)
}

// AddPlatformOverride adds platform-specific values.
func (r *ParameterResolver) AddPlatformOverride(platformName string, values map[string]interface{}) {
	layer := NewParameterLayer(SourcePlatformOverride, platformName)
	r.flattenValues("", values, layer)
	r.AddLayer(layer)
}

// AddParentBlueprint adds values from a parent blueprint.
func (r *ParameterResolver) AddParentBlueprint(blueprintName string, values map[string]interface{}) {
	layer := NewParameterLayer(SourceParentBlueprint, blueprintName)
	r.flattenValues("", values, layer)
	r.AddLayer(layer)
}

// AddIncludedBlueprint adds values from an included blueprint.
func (r *ParameterResolver) AddIncludedBlueprint(blueprintName string, values map[string]interface{}) {
	layer := NewParameterLayer(SourceIncludedBlueprint, blueprintName)
	r.flattenValues("", values, layer)
	r.AddLayer(layer)
}

// AddEnvironment adds environment-specific values.
func (r *ParameterResolver) AddEnvironment(envName string, values map[string]interface{}) {
	layer := NewParameterLayer(SourceEnvironment, envName)
	r.flattenValues("", values, layer)
	r.AddLayer(layer)
}

// AddUserValues adds user-provided values (highest priority).
func (r *ParameterResolver) AddUserValues(values map[string]interface{}) {
	layer := NewParameterLayer(SourceUserProvided, "user")
	r.flattenValues("", values, layer)
	r.AddLayer(layer)
}

func (r *ParameterResolver) flattenValues(prefix string, values map[string]interface{}, layer *ParameterLayer) {
	for k, v := range values {
		fullKey := k
		if prefix != "" {
			fullKey = prefix + "." + k
		}

		// Check for special merge directives
		if m, ok := v.(map[string]interface{}); ok {
			if strategy, hasStrategy := m["$merge"]; hasStrategy {
				if strategyStr, ok := strategy.(string); ok {
					layer.MergeStrategies[fullKey] = MergeStrategy(strategyStr)
					delete(m, "$merge")
				}
			}
			if locked, hasLocked := m["$locked"]; hasLocked {
				if lockedBool, ok := locked.(bool); ok && lockedBool {
					layer.Locked[fullKey] = true
					delete(m, "$locked")
				}
			}
			if actualValue, hasValue := m["$value"]; hasValue {
				// Explicit value wrapper
				layer.Values[fullKey] = actualValue
				continue
			}
			// Not a special directive, recurse
			r.flattenValues(fullKey, m, layer)
			continue
		}

		layer.Values[fullKey] = v
	}
}

// Resolve resolves all parameter values through the inheritance chain.
func (r *ParameterResolver) Resolve() (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Sort layers by priority
	sortedLayers := make([]*ParameterLayer, len(r.layers))
	copy(sortedLayers, r.layers)
	sort.Slice(sortedLayers, func(i, j int) bool {
		return sortedLayers[i].Source.Priority() < sortedLayers[j].Source.Priority()
	})

	// Track which parameters are locked
	locked := make(map[string]bool)

	// Process layers in priority order
	for _, layer := range sortedLayers {
		for key, value := range layer.Values {
			// Check if locked by a lower priority layer
			if locked[key] {
				continue
			}

			// Get merge strategy
			strategy := r.getMergeStrategy(key, layer)

			// Get existing value
			existing, hasExisting := r.getNestedValue(result, key)

			// Merge or replace based on strategy
			var mergedValue interface{}
			if hasExisting {
				mergedValue = r.mergeValues(existing, value, strategy)
			} else {
				mergedValue = value
			}

			// Set the value
			r.setNestedValue(result, key, mergedValue)

			// Update provenance
			r.provenance[key] = ParameterValue{
				Value:         mergedValue,
				Source:        layer.Source,
				SourceName:    layer.Name,
				MergeStrategy: strategy,
				Locked:        layer.Locked[key],
			}

			// Mark as locked if this layer locks it
			if layer.Locked[key] {
				locked[key] = true
			}
		}
	}

	return result, nil
}

func (r *ParameterResolver) getMergeStrategy(key string, layer *ParameterLayer) MergeStrategy {
	// Check layer-specific strategy
	if strategy, ok := layer.MergeStrategies[key]; ok {
		return strategy
	}

	// Check schema for type-based default
	if schema, ok := r.schemas[key]; ok {
		if defaultStrategy, ok := r.defaultMergeStrategies[schema.Type]; ok {
			return defaultStrategy
		}
	}

	// Infer from value type
	return MergeStrategyReplace
}

func (r *ParameterResolver) mergeValues(existing, incoming interface{}, strategy MergeStrategy) interface{} {
	switch strategy {
	case MergeStrategyReplace:
		return incoming

	case MergeStrategyMerge:
		existingMap, existingOk := existing.(map[string]interface{})
		incomingMap, incomingOk := incoming.(map[string]interface{})
		if existingOk && incomingOk {
			result := make(map[string]interface{})
			for k, v := range existingMap {
				result[k] = v
			}
			for k, v := range incomingMap {
				if existingV, ok := result[k]; ok {
					// Recursively merge nested objects
					result[k] = r.mergeValues(existingV, v, MergeStrategyMerge)
				} else {
					result[k] = v
				}
			}
			return result
		}
		return incoming

	case MergeStrategyAppend:
		existingArr, existingOk := existing.([]interface{})
		incomingArr, incomingOk := incoming.([]interface{})
		if existingOk && incomingOk {
			result := make([]interface{}, len(existingArr)+len(incomingArr))
			copy(result, existingArr)
			copy(result[len(existingArr):], incomingArr)
			return result
		}
		return incoming

	case MergeStrategyPrepend:
		existingArr, existingOk := existing.([]interface{})
		incomingArr, incomingOk := incoming.([]interface{})
		if existingOk && incomingOk {
			result := make([]interface{}, len(incomingArr)+len(existingArr))
			copy(result, incomingArr)
			copy(result[len(incomingArr):], existingArr)
			return result
		}
		return incoming

	case MergeStrategyUnion:
		existingArr, existingOk := existing.([]interface{})
		incomingArr, incomingOk := incoming.([]interface{})
		if existingOk && incomingOk {
			seen := make(map[interface{}]bool)
			var result []interface{}
			for _, v := range existingArr {
				if !seen[v] {
					seen[v] = true
					result = append(result, v)
				}
			}
			for _, v := range incomingArr {
				if !seen[v] {
					seen[v] = true
					result = append(result, v)
				}
			}
			return result
		}
		return incoming

	default:
		return incoming
	}
}

func (r *ParameterResolver) getNestedValue(m map[string]interface{}, key string) (interface{}, bool) {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			v, ok := current[part]
			return v, ok
		}

		next, ok := current[part].(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

func (r *ParameterResolver) setNestedValue(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}

		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[part] = next
		}
		current = next
	}
}

// GetProvenance returns the provenance information for all resolved parameters.
func (r *ParameterResolver) GetProvenance() map[string]ParameterValue {
	return r.provenance
}

// GetProvenanceFor returns the provenance for a specific parameter.
func (r *ParameterResolver) GetProvenanceFor(name string) (ParameterValue, bool) {
	pv, ok := r.provenance[name]
	return pv, ok
}

// InheritanceChain represents a chain of blueprint inheritance.
type InheritanceChain struct {
	// blueprints in inheritance order (parent first).
	blueprints []*Blueprint

	// resolver for merging parameters.
	resolver *ParameterResolver
}

// NewInheritanceChain creates a new inheritance chain.
func NewInheritanceChain() *InheritanceChain {
	return &InheritanceChain{
		blueprints: make([]*Blueprint, 0),
		resolver:   NewParameterResolver(),
	}
}

// AddParent adds a parent blueprint to the chain.
func (c *InheritanceChain) AddParent(bp *Blueprint) {
	c.blueprints = append([]*Blueprint{bp}, c.blueprints...)
}

// AddChild adds a child blueprint (the current one being instantiated).
func (c *InheritanceChain) AddChild(bp *Blueprint) {
	c.blueprints = append(c.blueprints, bp)
}

// ResolveParameters resolves parameters through the inheritance chain.
func (c *InheritanceChain) ResolveParameters(userParams map[string]interface{}) (map[string]interface{}, error) {
	// Collect all schemas and merge defaults from parent → child order
	// Child defaults override parent defaults for the same parameter
	allSchemas := make(map[string]ParameterSchema)
	mergedDefaults := make(map[string]interface{})

	// Process blueprints in parent → child order
	// c.blueprints is ordered [parent, ..., child], so iterating forward
	// means child values will override parent values in the merged map
	for _, bp := range c.blueprints {
		for name, schema := range bp.Parameters {
			allSchemas[name] = schema
			// Extract defaults, child overwrites parent
			if schema.Default != nil {
				mergedDefaults[name] = schema.Default
			}
		}
	}
	c.resolver.SetSchemas(allSchemas)

	// Add the merged schema defaults as a single layer
	layer := NewParameterLayer(SourceSchemaDefault, "schema")
	for key, value := range mergedDefaults {
		layer.Set(key, value)
	}
	c.resolver.AddLayer(layer)

	// Add user values (highest priority)
	if userParams != nil {
		c.resolver.AddUserValues(userParams)
	}

	return c.resolver.Resolve()
}

// GetProvenance returns provenance information for resolved parameters.
func (c *InheritanceChain) GetProvenance() map[string]ParameterValue {
	return c.resolver.GetProvenance()
}

// extractDefaultsFromBlueprint extracts non-nil defaults from a blueprint's parameters.
func extractDefaultsFromBlueprint(bp *Blueprint) map[string]interface{} {
	result := make(map[string]interface{})
	extractDefaultsRecursive("", bp.Parameters, result)
	return result
}

func extractDefaultsRecursive(prefix string, schemas map[string]ParameterSchema, result map[string]interface{}) {
	for name, schema := range schemas {
		fullName := name
		if prefix != "" {
			fullName = prefix + "." + name
		}

		if schema.Default != nil {
			setNestedValueForDefaults(result, fullName, schema.Default)
		}

		if schema.Type == "object" && schema.Properties != nil {
			extractDefaultsRecursive(fullName, schema.Properties, result)
		}
	}
}

func setNestedValueForDefaults(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	current := m

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}

		next, ok := current[part].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			current[part] = next
		}
		current = next
	}
}

// ValidateInheritance checks that a blueprint's inheritance is valid.
func ValidateInheritance(bp *Blueprint, parents []*Blueprint) error {
	// Check for circular inheritance
	seen := make(map[string]bool)
	seen[bp.Metadata.Name] = true

	for _, parent := range parents {
		if seen[parent.Metadata.Name] {
			return fmt.Errorf("circular inheritance detected: %s", parent.Metadata.Name)
		}
		seen[parent.Metadata.Name] = true
	}

	// Validate parameter compatibility
	for _, parent := range parents {
		for name, childSchema := range bp.Parameters {
			if parentSchema, ok := parent.Parameters[name]; ok {
				// Parameter exists in both - check compatibility
				if err := validateSchemaCompatibility(name, parentSchema, childSchema); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func validateSchemaCompatibility(name string, parent, child ParameterSchema) error {
	// Types must match if both define the parameter
	if parent.Type != "" && child.Type != "" && parent.Type != child.Type {
		return fmt.Errorf("parameter %s: type mismatch (parent: %s, child: %s)",
			name, parent.Type, child.Type)
	}

	// Child cannot make a required parameter optional
	if parent.Required && !child.Required {
		return fmt.Errorf("parameter %s: cannot make required parameter optional in child",
			name)
	}

	return nil
}
