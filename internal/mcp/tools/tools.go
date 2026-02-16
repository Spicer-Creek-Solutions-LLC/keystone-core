// Package tools implements MCP tool handlers for Keystone Core operations.
package tools

import (
	"math"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
)

// clampInt32 safely converts an int to int32, capping at MaxInt32.
func clampInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(v) //nolint:gosec // bounds checked above
}

// RegisterAll returns all MCP tool registrars. Each registrar registers
// its tools directly on the SDK server, filtered by profile.
func RegisterAll(client *mcpserver.GRPCClient, maxTargetCount int) []mcpserver.ToolRegistrar {
	return []mcpserver.ToolRegistrar{
		agentRegistrar(client),
		execRegistrar(client, maxTargetCount),
		clusterRegistrar(client),
		stateRegistrar(client),
		eventRegistrar(client),
		runbookRegistrar(client),
	}
}
