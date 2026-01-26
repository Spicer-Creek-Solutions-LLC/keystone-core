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

// DiskModule manages disk partitions.
type DiskModule struct {
	*BaseModule
}

// FilesystemModule manages filesystems.
type FilesystemModule struct {
	*BaseModule
}

// DiskConfig holds disk partition configuration.
type DiskConfig struct {
	// Device is the disk device (e.g., /dev/sda)
	Device string
	// PartitionNumber is the partition number
	PartitionNumber int
	// Start is the partition start (e.g., "1MiB", "0%", sector number)
	Start string
	// End is the partition end (e.g., "100%", "-1", "10GiB")
	End string
	// Size is an alternative to End (e.g., "10G")
	Size string
	// Type is the partition type (primary, extended, logical)
	Type string
	// FSType is the partition filesystem type code (for GPT: linux, swap, efi, etc.)
	FSType string
	// Label is the partition label
	Label string
	// Flags are partition flags (boot, lvm, raid, etc.)
	Flags []string
	// Unit is the unit for start/end (MiB, GiB, %, sector)
	Unit string
	// TableType is the partition table type (gpt, msdos)
	TableType string
}

// FilesystemConfig holds filesystem configuration.
type FilesystemConfig struct {
	// Device is the block device
	Device string
	// FSType is the filesystem type (ext4, xfs, btrfs, etc.)
	FSType string
	// Label is the filesystem label
	Label string
	// UUID is the filesystem UUID
	UUID string
	// Force if true, forces creation even if filesystem exists
	Force bool
	// Options are mkfs options
	Options []string
}

// PartitionInfo holds information about a partition.
type PartitionInfo struct {
	Number int
	Start  int64
	End    int64
	Size   int64
	Type   string
	FSType string
	Name   string
	Flags  []string
}

// NewDiskModule creates a new disk module.
func NewDiskModule() *DiskModule {
	return &DiskModule{
		BaseModule: NewBaseModule("disk", []string{"present", "absent", "formatted"}),
	}
}

// NewFilesystemModule creates a new filesystem module.
func NewFilesystemModule() *FilesystemModule {
	return &FilesystemModule{
		BaseModule: NewBaseModule("filesystem", []string{"present", "absent"}),
	}
}

// ==================== Disk Module ====================

