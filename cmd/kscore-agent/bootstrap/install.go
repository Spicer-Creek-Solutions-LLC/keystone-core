package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/platform"
)

// InstallArtifacts holds information about packages and files installed during bootstrap.
type InstallArtifacts struct {
	PackageManager platform.PackageManager
	Packages       []string
	CreatedFiles   []string
}

func installPhase(ctx context.Context, state *State) error {
	if state.System == nil || state.System.Platform == nil {
		return fmt.Errorf("system detection not completed")
	}

	if state.BootstrapConfig == nil {
		return nil
	}

	packages := packagesForRole(state.BootstrapConfig.NodeRole)
	if len(packages) == 0 {
		return fmt.Errorf("no packages to install for node role %q", state.BootstrapConfig.NodeRole)
	}

	channel := state.BootstrapConfig.PackageChannel
	if channel == "" {
		channel = "stable"
	}
	plan, err := BuildPackagePlan(state.System.Platform.PackageManager, channel, packages, state.BootstrapConfig.PackageVersion)
	if err != nil {
		return err
	}

	if state.DryRun || state.Verbose {
		fmt.Fprintln(state.Output, "package installation plan:")
		fmt.Fprintln(state.Output, renderPackagePlan(plan))
	}

	if state.DryRun {
		return nil
	}

	artifacts := &InstallArtifacts{
		PackageManager: state.System.Platform.PackageManager,
		Packages:       append([]string(nil), packages...),
	}
	state.InstallArtifacts = artifacts

	var created []string

	// Skip repo setup and package installation if packages are pre-installed
	if state.BootstrapConfig.SkipRepoSetup {
		if state.Verbose {
			fmt.Fprintln(state.Output, "skipping repository setup (--skip-repo-setup)")
		}
	} else {
		created, err = applyRepoPlan(ctx, plan.RepoPlan, state.Output, state.Verbose)
		if err != nil {
			return err
		}
		artifacts.CreatedFiles = append(artifacts.CreatedFiles, created...)

		if err := runCommands(ctx, plan.Commands, state.Output, state.Verbose); err != nil {
			return err
		}
	}

	if state.BootstrapConfig.GenerateCerts {
		created, err = ensureTLSFiles(state.BootstrapConfig, state.Output, state.Verbose)
		if err != nil {
			return err
		}
		artifacts.CreatedFiles = append(artifacts.CreatedFiles, created...)
	} else if state.BootstrapConfig.TLSCSRFile != "" {
		created, err = ensureTLSCSRFiles(state.BootstrapConfig, state.Output, state.Verbose)
		if err != nil {
			return err
		}
		artifacts.CreatedFiles = append(artifacts.CreatedFiles, created...)
	}

	if state.BootstrapConfig.TLSRenewalCommand != "" {
		created, err = ensureTLSRenewalScript(state.BootstrapConfig, state.Output, state.Verbose)
		if err != nil {
			return err
		}
		artifacts.CreatedFiles = append(artifacts.CreatedFiles, created...)
	}

	created, err = setupDatabase(state.BootstrapConfig, state.Output, state.Verbose)
	if err != nil {
		return err
	}
	artifacts.CreatedFiles = append(artifacts.CreatedFiles, created...)

	if err := runDatabaseMigration(ctx, state.BootstrapConfig, state.Output, state.Verbose); err != nil {
		return err
	}

	if err := setupNATS(state.BootstrapConfig, state.Output, state.Verbose); err != nil {
		return err
	}

	servicePlan, err := BuildServicePlan(state.BootstrapConfig, string(state.System.Platform.InitSystem))
	if err != nil {
		return err
	}

	if state.Verbose {
		fmt.Fprintln(state.Output, "service configuration plan:")
		fmt.Fprintln(state.Output, renderServicePlan(servicePlan))
	}

	created, err = applyServicePlan(ctx, servicePlan, state.Output, state.Verbose)
	if err != nil {
		return err
	}
	artifacts.CreatedFiles = append(artifacts.CreatedFiles, created...)
	return nil
}

