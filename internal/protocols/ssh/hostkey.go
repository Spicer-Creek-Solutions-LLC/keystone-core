// Package ssh provides an SSH protocol adapter for proxy agents.
package ssh

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyCheckMode defines how host keys are verified.
type HostKeyCheckMode string

const (
	// HostKeyCheckStrict rejects any unknown host keys.
	HostKeyCheckStrict HostKeyCheckMode = "strict"

	// HostKeyCheckTOFU (Trust On First Use) accepts unknown hosts on first connection
	// and stores the key for future verification.
	HostKeyCheckTOFU HostKeyCheckMode = "tofu"

	// HostKeyCheckAcceptNew accepts new hosts but rejects changed keys for known hosts.
	HostKeyCheckAcceptNew HostKeyCheckMode = "accept-new"

	// HostKeyCheckNo disables host key checking (INSECURE - use only for testing).
	HostKeyCheckNo HostKeyCheckMode = "no"
)

// HostKeyVerifier provides SSH host key verification with support for
// known_hosts files and Trust On First Use (TOFU) mode.
type HostKeyVerifier struct {
	// Mode determines how host keys are verified.
	Mode HostKeyCheckMode

	// KnownHostsPath is the path to the known_hosts file.
	// Defaults to ~/.ssh/known_hosts if empty.
	KnownHostsPath string

	// SystemKnownHostsPath is the path to the system-wide known_hosts file.
	// Defaults to /etc/ssh/ssh_known_hosts if empty.
	SystemKnownHostsPath string

	// StoreLearnedKeys indicates whether to persist newly learned keys (TOFU mode).
	StoreLearnedKeys bool

	// OnKeyMismatch is called when a host key doesn't match the stored key.
	// Provides the hostname, expected key, and actual key for logging/alerting.
	OnKeyMismatch func(hostname string, expected, actual ssh.PublicKey)

	// OnNewKey is called when a new host key is learned (TOFU mode).
	// Provides the hostname and key for logging/auditing.
	OnNewKey func(hostname string, key ssh.PublicKey)

	mu          sync.RWMutex
	knownHosts  ssh.HostKeyCallback
	learnedKeys map[string]ssh.PublicKey
	initialized bool
}

// NewHostKeyVerifier creates a new host key verifier with the specified mode.
func NewHostKeyVerifier(mode HostKeyCheckMode) *HostKeyVerifier {
	return &HostKeyVerifier{
		Mode:             mode,
		StoreLearnedKeys: true,
		learnedKeys:      make(map[string]ssh.PublicKey),
	}
}

// NewStrictVerifier creates a verifier that only accepts known hosts.
func NewStrictVerifier(knownHostsPath string) *HostKeyVerifier {
	v := NewHostKeyVerifier(HostKeyCheckStrict)
	v.KnownHostsPath = knownHostsPath
	return v
}

// NewTOFUVerifier creates a verifier with Trust On First Use semantics.
func NewTOFUVerifier(knownHostsPath string) *HostKeyVerifier {
	v := NewHostKeyVerifier(HostKeyCheckTOFU)
	v.KnownHostsPath = knownHostsPath
	return v
}

// HostKeyCallback returns an ssh.HostKeyCallback that can be used with ssh.ClientConfig.
func (v *HostKeyVerifier) HostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		return v.Verify(hostname, remote, key)
	}
}

// Verify checks the host key against known hosts.
func (v *HostKeyVerifier) Verify(hostname string, remote net.Addr, key ssh.PublicKey) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Initialize on first use
	if !v.initialized {
		if err := v.initializeLocked(); err != nil {
			return fmt.Errorf("failed to initialize host key verifier: %w", err)
		}
	}

	// Handle "no" mode (insecure)
	if v.Mode == HostKeyCheckNo {
		return nil
	}

	// Normalize hostname (remove port if standard)
	normalizedHost := normalizeHostname(hostname)

	// First check learned keys (in-memory)
	if knownKey, ok := v.learnedKeys[normalizedHost]; ok {
		if !keysEqual(knownKey, key) {
			if v.OnKeyMismatch != nil {
				v.OnKeyMismatch(normalizedHost, knownKey, key)
			}
			return &HostKeyMismatchError{
				Hostname:    normalizedHost,
				ExpectedKey: knownKey,
				ActualKey:   key,
			}
		}
		return nil // Key matches
	}

	// Then check known_hosts file
	if v.knownHosts != nil {
		err := v.knownHosts(hostname, remote, key)
		if err == nil {
			return nil // Key found and matches
		}

		// Check if it's a key mismatch (vs just unknown)
		var keyErr *knownhosts.KeyError
		if isKeyMismatch(err) {
			if v.OnKeyMismatch != nil {
				v.OnKeyMismatch(normalizedHost, nil, key)
			}
			return &HostKeyMismatchError{
				Hostname:  normalizedHost,
				ActualKey: key,
				Wrapped:   err,
			}
		}
		_ = keyErr // Silence unused variable warning
	}

	// Host is unknown - handle based on mode
	switch v.Mode {
	case HostKeyCheckStrict:
		return &UnknownHostError{
			Hostname:    normalizedHost,
			Fingerprint: FingerprintSHA256(key),
			KeyType:     key.Type(),
		}

	case HostKeyCheckTOFU, HostKeyCheckAcceptNew:
		// Learn the key
		v.learnedKeys[normalizedHost] = key

		if v.OnNewKey != nil {
			v.OnNewKey(normalizedHost, key)
		}

		// Persist if configured - best-effort, key is still learned in memory
		if v.StoreLearnedKeys {
			_ = v.persistKey(normalizedHost, key)
		}

		return nil

	default:
		return nil
	}
}

