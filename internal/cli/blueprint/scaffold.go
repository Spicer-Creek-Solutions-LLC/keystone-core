package blueprint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
)

const scaffoldManifest = `metadata:
  name: %s
  version: 0.1.0
  description: TODO describe this blueprint.
compatibility:
  min_keystone_version: 0.1.0
  platforms:
    - linux
parameters:
  app_name:
    type: string
    description: Logical application name.
    default: %s
entrypoints:
  default: apply.yaml
outputs:
  summary:
    value: "deployed {{ .Params.app_name }}"
`

const scaffoldApply = `metadata:
  name: %s-apply
  version: "0.1"

file:
  /etc/{{ .Params.app_name }}.marker:
    state: present
    content: "{{ .Params.app_name }} deployed\n"
`

const scaffoldReadme = "# %s\n\nScaffolded by `kscore-blueprint init`. Edit `blueprint.yaml`\nand `apply.yaml`, then `kscore-blueprint validate`.\n"

func initCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a new blueprint",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := argDir(args)
			if name == "" {
				name = filepath.Base(filepath.Clean(dir))
				if name == "." || name == "/" {
					name = "my-blueprint"
				}
			}
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return err
			}
			files := map[string]string{
				bp.ManifestFilename: fmt.Sprintf(scaffoldManifest, name, name),
				"apply.yaml":        fmt.Sprintf(scaffoldApply, name),
				"README.md":         fmt.Sprintf(scaffoldReadme, name),
			}
			for fn, body := range files {
				p := filepath.Join(dir, fn)
				if _, err := os.Stat(p); err == nil {
					return fmt.Errorf("refusing to overwrite existing %s", p)
				}
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "scaffolded blueprint %q in %s\n", name, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "blueprint name (default: directory base name)")
	return cmd
}

func validateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [dir]",
		Short: "Load and structurally validate a blueprint manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(argDir(args))
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok: %s@%s\n", m.Metadata.Name, m.Metadata.Version)
			return nil
		},
	}
}

func lintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint [dir]",
		Short: "Validate the manifest and that every entrypoint file is present",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(argDir(args))
			if err != nil {
				return err
			}
			eps := map[string]string{}
			if m.Entrypoints.Default != "" {
				eps["default"] = m.Entrypoints.Default
			}
			if m.Entrypoints.Rollback != "" {
				eps["rollback"] = m.Entrypoints.Rollback
			}
			for k, v := range m.Entrypoints.Named {
				eps[k] = v
			}
			var problems []string
			for name, rel := range eps {
				p := filepath.Join(m.SourcePath, rel)
				if st, err := os.Stat(p); err != nil || st.IsDir() {
					problems = append(problems, fmt.Sprintf("entrypoint %q → %s: not a readable file", name, rel))
				}
			}
			for _, h := range allHooks(m) {
				p := filepath.Join(m.SourcePath, h)
				if st, err := os.Stat(p); err != nil || st.IsDir() {
					problems = append(problems, fmt.Sprintf("hook %q: not a readable file", h))
				}
			}
			if len(problems) > 0 {
				sort.Strings(problems)
				for _, p := range problems {
					fmt.Fprintf(cmd.ErrOrStderr(), "lint: %s\n", p)
				}
				return fmt.Errorf("lint: %d problem(s) in %s", len(problems), m.Metadata.Name)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "lint ok: %s@%s\n", m.Metadata.Name, m.Metadata.Version)
			return nil
		},
	}
}

func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info [dir]",
		Short: "Print a summary of a blueprint manifest",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(argDir(args))
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "name:        %s\n", m.Metadata.Name)
			fmt.Fprintf(w, "version:     %s\n", m.Metadata.Version)
			if m.Metadata.Description != "" {
				fmt.Fprintf(w, "description: %s\n", m.Metadata.Description)
			}
			fmt.Fprintf(w, "entrypoints: default=%s rollback=%s\n",
				orDash(m.Entrypoints.Default), orDash(m.Entrypoints.Rollback))
			fmt.Fprintf(w, "parameters:  %s\n", joinSortedParams(m))
			fmt.Fprintf(w, "features:    %s\n", joinSortedKeys(featureNames(m)))
			if hooks := allHooks(m); len(hooks) > 0 {
				fmt.Fprintf(w, "hooks:       %v\n", hooks)
			}
			return nil
		},
	}
}

func allHooks(m *bp.Manifest) []string {
	var out []string
	out = append(out, m.Hooks.PreApply...)
	out = append(out, m.Hooks.PostApply...)
	out = append(out, m.Hooks.PreRollback...)
	out = append(out, m.Hooks.PostRollback...)
	return out
}

func featureNames(m *bp.Manifest) []string {
	names := make([]string, 0, len(m.Features))
	for n := range m.Features {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func joinSortedParams(m *bp.Manifest) string {
	names := make([]string, 0, len(m.Parameters))
	for n := range m.Parameters {
		names = append(names, n)
	}
	sort.Strings(names)
	return joinSortedKeys(names)
}

func joinSortedKeys(keys []string) string {
	if len(keys) == 0 {
		return "(none)"
	}
	out := keys[0]
	for _, k := range keys[1:] {
		out += ", " + k
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
