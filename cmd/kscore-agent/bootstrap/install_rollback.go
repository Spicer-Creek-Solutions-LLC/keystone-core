package bootstrap

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/shawnbutts/keystone-core/pkg/platform"
)

func installRollback(ctx context.Context, state *State) error {
	if state.System == nil || state.System.Platform == nil {
		return nil
	}
	if state.InstallArtifacts == nil {
		return nil
	}

	var errs []string
	if err := rollbackServices(ctx, state); err != nil {
		errs = append(errs, err.Error())
	}
	if err := rollbackPackages(ctx, state); err != nil {
		errs = append(errs, err.Error())
	}
	if err := rollbackFiles(state); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf("install rollback encountered errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func rollbackServices(ctx context.Context, state *State) error {
	if state.BootstrapConfig == nil {
		return nil
	}
	initSystem := string(state.System.Platform.InitSystem)
	if !strings.EqualFold(initSystem, "systemd") {
		return nil
	}
	services := servicesForRole(state.BootstrapConfig.NodeRole)
	if len(services) == 0 {
		return nil
	}
	var commands []CommandPlan
	for _, service := range services {
		commands = append(commands,
			CommandPlan{Name: "systemctl", Args: []string{"stop", service}},
			CommandPlan{Name: "systemctl", Args: []string{"disable", service}},
		)
	}
	commands = append(commands, CommandPlan{Name: "systemctl", Args: []string{"daemon-reload"}})
	if err := runCommands(ctx, commands, state.Output, state.Verbose); err != nil {
		return fmt.Errorf("rollback services: %w", err)
	}
	return nil
}

func rollbackPackages(ctx context.Context, state *State) error {
	artifacts := state.InstallArtifacts
	if len(artifacts.Packages) == 0 {
		return nil
	}
	commands, ok := uninstallCommands(artifacts.PackageManager, artifacts.Packages)
	if !ok {
		return nil
	}
	if err := runCommands(ctx, commands, state.Output, state.Verbose); err != nil {
		return fmt.Errorf("rollback packages: %w", err)
	}
	return nil
}

func uninstallCommands(manager platform.PackageManager, packages []string) ([]CommandPlan, bool) {
	switch manager {
	case platform.PackageManagerAPT:
		return []CommandPlan{{Name: "apt-get", Args: append([]string{"remove", "-y"}, packages...)}}, true
	case platform.PackageManagerDNF, platform.PackageManagerYum:
		return []CommandPlan{{Name: manager.String(), Args: append([]string{"remove", "-y"}, packages...)}}, true
	case platform.PackageManagerZypper:
		return []CommandPlan{{Name: "zypper", Args: append([]string{"--non-interactive", "rm", "-y"}, packages...)}}, true
	case platform.PackageManagerAPK:
		return []CommandPlan{{Name: "apk", Args: append([]string{"del"}, packages...)}}, true
	default:
		return nil, false
	}
}

func rollbackFiles(state *State) error {
	created := state.InstallArtifacts.CreatedFiles
	if len(created) == 0 {
		return nil
	}
	var errs []string
	for i := len(created) - 1; i >= 0; i-- {
		path := created[i]
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("remove %s: %v", path, err))
		} else if state.Verbose {
			fmt.Fprintf(state.Output, "rollback removed %s\n", path)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("rollback files: %s", strings.Join(errs, "; "))
	}
	return nil
}

func servicesForRole(role string) []string {
	switch strings.ToLower(role) {
	case "control-plane":
		return []string{"kscore-server"}
	case "agent":
		return []string{"kscore-agent"}
	case "both":
		return []string{"kscore-server", "kscore-agent"}
	default:
		return nil
	}
}
