package injection

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// EnvInjector injects secrets as environment variables.
type EnvInjector struct {
	config *EnvInjectionConfig
	source SecretSource

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Track current environment
	injectedVars map[string]string
}

// NewEnvInjector creates a new environment variable injector.
func NewEnvInjector(config *EnvInjectionConfig, source SecretSource) (*EnvInjector, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if source == nil {
		return nil, fmt.Errorf("source is required")
	}

	if config.RefreshInterval <= 0 {
		config.RefreshInterval = 30 * time.Second
	}

	return &EnvInjector{
		config:       config,
		source:       source,
		injectedVars: make(map[string]string),
	}, nil
}

// Inject performs the environment variable injection.
func (e *EnvInjector) Inject(ctx context.Context) ([]InjectionResult, error) {
	results := make([]InjectionResult, 0, len(e.config.Secrets))

	for _, rule := range e.config.Secrets {
		result := e.injectEnv(ctx, rule)
		results = append(results, result)

		if result.Success {
			e.mu.Lock()
			e.injectedVars[result.Target] = os.Getenv(result.Target)
			e.mu.Unlock()
		}
	}

	return results, nil
}

func (e *EnvInjector) injectEnv(ctx context.Context, rule EnvRule) InjectionResult {
	result := InjectionResult{
		Type:      InjectionTypeEnv,
		Target:    rule.EnvVar,
		Timestamp: time.Now(),
	}

	// Get the secret
	secret, err := e.source.GetSecret(ctx, rule.SecretPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to get secret %s: %w", rule.SecretPath, err)
		return result
	}

	if secret == nil {
		result.Error = fmt.Errorf("secret not found: %s", rule.SecretPath)
		return result
	}

	result.SecretVersion = secret.Version

	// Get the value
	value := secret.GetString(rule.SecretKey)
	if value == "" {
		// Try getting as bytes and convert
		if data := secret.GetBytes(rule.SecretKey); data != nil {
			value = string(data)
		}
	}

	if value == "" {
		result.Error = fmt.Errorf("key %q not found or empty in secret %s", rule.SecretKey, rule.SecretPath)
		return result
	}

	// Determine environment variable name
	envVar := rule.EnvVar
	if envVar == "" {
		envVar = rule.SecretKey
	}

	// Apply prefix and case transformation
	if e.config.Prefix != "" {
		envVar = e.config.Prefix + envVar
	}
	if e.config.Uppercase {
		envVar = strings.ToUpper(envVar)
	}

	// Replace invalid characters
	envVar = sanitizeEnvVar(envVar)

	result.Target = envVar

	// Set the environment variable
	if err := os.Setenv(envVar, value); err != nil {
		result.Error = fmt.Errorf("failed to set env var %s: %w", envVar, err)
		return result
	}

	result.Success = true
	return result
}

// sanitizeEnvVar replaces invalid characters in environment variable names.
func sanitizeEnvVar(name string) string {
	var result strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r == '_' {
			result.WriteRune(r)
		} else if r >= '0' && r <= '9' && i > 0 {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}

// GetEnv returns the current injected environment as a map.
func (e *EnvInjector) GetEnv() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]string, len(e.injectedVars))
	for k, v := range e.injectedVars {
		result[k] = v
	}
	return result
}

// GetEnvSlice returns the current injected environment as KEY=VALUE slice.
func (e *EnvInjector) GetEnvSlice() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]string, 0, len(e.injectedVars))
	for k := range e.injectedVars {
		result = append(result, k+"="+os.Getenv(k))
	}
	return result
}

// Watch starts watching for secret changes and re-injecting.
func (e *EnvInjector) Watch(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("already running")
	}
	e.running = true
	e.stopCh = make(chan struct{})
	e.doneCh = make(chan struct{})
	e.mu.Unlock()

	go e.watchLoop(ctx)
	return nil
}

func (e *EnvInjector) watchLoop(ctx context.Context) {
	defer close(e.doneCh)

	ticker := time.NewTicker(e.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			results, err := e.Inject(ctx)
			if err != nil {
				fmt.Printf("env injection error: %v\n", err)
				continue
			}

			for _, r := range results {
				if !r.Success {
					fmt.Printf("env injection failed for %s: %v\n", r.Target, r.Error)
				}
			}
		}
	}
}

// Stop stops the environment injector.
func (e *EnvInjector) Stop() error {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()

	close(e.stopCh)
	<-e.doneCh

	e.mu.Lock()
	e.running = false
	e.mu.Unlock()

	return nil
}

// Clear removes all injected environment variables.
func (e *EnvInjector) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for envVar := range e.injectedVars {
		os.Unsetenv(envVar)
	}
	e.injectedVars = make(map[string]string)
}

// =============================================================================
// Process Environment Builder
// =============================================================================

// ProcessEnvBuilder builds environment for spawned processes.
type ProcessEnvBuilder struct {
	config *EnvInjectionConfig
	source SecretSource

	// Base environment (inherited or custom)
	baseEnv []string
}

// NewProcessEnvBuilder creates a new process environment builder.
func NewProcessEnvBuilder(config *EnvInjectionConfig, source SecretSource) *ProcessEnvBuilder {
	return &ProcessEnvBuilder{
		config:  config,
		source:  source,
		baseEnv: os.Environ(),
	}
}

// WithBaseEnv sets the base environment.
func (b *ProcessEnvBuilder) WithBaseEnv(env []string) *ProcessEnvBuilder {
	b.baseEnv = env
	return b
}

// WithInheritedEnv uses the current process environment as base.
func (b *ProcessEnvBuilder) WithInheritedEnv() *ProcessEnvBuilder {
	b.baseEnv = os.Environ()
	return b
}

// WithCleanEnv starts with an empty environment.
func (b *ProcessEnvBuilder) WithCleanEnv() *ProcessEnvBuilder {
	b.baseEnv = nil
	return b
}

// Build builds the environment for a process.
func (b *ProcessEnvBuilder) Build(ctx context.Context) ([]string, error) {
	env := make([]string, len(b.baseEnv))
	copy(env, b.baseEnv)

	// Track existing vars for deduplication
	existing := make(map[string]int) // var name -> index in env
	for i, e := range env {
		if idx := strings.Index(e, "="); idx > 0 {
			existing[e[:idx]] = i
		}
	}

	// Inject secrets
	for _, rule := range b.config.Secrets {
		secret, err := b.source.GetSecret(ctx, rule.SecretPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get secret %s: %w", rule.SecretPath, err)
		}
		if secret == nil {
			return nil, fmt.Errorf("secret not found: %s", rule.SecretPath)
		}

		value := secret.GetString(rule.SecretKey)
		if value == "" {
			if data := secret.GetBytes(rule.SecretKey); data != nil {
				value = string(data)
			}
		}

		envVar := rule.EnvVar
		if envVar == "" {
			envVar = rule.SecretKey
		}
		if b.config.Prefix != "" {
			envVar = b.config.Prefix + envVar
		}
		if b.config.Uppercase {
			envVar = strings.ToUpper(envVar)
		}
		envVar = sanitizeEnvVar(envVar)

		entry := envVar + "=" + value

		// Update or append
		if idx, exists := existing[envVar]; exists {
			env[idx] = entry
		} else {
			env = append(env, entry)
			existing[envVar] = len(env) - 1
		}
	}

	return env, nil
}
