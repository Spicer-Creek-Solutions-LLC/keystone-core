// Package statemgmt provides state management modules.
package statemgmt

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// MountModule manages filesystem mount points.
type MountModule struct {
	*BaseModule
}

// MountConfig holds mount configuration.
type MountConfig struct {
	// Device is the block device, UUID, LABEL, or network path
	Device string
	// Path is the mount point path
	Path string
	// FSType is the filesystem type (ext4, xfs, nfs, cifs, etc.)
	FSType string
	// Options are mount options (defaults, noatime, ro, etc.)
	Options []string
	// Dump is the dump flag for fstab (0 or 1)
	Dump int
	// Pass is the fsck pass number (0, 1, or 2)
	Pass int
	// Persist if true, adds entry to fstab/equivalent
	Persist bool
	// CreatePath if true, creates mount point directory
	CreatePath bool
	// Owner for the mount point directory
	Owner string
	// Group for the mount point directory
	Group string
	// Mode for the mount point directory
	Mode string
}

// FstabEntry represents a parsed fstab entry.
type FstabEntry struct {
	Device  string
	Path    string
	FSType  string
	Options string
	Dump    int
	Pass    int
	Comment string
}

// NewMountModule creates a new mount module.
func NewMountModule() *MountModule {
	return &MountModule{
		BaseModule: NewBaseModule("mount", []string{"mounted", "unmounted", "present", "absent"}),
	}
}

// Check determines if the mount point is in the desired state.
func (m *MountModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	isMounted := m.isMounted(ctx, config.Path)
	inFstab := m.isInFstab(config)

	switch decl.State {
	case "mounted":
		// mounted: must be currently mounted AND in fstab (if persist)
		switch {
		case !isMounted:
			result.Present = false
			result.CurrentState = "unmounted"
			result.Matches = false
			result.Diff["mounted"] = map[string]interface{}{"current": false, "desired": true}
		case config.Persist && !inFstab:
			result.Present = true
			result.CurrentState = "mounted (not persistent)"
			result.Matches = false
			result.Diff["persistent"] = map[string]interface{}{"current": false, "desired": true}
		default:
			result.Present = true
			result.CurrentState = "mounted"
			result.Matches = true
		}

	case "unmounted":
		// unmounted: must not be currently mounted (fstab entry can stay)
		if isMounted {
			result.Present = true
			result.CurrentState = "mounted"
			result.Matches = false
			result.Diff["mounted"] = map[string]interface{}{"current": true, "desired": false}
		} else {
			result.Present = false
			result.CurrentState = "unmounted"
			result.Matches = true
		}

	case "present":
		// present: must be in fstab (doesn't need to be mounted)
		if !inFstab {
			result.Present = false
			result.CurrentState = "absent from fstab"
			result.Matches = false
			result.Diff["fstab"] = map[string]interface{}{"current": "absent", "desired": "present"}
		} else {
			result.Present = true
			result.CurrentState = "present in fstab"
			result.Matches = true
		}

	case "absent":
		// absent: not in fstab AND not mounted
		if inFstab || isMounted {
			result.Present = true
			switch {
			case isMounted && inFstab:
				result.CurrentState = "mounted and in fstab"
			case isMounted:
				result.CurrentState = "mounted"
			default:
				result.CurrentState = "in fstab"
			}
			result.Matches = false
			result.Diff["state"] = map[string]interface{}{"current": result.CurrentState, "desired": "absent"}
		} else {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = true
		}

	default:
		return nil, fmt.Errorf("unknown state: %s", decl.State)
	}

	result.Metadata["is_mounted"] = isMounted
	result.Metadata["in_fstab"] = inFstab

	return result, nil
}

