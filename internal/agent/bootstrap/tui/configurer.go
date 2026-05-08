package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/charmbracelet/huh"

	"go.keystone-core.io/keystone-core/internal/agent/bootstrap"
)

// Configurer drives the bootstrap wizard. It satisfies
// bootstrap.Configurer — the engine calls Configure, the wizard
// runs synchronously, and a populated *bootstrap.Configuration
// comes back.
//
// Defaults seed the form so the user can ⏎ through the demo flow
// in seconds. Defaults come from cfg.Agent + sensible fallbacks
// (hostname for AgentID, /etc/keystone-core/keystone-core-agent.yaml
// for ConfigPath); operators override per-field as needed.
type Configurer struct {
	defaults Defaults
	log      *slog.Logger
}

// Defaults seeds the wizard's prefilled values. Pulled from
// cfg.Agent + cfg.NATS by the binary wiring layer; missing
// fields fall back to the constants below.
type Defaults struct {
	ClusterName string
	AgentID     string
	NodeRole    string
	JoinURL     string
	ConfigPath  string
}

const (
	defaultNodeRole   = "worker"
	defaultConfigPath = "/etc/keystone-core/keystone-core-agent.yaml"
	defaultJoinURL    = "nats://localhost:4222"
)

// NewConfigurer constructs a wizard. Logger is required so the
// wizard logs when production / enterprise modes abort with the
// "v1.0 supports demo only" notice.
func NewConfigurer(d Defaults, log *slog.Logger) *Configurer {
	if log == nil {
		log = slog.Default()
	}
	if d.NodeRole == "" {
		d.NodeRole = defaultNodeRole
	}
	if d.ConfigPath == "" {
		d.ConfigPath = defaultConfigPath
	}
	if d.JoinURL == "" {
		d.JoinURL = defaultJoinURL
	}
	if d.AgentID == "" {
		if hn, err := os.Hostname(); err == nil {
			d.AgentID = hn
		}
	}
	return &Configurer{defaults: d, log: log}
}

// Configure runs the wizard. Returns the user-confirmed
// Configuration on success, or an error on cancellation /
// non-demo mode selection.
//
// Production and enterprise modes are reserved for future
// versions — they need TLS cert collection (Epic 11) and
// blueprint selection (Epic 14 + 17). The wizard surfaces the
// modes as visibly-disabled options with a description pointing
// at the deferral, and aborts cleanly if the operator picks one.
// Tracked in docs/project/V1X-BACKLOG.md.
func (c *Configurer) Configure(ctx context.Context, detection *bootstrap.DetectionResult) (*bootstrap.Configuration, error) {
	values := c.seedValues(detection)
	form := c.buildForm(detection, &values)

	if err := form.RunWithContext(ctx); err != nil {
		return nil, fmt.Errorf("tui: %w", err)
	}

	cfg, err := configurationFromValues(values)
	if err != nil {
		c.log.WarnContext(ctx, "bootstrap: tui form rejected",
			"selected_mode", values.Mode, "err", err.Error())
		return nil, err
	}
	return cfg, nil
}

// configurationFromValues runs the post-form gate: blocks
// non-demo modes (deferred to v1.x) and validates the
// Configuration. Lifted out of Configure so unit tests can
// exercise the gate without driving an interactive form.
func configurationFromValues(v formValues) (*bootstrap.Configuration, error) {
	mode := bootstrap.Mode(v.Mode)
	if mode != bootstrap.ModeDemo {
		return nil, fmt.Errorf("bootstrap: %s mode is reserved for v1.x (see docs/project/V1X-BACKLOG.md); use demo mode for v1.0", v.Mode)
	}
	cfg := &bootstrap.Configuration{
		Mode:        mode,
		ClusterName: v.ClusterName,
		AgentID:     v.AgentID,
		NodeRole:    v.NodeRole,
		JoinURL:     v.JoinURL,
		JoinToken:   v.JoinToken,
		ConfigPath:  v.ConfigPath,
		DryRun:      v.DryRun,
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("tui: configuration invalid: %w", err)
	}
	return cfg, nil
}

// formValues holds the wizard's bound state. huh.Field.Value
// takes a *string / *bool; binding directly into Configuration
// would tie field shape to wire format, so we keep an internal
// scratchpad and copy across after the form runs.
type formValues struct {
	Mode        string
	ClusterName string
	AgentID     string
	NodeRole    string
	JoinURL     string
	JoinToken   string
	ConfigPath  string
	DryRun      bool
}

func (c *Configurer) seedValues(detection *bootstrap.DetectionResult) formValues {
	v := formValues{
		Mode:        string(bootstrap.ModeDemo),
		ClusterName: c.defaults.ClusterName,
		AgentID:     c.defaults.AgentID,
		NodeRole:    c.defaults.NodeRole,
		JoinURL:     c.defaults.JoinURL,
		ConfigPath:  c.defaults.ConfigPath,
	}
	// Detection currently only feeds the read-only banner on
	// screen 1; nothing in detection narrows the value defaults
	// in v1.0. Reserved here so future tasks (e.g. picking the
	// init system to derive systemd path) can plug in.
	_ = detection
	return v
}

