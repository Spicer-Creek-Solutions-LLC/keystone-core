package tools

import (
	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
)

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
