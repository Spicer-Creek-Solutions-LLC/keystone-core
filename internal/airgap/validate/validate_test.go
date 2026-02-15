package validate

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// Interface compliance
var (
	_ Checker = (*BinaryChecker)(nil)
	_ Checker = (*ConfigChecker)(nil)
	_ Checker = (*ModuleChecker)(nil)
	_ Checker = (*NetworkChecker)(nil)
)

func TestBinaryChecker_ExternalURL(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "kscore-test")
	content := []byte("some binary content https://evil.example.com/api more content")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	checker := &BinaryChecker{BinaryDir: dir}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var hasFail bool
	for _, f := range findings {
		if f.Severity == SeverityFail {
			hasFail = true
		}
	}
	if !hasFail {
		t.Error("expected fail finding for external URL")
	}
}

func TestBinaryChecker_InternalOnly(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "kscore-clean")
	content := []byte("binary with http://localhost:8080/api and http://127.0.0.1/health")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	checker := &BinaryChecker{BinaryDir: dir}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range findings {
		if f.Severity == SeverityFail {
			t.Errorf("unexpected fail: %s", f.Message)
		}
	}
}

func TestBinaryChecker_NoBinaries(t *testing.T) {
	dir := t.TempDir()
	checker := &BinaryChecker{BinaryDir: dir}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}
	if findings[0].Severity != SeverityWarn {
		t.Errorf("expected warn for no binaries, got %s", findings[0].Severity)
	}
}

func TestConfigChecker_ExternalNTP(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	content := "ntp_server: pool.ntp.org\nport: 8080\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	checker := &ConfigChecker{ConfigDir: dir}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var hasWarn bool
	for _, f := range findings {
		if f.Severity == SeverityWarn {
			hasWarn = true
		}
	}
	if !hasWarn {
		t.Error("expected warn finding for NTP reference")
	}
}

func TestConfigChecker_ExternalRegistry(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "registry.yaml")
	content := "image: docker.io/library/nginx:latest\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	checker := &ConfigChecker{ConfigDir: dir}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var hasFail bool
	for _, f := range findings {
		if f.Severity == SeverityFail {
			hasFail = true
		}
	}
	if !hasFail {
		t.Error("expected fail finding for external registry")
	}
}

func TestConfigChecker_ExternalIP(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "network.yaml")
	content := "server: 203.0.113.5\nlocal: 10.0.0.1\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, internalNet, _ := net.ParseCIDR("10.0.0.0/8")
	checker := &ConfigChecker{
		ConfigDir:    dir,
		InternalNets: []*net.IPNet{internalNet},
	}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var failCount int
	for _, f := range findings {
		if f.Severity == SeverityFail {
			failCount++
		}
	}
	if failCount != 1 {
		t.Errorf("expected 1 fail (203.0.113.5), got %d", failCount)
	}
}

func TestConfigChecker_Clean(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "clean.yaml")
	content := "port: 8080\nlog_level: info\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	checker := &ConfigChecker{ConfigDir: dir}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range findings {
		if f.Severity == SeverityFail {
			t.Errorf("unexpected fail: %s", f.Message)
		}
	}
}

