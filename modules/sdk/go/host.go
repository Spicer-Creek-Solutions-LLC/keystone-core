//go:build tinygo.wasm

package titansdk

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"unsafe"
)

// Host function imports from WASM runtime
// These are provided by the TitanAnvil runtime

//go:wasm-module env
//export host_fs_read
func hostFsRead(pathPtr, pathLen uint32, outPtr, outLenPtr uint32) int32

//go:wasm-module env
//export host_fs_write
func hostFsWrite(pathPtr, pathLen, dataPtr, dataLen uint32) int32

//go:wasm-module env
//export host_http_get
func hostHttpGet(urlPtr, urlLen, outPtr, outLenPtr uint32) int32

//go:wasm-module env
//export host_http_post
func hostHttpPost(urlPtr, urlLen, bodyPtr, bodyLen, outPtr, outLenPtr uint32) int32

//go:wasm-module env
//export host_exec
func hostExec(cmdPtr, cmdLen, outPtr, outLenPtr uint32) int32

//go:wasm-module env
//export host_log
func hostLog(level int32, msgPtr, msgLen uint32)

//go:wasm-module env
//export host_kv_get
func hostKvGet(keyPtr, keyLen, outPtr, outLenPtr uint32) int32

//go:wasm-module env
//export host_kv_set
func hostKvSet(keyPtr, keyLen, valPtr, valLen uint32) int32

// Helper functions for memory passing

func stringToPtr(s string) (uint32, uint32) {
	ptr := unsafe.Pointer(unsafe.StringData(s))
	return uint32(uintptr(ptr)), uint32(len(s))
}

func bytesToPtr(b []byte) (uint32, uint32) {
	if len(b) == 0 {
		return 0, 0
	}
	ptr := unsafe.Pointer(&b[0])
	return uint32(uintptr(ptr)), uint32(len(b))
}

func ptrToBytes(ptr uint32, length uint32) []byte {
	if length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}

// Filesystem operations

// ReadFile reads a file and returns its contents
func ReadFile(path string) ([]byte, error) {
	pathPtr, pathLen := stringToPtr(path)

	// Allocate output buffer (1MB)
	outBuf := make([]byte, 1024*1024)
	outPtr, _ := bytesToPtr(outBuf)
	outLen := uint32(len(outBuf))

	result := hostFsRead(pathPtr, pathLen, outPtr, uint32(uintptr(unsafe.Pointer(&outLen))))

	if result == 0 {
		return outBuf[:outLen], nil
	}
	return nil, NewFileSystemError("failed to read file")
}

