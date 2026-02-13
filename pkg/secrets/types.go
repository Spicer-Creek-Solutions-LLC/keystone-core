// Package secrets provides a public client for the Keystone Core secrets API.
//
// This package is UNSTABLE — the API may change between minor versions.
package secrets

import "time"

// Secret represents a secret retrieved from the secrets API.
type Secret struct {
	Path      string
	Backend   string
	Type      string
	Data      map[string]interface{}
	Metadata  map[string]string
	Version   int
	CreatedAt time.Time
	ExpiresAt time.Time
	Renewable bool
	LeaseID   string
}

// Get returns a value from the secret data by key.
func (s *Secret) Get(key string) (interface{}, bool) {
	if s.Data == nil {
		return nil, false
	}
	v, ok := s.Data[key]
	return v, ok
}

// GetString returns a string value from the secret data by key.
func (s *Secret) GetString(key string) (string, bool) {
	v, ok := s.Get(key)
	if !ok {
		return "", false
	}
	str, ok := v.(string)
	return str, ok
}

// IsExpired returns true if the secret has expired.
func (s *Secret) IsExpired() bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(s.ExpiresAt)
}

// Lease represents a secret lease.
type Lease struct {
	ID           string
	SecretPath   string
	Backend      string
	State        string
	TTL          time.Duration
	MaxTTL       time.Duration
	IssuedAt     time.Time
	ExpiresAt    time.Time
	LastRenewal  time.Time
	RenewalCount int
	Renewable    bool
	Revocable    bool
	Metadata     map[string]string
}

// IsExpired returns true if the lease has expired.
func (l *Lease) IsExpired() bool {
	if l.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(l.ExpiresAt)
}

// TimeRemaining returns the time remaining until the lease expires.
func (l *Lease) TimeRemaining() time.Duration {
	if l.ExpiresAt.IsZero() {
		return 0
	}
	remaining := time.Until(l.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ListSecretsResult contains the result of a ListSecrets call.
type ListSecretsResult struct {
	Keys          []string
	NextPageToken string
}

// ListLeasesResult contains the result of a ListLeases call.
type ListLeasesResult struct {
	Leases        []*Lease
	NextPageToken string
	TotalCount    int
}

// EncryptResult contains the result of an Encrypt call.
type EncryptResult struct {
	Ciphertext string
	KeyVersion int
}

// DecryptResult contains the result of a Decrypt call.
type DecryptResult struct {
	Plaintext  []byte
	KeyVersion int
}

// SignResult contains the result of a Sign call.
type SignResult struct {
	Signature  string
	KeyVersion int
}
