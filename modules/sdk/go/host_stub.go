//go:build !tinygo.wasm

package kscoresdk

// Stub implementations for non-WASM builds (testing)

// ReadFile reads a file (stubbed)
func ReadFile(path string) ([]byte, error) {
	return nil, NewFileSystemError("not running in WASM environment")
}

// ReadString reads a file as string (stubbed)
func ReadString(path string) (string, error) {
	return "", NewFileSystemError("not running in WASM environment")
}

// WriteFile writes data to a file (stubbed)
func WriteFile(path string, data []byte) error {
	return NewFileSystemError("not running in WASM environment")
}

// WriteString writes a string to a file (stubbed)
func WriteString(path string, data string) error {
	return NewFileSystemError("not running in WASM environment")
}

// HTTPGet performs an HTTP GET request (stubbed)
func HTTPGet(url string) (*HttpResponse, error) {
	return nil, NewHTTPError("not running in WASM environment")
}

// HTTPPost performs an HTTP POST request (stubbed)
func HTTPPost(url string, body []byte) (*HttpResponse, error) {
	return nil, NewHTTPError("not running in WASM environment")
}

// Exec executes a command (stubbed)
func Exec(command string, args ...string) (*ExecResult, error) {
	return nil, NewExecError("not running in WASM environment")
}

// ExecWithInput executes a command with stdin (stubbed)
func ExecWithInput(command string, stdin string, args ...string) (*ExecResult, error) {
	return nil, NewExecError("not running in WASM environment")
}

// LogDebug logs a debug message (stubbed)
func LogDebug(message string) {}

// LogInfo logs an info message (stubbed)
func LogInfo(message string) {}

// LogWarn logs a warning message (stubbed)
func LogWarn(message string) {}

// LogError logs an error message (stubbed)
func LogError(message string) {}

// KvGet gets a value from key-value storage (stubbed)
func KvGet(key string) (string, bool, error) {
	return "", false, NewError("not running in WASM environment")
}

// KvSet sets a value in key-value storage (stubbed)
func KvSet(key string, value string) error {
	return NewError("not running in WASM environment")
}

// GetCPUInfo returns CPU information (stubbed)
func GetCPUInfo() (string, error) {
	return "", NewError("not running in WASM environment")
}

// SHA256 computes SHA256 hash (stubbed)
func SHA256(data []byte) (string, error) {
	return "", NewError("not running in WASM environment")
}

// SHA256String computes SHA256 hash of string (stubbed)
func SHA256String(s string) (string, error) {
	return "", NewError("not running in WASM environment")
}