// ReadString reads a file and returns its contents as a string
func ReadString(path string) (string, error) {
	data, err := ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes data to a file
func WriteFile(path string, data []byte) error {
	pathPtr, pathLen := stringToPtr(path)
	dataPtr, dataLen := bytesToPtr(data)

	result := hostFsWrite(pathPtr, pathLen, dataPtr, dataLen)

	if result == 0 {
		return nil
	}
	return NewFileSystemError("failed to write file")
}

// WriteString writes a string to a file
func WriteString(path string, data string) error {
	return WriteFile(path, []byte(data))
}

// HTTP operations

// HTTPGet performs an HTTP GET request
func HTTPGet(url string) (*HttpResponse, error) {
	urlPtr, urlLen := stringToPtr(url)

	// Allocate output buffer (10MB)
	outBuf := make([]byte, 10*1024*1024)
	outPtr, _ := bytesToPtr(outBuf)
	outLen := uint32(len(outBuf))

	result := hostHttpGet(urlPtr, urlLen, outPtr, uint32(uintptr(unsafe.Pointer(&outLen))))

	if result == 0 {
		var response HttpResponse
		if err := json.Unmarshal(outBuf[:outLen], &response); err != nil {
			return nil, NewHTTPError(fmt.Sprintf("failed to parse response: %v", err))
		}
		return &response, nil
	}
	return nil, NewHTTPError("GET request failed")
}

// HTTPPost performs an HTTP POST request
func HTTPPost(url string, body []byte) (*HttpResponse, error) {
	urlPtr, urlLen := stringToPtr(url)
	bodyPtr, bodyLen := bytesToPtr(body)

	// Allocate output buffer (10MB)
	outBuf := make([]byte, 10*1024*1024)
	outPtr, _ := bytesToPtr(outBuf)
	outLen := uint32(len(outBuf))

	result := hostHttpPost(urlPtr, urlLen, bodyPtr, bodyLen, outPtr, uint32(uintptr(unsafe.Pointer(&outLen))))

	if result == 0 {
		var response HttpResponse
		if err := json.Unmarshal(outBuf[:outLen], &response); err != nil {
			return nil, NewHTTPError(fmt.Sprintf("failed to parse response: %v", err))
		}
		return &response, nil
	}
	return nil, NewHTTPError("POST request failed")
}

// Command execution

// Exec executes a command and returns the result
func Exec(command string, args ...string) (*ExecResult, error) {
	request := ExecRequest{
		Command: command,
		Args:    args,
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, NewExecError(fmt.Sprintf("failed to serialize request: %v", err))
	}

	cmdPtr, cmdLen := bytesToPtr(requestJSON)

	// Allocate output buffer (10MB)
	outBuf := make([]byte, 10*1024*1024)
	outPtr, _ := bytesToPtr(outBuf)
	outLen := uint32(len(outBuf))

	result := hostExec(cmdPtr, cmdLen, outPtr, uint32(uintptr(unsafe.Pointer(&outLen))))

	if result == 0 {
		var execResult ExecResult
		if err := json.Unmarshal(outBuf[:outLen], &execResult); err != nil {
			return nil, NewExecError(fmt.Sprintf("failed to parse result: %v", err))
		}
		return &execResult, nil
	}
	return nil, NewExecError("command execution failed")
}

// ExecWithInput executes a command with stdin and returns the result
func ExecWithInput(command string, stdin string, args ...string) (*ExecResult, error) {
	request := ExecRequest{
		Command: command,
		Args:    args,
		Stdin:   stdin,
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, NewExecError(fmt.Sprintf("failed to serialize request: %v", err))
	}

	cmdPtr, cmdLen := bytesToPtr(requestJSON)

	// Allocate output buffer (10MB)
	outBuf := make([]byte, 10*1024*1024)
	outPtr, _ := bytesToPtr(outBuf)
	outLen := uint32(len(outBuf))

	result := hostExec(cmdPtr, cmdLen, outPtr, uint32(uintptr(unsafe.Pointer(&outLen))))

	if result == 0 {
		var execResult ExecResult
		if err := json.Unmarshal(outBuf[:outLen], &execResult); err != nil {
			return nil, NewExecError(fmt.Sprintf("failed to parse result: %v", err))
		}
		return &execResult, nil
	}
	return nil, NewExecError("command execution failed")
}

// Logging

// LogDebug logs a debug message
func LogDebug(message string) {
	msgPtr, msgLen := stringToPtr(message)
	hostLog(int32(LogLevelDebug), msgPtr, msgLen)
}

// LogInfo logs an info message
func LogInfo(message string) {
	msgPtr, msgLen := stringToPtr(message)
	hostLog(int32(LogLevelInfo), msgPtr, msgLen)
}

// LogWarn logs a warning message
func LogWarn(message string) {
	msgPtr, msgLen := stringToPtr(message)
	hostLog(int32(LogLevelWarn), msgPtr, msgLen)
}

// LogError logs an error message
func LogError(message string) {
	msgPtr, msgLen := stringToPtr(message)
	hostLog(int32(LogLevelError), msgPtr, msgLen)
}

// Key-value storage

// KvGet gets a value from key-value storage
func KvGet(key string) (string, bool, error) {
	keyPtr, keyLen := stringToPtr(key)

	// Allocate output buffer (1MB)
	outBuf := make([]byte, 1024*1024)
	outPtr, _ := bytesToPtr(outBuf)
	outLen := uint32(len(outBuf))

	result := hostKvGet(keyPtr, keyLen, outPtr, uint32(uintptr(unsafe.Pointer(&outLen))))

	if result == 0 {
		if outLen == 0 {
			return "", false, nil
		}
		return string(outBuf[:outLen]), true, nil
	}
	return "", false, NewError("kv get failed")
}

// KvSet sets a value in key-value storage
func KvSet(key string, value string) error {
	keyPtr, keyLen := stringToPtr(key)
	valPtr, valLen := stringToPtr(value)

	result := hostKvSet(keyPtr, keyLen, valPtr, valLen)

	if result == 0 {
		return nil
	}
	return NewError("kv set failed")
}

// System information

// GetCPUInfo returns CPU information
func GetCPUInfo() (string, error) {
	// Try different methods based on OS
	switch runtime.GOOS {
	case "linux":
		data, err := ReadString("/proc/cpuinfo")
		if err == nil {
			// Parse model name from /proc/cpuinfo
			lines := splitLines(data)
			for _, line := range lines {
				if len(line) > 10 && line[:10] == "model name" {
					parts := splitString(line, ":")
					if len(parts) > 1 {
						return trimSpace(parts[1]), nil
					}
				}
			}
		}

	case "darwin":
		result, err := Exec("sysctl", "-n", "machdep.cpu.brand_string")
		if err == nil && result.ExitCode == 0 {
			return trimSpace(result.Stdout), nil
		}

	case "windows":
		result, err := Exec("wmic", "cpu", "get", "name")
		if err == nil && result.ExitCode == 0 {
			lines := splitLines(result.Stdout)
			if len(lines) > 1 {
				return trimSpace(lines[1]), nil
			}
		}
	}

	return "", NewError("failed to get CPU info")
}

// Cryptography

// SHA256 computes the SHA256 hash of data
func SHA256(data []byte) (string, error) {
	// Write to temp file
	tempFile := "/tmp/titan-hash-input"
	if err := WriteFile(tempFile, data); err != nil {
		return "", err
	}

	var result *ExecResult
	var err error

	if runtime.GOOS != "windows" {
		result, err = Exec("sha256sum", tempFile)
	} else {
		result, err = Exec("certutil", "-hashfile", tempFile, "SHA256")
	}

	if err != nil {
		return "", err
	}

	if result.ExitCode == 0 {
		// Parse hash from output
		parts := splitWhitespace(result.Stdout)
		if len(parts) > 0 {
			return parts[0], nil
		}
	}

	return "", NewError("failed to compute hash")
}

// SHA256String computes the SHA256 hash of a string
func SHA256String(s string) (string, error) {
	return SHA256([]byte(s))
}

// String utility functions (simple implementations to avoid imports)

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func splitString(s string, sep string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func splitWhitespace(s string) []string {
	var parts []string
	inWord := false
	start := 0

	for i := 0; i < len(s); i++ {
		isSpace := s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r'
		if !isSpace && !inWord {
			start = i
			inWord = true
		} else if isSpace && inWord {
			parts = append(parts, s[start:i])
			inWord = false
		}
	}

	if inWord {
		parts = append(parts, s[start:])
	}

	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}
