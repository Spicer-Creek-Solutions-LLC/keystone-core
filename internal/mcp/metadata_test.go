package mcp

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestInjectMCPMetadata(t *testing.T) {
	ctx := context.Background()
	ctx = InjectMCPMetadata(ctx, "sess-123", "agent_list", "claude-desktop/1.0")

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}

	tests := []struct {
		key  string
		want string
	}{
		{MetaKeyClientType, "mcp"},
		{MetaKeyMCPTool, "agent_list"},
		{MetaKeyMCPSession, "sess-123"},
		{MetaKeyMCPClient, "claude-desktop/1.0"},
	}
	for _, tt := range tests {
		got := md.Get(tt.key)
		if len(got) != 1 || got[0] != tt.want {
			t.Errorf("metadata[%s] = %v, want [%s]", tt.key, got, tt.want)
		}
	}
}

func TestInjectMCPMetadata_PreservesExisting(t *testing.T) {
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-api-key", "secret"))
	ctx = InjectMCPMetadata(ctx, "s1", "exec_run", "cursor/2.0")

	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		t.Fatal("expected outgoing metadata")
	}

	if got := md.Get("x-api-key"); len(got) != 1 || got[0] != "secret" {
		t.Errorf("existing metadata lost: got %v", got)
	}
	if got := md.Get(MetaKeyClientType); len(got) != 1 || got[0] != "mcp" {
		t.Errorf("mcp metadata missing: got %v", got)
	}
}

func TestMCPMetadataKeys(t *testing.T) {
	if len(MCPMetadataKeys) != 4 {
		t.Errorf("expected 4 MCP metadata keys, got %d", len(MCPMetadataKeys))
	}
}
