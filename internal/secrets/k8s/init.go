package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets/injection"
)

// InitContainer fetches secrets before the main container starts.
type InitContainer struct {
	config *InitContainerConfig
	source injection.SecretSource

	// Injectors
	fileInjector *injection.FileInjector
}

// InitContainerConfig configures the init container.
type InitContainerConfig struct {
	// SecretVolumePath is the path to write secrets.
	SecretVolumePath string

	// Secrets is the list of secrets to inject.
	Secrets []SecretInjection

	// Templates is the list of templates to render.
	Templates []TemplateSpec

	// Timeout is the maximum time to wait for secrets.
	Timeout time.Duration

	// RetryInterval is how often to retry failed fetches.
	RetryInterval time.Duration

	// MaxRetries is the maximum number of retries.
	MaxRetries int

	// FailOnError determines if the init container should fail on error.
	FailOnError bool
}

// NewInitContainer creates a new init container instance.
func NewInitContainer(config *InitContainerConfig, source injection.SecretSource) (*InitContainer, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if source == nil {
		return nil, fmt.Errorf("source is required")
	}

	// Set defaults
	if config.SecretVolumePath == "" {
		config.SecretVolumePath = "/secrets"
	}
	if config.Timeout <= 0 {
		config.Timeout = 60 * time.Second
	}
	if config.RetryInterval <= 0 {
		config.RetryInterval = 5 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}

	return &InitContainer{
		config: config,
		source: source,
	}, nil
}

// Run executes the init container logic.
func (i *InitContainer) Run(ctx context.Context) error {
	// Create timeout context
	ctx, cancel := context.WithTimeout(ctx, i.config.Timeout)
	defer cancel()

	// Initialize injector
	if err := i.initializeInjector(); err != nil {
		return fmt.Errorf("failed to initialize injector: %w", err)
	}

	// Retry loop
	var lastErr error
	for attempt := 0; attempt <= i.config.MaxRetries; attempt++ {
		if attempt > 0 {
			fmt.Printf("retrying secret injection (attempt %d/%d)\n", attempt, i.config.MaxRetries)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(i.config.RetryInterval):
			}
		}

		err := i.inject(ctx)
		if err == nil {
			fmt.Println("secrets injected successfully")
			return nil
		}

		lastErr = err
		fmt.Printf("injection failed: %v\n", err)
	}

	if i.config.FailOnError {
		return fmt.Errorf("failed to inject secrets after %d attempts: %w", i.config.MaxRetries, lastErr)
	}

	fmt.Printf("warning: failed to inject secrets, continuing anyway: %v\n", lastErr)
	return nil
}

func (i *InitContainer) initializeInjector() error {
	// Build file injection rules
	var fileRules []injection.FileRule
	for j := range i.config.Secrets {
		secret := &i.config.Secrets[j]
		if secret.Type == SecretTypeFile || secret.Type == "" {
			rule := injection.FileRule{
				SecretPath: secret.SecretPath,
				SecretKey:  secret.SecretKey,
				FilePath:   i.resolveFilePath(*secret),
			}
			if secret.FileMode != "" {
				mode, err := parseFileMode(secret.FileMode)
				if err != nil {
					return fmt.Errorf("invalid file mode %s: %w", secret.FileMode, err)
				}
				rule.Mode = mode
			}
			fileRules = append(fileRules, rule)
		}
	}

	if len(fileRules) > 0 {
		fileConfig := &injection.FileInjectionConfig{
			Config: injection.Config{Enabled: true},
			BasePath:        i.config.SecretVolumePath,
			DefaultMode:     0o600,
			AtomicWrite:     true,
			Files:           fileRules,
		}

		injector, err := injection.NewFileInjector(fileConfig, i.source, nil)
		if err != nil {
			return fmt.Errorf("failed to create file injector: %w", err)
		}
		i.fileInjector = injector
	}

	return nil
}

