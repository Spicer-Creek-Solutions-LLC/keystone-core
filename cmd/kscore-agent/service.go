package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/agent/systemd"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/logging"
)

// serviceFlags holds the parsed `service install` flag set.
// Flag and KSCORE_SERVICE_* env-var pair per field; flag wins.
// Defaults are populated by buildInstallOptions / Render's
// fillDefaults — none are required.
type serviceFlags struct {
	Binary          string
	ConfigPath      string
	UnitDir         string
	UnitName        string
	User            string
	Group           string
	EnvironmentFile string
	ExtraEnv        []string
	Enable          bool
	Start           bool
	DryRun          bool
}

// runnerFactory exists so tests inject a FakeRunner without
// shelling out to systemctl. Production binary uses the real
// one; service_test.go overrides this var.
var runnerFactory = func() systemd.Runner { return systemd.NewDefaultRunner() }

// newServiceCommand assembles `kscore-agent service` with its
// install / uninstall / status children. The subcommand is
// Linux-only; non-Linux invocations short-circuit with a clear
// "v1.x" message to the operator.
//
// PROJECT-DETAILS §4.6 lists `service start|stop|status` too;
// v1.0 ships only `install|uninstall|status` — start/stop add
// no value over native `systemctl start/stop`. Tracked in
// docs/project/ROADMAP.md.
func newServiceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "service",
		Short:         "Manage the keystone-core-agent systemd unit",
		Long:          "Linux-only: write/remove the keystone-core-agent.service unit and report its systemd status. v1.0 ships install / uninstall / status; start and stop are deferred (use systemctl directly).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newServiceInstallCommand())
	cmd.AddCommand(newServiceUninstallCommand())
	cmd.AddCommand(newServiceStatusCommand())
	return cmd
}

func newServiceInstallCommand() *cobra.Command {
	flags := &serviceFlags{}
	cmd := &cobra.Command{
		Use:           "install",
		Short:         "Render + install the keystone-core-agent systemd unit",
		Long:          "Renders /etc/systemd/system/keystone-core-agent.service, runs systemctl daemon-reload, optionally enables and starts the service. Idempotent — same content on a re-run is a no-op.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServiceInstall(cmd, flags)
		},
	}
	cmd.Flags().StringVar(&flags.Binary, "binary", "",
		"absolute path to the kscore-agent binary (KSCORE_SERVICE_BINARY; default = /usr/local/bin/kscore-agent)")
	cmd.Flags().StringVar(&flags.ConfigPath, "config-path", "",
		"absolute path to the agent config (KSCORE_SERVICE_CONFIG_PATH; default = /etc/keystone-core/keystone-core-agent.yaml)")
	cmd.Flags().StringVar(&flags.UnitDir, "unit-dir", "",
		"directory the unit file is written to (KSCORE_SERVICE_UNIT_DIR; default = /etc/systemd/system)")
	cmd.Flags().StringVar(&flags.UnitName, "unit-name", "",
		"unit filename (KSCORE_SERVICE_UNIT_NAME; default = keystone-core-agent.service)")
	cmd.Flags().StringVar(&flags.User, "user", "",
		"unit User= (KSCORE_SERVICE_USER; default = root)")
	cmd.Flags().StringVar(&flags.Group, "group", "",
		"unit Group= (KSCORE_SERVICE_GROUP; required when --user is set)")
	cmd.Flags().StringVar(&flags.EnvironmentFile, "environment-file", "",
		"absolute path to a key=value file systemd reads before launch (KSCORE_SERVICE_ENVIRONMENT_FILE)")
	cmd.Flags().StringSliceVar(&flags.ExtraEnv, "env", nil,
		"additional Environment= entries (KEY=VALUE; repeatable)")
	cmd.Flags().BoolVar(&flags.Enable, "enable", false,
		"systemctl enable the unit after installing (KSCORE_SERVICE_ENABLE)")
	cmd.Flags().BoolVar(&flags.Start, "start", false,
		"systemctl start the unit after installing (KSCORE_SERVICE_START)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false,
		"render + report without writing or invoking systemctl (KSCORE_SERVICE_DRY_RUN)")
	return cmd
}

