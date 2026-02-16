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

type execRunArgs struct {
	Command string `json:"command" jsonschema:"command to execute on agents"`
	Target  string `json:"target" jsonschema:"target expression to select agents (e.g. os:linux)"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"execution timeout in seconds (default: server-configured)"`
}

type execStatusArgs struct {
	BatchJobID string `json:"batch_job_id" jsonschema:"batch job ID to check status for"`
}

type execHistoryArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of commands to return (default 20)"`
}

func execRegistrar(client *mcpserver.GRPCClient, maxTargetCount int) mcpserver.ToolRegistrar {
	return func(srv *sdkmcp.Server, profile mcpserver.Profile) {
		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "exec_run", Description: "Execute a command on targeted agents"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args execRunArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "exec_run")

				if args.Command == "" {
					return mcpserver.Err3(mcpserver.ErrorResult(fmt.Errorf("command is required")))
				}
				if args.Target == "" {
					return mcpserver.Err3(mcpserver.ErrorResult(fmt.Errorf("target is required")))
				}

				grpcReq := &pb.BatchExecuteCommandRequest{
					Target:  args.Target,
					Command: args.Command,
				}
				if args.Timeout > 0 {
					grpcReq.Timeout = clampInt32(args.Timeout)
				}

				stream, err := client.ControlPlane.BatchExecuteCommand(ctx, grpcReq)
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}

				type execResult struct {
					BatchJobID string `json:"batch_job_id,omitempty"`
					AgentID    string `json:"agent_id,omitempty"`
					Type       string `json:"type"`
					Output     string `json:"output,omitempty"`
					ExitCode   int32  `json:"exit_code"`
					Error      string `json:"error,omitempty"`
				}

				var results []execResult
				for {
					resp, err := stream.Recv()
					if errors.Is(err, io.EOF) {
						break
					}
					if err != nil {
						return mcpserver.Err3(mcpserver.ErrorResult(fmt.Errorf("stream error: %w", err)))
					}
					results = append(results, execResult{
						BatchJobID: resp.BatchJobId,
						AgentID:    resp.AgentId,
						Type:       resp.Type.String(),
						Output:     string(resp.Data),
						ExitCode:   resp.ExitCode,
						Error:      resp.Error,
					})
				}

				data, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}
				return mcpserver.Err3(mcpserver.UntrustedTextResult(fmt.Sprintf("Executed (%d responses):\n%s", len(results), string(data))))
			},
		)

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "exec_status", Description: "Check the status of a running batch execution job"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args execStatusArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "exec_status")

				if args.BatchJobID == "" {
					return mcpserver.Err3(mcpserver.ErrorResult(fmt.Errorf("batch_job_id is required")))
				}

				resp, err := client.ControlPlane.GetBatchJobStatus(ctx, &pb.GetBatchJobStatusRequest{BatchJobId: args.BatchJobID})
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}

				data, err := json.MarshalIndent(resp, "", "  ")
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}
				return mcpserver.Err3(mcpserver.TextResult(string(data)))
			},
		)

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "exec_history", Description: "List recent command execution history"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args execHistoryArgs) (*sdkmcp.CallToolResult, any, error) {
				ctx = client.WithTool(ctx, "exec_history")

				pageSize := int32(20)
				if args.Limit > 0 {
					pageSize = clampInt32(args.Limit)
				}

				resp, err := client.ControlPlane.ListCommands(ctx, &pb.ListCommandsRequest{PageSize: pageSize})
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}

				data, err := json.MarshalIndent(resp.Commands, "", "  ")
				if err != nil {
					return mcpserver.Err3(mcpserver.ErrorResult(err))
				}
				return mcpserver.Err3(mcpserver.TextResult(fmt.Sprintf("Last %d commands:\n%s", len(resp.Commands), string(data))))
			},
		)
	}
}
