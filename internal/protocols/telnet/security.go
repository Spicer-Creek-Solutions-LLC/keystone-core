package telnet

import (
	"fmt"
	"log/slog"
	"net"
	"time"
)

// SecurityConfig contains Telnet-specific security controls.
type SecurityConfig struct {
	// AllowedNetworks restricts Telnet connections to these CIDRs.
	// Empty means all addresses are allowed.
	AllowedNetworks []string `json:"allowed_networks,omitempty"`

	// RequireAudit requires all commands to be audit-logged.
	RequireAudit bool `json:"require_audit,omitempty"`

	// DeprecationWarning logs a warning on every Telnet connection.
	DeprecationWarning bool `json:"deprecation_warning,omitempty"`

	// MaxSessionDuration is the maximum time a Telnet session can be active.
	// Zero means no limit.
	MaxSessionDuration time.Duration `json:"max_session_duration,omitempty"`
}

// SecurityEnforcer enforces Telnet security policies.
type SecurityEnforcer struct {
	config         *SecurityConfig
	logger         *slog.Logger
	parsedNetworks []*net.IPNet
}

// NewSecurityEnforcer creates a security enforcer from the given configuration.
func NewSecurityEnforcer(config *SecurityConfig, logger *slog.Logger) (*SecurityEnforcer, error) {
	if config == nil {
		config = &SecurityConfig{DeprecationWarning: true}
	}
	if logger == nil {
		logger = slog.Default()
	}

	enforcer := &SecurityEnforcer{
		config: config,
		logger: logger,
	}

	for _, cidr := range config.AllowedNetworks {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		enforcer.parsedNetworks = append(enforcer.parsedNetworks, network)
	}

	return enforcer, nil
}

// CheckConnection validates a connection attempt against the allowlist.
func (s *SecurityEnforcer) CheckConnection(addr string) error {
	if len(s.parsedNetworks) == 0 {
		return nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Try resolving hostname
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return fmt.Errorf("telnet: cannot resolve address %q", host)
		}
		ip = ips[0]
	}

	for _, network := range s.parsedNetworks {
		if network.Contains(ip) {
			return nil
		}
	}

	return fmt.Errorf("telnet: address %s not in allowed networks", ip)
}

// EmitDeprecationWarning logs a deprecation warning if configured.
func (s *SecurityEnforcer) EmitDeprecationWarning(deviceID, addr string) {
	if !s.config.DeprecationWarning {
		return
	}
	s.logger.Warn("telnet connection uses unencrypted protocol; consider migrating to SSH",
		"device_id", deviceID,
		"address", addr,
		"protocol", "telnet",
	)
}

// AuditCommand logs a command execution for audit trail.
func (s *SecurityEnforcer) AuditCommand(deviceID, command string, duration time.Duration, err error) {
	if !s.config.RequireAudit {
		return
	}
	attrs := []any{
		"device_id", deviceID,
		"command", command,
		"duration", duration,
		"protocol", "telnet",
	}
	if err != nil {
		attrs = append(attrs, "error", err.Error())
		s.logger.Error("telnet command failed", attrs...)
	} else {
		s.logger.Info("telnet command executed", attrs...)
	}
}

// IsSessionExpired returns true if the session has exceeded MaxSessionDuration.
func (s *SecurityEnforcer) IsSessionExpired(sessionStart time.Time) bool {
	if s.config.MaxSessionDuration <= 0 {
		return false
	}
	return time.Since(sessionStart) > s.config.MaxSessionDuration
}
