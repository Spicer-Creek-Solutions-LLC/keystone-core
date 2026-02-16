// Package resources implements MCP resource handlers for Keystone Core.
package resources

import (
	"context"
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// RegisterAll returns all MCP resource registrations.
func RegisterAll(client *mcpserver.GRPCClient) []mcpserver.ResourceRegistration {
	return []mcpserver.ResourceRegistration{
		agentsResource(client),
		clusterStatusResource(client),
		recentEventsResource(client),
	}
}

func agentsResource(client *mcpserver.GRPCClient) mcpserver.ResourceRegistration {
	return mcpserver.ResourceRegistration{
		Resource: &sdkmcp.Resource{
			URI:         "keystone://agents",
			Name:        "Agent Inventory",
			Description: "Current agent inventory with status, OS, and last heartbeat",
			MIMEType:    "application/json",
		},
		Handler: func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
			ctx = client.WithTool(ctx, "resource:agents")
			resp, err := client.ControlPlane.ListAgents(ctx, &pb.ListAgentsRequest{})
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(resp.Agents, "", "  ")
			if err != nil {
				return nil, err
			}
			return &sdkmcp.ReadResourceResult{
				Contents: []*sdkmcp.ResourceContents{
					{URI: "keystone://agents", Text: string(data)},
				},
			}, nil
		},
	}
}

func clusterStatusResource(client *mcpserver.GRPCClient) mcpserver.ResourceRegistration {
	return mcpserver.ResourceRegistration{
		Resource: &sdkmcp.Resource{
			URI:         "keystone://cluster/status",
			Name:        "Cluster Status",
			Description: "Cluster health overview with version, uptime, and agent counts",
			MIMEType:    "application/json",
		},
		Handler: func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
			ctx = client.WithTool(ctx, "resource:cluster_status")
			resp, err := client.ControlPlane.GetServerStatus(ctx, &pb.GetServerStatusRequest{})
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(resp.Status, "", "  ")
			if err != nil {
				return nil, err
			}
			return &sdkmcp.ReadResourceResult{
				Contents: []*sdkmcp.ResourceContents{
					{URI: "keystone://cluster/status", Text: string(data)},
				},
			}, nil
		},
	}
}

func recentEventsResource(client *mcpserver.GRPCClient) mcpserver.ResourceRegistration {
	return mcpserver.ResourceRegistration{
		Resource: &sdkmcp.Resource{
			URI:         "keystone://events/recent",
			Name:        "Recent Events",
			Description: "Last 50 events for operational context",
			MIMEType:    "application/json",
		},
		Handler: func(ctx context.Context, req *sdkmcp.ReadResourceRequest) (*sdkmcp.ReadResourceResult, error) {
			ctx = client.WithTool(ctx, "resource:events_recent")
			resp, err := client.Event.ListEvents(ctx, &pb.ListEventsRequest{PageSize: 50})
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(resp.Events, "", "  ")
			if err != nil {
				return nil, err
			}
			return &sdkmcp.ReadResourceResult{
				Contents: []*sdkmcp.ResourceContents{
					{URI: "keystone://events/recent", Text: string(data)},
				},
			}, nil
		},
	}
}
