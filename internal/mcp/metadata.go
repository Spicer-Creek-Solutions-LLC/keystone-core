package mcp

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// gRPC metadata keys for MCP audit attribution.
const (
	MetaKeyClientType = "x-kscore-client-type"
	MetaKeyMCPTool    = "x-kscore-mcp-tool"
	MetaKeyMCPSession = "x-kscore-mcp-session"
	MetaKeyMCPClient  = "x-kscore-mcp-ai-client"
)

// MCPMetadataKeys is the whitelist of metadata keys captured by the server-side
// auth interceptor into Principal.Metadata.
var MCPMetadataKeys = []string{
	MetaKeyClientType,
	MetaKeyMCPTool,
	MetaKeyMCPSession,
	MetaKeyMCPClient,
}

// InjectMCPMetadata adds MCP attribution headers to outgoing gRPC context.
func InjectMCPMetadata(ctx context.Context, sessionID, toolName, aiClient string) context.Context {
	md := metadata.Pairs(
		MetaKeyClientType, "mcp",
		MetaKeyMCPTool, toolName,
		MetaKeyMCPSession, sessionID,
		MetaKeyMCPClient, aiClient,
	)
	return metadata.NewOutgoingContext(ctx, metadata.Join(
		metadataFromOutgoing(ctx),
		md,
	))
}

// metadataFromOutgoing extracts existing outgoing metadata, if any.
func metadataFromOutgoing(ctx context.Context) metadata.MD {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return metadata.MD{}
	}
	return md
}
