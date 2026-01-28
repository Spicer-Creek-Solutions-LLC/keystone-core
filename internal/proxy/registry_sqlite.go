// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents.
package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteDeviceRegistryConfig configures the SQLite device registry.
type SQLiteDeviceRegistryConfig struct {
	// Path is the path to the SQLite database file.
	Path string

	// WALMode enables Write-Ahead Logging for better concurrency.
	WALMode bool

	// BusyTimeout is the timeout in milliseconds for busy retries.
	BusyTimeout int
}

// DefaultSQLiteDeviceRegistryConfig returns a config with sensible defaults.
func DefaultSQLiteDeviceRegistryConfig() *SQLiteDeviceRegistryConfig {
	return &SQLiteDeviceRegistryConfig{
		Path:        "./data/proxy-devices.db",
		WALMode:     true,
		BusyTimeout: 5000,
	}
}

// SQLiteDeviceRegistry implements DeviceRegistry with SQLite persistence.
// Use this for production deployments where device state must survive restarts
// and can be shared across multiple proxy agent instances.
//
// Limitations vs InMemoryDeviceRegistry:
//   - Slower for high-frequency updates (disk I/O)
//   - Requires file system access
//   - Better for persistence and sharing across instances
//   - Recommended for >100 devices or when persistence is required
type SQLiteDeviceRegistry struct {
	db   *sql.DB
	path string

	// observers are notified of device changes
	observers []DeviceObserver
	obsMu     sync.RWMutex
}

// NewSQLiteDeviceRegistry creates a new SQLite-backed device registry.
func NewSQLiteDeviceRegistry(config *SQLiteDeviceRegistryConfig) (*SQLiteDeviceRegistry, error) {
	if config == nil {
		config = DefaultSQLiteDeviceRegistryConfig()
	}

	path := config.Path
	if path == "" {
		path = "./data/proxy-devices.db"
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Build connection string with options
	connStr := path
	params := []string{}

	if config.WALMode {
		params = append(params, "_journal_mode=WAL")
	}

	if config.BusyTimeout > 0 {
		params = append(params, fmt.Sprintf("_timeout=%d", config.BusyTimeout))
	}

	if len(params) > 0 {
		connStr = fmt.Sprintf("%s?%s", path, strings.Join(params, "&"))
	}

	// Open database
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings for SQLite
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)

	r := &SQLiteDeviceRegistry{
		db:        db,
		path:      path,
		observers: make([]DeviceObserver, 0),
	}

	// Initialize schema
	if err := r.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return r, nil
}

