package testing

import (
	"testing"
	"time"
)

func TestTestSuite_Structure(t *testing.T) {
	suite := TestSuite{
		Name:        "Integration Tests",
		Description: "Tests for integration scenarios",
		Tags:        []string{"integration", "slow"},
		Setup: &TestSetup{
			Commands: []TestCommand{{Command: "setup.sh"}},
		},
		Teardown: &TestTeardown{
			Commands: []TestCommand{{Command: "cleanup.sh"}},
		},
		Defaults: &TestDefaults{
			Timeout: Duration(30 * time.Second),
			DryRun:  true,
		},
		Tests: []TestCase{
			{Name: "test1"},
			{Name: "test2"},
		},
	}

	if suite.Name != "Integration Tests" {
		t.Errorf("Name = %q, want %q", suite.Name, "Integration Tests")
	}
	if len(suite.Tags) != 2 {
		t.Errorf("len(Tags) = %d, want 2", len(suite.Tags))
	}
	if suite.Setup == nil {
		t.Error("Setup should not be nil")
	}
	if suite.Teardown == nil {
		t.Error("Teardown should not be nil")
	}
	if suite.Defaults == nil {
		t.Error("Defaults should not be nil")
	}
	if len(suite.Tests) != 2 {
		t.Errorf("len(Tests) = %d, want 2", len(suite.Tests))
	}
}

func TestTestCase_Structure(t *testing.T) {
	testCase := TestCase{
		Name:          "my_test",
		Description:   "A test case",
		Tags:          []string{"quick", "smoke"},
		Skip:          "",
		ExpectFailure: false,
		ExpectError:   "",
		Timeout:       Duration(60 * time.Second),
		DryRun:        true,
		Parameters: map[string]interface{}{
			"port":    8080,
			"enabled": true,
		},
		Setup: &TestSetup{
			Commands: []TestCommand{{Command: "pre.sh"}},
		},
		Teardown: &TestTeardown{
			Commands: []TestCommand{{Command: "post.sh"}},
		},
		Mocks: []MockConfig{
			{Type: "command"},
		},
		Assertions: []Assertion{
			{Type: AssertNoFailures},
		},
	}

	if testCase.Name != "my_test" {
		t.Errorf("Name = %q, want %q", testCase.Name, "my_test")
	}
	if testCase.Timeout != Duration(60*time.Second) {
		t.Errorf("Timeout = %v, want 60s", testCase.Timeout)
	}
	if !testCase.DryRun {
		t.Error("DryRun should be true")
	}
	if testCase.Parameters["port"] != 8080 {
		t.Errorf("Parameters[port] = %v, want 8080", testCase.Parameters["port"])
	}
	if len(testCase.Mocks) != 1 {
		t.Errorf("len(Mocks) = %d, want 1", len(testCase.Mocks))
	}
	if len(testCase.Assertions) != 1 {
		t.Errorf("len(Assertions) = %d, want 1", len(testCase.Assertions))
	}
}

func TestTestDefaults_Structure(t *testing.T) {
	defaults := TestDefaults{
		Timeout: Duration(5 * time.Minute),
		DryRun:  true,
		Parameters: map[string]interface{}{
			"env": "test",
		},
		Mocks: []MockConfig{
			{Type: "file"},
		},
	}

	if defaults.Timeout != Duration(5*time.Minute) {
		t.Errorf("Timeout = %v, want 5m", defaults.Timeout)
	}
	if !defaults.DryRun {
		t.Error("DryRun should be true")
	}
	if defaults.Parameters["env"] != "test" {
		t.Errorf("Parameters[env] = %v, want %q", defaults.Parameters["env"], "test")
	}
}

func TestTestSetup_Structure(t *testing.T) {
	setup := TestSetup{
		Commands: []TestCommand{
			{Command: "cmd1", Args: []string{"arg1", "arg2"}},
			{Command: "cmd2"},
		},
		States: []string{"state1", "state2"},
		Files: []TestFile{
			{Path: "/tmp/test", Content: "content"},
		},
	}

	if len(setup.Commands) != 2 {
		t.Errorf("len(Commands) = %d, want 2", len(setup.Commands))
	}
	if len(setup.States) != 2 {
		t.Errorf("len(States) = %d, want 2", len(setup.States))
	}
	if len(setup.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(setup.Files))
	}
}

