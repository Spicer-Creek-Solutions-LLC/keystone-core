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
	if cmd.Use != "kscore-files" {
		t.Errorf("expected Use to be 'kscore-files', got %s", cmd.Use)
	}

	if !strings.Contains(cmd.Short, "File") {
		t.Errorf("expected Short to contain 'File', got %s", cmd.Short)
	}

	// Check that all expected subcommands exist
	expectedCommands := []string{"version", "serve", "list", "get", "put", "delete", "info", "sync", "cache", "namespace", "backend", "mirrors"}
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
	versionCmd := findSubcommand(cmd, "version")
	if versionCmd == nil {
		t.Fatal("version subcommand not found")
	}

	if versionCmd.Use != "version" {
		t.Errorf("expected Use to be 'version', got %s", versionCmd.Use)
	}
}

func TestHelpCommand(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected help output to contain 'Usage:', got: %s", output)
	}
	if !strings.Contains(output, "kscore-files") {
		t.Errorf("expected help output to contain 'kscore-files', got: %s", output)
	}
}

func TestGlobalFlags(t *testing.T) {
	cmd := newRootCmd()

	// Check config flag
	configFlag := cmd.PersistentFlags().Lookup("config")
	if configFlag == nil {
		t.Error("expected --config flag")
	}

	// Check nats-url flag
	natsURLFlag := cmd.PersistentFlags().Lookup("nats-url")
	if natsURLFlag == nil {
		t.Fatal("expected --nats-url flag")
	}
	if natsURLFlag.DefValue != "nats://localhost:4222" {
		t.Errorf("expected nats-url default to be 'nats://localhost:4222', got %s", natsURLFlag.DefValue)
	}

	// Check cluster-id flag
	clusterIDFlag := cmd.PersistentFlags().Lookup("cluster-id")
	if clusterIDFlag == nil {
		t.Error("expected --cluster-id flag")
	}

	// Check instance-id flag
	instanceIDFlag := cmd.PersistentFlags().Lookup("instance-id")
	if instanceIDFlag == nil {
		t.Error("expected --instance-id flag")
	}

	// Check audit-level flag
	auditLevelFlag := cmd.PersistentFlags().Lookup("audit-level")
	if auditLevelFlag == nil {
		t.Error("expected --audit-level flag")
	}

	// Check audit-output flag
	auditOutputFlag := cmd.PersistentFlags().Lookup("audit-output")
	if auditOutputFlag == nil {
		t.Error("expected --audit-output flag")
	}
}

func TestServeCommandExists(t *testing.T) {
	cmd := newRootCmd()
	serveCmd := findSubcommand(cmd, "serve")
	if serveCmd == nil {
		t.Fatal("serve subcommand not found")
	}

	if !strings.Contains(serveCmd.Short, "Start") || !strings.Contains(serveCmd.Short, "server") {
		t.Errorf("expected Short to mention starting the server, got %s", serveCmd.Short)
	}
}

func TestFileSubcommandsExist(t *testing.T) {
	cmd := newRootCmd()
	for _, name := range []string{"list", "get", "put", "delete", "info", "sync"} {
		if findSubcommand(cmd, name) == nil {
			t.Errorf("file subcommand %q not found on root", name)
		}
	}
}

func TestCacheCommandExists(t *testing.T) {
	cmd := newRootCmd()
	cacheCmd := findSubcommand(cmd, "cache")
	if cacheCmd == nil {
		t.Fatal("cache subcommand not found")
	}
}

func TestNamespaceCommandExists(t *testing.T) {
	cmd := newRootCmd()
	namespaceCmd := findSubcommand(cmd, "namespace")
	if namespaceCmd == nil {
		t.Fatal("namespace subcommand not found")
	}
}

func TestBackendCommandExists(t *testing.T) {
	cmd := newRootCmd()
	backendCmd := findSubcommand(cmd, "backend")
	if backendCmd == nil {
		t.Fatal("backend subcommand not found (deprecated but should exist)")
	}
}