func (i *InitContainer) inject(ctx context.Context) error {
	if i.fileInjector == nil {
		return fmt.Errorf("no secrets configured")
	}

	results, err := i.fileInjector.Inject(ctx)
	if err != nil {
		return err
	}

	var errors []error
	for _, r := range results {
		if !r.Success && r.Error != nil {
			errors = append(errors, fmt.Errorf("%s: %w", r.Target, r.Error))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("injection errors: %v", errors)
	}

	return nil
}

func (i *InitContainer) resolveFilePath(secret SecretInjection) string {
	if secret.FilePath != "" {
		return secret.FilePath
	}
	return fmt.Sprintf("%s/%s", i.config.SecretVolumePath, secret.Name)
}

// =============================================================================
// Init Container Spec Builder
// =============================================================================

// InitContainerSpecBuilder builds container specs for the init container.
type InitContainerSpecBuilder struct {
	config *InjectorConfig
	spec   *PodInjectionSpec
}

// NewInitContainerSpecBuilder creates a new init container spec builder.
func NewInitContainerSpecBuilder(config *InjectorConfig) *InitContainerSpecBuilder {
	if config == nil {
		config = DefaultInjectorConfig()
	}
	return &InitContainerSpecBuilder{
		config: config,
	}
}

// WithInjectionSpec sets the injection specification.
func (b *InitContainerSpecBuilder) WithInjectionSpec(spec *PodInjectionSpec) *InitContainerSpecBuilder {
	b.spec = spec
	return b
}

// BuildContainerSpec builds the init container specification.
func (b *InitContainerSpecBuilder) BuildContainerSpec() map[string]interface{} {
	container := map[string]interface{}{
		"name":            "keystone-secret-init",
		"image":           b.config.Image,
		"imagePullPolicy": b.config.ImagePullPolicy,
		"args": []string{
			"init",
			"--volume-path", b.config.SecretVolumePath,
			"--timeout", "60s",
		},
		"volumeMounts": []map[string]interface{}{
			{
				"name":      "secrets-volume",
				"mountPath": b.config.SecretVolumePath,
			},
		},
	}

	// Add resource requirements (smaller for init)
	container["resources"] = map[string]interface{}{
		"limits": map[string]string{
			"cpu":    "50m",
			"memory": "32Mi",
		},
		"requests": map[string]string{
			"cpu":    "10m",
			"memory": "16Mi",
		},
	}

	// Add secrets configuration as environment variable
	if b.spec != nil && len(b.spec.Secrets) > 0 {
		secretsJSON, _ := json.Marshal(b.spec.Secrets)
		container["env"] = []map[string]interface{}{
			{
				"name":  "KEYSTONE_SECRETS",
				"value": string(secretsJSON),
			},
			{
				"name":  "KEYSTONE_BROKER_ADDRESS",
				"value": b.config.BrokerAddress,
			},
		}
	}

	// Add service account token mount if using service account auth
	if b.spec != nil && b.spec.ServiceAccountAuth {
		volumeMounts := container["volumeMounts"].([]map[string]interface{})
		volumeMounts = append(volumeMounts, map[string]interface{}{
			"name":      "sa-token",
			"mountPath": "/var/run/secrets/tokens",
			"readOnly":  true,
		})
		container["volumeMounts"] = volumeMounts
	}

	return container
}

// =============================================================================
// Main function for init/sidecar binary
// =============================================================================

// RunAsInit runs the injector in init container mode.
func RunAsInit(ctx context.Context, config *InitContainerConfig, source injection.SecretSource) error {
	init, err := NewInitContainer(config, source)
	if err != nil {
		return err
	}
	return init.Run(ctx)
}

// RunAsSidecar runs the injector in sidecar mode.
func RunAsSidecar(ctx context.Context, config *SidecarConfig, source injection.SecretSource) error {
	sidecar, err := NewSidecar(config, source)
	if err != nil {
		return err
	}
	return sidecar.Run(ctx)
}

// ParseSecretsFromEnv parses the KEYSTONE_SECRETS environment variable.
func ParseSecretsFromEnv() ([]SecretInjection, error) {
	secretsJSON := os.Getenv("KEYSTONE_SECRETS")
	if secretsJSON == "" {
		return nil, nil
	}

	var secrets []SecretInjection
	if err := json.Unmarshal([]byte(secretsJSON), &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse KEYSTONE_SECRETS: %w", err)
	}

	return secrets, nil
}
