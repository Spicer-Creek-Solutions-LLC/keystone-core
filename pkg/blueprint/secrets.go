package blueprint

import (
	"fmt"
	"regexp"
	"strings"
)

// SecretReference represents a reference to a secret value.
// Secrets are referenced using the !secret YAML tag:
//
//	database:
//	  password: !secret databases/prod/postgres
//	  api_key: !secret path/to/api-key
type SecretReference struct {
	// Path is the secret path (e.g., "databases/prod/postgres")
	Path string

	// Backend specifies which secrets backend to use (optional)
	// If empty, uses the default configured backend
	Backend string

	// Version specifies a specific version of the secret (optional)
	// If empty, uses the latest version
	Version string
}

// String returns the original secret reference format
func (s *SecretReference) String() string {
	result := s.Path
	if s.Backend != "" {
		result = s.Backend + ":" + result
	}
	if s.Version != "" {
		result = result + "@" + s.Version
	}
	return result
}

// SecretResolver resolves secret references to their actual values.
type SecretResolver interface {
	// Resolve retrieves a secret value from the specified path
	Resolve(path string) (string, error)

	// ResolveWithVersion retrieves a specific version of a secret
	ResolveWithVersion(path, version string) (string, error)

	// Exists checks if a secret exists at the given path
	Exists(path string) (bool, error)
}

// MultiBackendResolver routes secret resolution to different backends based on prefix.
type MultiBackendResolver struct {
	// backends maps backend names to their resolvers
	backends map[string]SecretResolver

	// defaultBackend is used when no backend is specified
	defaultBackend string
}

// NewMultiBackendResolver creates a new multi-backend resolver.
func NewMultiBackendResolver() *MultiBackendResolver {
	return &MultiBackendResolver{
		backends: make(map[string]SecretResolver),
	}
}

// RegisterBackend registers a secrets backend.
func (m *MultiBackendResolver) RegisterBackend(name string, resolver SecretResolver) {
	m.backends[name] = resolver
}

// SetDefaultBackend sets the default backend to use when none is specified.
func (m *MultiBackendResolver) SetDefaultBackend(name string) {
	m.defaultBackend = name
}

// Resolve resolves a secret reference using the appropriate backend.
func (m *MultiBackendResolver) Resolve(ref *SecretReference) (string, error) {
	backendName := ref.Backend
	if backendName == "" {
		backendName = m.defaultBackend
	}

	backend, ok := m.backends[backendName]
	if !ok {
		if backendName == "" {
			return "", fmt.Errorf("no default secrets backend configured")
		}
		return "", fmt.Errorf("unknown secrets backend: %s", backendName)
	}

	if ref.Version != "" {
		return backend.ResolveWithVersion(ref.Path, ref.Version)
	}
	return backend.Resolve(ref.Path)
}

// secretReferencePattern matches !secret tags in various formats:
// - !secret path/to/secret
// - !secret backend:path/to/secret
// - !secret path/to/secret@version
// - !secret backend:path/to/secret@version
var secretReferencePattern = regexp.MustCompile(`^(?:([a-zA-Z0-9_-]+):)?([a-zA-Z0-9_/.-]+)(?:@([a-zA-Z0-9_.-]+))?$`)

// ParseSecretReference parses a secret reference string.
// Supported formats:
//   - path/to/secret
//   - backend:path/to/secret
//   - path/to/secret@version
//   - backend:path/to/secret@version
func ParseSecretReference(ref string) (*SecretReference, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("empty secret reference")
	}

	matches := secretReferencePattern.FindStringSubmatch(ref)
	if matches == nil {
		return nil, fmt.Errorf("invalid secret reference format: %s", ref)
	}

	return &SecretReference{
		Backend: matches[1],
		Path:    matches[2],
		Version: matches[3],
	}, nil
}

// IsSecretReference checks if a value appears to be a secret reference.
// This is used to detect !secret tags in YAML.
func IsSecretReference(value interface{}) bool {
	if str, ok := value.(string); ok {
		// Check for explicit marker or typical secret patterns
		return strings.HasPrefix(str, "!secret ") || strings.HasPrefix(str, "secret:")
	}
	return false
}