func TestModuleChecker_Partial(t *testing.T) {
	dir := t.TempDir()
	modulesDir := filepath.Join(dir, "modules", "std")
	if err := os.MkdirAll(filepath.Join(modulesDir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(modulesDir, "exec"), 0o755); err != nil {
		t.Fatal(err)
	}

	checker := &ModuleChecker{
		RegistryDir:     dir,
		RequiredModules: []string{"std/files", "std/exec", "std/http"},
	}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var passCount, failCount int
	for _, f := range findings {
		switch f.Severity {
		case SeverityPass:
			passCount++
		case SeverityFail:
			failCount++
		case SeverityWarn:
			// ignored for this test
		}
	}
	if passCount != 2 {
		t.Errorf("passCount = %d, want 2", passCount)
	}
	if failCount != 1 {
		t.Errorf("failCount = %d, want 1", failCount)
	}
}

func TestModuleChecker_FromIndex(t *testing.T) {
	dir := t.TempDir()
	idx := map[string]interface{}{
		"modules": []map[string]string{
			{"name": "std/files"},
			{"name": "std/exec"},
		},
	}
	data, _ := json.Marshal(idx)
	if err := os.WriteFile(filepath.Join(dir, "index.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	checker := &ModuleChecker{
		RegistryDir:     dir,
		RequiredModules: []string{"std/files", "std/missing"},
	}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var passCount, failCount int
	for _, f := range findings {
		switch f.Severity {
		case SeverityPass:
			passCount++
		case SeverityFail:
			failCount++
		case SeverityWarn:
			// ignored for this test
		}
	}
	if passCount != 1 || failCount != 1 {
		t.Errorf("pass=%d fail=%d, want pass=1 fail=1", passCount, failCount)
	}
}

func TestModuleChecker_NoRegistry(t *testing.T) {
	checker := &ModuleChecker{}
	findings, err := checker.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Severity != SeverityWarn {
		t.Error("expected warn for no registry")
	}
}

func TestNetworkChecker_ParseHexAddr_IPv4(t *testing.T) {
	// 0100007F = 127.0.0.1 in little-endian
	ip, err := parseHexAddr("0100007F:0050")
	if err != nil {
		t.Fatal(err)
	}
	if !ip.IsLoopback() {
		t.Errorf("expected loopback, got %s", ip)
	}
}

func TestNetworkChecker_ParseHexAddr_External(t *testing.T) {
	// 08080808 = 8.8.8.8 in little-endian
	ip, err := parseHexAddr("08080808:0035")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "8.8.8.8" {
		t.Errorf("expected 8.8.8.8, got %s", ip)
	}
}

func TestNetworkChecker_ParseProcNetLine(t *testing.T) {
	line := "   0: 0100007F:0CEA 0100007F:BF76 01 00000000:00000000 02:00000001 00000000     0        0 12345 2 0000000000000000 20 4 30 10 -1"
	ip, err := parseProcNetLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if !ip.IsLoopback() {
		t.Errorf("expected loopback remote, got %s", ip)
	}
}

func TestValidator_Aggregation(t *testing.T) {
	v := NewValidator()

	// Add a checker that always passes
	v.AddChecker(&staticChecker{
		name:     "always-pass",
		category: CategoryBinary,
		findings: []Finding{
			{Category: CategoryBinary, Check: "always-pass", Severity: SeverityPass, Message: "all good"},
		},
	})

	// Add a checker that fails
	v.AddChecker(&staticChecker{
		name:     "always-fail",
		category: CategoryNetwork,
		findings: []Finding{
			{Category: CategoryNetwork, Check: "always-fail", Severity: SeverityFail, Message: "bad connection"},
		},
	})

	// Add a checker that warns
	v.AddChecker(&staticChecker{
		name:     "always-warn",
		category: CategoryConfiguration,
		findings: []Finding{
			{Category: CategoryConfiguration, Check: "always-warn", Severity: SeverityWarn, Message: "maybe ok"},
		},
	})

	report, err := v.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if report.Compliant {
		t.Error("expected non-compliant report (has failures)")
	}
	if report.PassCount != 1 {
		t.Errorf("PassCount = %d, want 1", report.PassCount)
	}
	if report.WarnCount != 1 {
		t.Errorf("WarnCount = %d, want 1", report.WarnCount)
	}
	if report.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", report.FailCount)
	}
	if len(report.Findings) != 3 {
		t.Errorf("Findings len = %d, want 3", len(report.Findings))
	}
}

func TestValidator_AllPass(t *testing.T) {
	v := NewValidator()
	v.AddChecker(&staticChecker{
		name:     "pass1",
		category: CategoryBinary,
		findings: []Finding{
			{Category: CategoryBinary, Check: "pass1", Severity: SeverityPass, Message: "ok"},
		},
	})

	report, err := v.Validate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Compliant {
		t.Error("expected compliant report")
	}
}

func TestReport_JSONRoundtrip(t *testing.T) {
	report := &Report{
		Compliant: true,
		PassCount: 5,
		Findings: []Finding{
			{Category: CategoryBinary, Check: "test", Severity: SeverityPass, Message: "ok"},
		},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PassCount != 5 {
		t.Errorf("PassCount = %d, want 5", decoded.PassCount)
	}
	if len(decoded.Findings) != 1 {
		t.Errorf("Findings len = %d, want 1", len(decoded.Findings))
	}
}

func TestWriteReportToFile(t *testing.T) {
	report := &Report{
		Compliant: true,
		PassCount: 1,
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteReportToFile(report, path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Compliant {
		t.Error("expected compliant")
	}
}

// staticChecker is a test helper that returns fixed findings.
type staticChecker struct {
	name     string
	category CheckCategory
	findings []Finding
}

func (c *staticChecker) Name() string           { return c.name }
func (c *staticChecker) Category() CheckCategory { return c.category }
func (c *staticChecker) Check(_ context.Context) ([]Finding, error) {
	return c.findings, nil
}