func TestMirrorsCommandExists(t *testing.T) {
	cmd := newRootCmd()
	mirrorsCmd := findSubcommand(cmd, "mirrors")
	if mirrorsCmd == nil {
		t.Fatal("mirrors subcommand not found (deprecated but should exist)")
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcommands := []string{"serve", "list", "cache", "namespace"}

	for _, subcmd := range subcommands {
		t.Run(subcmd, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{subcmd, "--help"})

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("%s --help failed: %v", subcmd, err)
			}

			output := buf.String()
			if !strings.Contains(output, "Usage:") {
				t.Errorf("expected help output to contain 'Usage:', got: %s", output)
			}
		})
	}
}

func TestCommandStructure(t *testing.T) {
	tests := []struct {
		name        string
		cmdFactory  func() *cobra.Command
		expectedUse string
	}{
		{
			name:        "root command",
			cmdFactory:  newRootCmd,
			expectedUse: "kscore-files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmdFactory()
			if cmd.Use != tt.expectedUse {
				t.Errorf("expected Use to be %s, got %s", tt.expectedUse, cmd.Use)
			}
		})
	}
}

func TestMultipleCommandCreations(t *testing.T) {
	// Test that we can create multiple command instances
	// This tests for state isolation between instances
	for i := 0; i < 3; i++ {
		cmd := newRootCmd()
		if cmd == nil {
			t.Fatalf("execution %d: command is nil", i)
		}
	}
}

func TestDescriptionMentionsNATS(t *testing.T) {
	cmd := newRootCmd()

	// Long description should mention NATS
	if !strings.Contains(cmd.Long, "NATS") {
		t.Errorf("expected Long description to mention NATS, got: %s", cmd.Long)
	}
}

func TestServerConfig(t *testing.T) {
	// Test ServerConfig struct
	config := ServerConfig{}
	config.Server.ClusterID = "test-cluster"
	config.Server.InstanceID = "instance-1"
	config.Server.Workers = 4
	config.NATS.URL = "nats://localhost:4222"

	if config.Server.ClusterID != "test-cluster" {
		t.Errorf("expected ClusterID to be 'test-cluster', got %s", config.Server.ClusterID)
	}
	if config.Server.Workers != 4 {
		t.Errorf("expected Workers to be 4, got %d", config.Server.Workers)
	}
}

func TestBackendConfig(t *testing.T) {
	// Test BackendConfig struct
	config := BackendConfig{
		Name:     "local",
		Type:     "filesystem",
		RootPath: "/var/lib/keystone-core/files",
		Paths:    []string{"/data"},
		ReadOnly: false,
	}

	if config.Name != "local" {
		t.Errorf("expected Name to be 'local', got %s", config.Name)
	}
	if config.Type != "filesystem" {
		t.Errorf("expected Type to be 'filesystem', got %s", config.Type)
	}
}

func TestCacheSubcommands(t *testing.T) {
	cacheCmd := newCacheCmd()
	if cacheCmd == nil {
		t.Fatal("expected cache command to not be nil")
	}

	expected := []string{
		"status", "clear", "warm", "list", "evict", "stats",
		"invalidate", "verify", "show", "refresh", "set-ttl", "history",
	}

	for _, name := range expected {
		found := false
		for _, sub := range cacheCmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected cache subcommand %q not found", name)
		}
	}
}

func TestCacheInvalidateCmd(t *testing.T) {
	cmd := newCacheInvalidateCmd()
	if cmd == nil {
		t.Fatal("expected invalidate command to not be nil")
	}
	if cmd.Use != "invalidate <path-or-pattern>" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	flags := []string{"target", "priority", "force", "dry-run"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s", name)
		}
	}

	if cmd.Flags().Lookup("priority").DefValue != "normal" {
		t.Errorf("expected priority default to be 'normal', got %s", cmd.Flags().Lookup("priority").DefValue)
	}
}