// SecretTag represents the !secret YAML tag marker.
// When YAML is parsed with a custom unmarshaller, this type is used
// to preserve secret references.
type SecretTag struct {
	Reference *SecretReference
	Raw       string
}

// MarshalYAML implements yaml.Marshaler.
func (s SecretTag) MarshalYAML() (interface{}, error) {
	return "!secret " + s.Raw, nil
}

// SecretParameterProcessor processes parameters and resolves secret references.
type SecretParameterProcessor struct {
	resolver *MultiBackendResolver
}

// NewSecretParameterProcessor creates a new secret parameter processor.
func NewSecretParameterProcessor(resolver *MultiBackendResolver) *SecretParameterProcessor {
	return &SecretParameterProcessor{
		resolver: resolver,
	}
}

// ProcessParameters walks through parameters and resolves secret references.
// It returns a new map with secrets resolved to their actual values.
func (p *SecretParameterProcessor) ProcessParameters(params map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for key, value := range params {
		resolved, err := p.processValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to process parameter %s: %w", key, err)
		}
		result[key] = resolved
	}

	return result, nil
}

// processValue recursively processes a value, resolving secret references.
func (p *SecretParameterProcessor) processValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case SecretTag:
		// Direct secret tag - resolve it
		if p.resolver == nil {
			return nil, fmt.Errorf("no secret resolver configured")
		}
		return p.resolver.Resolve(v.Reference)

	case *SecretReference:
		// Secret reference - resolve it
		if p.resolver == nil {
			return nil, fmt.Errorf("no secret resolver configured")
		}
		return p.resolver.Resolve(v)

	case string:
		// Check if string contains embedded secret reference
		if strings.HasPrefix(v, "!secret ") {
			refStr := strings.TrimPrefix(v, "!secret ")
			ref, err := ParseSecretReference(refStr)
			if err != nil {
				return nil, err
			}
			if p.resolver == nil {
				return nil, fmt.Errorf("no secret resolver configured")
			}
			return p.resolver.Resolve(ref)
		}
		return v, nil

	case map[string]interface{}:
		// Recursively process map values
		result := make(map[string]interface{})
		for k, val := range v {
			resolved, err := p.processValue(val)
			if err != nil {
				return nil, fmt.Errorf("key %s: %w", k, err)
			}
			result[k] = resolved
		}
		return result, nil

	case []interface{}:
		// Recursively process array values
		result := make([]interface{}, len(v))
		for i, val := range v {
			resolved, err := p.processValue(val)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			result[i] = resolved
		}
		return result, nil

	default:
		// Return other types as-is
		return v, nil
	}
}

// CollectSecretReferences walks through parameters and collects all secret references
// without resolving them. This is useful for validation and auditing.
func CollectSecretReferences(params map[string]interface{}) []*SecretReference {
	var refs []*SecretReference
	collectSecretRefsFromValue(params, &refs)
	return refs
}

func collectSecretRefsFromValue(value interface{}, refs *[]*SecretReference) {
	switch v := value.(type) {
	case SecretTag:
		*refs = append(*refs, v.Reference)

	case *SecretReference:
		*refs = append(*refs, v)

	case string:
		if strings.HasPrefix(v, "!secret ") {
			refStr := strings.TrimPrefix(v, "!secret ")
			ref, err := ParseSecretReference(refStr)
			if err == nil {
				*refs = append(*refs, ref)
			}
		}

	case map[string]interface{}:
		for _, val := range v {
			collectSecretRefsFromValue(val, refs)
		}

	case []interface{}:
		for _, val := range v {
			collectSecretRefsFromValue(val, refs)
		}
	}
}

// ValidateSecretReferences validates that all secret references in parameters
// can be resolved (the secrets exist).
func ValidateSecretReferences(params map[string]interface{}, resolver *MultiBackendResolver) error {
	refs := CollectSecretReferences(params)

	for _, ref := range refs {
		backendName := ref.Backend
		if backendName == "" {
			backendName = resolver.defaultBackend
		}

		backend, ok := resolver.backends[backendName]
		if !ok {
			if backendName == "" {
				return fmt.Errorf("no default secrets backend configured for secret: %s", ref.Path)
			}
			return fmt.Errorf("unknown secrets backend %q for secret: %s", backendName, ref.Path)
		}

		exists, err := backend.Exists(ref.Path)
		if err != nil {
			return fmt.Errorf("failed to check secret %s: %w", ref.Path, err)
		}
		if !exists {
			return fmt.Errorf("secret not found: %s", ref.Path)
		}
	}

	return nil
}

