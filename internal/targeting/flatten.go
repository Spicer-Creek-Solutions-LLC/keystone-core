package targeting

import (
	"go.keystone-core.io/keystone-core/internal/state"
)

// Flatten projects an AgentRecord into the env shape Compile declares.
//
// Status normalization: PROJECT-DETAILS §4.7 documents the user-facing
// form `status:online`, but the registry uses AgentStatusConnected
// internally. Flatten maps connected → "online" and passes the rest
// (stale, pending, disabled) through so operators can target those
// states by name.
//
// IPs stay as a slice; matchValue tests each element so `ip:10.0.0.0/8`
// (CIDR) and `ip:10.0.1.5` (literal) both behave as users expect on a
// multi-homed agent. Labels are passed through; an unset key surfaces
// as the zero string at evaluation, which never matches a non-empty
// pattern.
func Flatten(rec state.AgentRecord) map[string]any {
	ips := rec.IPAddresses
	if ips == nil {
		ips = []string{}
	}
	labels := rec.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return map[string]any{
		"id":       rec.ID,
		"hostname": rec.Hostname,
		"os":       rec.OS,
		"arch":     rec.Architecture,
		"status":   normalizeStatus(rec.Status),
		"ip":       ips,
		"labels":   labels,
	}
}

// normalizeStatus converts the registry's internal status to the
// user-facing form documented in PROJECT-DETAILS §4.7.
func normalizeStatus(s state.AgentStatus) string {
	if s == state.AgentStatusConnected {
		return "online"
	}
	return string(s)
}
