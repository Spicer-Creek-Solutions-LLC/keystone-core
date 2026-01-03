// Package statemgmt provides state management modules.
package statemgmt

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// SwapModule manages swap space.
type SwapModule struct {
	*BaseModule
}

// SwapConfig holds swap configuration.
type SwapConfig struct {
	// Path is the swap file or partition path
	Path string
	// Size is the swap size (e.g., "1G", "512M", "2048" for MiB)
	Size string
	// Priority is the swap priority (-1 to 32767)
	Priority int
	// Persist if true, adds entry to /etc/fstab
	Persist bool
	// Label is the swap partition label
	Label string
	// UUID is the swap partition UUID
	UUID string
}

// NewSwapModule creates a new swap module.
func NewSwapModule() *SwapModule {
	return &SwapModule{
		BaseModule: NewBaseModule("swap", []string{"enabled", "disabled", "present", "absent"}),
	}
}

// Check determines if the swap is in the desired state.
func (m *SwapModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("swap module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	isActive := m.isSwapActive(config.Path)
	exists := m.swapExists(config.Path)
	inFstab := m.isInFstab(config.Path)

	result.Metadata["is_active"] = isActive
	result.Metadata["exists"] = exists
	result.Metadata["in_fstab"] = inFstab

	switch decl.State {
	case "enabled":
		// enabled: swap is active and in fstab (if persist)
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["exists"] = map[string]interface{}{"current": false, "desired": true}
		} else if !isActive {
			result.Present = true
			result.CurrentState = "disabled"
			result.Matches = false
			result.Diff["active"] = map[string]interface{}{"current": false, "desired": true}
		} else if config.Persist && !inFstab {
			result.Present = true
			result.CurrentState = "enabled (not persistent)"
			result.Matches = false
			result.Diff["persistent"] = map[string]interface{}{"current": false, "desired": true}
		} else {
			result.Present = true
			result.CurrentState = "enabled"
			result.Matches = true
		}

	case "disabled":
		// disabled: swap exists but is not active
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["exists"] = map[string]interface{}{"current": false, "desired": true}
		} else if isActive {
			result.Present = true
			result.CurrentState = "enabled"
			result.Matches = false
			result.Diff["active"] = map[string]interface{}{"current": true, "desired": false}
		} else {
			result.Present = true
			result.CurrentState = "disabled"
			result.Matches = true
		}

	case "present":
		// present: swap exists (doesn't need to be active)
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["exists"] = map[string]interface{}{"current": false, "desired": true}
		} else {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = true
		}

	case "absent":
		// absent: swap does not exist
		if exists || isActive {
			result.Present = true
			if isActive {
				result.CurrentState = "enabled"
			} else {
				result.CurrentState = "disabled"
			}
			result.Matches = false
			result.Diff["exists"] = map[string]interface{}{"current": true, "desired": false}
		} else {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = true
		}

	default:
		return nil, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Apply makes changes to reach the desired state.
