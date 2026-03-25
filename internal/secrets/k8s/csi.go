package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/secrets/injection"
)

// CSIDriver implements a Container Storage Interface (CSI) driver
// for mounting secrets as volumes in Kubernetes pods.
type CSIDriver struct {
	config *CSIDriverConfig
	source injection.SecretSource

	mu     sync.RWMutex
	mounts map[string]*MountInfo
}

// CSIDriverConfig configures the CSI driver.
type CSIDriverConfig struct {
	// DriverName is the CSI driver name.
	DriverName string

	// NodeID is the node identifier.
	NodeID string

	// Endpoint is the CSI driver endpoint.
	Endpoint string

	// DataDir is the directory for driver state.
	DataDir string

	// DefaultMode is the default file permissions.
	DefaultMode os.FileMode
}

// MountInfo tracks a mounted volume.
type MountInfo struct {
	VolumeID   string
	TargetPath string
	Secrets    []SecretMount
	ReadOnly   bool
}

// SecretMount defines a secret to mount.
type SecretMount struct {
	SecretPath string `json:"secretPath"`
	SecretKey  string `json:"secretKey,omitempty"`
	FileName   string `json:"fileName"`
	Mode       int    `json:"mode,omitempty"`
}

// DefaultCSIDriverConfig returns a default CSI driver configuration.
func DefaultCSIDriverConfig() *CSIDriverConfig {
	return &CSIDriverConfig{
		DriverName:  "secrets.csi.keystone.io",
		Endpoint:    "unix:///var/lib/kubelet/plugins/secrets.csi.keystone.io/csi.sock",
		DataDir:     "/var/lib/keystone-csi",
		DefaultMode: 0o600,
	}
}

// NewCSIDriver creates a new CSI driver.
func NewCSIDriver(config *CSIDriverConfig, source injection.SecretSource) (*CSIDriver, error) {
	if config == nil {
		config = DefaultCSIDriverConfig()
	}
	if source == nil {
		return nil, fmt.Errorf("source is required")
	}
	if config.NodeID == "" {
		hostname, _ := os.Hostname()
		config.NodeID = hostname
	}

	return &CSIDriver{
		config: config,
		source: source,
		mounts: make(map[string]*MountInfo),
	}, nil
}

// GetPluginInfo returns plugin information.
func (d *CSIDriver) GetPluginInfo() map[string]string {
	return map[string]string{
		"name":    d.config.DriverName,
		"version": "1.0.0",
	}
}

// GetPluginCapabilities returns plugin capabilities.
func (d *CSIDriver) GetPluginCapabilities() []string {
	return []string{
		"CONTROLLER_SERVICE",
		"VOLUME_ACCESSIBILITY_CONSTRAINTS",
	}
}

// Probe checks if the driver is healthy.
func (d *CSIDriver) Probe(ctx context.Context) (bool, error) {
	return true, nil
}

// NodeStageVolume stages a volume on a node.
func (d *CSIDriver) NodeStageVolume(ctx context.Context, volumeID, stagingPath string, volumeContext map[string]string) error {
	// CSI staging is not required for secrets
	return nil
}

// NodeUnstageVolume unstages a volume from a node.
func (d *CSIDriver) NodeUnstageVolume(ctx context.Context, volumeID, stagingPath string) error {
	// CSI staging is not required for secrets
	return nil
}

