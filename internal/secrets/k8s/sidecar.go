package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets/injection"
)

// Sidecar is the secret injection sidecar that runs alongside application containers.
// It fetches secrets from the broker and writes them to a shared volume.
type Sidecar struct {
	config *SidecarConfig
	source injection.SecretSource

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Injectors
	fileInjector     *injection.FileInjector
	templateInjector *injection.TemplateInjector

	// Stats
	stats SidecarStats
}

// SidecarConfig configures the sidecar.
type SidecarConfig struct {
	// SecretVolumePath is the path to write secrets.
	SecretVolumePath string

	// RefreshInterval is how often to refresh secrets.
	RefreshInterval time.Duration

	// Secrets is the list of secrets to inject.
	Secrets []SecretInjection

	// Templates is the list of templates to render.
	Templates []TemplateSpec

	// NotifySignal is the signal to send to the main container on update.
	NotifySignal string

	// NotifyPIDFile is the PID file of the main container.
	NotifyPIDFile string

	// GracePeriod is the time to wait before first injection.
	GracePeriod time.Duration
}

// TemplateSpec defines a template to render.
type TemplateSpec struct {
	// Source is the template content or file path.
	Source string

	// Destination is the output file path.
	Destination string

	// Mode is the file permissions.
	Mode os.FileMode
}

// SidecarStats contains sidecar statistics.
type SidecarStats struct {
	StartTime        time.Time
	LastRefreshTime  time.Time
	RefreshCount     int64
	RefreshErrors    int64
	SecretsInjected  int64
	TemplatesRendered int64
}

// NewSidecar creates a new sidecar instance.
func NewSidecar(config *SidecarConfig, source injection.SecretSource) (*Sidecar, error) {
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
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = 30 * time.Second
	}

	return &Sidecar{
		config: config,
		source: source,
		stats: SidecarStats{
			StartTime: time.Now(),
		},
	}, nil
}

// Run starts the sidecar and blocks until stopped.
func (s *Sidecar) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("sidecar already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.mu.Unlock()

	defer func() {
		close(s.doneCh)
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// Initialize injectors
	if err := s.initializeInjectors(); err != nil {
		return fmt.Errorf("failed to initialize injectors: %w", err)
	}

	// Wait for grace period
	if s.config.GracePeriod > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopCh:
			return nil
		case <-time.After(s.config.GracePeriod):
		}
	}

	// Initial injection
	if err := s.refresh(ctx); err != nil {
		fmt.Printf("warning: initial injection failed: %v\n", err)
	}

	// Setup signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	// Start refresh loop
	ticker := time.NewTicker(s.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stopCh:
			return nil
		case <-sigCh:
			fmt.Println("received shutdown signal")
			return nil
		case <-ticker.C:
			if err := s.refresh(ctx); err != nil {
				fmt.Printf("refresh error: %v\n", err)
				s.mu.Lock()
				s.stats.RefreshErrors++
				s.mu.Unlock()
			}
		}
	}
}

// Stop stops the sidecar.
func (s *Sidecar) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	close(s.stopCh)
	<-s.doneCh
	return nil
}

