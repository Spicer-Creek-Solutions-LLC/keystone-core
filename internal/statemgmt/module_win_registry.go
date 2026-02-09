// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package statemgmt

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// WinRegistryModule implements Windows registry management using native APIs
type WinRegistryModule struct {
	*BaseModule
}

// NewWinRegistryModule creates a new Windows registry module
func NewWinRegistryModule() *WinRegistryModule {
	return &WinRegistryModule{
		BaseModule: NewBaseModule("win_registry", []string{
			"present", "absent",
		}),
	}
}

// Registry root key constants
var rootKeys = map[string]registry.Key{
	"HKEY_CLASSES_ROOT":   registry.CLASSES_ROOT,
	"HKCR":                registry.CLASSES_ROOT,
	"HKEY_CURRENT_USER":   registry.CURRENT_USER,
	"HKCU":                registry.CURRENT_USER,
	"HKEY_LOCAL_MACHINE":  registry.LOCAL_MACHINE,
	"HKLM":                registry.LOCAL_MACHINE,
	"HKEY_USERS":          registry.USERS,
	"HKU":                 registry.USERS,
	"HKEY_CURRENT_CONFIG": registry.CURRENT_CONFIG,
	"HKCC":                registry.CURRENT_CONFIG,
}

// Registry value type constants
const (
	RegNone     = registry.NONE
	RegSZ       = registry.SZ
	RegExpandSZ = registry.EXPAND_SZ
	RegBinary   = registry.BINARY
	RegDWORD    = registry.DWORD
	RegQWORD    = registry.QWORD
	RegMultiSZ  = registry.MULTI_SZ
)

// Check checks the current state of a Windows registry key/value
func (m *WinRegistryModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	// Get registry path and value name
	keyPath := getStringParameter(decl, "key", decl.ID)
	valueName := getStringParameter(decl, "name", "")

	// Parse the key path
	rootKey, subKeyPath, err := parseKeyPath(keyPath)
	if err != nil {
		return nil, err
	}

	result.Metadata["root_key"] = keyPath[:strings.Index(keyPath, "\\")]
	result.Metadata["sub_key"] = subKeyPath
	result.Metadata["value_name"] = valueName

	// Try to open the key
	key, err := registry.OpenKey(rootKey, subKeyPath, registry.QUERY_VALUE|registry.READ)
	if err != nil {
		// Key doesn't exist
		result.Present = false
		result.CurrentState = "absent"

		if decl.State == "absent" {
			result.Matches = true
		} else {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "absent", "desired": "present"}
		}
		return result, nil
	}
	defer key.Close()

	// Key exists
	result.Present = true
	result.CurrentState = "present"

	// If no value name specified, we're just checking the key exists
	if valueName == "" {
		if decl.State == "present" {
			result.Matches = true
		} else {
			result.Matches = false
			result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		}
		return result, nil
	}

	// Check the value - first get the size, then the data
	size, valType, err := key.GetValue(valueName, nil)
	if err != nil {
		// Value doesn't exist
		result.Metadata["value_exists"] = false

		if decl.State == "absent" {
			result.Matches = true
		} else {
			result.Matches = false
			result.Diff["value"] = map[string]string{"current": "absent", "desired": "present"}
		}
		return result, nil
	}

	// Now get the actual data
	valData := make([]byte, size)
	_, _, err = key.GetValue(valueName, valData)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry value data: %w", err)
	}

	result.Metadata["value_exists"] = true
	result.Metadata["value_type"] = regTypeToString(valType)
	result.Metadata["value_data"] = formatValueData(valData, valType)

	// Compare with desired state
	if decl.State == "absent" {
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": "present", "desired": "absent"}
		return result, nil
	}

	// Check if value matches
	desiredType := getStringParameter(decl, "type", "REG_SZ")
	desiredData := getParameter(decl, "data", nil)

	// Check type match
	desiredTypeInt := parseRegType(desiredType)
	if valType != desiredTypeInt {
		result.Matches = false
		result.Diff["type"] = map[string]string{
			"current": regTypeToString(valType),
			"desired": desiredType,
		}
	}

	// Check data match
	if desiredData != nil {
		currentData := formatValueData(valData, valType)
		desiredDataStr := formatDesiredData(desiredData, desiredTypeInt)
		if currentData != desiredDataStr {
			result.Matches = false
			result.Diff["data"] = map[string]interface{}{
				"current": currentData,
				"desired": desiredDataStr,
			}
		}
	}

	if len(result.Diff) == 0 {
		result.Matches = true
	}

	return result, nil
}