// NodePublishVolume mounts a volume to a target path.
func (d *CSIDriver) NodePublishVolume(ctx context.Context, volumeID, targetPath string, volumeContext map[string]string, readOnly bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if already mounted
	if _, exists := d.mounts[volumeID]; exists {
		return nil
	}

	// Parse secrets from volume context
	secrets, err := d.parseSecretMounts(volumeContext)
	if err != nil {
		return fmt.Errorf("failed to parse secrets: %w", err)
	}

	// Create target directory
	if err := os.MkdirAll(targetPath, 0o750); err != nil {
		return fmt.Errorf("failed to create target path: %w", err)
	}

	// Fetch and write secrets
	for _, secretMount := range secrets {
		if err := d.writeSecret(ctx, targetPath, secretMount); err != nil {
			// Clean up on failure
			os.RemoveAll(targetPath)
			return fmt.Errorf("failed to write secret %s: %w", secretMount.SecretPath, err)
		}
	}

	// Track mount
	d.mounts[volumeID] = &MountInfo{
		VolumeID:   volumeID,
		TargetPath: targetPath,
		Secrets:    secrets,
		ReadOnly:   readOnly,
	}

	return nil
}

// NodeUnpublishVolume unmounts a volume from a target path.
func (d *CSIDriver) NodeUnpublishVolume(ctx context.Context, volumeID, targetPath string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	mount, exists := d.mounts[volumeID]
	if !exists {
		return nil
	}

	// Remove secret files
	if err := os.RemoveAll(mount.TargetPath); err != nil {
		return fmt.Errorf("failed to remove target path: %w", err)
	}

	delete(d.mounts, volumeID)
	return nil
}

// NodeGetCapabilities returns node capabilities.
func (d *CSIDriver) NodeGetCapabilities() []string {
	return []string{
		"STAGE_UNSTAGE_VOLUME",
	}
}

// NodeGetInfo returns node information.
func (d *CSIDriver) NodeGetInfo() map[string]interface{} {
	return map[string]interface{}{
		"nodeId":            d.config.NodeID,
		"maxVolumesPerNode": 256,
	}
}

// CreateVolume creates a new volume (controller).
func (d *CSIDriver) CreateVolume(ctx context.Context, name string, capacityBytes int64, parameters map[string]string) (string, error) {
	// Volume ID is just the name for secrets
	return name, nil
}

// DeleteVolume deletes a volume (controller).
func (d *CSIDriver) DeleteVolume(ctx context.Context, volumeID string) error {
	// Secrets are ephemeral, nothing to delete
	return nil
}

// ControllerPublishVolume attaches a volume to a node (controller).
func (d *CSIDriver) ControllerPublishVolume(ctx context.Context, volumeID, nodeID string, readOnly bool) error {
	// No-op for secrets
	return nil
}

// ControllerUnpublishVolume detaches a volume from a node (controller).
func (d *CSIDriver) ControllerUnpublishVolume(ctx context.Context, volumeID, nodeID string) error {
	// No-op for secrets
	return nil
}

// GetCapacity returns the capacity of the storage pool.
func (d *CSIDriver) GetCapacity(ctx context.Context) (int64, error) {
	// Secrets don't consume traditional storage
	return 0, nil
}

// ListVolumes lists all volumes.
func (d *CSIDriver) ListVolumes(ctx context.Context) ([]*VolumeInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	volumes := make([]*VolumeInfo, 0, len(d.mounts))
	for id, mount := range d.mounts {
		volumes = append(volumes, &VolumeInfo{
			VolumeID:   id,
			TargetPath: mount.TargetPath,
		})
	}
	return volumes, nil
}

// VolumeInfo contains volume information.
type VolumeInfo struct {
	VolumeID   string
	TargetPath string
}

// RefreshSecrets refreshes secrets in all mounted volumes.
func (d *CSIDriver) RefreshSecrets(ctx context.Context) error {
	d.mu.RLock()
	mounts := make([]*MountInfo, 0, len(d.mounts))
	for _, m := range d.mounts {
		mounts = append(mounts, m)
	}
	d.mu.RUnlock()

	var lastErr error
	for _, mount := range mounts {
		for _, secretMount := range mount.Secrets {
			if err := d.writeSecret(ctx, mount.TargetPath, secretMount); err != nil {
				slog.Error("failed to refresh secret", "secret_path", secretMount.SecretPath, "error", err)
				lastErr = err
			}
		}
	}
	return lastErr
}