// initializeLocked initializes the verifier (must be called with lock held).
func (v *HostKeyVerifier) initializeLocked() error {
	v.initialized = true

	if v.Mode == HostKeyCheckNo {
		return nil
	}

	// Determine known_hosts paths
	userPath := v.KnownHostsPath
	if userPath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			userPath = filepath.Join(home, ".ssh", "known_hosts")
		}
	}

	systemPath := v.SystemKnownHostsPath
	if systemPath == "" {
		systemPath = "/etc/ssh/ssh_known_hosts"
	}

	// Collect existing known_hosts files
	var paths []string
	if userPath != "" {
		if _, err := os.Stat(userPath); err == nil {
			paths = append(paths, userPath)
		}
	}
	if systemPath != "" {
		if _, err := os.Stat(systemPath); err == nil {
			paths = append(paths, systemPath)
		}
	}

	// Create callback from known_hosts files
	if len(paths) > 0 {
		callback, err := knownhosts.New(paths...)
		if err != nil {
			// Don't fail if we can't read known_hosts - just won't have pre-existing keys
			v.knownHosts = nil
		} else {
			v.knownHosts = callback
		}
	}

	return nil
}

// persistKey writes a host key to the known_hosts file.
func (v *HostKeyVerifier) persistKey(hostname string, key ssh.PublicKey) error {
	path := v.KnownHostsPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		sshDir := filepath.Join(home, ".ssh")
		if err := os.MkdirAll(sshDir, 0o700); err != nil {
			return err
		}
		path = filepath.Join(sshDir, "known_hosts")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// Format the known_hosts line
	line := formatKnownHostsLine(hostname, key)

	// Append to file
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(line + "\n")
	return err
}

// AddKnownHost manually adds a host key to the in-memory store.
func (v *HostKeyVerifier) AddKnownHost(hostname string, key ssh.PublicKey) {
	v.mu.Lock()
	defer v.mu.Unlock()

	normalizedHost := normalizeHostname(hostname)
	v.learnedKeys[normalizedHost] = key
}

// RemoveKnownHost removes a host from the in-memory store.
func (v *HostKeyVerifier) RemoveKnownHost(hostname string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	normalizedHost := normalizeHostname(hostname)
	delete(v.learnedKeys, normalizedHost)
}

// GetKnownHosts returns a copy of the learned hosts.
func (v *HostKeyVerifier) GetKnownHosts() map[string]string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	result := make(map[string]string)
	for host, key := range v.learnedKeys {
		result[host] = FingerprintSHA256(key)
	}
	return result
}

// HostKeyMismatchError is returned when a host key doesn't match the expected value.
type HostKeyMismatchError struct {
	Hostname    string
	ExpectedKey ssh.PublicKey
	ActualKey   ssh.PublicKey
	Wrapped     error
}

func (e *HostKeyMismatchError) Error() string {
	actualFP := FingerprintSHA256(e.ActualKey)
	if e.ExpectedKey != nil {
		expectedFP := FingerprintSHA256(e.ExpectedKey)
		return fmt.Sprintf("host key mismatch for %s: expected %s, got %s (possible MITM attack)",
			e.Hostname, expectedFP, actualFP)
	}
	return fmt.Sprintf("host key mismatch for %s: received %s (possible MITM attack)",
		e.Hostname, actualFP)
}