func (m *SwapModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "linux" {
		result.Success = false
		result.Comment = "swap module only works on Linux"
		return result, fmt.Errorf("swap module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	isActive := m.isSwapActive(config.Path)
	exists := m.swapExists(config.Path)
	inFstab := m.isInFstab(config.Path)

	switch decl.State {
	case "enabled":
		// Create swap if it doesn't exist
		if !exists {
			if err := m.createSwap(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create swap: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Enable swap if not active
		if !isActive {
			if err := m.enableSwap(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to enable swap: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Add to fstab if persist is true
		if config.Persist && !inFstab {
			if err := m.addToFstab(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to add to fstab: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Swap '%s' is enabled", config.Path)

	case "disabled":
		// Create swap if it doesn't exist
		if !exists {
			if err := m.createSwap(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create swap: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Disable swap if active
		if isActive {
			if err := m.disableSwap(config.Path); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to disable swap: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Swap '%s' is disabled", config.Path)

	case "present":
		// Create swap if it doesn't exist
		if !exists {
			if err := m.createSwap(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create swap: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Swap '%s' is present", config.Path)

	case "absent":
		// Disable swap if active
		if isActive {
			if err := m.disableSwap(config.Path); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to disable swap: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Remove from fstab
		if inFstab {
			if err := m.removeFromFstab(config.Path); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to remove from fstab: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Remove swap file (not partition)
		if exists && m.isSwapFile(config.Path) {
			if err := os.Remove(config.Path); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to remove swap file: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Swap '%s' is absent", config.Path)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check.
func (m *SwapModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses the state declaration into SwapConfig.
func (m *SwapModule) parseConfig(decl *StateDeclaration) (*SwapConfig, error) {
	config := &SwapConfig{
		Priority: -1,
		Persist:  true,
	}

	config.Path = getStringParameter(decl, "path", "")
	if config.Path == "" {
		config.Path = decl.ID
	}
	if config.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	config.Size = getStringParameter(decl, "size", "")
	config.Priority = getIntParameter(decl, "priority", -1)
	config.Persist = getBoolParameter(decl, "persist", true)
	config.Label = getStringParameter(decl, "label", "")
	config.UUID = getStringParameter(decl, "uuid", "")

	return config, nil
}

// isSwapActive checks if swap is currently active.
func (m *SwapModule) isSwapActive(path string) bool {
	file, err := os.Open("/proc/swaps")
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip header line
	scanner.Scan()

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 1 && fields[0] == path {
			return true
		}
	}
	return false
}

// swapExists checks if the swap file/partition exists.
func (m *SwapModule) swapExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isSwapFile checks if the path is a regular file (not a partition).
func (m *SwapModule) isSwapFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// isInFstab checks if the swap is in /etc/fstab.
func (m *SwapModule) isInFstab(path string) bool {
	file, err := os.Open("/etc/fstab")
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == "swap" {
			if fields[0] == path {
				return true
			}
		}
	}
	return false
}

// createSwap creates a swap file or initializes a swap partition.
func (m *SwapModule) createSwap(config *SwapConfig) error {
	// Check if it's a partition or file
	info, err := os.Stat(config.Path)

	if os.IsNotExist(err) {
		// Create swap file
		if config.Size == "" {
			return fmt.Errorf("size is required when creating a swap file")
		}

		sizeBytes, err := m.parseSize(config.Size)
		if err != nil {
			return err
		}

		// Create the swap file using fallocate or dd
		if err := m.createSwapFile(config.Path, sizeBytes); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if info.Mode().IsRegular() {
		// File exists, check if we need to resize
		// For now, just use mkswap
	}

	// Set proper permissions
	if err := os.Chmod(config.Path, 0600); err != nil {
		return err
	}

	// Initialize swap
	args := []string{config.Path}
	if config.Label != "" {
		args = append([]string{"-L", config.Label}, args...)
	}
	if config.UUID != "" {
		args = append([]string{"-U", config.UUID}, args...)
	}

	cmd := exec.Command("mkswap", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkswap failed: %v: %s", err, string(output))
	}

	return nil
}

// createSwapFile creates a swap file of the specified size.
func (m *SwapModule) createSwapFile(path string, sizeBytes int64) error {
	// Try fallocate first (faster)
	cmd := exec.Command("fallocate", "-l", fmt.Sprintf("%d", sizeBytes), path)
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Fallback to dd
	sizeMB := sizeBytes / (1024 * 1024)
	if sizeMB < 1 {
		sizeMB = 1
	}

	cmd = exec.Command("dd", "if=/dev/zero", "of="+path, "bs=1M", fmt.Sprintf("count=%d", sizeMB))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dd failed: %v: %s", err, string(output))
	}

	return nil
}

// parseSize parses a size string into bytes.
func (m *SwapModule) parseSize(size string) (int64, error) {
	size = strings.TrimSpace(size)
	if size == "" {
		return 0, fmt.Errorf("empty size")
	}

	// Check for suffix
	var multiplier int64 = 1024 * 1024 // Default to MiB
	var numStr string

	size = strings.ToUpper(size)
	if strings.HasSuffix(size, "G") || strings.HasSuffix(size, "GB") || strings.HasSuffix(size, "GIB") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimRight(size, "GBIB")
	} else if strings.HasSuffix(size, "M") || strings.HasSuffix(size, "MB") || strings.HasSuffix(size, "MIB") {
		multiplier = 1024 * 1024
		numStr = strings.TrimRight(size, "MBIB")
	} else if strings.HasSuffix(size, "K") || strings.HasSuffix(size, "KB") || strings.HasSuffix(size, "KIB") {
		multiplier = 1024
		numStr = strings.TrimRight(size, "KBIB")
	} else if strings.HasSuffix(size, "T") || strings.HasSuffix(size, "TB") || strings.HasSuffix(size, "TIB") {
		multiplier = 1024 * 1024 * 1024 * 1024
		numStr = strings.TrimRight(size, "TBIB")
	} else {
		numStr = size
	}

	num, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %s", size)
	}

	return int64(num * float64(multiplier)), nil
}

// enableSwap enables the swap.
func (m *SwapModule) enableSwap(config *SwapConfig) error {
	args := []string{config.Path}
	if config.Priority >= 0 {
		args = append([]string{"-p", fmt.Sprintf("%d", config.Priority)}, args...)
	}

	cmd := exec.Command("swapon", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("swapon failed: %v: %s", err, string(output))
	}
	return nil
}

// disableSwap disables the swap.
func (m *SwapModule) disableSwap(path string) error {
	cmd := exec.Command("swapoff", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("swapoff failed: %v: %s", err, string(output))
	}
	return nil
}

// addToFstab adds swap entry to /etc/fstab.
func (m *SwapModule) addToFstab(config *SwapConfig) error {
	// Read existing content
	content, err := os.ReadFile("/etc/fstab")
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Build entry
	priority := "defaults"
	if config.Priority >= 0 {
		priority = fmt.Sprintf("sw,pri=%d", config.Priority)
	}

	entry := fmt.Sprintf("%s\tnone\tswap\t%s\t0\t0\n", config.Path, priority)

	// Append to file
	f, err := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline if needed
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	// Add comment
	comment := fmt.Sprintf("# Keystone Core: %s\n", config.Path)
	if _, err := f.WriteString(comment); err != nil {
		return err
	}

	if _, err := f.WriteString(entry); err != nil {
		return err
	}

	return nil
}

// removeFromFstab removes swap entry from /etc/fstab.
func (m *SwapModule) removeFromFstab(path string) error {
	file, err := os.Open("/etc/fstab")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	skipNext := false

	for scanner.Scan() {
		line := scanner.Text()

		// Skip Keystone comment line
		if strings.Contains(line, "# Keystone Core: "+path) {
			skipNext = true
			continue
		}

		if skipNext {
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[0] == path && fields[2] == "swap" {
				skipNext = false
				continue
			}
			skipNext = false
		}

		// Check if this is a swap entry for our path
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[0] == path && fields[2] == "swap" {
				continue
			}
		}

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return os.WriteFile("/etc/fstab", []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// GetSwapInfo returns information about active swap.
func (m *SwapModule) GetSwapInfo(path string) (swapType string, size, used int64, priority int, err error) {
	file, err := os.Open("/proc/swaps")
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Skip header
	scanner.Scan()

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 5 && fields[0] == path {
			swapType = fields[1]
			fmt.Sscanf(fields[2], "%d", &size)
			fmt.Sscanf(fields[3], "%d", &used)
			fmt.Sscanf(fields[4], "%d", &priority)
			return swapType, size * 1024, used * 1024, priority, nil
		}
	}

	return "", 0, 0, 0, fmt.Errorf("swap not found: %s", path)
}