// InMemorySecretResolver is a simple in-memory secrets resolver for testing.
type InMemorySecretResolver struct {
	secrets map[string]map[string]string // path -> version -> value
}

// NewInMemorySecretResolver creates a new in-memory resolver.
func NewInMemorySecretResolver() *InMemorySecretResolver {
	return &InMemorySecretResolver{
		secrets: make(map[string]map[string]string),
	}
}

// SetSecret stores a secret value.
func (r *InMemorySecretResolver) SetSecret(path, value string) {
	r.SetSecretVersion(path, "latest", value)
}

// SetSecretVersion stores a specific version of a secret.
func (r *InMemorySecretResolver) SetSecretVersion(path, version, value string) {
	if r.secrets[path] == nil {
		r.secrets[path] = make(map[string]string)
	}
	r.secrets[path][version] = value
	// Also update "latest" if this is a new version
	r.secrets[path]["latest"] = value
}

// Resolve retrieves a secret value.
func (r *InMemorySecretResolver) Resolve(path string) (string, error) {
	return r.ResolveWithVersion(path, "latest")
}

// ResolveWithVersion retrieves a specific version of a secret.
func (r *InMemorySecretResolver) ResolveWithVersion(path, version string) (string, error) {
	versions, ok := r.secrets[path]
	if !ok {
		return "", fmt.Errorf("secret not found: %s", path)
	}

	value, ok := versions[version]
	if !ok {
		return "", fmt.Errorf("secret version not found: %s@%s", path, version)
	}

	return value, nil
}

// Exists checks if a secret exists.
func (r *InMemorySecretResolver) Exists(path string) (bool, error) {
	_, ok := r.secrets[path]
	return ok, nil
}

// EnvironmentSecretResolver resolves secrets from environment variables.
// Secret paths are converted to environment variable names:
// "database/password" -> "KSCORE_SECRET_DATABASE_PASSWORD"
type EnvironmentSecretResolver struct {
	prefix string                 // Environment variable prefix (default: "KSCORE_SECRET")
	getter func(string) string    // Function to get env vars (defaults to os.Getenv)
}

// NewEnvironmentSecretResolver creates a new environment variable resolver.
func NewEnvironmentSecretResolver(getter func(string) string) *EnvironmentSecretResolver {
	return &EnvironmentSecretResolver{
		prefix: "KSCORE_SECRET",
		getter: getter,
	}
}

// SetPrefix sets the environment variable prefix.
func (r *EnvironmentSecretResolver) SetPrefix(prefix string) {
	r.prefix = prefix
}

// Resolve retrieves a secret from environment variables.
func (r *EnvironmentSecretResolver) Resolve(path string) (string, error) {
	envName := r.pathToEnvName(path)
	value := r.getter(envName)
	if value == "" {
		return "", fmt.Errorf("environment variable not set: %s", envName)
	}
	return value, nil
}

// ResolveWithVersion is the same as Resolve (env vars don't have versions).
func (r *EnvironmentSecretResolver) ResolveWithVersion(path, version string) (string, error) {
	return r.Resolve(path)
}

// Exists checks if the environment variable exists.
func (r *EnvironmentSecretResolver) Exists(path string) (bool, error) {
	envName := r.pathToEnvName(path)
	value := r.getter(envName)
	return value != "", nil
}

// pathToEnvName converts a secret path to an environment variable name.
// Example: "database/password" -> "KSCORE_SECRET_DATABASE_PASSWORD"
func (r *EnvironmentSecretResolver) pathToEnvName(path string) string {
	// Replace path separators with underscores
	name := strings.ReplaceAll(path, "/", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ToUpper(name)
	return r.prefix + "_" + name
}