// initSchema creates the database schema for proxied devices.
func (r *SQLiteDeviceRegistry) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS proxied_devices (
		id TEXT PRIMARY KEY,
		proxy_agent_id TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		vendor TEXT,
		model TEXT,
		profile_id TEXT,
		protocol TEXT NOT NULL,
		address TEXT NOT NULL,
		port INTEGER,
		credential_ref TEXT,
		metadata TEXT,
		labels TEXT,
		status TEXT NOT NULL,
		status_message TEXT,
		last_seen TIMESTAMP,
		health_check_interval INTEGER,
		registered_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_proxied_devices_proxy_agent_id ON proxied_devices(proxy_agent_id);
	CREATE INDEX IF NOT EXISTS idx_proxied_devices_status ON proxied_devices(status);
	CREATE INDEX IF NOT EXISTS idx_proxied_devices_type ON proxied_devices(type);
	CREATE INDEX IF NOT EXISTS idx_proxied_devices_protocol ON proxied_devices(protocol);
	`

	_, err := r.db.Exec(schema)
	return err
}

// Close closes the database connection.
func (r *SQLiteDeviceRegistry) Close() error {
	return r.db.Close()
}

// AddObserver adds an observer to be notified of device changes.
func (r *SQLiteDeviceRegistry) AddObserver(observer DeviceObserver) {
	r.obsMu.Lock()
	defer r.obsMu.Unlock()
	r.observers = append(r.observers, observer)
}

// RemoveObserver removes an observer.
func (r *SQLiteDeviceRegistry) RemoveObserver(observer DeviceObserver) {
	r.obsMu.Lock()
	defer r.obsMu.Unlock()
	for i, obs := range r.observers {
		if obs == observer {
			r.observers = append(r.observers[:i], r.observers[i+1:]...)
			return
		}
	}
}

// notifyRegistered notifies observers of a device registration.
func (r *SQLiteDeviceRegistry) notifyRegistered(device *ProxiedDevice) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceRegistered(device)
	}
}

// notifyUnregistered notifies observers of a device unregistration.
func (r *SQLiteDeviceRegistry) notifyUnregistered(deviceID string) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceUnregistered(deviceID)
	}
}

// notifyUpdated notifies observers of a device update.
func (r *SQLiteDeviceRegistry) notifyUpdated(device *ProxiedDevice) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceUpdated(device)
	}
}

// notifyStatusChanged notifies observers of a device status change.
func (r *SQLiteDeviceRegistry) notifyStatusChanged(deviceID string, oldStatus, newStatus DeviceStatus) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceStatusChanged(deviceID, oldStatus, newStatus)
	}
}

// Register registers a new proxied device.
func (r *SQLiteDeviceRegistry) Register(ctx context.Context, device *ProxiedDevice) error {
	if err := device.Validate(); err != nil {
		return err
	}

	// Check if device already exists
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM proxied_devices WHERE id = ?", device.ID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check device existence: %w", err)
	}
	if count > 0 {
		return ErrDeviceAlreadyExists
	}

	now := time.Now()
	clone := device.Clone()
	clone.RegisteredAt = now
	clone.UpdatedAt = now
	if clone.Status == "" {
		clone.Status = DeviceStatusUnknown
	}

	metadataJSON, err := json.Marshal(clone.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	labelsJSON, err := json.Marshal(clone.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO proxied_devices (
			id, proxy_agent_id, name, type, vendor, model, profile_id,
			protocol, address, port, credential_ref, metadata, labels,
			status, status_message, last_seen, health_check_interval,
			registered_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		clone.ID, clone.ProxyAgentID, clone.Name, string(clone.Type),
		clone.Vendor, clone.Model, clone.ProfileID, string(clone.Protocol),
		clone.Address, clone.Port, clone.CredentialRef, string(metadataJSON),
		string(labelsJSON), string(clone.Status), clone.StatusMessage,
		nullableTime(clone.LastSeen), clone.HealthCheckInterval.Nanoseconds(),
		clone.RegisteredAt, clone.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert device: %w", err)
	}

	go r.notifyRegistered(clone.Clone())

	return nil
}

// Unregister removes a device from the registry.
func (r *SQLiteDeviceRegistry) Unregister(ctx context.Context, deviceID string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM proxied_devices WHERE id = ?", deviceID)
	if err != nil {
		return fmt.Errorf("failed to delete device: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrDeviceNotFound
	}

	go r.notifyUnregistered(deviceID)

	return nil
}

// Get retrieves a device by ID.
func (r *SQLiteDeviceRegistry) Get(ctx context.Context, deviceID string) (*ProxiedDevice, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, proxy_agent_id, name, type, vendor, model, profile_id,
			protocol, address, port, credential_ref, metadata, labels,
			status, status_message, last_seen, health_check_interval,
			registered_at, updated_at
		FROM proxied_devices WHERE id = ?
	`, deviceID)

	device, err := r.scanDevice(row)
	if err == sql.ErrNoRows {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return device, nil
}

