package agent

import "errors"

// Errors returned by the secret agent client.
var (
	// ErrSecretNotFound is returned when a secret does not exist.
	ErrSecretNotFound = errors.New("secret not found")

	// ErrNotConnected is returned when the client is not connected.
	ErrNotConnected = errors.New("client not connected")

	// ErrCacheMiss is returned when a secret is not in the cache.
	ErrCacheMiss = errors.New("cache miss")

	// ErrCacheExpired is returned when a cached secret has expired.
	ErrCacheExpired = errors.New("cached secret expired")

	// ErrLeaseExpired is returned when a secret's lease has expired.
	ErrLeaseExpired = errors.New("lease expired")

	// ErrAuthenticationFailed is returned when SPIFFE authentication fails.
	ErrAuthenticationFailed = errors.New("authentication failed")

	// ErrBrokerUnavailable is returned when the broker is not reachable.
	ErrBrokerUnavailable = errors.New("broker unavailable")
)
