package injection

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// FileInjector injects secrets into files.
type FileInjector struct {
	config *FileInjectionConfig
	source SecretSource
	notify *NotifyConfig

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	// Track injected files for cleanup
	injectedFiles map[string]int // path -> secret version
}

// NewFileInjector creates a new file injector.
func NewFileInjector(config *FileInjectionConfig, source SecretSource, notify *NotifyConfig) (*FileInjector, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if source == nil {
		return nil, fmt.Errorf("source is required")
	}

	// Set defaults
	if config.DefaultMode == 0 {
		config.DefaultMode = 0o600
	}
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = 30 * time.Second
	}

	return &FileInjector{
		config:        config,
		source:        source,
		notify:        notify,
		injectedFiles: make(map[string]int),
	}, nil
}

// Inject performs the file injection.
func (f *FileInjector) Inject(ctx context.Context) ([]Result, error) {
	results := make([]Result, 0, len(f.config.Files))

	for _, rule := range f.config.Files {
		result := f.injectFile(ctx, rule)
		results = append(results, result)

		if result.Success {
			f.mu.Lock()
			f.injectedFiles[result.Target] = result.SecretVersion
			f.mu.Unlock()
		}
	}

	// Send notification if configured and any injection succeeded
	if f.notify != nil {
		for _, r := range results {
			if r.Success {
				if err := f.sendNotification(ctx); err != nil {
					// Log but don't fail - injection succeeded
					fmt.Printf("warning: notification failed: %v\n", err)
				}
				break
			}
		}
	}

	return results, nil
}

func (f *FileInjector) injectFile(ctx context.Context, rule FileRule) Result {
	result := Result{
		Type:      TypeFile,
		Target:    rule.FilePath,
		Timestamp: time.Now(),
	}

	// Get the secret
	secret, err := f.source.GetSecret(ctx, rule.SecretPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to get secret %s: %w", rule.SecretPath, err)
		return result
	}

	if secret == nil {
		result.Error = fmt.Errorf("secret not found: %s", rule.SecretPath)
		return result
	}

	result.SecretVersion = secret.Version

	// Get the data to write
	var data []byte
	if rule.SecretKey != "" {
		// Get specific key
		data = secret.GetBytes(rule.SecretKey)
		if data == nil {
			result.Error = fmt.Errorf("key %q not found in secret %s", rule.SecretKey, rule.SecretPath)
			return result
		}
	} else {
		// Write all data as JSON if no specific key
		// For single-value secrets, use the first value
		if len(secret.Data) == 1 {
			for _, v := range secret.Data {
				switch val := v.(type) {
				case []byte:
					data = val
				case string:
					data = []byte(val)
				default:
					result.Error = fmt.Errorf("unsupported data type for file injection")
					return result
				}
			}
		} else {
			result.Error = fmt.Errorf("secret_key required when secret has multiple values")
			return result
		}
	}

	// Handle encoding
	if rule.Encoding == "base64" && !rule.Binary {
		decoded, err := base64.StdEncoding.DecodeString(string(data))
		if err != nil {
			result.Error = fmt.Errorf("failed to decode base64: %w", err)
			return result
		}
		data = decoded
	}

	// Determine file mode
	mode := f.config.DefaultMode
	if rule.Mode != 0 {
		mode = rule.Mode
	}

	// Ensure parent directory exists
	dir := filepath.Dir(rule.FilePath)
	//nolint:gosec // G301: secret file directory needs to be accessible by service user
	if err := os.MkdirAll(dir, 0o755); err != nil {
		result.Error = fmt.Errorf("failed to create directory %s: %w", dir, err)
		return result
	}

	// Write the file
	if f.config.AtomicWrite {
		err = f.atomicWrite(rule.FilePath, data, mode)
	} else {
		err = os.WriteFile(rule.FilePath, data, mode)
	}

	if err != nil {
		result.Error = fmt.Errorf("failed to write file %s: %w", rule.FilePath, err)
		return result
	}

	// Set ownership if specified
	owner := f.config.DefaultOwner
	group := f.config.DefaultGroup
	if rule.Owner != "" {
		owner = rule.Owner
	}
	if rule.Group != "" {
		group = rule.Group
	}

	if owner != "" || group != "" {
		if err := f.setOwnership(rule.FilePath, owner, group); err != nil {
			result.Error = fmt.Errorf("failed to set ownership: %w", err)
			return result
		}
	}

	result.Success = true
	return result
}

