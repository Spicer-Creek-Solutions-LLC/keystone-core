// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/agent"
)

// newServiceInstallCmd creates the service install command
func newServiceInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-install",
		Short: "Install the agent as a Windows service",
		Long: `Install the Keystone Core agent as a Windows service.
The service will be configured to start automatically on boot.

This command must be run as Administrator.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "windows" {
				return fmt.Errorf("service installation is only supported on Windows")
			}

			// Create installer with default config
			installer := agent.NewServiceInstaller(nil)

			// Check if already installed
			if installer.Exists() {
				return fmt.Errorf("service is already installed")
			}

			// Install the service
			if err := installer.Install(); err != nil {
				return fmt.Errorf("failed to install service: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Service installed successfully")
			fmt.Fprintln(cmd.OutOrStdout(), "Use 'kscore-agent service-start' to start the service")
			return nil
		},
		SilenceUsage: true,
	}
	return cmd
}

// newServiceUninstallCmd creates the service uninstall command
func newServiceUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-uninstall",
		Short: "Uninstall the agent Windows service",
		Long: `Uninstall the Keystone Core agent Windows service.
The service will be stopped if running and removed from the system.

This command must be run as Administrator.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "windows" {
				return fmt.Errorf("service uninstallation is only supported on Windows")
			}

			// Create installer with default config
			installer := agent.NewServiceInstaller(nil)

			// Check if installed
			if !installer.Exists() {
				return fmt.Errorf("service is not installed")
			}

			// Uninstall the service
			if err := installer.Uninstall(); err != nil {
				return fmt.Errorf("failed to uninstall service: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Service uninstalled successfully")
			return nil
		},
		SilenceUsage: true,
	}
	return cmd
}

// newServiceStartCmd creates the service start command
func newServiceStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-start",
		Short: "Start the agent Windows service",
		Long: `Start the Keystone Core agent Windows service.

This command must be run as Administrator.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "windows" {
				return fmt.Errorf("service management is only supported on Windows")
			}

			// Create installer with default config
			installer := agent.NewServiceInstaller(nil)

			// Check if installed
			if !installer.Exists() {
				return fmt.Errorf("service is not installed")
			}

			// Start the service
			if err := installer.Start(); err != nil {
				return fmt.Errorf("failed to start service: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Service started successfully")
			return nil
		},
		SilenceUsage: true,
	}
	return cmd
}

// newServiceStopCmd creates the service stop command
func newServiceStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-stop",
		Short: "Stop the agent Windows service",
		Long: `Stop the Keystone Core agent Windows service.

This command must be run as Administrator.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "windows" {
				return fmt.Errorf("service management is only supported on Windows")
			}

			// Create installer with default config
			installer := agent.NewServiceInstaller(nil)

			// Check if installed
			if !installer.Exists() {
				return fmt.Errorf("service is not installed")
			}

			// Stop the service
			if err := installer.Stop(); err != nil {
				return fmt.Errorf("failed to stop service: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Service stopped successfully")
			return nil
		},
		SilenceUsage: true,
	}
	return cmd
}

// newServiceStatusCmd creates the service status command
func newServiceStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service-status",
		Short: "Show the agent Windows service status",
		Long:  `Show the current status of the Keystone Core agent Windows service.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "windows" {
				return fmt.Errorf("service management is only supported on Windows")
			}

			// Create installer with default config
			installer := agent.NewServiceInstaller(nil)

			// Check if installed
			if !installer.Exists() {
				fmt.Fprintln(cmd.OutOrStdout(), "Service is not installed")
				return nil
			}

			// Get status
			status, err := installer.Status()
			if err != nil {
				return fmt.Errorf("failed to get service status: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Service Status: %s\n", status.State)
			fmt.Fprintf(cmd.OutOrStdout(), "Description:    %s\n", status.Description)
			if status.ProcessID > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Process ID:     %d\n", status.ProcessID)
			}
			return nil
		},
		SilenceUsage: true,
	}
	return cmd
}