func TestCacheInvalidateDryRun(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	cmd.SetArgs([]string{"cache", "invalidate", "states/nginx-config", "--dry-run"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
}

func TestCacheVerifyCmd(t *testing.T) {
	cmd := newCacheVerifyCmd()
	if cmd == nil {
		t.Fatal("expected verify command to not be nil")
	}
	if cmd.Use != "verify [name]" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	flags := []string{"fix", "output"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s", name)
		}
	}
}

func TestCacheShowCmd(t *testing.T) {
	cmd := newCacheShowCmd()
	if cmd == nil {
		t.Fatal("expected show command to not be nil")
	}
	if cmd.Use != "show <key>" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	if cmd.Flags().Lookup("output") == nil {
		t.Error("expected flag --output")
	}
}

func TestCacheRefreshCmd(t *testing.T) {
	cmd := newCacheRefreshCmd()
	if cmd == nil {
		t.Fatal("expected refresh command to not be nil")
	}
	if cmd.Use != "refresh <path-or-pattern>" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	flags := []string{"force", "dry-run"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s", name)
		}
	}
}

func TestCacheRefreshDryRun(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	cmd.SetArgs([]string{"cache", "refresh", "states/nginx-config", "--dry-run"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
}

func TestCacheSetTTLCmd(t *testing.T) {
	cmd := newCacheSetTTLCmd()
	if cmd == nil {
		t.Fatal("expected set-ttl command to not be nil")
	}
	if cmd.Use != "set-ttl <key> <duration>" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}
}

func TestCacheHistoryCmd(t *testing.T) {
	cmd := newCacheHistoryCmd()
	if cmd == nil {
		t.Fatal("expected history command to not be nil")
	}
	if cmd.Use != "history [name]" {
		t.Errorf("unexpected Use: %s", cmd.Use)
	}

	flags := []string{"type", "limit", "output"}
	for _, name := range flags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s", name)
		}
	}

	if cmd.Flags().Lookup("limit").DefValue != "50" {
		t.Errorf("expected limit default to be '50', got %s", cmd.Flags().Lookup("limit").DefValue)
	}
}

func TestCacheSubcommandHelp(t *testing.T) {
	subcommands := []string{"invalidate", "verify", "show", "refresh", "set-ttl", "history"}

	for _, subcmd := range subcommands {
		t.Run(subcmd, func(t *testing.T) {
			cmd := newRootCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{"cache", subcmd, "--help"})

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("cache %s --help failed: %v", subcmd, err)
			}

			out := buf.String()
			if !strings.Contains(out, "Usage:") {
				t.Errorf("expected help output to contain 'Usage:', got: %s", out)
			}
		})
	}
}

func TestCacheNewTypes(t *testing.T) {
	t.Run("CacheItemDetail", func(t *testing.T) {
		item := CacheItemDetail{
			Key:       "test/key",
			Path:      "/data/test/key",
			Size:      1024,
			Checksum:  "abc123",
			Algorithm: "sha256",
			Source:    "origin",
			Metadata:  map[string]string{"env": "prod"},
		}
		if item.Key != "test/key" {
			t.Errorf("unexpected Key: %s", item.Key)
		}
		if item.Metadata["env"] != "prod" {
			t.Errorf("unexpected Metadata: %v", item.Metadata)
		}
	})

	t.Run("CacheInvalidateResult", func(t *testing.T) {
		r := CacheInvalidateResult{Invalidated: 5, Errors: 1}
		if r.Invalidated != 5 {
			t.Errorf("unexpected Invalidated: %d", r.Invalidated)
		}
	})

	t.Run("CacheVerifyResult", func(t *testing.T) {
		r := CacheVerifyResult{
			TotalEntries:   100,
			ValidEntries:   98,
			CorruptEntries: 1,
			MissingEntries: 1,
			Details:        []CacheVerifyErr{{Key: "bad", Reason: "checksum mismatch"}},
		}
		if r.TotalEntries != 100 {
			t.Errorf("unexpected TotalEntries: %d", r.TotalEntries)
		}
		if len(r.Details) != 1 {
			t.Errorf("unexpected Details length: %d", len(r.Details))
		}
	})

	t.Run("CacheRefreshResult", func(t *testing.T) {
		r := CacheRefreshResult{Refreshed: 10, Errors: 0}
		if r.Refreshed != 10 {
			t.Errorf("unexpected Refreshed: %d", r.Refreshed)
		}
	})

	t.Run("CacheHistoryEntry", func(t *testing.T) {
		e := CacheHistoryEntry{
			Operation: "invalidation",
			Key:       "states/nginx",
			Result:    "success",
		}
		if e.Operation != "invalidation" {
			t.Errorf("unexpected Operation: %s", e.Operation)
		}
	})
}

