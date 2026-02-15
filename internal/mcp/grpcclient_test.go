package mcp

import (
	"context"
	"testing"
)

func TestAPIKeyCredentials_GetRequestMetadata(t *testing.T) {
	cred := &apiKeyCredentials{
		key:         "test-key",
		requireTLS:  false,
		metadataKey: "x-api-key",
	}
	md, err := cred.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if md["x-api-key"] != "test-key" {
		t.Errorf("got %q, want test-key", md["x-api-key"])
	}
}

func TestAPIKeyCredentials_RequireTransportSecurity(t *testing.T) {
	cred := &apiKeyCredentials{requireTLS: true}
	if !cred.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity=true")
	}
	cred.requireTLS = false
	if cred.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity=false")
	}
}

func TestJWTCredentials_GetRequestMetadata(t *testing.T) {
	cred := &jwtCredentials{
		token:      "my-jwt",
		requireTLS: false,
	}
	md, err := cred.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if md["authorization"] != "Bearer my-jwt" {
		t.Errorf("got %q, want Bearer my-jwt", md["authorization"])
	}
}

func TestJWTCredentials_RequireTransportSecurity(t *testing.T) {
	cred := &jwtCredentials{requireTLS: true}
	if !cred.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity=true")
	}
}

func TestGRPCClient_WithTool(t *testing.T) {
	client := &GRPCClient{
		sessionID: "sess-abc",
		aiClient:  "claude-code/1.0",
	}
	ctx := client.WithTool(context.Background(), "agent_list")

	// Verify metadata was injected via InjectMCPMetadata
	// (tested more thoroughly in metadata_test.go)
	if ctx == context.Background() {
		t.Error("WithTool should return a new context")
	}
}
