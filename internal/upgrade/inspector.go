package upgrade

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/version"
	"github.com/nats-io/nats.go"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ComponentInspector inspects the version of a locally running component.
type ComponentInspector interface {
	// GetVersion returns the current version of the component.
	GetVersion(ctx context.Context) (Version, error)
	// ComponentType returns the type of component this inspector handles.
	ComponentType() ComponentType
}

// SelfInspector inspects the version of the current binary (server or agent).
type SelfInspector struct {
	component ComponentType
}

// NewSelfInspector creates a new self inspector for server or agent.
func NewSelfInspector(component ComponentType) *SelfInspector {
	return &SelfInspector{component: component}
}

// GetVersion returns the version embedded in the current binary.
func (i *SelfInspector) GetVersion(ctx context.Context) (Version, error) {
	info := version.Get()
	if info.Version == "" || info.Version == "dev" {
		return Version{}, fmt.Errorf("development build - version not set (use -ldflags to set version)")
	}
	return ParseVersion(info.Version)
}

// ComponentType returns the component type.
func (i *SelfInspector) ComponentType() ComponentType {
	return i.component
}

// NATSInspector inspects the version of a connected NATS server.
type NATSInspector struct {
	conn     *nats.Conn
	endpoint string
}

// NewNATSInspector creates a new NATS inspector.
// Either provide an existing connection or an endpoint to connect to.
func NewNATSInspector(conn *nats.Conn, endpoint string) *NATSInspector {
	return &NATSInspector{
		conn:     conn,
		endpoint: endpoint,
	}
}

// GetVersion returns the version of the connected NATS server.
func (i *NATSInspector) GetVersion(ctx context.Context) (Version, error) {
	conn := i.conn

	// If no existing connection, create a temporary one
	if conn == nil {
		if i.endpoint == "" {
			return Version{}, fmt.Errorf("no NATS connection or endpoint provided")
		}

		var err error
		conn, err = nats.Connect(i.endpoint,
			nats.Timeout(10*time.Second),
			nats.Name("version-inspector"),
		)
		if err != nil {
			return Version{}, fmt.Errorf("connecting to NATS: %w", err)
		}
		defer conn.Close()
	}

	// Get server info
	serverVersion := conn.ConnectedServerVersion()
	if serverVersion == "" {
		return Version{}, fmt.Errorf("could not determine NATS server version")
	}

	return ParseVersion(serverVersion)
}

// ComponentType returns ComponentNATS.
func (i *NATSInspector) ComponentType() ComponentType {
	return ComponentNATS
}

// DatabaseInspector inspects the schema version of the database.
type DatabaseInspector struct {
	db         *sql.DB
	driverName string
}

// NewDatabaseInspector creates a new database inspector.
func NewDatabaseInspector(db *sql.DB, driverName string) *DatabaseInspector {
	return &DatabaseInspector{
		db:         db,
		driverName: driverName,
	}
}

// GetVersion returns the database schema version.
// It first checks for a schema_version table, then falls back to database version.
func (i *DatabaseInspector) GetVersion(ctx context.Context) (Version, error) {
	if i.db == nil {
		return Version{}, fmt.Errorf("no database connection provided")
	}

	// Try to read from schema_version table (if migrations are tracked)
	var schemaVersion string
	err := i.db.QueryRowContext(ctx,
		"SELECT version FROM schema_version ORDER BY applied_at DESC LIMIT 1",
	).Scan(&schemaVersion)
	if err == nil && schemaVersion != "" {
		return ParseVersion(schemaVersion)
	}

	// Try kscore_metadata table
	err = i.db.QueryRowContext(ctx,
		"SELECT value FROM kscore_metadata WHERE key = 'schema_version'",
	).Scan(&schemaVersion)
	if err == nil && schemaVersion != "" {
		return ParseVersion(schemaVersion)
	}

	// Fall back to database engine version
	switch i.driverName {
	case "sqlite", "sqlite3":
		err = i.db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&schemaVersion)
	case "postgres", "postgresql":
		err = i.db.QueryRowContext(ctx, "SELECT version()").Scan(&schemaVersion)
		if err == nil {
			// Parse PostgreSQL version string: "PostgreSQL 14.5 on ..."
			parts := strings.Fields(schemaVersion)
			if len(parts) >= 2 {
				schemaVersion = parts[1]
			}
		}
	default:
		return Version{}, fmt.Errorf("unsupported database driver: %s", i.driverName)
	}

	if err != nil {
		return Version{}, fmt.Errorf("querying database version: %w", err)
	}

	// For SQLite version like "3.39.0", convert to semver
	return ParseVersion(schemaVersion)
}

