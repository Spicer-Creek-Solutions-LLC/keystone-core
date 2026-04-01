// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootCommand(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatal("expected root command to not be nil")
	}

	// Check basic properties
	if cmd.Use != "kscore-identity" {
		t.Errorf("expected Use to be 'kscore-identity', got %s", cmd.Use)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"token", "ca", "federation", "bundle", "events", "status", "version"}
	for _, expected := range expectedCommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %s not found", expected)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "kscore-identity version") {
		t.Errorf("expected version output to contain 'kscore-identity version', got: %s", output)
	}
}

func TestTokenCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "token list",
			args: []string{"token", "list"},
			want: "TOKEN",
		},
		{
			name: "token create",
			args: []string{"token", "create", "--path", "/agent/test"},
			want: "Token created successfully",
		},
		{
			name: "token show",
			args: []string{"token", "show", "test-token-1"},
			want: "Token Details",
		},
		{
			name: "token revoke",
			args: []string{"token", "revoke", "test-token-1"},
			want: "Token revoked successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.want) {
				t.Errorf("expected output to contain %q, got: %s", tt.want, output)
			}
		})
	}
}

func TestCACommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "ca info",
			args: []string{"ca", "info"},
			want: "Trust Domain",
		},
		{
			name: "ca backup",
			args: []string{"ca", "backup", "--output", "/tmp/ca-backup.tar"},
			want: "CA backup created",
		},
		{
			name: "ca rotate",
			args: []string{"ca", "rotate"},
			want: "CA rotation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.want) {
				t.Errorf("expected output to contain %q, got: %s", tt.want, output)
			}
		})
	}
}

func TestFederationCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "federation list",
			args: []string{"federation", "list"},
			want: "TRUST DOMAIN",
		},
		{
			name: "federation add",
			args: []string{"federation", "add", "partner.example.com", "--endpoint", "https://partner.example.com/.well-known/spiffe-bundle"},
			want: "Federation relationship added",
		},
		{
			name: "federation show",
			args: []string{"federation", "show", "partner.example.com"},
			want: "Trust Domain:",
		},
		{
			name: "federation suspend",
			args: []string{"federation", "suspend", "partner.example.com"},
			want: "Federation relationship suspended",
		},
		{
			name: "federation activate",
			args: []string{"federation", "activate", "partner.example.com"},
			want: "Federation relationship activated",
		},
		{
			name: "federation refresh",
			args: []string{"federation", "refresh", "partner.example.com"},
			want: "Trust bundle refreshed",
		},
		{
			name: "federation remove",
			args: []string{"federation", "remove", "partner.example.com", "--force"},
			want: "Federation relationship removed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.want) {
				t.Errorf("expected output to contain %q, got: %s", tt.want, output)
			}
		})
	}
}

func TestBundleCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "bundle show",
			args: []string{"bundle", "show"},
			want: "Trust Domain:",
		},
		{
			name: "bundle export pem",
			args: []string{"bundle", "export", "--format", "pem"},
			want: "-----BEGIN CERTIFICATE-----",
		},
		{
			name: "bundle export jwks",
			args: []string{"bundle", "export", "--format", "jwks"},
			want: "keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.want) {
				t.Errorf("expected output to contain %q, got: %s", tt.want, output)
			}
		})
	}
}

func TestEventsCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"events", "--limit", "5"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("events command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "TIME") && !strings.Contains(output, "TYPE") {
		t.Errorf("expected events output to contain headers, got: %s", output)
	}
}

func TestStatusCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"status"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("status command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Identity Provider Status") {
		t.Errorf("expected status output to contain 'Identity Provider Status', got: %s", output)
	}
}

func TestOutputFormats(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		format string
		want   string
	}{
		{
			name:   "json output",
			args:   []string{"--output", "json", "status"},
			format: "json",
			want:   "{",
		},
		{
			name:   "yaml output",
			args:   []string{"--output", "yaml", "ca", "info"},
			format: "yaml",
			want:   "trust_domain:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			output := buf.String()
			if !strings.Contains(output, tt.want) {
				t.Errorf("expected %s output to contain %q, got: %s", tt.format, tt.want, output)
			}
		})
	}
}

func TestCommandAliases(t *testing.T) {
	// Test that federation alias works
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"fed", "list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("fed alias failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "TRUST DOMAIN") {
		t.Errorf("expected federation list output, got: %s", output)
	}
}

func TestHelpCommands(t *testing.T) {
	commands := [][]string{
		{"--help"},
		{"token", "--help"},
		{"ca", "--help"},
		{"federation", "--help"},
		{"bundle", "--help"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("help command %v failed: %v", args, err)
			}

			output := buf.String()
			if !strings.Contains(output, "Usage:") {
				t.Errorf("expected help output to contain 'Usage:', got: %s", output)
			}
		})
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check that global flags exist
	serverFlag := cmd.PersistentFlags().Lookup("server")
	if serverFlag == nil {
		t.Fatal("expected --server flag to exist")
	}
	if serverFlag.DefValue != "localhost:9090" {
		t.Errorf("expected server default to be localhost:9090, got %s", serverFlag.DefValue)
	}

	outputFlag := cmd.PersistentFlags().Lookup("output")
	if outputFlag == nil {
		t.Fatal("expected --output flag to exist")
	}
	if outputFlag.DefValue != "table" {
		t.Errorf("expected output default to be table, got %s", outputFlag.DefValue)
	}
}

func TestTokenCreateFlags(t *testing.T) {
	cmd := newRootCmd()
	tokenCmd := findSubcommand(cmd, "token")
	if tokenCmd == nil {
		t.Fatal("token subcommand not found")
	}

	createCmd := findSubcommand(tokenCmd, "create")
	if createCmd == nil {
		t.Fatal("token create subcommand not found")
	}

	// Check that create flags exist
	pathFlag := createCmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Error("expected --path flag on token create")
	}

	ttlFlag := createCmd.Flags().Lookup("ttl")
	if ttlFlag == nil {
		t.Error("expected --ttl flag on token create")
	}

	usesFlag := createCmd.Flags().Lookup("uses")
	if usesFlag == nil {
		t.Error("expected --uses flag on token create")
	}
}

func TestFederationAddFlags(t *testing.T) {
	cmd := newRootCmd()
	fedCmd := findSubcommand(cmd, "federation")
	if fedCmd == nil {
		t.Fatal("federation subcommand not found")
	}

	addCmd := findSubcommand(fedCmd, "add")
	if addCmd == nil {
		t.Fatal("federation add subcommand not found")
	}

	// Check that add flags exist
	endpointFlag := addCmd.Flags().Lookup("endpoint")
	if endpointFlag == nil {
		t.Error("expected --endpoint flag on federation add")
	}

	profileFlag := addCmd.Flags().Lookup("profile")
	if profileFlag == nil {
		t.Error("expected --profile flag on federation add")
	}

	intervalFlag := addCmd.Flags().Lookup("refresh-interval")
	if intervalFlag == nil {
		t.Error("expected --refresh-interval flag on federation add")
	}
}

// findSubcommand finds a subcommand by name
func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}
