package tools

import (
	"context"
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

type eventQueryArgs struct {
	Type  string `json:"type,omitempty" jsonschema:"filter by event type"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of events to return (default 50)"`
}

type eventStatsArgs struct{}

func eventRegistrar(client *mcpserver.GRPCClient) mcpserver.ToolRegistrar {
	return func(srv *sdkmcp.Server, profile mcpserver.Profile) {
		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "event_query", Description: "Query events with optional filters: type, limit"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args eventQueryArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "event_query")
				grpcReq := &pb.ListEventsRequest{PageSize: 50}
				if args.Type != "" {
					grpcReq.Type = args.Type
				}
				if args.Limit > 0 {
					grpcReq.PageSize = int32(args.Limit)
				}
				resp, err := client.Event.ListEvents(ctx, grpcReq)
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				data, err := json.MarshalIndent(resp.Events, "", "  ")
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				r, _ := mcpserver.TextResult(fmt.Sprintf("Found %d events:\n%s", len(resp.Events), string(data)))
				return r, nil, nil
			},
		)

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "event_stats", Description: "Get event statistics and counts grouped by type"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args eventStatsArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "event_stats")
				resp, err := client.Event.GetEventStats(ctx, &pb.GetEventStatsRequest{})
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				data, err := json.MarshalIndent(resp, "", "  ")
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
