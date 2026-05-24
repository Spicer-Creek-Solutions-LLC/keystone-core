// SPDX-License-Identifier: Apache-2.0

package secrets

import "time"

// RotationPolicy is a v1.0 placeholder for the post-v1.0 rotation
// orchestration work scoped out of epic 10. The shape lives here so
// [SecretBackend] method signatures + the broker's config-loader can
// reference it without dragging a second package boundary across the
// v1.0 → post-v1.0 line, and so the v0.x test suite has something concrete
// to round-trip in serde fixtures.
//
// Per `epics/10-secrets.md` Scope-out + `docs/project/ROADMAP.md`:
//
//   - Strategies (`blue-green` / `rolling` / `canary` / `immediate`),
//     health-check gates, auto-rollback — post-v1.0.
//   - Cron scheduling + Slack / PagerDuty notifications — post-v1.0.
//
// v1.0 honours only [RotationPolicy.Enabled] = false (the default).
// Any non-zero policy at v1.0 surfaces a warning at config load time
// and is otherwise ignored.
type RotationPolicy struct {
	// Enabled toggles rotation. v1.0 only honours false; post-v1.0 wires
	// the strategies.
	Enabled bool `json:"enabled"`

	// Interval is the cadence the post-v1.0 cron scheduler will use. 0 in
	// v1.0; populated from config when the post-v1.0 features land.
	Interval time.Duration `json:"interval,omitempty"`

	// Strategy is free-form at v1.0 — the post-v1.0 work replaces it with
	// a typed enum once the four strategies + their failure modes are
	// pinned down.
	Strategy string `json:"strategy,omitempty"`
}

// DisabledRotationPolicy returns the canonical v1.0 zero policy.
// Constructor exists so call sites read as `secrets.DisabledRotationPolicy()`
// rather than the bare struct literal, which is the same shape post-v1.0
// will swap out from under them.
func DisabledRotationPolicy() RotationPolicy {
	return RotationPolicy{Enabled: false}
}
