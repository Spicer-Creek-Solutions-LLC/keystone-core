package tools

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

type agentListArgs struct {
	Status string `json:"status,omitempty" jsonschema:"optional filter by agent status (AGENT_STATUS_ONLINE, AGENT_STATUS_OFFLINE, AGENT_STATUS_DEGRADED)"`
}

type agentShowArgs struct {
	AgentID string `json:"agent_id" jsonschema:"agent ID to show details for"`
}

type agentHealthArgs struct{}

func agentRegistrar(client *mcpserver.GRPCClient) mcpserver.ToolRegistrar {
	return func(srv *sdkmcp.Server, profile mcpserver.Profile) {
		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "agent_list", Description: "List registered agents with optional filtering by status"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args agentListArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "agent_list")

				grpcReq := &pb.ListAgentsRequest{}
				if args.Status != "" {
					if v, ok := pb.AgentStatus_value[args.Status]; ok {
						grpcReq.Status = pb.AgentStatus(v)
					}
				}

				resp, err := client.ControlPlane.ListAgents(ctx, grpcReq)
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}

				type agentSummary struct {
					ID            string `json:"id"`
					Hostname      string `json:"hostname"`
					OS            string `json:"os"`
					Status        string `json:"status"`
					LastHeartbeat string `json:"last_heartbeat,omitempty"`
				}

				agents := make([]agentSummary, 0, len(resp.Agents))
				for _, a := range resp.Agents {
					hostname := ""
					osName := ""
					if a.Metadata != nil {
						hostname = a.Metadata.Hostname
						osName = a.Metadata.Os
					}
					lastHB := ""
					if a.LastHeartbeat != nil {
						lastHB = a.LastHeartbeat.AsTime().String()
					}
					agents = append(agents, agentSummary{
						ID:            a.AgentId,
						Hostname:      hostname,
						OS:            osName,
						Status:        a.Status.String(),
						LastHeartbeat: lastHB,
					})
				}

				data, err := json.MarshalIndent(agents, "", "  ")
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}
				r, _ := mcpserver.TextResult(fmt.Sprintf("Found %d agents:\n%s", len(agents), string(data)))
				return r, nil, nil
			},
		)

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "agent_show", Description: "Show detailed information for a specific agent by ID"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args agentShowArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "agent_show")

				if args.AgentID == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("agent_id is required"))
					return r, nil, nil
				}

				resp, err := client.ControlPlane.GetAgent(ctx, &pb.GetAgentRequest{AgentId: args.AgentID})
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}

				data, err := json.MarshalIndent(resp.Agent, "", "  ")
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				r, _ := mcpserver.TextResult(string(data))
				return r, nil, nil
			},
		)

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "agent_health", Description: "Get cluster-wide agent health summary"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args agentHealthArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "agent_health")

				resp, err := client.ControlPlane.GetServerStatus(ctx, &pb.GetServerStatusRequest{})
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}

				summary := map[string]any{"status": "unknown"}
				if resp.Status != nil {
					summary = map[string]any{
						"version":          resp.Status.Version,
						"uptime_seconds":   resp.Status.UptimeSeconds,
						"connected_agents": resp.Status.ConnectedAgents,
						"total_agents":     resp.Status.TotalAgents,
					}
				}

				data, err := json.MarshalIndent(summary, "", "  ")
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
