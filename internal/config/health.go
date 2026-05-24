// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"
)

// HealthConfig configures the kscore-server liveness/readiness
// surface. PROJECT-DETAILS §4.4: /health/ready respects a startup
// grace period to avoid false-not-ready signals during boot.
type HealthConfig struct {
	StartupGracePeriod time.Duration `koanf:"startupgraceperiod"`
	CheckTimeout       time.Duration `koanf:"checktimeout"`
}

// Validate rejects negative durations. Zero is allowed and gets
// defaulted at construction time (see defaultConfig).
func (h HealthConfig) Validate() error {
	if h.StartupGracePeriod < 0 {
		return fmt.Errorf("startupgraceperiod: must not be negative, got %s", h.StartupGracePeriod)
	}
	if h.CheckTimeout < 0 {
		return fmt.Errorf("checktimeout: must not be negative, got %s", h.CheckTimeout)
	}
	return nil
}