func TestTestTeardown_Structure(t *testing.T) {
	teardown := TestTeardown{
		Always:   true,
		Commands: []TestCommand{{Command: "cleanup.sh"}},
		States:   []string{"cleanup_state"},
		Files:    []string{"/tmp/test1", "/tmp/test2"},
	}

	if !teardown.Always {
		t.Error("Always should be true")
	}
	if len(teardown.Commands) != 1 {
		t.Errorf("len(Commands) = %d, want 1", len(teardown.Commands))
	}
	if len(teardown.States) != 1 {
		t.Errorf("len(States) = %d, want 1", len(teardown.States))
	}
	if len(teardown.Files) != 2 {
		t.Errorf("len(Files) = %d, want 2", len(teardown.Files))
	}
}

func TestTestCommand_Structure(t *testing.T) {
	cmd := TestCommand{
		Command:      "docker",
		Args:         []string{"run", "-d", "nginx"},
		Shell:        "/bin/bash",
		IgnoreErrors: true,
	}

	if cmd.Command != "docker" {
		t.Errorf("Command = %q, want %q", cmd.Command, "docker")
	}
	if len(cmd.Args) != 3 {
		t.Errorf("len(Args) = %d, want 3", len(cmd.Args))
	}
	if cmd.Shell != "/bin/bash" {
		t.Errorf("Shell = %q, want %q", cmd.Shell, "/bin/bash")
	}
	if !cmd.IgnoreErrors {
		t.Error("IgnoreErrors should be true")
	}
}

func TestTestFile_Structure(t *testing.T) {
	file := TestFile{
		Path:    "/etc/app.conf",
		Content: "key=value",
		Mode:    "0644",
		Owner:   "root",
		Group:   "root",
		IsDir:   false,
	}

	if file.Path != "/etc/app.conf" {
		t.Errorf("Path = %q, want %q", file.Path, "/etc/app.conf")
	}
	if file.Content != "key=value" {
		t.Errorf("Content = %q, want %q", file.Content, "key=value")
	}
	if file.Mode != "0644" {
		t.Errorf("Mode = %q, want %q", file.Mode, "0644")
	}
}

func TestAssertion_AllTypes(t *testing.T) {
	// Verify all assertion types are defined
	assertionTypes := []AssertionType{
		AssertNoFailures,
		AssertStateApplied,
		AssertStateChanged,
		AssertStateUnchanged,
		AssertStateFailed,
		AssertStatesApplied,
		AssertStatesChanged,
		AssertStatesFailed,
		AssertFileExists,
		AssertFileNotExists,
		AssertFileContains,
		AssertFileMode,
		AssertFileOwner,
		AssertDirectoryExists,
		AssertCommandSuccess,
		AssertCommandFailure,
		AssertCommandOutput,
		AssertOutputContains,
		AssertOutputEquals,
		AssertOutputMatches,
		AssertExpression,
		AssertIdempotent,
	}

	for _, at := range assertionTypes {
		if at == "" {
			t.Error("AssertionType should not be empty")
		}
	}
}

func TestAssertion_Structure(t *testing.T) {
	assertion := Assertion{
		Type:        AssertFileContains,
		Description: "Config should contain port",
		Target:      "configure_app",
		Pattern:     "port=8080",
		Expected:    "8080",
		File: &FileAssertion{
			Path:     "/etc/app.conf",
			Contains: "port=8080",
		},
		State: &StateAssertion{
			ID:     "configure_app",
			Module: "file",
		},
		Command: &CommandAssertion{
			Command: "grep port /etc/app.conf",
		},
		Output: &OutputAssertion{
			Name:  "app_port",
			Value: "8080",
		},
	}

	if assertion.Type != AssertFileContains {
		t.Errorf("Type = %q, want %q", assertion.Type, AssertFileContains)
	}
	if assertion.File == nil {
		t.Error("File should not be nil")
	}
	if assertion.State == nil {
		t.Error("State should not be nil")
	}
	if assertion.Command == nil {
		t.Error("Command should not be nil")
	}
	if assertion.Output == nil {
		t.Error("Output should not be nil")
	}
}