// atomicWrite writes data atomically using temp file + rename.
func (f *FileInjector) atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Create temp file in same directory
	tmpFile, err := os.CreateTemp(dir, "."+base+".tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Set permissions before rename
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

// setOwnership sets file ownership.
func (f *FileInjector) setOwnership(path, owner, group string) error {
	uid := -1
	gid := -1

	if owner != "" {
		u, err := user.Lookup(owner)
		if err != nil {
			// Try as numeric UID
			if id, err := strconv.Atoi(owner); err == nil {
				uid = id
			} else {
				return fmt.Errorf("failed to lookup user %s: %w", owner, err)
			}
		} else {
			uid, _ = strconv.Atoi(u.Uid)
		}
	}

	if group != "" {
		g, err := user.LookupGroup(group)
		if err != nil {
			// Try as numeric GID
			if id, err := strconv.Atoi(group); err == nil {
				gid = id
			} else {
				return fmt.Errorf("failed to lookup group %s: %w", group, err)
			}
		} else {
			gid, _ = strconv.Atoi(g.Gid)
		}
	}

	return os.Chown(path, uid, gid)
}

// sendNotification sends process notification after injection.
func (f *FileInjector) sendNotification(ctx context.Context) error {
	if f.notify == nil {
		return nil
	}

	// Send signal if configured
	if f.notify.Signal != "" && f.notify.PIDFile != "" {
		if err := f.sendSignal(); err != nil {
			return err
		}
	}

	// Run command if configured
	if len(f.notify.Command) > 0 {
		if err := f.runCommand(ctx); err != nil {
			return err
		}
	}

	return nil
}

// sendSignal sends a signal to the process specified in the PID file.
func (f *FileInjector) sendSignal() error {
	// Read PID file
	pidData, err := os.ReadFile(f.notify.PIDFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		return fmt.Errorf("invalid PID in file: %w", err)
	}

	// Parse signal
	sig, err := parseSignal(f.notify.Signal)
	if err != nil {
		return err
	}

	// Find process
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %w", pid, err)
	}

	// Send signal
	if err := proc.Signal(sig); err != nil {
		return fmt.Errorf("failed to send signal %s to process %d: %w", f.notify.Signal, pid, err)
	}

	return nil
}

// runCommand runs the notification command.
func (f *FileInjector) runCommand(ctx context.Context) error {
	timeout := f.notify.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := execCommand(cmdCtx, f.notify.Command[0], f.notify.Command[1:]...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("notification command failed: %w", err)
	}

	return nil
}

// Watch starts watching for secret changes and re-injecting.
func (f *FileInjector) Watch(ctx context.Context) error {
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return fmt.Errorf("already running")
	}
	f.running = true
	f.stopCh = make(chan struct{})
	f.doneCh = make(chan struct{})
	f.mu.Unlock()

	go f.watchLoop(ctx)
	return nil
}

func (f *FileInjector) watchLoop(ctx context.Context) {
	defer close(f.doneCh)

	ticker := time.NewTicker(f.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-f.stopCh:
			return
		case <-ticker.C:
			// Re-inject all files
			results, err := f.Inject(ctx)
			if err != nil {
				fmt.Printf("injection error: %v\n", err)
				continue
			}

			// Log any failures
			for _, r := range results {
				if !r.Success {
					fmt.Printf("injection failed for %s: %v\n", r.Target, r.Error)
				}
			}
		}
	}
}

// Stop stops the file injector.
func (f *FileInjector) Stop() error {
	f.mu.Lock()
	if !f.running {
		f.mu.Unlock()
		return nil
	}
	f.mu.Unlock()

	close(f.stopCh)
	<-f.doneCh

	f.mu.Lock()
	f.running = false
	f.mu.Unlock()

	return nil
}

// parseSignal parses a signal name to a syscall.Signal.
func parseSignal(name string) (os.Signal, error) {
	signals := map[string]syscall.Signal{
		"SIGHUP":  syscall.SIGHUP,
		"SIGINT":  syscall.SIGINT,
		"SIGQUIT": syscall.SIGQUIT,
		"SIGTERM": syscall.SIGTERM,
		"SIGUSR1": syscall.SIGUSR1,
		"SIGUSR2": syscall.SIGUSR2,
		"HUP":     syscall.SIGHUP,
		"INT":     syscall.SIGINT,
		"QUIT":    syscall.SIGQUIT,
		"TERM":    syscall.SIGTERM,
		"USR1":    syscall.SIGUSR1,
		"USR2":    syscall.SIGUSR2,
	}

	if sig, ok := signals[name]; ok {
		return sig, nil
	}

	// Try as numeric signal
	if num, err := strconv.Atoi(name); err == nil {
		return syscall.Signal(num), nil
	}

	return nil, fmt.Errorf("unknown signal: %s", name)
}