// Stats returns the sidecar statistics.
func (s *Sidecar) Stats() SidecarStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *Sidecar) initializeInjectors() error {
	// Build file injection rules
	var fileRules []injection.FileRule
	for _, secret := range s.config.Secrets {
		if secret.Type == SecretTypeFile || secret.Type == "" {
			rule := injection.FileRule{
				SecretPath: secret.SecretPath,
				SecretKey:  secret.SecretKey,
				FilePath:   s.resolveFilePath(secret),
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
		var notify *injection.NotifyConfig
		if s.config.NotifySignal != "" && s.config.NotifyPIDFile != "" {
			notify = &injection.NotifyConfig{
				Signal:  s.config.NotifySignal,
				PIDFile: s.config.NotifyPIDFile,
			}
		}

		fileConfig := &injection.FileInjectionConfig{
			InjectionConfig: injection.InjectionConfig{
				Enabled:         true,
				RefreshInterval: s.config.RefreshInterval,
			},
			BasePath:    s.config.SecretVolumePath,
			DefaultMode: 0600,
			AtomicWrite: true,
			Files:       fileRules,
		}

		injector, err := injection.NewFileInjector(fileConfig, s.source, notify)
		if err != nil {
			return fmt.Errorf("failed to create file injector: %w", err)
		}
		s.fileInjector = injector
	}

	// Build template injection rules
	if len(s.config.Templates) > 0 {
		var templateRules []injection.TemplateRule
		for _, tmpl := range s.config.Templates {
			rule := injection.TemplateRule{
				Source:      tmpl.Source,
				Destination: tmpl.Destination,
				Mode:        tmpl.Mode,
			}
			if rule.Mode == 0 {
				rule.Mode = 0644
			}
			templateRules = append(templateRules, rule)
		}

		var notify *injection.NotifyConfig
		if s.config.NotifySignal != "" && s.config.NotifyPIDFile != "" {
			notify = &injection.NotifyConfig{
				Signal:  s.config.NotifySignal,
				PIDFile: s.config.NotifyPIDFile,
			}
		}

		templateConfig := &injection.TemplateInjectionConfig{
			InjectionConfig: injection.InjectionConfig{
				Enabled:         true,
				RefreshInterval: s.config.RefreshInterval,
			},
			Templates: templateRules,
		}

		injector, err := injection.NewTemplateInjector(templateConfig, s.source, notify)
		if err != nil {
			return fmt.Errorf("failed to create template injector: %w", err)
		}
		s.templateInjector = injector
	}

	return nil
}

func (s *Sidecar) refresh(ctx context.Context) error {
	s.mu.Lock()
	s.stats.RefreshCount++
	s.stats.LastRefreshTime = time.Now()
	s.mu.Unlock()

	var lastErr error

	// Inject files
	if s.fileInjector != nil {
		results, err := s.fileInjector.Inject(ctx)
		if err != nil {
			lastErr = err
		} else {
			for _, r := range results {
				if r.Success {
					s.mu.Lock()
					s.stats.SecretsInjected++
					s.mu.Unlock()
				} else if r.Error != nil {
					fmt.Printf("file injection error for %s: %v\n", r.Target, r.Error)
					lastErr = r.Error
				}
			}
		}
	}

	// Render templates
	if s.templateInjector != nil {
		results, err := s.templateInjector.Inject(ctx)
		if err != nil {
			lastErr = err
		} else {
			for _, r := range results {
				if r.Success {
					s.mu.Lock()
					s.stats.TemplatesRendered++
					s.mu.Unlock()
				} else if r.Error != nil {
					fmt.Printf("template injection error for %s: %v\n", r.Target, r.Error)
					lastErr = r.Error
				}
			}
		}
	}

	return lastErr
}

func (s *Sidecar) resolveFilePath(secret SecretInjection) string {
	if secret.FilePath != "" {
		return secret.FilePath
	}
	// Default to secret name as filename
	return fmt.Sprintf("%s/%s", s.config.SecretVolumePath, secret.Name)
}

// parseFileMode parses a file mode string (e.g., "0600", "600").
func parseFileMode(mode string) (os.FileMode, error) {
	var m uint32
	_, err := fmt.Sscanf(mode, "%o", &m)
	if err != nil {
		return 0, err
	}
	return os.FileMode(m), nil
}

// =============================================================================
// Sidecar Container Spec Builder
// =============================================================================

// SidecarSpecBuilder builds container specs for the sidecar.
type SidecarSpecBuilder struct {
	config *InjectorConfig
	spec   *PodInjectionSpec
}

// NewSidecarSpecBuilder creates a new sidecar spec builder.
func NewSidecarSpecBuilder(config *InjectorConfig) *SidecarSpecBuilder {
	if config == nil {
		config = DefaultInjectorConfig()
	}
	return &SidecarSpecBuilder{
		config: config,
	}
}

// WithInjectionSpec sets the injection specification.
func (b *SidecarSpecBuilder) WithInjectionSpec(spec *PodInjectionSpec) *SidecarSpecBuilder {
	b.spec = spec
	return b
}

// BuildContainerSpec builds the sidecar container specification.
func (b *SidecarSpecBuilder) BuildContainerSpec() map[string]interface{} {
	container := map[string]interface{}{
		"name":            "keystone-secret-sidecar",
		"image":           b.config.Image,
		"imagePullPolicy": b.config.ImagePullPolicy,
		"args": []string{
			"sidecar",
			"--volume-path", b.config.SecretVolumePath,
			"--refresh-interval", b.config.RefreshInterval.String(),
		},
		"volumeMounts": []map[string]interface{}{
			{
				"name":      "secrets-volume",
				"mountPath": b.config.SecretVolumePath,
			},
		},
	}

	// Add resource requirements
	if b.config.Resources != nil {
		resources := make(map[string]interface{})
		if len(b.config.Resources.Limits) > 0 {
			resources["limits"] = b.config.Resources.Limits
		}
		if len(b.config.Resources.Requests) > 0 {
			resources["requests"] = b.config.Resources.Requests
		}
		container["resources"] = resources
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

// BuildVolumeSpec builds the shared volume specification.
func (b *SidecarSpecBuilder) BuildVolumeSpec() map[string]interface{} {
	return map[string]interface{}{
		"name": "secrets-volume",
		"emptyDir": map[string]interface{}{
			"medium": "Memory",
		},
	}
}

// BuildServiceAccountVolumeSpec builds the service account token volume.
func (b *SidecarSpecBuilder) BuildServiceAccountVolumeSpec() map[string]interface{} {
	return map[string]interface{}{
		"name": "sa-token",
		"projected": map[string]interface{}{
			"sources": []map[string]interface{}{
				{
					"serviceAccountToken": map[string]interface{}{
						"path":              "token",
						"expirationSeconds": 3600,
						"audience":          "keystone-secrets",
					},
				},
			},
		},
	}
}
