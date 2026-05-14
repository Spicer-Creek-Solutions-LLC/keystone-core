package auth

import (
	"fmt"
	"log/slog"
	"strings"
)

// DefaultSPIFFERoleResolver implements the §4.10 SPIFFE-path →
// Role mapping for v0.1:
//
//	server/control-plane → RoleAdmin
//	agent/<id>           → RoleOperator
//	service/<name>       → RoleOperator
//	any other path       → RoleReadonly  (+ logged at WARN)
//
// Empty path → error (caller — the MTLSAuthenticator — already
// guards against this, but we double-check).
//
// logger may be nil; the WARN line is suppressed in that case.
func DefaultSPIFFERoleResolver(logger *slog.Logger) RoleResolver {
	return func(path string) (Role, error) {
		if path == "" {
			return RoleNone, fmt.Errorf("empty SPIFFE path")
		}
		switch {
		case path == "server/control-plane":
			return RoleAdmin, nil
		case strings.HasPrefix(path, "agent/"):
			rest := strings.TrimPrefix(path, "agent/")
			if rest == "" {
				return RoleNone, fmt.Errorf("agent path missing identifier: %q", path)
			}
			return RoleOperator, nil
		case strings.HasPrefix(path, "service/"):
			rest := strings.TrimPrefix(path, "service/")
			if rest == "" {
				return RoleNone, fmt.Errorf("service path missing identifier: %q", path)
			}
			return RoleOperator, nil
		case strings.HasPrefix(path, "server/"):
			// server/<other> — operators may eventually run more
			// than one server identity; for now resolve to admin
			// so the v0.1 ergonomics don't break when a future
			// task adds e.g. server/api-gateway.
			return RoleAdmin, nil
		}
		// Unrecognized path — least-privilege fallback. Log at
		// WARN so operators see the unexpected ID without
		// auth failing in production.
		if logger != nil {
			logger.Warn("spiffe: unrecognized path; defaulting to readonly",
				"path", path)
		}
		return RoleReadonly, nil
	}
}
