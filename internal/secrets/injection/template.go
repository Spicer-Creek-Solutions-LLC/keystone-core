package injection

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"
)

// TemplateInjector renders secrets into templates.
type TemplateInjector struct {
	config *TemplateInjectionConfig
	source SecretSource
	notify *NotifyConfig

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Cache for loaded templates
	templates map[string]*template.Template

	// Track rendered files
	renderedFiles map[string]string // destination -> content hash
}

// NewTemplateInjector creates a new template injector.
func NewTemplateInjector(config *TemplateInjectionConfig, source SecretSource, notify *NotifyConfig) (*TemplateInjector, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if source == nil {
		return nil, fmt.Errorf("source is required")
	}

	if config.RefreshInterval <= 0 {
		config.RefreshInterval = 30 * time.Second
	}

	return &TemplateInjector{
		config:        config,
		source:        source,
		notify:        notify,
		templates:     make(map[string]*template.Template),
		renderedFiles: make(map[string]string),
	}, nil
}

// Inject performs the template injection.
func (t *TemplateInjector) Inject(ctx context.Context) ([]Result, error) {
	results := make([]Result, 0, len(t.config.Templates))

	anyChanged := false
	for _, rule := range t.config.Templates {
		result := t.injectTemplate(ctx, rule)
		results = append(results, result)

		if result.Success && result.Error == nil {
			// Check if content actually changed
			t.mu.Lock()
			if oldHash, exists := t.renderedFiles[rule.Destination]; !exists || oldHash != result.Target {
				anyChanged = true
			}
			t.mu.Unlock()
		}
	}

	// Send notification if configured and content changed
	if t.notify != nil && anyChanged {
		if err := t.sendNotification(ctx); err != nil {
			fmt.Printf("warning: notification failed: %v\n", err)
		}
	}

	return results, nil
}

func (t *TemplateInjector) injectTemplate(ctx context.Context, rule TemplateRule) Result {
	result := Result{
		Type:      TypeTemplate,
		Target:    rule.Destination,
		Timestamp: time.Now(),
	}

	// Create template context with secret functions
	funcs := t.createTemplateFuncs(ctx)

	// Parse template with functions (functions must be defined before parsing)
	tmpl, err := t.parseTemplateWithFuncs(rule, funcs)
	if err != nil {
		result.Error = fmt.Errorf("failed to load template %s: %w", rule.Source, err)
		return result
	}

	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		result.Error = fmt.Errorf("failed to render template %s: %w", rule.Source, err)
		return result
	}

	content := buf.Bytes()
	contentHash := hashContent(content)

	// Check if content changed
	t.mu.RLock()
	oldHash := t.renderedFiles[rule.Destination]
	t.mu.RUnlock()

	if oldHash == contentHash {
		// No change
		result.Success = true
		return result
	}

	// Determine file mode
	mode := rule.Mode
	if mode == 0 {
		mode = 0o644
	}

	// Ensure directory exists
	dir := filepath.Dir(rule.Destination)
	//nolint:gosec // G301: template output directory needs to be accessible by service user
	if err := os.MkdirAll(dir, 0o755); err != nil {
		result.Error = fmt.Errorf("failed to create directory %s: %w", dir, err)
		return result
	}

	// Write atomically
	if err := atomicWriteFile(rule.Destination, content, mode); err != nil {
		result.Error = fmt.Errorf("failed to write %s: %w", rule.Destination, err)
		return result
	}

	// Set ownership if specified
	if rule.Owner != "" || rule.Group != "" {
		if err := setFileOwnership(rule.Destination, rule.Owner, rule.Group); err != nil {
			result.Error = fmt.Errorf("failed to set ownership: %w", err)
			return result
		}
	}

	// Update cache
	t.mu.Lock()
	t.renderedFiles[rule.Destination] = contentHash
	t.mu.Unlock()

	result.Success = true
	result.Target = contentHash // Use hash to track changes
	return result
}

func (t *TemplateInjector) parseTemplateWithFuncs(rule TemplateRule, funcs template.FuncMap) (*template.Template, error) {
	content, err := os.ReadFile(rule.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to read template: %w", err)
	}

	// Create template with custom delimiters
	leftDelim := rule.LeftDelim
	rightDelim := rule.RightDelim
	if leftDelim == "" {
		leftDelim = "{{"
	}
	if rightDelim == "" {
		rightDelim = "}}"
	}

	tmpl := template.New(filepath.Base(rule.Source)).
		Delims(leftDelim, rightDelim).
		Funcs(funcs)

	return tmpl.Parse(string(content))
}