// Check determines if the partition is in the desired state.
func (m *DiskModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("disk module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	partInfo := m.getPartitionInfo(config.Device, config.PartitionNumber)
	exists := partInfo != nil

	result.Metadata["exists"] = exists
	if partInfo != nil {
		result.Metadata["size"] = partInfo.Size
		result.Metadata["type"] = partInfo.Type
		result.Metadata["fstype"] = partInfo.FSType
	}

	switch decl.State {
	case "present", "formatted":
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]interface{}{"current": "absent", "desired": decl.State}
		} else {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = true
		}

	case "absent":
		if exists {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = false
			result.Diff["state"] = map[string]interface{}{"current": "present", "desired": "absent"}
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
func (m *DiskModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "linux" {
		result.Success = false
		result.Comment = "disk module only works on Linux"
		return result, fmt.Errorf("disk module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	partInfo := m.getPartitionInfo(config.Device, config.PartitionNumber)
	exists := partInfo != nil

	switch decl.State {
	case "present":
		if !exists {
			// Ensure partition table exists
			if err := m.ensurePartitionTable(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create partition table: %v", err)
				return result, err
			}

			if err := m.createPartition(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create partition: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Set flags if specified
		if len(config.Flags) > 0 {
			if err := m.setPartitionFlags(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to set partition flags: %v", err)
				return result, err
			}
		}

		result.Comment = fmt.Sprintf("Partition %s%d is present", config.Device, config.PartitionNumber)

	case "formatted":
		if !exists {
			// Create partition first
			if err := m.ensurePartitionTable(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create partition table: %v", err)
				return result, err
			}

			if err := m.createPartition(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create partition: %v", err)
				return result, err
			}
			result.Changed = true
		}

		// Create filesystem
		if config.FSType != "" {
			partDevice := fmt.Sprintf("%s%d", config.Device, config.PartitionNumber)
			if err := m.createFilesystem(partDevice, config.FSType, config.Label); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create filesystem: %v", err)
				return result, err
			}
			result.Changed = true
		}

		result.Comment = fmt.Sprintf("Partition %s%d is formatted", config.Device, config.PartitionNumber)

	case "absent":
		if exists {
			if err := m.deletePartition(config.Device, config.PartitionNumber); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete partition: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("Partition %s%d is absent", config.Device, config.PartitionNumber)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check.
func (m *DiskModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses disk configuration.
func (m *DiskModule) parseConfig(decl *StateDeclaration) (*DiskConfig, error) {
	config := &DiskConfig{
		Type:      "primary",
		Unit:      "MiB",
		TableType: "gpt",
	}

	config.Device = getStringParameter(decl, "device", "")
	if config.Device == "" {
		return nil, fmt.Errorf("device is required")
	}

	config.PartitionNumber = getIntParameter(decl, "number", 1)
	config.Start = getStringParameter(decl, "start", "0%")
	config.End = getStringParameter(decl, "end", "")
	config.Size = getStringParameter(decl, "size", "")
	config.Type = getStringParameter(decl, "type", "primary")
	config.FSType = getStringParameter(decl, "fstype", "")
	config.Label = getStringParameter(decl, "label", "")
	config.Unit = getStringParameter(decl, "unit", "MiB")
	config.TableType = getStringParameter(decl, "table_type", "gpt")

	// Parse flags
	if flags, ok := decl.Parameters["flags"].([]interface{}); ok {
		for _, f := range flags {
			if s, ok := f.(string); ok {
				config.Flags = append(config.Flags, s)
			}
		}
	}

	return config, nil
}

// getPartitionInfo retrieves partition information.
func (m *DiskModule) getPartitionInfo(device string, number int) *PartitionInfo {
	cmd := exec.Command("parted", "-m", "-s", device, "unit", "B", "print")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "BYT;") {
			continue
		}

		fields := strings.Split(strings.TrimSuffix(line, ";"), ":")
		if len(fields) < 7 {
			continue
		}

		partNum, err := strconv.Atoi(fields[0])
		if err != nil || partNum != number {
			continue
		}

		info := &PartitionInfo{Number: partNum}

		// Parse start/end/size (remove "B" suffix)
		fmt.Sscanf(strings.TrimSuffix(fields[1], "B"), "%d", &info.Start)
		fmt.Sscanf(strings.TrimSuffix(fields[2], "B"), "%d", &info.End)
		fmt.Sscanf(strings.TrimSuffix(fields[3], "B"), "%d", &info.Size)

		info.FSType = fields[4]
		if len(fields) > 5 {
			info.Name = fields[5]
		}
		if len(fields) > 6 && fields[6] != "" {
			info.Flags = strings.Split(fields[6], ", ")
		}

		return info
	}

	return nil
}

// ensurePartitionTable creates a partition table if needed.
func (m *DiskModule) ensurePartitionTable(config *DiskConfig) error {
	// Check if partition table exists
	cmd := exec.Command("parted", "-s", config.Device, "print")
	if cmd.Run() == nil {
		return nil
	}

	// Create partition table
	label := config.TableType
	if label == "" {
		label = "gpt"
	}

	cmd = exec.Command("parted", "-s", config.Device, "mklabel", label)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklabel failed: %v: %s", err, string(output))
	}
	return nil
}

// createPartition creates a partition.
func (m *DiskModule) createPartition(config *DiskConfig) error {
	args := []string{"-s", "-a", "optimal", config.Device}

	// Set unit
	args = append(args, "unit", config.Unit)

	// Create partition command
	args = append(args, "mkpart")

	// For GPT, we need a name (use label or default)
	name := config.Label
	if name == "" {
		name = fmt.Sprintf("part%d", config.PartitionNumber)
	}
	args = append(args, name)

	// Filesystem type (optional hint)
	if config.FSType != "" {
		args = append(args, config.FSType)
	}

	// Start and end
	args = append(args, config.Start)

	if config.End != "" {
		args = append(args, config.End)
	} else if config.Size != "" {
		// parted doesn't directly support size, need to calculate end
		// For simplicity, just use the size as end (parted handles this)
		args = append(args, config.Size)
	} else {
		args = append(args, "100%")
	}

	cmd := exec.Command("parted", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkpart failed: %v: %s", err, string(output))
	}
	return nil
}

// deletePartition deletes a partition.
func (m *DiskModule) deletePartition(device string, number int) error {
	cmd := exec.Command("parted", "-s", device, "rm", strconv.Itoa(number))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rm partition failed: %v: %s", err, string(output))
	}
	return nil
}

// setPartitionFlags sets partition flags.
func (m *DiskModule) setPartitionFlags(config *DiskConfig) error {
	for _, flag := range config.Flags {
		cmd := exec.Command("parted", "-s", config.Device, "set",
			strconv.Itoa(config.PartitionNumber), flag, "on")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("set flag %s failed: %v: %s", flag, err, string(output))
		}
	}
	return nil
}

// createFilesystem creates a filesystem on a device.
func (m *DiskModule) createFilesystem(device, fstype, label string) error {
	var cmd *exec.Cmd

	switch fstype {
	case "ext4":
		args := []string{"-F"}
		if label != "" {
			args = append(args, "-L", label)
		}
		args = append(args, device)
		cmd = exec.Command("mkfs.ext4", args...)

	case "ext3":
		args := []string{"-F"}
		if label != "" {
			args = append(args, "-L", label)
		}
		args = append(args, device)
		cmd = exec.Command("mkfs.ext3", args...)

	case "xfs":
		args := []string{"-f"}
		if label != "" {
			args = append(args, "-L", label)
		}
		args = append(args, device)
		cmd = exec.Command("mkfs.xfs", args...)

	case "btrfs":
		args := []string{"-f"}
		if label != "" {
			args = append(args, "-L", label)
		}
		args = append(args, device)
		cmd = exec.Command("mkfs.btrfs", args...)

	case "vfat", "fat32":
		args := []string{"-F", "32"}
		if label != "" {
			args = append(args, "-n", label)
		}
		args = append(args, device)
		cmd = exec.Command("mkfs.vfat", args...)

	case "swap":
		args := []string{}
		if label != "" {
			args = append(args, "-L", label)
		}
		args = append(args, device)
		cmd = exec.Command("mkswap", args...)

	default:
		args := []string{"-t", fstype}
		args = append(args, device)
		cmd = exec.Command("mkfs", args...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs failed: %v: %s", err, string(output))
	}
	return nil
}

// ==================== Filesystem Module ====================

// Check determines if the filesystem is in the desired state.
func (m *FilesystemModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("filesystem module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	currentFS := m.getFilesystemType(config.Device)
	hasFS := currentFS != ""

	result.Metadata["current_fstype"] = currentFS

	switch decl.State {
	case "present":
		if !hasFS || (config.FSType != "" && currentFS != config.FSType) {
			result.Present = hasFS
			if hasFS {
				result.CurrentState = fmt.Sprintf("present (%s)", currentFS)
			} else {
				result.CurrentState = "absent"
			}
			result.Matches = false
			result.Diff["fstype"] = map[string]interface{}{"current": currentFS, "desired": config.FSType}
		} else {
			result.Present = true
			result.CurrentState = fmt.Sprintf("present (%s)", currentFS)
			result.Matches = true
		}

	case "absent":
		if hasFS {
			result.Present = true
			result.CurrentState = fmt.Sprintf("present (%s)", currentFS)
			result.Matches = false
			result.Diff["state"] = map[string]interface{}{"current": "present", "desired": "absent"}
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
func (m *FilesystemModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "linux" {
		result.Success = false
		result.Comment = "filesystem module only works on Linux"
		return result, fmt.Errorf("filesystem module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	currentFS := m.getFilesystemType(config.Device)
	hasFS := currentFS != ""

	switch decl.State {
	case "present":
		if !hasFS || config.Force || (config.FSType != "" && currentFS != config.FSType) {
			if err := m.createFS(config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create filesystem: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("Filesystem %s on %s is present", config.FSType, config.Device)

	case "absent":
		if hasFS {
			if err := m.wipeFS(config.Device); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to wipe filesystem: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("Filesystem on %s is absent", config.Device)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check.
func (m *FilesystemModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses filesystem configuration.
func (m *FilesystemModule) parseConfig(decl *StateDeclaration) (*FilesystemConfig, error) {
	config := &FilesystemConfig{}

	config.Device = getStringParameter(decl, "device", "")
	if config.Device == "" {
		config.Device = decl.ID
	}
	if config.Device == "" {
		return nil, fmt.Errorf("device is required")
	}

	config.FSType = getStringParameter(decl, "fstype", "")
	if config.FSType == "" && decl.State == "present" {
		return nil, fmt.Errorf("fstype is required for present state")
	}

	config.Label = getStringParameter(decl, "label", "")
	config.UUID = getStringParameter(decl, "uuid", "")
	config.Force = getBoolParameter(decl, "force", false)

	// Parse options
	if opts, ok := decl.Parameters["opts"].([]interface{}); ok {
		for _, o := range opts {
			if s, ok := o.(string); ok {
				config.Options = append(config.Options, s)
			}
		}
	}

	return config, nil
}

// getFilesystemType returns the filesystem type of a device.
func (m *FilesystemModule) getFilesystemType(device string) string {
	cmd := exec.Command("blkid", "-o", "value", "-s", "TYPE", device)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// createFS creates a filesystem.
func (m *FilesystemModule) createFS(config *FilesystemConfig) error {
	var args []string
	var cmdName string

	switch config.FSType {
	case "ext4":
		cmdName = "mkfs.ext4"
		args = []string{"-F"}
		if config.Label != "" {
			args = append(args, "-L", config.Label)
		}
		if config.UUID != "" {
			args = append(args, "-U", config.UUID)
		}

	case "ext3":
		cmdName = "mkfs.ext3"
		args = []string{"-F"}
		if config.Label != "" {
			args = append(args, "-L", config.Label)
		}

	case "xfs":
		cmdName = "mkfs.xfs"
		args = []string{"-f"}
		if config.Label != "" {
			args = append(args, "-L", config.Label)
		}

	case "btrfs":
		cmdName = "mkfs.btrfs"
		args = []string{"-f"}
		if config.Label != "" {
			args = append(args, "-L", config.Label)
		}

	case "vfat", "fat32":
		cmdName = "mkfs.vfat"
		args = []string{"-F", "32"}
		if config.Label != "" {
			args = append(args, "-n", config.Label)
		}

	case "ntfs":
		cmdName = "mkfs.ntfs"
		args = []string{"-f"}
		if config.Label != "" {
			args = append(args, "-L", config.Label)
		}

	default:
		cmdName = "mkfs"
		args = []string{"-t", config.FSType}
	}

	args = append(args, config.Options...)
	args = append(args, config.Device)

	cmd := exec.Command(cmdName, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs failed: %v: %s", err, string(output))
	}
	return nil
}

// wipeFS wipes the filesystem signature from a device.
func (m *FilesystemModule) wipeFS(device string) error {
	cmd := exec.Command("wipefs", "-a", device)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wipefs failed: %v: %s", err, string(output))
	}
	return nil
}

// GetBlockDevices returns a list of block devices.
func GetBlockDevices() ([]string, error) {
	file, err := os.Open("/proc/partitions")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var devices []string
	scanner := bufio.NewScanner(file)

	// Skip header lines
	scanner.Scan()
	scanner.Scan()

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 {
			name := fields[3]
			// Filter to whole disks (not partitions)
			if !strings.ContainsAny(name, "0123456789") || strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "loop") {
				devices = append(devices, "/dev/"+name)
			}
		}
	}

	return devices, scanner.Err()
}
