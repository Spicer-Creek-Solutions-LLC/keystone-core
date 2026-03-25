package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
)

// httpClient is a shared HTTP client with a reasonable timeout to prevent
// hung connections when the server is unresponsive.
var httpClient = &http.Client{Timeout: 30 * time.Second}

type runbookListArgs struct{}

type runbookExecuteArgs struct {
	RunbookID string `json:"runbook_id" jsonschema:"runbook ID to execute"`
}

type runbookStatusArgs struct {
	RunbookID   string `json:"runbook_id" jsonschema:"runbook ID"`
	ExecutionID string `json:"execution_id" jsonschema:"execution ID to check status for"`
}

func runbookRegistrar(client *mcpserver.GRPCClient) mcpserver.ToolRegistrar {
	return func(srv *sdkmcp.Server, profile mcpserver.Profile) {
		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "runbook_list", Description: "List available runbooks"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args runbookListArgs) (*sdkmcp.CallToolResult, any, error) {
				baseURL := client.RESTBaseURL()
				if baseURL == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("REST base URL not configured"))
					return r, nil, nil
				}
				httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/runbooks", http.NoBody)
				if err != nil {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("creating request: %w", err))
					return r, nil, nil
				}
				resp, err := httpClient.Do(httpReq)
				if err != nil {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("requesting runbooks: %w", err))
					return r, nil, nil
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				if resp.StatusCode != http.StatusOK {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body)))
					return r, nil, nil
				}
				var parsed any
				if err := json.Unmarshal(body, &parsed); err == nil {
					data, _ := json.MarshalIndent(parsed, "", "  ")
					r, _ := mcpserver.TextResult(string(data))
					return r, nil, nil
				}
				r, _ := mcpserver.TextResult(string(body))
				return r, nil, nil
			},
		)

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "runbook_execute", Description: "Execute a runbook by ID"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args runbookExecuteArgs) (*sdkmcp.CallToolResult, any, error) {
				if args.RunbookID == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("runbook_id is required"))
					return r, nil, nil
				}
				baseURL := client.RESTBaseURL()
				if baseURL == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("REST base URL not configured"))
					return r, nil, nil
				}
				execURL := fmt.Sprintf("%s/api/v1/runbooks/%s/execute", baseURL, args.RunbookID)
				httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, execURL, http.NoBody)
				if err != nil {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("creating request: %w", err))
					return r, nil, nil
				}
				httpReq.Header.Set("Content-Type", "application/json")
				httpResp, err := httpClient.Do(httpReq)
				if err != nil {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("executing runbook: %w", err))
					return r, nil, nil
				}
				defer httpResp.Body.Close()
				body, err := io.ReadAll(httpResp.Body)
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("server returned %d: %s", httpResp.StatusCode, string(body)))
					return r, nil, nil
				}
				var parsed any
				if err := json.Unmarshal(body, &parsed); err == nil {
					data, _ := json.MarshalIndent(parsed, "", "  ")
					r, _ := mcpserver.TextResult(fmt.Sprintf("Runbook %s execution started:\n%s", args.RunbookID, string(data)))
					return r, nil, nil
				}
				r, _ := mcpserver.TextResult(string(body))
				return r, nil, nil
			},
		)

		mcpserver.AddToolIfAllowed(srv, profile,
			&sdkmcp.Tool{Name: "runbook_status", Description: "Check execution status of a runbook"},
			func(ctx context.Context, req *sdkmcp.CallToolRequest, args runbookStatusArgs) (*sdkmcp.CallToolResult, any, error) {
				if args.RunbookID == "" || args.ExecutionID == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("runbook_id and execution_id are required"))
					return r, nil, nil
				}
				baseURL := client.RESTBaseURL()
				if baseURL == "" {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("REST base URL not configured"))
					return r, nil, nil
				}
				statusURL := fmt.Sprintf("%s/api/v1/runbooks/%s/executions/%s", baseURL, args.RunbookID, args.ExecutionID)
				httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, http.NoBody)
				if err != nil {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("creating request: %w", err))
					return r, nil, nil
				}
				resp, err := httpClient.Do(httpReq)
				if err != nil {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("checking runbook status: %w", err))
					return r, nil, nil
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					r, _ := mcpserver.ErrorResult(err)
					return r, nil, nil
				}
				if resp.StatusCode != http.StatusOK {
					r, _ := mcpserver.ErrorResult(fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body)))
					return r, nil, nil
				}
				var parsed any
				if err := json.Unmarshal(body, &parsed); err == nil {
					data, _ := json.MarshalIndent(parsed, "", "  ")
					r, _ := mcpserver.TextResult(string(data))
					return r, nil, nil
				}
				r, _ := mcpserver.TextResult(string(body))
				return r, nil, nil
			},
		)
	}
}