func newServiceUninstallCommand() *cobra.Command {
	flags := &serviceFlags{}
	cmd := &cobra.Command{
		Use:           "uninstall",
		Short:         "Stop, disable, and remove the keystone-core-agent systemd unit",
		Long:          "Reverses install: systemctl stop + disable, removes the unit file, runs daemon-reload. Idempotent — no-op if no unit is installed.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServiceUninstall(cmd, flags)
		},
	}
	cmd.Flags().StringVar(&flags.UnitDir, "unit-dir", "",
		"directory containing the unit file (KSCORE_SERVICE_UNIT_DIR; default = /etc/systemd/system)")
	cmd.Flags().StringVar(&flags.UnitName, "unit-name", "",
		"unit filename (KSCORE_SERVICE_UNIT_NAME; default = keystone-core-agent.service)")
	cmd.Flags().BoolVar(&flags.DryRun, "dry-run", false,
		"report what would happen without invoking systemctl (KSCORE_SERVICE_DRY_RUN)")
	return cmd
}

func newServiceStatusCommand() *cobra.Command {
	flags := &serviceFlags{}
	cmd := &cobra.Command{
		Use:           "status",
		Short:         "Report whether the unit is installed, active, and enabled",
		Long:          "Wraps `systemctl is-active` + `is-enabled`. Returns nonzero exit when the unit is missing, inactive, or disabled — useful for scripting.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServiceStatus(cmd, flags)
		},
	}
	cmd.Flags().StringVar(&flags.UnitDir, "unit-dir", "",
		"directory containing the unit file (KSCORE_SERVICE_UNIT_DIR; default = /etc/systemd/system)")
	cmd.Flags().StringVar(&flags.UnitName, "unit-name", "",
		"unit filename (KSCORE_SERVICE_UNIT_NAME; default = keystone-core-agent.service)")
	return cmd
}

// applyServiceEnvFallback fills in flag values from KSCORE_SERVICE_*
// env vars when the flag wasn't passed explicitly. Same pattern
// as the bootstrap subcommand's applyEnvFallback.
func applyServiceEnvFallback(cmd *cobra.Command, f *serviceFlags) error {
	envForString := func(flagName, envName string, target *string) {
		if cmd.Flags().Lookup(flagName) == nil || cmd.Flags().Lookup(flagName).Changed {
			return
		}
		if v, ok := os.LookupEnv(envName); ok {
			*target = v
		}
	}
	envForBool := func(flagName, envName string, target *bool) error {
		if cmd.Flags().Lookup(flagName) == nil || cmd.Flags().Lookup(flagName).Changed {
			return nil
		}
		if v, ok := os.LookupEnv(envName); ok {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("%s: %w", envName, err)
			}
			*target = b
		}
		return nil
	}
	envForString("binary", "KSCORE_SERVICE_BINARY", &f.Binary)
	envForString("config-path", "KSCORE_SERVICE_CONFIG_PATH", &f.ConfigPath)
	envForString("unit-dir", "KSCORE_SERVICE_UNIT_DIR", &f.UnitDir)
	envForString("unit-name", "KSCORE_SERVICE_UNIT_NAME", &f.UnitName)
	envForString("user", "KSCORE_SERVICE_USER", &f.User)
	envForString("group", "KSCORE_SERVICE_GROUP", &f.Group)
	envForString("environment-file", "KSCORE_SERVICE_ENVIRONMENT_FILE", &f.EnvironmentFile)
	if err := envForBool("enable", "KSCORE_SERVICE_ENABLE", &f.Enable); err != nil {
		return err
	}
	if err := envForBool("start", "KSCORE_SERVICE_START", &f.Start); err != nil {
		return err
	}
	return envForBool("dry-run", "KSCORE_SERVICE_DRY_RUN", &f.DryRun)
}

// linuxOnlyGuard short-circuits all service subcommands on
// non-Linux platforms. Same pattern as
// internal/agent/exec_user_windows.go.
func linuxOnlyGuard() error {
	if runtime.GOOS != "linux" {
		return errors.New("service subcommands are Linux-only in v1.0 (Windows agent v1.1, macOS v1.2)")
	}
	return nil
}