// scanDevice scans a database row into a ProxiedDevice.
func (r *SQLiteDeviceRegistry) scanDevice(row *sql.Row) (*ProxiedDevice, error) {
	var (
		device              ProxiedDevice
		deviceType          string
		protocol            string
		status              string
		metadataJSON        string
		labelsJSON          string
		lastSeen            sql.NullTime
		healthCheckInterval int64
	)

	err := row.Scan(
		&device.ID, &device.ProxyAgentID, &device.Name, &deviceType,
		&device.Vendor, &device.Model, &device.ProfileID, &protocol,
		&device.Address, &device.Port, &device.CredentialRef, &metadataJSON,
		&labelsJSON, &status, &device.StatusMessage, &lastSeen,
		&healthCheckInterval, &device.RegisteredAt, &device.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	device.Type = DeviceType(deviceType)
	device.Protocol = ProtocolType(protocol)
	device.Status = DeviceStatus(status)
	device.HealthCheckInterval = time.Duration(healthCheckInterval)

	if lastSeen.Valid {
		device.LastSeen = lastSeen.Time
	}

	if metadataJSON != "" && metadataJSON != "null" {
		if err := json.Unmarshal([]byte(metadataJSON), &device.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}
	if device.Metadata == nil {
		device.Metadata = make(map[string]string)
	}

	if labelsJSON != "" && labelsJSON != "null" {
		if err := json.Unmarshal([]byte(labelsJSON), &device.Labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
		}
	}
	if device.Labels == nil {
		device.Labels = make(map[string]string)
	}

	return &device, nil
}

// scanDeviceRows scans multiple database rows into a slice of ProxiedDevices.
func (r *SQLiteDeviceRegistry) scanDeviceRows(rows *sql.Rows) ([]*ProxiedDevice, error) {
	var devices []*ProxiedDevice

	for rows.Next() {
		var (
			device              ProxiedDevice
			deviceType          string
			protocol            string
			status              string
			metadataJSON        string
			labelsJSON          string
			lastSeen            sql.NullTime
			healthCheckInterval int64
		)

		err := rows.Scan(
			&device.ID, &device.ProxyAgentID, &device.Name, &deviceType,
			&device.Vendor, &device.Model, &device.ProfileID, &protocol,
			&device.Address, &device.Port, &device.CredentialRef, &metadataJSON,
			&labelsJSON, &status, &device.StatusMessage, &lastSeen,
			&healthCheckInterval, &device.RegisteredAt, &device.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		device.Type = DeviceType(deviceType)
		device.Protocol = ProtocolType(protocol)
		device.Status = DeviceStatus(status)
		device.HealthCheckInterval = time.Duration(healthCheckInterval)

		if lastSeen.Valid {
			device.LastSeen = lastSeen.Time
		}

		if metadataJSON != "" && metadataJSON != "null" {
			if err := json.Unmarshal([]byte(metadataJSON), &device.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}
		if device.Metadata == nil {
			device.Metadata = make(map[string]string)
		}

		if labelsJSON != "" && labelsJSON != "null" {
			if err := json.Unmarshal([]byte(labelsJSON), &device.Labels); err != nil {
				return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
			}
		}
		if device.Labels == nil {
			device.Labels = make(map[string]string)
		}

		devices = append(devices, &device)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return devices, nil
}

// List lists devices with optional filtering.
func (r *SQLiteDeviceRegistry) List(ctx context.Context, filter *DeviceFilter) ([]*ProxiedDevice, error) {
	query := `
		SELECT id, proxy_agent_id, name, type, vendor, model, profile_id,
			protocol, address, port, credential_ref, metadata, labels,
			status, status_message, last_seen, health_check_interval,
			registered_at, updated_at
		FROM proxied_devices
	`

	args := []interface{}{}
	conditions := []string{}

	if filter != nil {
		if len(filter.IDs) > 0 {
			placeholders := make([]string, len(filter.IDs))
			for i, id := range filter.IDs {
				placeholders[i] = "?"
				args = append(args, id)
			}
			conditions = append(conditions, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
		}

		if filter.ProxyAgentID != "" {
			conditions = append(conditions, "proxy_agent_id = ?")
			args = append(args, filter.ProxyAgentID)
		}

		if len(filter.Types) > 0 {
			placeholders := make([]string, len(filter.Types))
			for i, t := range filter.Types {
				placeholders[i] = "?"
				args = append(args, string(t))
			}
			conditions = append(conditions, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
		}

		if len(filter.Vendors) > 0 {
			placeholders := make([]string, len(filter.Vendors))
			for i, v := range filter.Vendors {
				placeholders[i] = "?"
				args = append(args, v)
			}
			conditions = append(conditions, fmt.Sprintf("vendor IN (%s)", strings.Join(placeholders, ",")))
		}

		if len(filter.Protocols) > 0 {
			placeholders := make([]string, len(filter.Protocols))
			for i, p := range filter.Protocols {
				placeholders[i] = "?"
				args = append(args, string(p))
			}
			conditions = append(conditions, fmt.Sprintf("protocol IN (%s)", strings.Join(placeholders, ",")))
		}

		if len(filter.Statuses) > 0 {
			placeholders := make([]string, len(filter.Statuses))
			for i, s := range filter.Statuses {
				placeholders[i] = "?"
				args = append(args, string(s))
			}
			conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY id"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %w", err)
	}
	defer rows.Close()

	devices, err := r.scanDeviceRows(rows)
	if err != nil {
		return nil, err
	}

	// Handle label filtering in memory (complex to do in SQL with JSON)
	if filter != nil && len(filter.Labels) > 0 {
		var filtered []*ProxiedDevice
		for _, device := range devices {
			match := true
			for key, value := range filter.Labels {
				if deviceValue, exists := device.Labels[key]; !exists || deviceValue != value {
					match = false
					break
				}
			}
			if match {
				filtered = append(filtered, device)
			}
		}
		devices = filtered
	}

	// Sort by ID for consistent ordering
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].ID < devices[j].ID
	})

	// Apply offset and limit
	if filter != nil {
		if filter.Offset > 0 {
			if filter.Offset >= len(devices) {
				return []*ProxiedDevice{}, nil
			}
			devices = devices[filter.Offset:]
		}
		if filter.Limit > 0 && filter.Limit < len(devices) {
			devices = devices[:filter.Limit]
		}
	}

	return devices, nil
}

// Update updates device metadata, status, or configuration.
func (r *SQLiteDeviceRegistry) Update(ctx context.Context, device *ProxiedDevice) error {
	if err := device.Validate(); err != nil {
		return err
	}

	// Get existing device to preserve registration time
	existing, err := r.Get(ctx, device.ID)
	if err != nil {
		return err
	}

	clone := device.Clone()
	clone.RegisteredAt = existing.RegisteredAt
	clone.UpdatedAt = time.Now()

	metadataJSON, err := json.Marshal(clone.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	labelsJSON, err := json.Marshal(clone.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE proxied_devices SET
			proxy_agent_id = ?, name = ?, type = ?, vendor = ?, model = ?,
			profile_id = ?, protocol = ?, address = ?, port = ?,
			credential_ref = ?, metadata = ?, labels = ?, status = ?,
			status_message = ?, last_seen = ?, health_check_interval = ?,
			updated_at = ?
		WHERE id = ?
	`,
		clone.ProxyAgentID, clone.Name, string(clone.Type), clone.Vendor,
		clone.Model, clone.ProfileID, string(clone.Protocol), clone.Address,
		clone.Port, clone.CredentialRef, string(metadataJSON), string(labelsJSON),
		string(clone.Status), clone.StatusMessage, nullableTime(clone.LastSeen),
		clone.HealthCheckInterval.Nanoseconds(), clone.UpdatedAt, clone.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update device: %w", err)
	}

	go r.notifyUpdated(clone.Clone())

	return nil
}

// GetByProxyAgent returns all devices for a specific proxy agent.
func (r *SQLiteDeviceRegistry) GetByProxyAgent(ctx context.Context, proxyAgentID string) ([]*ProxiedDevice, error) {
	return r.List(ctx, &DeviceFilter{ProxyAgentID: proxyAgentID})
}

// UpdateStatus updates only the device status.
func (r *SQLiteDeviceRegistry) UpdateStatus(ctx context.Context, deviceID string, status DeviceStatus, message string) error {
	// Get old status for notification
	var oldStatus string
	err := r.db.QueryRowContext(ctx, "SELECT status FROM proxied_devices WHERE id = ?", deviceID).Scan(&oldStatus)
	if err == sql.ErrNoRows {
		return ErrDeviceNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get device status: %w", err)
	}

	now := time.Now()
	var lastSeenUpdate string
	var args []interface{}

	if status == DeviceStatusOnline {
		lastSeenUpdate = ", last_seen = ?"
		args = []interface{}{string(status), message, now, now, deviceID}
	} else {
		args = []interface{}{string(status), message, now, deviceID}
	}

	setClauses := []string{"status = ?", "status_message = ?", "updated_at = ?"}
	if lastSeenUpdate != "" {
		setClauses = append(setClauses, "last_seen = ?")
	}
	query := "UPDATE proxied_devices SET " + strings.Join(setClauses, ", ") + " WHERE id = ?" // nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- setClauses built from fixed column map with bound args

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update device status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrDeviceNotFound
	}

	if oldStatus != string(status) {
		go r.notifyStatusChanged(deviceID, DeviceStatus(oldStatus), status)
	}

	return nil
}

// Count returns the total number of registered devices.
func (r *SQLiteDeviceRegistry) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM proxied_devices").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count devices: %w", err)
	}
	return count, nil
}

// GetDevicesByStatus returns devices grouped by status.
func (r *SQLiteDeviceRegistry) GetDevicesByStatus(ctx context.Context) (map[DeviceStatus][]*ProxiedDevice, error) {
	devices, err := r.List(ctx, nil)
	if err != nil {
		return nil, err
	}

	result := make(map[DeviceStatus][]*ProxiedDevice)
	for _, device := range devices {
		result[device.Status] = append(result[device.Status], device)
	}

	return result, nil
}

// GetOnlineDevices returns all devices that are currently online.
func (r *SQLiteDeviceRegistry) GetOnlineDevices(ctx context.Context) ([]*ProxiedDevice, error) {
	return r.List(ctx, &DeviceFilter{Statuses: []DeviceStatus{DeviceStatusOnline}})
}

// GetOfflineDevices returns all devices that are currently offline or unreachable.
func (r *SQLiteDeviceRegistry) GetOfflineDevices(ctx context.Context) ([]*ProxiedDevice, error) {
	return r.List(ctx, &DeviceFilter{
		Statuses: []DeviceStatus{DeviceStatusOffline, DeviceStatusUnreachable},
	})
}

// GetStaleDevices returns devices that haven't been seen for longer than the threshold.
func (r *SQLiteDeviceRegistry) GetStaleDevices(ctx context.Context, threshold time.Duration) ([]*ProxiedDevice, error) {
	cutoff := time.Now().Add(-threshold)

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, proxy_agent_id, name, type, vendor, model, profile_id,
			protocol, address, port, credential_ref, metadata, labels,
			status, status_message, last_seen, health_check_interval,
			registered_at, updated_at
		FROM proxied_devices
		WHERE last_seen IS NOT NULL AND last_seen < ?
		ORDER BY id
	`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale devices: %w", err)
	}
	defer rows.Close()

	return r.scanDeviceRows(rows)
}

// Clear removes all devices from the registry.
func (r *SQLiteDeviceRegistry) Clear(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM proxied_devices")
	if err != nil {
		return fmt.Errorf("failed to clear devices: %w", err)
	}
	return nil
}

// GetStats returns statistics about registered devices.
func (r *SQLiteDeviceRegistry) GetStats(ctx context.Context) (*RegistryStats, error) {
	devices, err := r.List(ctx, nil)
	if err != nil {
		return nil, err
	}

	stats := &RegistryStats{
		TotalDevices: len(devices),
		ByType:       make(map[DeviceType]int),
		ByProtocol:   make(map[ProtocolType]int),
		ByProxyAgent: make(map[string]int),
	}

	for _, device := range devices {
		// Count by status
		switch device.Status {
		case DeviceStatusOnline:
			stats.OnlineDevices++
		case DeviceStatusOffline, DeviceStatusUnreachable:
			stats.OfflineDevices++
		case DeviceStatusDegraded:
			stats.DegradedDevices++
		default:
			stats.UnknownDevices++
		}

		// Count by type
		stats.ByType[device.Type]++

		// Count by protocol
		stats.ByProtocol[device.Protocol]++

		// Count by proxy agent
		stats.ByProxyAgent[device.ProxyAgentID]++
	}

	return stats, nil
}

// nullableTime converts a time.Time to a value suitable for SQL.
// Returns nil for zero times.
func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// Ensure SQLiteDeviceRegistry implements DeviceRegistry.
var _ DeviceRegistry = (*SQLiteDeviceRegistry)(nil)
