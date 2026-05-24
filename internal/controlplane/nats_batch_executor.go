// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// NATSBatchExecutor is the BatchExecutor implementation that fans
// per-agent commands over NATS and waits on the per-correlation-ID
// response delivered by ResponseRouter. It's the production runner
// the orchestrator (ExecuteBatch) uses once the server is wired.
type NATSBatchExecutor struct {
	dispatcher *CommandDispatcher
	router     *ResponseRouter
	timeout    time.Duration
}

// NATSBatchExecutorConfig configures a NATSBatchExecutor.
type NATSBatchExecutorConfig struct {
	Dispatcher *CommandDispatcher
	Router     *ResponseRouter
	// DefaultTimeout is applied when ctx has no deadline. Per-call
	// req.Timeout (via TimeoutSeconds on the dispatch path) is the
	// agent-side cap; this one bounds the server-side wait so a
	// missing response never wedges a worker goroutine.
	DefaultTimeout time.Duration
}

// NewNATSBatchExecutor validates cfg and returns an executor.
func NewNATSBatchExecutor(cfg NATSBatchExecutorConfig) (*NATSBatchExecutor, error) {
	if cfg.Dispatcher == nil {
		return nil, errors.New("controlplane: NATSBatchExecutor requires a CommandDispatcher")
	}
	if cfg.Router == nil {
		return nil, errors.New("controlplane: NATSBatchExecutor requires a ResponseRouter")
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 5 * time.Minute
	}
	return &NATSBatchExecutor{
		dispatcher: cfg.Dispatcher,
		router:     cfg.Router,
		timeout:    cfg.DefaultTimeout,
	}, nil
}

// Execute publishes the command to the target agent via the dispatcher
// and waits for the correlated response on the router. The returned
// BatchAgentResultRecord carries stdout / stderr / exit / error /
// timing / truncation flags so the dispatcher's RecordAgentResult call
// persists the full picture.
//
// Go errors are returned only for dispatch-time failures (agent
// unknown, NATS publish failed). Wait-time failures (timeout, ctx
// cancel, agent reject) produce a populated result with Success=false
// instead.
func (e *NATSBatchExecutor) Execute(
	ctx context.Context, batchID, agentID, command string, args []string,
) (state.BatchAgentResultRecord, error) {
	startedAt := time.Now()

	cmdID, err := e.dispatcher.Dispatch(ctx, DispatchRequest{
		AgentID:   agentID,
		Command:   command,
		Args:      args,
		Principal: "batch:" + batchID,
	})
	if err != nil {
		return state.BatchAgentResultRecord{}, fmt.Errorf("controlplane: dispatch %s/%s: %w", batchID, agentID, err)
	}

	ch, cancel := e.router.register(cmdID)
	defer cancel()

	waitCtx, waitCancel := context.WithTimeout(ctx, e.timeout)
	defer waitCancel()

	select {
	case <-waitCtx.Done():
		err := waitCtx.Err()
		reason := "ctx cancelled"
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			reason = "response timeout"
		case errors.Is(err, context.Canceled):
			reason = "cancelled"
		}
		return state.BatchAgentResultRecord{
			BatchJobID:  batchID,
			AgentID:     agentID,
			Success:     false,
			Error:       reason,
			StartedAt:   startedAt,
			CompletedAt: time.Now(),
		}, nil
	case resp, ok := <-ch:
		if !ok {
			// Router shut down mid-wait.
			return state.BatchAgentResultRecord{
				BatchJobID:  batchID,
				AgentID:     agentID,
				Success:     false,
				Error:       "response router stopped",
				StartedAt:   startedAt,
				CompletedAt: time.Now(),
			}, nil
		}
		return responseToBatchResult(batchID, agentID, startedAt, resp), nil
	}
}

func responseToBatchResult(batchID, agentID string, startedAt time.Time, p AgentResponsePayload) state.BatchAgentResultRecord {
	rec := state.BatchAgentResultRecord{
		BatchJobID:      batchID,
		AgentID:         agentID,
		ExitCode:        p.ExitCode,
		Stdout:          p.Stdout,
		Stderr:          p.Stderr,
		StdoutTruncated: p.StdoutTruncated,
		StderrTruncated: p.StderrTruncated,
		StartedAt:       startedAt,
		CompletedAt:     time.Now(),
	}
	switch {
	case p.Rejected:
		rec.Success = false
		rec.Error = "rejected: " + p.RejectReason
	case p.TimedOut:
		rec.Success = false
		rec.Error = "agent-side timeout"
	case p.Error != "":
		rec.Success = false
		rec.Error = p.Error
	case p.ExitCode != 0:
		rec.Success = false
	default:
		rec.Success = true
	}
	return rec
}