func applyRepoPlan(ctx context.Context, plan RepoPlan, output io.Writer, verbose bool) ([]string, error) {
	var created []string
	if plan.KeyURL != "" && plan.KeyPath != "" {
		if verbose {
			fmt.Fprintf(output, "downloading gpg key from %s\n", plan.KeyURL)
		}
		keyData, err := fetchURL(ctx, plan.KeyURL)
		if err != nil {
			return nil, fmt.Errorf("fetch gpg key: %w", err)
		}
		if createdFile, err := writeFileTracked(plan.KeyPath, keyData, 0o644); err != nil {
			return nil, fmt.Errorf("write gpg key: %w", err)
		} else if createdFile {
			created = append(created, plan.KeyPath)
		}
	}

	for path, content := range plan.Files {
		if verbose {
			fmt.Fprintf(output, "writing repo file %s\n", path)
		}
		if createdFile, err := writeFileTracked(path, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("write repo file %s: %w", path, err)
		} else if createdFile {
			created = append(created, path)
		}
	}

	return created, nil
}

func runCommands(ctx context.Context, commands []CommandPlan, output io.Writer, verbose bool) error {
	for _, cmd := range commands {
		if verbose {
			fmt.Fprintf(output, "running: %s %s\n", cmd.Name, strings.Join(cmd.Args, " "))
		}
		//nolint:gosec // G204: bootstrap install runs predefined commands from validated plans
		result, err := exec.CommandContext(ctx, cmd.Name, cmd.Args...).CombinedOutput() // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		if err != nil {
			return fmt.Errorf("command %s failed: %w (output: %s)", cmd.Name, err, strings.TrimSpace(string(result)))
		}
		if verbose && len(result) > 0 {
			fmt.Fprintln(output, strings.TrimSpace(string(result)))
		}
	}
	return nil
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	//nolint:gosec // G301: parent directory needs to be accessible by service user
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

func writeFileTracked(path string, data []byte, mode os.FileMode) (bool, error) {
	existed := fileExists(path)
	if err := writeFile(path, data, mode); err != nil {
		return false, err
	}
	return !existed, nil
}

func applyServicePlan(ctx context.Context, plan ServicePlan, output io.Writer, verbose bool) ([]string, error) {
	var created []string
	for _, file := range plan.Files {
		if verbose {
			fmt.Fprintf(output, "writing %s\n", file.Path)
		}
		//nolint:gosec // G115: file.Mode is a Unix permission mode (0-0777), always fits in uint32
		if createdFile, err := writeFileTracked(file.Path, file.Content, os.FileMode(file.Mode)); err != nil {
			return nil, fmt.Errorf("write %s: %w", file.Path, err)
		} else if createdFile {
			created = append(created, file.Path)
		}
	}

	if err := runCommands(ctx, plan.Commands, output, verbose); err != nil {
		return nil, err
	}
	return created, nil
}

func ensureTLSFiles(cfg *Config, output io.Writer, verbose bool) ([]string, error) {
	certPath, keyPath, caPath := resolveTLSPaths(cfg)
	cfg.TLSCertFile = certPath
	cfg.TLSKeyFile = keyPath
	cfg.TLSCAFile = caPath

	certExists := fileExists(certPath)
	keyExists := fileExists(keyPath)
	caExists := fileExists(caPath)

	if certExists && keyExists && caExists {
		if verbose {
			fmt.Fprintln(output, "tls files already exist, skipping generation")
		}
		return nil, nil
	}

	if certExists || keyExists || caExists {
		return nil, fmt.Errorf("partial tls files already exist, refusing to overwrite")
	}

	if verbose {
		fmt.Fprintln(output, "generating self-signed tls certificates")
	}
	bundle, err := GenerateTLSBundle(collectTLSHosts(cfg))
	if err != nil {
		return nil, err
	}

	if err := writeFile(certPath, bundle.CertPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write cert: %w", err)
	}
	if err := writeFile(keyPath, bundle.KeyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	if err := writeFile(caPath, bundle.CAPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write ca: %w", err)
	}

	return []string{certPath, keyPath, caPath}, nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}