func (d *CSIDriver) parseSecretMounts(volumeContext map[string]string) ([]SecretMount, error) {
	secretsJSON, ok := volumeContext["secrets"]
	if !ok {
		return nil, fmt.Errorf("secrets configuration not found in volume context")
	}

	var secrets []SecretMount
	if err := json.Unmarshal([]byte(secretsJSON), &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets JSON: %w", err)
	}

	return secrets, nil
}

func (d *CSIDriver) writeSecret(ctx context.Context, targetPath string, mount SecretMount) error {
	// Fetch secret from source
	secret, err := d.source.GetSecret(ctx, mount.SecretPath)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}
	if secret == nil {
		return fmt.Errorf("secret not found: %s", mount.SecretPath)
	}

	// Get the value to write
	var data []byte
	if mount.SecretKey != "" {
		// Get specific key
		value, ok := secret.Data[mount.SecretKey]
		if !ok {
			return fmt.Errorf("key %s not found in secret %s", mount.SecretKey, mount.SecretPath)
		}
		data, err = valueToBytes(value)
		if err != nil {
			return fmt.Errorf("failed to convert value: %w", err)
		}
	} else {
		// Get all keys as JSON
		data, err = json.Marshal(secret.Data)
		if err != nil {
			return fmt.Errorf("failed to marshal secret data: %w", err)
		}
	}

	// Determine file name
	fileName := mount.FileName
	if fileName == "" {
		fileName = filepath.Base(mount.SecretPath)
		if mount.SecretKey != "" {
			fileName = mount.SecretKey
		}
	}

	// Determine file mode
	mode := os.FileMode(mount.Mode) //nolint:gosec // G115: file mode is 0-0777
	if mode == 0 {
		mode = d.config.DefaultMode
	}

	// Write file atomically
	filePath := filepath.Join(targetPath, fileName)
	tempPath := filePath + ".tmp"

	if err := os.WriteFile(tempPath, data, mode); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func valueToBytes(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case []byte:
		return val, nil
	case string:
		return []byte(val), nil
	default:
		return json.Marshal(val)
	}
}

// =============================================================================
// CSI Volume Spec Builder
// =============================================================================

// CSIVolumeSpecBuilder builds CSI volume specifications for pods.
type CSIVolumeSpecBuilder struct {
	driverName string
	secrets    []SecretMount
	readOnly   bool
}

// NewCSIVolumeSpecBuilder creates a new CSI volume spec builder.
func NewCSIVolumeSpecBuilder(driverName string) *CSIVolumeSpecBuilder {
	if driverName == "" {
		driverName = DefaultCSIDriverConfig().DriverName
	}
	return &CSIVolumeSpecBuilder{
		driverName: driverName,
		readOnly:   true,
	}
}

// WithSecrets adds secrets to mount.
func (b *CSIVolumeSpecBuilder) WithSecrets(secrets []SecretMount) *CSIVolumeSpecBuilder {
	b.secrets = secrets
	return b
}

// WithReadOnly sets the read-only flag.
func (b *CSIVolumeSpecBuilder) WithReadOnly(readOnly bool) *CSIVolumeSpecBuilder {
	b.readOnly = readOnly
	return b
}

// Build builds the CSI volume specification.
func (b *CSIVolumeSpecBuilder) Build(volumeName string) map[string]interface{} {
	secretsJSON, _ := json.Marshal(b.secrets)

	return map[string]interface{}{
		"name": volumeName,
		"csi": map[string]interface{}{
			"driver":   b.driverName,
			"readOnly": b.readOnly,
			"volumeAttributes": map[string]string{
				"secrets": string(secretsJSON),
			},
		},
	}
}

// BuildVolumeMount builds a volume mount for the CSI volume.
func (b *CSIVolumeSpecBuilder) BuildVolumeMount(volumeName, mountPath string) map[string]interface{} {
	return map[string]interface{}{
		"name":      volumeName,
		"mountPath": mountPath,
		"readOnly":  b.readOnly,
	}
}
