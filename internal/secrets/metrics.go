package secrets

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/metrics"
)

// Metrics is the secrets-package emitter for kscore_secrets_access_total.
// Nil-safe.
type Metrics struct {
	access metrics.Counter
}

// NewMetrics registers the secrets metric against r.
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	c, err := r.NewCounter(metrics.DefSecretsAccessTotal)
	if err != nil {
		return nil, fmt.Errorf("secrets: register access metric: %w", err)
	}
	return &Metrics{access: c}, nil
}

// RecordAccess records one broker operation. backend is the resolved
// backend name (e.g. "file", "vault"); op is one of the Action* string
// constants (get_secret, write_secret, ...); allowed=true → result
// "success", allowed=false → "error".
func (m *Metrics) RecordAccess(backend, op string, allowed bool) {
	if m == nil {
		return
	}
	if backend == "" {
		backend = "_unresolved"
	}
	result := "success"
	if !allowed {
		result = "error"
	}
	m.access.With(metrics.Labels{
		"backend": backend,
		"op":      op,
		"result":  result,
	}).Inc()
}
