package bootstrap

import (
	"context"
	"errors"
	"fmt"
)

// Mode is the bootstrap profile. demo runs without systemd /
// package-repo / certs (single-host trial). production layers
// systemd (Task 9). enterprise additionally layers cert generation
// (Identity epic).
type Mode string

const (
	ModeDemo       Mode = "demo"
	ModeProduction Mode = "production"
	ModeEnterprise Mode = "enterprise"
)

// Configuration is the operator-supplied + detection-augmented
// bootstrap input. Persisted as part of State after the Configure
// phase succeeds; subsequent phases (Validate, Install, Verify)
// read it.
type Configuration struct {
	Mode          Mode   `json:"mode"`
	ClusterName   string `json:"cluster_name"`
	AgentID       string `json:"agent_id"`
	NodeRole      string `json:"node_role,omitempty"`
	JoinURL       string `json:"join_url,omitempty"`
	JoinToken     string `json:"join_token,omitempty"` //nolint:gosec // PSK string field — same false-positive G117 pattern as elsewhere
	ConfigPath    string `json:"config_path"`
	GenerateCerts bool   `json:"generate_certs,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`
}

// Validate enforces the v1.0 minimum-viable Configuration shape.
// Validator (the phase) repeats some of these checks alongside
// reachability probes; this is the cheap front-loaded pass that
// runs before Validate spends time dialing.
//
// Validate accepts all three modes — production / enterprise are
// rejected separately by ValidateForV10 so the v1.x deferral
// message can be surfaced cleanly by the TUI + CLI before the
// Engine spends time on Validate / Install.
func (c Configuration) Validate() error {
	switch c.Mode {
	case ModeDemo, ModeProduction, ModeEnterprise:
	case "":
		return errors.New("mode: must not be empty (one of demo|production|enterprise)")
	default:
		return fmt.Errorf("mode: %q (must be demo, production, or enterprise)", string(c.Mode))
	}
	if c.ClusterName == "" {
		return errors.New("cluster_name: must not be empty")
	}
	if c.AgentID == "" {
		return errors.New("agent_id: must not be empty")
	}
	if c.ConfigPath == "" {
		return errors.New("config_path: must not be empty")
	}
	return nil
}

// ValidateForV10 is the runtime gate every Configurer (TUI + CLI)
// runs before handing a Configuration to the Engine. It rejects
// production / enterprise modes with a v1.x deferral message —
// production needs TLS cert collection (Epic 11) and enterprise
// needs blueprint selection (Epic 14 + 17). Tracked in
// docs/project/V1X-BACKLOG.md.
//
// Drop this function (or change it to a no-op) when those epics
// land and the deferral lifts.
func ValidateForV10(c *Configuration) error {
	if c == nil {
		return errors.New("bootstrap: configuration is nil")
	}
	if c.Mode != ModeDemo {
		return fmt.Errorf("bootstrap: %s mode is reserved for v1.x (see docs/project/V1X-BACKLOG.md); use demo mode for v1.0", string(c.Mode))
	}
	return nil
}

// Configurer produces a Configuration. Task 7 ships TUIConfigurer
// (Bubble Tea screens); Task 8 ships FlagConfigurer (CLI flags).
// Task 6 ships StaticConfigurer for tests + as the building block
// Task 8 wraps.
type Configurer interface {
	Configure(ctx context.Context, detected *DetectionResult) (*Configuration, error)
}

// StaticConfigurer returns a pre-set Configuration verbatim. Used
// by tests and (eventually) by Task 8's flag-driven driver after
// it parses CLI args.
type StaticConfigurer struct {
	Config *Configuration
}

func (s StaticConfigurer) Configure(_ context.Context, _ *DetectionResult) (*Configuration, error) {
	if s.Config == nil {
		return nil, errors.New("bootstrap: StaticConfigurer.Config is nil")
	}
	out := *s.Config
	return &out, nil
}
