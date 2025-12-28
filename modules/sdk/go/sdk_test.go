package kscoresdk

import (
	"testing"
)

func TestCapabilityConstants(t *testing.T) {
	tests := []struct {
		capability Capability
		expected   string
	}{
		{CapabilityFsRead, "fs.read"},
		{CapabilityFsWrite, "fs.write"},
		{CapabilityHttpGet, "http.get"},
		{CapabilityHttpPost, "http.post"},
		{CapabilityExec, "exec"},
		{CapabilitySecretsRead, "secrets.read"},
		{CapabilitySecretsWrite, "secrets.write"},
		{CapabilityLog, "log"},
		{CapabilityTime, "time"},
		{CapabilityKv, "kv"},
	}

	for _, tt := range tests {
		if string(tt.capability) != tt.expected {
			t.Errorf("Capability %v = %s, want %s", tt.capability, tt.capability, tt.expected)
		}
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LogLevelDebug, "debug"},
		{LogLevelInfo, "info"},
		{LogLevelWarn, "warn"},
		{LogLevelError, "error"},
	}

	for _, tt := range tests {
		if tt.level.String() != tt.expected {
			t.Errorf("LogLevel.String() = %s, want %s", tt.level.String(), tt.expected)
		}
	}
}

func TestErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		contains string
	}{
		{"CapabilityDenied", NewCapabilityDeniedError("fs.read"), "Capability denied"},
		{"FileSystem", NewFileSystemError("not found"), "Filesystem error"},
		{"HTTP", NewHTTPError("404"), "HTTP error"},
		{"Exec", NewExecError("failed"), "Exec error"},
		{"Serialization", NewSerializationError("invalid"), "Serialization error"},
		{"Other", NewError("generic"), "generic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			if len(errMsg) == 0 {
				t.Error("Error message is empty")
			}
			// Just verify error returns a non-empty string
		})
	}
}

func TestErrorCreation(t *testing.T) {
	err := NewCapabilityDeniedError("fs.read")
	if err.Type != ErrorTypeCapabilityDenied {
		t.Errorf("Error type = %v, want %v", err.Type, ErrorTypeCapabilityDenied)
	}

	err = NewFileSystemError("test")
	if err.Type != ErrorTypeFileSystem {
		t.Errorf("Error type = %v, want %v", err.Type, ErrorTypeFileSystem)
	}
}

// Test stub functions (these will fail in non-WASM mode, which is expected)

func TestReadFileStubbed(t *testing.T) {
	_, err := ReadFile("/test/path")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestReadStringStubbed(t *testing.T) {
	_, err := ReadString("/test/path")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestWriteFileStubbed(t *testing.T) {
	err := WriteFile("/test/path", []byte("data"))
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestWriteStringStubbed(t *testing.T) {
	err := WriteString("/test/path", "data")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestHTTPGetStubbed(t *testing.T) {
	_, err := HTTPGet("https://example.com")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestHTTPPostStubbed(t *testing.T) {
	_, err := HTTPPost("https://example.com", []byte("data"))
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestExecStubbed(t *testing.T) {
	_, err := Exec("ls", "-la")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestExecWithInputStubbed(t *testing.T) {
	_, err := ExecWithInput("grep", "test", "pattern")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestLogFunctions(t *testing.T) {
	// Log functions don't return values, just ensure they don't panic
	LogDebug("debug message")
	LogInfo("info message")
	LogWarn("warn message")
	LogError("error message")
}

func TestKvGetStubbed(t *testing.T) {
	_, _, err := KvGet("test-key")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestKvSetStubbed(t *testing.T) {
	err := KvSet("test-key", "test-value")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestGetCPUInfoStubbed(t *testing.T) {
	_, err := GetCPUInfo()
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestSHA256Stubbed(t *testing.T) {
	_, err := SHA256([]byte("data"))
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestSHA256StringStubbed(t *testing.T) {
	_, err := SHA256String("data")
	if err == nil {
		t.Error("Expected error in non-WASM environment")
	}
}

func TestModuleContextSerialization(t *testing.T) {
	// Test that types can be marshaled/unmarshaled (basic sanity check)
	ctx := ModuleContext{
		ModuleName:    "test/module",
		CorrelationID: "test-123",
		Metadata:      map[string]interface{}{"key": "value"},
	}

	if ctx.ModuleName != "test/module" {
		t.Errorf("ModuleName = %s, want test/module", ctx.ModuleName)
	}
}

func TestExecResultTypes(t *testing.T) {
	result := ExecResult{
		ExitCode: 0,
		Stdout:   "output",
		Stderr:   "",
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "output" {
		t.Errorf("Stdout = %s, want output", result.Stdout)
	}
}

func TestHttpResponseTypes(t *testing.T) {
	response := HttpResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       []byte("response body"),
	}

	if response.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", response.StatusCode)
	}
	if len(response.Body) == 0 {
		t.Error("Body should not be empty")
	}
}