// Apply makes changes to reach the desired state.
func (m *MountModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	isMounted := m.isMounted(ctx, config.Path)
	inFstab := m.isInFstab(config)

	switch decl.State {
	case "mounted":
		// Ensure mount point directory exists
		if config.CreatePath {
			if err := m.ensureMountPoint(ctx, config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create mount point: %v", err)
				return result, err
			}
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

		// Mount if not already mounted
		if !isMounted {
			if err := m.mount(ctx, config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to mount: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Mount point '%s' is mounted", config.Path)

	case "unmounted":
		// Unmount if mounted
		if isMounted {
			if err := m.unmount(ctx, config.Path); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to unmount: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Mount point '%s' is unmounted", config.Path)

	case "present":
		// Add to fstab if not present
		if !inFstab {
			if err := m.addToFstab(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to add to fstab: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Mount point '%s' is present in fstab", config.Path)

	case "absent":
		// Unmount if mounted
		if isMounted {
			if err := m.unmount(ctx, config.Path); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to unmount: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Remove from fstab if present
		if inFstab {
			if err := m.removeFromFstab(config.Path); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to remove from fstab: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Mount point '%s' is absent", config.Path)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check.
func (m *MountModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses the state declaration into MountConfig.
func (m *MountModule) parseConfig(decl *StateDeclaration) (*MountConfig, error) {
	config := &MountConfig{
		Dump:       0,
		Pass:       0,
		Persist:    true,
		CreatePath: true,
		Mode:       "0755",
	}

	config.Device = getStringParameter(decl, "device", "")
	if config.Device == "" {
		return nil, fmt.Errorf("device is required")
	}

	config.Path = getStringParameter(decl, "path", "")
	if config.Path == "" {
		config.Path = decl.ID
	}
	if config.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	config.FSType = getStringParameter(decl, "fstype", "")
	// FSType can be auto-detected on mount, so not strictly required

	// Parse options - check "opts" key first, then "options" key
	optsVal := decl.Parameters["opts"]
	if optsVal == nil {
		optsVal = decl.Parameters["options"]
	}
	switch opts := optsVal.(type) {
	case []interface{}:
		for _, o := range opts {
			if s, ok := o.(string); ok {
				config.Options = append(config.Options, s)
			}
		}
	case string:
		config.Options = strings.Split(opts, ",")
	}

	config.Dump = getIntParameter(decl, "dump", 0)
	config.Pass = getIntParameter(decl, "pass", 0)
	config.Persist = getBoolParameter(decl, "persist", true)
	config.CreatePath = getBoolParameter(decl, "create_path", true)
	config.Owner = getStringParameter(decl, "owner", "")
	config.Group = getStringParameter(decl, "group", "")
	config.Mode = getStringParameter(decl, "mode", "0755")

	return config, nil
}

// isMounted checks if a path is currently mounted.
func (m *MountModule) isMounted(ctx context.Context, path string) bool {
	switch runtime.GOOS {
	case "linux":
		return m.isMountedLinux(path)
	case "darwin":
		return m.isMountedDarwin(ctx, path)
	case "windows":
		return m.isMountedWindows(path)
	default:
		return false
	}
}

// isMountedLinux checks /proc/mounts for the mount point.
func (m *MountModule) isMountedLinux(path string) bool {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer file.Close()

	absPath, _ := filepath.Abs(path)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			mountPoint := fields[1]
			if mountPoint == absPath || mountPoint == path {
				return true
			}
		}
	}
	return false
}

// isMountedDarwin uses mount command to check.
func (m *MountModule) isMountedDarwin(ctx context.Context, path string) bool {
	cmd := exec.CommandContext(ctx, "mount")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	absPath, _ := filepath.Abs(path)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Format: /dev/disk1s1 on /path type apfs (options)
		if strings.Contains(line, " on "+absPath+" ") || strings.Contains(line, " on "+path+" ") {
			return true
		}
	}
	return false
}

// isMountedWindows checks for mounted drives.
func (m *MountModule) isMountedWindows(path string) bool {
	// Windows uses drive letters primarily
	// For network mounts, check if the path exists and is a reparse point
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// isInFstab checks if the mount point is in fstab or equivalent.
func (m *MountModule) isInFstab(config *MountConfig) bool {
	switch runtime.GOOS {
	case "linux":
		return m.isInFstabLinux(config.Path)
	case "darwin":
		return m.isInFstabDarwin(config.Path)
	case "windows":
		// Windows doesn't have fstab - always return false
		return false
	default:
		return false
	}
}

// isInFstabLinux parses /etc/fstab.
func (m *MountModule) isInFstabLinux(path string) bool {
	entries, err := m.parseFstab("/etc/fstab")
	if err != nil {
		return false
	}

	absPath, _ := filepath.Abs(path)
	for _, entry := range entries {
		if entry.Path == absPath || entry.Path == path {
			return true
		}
	}
	return false
}

// isInFstabDarwin parses /etc/fstab (less common on macOS).
func (m *MountModule) isInFstabDarwin(path string) bool {
	// macOS uses /etc/fstab but it's often empty
	// Most mounts are handled by automount or diskutil
	entries, err := m.parseFstab("/etc/fstab")
	if err != nil {
		return false
	}

	absPath, _ := filepath.Abs(path)
	for _, entry := range entries {
		if entry.Path == absPath || entry.Path == path {
			return true
		}
	}
	return false
}

// parseFstab parses an fstab file.
func (m *MountModule) parseFstab(path string) ([]FstabEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var entries []FstabEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		entry := FstabEntry{
			Device:  fields[0],
			Path:    fields[1],
			FSType:  fields[2],
			Options: fields[3],
		}

		if len(fields) > 4 {
			fmt.Sscanf(fields[4], "%d", &entry.Dump)
		}
		if len(fields) > 5 {
			fmt.Sscanf(fields[5], "%d", &entry.Pass)
		}

		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

// mount mounts the filesystem.
func (m *MountModule) mount(ctx context.Context, config *MountConfig) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		return m.mountUnix(ctx, config)
	case "windows":
		return m.mountWindows(ctx, config)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// mountUnix mounts using the mount command.
func (m *MountModule) mountUnix(ctx context.Context, config *MountConfig) error {
	args := []string{}

	if config.FSType != "" {
		args = append(args, "-t", config.FSType)
	}

	if len(config.Options) > 0 {
		args = append(args, "-o", strings.Join(config.Options, ","))
	}

	args = append(args, config.Device, config.Path)

	cmd := exec.CommandContext(ctx, "mount", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mount failed: %w: %s", err, string(output))
	}
	return nil
}

// mountWindows mounts a network share or disk.
func (m *MountModule) mountWindows(ctx context.Context, config *MountConfig) error {
	// For network shares, use net use
	if strings.HasPrefix(config.Device, "\\\\") {
		args := []string{"use", config.Path, config.Device}
		cmd := exec.CommandContext(ctx, "net", args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("net use failed: %w: %s", err, string(output))
		}
		return nil
	}

	// For local disks, Windows handles this automatically
	return nil
}

// unmount unmounts the filesystem.
func (m *MountModule) unmount(ctx context.Context, path string) error {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.CommandContext(ctx, "umount", path)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("umount failed: %w: %s", err, string(output))
		}
		return nil

	case "darwin":
		cmd := exec.CommandContext(ctx, "diskutil", "unmount", path)
		_, err := cmd.CombinedOutput()
		if err != nil {
			// Fallback to umount
			cmd = exec.CommandContext(ctx, "umount", path)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("unmount failed: %w: %s", err, string(output))
			}
		}
		return nil

	case "windows":
		// For network shares, use net use /delete
		cmd := exec.CommandContext(ctx, "net", "use", path, "/delete")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("net use /delete failed: %w: %s", err, string(output))
		}
		return nil

	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// ensureMountPoint creates the mount point directory.
func (m *MountModule) ensureMountPoint(ctx context.Context, config *MountConfig) error {
	info, err := os.Stat(config.Path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", config.Path)
		}
		return nil
	}

	if !os.IsNotExist(err) {
		return err
	}

	// Parse mode
	var mode os.FileMode = 0o755
	if config.Mode != "" {
		var modeInt int
		fmt.Sscanf(config.Mode, "%o", &modeInt)
		mode = os.FileMode(modeInt) //nolint:gosec // G115: file mode is 0-0777
	}

	if err := os.MkdirAll(config.Path, mode); err != nil {
		return err
	}

	// Set owner/group if specified (Unix only)
	if runtime.GOOS != "windows" && (config.Owner != "" || config.Group != "") {
		args := []string{}
		ownership := ""
		if config.Owner != "" {
			ownership = config.Owner
		}
		if config.Group != "" {
			ownership += ":" + config.Group
		}
		if ownership != "" {
			args = append(args, ownership, config.Path)
			cmd := exec.CommandContext(ctx, "chown", args...)
			if output, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("chown failed: %w: %s", err, string(output))
			}
		}
	}

	return nil
}

// addToFstab adds an entry to fstab.
func (m *MountModule) addToFstab(config *MountConfig) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		return m.addToFstabUnix(config)
	case "windows":
		// Windows doesn't use fstab
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// addToFstabUnix adds an entry to /etc/fstab.
func (m *MountModule) addToFstabUnix(config *MountConfig) error {
	fstabPath := "/etc/fstab"

	// Read existing content
	content, err := os.ReadFile(fstabPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Build the new entry
	options := "defaults"
	if len(config.Options) > 0 {
		options = strings.Join(config.Options, ",")
	}

	fstype := config.FSType
	if fstype == "" {
		fstype = "auto"
	}

	entry := fmt.Sprintf("%s\t%s\t%s\t%s\t%d\t%d\n",
		config.Device,
		config.Path,
		fstype,
		options,
		config.Dump,
		config.Pass,
	)

	// Append to file
	f, err := os.OpenFile(fstabPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec // G302: /etc/fstab must be world-readable for mount operations
	if err != nil {
		return err
	}
	defer f.Close()

	// Add comment if there's existing content without trailing newline
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}

	// Add comment with keystone marker
	comment := fmt.Sprintf("# Keystone Core: %s\n", config.Path)
	if _, err := f.WriteString(comment); err != nil {
		return err
	}

	if _, err := f.WriteString(entry); err != nil {
		return err
	}

	return nil
}

// removeFromFstab removes an entry from fstab.
func (m *MountModule) removeFromFstab(path string) error {
	switch runtime.GOOS {
	case "linux", "darwin":
		return m.removeFromFstabUnix(path)
	case "windows":
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// removeFromFstabUnix removes an entry from /etc/fstab.
func (m *MountModule) removeFromFstabUnix(mountPath string) error {
	fstabPath := "/etc/fstab"

	file, err := os.Open(fstabPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var lines []string
	absPath, _ := filepath.Abs(mountPath)
	scanner := bufio.NewScanner(file)
	skipNext := false

	for scanner.Scan() {
		line := scanner.Text()

		// Skip Keystone comment line for this mount
		if strings.Contains(line, "# Keystone Core: "+mountPath) ||
			strings.Contains(line, "# Keystone Core: "+absPath) {
			skipNext = true
			continue
		}

		// Skip the actual fstab entry
		if skipNext {
			fields := strings.Fields(line)
			if len(fields) >= 2 && (fields[1] == mountPath || fields[1] == absPath) {
				skipNext = false
				continue
			}
			skipNext = false
		}

		// Check if this line is a mount entry for our path
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && (fields[1] == mountPath || fields[1] == absPath) {
				continue
			}
		}

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Write back
	//nolint:gosec // G306: fstab needs to be readable by mount commands
	return os.WriteFile(fstabPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// GetFstabEntry retrieves the fstab entry for a mount point.
func (m *MountModule) GetFstabEntry(path string) (*FstabEntry, error) {
	entries, err := m.parseFstab("/etc/fstab")
	if err != nil {
		return nil, err
	}

	absPath, _ := filepath.Abs(path)
	for _, entry := range entries {
		if entry.Path == absPath || entry.Path == path {
			return &entry, nil
		}
	}

	return nil, nil
}

// GetMountInfo returns information about a mounted filesystem.
func (m *MountModule) GetMountInfo(ctx context.Context, path string) (device, fstype, options string, err error) {
	switch runtime.GOOS {
	case "linux":
		return m.getMountInfoLinux(path)
	case "darwin":
		return m.getMountInfoDarwin(ctx, path)
	default:
		return "", "", "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// getMountInfoLinux reads mount info from /proc/mounts.
func (m *MountModule) getMountInfoLinux(path string) (device, fstype, options string, err error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", "", "", err
	}
	defer file.Close()

	absPath, _ := filepath.Abs(path)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 {
			mountPoint := fields[1]
			if mountPoint == absPath || mountPoint == path {
				return fields[0], fields[2], fields[3], nil
			}
		}
	}

	return "", "", "", fmt.Errorf("mount point not found: %s", path)
}

// getMountInfoDarwin reads mount info using mount command.
func (m *MountModule) getMountInfoDarwin(ctx context.Context, path string) (device, fstype, options string, err error) {
	cmd := exec.CommandContext(ctx, "mount")
	output, err := cmd.Output()
	if err != nil {
		return "", "", "", err
	}

	absPath, _ := filepath.Abs(path)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Format: /dev/disk1s1 on /path type apfs (options)
		if strings.Contains(line, " on "+absPath+" ") || strings.Contains(line, " on "+path+" ") {
			// Parse the line
			parts := strings.Split(line, " on ")
			if len(parts) >= 2 {
				device = parts[0]
				rest := parts[1]

				// Find type and options
				if idx := strings.Index(rest, " type "); idx != -1 {
					typeParts := strings.SplitN(rest[idx+6:], " ", 2)
					fstype = typeParts[0]
					if len(typeParts) > 1 {
						options = strings.Trim(typeParts[1], "()")
					}
				}
				return device, fstype, options, nil
			}
		}
	}

	return "", "", "", fmt.Errorf("mount point not found: %s", path)
}
