// Package statemgmt provides state management modules.
package statemgmt

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// LVMPVModule manages LVM physical volumes.
type LVMPVModule struct {
	*BaseModule
}

// LVMVGModule manages LVM volume groups.
type LVMVGModule struct {
	*BaseModule
}

// LVMLVModule manages LVM logical volumes.
type LVMLVModule struct {
	*BaseModule
}

// LVMPVConfig holds physical volume configuration.
type LVMPVConfig struct {
	// Device is the block device path
	Device string
	// Force if true, forces PV creation even if device has data
	Force bool
	// DataAlignment is the data alignment in KiB
	DataAlignment int
	// MetadataSize is the metadata area size
	MetadataSize string
}

// LVMVGConfig holds volume group configuration.
type LVMVGConfig struct {
	// Name is the volume group name
	Name string
	// PhysicalVolumes are the PVs to include
	PhysicalVolumes []string
	// PESize is the physical extent size (e.g., "4M")
	PESize string
}

// LVMLVConfig holds logical volume configuration.
type LVMLVConfig struct {
	// Name is the logical volume name
	Name string
	// VGName is the volume group name
	VGName string
	// Size is the LV size (e.g., "10G", "100%FREE")
	Size string
	// Extents is the LV size in extents
	Extents string
	// FSType is the filesystem type to create (optional)
	FSType string
	// Stripes is the number of stripes
	Stripes int
	// StripeSize is the stripe size
	StripeSize string
	// Mirrors is the number of mirrors
	Mirrors int
	// Snapshot is the source LV for snapshot
	Snapshot string
	// ThinPool is the thin pool name
	ThinPool string
	// PoolMetadataSize is the thin pool metadata size
	PoolMetadataSize string
	// Resize if true, allows resizing existing LV
	Resize bool
}

// NewLVMPVModule creates a new LVM PV module.
func NewLVMPVModule() *LVMPVModule {
	return &LVMPVModule{
		BaseModule: NewBaseModule("lvm_pv", []string{"present", "absent"}),
	}
}

// NewLVMVGModule creates a new LVM VG module.
func NewLVMVGModule() *LVMVGModule {
	return &LVMVGModule{
		BaseModule: NewBaseModule("lvm_vg", []string{"present", "absent"}),
	}
}

// NewLVMLVModule creates a new LVM LV module.
func NewLVMLVModule() *LVMLVModule {
	return &LVMLVModule{
		BaseModule: NewBaseModule("lvm_lv", []string{"present", "absent"}),
	}
}

// ==================== LVM PV Module ====================