// Apply applies the Windows registry state
func (m *WinRegistryModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	keyPath := getStringParameter(decl, "key", decl.ID)
	valueName := getStringParameter(decl, "name", "")

	rootKey, subKeyPath, err := parseKeyPath(keyPath)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Invalid registry key path: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	var applyErr error
	switch decl.State {
	case "present":
		applyErr = m.applyPresent(rootKey, subKeyPath, valueName, decl, result)
	case "absent":
		applyErr = m.applyAbsent(rootKey, subKeyPath, valueName, decl, result)
	default:
		applyErr = fmt.Errorf("unsupported state: %s", decl.State)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// Test tests if the registry key/value is in the desired state
func (m *WinRegistryModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// applyPresent ensures the registry key/value exists with the desired value
func (m *WinRegistryModule) applyPresent(rootKey registry.Key, subKeyPath, valueName string, decl *StateDeclaration, result *StateResult) error {
	// Create or open the key
	key, _, err := registry.CreateKey(rootKey, subKeyPath, registry.ALL_ACCESS)
	if err != nil {
		return fmt.Errorf("failed to create/open registry key: %w", err)
	}
	defer key.Close()

	// If no value name specified, we're done (key created)
	if valueName == "" {
		result.Comment = fmt.Sprintf("Registry key %s created", subKeyPath)
		return nil
	}

	// Set the value
	valueType := parseRegType(getStringParameter(decl, "type", "REG_SZ"))
	data := getParameter(decl, "data", "")

	switch valueType {
	case registry.SZ, registry.EXPAND_SZ:
		strData, ok := data.(string)
		if !ok {
			strData = fmt.Sprintf("%v", data)
		}
		if valueType == registry.SZ {
			err = key.SetStringValue(valueName, strData)
		} else {
			err = key.SetExpandStringValue(valueName, strData)
		}

	case registry.DWORD:
		var dwordData uint32
		switch v := data.(type) {
		case int:
			dwordData = uint32(v)
		case int64:
			dwordData = uint32(v)
		case float64:
			dwordData = uint32(v)
		case string:
			parsed, parseErr := strconv.ParseUint(v, 10, 32)
			if parseErr != nil {
				return fmt.Errorf("invalid DWORD value: %s", v)
			}
			dwordData = uint32(parsed)
		default:
			return fmt.Errorf("unsupported data type for DWORD: %T", data)
		}
		err = key.SetDWordValue(valueName, dwordData)

	case registry.QWORD:
		var qwordData uint64
		switch v := data.(type) {
		case int:
			qwordData = uint64(v)
		case int64:
			qwordData = uint64(v)
		case float64:
			qwordData = uint64(v)
		case string:
			qwordData, err = strconv.ParseUint(v, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid QWORD value: %s", v)
			}
		default:
			return fmt.Errorf("unsupported data type for QWORD: %T", data)
		}
		err = key.SetQWordValue(valueName, qwordData)

	case registry.BINARY:
		var binData []byte
		switch v := data.(type) {
		case string:
			// Expect hex-encoded string
			binData, err = hex.DecodeString(v)
			if err != nil {
				return fmt.Errorf("invalid binary data (expected hex string): %w", err)
			}
		case []byte:
			binData = v
		default:
			return fmt.Errorf("unsupported data type for BINARY: %T", data)
		}
		err = key.SetBinaryValue(valueName, binData)

	case registry.MULTI_SZ:
		var multiData []string
		switch v := data.(type) {
		case []string:
			multiData = v
		case []interface{}:
			multiData = make([]string, len(v))
			for i, item := range v {
				multiData[i] = fmt.Sprintf("%v", item)
			}
		default:
			return fmt.Errorf("unsupported data type for MULTI_SZ: %T", data)
		}
		err = key.SetStringsValue(valueName, multiData)

	default:
		return fmt.Errorf("unsupported registry value type: %d", valueType)
	}

	if err != nil {
		return fmt.Errorf("failed to set registry value: %w", err)
	}

	result.Comment = fmt.Sprintf("Registry value %s\\%s set", subKeyPath, valueName)
	return nil
}

// applyAbsent removes the registry key or value
func (m *WinRegistryModule) applyAbsent(rootKey registry.Key, subKeyPath, valueName string, decl *StateDeclaration, result *StateResult) error {
	// If no value name, delete the entire key
	if valueName == "" {
		err := registry.DeleteKey(rootKey, subKeyPath)
		if err != nil {
			// Check if key already doesn't exist
			if err == registry.ErrNotExist {
				result.Comment = fmt.Sprintf("Registry key %s already absent", subKeyPath)
				return nil
			}
			return fmt.Errorf("failed to delete registry key: %w", err)
		}
		result.Comment = fmt.Sprintf("Registry key %s deleted", subKeyPath)
		return nil
	}

	// Delete the value
	key, err := registry.OpenKey(rootKey, subKeyPath, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			result.Comment = fmt.Sprintf("Registry key %s already absent", subKeyPath)
			return nil
		}
		return fmt.Errorf("failed to open registry key: %w", err)
	}
	defer key.Close()

	err = key.DeleteValue(valueName)
	if err != nil {
		if err == registry.ErrNotExist {
			result.Comment = fmt.Sprintf("Registry value %s\\%s already absent", subKeyPath, valueName)
			return nil
		}
		return fmt.Errorf("failed to delete registry value: %w", err)
	}

	result.Comment = fmt.Sprintf("Registry value %s\\%s deleted", subKeyPath, valueName)
	return nil
}

// Helper functions

func parseKeyPath(path string) (registry.Key, string, error) {
	parts := strings.SplitN(path, "\\", 2)
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("invalid registry key path: %s (expected format: HKLM\\path\\to\\key)", path)
	}

	rootKey, ok := rootKeys[strings.ToUpper(parts[0])]
	if !ok {
		return 0, "", fmt.Errorf("unknown registry root key: %s", parts[0])
	}

	return rootKey, parts[1], nil
}

func regTypeToString(regType uint32) string {
	switch regType {
	case registry.NONE:
		return "REG_NONE"
	case registry.SZ:
		return "REG_SZ"
	case registry.EXPAND_SZ:
		return "REG_EXPAND_SZ"
	case registry.BINARY:
		return "REG_BINARY"
	case registry.DWORD:
		return "REG_DWORD"
	case registry.QWORD:
		return "REG_QWORD"
	case registry.MULTI_SZ:
		return "REG_MULTI_SZ"
	default:
		return fmt.Sprintf("REG_UNKNOWN(%d)", regType)
	}
}

func parseRegType(typeStr string) uint32 {
	switch strings.ToUpper(typeStr) {
	case "REG_NONE":
		return registry.NONE
	case "REG_SZ", "STRING":
		return registry.SZ
	case "REG_EXPAND_SZ", "EXPANDSTRING":
		return registry.EXPAND_SZ
	case "REG_BINARY", "BINARY":
		return registry.BINARY
	case "REG_DWORD", "DWORD":
		return registry.DWORD
	case "REG_QWORD", "QWORD":
		return registry.QWORD
	case "REG_MULTI_SZ", "MULTISTRING":
		return registry.MULTI_SZ
	default:
		return registry.SZ // Default to REG_SZ
	}
}

func formatValueData(data []byte, regType uint32) string {
	switch regType {
	case registry.SZ, registry.EXPAND_SZ:
		// Remove null terminator if present
		if len(data) >= 2 && data[len(data)-1] == 0 && data[len(data)-2] == 0 {
			data = data[:len(data)-2]
		}
		// Convert UTF-16LE to string
		return utf16ToString(data)

	case registry.DWORD:
		if len(data) >= 4 {
			val := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
			return strconv.FormatUint(uint64(val), 10)
		}
		return "0"

	case registry.QWORD:
		if len(data) >= 8 {
			val := uint64(data[0]) | uint64(data[1])<<8 | uint64(data[2])<<16 | uint64(data[3])<<24 |
				uint64(data[4])<<32 | uint64(data[5])<<40 | uint64(data[6])<<48 | uint64(data[7])<<56
			return strconv.FormatUint(val, 10)
		}
		return "0"

	case registry.BINARY:
		return hex.EncodeToString(data)

	case registry.MULTI_SZ:
		// Multi-string is null-separated, double-null terminated
		return utf16MultiStringToString(data)

	default:
		return hex.EncodeToString(data)
	}
}

func formatDesiredData(data interface{}, regType uint32) string {
	switch regType {
	case registry.SZ, registry.EXPAND_SZ:
		return fmt.Sprintf("%v", data)
	case registry.DWORD, registry.QWORD:
		return fmt.Sprintf("%v", data)
	case registry.BINARY:
		if str, ok := data.(string); ok {
			return str
		}
		if bytes, ok := data.([]byte); ok {
			return hex.EncodeToString(bytes)
		}
		return fmt.Sprintf("%v", data)
	case registry.MULTI_SZ:
		if strs, ok := data.([]string); ok {
			return strings.Join(strs, "\x00")
		}
		return fmt.Sprintf("%v", data)
	default:
		return fmt.Sprintf("%v", data)
	}
}

func utf16ToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	u16s := make([]uint16, len(b)/2)
	for i := range u16s {
		u16s[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	// Remove null terminator
	for len(u16s) > 0 && u16s[len(u16s)-1] == 0 {
		u16s = u16s[:len(u16s)-1]
	}
	runes := make([]rune, len(u16s))
	for i, u := range u16s {
		runes[i] = rune(u)
	}
	return string(runes)
}

func utf16MultiStringToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	u16s := make([]uint16, len(b)/2)
	for i := range u16s {
		u16s[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}

	var result []string
	var current []rune
	for _, u := range u16s {
		if u == 0 {
			if len(current) > 0 {
				result = append(result, string(current))
				current = nil
			}
		} else {
			current = append(current, rune(u))
		}
	}
	return strings.Join(result, "\x00")
}

func init() {
	RegisterModule(NewWinRegistryModule())
}
