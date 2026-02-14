package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-events" {
		t.Errorf("Use = %v, want kscore-events", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"version",
		"list",
		"query",
		"emit",
		"replay",
		"watch",
		"subscribe",
		"export",
		"retention",
		"dlq",
		"analyze",
		"subscribers",
		"storage-stats",
		"prune",
		"archive",
	}

	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestRootCmdFlags(t *testing.T) {
	cmd := newRootCmd()

	persistentFlags := []string{
		"server", "output", "verbose", "timeout",
		"tls", "tls-ca-cert", "tls-cert", "tls-key",
		"tls-skip-verify", "tls-server-name", "tls-min-version",
	}

	for _, flag := range persistentFlags {
		if cmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("expected persistent flag %q not found", flag)
		}
	}
}

func TestRootCmdServerDefault(t *testing.T) {
	cmd := newRootCmd()
	f := cmd.PersistentFlags().Lookup("server")
	if f.DefValue != "localhost:50051" {
		t.Errorf("server default = %v, want localhost:50051", f.DefValue)
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()

	if cmd == nil {
		t.Fatal("newVersionCmd should not return nil")
	}
	if cmd.Use != "version" {
		t.Errorf("Use = %v, want version", cmd.Use)
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("version command should produce output")
	}
}

func TestNewListCmdFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newListCmd(cfg)

	if cmd == nil {
		t.Fatal("newListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}

	flags := []string{"type", "source", "severity", "since", "before", "correlation-id", "limit", "tag", "cursor"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewQueryCmdFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newQueryCmd(cfg)

	if cmd == nil {
		t.Fatal("newQueryCmd should not return nil")
	}
	if cmd.Use != "query <filter-expression>" {
		t.Errorf("Use = %v, want 'query <filter-expression>'", cmd.Use)
	}

	for _, flag := range []string{"limit", "since", "until"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}

	f := cmd.Flags().ShorthandLookup("n")
	if f == nil {
		t.Error("expected shorthand -n for --limit not found")
	} else if f.Name != "limit" {
		t.Errorf("-n shorthand maps to %q, want 'limit'", f.Name)
	}

	if cmd.Flags().Lookup("limit").DefValue != "100" {
		t.Errorf("limit default = %v, want 100", cmd.Flags().Lookup("limit").DefValue)
	}
}

func TestNewEmitCmdFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newEmitCmd(cfg)

	if cmd == nil {
		t.Fatal("newEmitCmd should not return nil")
	}
	if cmd.Use != "emit" {
		t.Errorf("Use = %v, want emit", cmd.Use)
	}

	flags := []string{"type", "source", "severity", "correlation-id", "tag", "data", "data-file"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewWatchCmdFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newWatchCmd(cfg)

	if cmd == nil {
		t.Fatal("newWatchCmd should not return nil")
	}
	if cmd.Use != "watch" {
		t.Errorf("Use = %v, want watch", cmd.Use)
	}

	flags := []string{"type", "source", "severity", "filter", "tag", "format"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}

	f := cmd.Flags().Lookup("format")
	if f.DefValue != "text" {
		t.Errorf("format default = %v, want text", f.DefValue)
	}
}

func TestNewSubscribeCmdFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newSubscribeCmd(cfg)

	if cmd == nil {
		t.Fatal("newSubscribeCmd should not return nil")
	}
	if cmd.Use != "subscribe <pattern>" {
		t.Errorf("Use = %v, want 'subscribe <pattern>'", cmd.Use)
	}

	flags := []string{"filter-type", "filter-severity", "output"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestSubscribeRequiresPattern(t *testing.T) {
	cfg := &Config{}
	cmd := newSubscribeCmd(cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no pattern argument is provided")
	}
}

func TestNewExportCmdFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newExportCmd(cfg)

	if cmd == nil {
		t.Fatal("newExportCmd should not return nil")
	}
	if cmd.Use != "export" {
		t.Errorf("Use = %v, want export", cmd.Use)
	}

	flags := []string{"output", "format", "start", "end", "type", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestExportRequiresOutput(t *testing.T) {
	cfg := &Config{}
	cmd := newExportCmd(cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --output is not provided")
	}
}

func TestNewRetentionCmd(t *testing.T) {
	cmd := newRetentionCmd()

	if cmd == nil {
		t.Fatal("newRetentionCmd should not return nil")
	}
	if cmd.Use != "retention" {
		t.Errorf("Use = %v, want retention", cmd.Use)
	}

	subcommands := []string{"list", "set", "apply"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestNewDLQCmd(t *testing.T) {
	cmd := newDLQCmd()

	if cmd == nil {
		t.Fatal("newDLQCmd should not return nil")
	}
	if cmd.Use != "dlq" {
		t.Errorf("Use = %v, want dlq", cmd.Use)
	}

	subcommands := []string{"list", "show", "retry", "purge"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestNewAnalyzeCmdFlags(t *testing.T) {
	cfg := &Config{}
	cmd := newAnalyzeCmd(cfg)

	if cmd == nil {
		t.Fatal("newAnalyzeCmd should not return nil")
	}
	if cmd.Use != "analyze" {
		t.Errorf("Use = %v, want analyze", cmd.Use)
	}

	flags := []string{"type", "since", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewStorageStatsCmdExists(t *testing.T) {
	cfg := &Config{}
	cmd := newStorageStatsCmd(cfg)

	if cmd == nil {
		t.Fatal("newStorageStatsCmd should not return nil")
	}
	if cmd.Use != "storage-stats" {
		t.Errorf("Use = %v, want storage-stats", cmd.Use)
	}
}

func TestNewPruneCmdFlags(t *testing.T) {
	cmd := newPruneCmd()

	if cmd == nil {
		t.Fatal("newPruneCmd should not return nil")
	}
	if cmd.Use != "prune" {
		t.Errorf("Use = %v, want prune", cmd.Use)
	}

	flags := []string{"older-than", "type", "dry-run", "force"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewArchiveCmdFlags(t *testing.T) {
	cmd := newArchiveCmd()

	if cmd == nil {
		t.Fatal("newArchiveCmd should not return nil")
	}
	if cmd.Use != "archive" {
		t.Errorf("Use = %v, want archive", cmd.Use)
	}

	flags := []string{"older-than", "type", "output", "dry-run"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

// --- Unavailable command tests ---

func TestUnavailableCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  func() *cobra.Command
		args []string
	}{
		{"replay", func() *cobra.Command { return newReplayCmd() }, nil},
		{"subscribers", func() *cobra.Command { return newSubscribersCmd() }, nil},
		{"prune", func() *cobra.Command {
			c := newPruneCmd()
			return c
		}, []string{"--older-than", "7d"}},
		{"archive", func() *cobra.Command {
			c := newArchiveCmd()
			return c
		}, []string{"--older-than", "7d", "--output", "archive.json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			if tt.args != nil {
				cmd.SetArgs(tt.args)
			}

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for unavailable command")
			}
			if !contains(err.Error(), "not yet available") {
				t.Errorf("expected 'not yet available' in error, got: %v", err)
			}
		})
	}
}

func TestRetentionSubcommandsUnavailable(t *testing.T) {
	cmd := newRetentionCmd()

	subs := []string{"list", "set", "apply"}
	for _, sub := range subs {
		t.Run(sub, func(t *testing.T) {
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{sub})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for unavailable command")
			}
			if !contains(err.Error(), "not yet available") {
				t.Errorf("expected 'not yet available' in error, got: %v", err)
			}
		})
	}
}

func TestDLQSubcommandsUnavailable(t *testing.T) {
	tests := []struct {
		sub  string
		args []string
	}{
		{"list", nil},
		{"show", []string{"evt-001"}},
		{"retry", nil},
		{"purge", nil},
	}

	for _, tt := range tests {
		t.Run(tt.sub, func(t *testing.T) {
			cmd := newDLQCmd()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			args := make([]string, 0, 1+len(tt.args))
			args = append(args, tt.sub)
			args = append(args, tt.args...)
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for unavailable command")
			}
			if !contains(err.Error(), "not yet available") {
				t.Errorf("expected 'not yet available' in error, got: %v", err)
			}
		})
	}
}

// --- Helper function tests ---

func TestParseTimeOrDuration(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"1h", false},
		{"30m", false},
		{"7d", false},
		{"60s", false},
		{time.Now().Format(time.RFC3339), false},
		{"invalid", true},
		{"x", true},
		{"10x", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseTimeOrDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimeOrDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseDurationWithDays(t *testing.T) {
	tests := []struct {
		input   string
		wantDur time.Duration
		wantErr bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := parseDurationWithDays(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDurationWithDays(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && d != tt.wantDur {
				t.Errorf("parseDurationWithDays(%q) = %v, want %v", tt.input, d, tt.wantDur)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect map[string]string
	}{
		{
			name:   "single tag",
			input:  []string{"env:prod"},
			expect: map[string]string{"env": "prod"},
		},
		{
			name:   "multiple tags",
			input:  []string{"env:prod", "role:web"},
			expect: map[string]string{"env": "prod", "role": "web"},
		},
		{
			name:   "invalid tag ignored",
			input:  []string{"invalid", "env:prod"},
			expect: map[string]string{"env": "prod"},
		},
		{
			name:   "empty",
			input:  nil,
			expect: map[string]string{},
		},
		{
			name:   "value with colon",
			input:  []string{"url:http://example.com"},
			expect: map[string]string{"url": "http://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTags(tt.input)
			if len(result) != len(tt.expect) {
				t.Fatalf("parseTags() returned %d entries, want %d", len(result), len(tt.expect))
			}
			for k, v := range tt.expect {
				if result[k] != v {
					t.Errorf("parseTags()[%q] = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

// --- Severity conversion tests ---

func TestSeverityToProto(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warning", "warning"},
		{"error", "error"},
		{"critical", "critical"},
		{"DEBUG", "debug"},
		{"INFO", "info"},
		{"unknown", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			proto := severityToProto(tt.input)
			got := protoToSeverity(proto)
			if got != tt.want {
				t.Errorf("roundtrip(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- TLS config tests ---

func TestParseTLSMinVersion(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"", false},
		{"1.3", false},
		{"1.2", false},
		{"tls1.3", false},
		{"tls1.2", false},
		{"TLS1.3", false},
		{"tls13", false},
		{"tls12", false},
		{"1.1", true},
		{"1.0", true},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseTLSMinVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTLSMinVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestBuildTLSConfigRequiresEnvForSkipVerify(t *testing.T) {
	cfg := &Config{
		TLSSkipVerify: true,
	}
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "0")

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error when KSCORE_ALLOW_INSECURE_TLS is not 1")
	}
}

func TestBuildTLSConfigMTLSRequiresBothCertAndKey(t *testing.T) {
	cfg := &Config{
		TLSCert: "/some/cert.pem",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error when only cert is provided without key")
	}
}

func TestBuildTLSConfigInvalidMinVersion(t *testing.T) {
	cfg := &Config{
		TLSMinVersion: "invalid",
	}

	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid TLS min version")
	}
}

// --- Options struct tests ---

func TestListOptionsFields(t *testing.T) {
	opts := ListOptions{
		Type:          "agent.*",
		Source:        "/agents/web-01",
		Severity:      "warning",
		Since:         "1h",
		Before:        "30m",
		CorrelationID: "corr-123",
		Limit:         100,
		Cursor:        "next-page",
		Tags:          []string{"env:prod"},
	}

	if opts.Type != "agent.*" {
		t.Errorf("Type = %v, want agent.*", opts.Type)
	}
	if opts.Limit != 100 {
		t.Errorf("Limit = %d, want 100", opts.Limit)
	}
	if opts.Cursor != "next-page" {
		t.Errorf("Cursor = %v, want next-page", opts.Cursor)
	}
	if len(opts.Tags) != 1 {
		t.Errorf("Tags count = %d, want 1", len(opts.Tags))
	}
}

func TestEmitOptionsFields(t *testing.T) {
	opts := EmitOptions{
		Type:          "custom.event",
		Source:        "/cli/test",
		Severity:      "info",
		CorrelationID: "corr-123",
		Tags:          []string{"env:prod"},
		Data:          `{"key":"value"}`,
		DataFile:      "/path/to/data.json",
	}

	if opts.Type != "custom.event" {
		t.Errorf("Type = %v, want custom.event", opts.Type)
	}
	if opts.Severity != "info" {
		t.Errorf("Severity = %v, want info", opts.Severity)
	}
}

func TestWatchOptionsFields(t *testing.T) {
	opts := WatchOptions{
		Type:     "agent.*",
		Source:   "/agents/*",
		Severity: "warning",
		Filter:   "tags.env == prod",
		Tags:     []string{"env:prod"},
		Format:   "jsonl",
	}

	if opts.Type != "agent.*" {
		t.Errorf("Type = %v, want agent.*", opts.Type)
	}
	if opts.Format != "jsonl" {
		t.Errorf("Format = %v, want jsonl", opts.Format)
	}
}

func TestSubscribeOptionsFields(t *testing.T) {
	opts := SubscribeOptions{
		FilterType:     "agent.connect",
		FilterSeverity: "warning",
		Output:         "json",
	}

	if opts.FilterType != "agent.connect" {
		t.Errorf("FilterType = %v, want agent.connect", opts.FilterType)
	}
	if opts.FilterSeverity != "warning" {
		t.Errorf("FilterSeverity = %v, want warning", opts.FilterSeverity)
	}
	if opts.Output != "json" {
		t.Errorf("Output = %v, want json", opts.Output)
	}
}

func TestExportOptionsFields(t *testing.T) {
	opts := ExportOptions{
		Output: "events.json",
		Format: "csv",
		Start:  "2024-01-01T00:00:00Z",
		End:    "2024-01-02T00:00:00Z",
		Type:   "agent.connect",
		Limit:  500,
	}

	if opts.Output != "events.json" {
		t.Errorf("Output = %v, want events.json", opts.Output)
	}
	if opts.Format != "csv" {
		t.Errorf("Format = %v, want csv", opts.Format)
	}
	if opts.Limit != 500 {
		t.Errorf("Limit = %d, want 500", opts.Limit)
	}
}

func TestAnalyzeOptionsFields(t *testing.T) {
	opts := AnalyzeOptions{
		EventType: "agent.heartbeat",
		Since:     "24h",
		Limit:     5000,
	}

	if opts.EventType != "agent.heartbeat" {
		t.Errorf("EventType = %v, want agent.heartbeat", opts.EventType)
	}
	if opts.Since != "24h" {
		t.Errorf("Since = %v, want 24h", opts.Since)
	}
	if opts.Limit != 5000 {
		t.Errorf("Limit = %d, want 5000", opts.Limit)
	}
}

// --- Config tests ---

func TestConfigDefaults(t *testing.T) {
	cfg := &Config{}
	if cfg.ServerAddr != "" {
		t.Errorf("default ServerAddr should be empty, got %q", cfg.ServerAddr)
	}
}

func TestNotAvailableError(t *testing.T) {
	err := notAvailableError("test feature")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !contains(err.Error(), "test feature") {
		t.Errorf("error should mention feature name, got: %v", err)
	}
	if !contains(err.Error(), "not yet available") {
		t.Errorf("error should mention 'not yet available', got: %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && containsSubstring(s, sub)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