// Check determines if the PV is in the desired state.
func (m *LVMPVModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("lvm_pv module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	exists := m.pvExists(ctx, config.Device)
	result.Metadata["exists"] = exists

	switch decl.State {
	case "present":
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]interface{}{"current": "absent", "desired": "present"}
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
func (m *LVMPVModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "linux" {
		result.Success = false
		result.Comment = "lvm_pv module only works on Linux"
		return result, fmt.Errorf("lvm_pv module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	exists := m.pvExists(ctx, config.Device)

	switch decl.State {
	case "present":
		if !exists {
			if err := m.createPV(ctx, config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create PV: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("PV '%s' is present", config.Device)

	case "absent":
		if exists {
			if err := m.removePV(ctx, config.Device); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to remove PV: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("PV '%s' is absent", config.Device)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check.
func (m *LVMPVModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses PV configuration.
func (m *LVMPVModule) parseConfig(decl *StateDeclaration) (*LVMPVConfig, error) {
	config := &LVMPVConfig{}

	config.Device = getStringParameter(decl, "device", "")
	if config.Device == "" {
		config.Device = decl.ID
	}
	if config.Device == "" {
		return nil, fmt.Errorf("device is required")
	}

	config.Force = getBoolParameter(decl, "force", false)
	config.DataAlignment = getIntParameter(decl, "data_alignment", 0)
	config.MetadataSize = getStringParameter(decl, "metadata_size", "")

	return config, nil
}

// pvExists checks if a PV exists.
func (m *LVMPVModule) pvExists(ctx context.Context, device string) bool {
	cmd := exec.CommandContext(ctx, "pvs", "--noheadings", device)
	return cmd.Run() == nil
}

// createPV creates a physical volume.
func (m *LVMPVModule) createPV(ctx context.Context, config *LVMPVConfig) error {
	args := []string{}

	if config.Force {
		args = append(args, "-ff", "-y")
	}
	if config.DataAlignment > 0 {
		args = append(args, "--dataalignment", fmt.Sprintf("%dk", config.DataAlignment))
	}
	if config.MetadataSize != "" {
		args = append(args, "--metadatasize", config.MetadataSize)
	}

	args = append(args, config.Device)

	cmd := exec.CommandContext(ctx, "pvcreate", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pvcreate failed: %w: %s", err, string(output))
	}
	return nil
}

// removePV removes a physical volume.
func (m *LVMPVModule) removePV(ctx context.Context, device string) error {
	cmd := exec.CommandContext(ctx, "pvremove", "-ff", "-y", device)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pvremove failed: %w: %s", err, string(output))
	}
	return nil
}

// ==================== LVM VG Module ====================

// Check determines if the VG is in the desired state.
func (m *LVMVGModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("lvm_vg module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	exists := m.vgExists(ctx, config.Name)
	result.Metadata["exists"] = exists

	switch decl.State {
	case "present":
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]interface{}{"current": "absent", "desired": "present"}
		} else {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = true
			result.Metadata["pvs_checked"] = false
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
func (m *LVMVGModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "linux" {
		result.Success = false
		result.Comment = "lvm_vg module only works on Linux"
		return result, fmt.Errorf("lvm_vg module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	exists := m.vgExists(ctx, config.Name)

	switch decl.State {
	case "present":
		if !exists {
			if err := m.createVG(ctx, config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create VG: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("VG '%s' is present", config.Name)

	case "absent":
		if exists {
			if err := m.removeVG(ctx, config.Name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to remove VG: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("VG '%s' is absent", config.Name)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check.
func (m *LVMVGModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses VG configuration.
func (m *LVMVGModule) parseConfig(decl *StateDeclaration) (*LVMVGConfig, error) {
	config := &LVMVGConfig{}

	config.Name = getStringParameter(decl, "name", "")
	if config.Name == "" {
		config.Name = decl.ID
	}
	if config.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Parse PVs
	if pvs, ok := decl.Parameters["pvs"].([]interface{}); ok {
		for _, pv := range pvs {
			if s, ok := pv.(string); ok {
				config.PhysicalVolumes = append(config.PhysicalVolumes, s)
			}
		}
	} else if pv, ok := decl.Parameters["pv"].(string); ok {
		config.PhysicalVolumes = []string{pv}
	}

	config.PESize = getStringParameter(decl, "pe_size", "")

	return config, nil
}

// vgExists checks if a VG exists.
func (m *LVMVGModule) vgExists(ctx context.Context, name string) bool {
	cmd := exec.CommandContext(ctx, "vgs", "--noheadings", name)
	return cmd.Run() == nil
}

// createVG creates a volume group.
func (m *LVMVGModule) createVG(ctx context.Context, config *LVMVGConfig) error {
	if len(config.PhysicalVolumes) == 0 {
		return fmt.Errorf("at least one PV is required")
	}

	args := []string{}

	if config.PESize != "" {
		args = append(args, "-s", config.PESize)
	}

	args = append(args, config.Name)
	args = append(args, config.PhysicalVolumes...)

	cmd := exec.CommandContext(ctx, "vgcreate", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vgcreate failed: %w: %s", err, string(output))
	}
	return nil
}

// removeVG removes a volume group.
func (m *LVMVGModule) removeVG(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "vgremove", "-f", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vgremove failed: %w: %s", err, string(output))
	}
	return nil
}

// ==================== LVM LV Module ====================

// Check determines if the LV is in the desired state.
func (m *LVMLVModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("lvm_lv module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		return nil, err
	}

	exists := m.lvExists(ctx, config.VGName, config.Name)
	result.Metadata["exists"] = exists

	switch decl.State {
	case "present":
		if !exists {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = false
			result.Diff["state"] = map[string]interface{}{"current": "absent", "desired": "present"}
		} else {
			result.Present = true
			result.CurrentState = "present"
			result.Matches = true
			result.Metadata["size_checked"] = false
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
func (m *LVMLVModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	result := &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
	}

	if runtime.GOOS != "linux" {
		result.Success = false
		result.Comment = "lvm_lv module only works on Linux"
		return result, fmt.Errorf("lvm_lv module only works on Linux")
	}

	config, err := m.parseConfig(decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to parse config: %v", err)
		return result, err
	}

	exists := m.lvExists(ctx, config.VGName, config.Name)

	switch decl.State {
	case "present":
		if !exists {
			if err := m.createLV(ctx, config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create LV: %v", err)
				return result, err
			}
			result.Changed = true

			// Create filesystem if specified
			if config.FSType != "" {
				if err := m.createFilesystem(ctx, config); err != nil {
					result.Success = false
					result.Comment = fmt.Sprintf("Failed to create filesystem: %v", err)
					return result, err
				}
			}
		} else if config.Resize {
			// Check if resize needed
			if changed, err := m.resizeLV(ctx, config); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to resize LV: %v", err)
				return result, err
			} else if changed {
				result.Changed = true
			}
		}
		result.Comment = fmt.Sprintf("LV '%s/%s' is present", config.VGName, config.Name)

	case "absent":
		if exists {
			if err := m.removeLV(ctx, config.VGName, config.Name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to remove LV: %v", err)
				return result, err
			}
			result.Changed = true
		}
		result.Comment = fmt.Sprintf("LV '%s/%s' is absent", config.VGName, config.Name)

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Unknown state: %s", decl.State)
		return result, fmt.Errorf("unknown state: %s", decl.State)
	}

	return result, nil
}

// Test performs a dry-run check.
func (m *LVMLVModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// parseConfig parses LV configuration.
func (m *LVMLVModule) parseConfig(decl *StateDeclaration) (*LVMLVConfig, error) {
	config := &LVMLVConfig{}

	config.Name = getStringParameter(decl, "name", "")
	if config.Name == "" {
		config.Name = decl.ID
	}
	if config.Name == "" {
		return nil, fmt.Errorf("name is required")
	}

	config.VGName = getStringParameter(decl, "vg", "")
	if config.VGName == "" {
		config.VGName = getStringParameter(decl, "vg_name", "")
	}
	if config.VGName == "" {
		return nil, fmt.Errorf("vg (volume group) is required")
	}

	config.Size = getStringParameter(decl, "size", "")
	config.Extents = getStringParameter(decl, "extents", "")
	config.FSType = getStringParameter(decl, "fstype", "")
	config.Stripes = getIntParameter(decl, "stripes", 0)
	config.StripeSize = getStringParameter(decl, "stripe_size", "")
	config.Mirrors = getIntParameter(decl, "mirrors", 0)
	config.Snapshot = getStringParameter(decl, "snapshot", "")
	config.ThinPool = getStringParameter(decl, "thinpool", "")
	config.PoolMetadataSize = getStringParameter(decl, "pool_metadata_size", "")
	config.Resize = getBoolParameter(decl, "resize", false)

	return config, nil
}

// lvExists checks if an LV exists.
func (m *LVMLVModule) lvExists(ctx context.Context, vg, lv string) bool {
	path := fmt.Sprintf("%s/%s", vg, lv)
	cmd := exec.CommandContext(ctx, "lvs", "--noheadings", path)
	return cmd.Run() == nil
}

// createLV creates a logical volume.
func (m *LVMLVModule) createLV(ctx context.Context, config *LVMLVConfig) error {
	args := []string{"-n", config.Name}

	// Size options
	if config.Size != "" {
		// Check for percentage
		if strings.HasSuffix(config.Size, "%FREE") || strings.HasSuffix(config.Size, "%VG") ||
			strings.HasSuffix(config.Size, "%PVS") || strings.HasSuffix(config.Size, "%ORIGIN") {
			args = append(args, "-l", config.Size)
		} else {
			args = append(args, "-L", config.Size)
		}
	} else if config.Extents != "" {
		args = append(args, "-l", config.Extents)
	}

	// Striping
	if config.Stripes > 0 {
		args = append(args, "-i", strconv.Itoa(config.Stripes))
		if config.StripeSize != "" {
			args = append(args, "-I", config.StripeSize)
		}
	}

	// Mirroring
	if config.Mirrors > 0 {
		args = append(args, "-m", strconv.Itoa(config.Mirrors))
	}

	// Snapshot
	if config.Snapshot != "" {
		args = append(args, "-s", config.Snapshot)
	}

	// Thin pool
	if config.ThinPool != "" {
		args = append(args, "--thinpool", config.ThinPool)
		if config.PoolMetadataSize != "" {
			args = append(args, "--poolmetadatasize", config.PoolMetadataSize)
		}
	}

	args = append(args, config.VGName)

	cmd := exec.CommandContext(ctx, "lvcreate", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lvcreate failed: %w: %s", err, string(output))
	}
	return nil
}

// removeLV removes a logical volume.
func (m *LVMLVModule) removeLV(ctx context.Context, vg, lv string) error {
	path := fmt.Sprintf("%s/%s", vg, lv)
	cmd := exec.CommandContext(ctx, "lvremove", "-f", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lvremove failed: %w: %s", err, string(output))
	}
	return nil
}

// resizeLV resizes a logical volume.
func (m *LVMLVModule) resizeLV(ctx context.Context, config *LVMLVConfig) (bool, error) {
	if config.Size == "" && config.Extents == "" {
		return false, nil
	}

	path := fmt.Sprintf("/dev/%s/%s", config.VGName, config.Name)
	args := []string{"-f"}

	if config.Size != "" {
		if strings.HasSuffix(config.Size, "%FREE") || strings.HasSuffix(config.Size, "%VG") {
			args = append(args, "-l", config.Size)
		} else {
			args = append(args, "-L", config.Size)
		}
	} else {
		args = append(args, "-l", config.Extents)
	}

	args = append(args, path)

	cmd := exec.CommandContext(ctx, "lvresize", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if no resize needed
		if strings.Contains(string(output), "matches existing size") {
			return false, nil
		}
		return false, fmt.Errorf("lvresize failed: %w: %s", err, string(output))
	}
	return true, nil
}

// createFilesystem creates a filesystem on the LV.
func (m *LVMLVModule) createFilesystem(ctx context.Context, config *LVMLVConfig) error {
	path := fmt.Sprintf("/dev/%s/%s", config.VGName, config.Name)

	var cmd *exec.Cmd
	switch config.FSType {
	case "ext4":
		cmd = exec.CommandContext(ctx, "mkfs.ext4", "-F", path)
	case "ext3":
		cmd = exec.CommandContext(ctx, "mkfs.ext3", "-F", path)
	case "xfs":
		cmd = exec.CommandContext(ctx, "mkfs.xfs", "-f", path)
	case "btrfs":
		cmd = exec.CommandContext(ctx, "mkfs.btrfs", "-f", path)
	default:
		cmd = exec.CommandContext(ctx, "mkfs", "-t", config.FSType, path)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs failed: %w: %s", err, string(output))
	}
	return nil
}

// GetLVInfo returns information about a logical volume.
func (m *LVMLVModule) GetLVInfo(ctx context.Context, vg, lv string) (size, attr string, err error) {
	path := fmt.Sprintf("%s/%s", vg, lv)
	cmd := exec.CommandContext(ctx, "lvs", "--noheadings", "-o", "lv_size,lv_attr", path)
	output, err := cmd.Output()
	if err != nil {
		return "", "", err
	}

	fields := strings.Fields(string(output))
	if len(fields) >= 2 {
		return fields[0], fields[1], nil
	}
	return "", "", fmt.Errorf("unexpected output format")
}
