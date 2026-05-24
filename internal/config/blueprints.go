// SPDX-License-Identifier: Apache-2.0

package config

// BlueprintsConfig wires the optional v1.0 BlueprintService.
//
// Epic 19 task 2b — until the gate-v1.0 ROADMAP item "Remote /
// distributed blueprint apply wiring" lands, the server-side
// BlueprintService applies blueprints against the server's local
// stdlib StateRunner (the same convergence path kscore-blueprint
// uses today). CatalogPath enables ListBlueprints / GetBlueprint /
// ApplyBlueprint over gRPC; empty disables them (clients reach
// Unavailable).
type BlueprintsConfig struct {
	CatalogPath string `koanf:"catalogpath"`
}

// Validate is a no-op for v1.0 — empty CatalogPath disables the
// service, any non-empty value is delegated to filesystem-existence
// checks at boot time.
func (BlueprintsConfig) Validate() error { return nil }
