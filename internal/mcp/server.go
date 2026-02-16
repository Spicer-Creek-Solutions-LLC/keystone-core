package mcp

import (
	"context"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/shawnbutts/keystone-core/pkg/version"
)

// ResourceRegistration holds a resource definition and its handler.
type ResourceRegistration struct {
	Resource *sdkmcp.Resource
	Handler  sdkmcp.ResourceHandler
}

// ToolRegistrar is a function that registers tools directly on the SDK server.
// It receives the server and profile, and only registers tools the profile allows.
type ToolRegistrar func(srv *sdkmcp.Server, profile Profile)

// Server wraps the MCP SDK server and coordinates tool registration with
// profile-based filtering.
type Server struct {
	config  *Config
	client  *GRPCClient
	profile Profile
	sdk     *sdkmcp.Server
}

// NewServer creates an MCP server with tools filtered by the configured profile.
func NewServer(cfg *Config, client *GRPCClient, registrars []ToolRegistrar, resources []ResourceRegistration) *Server {
	info := version.Get()
	sdk := sdkmcp.NewServer(
		&sdkmcp.Implementation{
			Name:    "keystone-core",
			Version: info.Version,
		},
		nil,
	)

	profile := cfg.Profile()

	for _, reg := range registrars {
		reg(sdk, profile)
	}

	for _, r := range resources {
		sdk.AddResource(r.Resource, r.Handler)
	}

	return &Server{
		config:  cfg,
		client:  client,
		profile: profile,
		sdk:     sdk,
	}
}

// Run starts the MCP server on stdio transport, blocking until the context
// is cancelled or the transport closes.
func (s *Server) Run(ctx context.Context) error {
	return s.sdk.Run(ctx, &sdkmcp.StdioTransport{})
}

// SDKServer returns the underlying MCP SDK server for testing.
func (s *Server) SDKServer() *sdkmcp.Server {
	return s.sdk
}

// RegisteredProfile returns the active profile for this server.
func (s *Server) RegisteredProfile() Profile {
	return s.profile
}

// TextResult creates a CallToolResult with a single text content item.
func TextResult(text string) (*sdkmcp.CallToolResult, error) {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: text},
		},
	}, nil
}

// ErrorResult creates a CallToolResult indicating an error.
func ErrorResult(err error) (*sdkmcp.CallToolResult, error) {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: fmt.Sprintf("Error: %v", err)},
		},
		IsError: true,
	}, nil
}

// UntrustedTextResult creates a CallToolResult with text marked as coming
// from an untrusted source (e.g., agent command output).
func UntrustedTextResult(text string) (*sdkmcp.CallToolResult, error) {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{
				Text: text,
				Annotations: &sdkmcp.Annotations{
					Audience: []sdkmcp.Role{"user"},
				},
			},
		},
	}, nil
}

// Err3 adapts a 2-return ErrorResult/TextResult into a 3-return for ToolHandlerFor.
func Err3(r *sdkmcp.CallToolResult, _ error) (*sdkmcp.CallToolResult, any, error) {
	return r, nil, nil
}

// AddToolIfAllowed registers a typed tool on the SDK server if the profile allows it.
func AddToolIfAllowed[In, Out any](srv *sdkmcp.Server, profile Profile, tool *sdkmcp.Tool, handler sdkmcp.ToolHandlerFor[In, Out]) {
	if !profile.Allows(tool.Name) {
		return
	}
	sdkmcp.AddTool(srv, tool, handler)
}
