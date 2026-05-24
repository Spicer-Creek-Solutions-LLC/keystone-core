// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"strings"
)

// MetricsConfig configures the Prometheus exposition surface.
// PROJECT-DETAILS §4.16: /metrics is on the main HTTP server,
// unauthenticated, no rate-limit — same posture as /health/*.
type MetricsConfig struct {
	// Enabled toggles registration of the /metrics handler. Default
	// true (epic line 14: "default true"); set false in deployments
	// where the operator routes scrape traffic differently.
	Enabled bool `koanf:"enabled"`

	// Path is the URL path the handler mounts on. Default "/metrics".
	// Operators who reverse-proxy under a non-standard prefix override
	// this so the inner mount matches the proxy's rewrite.
	Path string `koanf:"path"`
}

// Validate rejects empty or non-rooted paths when Enabled.
func (m MetricsConfig) Validate() error {
	if !m.Enabled {
		return nil
	}
	if m.Path == "" {
		return fmt.Errorf("metrics.path: must not be empty when enabled")
	}
	if !strings.HasPrefix(m.Path, "/") {
		return fmt.Errorf("metrics.path: must start with %q, got %q", "/", m.Path)
	}
	return nil
}
