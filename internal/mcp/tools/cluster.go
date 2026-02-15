package tools

import (
	"context"
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

type clusterStatusArgs struct{}

func clusterRegistrar(client *mcpserver.GRPCClient) mcpserver.ToolRegistrar {
	return func(srv *sdkmcp.Server, profile mcpserver.Profile) {
		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "cluster_status", Description: "Get cluster health overview including version, uptime, and agent count"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args clusterStatusArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "cluster_status")

				resp, err := client.ControlPlane.GetServerStatus(ctx, &pb.GetServerStatusRequest{})
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}

				status := map[string]any{"status": "unknown"}
				if resp.Status != nil {
					status = map[string]any{
						"version":          resp.Status.Version,
						"uptime_seconds":   resp.Status.UptimeSeconds,
						"connected_agents": resp.Status.ConnectedAgents,
						"total_agents":     resp.Status.TotalAgents,
						"event_rate":       resp.Status.EventRate,
						"api_request_rate": resp.Status.ApiRequestRate,
						"memory_usage_mb":  resp.Status.MemoryUsageMb,
						"goroutine_count":  resp.Status.GoroutineCount,
					}
				}

				data, err := json.MarshalIndent(status, "", "  ")
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				r, _ := mcpserver.TextResult(string(data))
				return r, nil, nil
			},
		)
	}
}