// buildForm assembles the wizard. Six groups, advanced linearly:
// (1) intro + mode, (2) cluster identity, (3) node role,
// (4) NATS join, (5) config path, (6) confirm.
//
// Validators bind to each Input so typos surface inline. The
// confirm group is a yes/no — operator backs out via Esc.
func (c *Configurer) buildForm(detection *bootstrap.DetectionResult, v *formValues) *huh.Form {
	intro := buildIntroNote(detection)
	modeSelect := huh.NewSelect[string]().
		Title("Mode").
		Description("Demo is the only mode available in v1.0. Production + enterprise are deferred — see docs/project/V1X-BACKLOG.md.").
		Options(
			huh.NewOption("demo (recommended for v1.0)", string(bootstrap.ModeDemo)),
			huh.NewOption("production [v1.x — needs TLS cert collection]", string(bootstrap.ModeProduction)),
			huh.NewOption("enterprise [v1.x — needs blueprint selection]", string(bootstrap.ModeEnterprise)),
		).
		Value(&v.Mode)

	clusterName := huh.NewInput().
		Title("Cluster name").
		Description("Logical cluster this agent joins. Operators usually mirror their environment (e.g. prod-east, staging).").
		Placeholder("default").
		Validate(validateClusterName).
		Value(&v.ClusterName)
	agentID := huh.NewInput().
		Title("Agent ID").
		Description("Unique within the cluster. Hostname is a fine default.").
		Placeholder(v.AgentID).
		Validate(validateAgentID).
		Value(&v.AgentID)

	nodeRole := huh.NewInput().
		Title("Node role").
		Description("Free-text label for targeting (e.g. worker, db, ingress).").
		Placeholder(defaultNodeRole).
		Value(&v.NodeRole)

	joinURL := huh.NewInput().
		Title("NATS join URL").
		Description("Control plane NATS endpoint. Brackets required for IPv6 (nats://[::1]:4222).").
		Placeholder(defaultJoinURL).
		Validate(validateJoinURL).
		Value(&v.JoinURL)
	joinToken := huh.NewInput().
		Title("NATS join token").
		Description("Optional bootstrap PSK / token. Leave blank if your control plane allows anonymous bootstrap.").
		EchoMode(huh.EchoModePassword).
		Value(&v.JoinToken)

	configPath := huh.NewInput().
		Title("Config path").
		Description("Where the rendered keystone-core-agent.yaml lands. Must be absolute.").
		Placeholder(defaultConfigPath).
		Validate(validateConfigPath).
		Value(&v.ConfigPath)
	dryRun := huh.NewConfirm().
		Title("Dry run?").
		Description("If yes, the installer reports what it would do but writes nothing.").
		Value(&v.DryRun)

	confirm := huh.NewConfirm().
		Title("Apply this configuration?").
		Description("Press y to continue with Validate → Install → Verify, or n to abort.").
		Affirmative("Yes — proceed").
		Negative("No — abort").
		Validate(func(ok bool) error {
			if !ok {
				return errors.New("aborted by operator")
			}
			return nil
		}).
		Value(new(bool))

	return huh.NewForm(
		huh.NewGroup(intro, modeSelect).Title("Welcome"),
		huh.NewGroup(clusterName, agentID).Title("Cluster identity"),
		huh.NewGroup(nodeRole).Title("Node role"),
		huh.NewGroup(joinURL, joinToken).Title("NATS join"),
		huh.NewGroup(configPath, dryRun).Title("Config path"),
		huh.NewGroup(confirm).Title("Confirm"),
	)
}

// buildIntroNote renders the read-only "we detected this much
// about your host" banner on screen 1. Helps operators verify
// the agent is about to bootstrap on the host they expect.
func buildIntroNote(detection *bootstrap.DetectionResult) *huh.Note {
	desc := "Detection results unavailable."
	if detection != nil {
		desc = fmt.Sprintf(
			"OS: %s   Distro: %s %s\nKernel: %s   Arch: %s\nInit: %s   Package mgr: %s",
			fallback(detection.OS, "unknown"),
			fallback(detection.Distro, "unknown"),
			fallback(detection.DistroVersion, ""),
			fallback(detection.KernelVersion, "unknown"),
			fallback(detection.Architecture, "unknown"),
			fallback(detection.InitSystem, "unknown"),
			fallback(detection.PackageManager, "unknown"),
		)
	}
	return huh.NewNote().
		Title("Keystone Core agent bootstrap").
		Description(desc)
}

func fallback(s, fb string) string {
	if s == "" {
		return fb
	}
	return s
}