func (e *HostKeyMismatchError) Unwrap() error {
	return e.Wrapped
}

// UnknownHostError is returned when a host is not in known_hosts and strict mode is enabled.
type UnknownHostError struct {
	Hostname    string
	Fingerprint string
	KeyType     string
}

func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("unknown host %s: key type %s, fingerprint %s (add to known_hosts or use TOFU mode)",
		e.Hostname, e.KeyType, e.Fingerprint)
}

// FingerprintSHA256 returns the SHA256 fingerprint of a public key in the standard format.
func FingerprintSHA256(key ssh.PublicKey) string {
	hash := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(hash[:])
}

// FingerprintMD5 returns the MD5 fingerprint of a public key (legacy format).
func FingerprintMD5(key ssh.PublicKey) string {
	return ssh.FingerprintLegacyMD5(key)
}

// formatKnownHostsLine formats a host key for the known_hosts file.
func formatKnownHostsLine(hostname string, key ssh.PublicKey) string {
	// Remove port if it's the default SSH port
	host := normalizeHostname(hostname)

	// Format: hostname keytype base64-key
	return fmt.Sprintf("%s %s %s",
		host,
		key.Type(),
		base64.StdEncoding.EncodeToString(key.Marshal()))
}

// normalizeHostname removes the default SSH port from a hostname.
func normalizeHostname(hostname string) string {
	// Handle [host]:port format
	if strings.HasPrefix(hostname, "[") {
		if idx := strings.LastIndex(hostname, "]:"); idx != -1 {
			port := hostname[idx+2:]
			if port == "22" {
				return hostname[1:idx]
			}
		}
		return hostname
	}

	// Handle host:port format
	if idx := strings.LastIndex(hostname, ":"); idx != -1 {
		port := hostname[idx+1:]
		if port == "22" {
			return hostname[:idx]
		}
	}
	return hostname
}

// keysEqual checks if two public keys are equal.
func keysEqual(a, b ssh.PublicKey) bool {
	return bytes.Equal(a.Marshal(), b.Marshal())
}

// isKeyMismatch checks if an error indicates a key mismatch (vs unknown host).
func isKeyMismatch(err error) bool {
	if err == nil {
		return false
	}
	// knownhosts.KeyError has a Want field populated when there's a mismatch
	var keyErr *knownhosts.KeyError
	if ok := errorAs(err, &keyErr); ok {
		return len(keyErr.Want) > 0
	}
	return false
}

// errorAs is a helper for errors.As.
func errorAs(err error, target interface{}) bool {
	if err == nil {
		return false
	}
	// Use standard errors.As for proper error unwrapping
	if t, ok := target.(**knownhosts.KeyError); ok {
		return errors.As(err, t)
	}
	return false
}

// ParseKnownHostsFile parses a known_hosts file and returns host-key mappings.
func ParseKnownHostsFile(path string) (map[string][]ssh.PublicKey, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string][]ssh.PublicKey)
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse the line: hostname keytype base64-key [comment]
		// Can also have multiple hostnames: host1,host2 keytype base64-key
		marker, hosts, pubKey, _, _, err := ssh.ParseKnownHosts([]byte(line + "\n"))
		if err != nil {
			continue // Skip malformed lines
		}
		_ = marker // Ignore @cert-authority and @revoked markers for now

		for _, host := range hosts {
			result[host] = append(result[host], pubKey)
		}
	}

	return result, scanner.Err()
}

// DefaultHostKeyVerifier returns a host key verifier with sensible defaults:
// - TOFU mode for ease of use
// - Uses ~/.ssh/known_hosts
// - Persists learned keys
func DefaultHostKeyVerifier() *HostKeyVerifier {
	return NewTOFUVerifier("")
}

// InsecureIgnoreHostKey returns a callback that accepts any host key.
// This is INSECURE and should only be used for testing.
//
// Deprecated: Use NewHostKeyVerifier with HostKeyCheckNo mode instead.
func InsecureIgnoreHostKey() ssh.HostKeyCallback {
	if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
		return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return fmt.Errorf("insecure host key verification blocked; set KSCORE_ALLOW_INSECURE_TLS=1 for development/testing only")
		}
	}
	//nolint:gosec // G106: InsecureIgnoreHostKey is gated by KSCORE_ALLOW_INSECURE_TLS env var for development/testing only
	return ssh.InsecureIgnoreHostKey() // nosemgrep: go.lang.security.audit.crypto.insecure_ssh.avoid-ssh-insecure-ignore-host-key -- gated by KSCORE_ALLOW_INSECURE_TLS
}
