// kscore-repo-gen generates distribution repositories for Keystone Core releases.
//
// This utility creates static repository structures that can be served by
// nginx, Apache, or any CDN for package distribution and registry hosting.
//
// Usage:
//
//	kscore-repo-gen all --version 0.1.0 --output build/repos
//	kscore-repo-gen dnf --version 0.1.0 --output build/repos/dnf
//	kscore-repo-gen apt --version 0.1.0 --output build/repos/apt
//	kscore-repo-gen windows --version 0.1.0 --output build/repos/windows
//	kscore-repo-gen blueprints --output build/repos/blueprints
//	kscore-repo-gen modules --output build/repos/modules
package main

import (
	"fmt"
	"os"

	"github.com/shawnbutts/keystone-core/internal/repogen"
	"github.com/spf13/cobra"
)

var (
	version      string
	outputDir    string
	distDir      string
	binariesDir  string
	gpgKeyID     string
	signPackages bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "kscore-repo-gen",
		Short: "Generate distribution repositories for Keystone Core",
		Long: `kscore-repo-gen generates static repository structures for package distribution.

Supported repository types:
  - dnf/yum: RPM packages for RHEL/CentOS/Fedora
  - apt: DEB packages for Debian/Ubuntu
  - windows: MSI/ZIP packages for Windows
  - blueprints: Blueprint registry (Go-mod style)
  - modules: Module registry for Keystone modules

All repositories are generated to the output directory and can be served
by any static file server (nginx, Apache, S3, CDN, etc.).`,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&version, "version", "v", "", "Version string (e.g., 0.1.0)")
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output", "o", "build/repos", "Output directory")
	rootCmd.PersistentFlags().StringVarP(&distDir, "dist", "d", "dist", "Goreleaser output directory containing packages")
	rootCmd.PersistentFlags().StringVar(&binariesDir, "binaries", "build/bin", "Directory containing built binaries (legacy)")
	rootCmd.PersistentFlags().StringVar(&gpgKeyID, "gpg-key", "", "GPG key ID for signing")
	rootCmd.PersistentFlags().BoolVar(&signPackages, "sign", false, "Sign packages and metadata")

	// Subcommands
	rootCmd.AddCommand(allCmd())
	rootCmd.AddCommand(dnfCmd())
	rootCmd.AddCommand(aptCmd())
	rootCmd.AddCommand(windowsCmd())
	rootCmd.AddCommand(blueprintsCmd())
	rootCmd.AddCommand(modulesCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func allCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "all",
		Short: "Generate all repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &repogen.Config{
				Version:      version,
				OutputDir:    outputDir,
				DistDir:      distDir,
				BinariesDir:  binariesDir,
				GPGKeyID:     gpgKeyID,
				SignPackages: signPackages,
			}

			gen := repogen.NewGenerator(config)
			return gen.GenerateAll()
		},
	}
}

func dnfCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dnf",
		Short: "Generate DNF/YUM repository",
		Long:  "Generate RPM repository with repodata for RHEL/CentOS/Fedora systems",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &repogen.Config{
				Version:      version,
				OutputDir:    outputDir,
				DistDir:      distDir,
				BinariesDir:  binariesDir,
				GPGKeyID:     gpgKeyID,
				SignPackages: signPackages,
			}

			gen := repogen.NewGenerator(config)
			return gen.GenerateDNF()
		},
	}
}

func aptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apt",
		Short: "Generate APT repository",
		Long:  "Generate DEB repository with Packages/Release files for Debian/Ubuntu",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &repogen.Config{
				Version:      version,
				OutputDir:    outputDir,
				DistDir:      distDir,
				BinariesDir:  binariesDir,
				GPGKeyID:     gpgKeyID,
				SignPackages: signPackages,
			}

			gen := repogen.NewGenerator(config)
			return gen.GenerateAPT()
		},
	}
}

func windowsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "windows",
		Short: "Generate Windows repository",
		Long:  "Generate Windows package repository with ZIP packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &repogen.Config{
				Version:      version,
				OutputDir:    outputDir,
				DistDir:      distDir,
				BinariesDir:  binariesDir,
				GPGKeyID:     gpgKeyID,
				SignPackages: signPackages,
			}

			gen := repogen.NewGenerator(config)
			return gen.GenerateWindows()
		},
	}
}

func blueprintsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blueprints",
		Short: "Generate blueprint registry",
		Long:  "Generate Go-mod style blueprint registry from examples/blueprints/kscore/",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &repogen.Config{
				Version:      version,
				OutputDir:    outputDir,
				DistDir:      distDir,
				BinariesDir:  binariesDir,
				GPGKeyID:     gpgKeyID,
				SignPackages: signPackages,
			}

			gen := repogen.NewGenerator(config)
			return gen.GenerateBlueprints()
		},
	}

	cmd.Flags().String("blueprints-dir", "examples/blueprints/kscore", "Source blueprints directory")

	return cmd
}

func modulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "modules",
		Short: "Generate module registry",
		Long:  "Generate Go-mod style module registry for Keystone modules",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := &repogen.Config{
				Version:      version,
				OutputDir:    outputDir,
				DistDir:      distDir,
				BinariesDir:  binariesDir,
				GPGKeyID:     gpgKeyID,
				SignPackages: signPackages,
			}

			gen := repogen.NewGenerator(config)
			return gen.GenerateModules()
		},
	}
}