func TestFileAssertion_Structure(t *testing.T) {
	fa := FileAssertion{
		Path:     "/etc/test.conf",
		Contains: "expected content",
		Mode:     "0644",
		Owner:    "root",
		Group:    "root",
	}

	if fa.Path != "/etc/test.conf" {
		t.Errorf("Path = %q, want %q", fa.Path, "/etc/test.conf")
	}
	if fa.Mode != "0644" {
		t.Errorf("Mode = %q, want %q", fa.Mode, "0644")
	}
}

func TestStateAssertion_Structure(t *testing.T) {
	changed := true
	sa := StateAssertion{
		ID:      "my_state",
		Module:  "package",
		Changed: &changed,
	}

	if sa.ID != "my_state" {
		t.Errorf("ID = %q, want %q", sa.ID, "my_state")
	}
	if sa.Changed == nil || !*sa.Changed {
		t.Error("Changed should be true")
	}
}

func TestCommandAssertion_Structure(t *testing.T) {
	exitCode := 0
	ca := CommandAssertion{
		Command:  "systemctl status nginx",
		ExitCode: &exitCode,
		Stdout:   "active (running)",
		Stderr:   "",
	}

	if ca.Command != "systemctl status nginx" {
		t.Errorf("Command = %q, want %q", ca.Command, "systemctl status nginx")
	}
	if ca.ExitCode == nil || *ca.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", ca.ExitCode)
	}
}

func TestOutputAssertion_Structure(t *testing.T) {
	oa := OutputAssertion{
		Name:     "result",
		Value:    "success",
		Contains: "succ",
		Matches:  "^succ.*",
	}

	if oa.Name != "result" {
		t.Errorf("Name = %q, want %q", oa.Name, "result")
	}
	if oa.Contains != "succ" {
		t.Errorf("Contains = %q, want %q", oa.Contains, "succ")
	}
}

func TestMockConfig_Structure(t *testing.T) {
	mock := MockConfig{
		Type:    "command",
		Command: &CommandMock{Pattern: "echo.*", Stdout: "hello"},
		File:    &FileMock{Path: "/test", Exists: true},
		HTTP:    &HTTPMock{URL: "https://api.test.com/.*", StatusCode: 200},
		Package: &PackageMock{Name: "nginx", Installed: true},
		Service: &ServiceMock{Name: "nginx", Running: true},
	}

	if mock.Type != "command" {
		t.Errorf("Type = %q, want %q", mock.Type, "command")
	}
	if mock.Command == nil {
		t.Error("Command should not be nil")
	}
	if mock.File == nil {
		t.Error("File should not be nil")
	}
	if mock.HTTP == nil {
		t.Error("HTTP should not be nil")
	}
	if mock.Package == nil {
		t.Error("Package should not be nil")
	}
	if mock.Service == nil {
		t.Error("Service should not be nil")
	}
}

func TestCommandMock_Structure(t *testing.T) {
	cm := CommandMock{
		Pattern:  "ls.*",
		Stdout:   "file1 file2",
		Stderr:   "",
		ExitCode: 0,
	}

	if cm.Pattern != "ls.*" {
		t.Errorf("Pattern = %q, want %q", cm.Pattern, "ls.*")
	}
	if cm.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", cm.ExitCode)
	}
}

func TestFileMock_Structure(t *testing.T) {
	fm := FileMock{
		Path:    "/etc/test.conf",
		Content: "content",
		Mode:    "0644",
		Owner:   "root",
		Group:   "root",
		Exists:  true,
		IsDir:   false,
	}

	if fm.Path != "/etc/test.conf" {
		t.Errorf("Path = %q, want %q", fm.Path, "/etc/test.conf")
	}
	if !fm.Exists {
		t.Error("Exists should be true")
	}
}

func TestHTTPMock_Structure(t *testing.T) {
	hm := HTTPMock{
		URL:        "https://api.test.com/.*",
		StatusCode: 201,
		Body:       `{"id": 123}`,
		Headers:    map[string]string{"Content-Type": "application/json"},
	}

	if hm.URL != "https://api.test.com/.*" {
		t.Errorf("URL = %q, want %q", hm.URL, "https://api.test.com/.*")
	}
	if hm.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", hm.StatusCode)
	}
}

