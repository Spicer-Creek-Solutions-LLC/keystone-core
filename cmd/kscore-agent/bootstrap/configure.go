package bootstrap

import (
	"context"
	"fmt"
	"strings"
)

func configurePhase(ctx context.Context, state *State) error {
	if state.BootstrapConfig == nil {
		return nil
	}

	if state.Verbose || state.DryRun {
		fmt.Fprintln(state.Output, "bootstrap configuration:")
		fmt.Fprintf(state.Output, "  mode: %s\n", state.BootstrapConfig.Mode)
		fmt.Fprintf(state.Output, "  cluster: %s\n", state.BootstrapConfig.ClusterName)
		if state.BootstrapConfig.NodeRole != "" {
			fmt.Fprintf(state.Output, "  node role: %s\n", state.BootstrapConfig.NodeRole)
		}
		if state.BootstrapConfig.NodeName != "" {
			fmt.Fprintf(state.Output, "  node name: %s\n", state.BootstrapConfig.NodeName)
		}
		if len(state.BootstrapConfig.NodeLabels) > 0 {
			fmt.Fprintf(state.Output, "  node labels: %s\n", formatNodeLabels(state.BootstrapConfig.NodeLabels))
		}
		if len(state.BootstrapConfig.Regions) > 0 {
			fmt.Fprintf(state.Output, "  regions: %s\n", strings.Join(state.BootstrapConfig.Regions, ", "))
		}
		if state.BootstrapConfig.HAEnabled {
			fmt.Fprintf(state.Output, "  ha enabled: true\n")
			if state.BootstrapConfig.HAReplicas != 0 {
				fmt.Fprintf(state.Output, "  ha replicas: %d\n", state.BootstrapConfig.HAReplicas)
			}
		}
		if state.BootstrapConfig.ObservabilityBackend != "" && state.BootstrapConfig.ObservabilityBackend != "none" {
			fmt.Fprintf(state.Output, "  observability: %s\n", state.BootstrapConfig.ObservabilityBackend)
			if state.BootstrapConfig.ObservabilityEndpoint != "" {
				fmt.Fprintf(state.Output, "  observability endpoint: %s\n", state.BootstrapConfig.ObservabilityEndpoint)
			}
		}
		if state.BootstrapConfig.IdentityProvider != "" && state.BootstrapConfig.IdentityProvider != "none" {
			fmt.Fprintf(state.Output, "  identity provider: %s\n", state.BootstrapConfig.IdentityProvider)
			if state.BootstrapConfig.IdentityEndpoint != "" {
				fmt.Fprintf(state.Output, "  identity endpoint: %s\n", state.BootstrapConfig.IdentityEndpoint)
			}
		}
		if state.BootstrapConfig.ExportStatesDir != "" {
			fmt.Fprintf(state.Output, "  export states dir: %s\n", state.BootstrapConfig.ExportStatesDir)
		}
		if state.BootstrapConfig.Storage != "" {
			fmt.Fprintf(state.Output, "  storage: %s\n", state.BootstrapConfig.Storage)
		}
		if state.BootstrapConfig.NATSMode != "" {
			fmt.Fprintf(state.Output, "  nats mode: %s\n", state.BootstrapConfig.NATSMode)
		}
		if state.BootstrapConfig.PackageChannel != "" {
			fmt.Fprintf(state.Output, "  package channel: %s\n", state.BootstrapConfig.PackageChannel)
		}
		if state.BootstrapConfig.PackageVersion != "" {
			fmt.Fprintf(state.Output, "  package version: %s\n", state.BootstrapConfig.PackageVersion)
		}
		if state.BootstrapConfig.BindAddress != "" {
			fmt.Fprintf(state.Output, "  bind address: %s\n", state.BootstrapConfig.BindAddress)
		}
		if state.BootstrapConfig.Advertise != "" {
			fmt.Fprintf(state.Output, "  advertise address: %s\n", state.BootstrapConfig.Advertise)
		}
		if state.BootstrapConfig.Join != "" {
			fmt.Fprintf(state.Output, "  join: %s\n", state.BootstrapConfig.Join)
		}
		if state.BootstrapConfig.JoinToken != "" {
			fmt.Fprintln(state.Output, "  join token: provided")
		}
		if state.BootstrapConfig.TLSCSRFile != "" {
			fmt.Fprintf(state.Output, "  tls csr file: %s\n", state.BootstrapConfig.TLSCSRFile)
		}
		if state.BootstrapConfig.TLSRenewalCommand != "" {
			fmt.Fprintf(state.Output, "  tls renewal command: %s\n", state.BootstrapConfig.TLSRenewalCommand)
		}
		if state.BootstrapConfig.MigrateFromSQLite != "" {
			fmt.Fprintf(state.Output, "  migrate from sqlite: %s\n", state.BootstrapConfig.MigrateFromSQLite)
		}
	}

	return nil
}
