package mcp

import (
	"context"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewServer_ProfileFiltering(t *testing.T) {
	cfg := &MCPConfig{
		Server:   ServerConfig{Address: "localhost:9443"},
		Auth:     AuthConfig{Method: "apikey", APIKey: "key"},
		Features: FeaturesConfig{DefaultProfile: "read_only"},
	}

	registrar := func(srv *sdkmcp.Server, profile Profile) {
		// agent_list is allowed in read_only
		type noArgs struct{}
		AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "agent_list", Description: "list agents"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args noArgs) (*sdkmcp.CallToolResult, any, error) {
				r, _ := TextResult("ok")
				return r, nil, nil
			},
		)
		// exec_run is NOT allowed in read_only
		AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "exec_run", Description: "run commands"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args noArgs) (*sdkmcp.CallToolResult, any, error) {
				r, _ := TextResult("ok")
				return r, nil, nil
			},
		)
	}

	srv := NewServer(cfg, nil, []ToolRegistrar{registrar}, nil)
	if srv.RegisteredProfile() != ProfileReadOnly {
		t.Errorf("profile = %q, want read_only", srv.RegisteredProfile())
	}
	if srv.SDKServer() == nil {
		t.Error("SDKServer() should not be nil")
	}
}

func TestNewServer_DefaultProfile(t *testing.T) {
	cfg := &MCPConfig{
		Server: ServerConfig{Address: "localhost:9443"},
		Auth:   AuthConfig{Method: "apikey", APIKey: "key"},
	}
	srv := NewServer(cfg, nil, nil, nil)
	if srv.RegisteredProfile() != ProfileOpsSafe {
		t.Errorf("default profile = %q, want ops_safe", srv.RegisteredProfile())
	}
}

func TestNewServer_WithResources(t *testing.T) {
	cfg := &MCPConfig{
		Server: ServerConfig{Address: "localhost:9443"},
		Auth:   AuthConfig{Method: "apikey", APIKey: "key"},
	}

	resources := []ResourceRegistration{
		{
			Resource: &sdkmcp.Resource{URI: "keystone://agents"},
			Handler: func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
				return &sdkmcp.ReadResourceResult{
					Contents: []*sdkmcp.ResourceContents{{URI: "keystone://agents", Text: "[]"}},
				}, nil
			},
		},
	}

	srv := NewServer(cfg, nil, nil, resources)
	if srv.SDKServer() == nil {
		t.Error("SDKServer() should not be nil")
	}
}

func TestTextResult(t *testing.T) {
	result, err := TextResult("hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text != "hello" {
		t.Errorf("text = %q, want hello", tc.Text)
	}
}

func TestErrorResult(t *testing.T) {
	result, err := ErrorResult(context.DeadlineExceeded)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	tc, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Text != "Error: context deadline exceeded" {
		t.Errorf("text = %q", tc.Text)
	}
}

func TestUntrustedTextResult(t *testing.T) {
	result, err := UntrustedTextResult("agent output")
	if err != nil {
		t.Fatal(err)
	}
	tc, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if tc.Annotations == nil {
		t.Fatal("expected annotations")
	}
	if len(tc.Annotations.Audience) != 1 || tc.Annotations.Audience[0] != "user" {
		t.Error("expected audience=[user]")
	}
}