func TestPackageMock_Structure(t *testing.T) {
	pm := PackageMock{
		Name:              "nginx",
		Installed:        true,
		Version:          "1.18.0",
		AvailableVersions: []string{"1.18.0", "1.19.0"},
	}

	if pm.Name != "nginx" {
		t.Errorf("Name = %q, want %q", pm.Name, "nginx")
	}
	if !pm.Installed {
		t.Error("Installed should be true")
	}
	if len(pm.AvailableVersions) != 2 {
		t.Errorf("len(AvailableVersions) = %d, want 2", len(pm.AvailableVersions))
	}
}

func TestServiceMock_Structure(t *testing.T) {
	sm := ServiceMock{
		Name:    "nginx",
		Running: true,
		Enabled: true,
	}

	if sm.Name != "nginx" {
		t.Errorf("Name = %q, want %q", sm.Name, "nginx")
	}
	if !sm.Running {
		t.Error("Running should be true")
	}
	if !sm.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestRunnerConfig_Structure(t *testing.T) {
	registry := NewMockRegistry()
	config := RunnerConfig{
		BlueprintPath: "/path/to/blueprint",
		DryRun:        true,
		Timeout:       5 * time.Minute,
		Parallel:      true,
		MaxParallel:   4,
		StopOnFailure: true,
		Verbose:       true,
		MockRegistry:  registry,
	}

	if config.BlueprintPath != "/path/to/blueprint" {
		t.Errorf("BlueprintPath = %q, want %q", config.BlueprintPath, "/path/to/blueprint")
	}
	if !config.DryRun {
		t.Error("DryRun should be true")
	}
	if config.MaxParallel != 4 {
		t.Errorf("MaxParallel = %d, want 4", config.MaxParallel)
	}
	if config.MockRegistry != registry {
		t.Error("MockRegistry should match")
	}
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	// Test parsing various duration strings
	testCases := []struct {
		input    string
		expected time.Duration
	}{
		{"1s", time.Second},
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h", time.Hour},
		{"1h30m", time.Hour + 30*time.Minute},
		{"", 0},
	}

	for _, tc := range testCases {
		var d Duration
		err := d.UnmarshalYAML(func(v interface{}) error {
			*(v.(*string)) = tc.input
			return nil
		})
		if err != nil {
			t.Errorf("UnmarshalYAML(%q) error: %v", tc.input, err)
			continue
		}
		if d.Duration() != tc.expected {
			t.Errorf("UnmarshalYAML(%q) = %v, want %v", tc.input, d.Duration(), tc.expected)
		}
	}
}

func TestDuration_MarshalYAML(t *testing.T) {
	d := Duration(30 * time.Second)
	v, err := d.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML error: %v", err)
	}
	if v != "30s" {
		t.Errorf("MarshalYAML() = %q, want %q", v, "30s")
	}

	// Zero duration
	d = Duration(0)
	v, err = d.MarshalYAML()
	if err != nil {
		t.Fatalf("MarshalYAML error: %v", err)
	}
	if v != "" {
		t.Errorf("MarshalYAML(0) = %q, want empty string", v)
	}
}

func TestComparisonOperator_Values(t *testing.T) {
	operators := map[ComparisonOperator]string{
		OpEquals:      "equals",
		OpNotEquals:   "not_equals",
		OpContains:    "contains",
		OpNotContains: "not_contains",
		OpMatches:     "matches",
		OpGreaterThan: "greater_than",
		OpLessThan:    "less_than",
		OpGreaterOrEq: "greater_or_equal",
		OpLessOrEq:    "less_or_equal",
	}

	for op, expected := range operators {
		if string(op) != expected {
			t.Errorf("Operator %v = %q, want %q", op, string(op), expected)
		}
	}
}

func TestMockType_Values(t *testing.T) {
	mockTypes := map[MockType]string{
		MockTypeCommand: "command",
		MockTypeFile:    "file",
		MockTypeHTTP:    "http",
		MockTypePackage: "package",
		MockTypeService: "service",
	}

	for mt, expected := range mockTypes {
		if string(mt) != expected {
			t.Errorf("MockType %v = %q, want %q", mt, string(mt), expected)
		}
	}
}