// ComponentType returns ComponentDatabase.
func (i *DatabaseInspector) ComponentType() ComponentType {
	return ComponentDatabase
}

// EtcdInspector inspects the version of an etcd cluster.
type EtcdInspector struct {
	client    *clientv3.Client
	endpoints []string
}

// NewEtcdInspector creates a new etcd inspector.
// Either provide an existing client or endpoints to connect to.
func NewEtcdInspector(client *clientv3.Client, endpoints []string) *EtcdInspector {
	return &EtcdInspector{
		client:    client,
		endpoints: endpoints,
	}
}

// GetVersion returns the version of the etcd cluster.
func (i *EtcdInspector) GetVersion(ctx context.Context) (Version, error) {
	client := i.client

	// If no existing client, create a temporary one
	if client == nil {
		if len(i.endpoints) == 0 {
			return Version{}, fmt.Errorf("no etcd client or endpoints provided")
		}

		var err error
		client, err = clientv3.New(clientv3.Config{
			Endpoints:   i.endpoints,
			DialTimeout: 10 * time.Second,
		})
		if err != nil {
			return Version{}, fmt.Errorf("connecting to etcd: %w", err)
		}
		defer client.Close()
	}

	// Get cluster status from the first responding endpoint
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	endpoints := client.Endpoints()
	if len(endpoints) == 0 {
		return Version{}, fmt.Errorf("no etcd endpoints available")
	}

	// Try each endpoint until one responds
	var lastErr error
	for _, endpoint := range endpoints {
		resp, err := client.Status(timeoutCtx, endpoint)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.Version == "" {
			lastErr = fmt.Errorf("empty version from endpoint %s", endpoint)
			continue
		}

		return ParseVersion(resp.Version)
	}

	if lastErr != nil {
		return Version{}, fmt.Errorf("querying etcd version: %w", lastErr)
	}
	return Version{}, fmt.Errorf("could not determine etcd version")
}

// ComponentType returns ComponentEtcd.
func (i *EtcdInspector) ComponentType() ComponentType {
	return ComponentEtcd
}

// BinaryInspector inspects the version of an external binary by running it with --version.
type BinaryInspector struct {
	binaryPath string
	component  ComponentType
	versionArg string
}

// NewBinaryInspector creates a new binary inspector.
func NewBinaryInspector(binaryPath string, component ComponentType) *BinaryInspector {
	return &BinaryInspector{
		binaryPath: binaryPath,
		component:  component,
		versionArg: "--version",
	}
}

// SetVersionArg sets the argument to pass to get version (default: --version).
func (i *BinaryInspector) SetVersionArg(arg string) {
	i.versionArg = arg
}

// GetVersion runs the binary with --version and parses the output.
func (i *BinaryInspector) GetVersion(ctx context.Context) (Version, error) {
	cmd := exec.CommandContext(ctx, i.binaryPath, i.versionArg)
	output, err := cmd.Output()
	if err != nil {
		return Version{}, fmt.Errorf("running %s %s: %w", i.binaryPath, i.versionArg, err)
	}

	// Parse version from output - look for semver pattern
	outputStr := string(output)
	lines := strings.Split(outputStr, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Try to find a version number in the line
		parts := strings.Fields(line)
		for _, part := range parts {
			// Clean up common prefixes
			part = strings.TrimPrefix(part, "v")
			part = strings.TrimPrefix(part, "V")
			if v, err := ParseVersion(part); err == nil {
				return v, nil
			}
		}
	}

	return Version{}, fmt.Errorf("could not parse version from output: %s", outputStr)
}

// ComponentType returns the component type.
func (i *BinaryInspector) ComponentType() ComponentType {
	return i.component
}

// InspectorRegistry holds component inspectors.
type InspectorRegistry struct {
	inspectors map[ComponentType]ComponentInspector
}

// NewInspectorRegistry creates a new inspector registry.
func NewInspectorRegistry() *InspectorRegistry {
	return &InspectorRegistry{
		inspectors: make(map[ComponentType]ComponentInspector),
	}
}

// Register adds an inspector to the registry.
func (r *InspectorRegistry) Register(inspector ComponentInspector) {
	r.inspectors[inspector.ComponentType()] = inspector
}

// Get returns the inspector for a component type.
func (r *InspectorRegistry) Get(component ComponentType) (ComponentInspector, bool) {
	inspector, ok := r.inspectors[component]
	return inspector, ok
}

// GetVersion gets the version for a component using the registered inspector.
func (r *InspectorRegistry) GetVersion(ctx context.Context, component ComponentType) (Version, error) {
	inspector, ok := r.inspectors[component]
	if !ok {
		return Version{}, fmt.Errorf("no inspector registered for component: %s", component)
	}
	return inspector.GetVersion(ctx)
}
