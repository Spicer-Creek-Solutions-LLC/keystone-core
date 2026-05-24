// SPDX-License-Identifier: Apache-2.0

package execution

import (
	"context"
	"errors"
	"fmt"
)

// ErrPipelineFailed is the sentinel returned by Pipeline.Run when
// FailFast is set and a stage doesn't succeed. The wrapped error has
// the failing stage index for diagnostics; callers can reach for the
// per-stage result in the returned slice.
var ErrPipelineFailed = errors.New("execution: pipeline stage failed")

// Stage is one step in a Pipeline. Transform, when non-nil, mutates
// the previous stage's stdout before it becomes this stage's stdin —
// useful for trimming, JSON re-shaping, etc. A nil Transform pipes
// stdout through unchanged.
type Stage struct {
	Request   ExecuteRequest
	Transform func(stdout []byte) []byte
}

// PipelineConfig configures a Pipeline. Executor and Stages are
// required (Stages may be empty — Run becomes a no-op).
type PipelineConfig struct {
	Executor Executor
	Stages   []Stage
	// FailFast stops execution at the first stage whose result does
	// not Succeed. Default false: every stage runs and the caller
	// inspects results to decide.
	FailFast bool
}

// Pipeline runs a sequence of execution stages, piping each stage's
// stdout into the next stage's stdin. Stderr is not piped — each
// stage's stderr stays in its own ExecuteResult.
//
// PROJECT-DETAILS §4.7 marks Pipeline as a "rare external use"
// primitive that underlies blueprint apply (Epic 15). Tests use it
// directly; blueprints will reach for it indirectly.
type Pipeline struct {
	exec     Executor
	stages   []Stage
	failFast bool
}

// NewPipeline returns a Pipeline. Executor must be non-nil; Stages
// may be empty or nil (Run becomes a no-op).
func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if cfg.Executor == nil {
		return nil, errors.New("execution: pipeline: nil executor")
	}
	stages := cfg.Stages
	if stages == nil {
		stages = []Stage{}
	}
	return &Pipeline{
		exec:     cfg.Executor,
		stages:   stages,
		failFast: cfg.FailFast,
	}, nil
}

// Run executes stages sequentially. The returned slice has one
// ExecuteResult per stage actually run (never longer than len(Stages);
// shorter when FailFast trips or ctx fires).
//
// Errors:
//   - ErrPipelineFailed (wrapped with stage index) when FailFast is
//     true and a stage doesn't succeed.
//   - The context's error when ctx fires between stages.
//   - nil otherwise — even when FailFast is false and a stage failed,
//     since the caller already has every result for inspection.
func (p *Pipeline) Run(ctx context.Context) ([]ExecuteResult, error) {
	if len(p.stages) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	results := make([]ExecuteResult, 0, len(p.stages))
	var prevStdout []byte
	for i, stage := range p.stages {
		req := stage.Request
		if i > 0 {
			if stage.Transform != nil {
				req.StdinInput = stage.Transform(prevStdout)
			} else {
				req.StdinInput = prevStdout
			}
		}

		res := p.exec.Execute(ctx, req)
		results = append(results, res)
		prevStdout = res.Stdout

		if !res.Succeeded() && p.failFast {
			return results, fmt.Errorf("%w: stage %d (%s)", ErrPipelineFailed, i, stage.Request.Command)
		}
		if i+1 < len(p.stages) {
			if err := ctx.Err(); err != nil {
				return results, err
			}
		}
	}
	return results, nil
}