func TestCreateBackendFilesystem(t *testing.T) {
	dir := t.TempDir()
	b, err := createBackend(BackendConfig{
		Name:     "local",
		Type:     "filesystem",
		RootPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendLocalAlias(t *testing.T) {
	dir := t.TempDir()
	b, err := createBackend(BackendConfig{
		Name:     "local-alias",
		Type:     "local",
		RootPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendS3(t *testing.T) {
	b, err := createBackend(BackendConfig{
		Name:   "s3-store",
		Type:   "s3",
		Bucket: "my-bucket",
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendGCS(t *testing.T) {
	b, err := createBackend(BackendConfig{
		Name:    "gcs-store",
		Type:    "gcs",
		Bucket:  "my-gcs-bucket",
		Project: "my-project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendAzure(t *testing.T) {
	b, err := createBackend(BackendConfig{
		Name:        "azure-store",
		Type:        "azure",
		Container:   "my-container",
		AccountName: "myaccount",
		AccountKey:  "dGVzdGtleQ==",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendGit(t *testing.T) {
	dir := t.TempDir()
	b, err := createBackend(BackendConfig{
		Name:      "git-store",
		Type:      "git",
		URL:       "https://github.com/example/repo.git",
		LocalPath: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendNATS(t *testing.T) {
	b, err := createBackend(BackendConfig{
		Name:       "nats-store",
		Type:       "nats",
		BucketName: "files",
		Endpoint:   "nats://localhost:4222",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendNATSObjectStoreAlias(t *testing.T) {
	b, err := createBackend(BackendConfig{
		Name:       "nats-store-2",
		Type:       "nats-object-store",
		BucketName: "files",
		Endpoint:   "nats://localhost:4222",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected backend to not be nil")
	}
}

func TestCreateBackendUnsupportedType(t *testing.T) {
	_, err := createBackend(BackendConfig{
		Name: "bad",
		Type: "redis",
	})
	if err == nil {
		t.Fatal("expected error for unsupported backend type")
	}
	if !strings.Contains(err.Error(), "unsupported backend type") {
		t.Errorf("expected error to mention 'unsupported backend type', got: %v", err)
	}
}

func TestCreateBackendSupportedTypes(t *testing.T) {
	types := []string{"filesystem", "local", "s3", "gcs", "azure", "git", "nats", "nats-object-store"}
	for _, typ := range types {
		bc := BackendConfig{
			Name:       "test-" + typ,
			Type:       typ,
			RootPath:   t.TempDir(),
			Bucket:     "test-bucket",
			Region:     "us-east-1",
			Project:    "test-project",
			Container:  "test-container",
			AccountName: "testaccount",
			AccountKey: "dGVzdA==",
			URL:        "https://example.com/repo.git",
			LocalPath:  t.TempDir(),
			BucketName: "test",
			Endpoint:   "nats://localhost:4222",
		}
		b, err := createBackend(bc)
		if err != nil {
			t.Errorf("createBackend(%q) failed: %v", typ, err)
			continue
		}
		if b == nil {
			t.Errorf("createBackend(%q) returned nil backend", typ)
		}
	}
}

func TestMirrorFailoverNotYetAvailable(t *testing.T) {
	err := runFailover(nil, nil)
	if err == nil {
		t.Fatal("expected error from failover command")
	}
	if !strings.Contains(err.Error(), "not yet available") {
		t.Errorf("expected 'not yet available' error, got: %v", err)
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
