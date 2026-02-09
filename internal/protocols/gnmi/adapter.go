package gnmi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	grpccreds "google.golang.org/grpc/credentials"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// Config holds configuration for the gNMI adapter.
type Config struct {
	*protocols.ConnectionConfig

	// Port is the gNMI gRPC port (default: 9339).
	Port int `json:"port,omitempty"`

	// Encoding is the default data encoding (default: json_ietf).
	Encoding string `json:"encoding,omitempty"`
}

// DefaultConfig returns the default gNMI configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             DefaultPort,
		Encoding:         EncodingJSONIETF,
	}
}

// Adapter implements the gNMI protocol adapter.
type Adapter struct {
	config     *Config
	grpcConn   *grpc.ClientConn
	gnmiClient gnmipb.GNMIClient
	device     *proxy.ProxiedDevice
	credential credentials.Credential
	mu         sync.RWMutex
	connected  bool
	lastError  error
	lastConnect time.Time
	metrics    *protocols.AdapterMetrics
}

var _ protocols.ProtocolAdapter = (*Adapter)(nil)
var _ protocols.GNMIAdapter = (*Adapter)(nil)

// Type returns the protocol type.
func (a *Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolGNMI
}

// Connect establishes a gRPC connection to the gNMI target.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		return fmt.Errorf("already connected to %s", device.Address)
	}

	gnmiCred, ok := cred.(*credentials.GNMICredential)
	if !ok {
		return fmt.Errorf("gNMI adapter requires GNMICredential, got %T", cred)
	}

	a.device = device
	a.credential = cred

	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}
	target := fmt.Sprintf("%s:%d", device.Address, port)

	tlsConfig, err := buildTLSConfig(gnmiCred)
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("failed to build TLS config: %w", err)
	}

	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(grpccreds.NewTLS(tlsConfig)))

	if gnmiCred.Username != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(&metadataCredentials{
			username: gnmiCred.Username,
			password: gnmiCred.Password,
		}))
	}

	dialCtx, cancel := context.WithTimeout(ctx, a.config.Timeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, target, dialOpts...) //nolint:staticcheck // DialContext is preferred for timeout control
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("failed to dial gNMI target %s: %w", target, err)
	}

	a.grpcConn = conn
	a.gnmiClient = gnmipb.NewGNMIClient(conn)
	a.connected = true
	a.lastConnect = time.Now()
	a.metrics.ConnectionCount++

	return nil
}

// Execute executes a command string against the gNMI target.
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	a.mu.RLock()
	if !a.connected {
		a.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	a.mu.RUnlock()

	start := time.Now()
	a.metrics.ExecutionCount++

	cmd := req.Command
	if len(req.Args) > 0 {
		for _, arg := range req.Args {
			cmd += " " + arg
		}
	}

	output, err := a.executeCommand(ctx, cmd)
	duration := time.Since(start)

	result := &protocols.ExecuteResult{
		StartTime: start,
		EndTime:   time.Now(),
		Duration:  duration,
	}

	if err != nil {
		a.metrics.ExecutionErrors++
		result.ExitCode = 1
		result.Error = err.Error()
		return result, err
	}

	result.Stdout = output
	a.metrics.LastActivity = time.Now()
	a.metrics.BytesReceived += int64(len(output))

	return result, nil
}

// Disconnect closes the gRPC connection.
func (a *Adapter) Disconnect(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.connected {
		return nil
	}

	var err error
	if a.grpcConn != nil {
		err = a.grpcConn.Close()
		a.grpcConn = nil
		a.gnmiClient = nil
	}

	a.connected = false
	return err
}

// HealthCheck verifies the gRPC connection is alive.
func (a *Adapter) HealthCheck(_ context.Context) (*protocols.HealthCheckResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
	}

	if !a.connected || a.grpcConn == nil {
		result.Healthy = false
		result.Status = "disconnected"
		return result, fmt.Errorf("not connected")
	}

	state := a.grpcConn.GetState()
	result.Details = map[string]interface{}{
		"grpc_state": state.String(),
	}

	switch state {
	case connectivity.Ready:
		result.Healthy = true
		result.Status = "connected"
	case connectivity.Idle:
		result.Healthy = true
		result.Status = "idle"
	default:
		result.Healthy = false
		result.Status = fmt.Sprintf("unhealthy: %s", state.String())
	}

	return result, nil
}

// IsConnected returns true if the adapter is connected.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

// Metrics returns adapter metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	return a.metrics
}

// NewAdapterFactory creates a new gNMI adapter factory.
func NewAdapterFactory(config *Config) protocols.AdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.ProtocolAdapter, error) {
		cfg := DefaultConfig()
		if config != nil {
			cfg = config
		}
		if connConfig != nil {
			cfg.ConnectionConfig = connConfig
		}
		return &Adapter{
			config:  cfg,
			metrics: &protocols.AdapterMetrics{},
		}, nil
	}
}

// NewGNMIAdapterFactory creates a new gNMI typed adapter factory.
func NewGNMIAdapterFactory(config *Config) protocols.GNMIAdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.GNMIAdapter, error) {
		cfg := DefaultConfig()
		if config != nil {
			cfg = config
		}
		if connConfig != nil {
			cfg.ConnectionConfig = connConfig
		}
		return &Adapter{
			config:  cfg,
			metrics: &protocols.AdapterMetrics{},
		}, nil
	}
}

func init() {
	protocols.Register(protocols.ProtocolGNMI, NewAdapterFactory(nil))
	protocols.RegisterGNMI(protocols.ProtocolGNMI, NewGNMIAdapterFactory(nil))
}