// createTemplateFuncs creates the template functions for secret access.
func (t *TemplateInjector) createTemplateFuncs(ctx context.Context) template.FuncMap {
	return template.FuncMap{
		// secret "path" "key" - get a secret value
		"secret": func(path, key string) string {
			secret, err := t.source.GetSecret(ctx, path)
			if err != nil || secret == nil {
				return ""
			}
			return secret.GetString(key)
		},

		// secret_bytes "path" "key" - get a secret as bytes (for binary data)
		"secret_bytes": func(path, key string) []byte {
			secret, err := t.source.GetSecret(ctx, path)
			if err != nil || secret == nil {
				return nil
			}
			return secret.GetBytes(key)
		},

		// dynamic_secret "path" - get entire dynamic secret as map
		"dynamic_secret": func(path string) map[string]interface{} {
			secret, err := t.source.GetSecret(ctx, path)
			if err != nil || secret == nil {
				return nil
			}
			return secret.Data
		},

		// with_secret "path" - returns Secret for use with range/with
		"with_secret": func(path string) *Secret {
			secret, err := t.source.GetSecret(ctx, path)
			if err != nil {
				return nil
			}
			return secret
		},

		// default - provide default value if empty
		"default": func(defaultVal, val interface{}) interface{} {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},

		// indent - indent text by n spaces
		"indent": func(spaces int, text string) string {
			indent := strings.Repeat(" ", spaces)
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if line != "" {
					lines[i] = indent + line
				}
			}
			return strings.Join(lines, "\n")
		},

		// json - marshal value as JSON
		"json": func(v interface{}) string {
			// Simple JSON for common types
			switch val := v.(type) {
			case string:
				return `"` + escapeJSON(val) + `"`
			case bool:
				if val {
					return "true"
				}
				return "false"
			case int, int64, float64:
				return fmt.Sprintf("%v", val)
			case nil:
				return "null"
			default:
				return fmt.Sprintf("%v", v)
			}
		},

		// toUpper - convert to uppercase
		"toUpper": strings.ToUpper,

		// toLower - convert to lowercase
		"toLower": strings.ToLower,

		// trimSpace - trim whitespace
		"trimSpace": strings.TrimSpace,

		// replace - replace string
		"replace": strings.ReplaceAll,

		// contains - check if string contains substring
		"contains": strings.Contains,

		// hasPrefix - check if string has prefix
		"hasPrefix": strings.HasPrefix,

		// hasSuffix - check if string has suffix
		"hasSuffix": strings.HasSuffix,

		// split - split string
		"split": strings.Split,

		// join - join strings
		"join": strings.Join,

		// env - get environment variable
		"env": os.Getenv,

		// now - current time
		"now": time.Now,

		// formatTime - format time
		"formatTime": func(layout string, t time.Time) string {
			return t.Format(layout)
		},
	}
}

// sendNotification sends process notification after template rendering.
func (t *TemplateInjector) sendNotification(ctx context.Context) error {
	if t.notify == nil {
		return nil
	}

	// Send signal if configured
	if t.notify.Signal != "" && t.notify.PIDFile != "" {
		sig, err := parseSignal(t.notify.Signal)
		if err != nil {
			return err
		}

		pidData, err := os.ReadFile(t.notify.PIDFile)
		if err != nil {
			return fmt.Errorf("failed to read PID file: %w", err)
		}

		var pid int
		if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err != nil {
			return fmt.Errorf("invalid PID: %w", err)
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("failed to find process: %w", err)
		}

		if err := proc.Signal(sig); err != nil {
			return fmt.Errorf("failed to send signal: %w", err)
		}
	}

	// Run command if configured
	if len(t.notify.Command) > 0 {
		timeout := t.notify.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}

		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := execCommand(cmdCtx, t.notify.Command[0], t.notify.Command[1:]...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("notification command failed: %w", err)
		}
	}

	return nil
}

// Watch starts watching for secret changes and re-rendering.
func (t *TemplateInjector) Watch(ctx context.Context) error {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return fmt.Errorf("already running")
	}
	t.running = true
	t.stopCh = make(chan struct{})
	t.doneCh = make(chan struct{})
	t.mu.Unlock()

	go t.watchLoop(ctx)
	return nil
}

func (t *TemplateInjector) watchLoop(ctx context.Context) {
	defer close(t.doneCh)

	ticker := time.NewTicker(t.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopCh:
			return
		case <-ticker.C:
			results, err := t.Inject(ctx)
			if err != nil {
				fmt.Printf("template injection error: %v\n", err)
				continue
			}

			for _, r := range results {
				if !r.Success {
					fmt.Printf("template injection failed for %s: %v\n", r.Target, r.Error)
				}
			}
		}
	}
}

// Stop stops the template injector.
func (t *TemplateInjector) Stop() error {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return nil
	}
	t.mu.Unlock()

	close(t.stopCh)
	<-t.doneCh

	t.mu.Lock()
	t.running = false
	t.mu.Unlock()

	return nil
}

// ClearCache clears the template cache.
func (t *TemplateInjector) ClearCache() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.templates = make(map[string]*template.Template)
}

// =============================================================================
// Helper functions
// =============================================================================

// atomicWriteFile writes a file atomically.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmpFile, err := os.CreateTemp(dir, "."+base+".tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	success = true
	return nil
}

// setFileOwnership sets file ownership.
func setFileOwnership(path, owner, group string) error {
	// Reuse the implementation from file.go
	f := &FileInjector{}
	return f.setOwnership(path, owner, group)
}

// hashContent creates a simple hash of content for change detection.
func hashContent(data []byte) string {
	// Use a simple checksum for change detection
	var sum uint64
	for i, b := range data {
		sum += uint64(b) * uint64(i+1) //nolint:gosec // G115: loop index is bounded by len(data)
	}
	return fmt.Sprintf("%016x", sum)
}

// escapeJSON escapes a string for JSON.
func escapeJSON(s string) string {
	var result strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			result.WriteRune(r)
		}
	}
	return result.String()
}