func runServiceInstall(cmd *cobra.Command, f *serviceFlags) error {
	if err := linuxOnlyGuard(); err != nil {
		return err
	}
	if err := applyServiceEnvFallback(cmd, f); err != nil {
		return err
	}
	log, ctx, cancel, err := setupServiceLifecycle(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	params := systemd.Params{
		BinaryPath:      f.Binary,
		ConfigPath:      f.ConfigPath,
		User:            f.User,
		Group:           f.Group,
		ExtraEnv:        f.ExtraEnv,
		EnvironmentFile: f.EnvironmentFile,
	}
	opts := systemd.Options{
		UnitDir:  f.UnitDir,
		UnitName: f.UnitName,
		Runner:   runnerFactory(),
		Logger:   log,
		Enable:   f.Enable,
		Start:    f.Start,
		DryRun:   f.DryRun,
	}
	res, err := systemd.Install(ctx, params, opts)
	if err != nil {
		return fmt.Errorf("service install: %w", err)
	}
	log.InfoContext(ctx, "service install complete",
		"unit_path", res.UnitPath,
		"created", res.Created,
		"updated", res.Updated,
		"enabled", res.Enabled,
		"started", res.Started,
		"dry_run", f.DryRun,
	)
	return nil
}

func runServiceUninstall(cmd *cobra.Command, f *serviceFlags) error {
	if err := linuxOnlyGuard(); err != nil {
		return err
	}
	if err := applyServiceEnvFallback(cmd, f); err != nil {
		return err
	}
	log, ctx, cancel, err := setupServiceLifecycle(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	opts := systemd.Options{
		UnitDir:  f.UnitDir,
		UnitName: f.UnitName,
		Runner:   runnerFactory(),
		Logger:   log,
		DryRun:   f.DryRun,
	}
	res, err := systemd.Uninstall(ctx, opts)
	if err != nil {
		return fmt.Errorf("service uninstall: %w", err)
	}
	log.InfoContext(ctx, "service uninstall complete",
		"unit_path", res.UnitPath,
		"no_unit", res.NoUnit,
		"removed", res.Removed,
	)
	return nil
}

func runServiceStatus(cmd *cobra.Command, f *serviceFlags) error {
	if err := linuxOnlyGuard(); err != nil {
		return err
	}
	if err := applyServiceEnvFallback(cmd, f); err != nil {
		return err
	}
	log, ctx, cancel, err := setupServiceLifecycle(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	opts := systemd.Options{
		UnitDir:  f.UnitDir,
		UnitName: f.UnitName,
		Runner:   runnerFactory(),
		Logger:   log,
	}
	res, err := systemd.Status(ctx, opts)
	if err != nil {
		return fmt.Errorf("service status: %w", err)
	}
	// Status output goes to stdout for scripting; nonzero exit
	// when not active / not enabled (script-friendly contract).
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "unit:    %s\n", res.UnitName)
	fmt.Fprintf(out, "present: %t\n", res.UnitPresent)
	fmt.Fprintf(out, "active:  %s (%t)\n", res.ActiveState, res.Active)
	fmt.Fprintf(out, "enabled: %s (%t)\n", res.EnabledState, res.Enabled)

	if !res.UnitPresent {
		return errors.New("unit not installed")
	}
	if !res.Active {
		return fmt.Errorf("unit not active (state=%s)", res.ActiveState)
	}
	if !res.Enabled {
		return fmt.Errorf("unit not enabled (state=%s)", res.EnabledState)
	}
	return nil
}

// setupServiceLifecycle loads the daemon config (for logging
// settings only — the service subcommand doesn't need NATS) and
// builds a signal-cancelable context. Mirrors the bootstrap
// subcommand's setup.
func setupServiceLifecycle(cmd *cobra.Command) (*slog.Logger, context.Context, func(), error) {
	cfgPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("flag: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("config: %w", err)
	}
	log, err := logging.New(logging.Options{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		Output: cmd.ErrOrStderr(),
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("logger: %w", err)
	}
	ctx, cancel := signal.NotifyContext(cmd.Context(),
		os.Interrupt, syscall.SIGTERM)
	return log, ctx, cancel, nil
}
