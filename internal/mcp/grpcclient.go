package mcp

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// apiKeyCredentials implements grpc.PerRPCCredentials for API key auth.
type apiKeyCredentials struct {
	key            string
	requireTLS     bool
	metadataKey    string
	extraMetadata  map[string]string
}

func (c *apiKeyCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	md := map[string]string{
		c.metadataKey: c.key,
	}
	for k, v := range c.extraMetadata {
		md[k] = v
	}
	return md, nil
}

func (c *apiKeyCredentials) RequireTransportSecurity() bool {
	return c.requireTLS
}

// jwtCredentials implements grpc.PerRPCCredentials for JWT bearer auth.
type jwtCredentials struct {
	token          string
	requireTLS     bool
	extraMetadata  map[string]string
}

func (c *jwtCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	md := map[string]string{
		"authorization": "Bearer " + c.token,
	}
	for k, v := range c.extraMetadata {
		md[k] = v
	}
	return md, nil
}

func (c *jwtCredentials) RequireTransportSecurity() bool {
	return c.requireTLS
}

// GRPCClient wraps gRPC service clients with credential pass-through and
// MCP metadata injection.
type GRPCClient struct {
	conn           *grpc.ClientConn
	ControlPlane   pb.ControlPlaneServiceClient
	State          pb.StateServiceClient
	Event          pb.EventServiceClient
	sessionID      string
	aiClient       string
	restBaseURL    string
}

// NewGRPCClient creates a gRPC connection to kscore-server using the operator's
// credentials from the MCP config.
func NewGRPCClient(cfg *Config, sessionID, aiClient string) (*GRPCClient, error) {
	var dialOpts []grpc.DialOption

	useTLS := cfg.Auth.TLSCACert != "" || cfg.Auth.TLSCert != "" || cfg.Auth.Method == "mtls"

	if useTLS {
		tlsCfg, err := cfg.BuildTLSConfig()
		if err != nil {
			return nil, fmt.Errorf("building TLS config: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Add per-RPC credentials based on auth method.
	switch cfg.Auth.Method {
	case "apikey":
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(&apiKeyCredentials{
			key:         cfg.Auth.APIKey,
			requireTLS:  useTLS,
			metadataKey: "x-api-key",
		}))
	case "jwt":
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(&jwtCredentials{
			token:      cfg.Auth.JWTToken,
			requireTLS: useTLS,
		}))
	case "mtls":
		// mTLS uses the transport credentials configured above.
	}

	conn, err := grpc.NewClient(cfg.Server.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", cfg.Server.Address, err)
	}

	return &GRPCClient{
		conn:         conn,
		ControlPlane: pb.NewControlPlaneServiceClient(conn),
		State:        pb.NewStateServiceClient(conn),
		Event:        pb.NewEventServiceClient(conn),
		sessionID:    sessionID,
		aiClient:     aiClient,
		restBaseURL:  cfg.Server.RESTBaseURL,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *GRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// WithTool returns a context with MCP metadata headers for the given tool name.
func (c *GRPCClient) WithTool(ctx context.Context, toolName string) context.Context {
	return InjectMCPMetadata(ctx, c.sessionID, toolName, c.aiClient)
}

// RESTBaseURL returns the configured REST API base URL for runbook/webhook operations.
func (c *GRPCClient) RESTBaseURL() string {
	return c.restBaseURL
}

// NewGRPCClientForTest constructs a GRPCClient with injected service clients
// and settings. Intended for use in tests only.
func NewGRPCClientForTest(
	cp pb.ControlPlaneServiceClient,
	st pb.StateServiceClient,
	ev pb.EventServiceClient,
	restBaseURL string,
) *GRPCClient {
	return &GRPCClient{
		ControlPlane: cp,
		State:        st,
		Event:        ev,
		sessionID:    "test-session",
		aiClient:     "test-client",
		restBaseURL:  restBaseURL,
	}
}
