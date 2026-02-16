package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

type stateCheckArgs struct {
	Target string `json:"target" jsonschema:"target expression to select agents"`
}

type stateDriftArgs struct {
	Target string `json:"target" jsonschema:"target expression to select agents"`
}

type stateHistoryArgs struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"optional filter by agent ID"`
	Limit   int    `json:"limit,omitempty" jsonschema:"page size (default 20)"`
}

type stateApplyArgs struct {
	Target string `json:"target" jsonschema:"target expression to select agents"`
}

func stateRegistrar(client *mcpserver.GRPCClient) mcpserver.ToolRegistrar {
	return func(srv *sdkmcp.Server, profile mcpserver.Profile) {
		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "state_check", Description: "Dry-run state check: show what would change on targeted agents"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args stateCheckArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "state_check")
				if args.Target == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("target is required"))
					return r, nil, nil
				}
				resp, err := client.State.CheckState(ctx, &pb.CheckStateRequest{Target: args.Target})
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

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "state_drift", Description: "Detect configuration drift from desired state"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args stateDriftArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "state_drift")
				if args.Target == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("target is required"))
					return r, nil, nil
				}
				resp, err := client.State.DetectDrift(ctx, &pb.DetectDriftRequest{Target: args.Target})
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

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "state_history", Description: "Get state apply history with optional filtering"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args stateHistoryArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "state_history")
				grpcReq := &pb.GetStateHistoryRequest{PageSize: 20}
				if args.Limit > 0 {
					grpcReq.PageSize = clampInt32(args.Limit)
				}
				if args.AgentID != "" {
					grpcReq.AgentId = args.AgentID
				}
				resp, err := client.State.GetStateHistory(ctx, grpcReq)
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

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "state_apply", Description: "Apply state declarations to targeted agents"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args stateApplyArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "state_apply")
				if args.Target == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("target is required"))
					return r, nil, nil
				}
				stream, err := client.State.ApplyState(ctx, &pb.ApplyStateRequest{Target: args.Target})
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				type applyResult struct {
					RunID   string `json:"run_id"`
					AgentID string `json:"agent_id,omitempty"`
					Type    string `json:"type"`
					Error   string `json:"error,omitempty"`
				}
				var results []applyResult
				for {
					resp, err := stream.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						r, _ := mcpserver.ErrorResult(fmt.Errorf("stream error: %w", err))
						return r, nil, nil
					}
					results = append(results, applyResult{
						RunID:   resp.RunId,
						AgentID: resp.AgentId,
						Type:    resp.Type.String(),
						Error:   resp.Error,
					})
				}
				data, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				r, _ := mcpserver.TextResult(fmt.Sprintf("Applied state (%d responses):\n%s", len(results), string(data)))
				return r, nil, nil
			},
		)
	}
}
