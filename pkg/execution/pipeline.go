package execution

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// PipelineStage represents a single stage in a command pipeline
type PipelineStage struct {
	ID          string            // Unique identifier for this stage
	Command     string            // Command to execute
	Args        []string          // Arguments for direct execution
	Shell       Shell             // Shell to use (nil means direct execution)
	ShellType   ShellType         // Shell type (alternative to Shell)
	Env         map[string]string // Environment variables
	WorkingDir  string            // Working directory
	Timeout     time.Duration     // Timeout for this stage
	Transform   TransformFunc     // Optional transform function for output
	FailOnError bool              // If true, pipeline stops on stage failure (default: true)
}

// TransformFunc transforms the output of one stage before passing to the next
type TransformFunc func(input []byte) ([]byte, error)

// Pipeline represents a sequence of commands where output flows between stages
type Pipeline struct {
	ID              string
	Stages          []*PipelineStage
	GlobalTimeout   time.Duration     // Overall pipeline timeout
	StopOnError     bool              // Stop pipeline on first error (default: true)
	ContinueOnError bool              // Continue and collect all errors
	Env             map[string]string // Global environment variables
}

// PipelineResult represents the result of a complete pipeline execution
type PipelineResult struct {
	PipelineID   string
	StageResults []*StageResult
	FinalOutput  []byte
	Success      bool
	StartTime    time.Time
	EndTime      time.Time
	Error        error
}

// StageResult represents the result of a single pipeline stage
type StageResult struct {
	StageID   string
	StageIndex int
	Input     []byte
	Output    []byte
	Stderr    []byte
	ExitCode  int
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Skipped   bool // True if stage was skipped due to earlier failure
}

// PipelineHandler is called for each stage completion
type PipelineHandler func(stageIndex int, stageID string, result *StageResult)

// PipelineExecutor executes command pipelines
type PipelineExecutor struct {
	executor *Executor
	mu       sync.RWMutex
	running  map[string]*runningPipeline
}

type runningPipeline struct {
	cancel context.CancelFunc
	stages []string
}

// PipelineExecutorOptions configures the pipeline executor
type PipelineExecutorOptions struct {
	Executor *Executor
}

// NewPipelineExecutor creates a new pipeline executor
func NewPipelineExecutor(opts *PipelineExecutorOptions) *PipelineExecutor {
	var executor *Executor
	if opts != nil && opts.Executor != nil {
		executor = opts.Executor
	} else {
		executor = NewExecutor(nil)
	}

	return &PipelineExecutor{
		executor: executor,
		running:  make(map[string]*runningPipeline),
	}
}

// Execute runs a command pipeline
func (pe *PipelineExecutor) Execute(ctx context.Context, pipeline *Pipeline, handler PipelineHandler) (*PipelineResult, error) {
	if pipeline == nil {
		return nil, fmt.Errorf("pipeline cannot be nil")
	}
	if len(pipeline.Stages) == 0 {
		return nil, fmt.Errorf("pipeline must have at least one stage")
	}

	result := &PipelineResult{
		PipelineID:   pipeline.ID,
		StageResults: make([]*StageResult, len(pipeline.Stages)),
		StartTime:    time.Now(),
		Success:      true,
	}

	// Apply global timeout
	var cancel context.CancelFunc
	pipelineCtx := ctx
	if pipeline.GlobalTimeout > 0 {
		pipelineCtx, cancel = context.WithTimeout(ctx, pipeline.GlobalTimeout)
		defer cancel()
	} else {
		pipelineCtx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	// Track running pipeline
	pe.mu.Lock()
	stageIDs := make([]string, len(pipeline.Stages))
	for i, stage := range pipeline.Stages {
		stageIDs[i] = stage.ID
	}
	pe.running[pipeline.ID] = &runningPipeline{
		cancel: cancel,
		stages: stageIDs,
	}
	pe.mu.Unlock()

	defer func() {
		pe.mu.Lock()
		delete(pe.running, pipeline.ID)
		pe.mu.Unlock()
	}()

	// Execute stages
	var currentInput []byte
	stopOnError := pipeline.StopOnError || (!pipeline.StopOnError && !pipeline.ContinueOnError)

	for i, stage := range pipeline.Stages {
		// Check for context cancellation
		select {
		case <-pipelineCtx.Done():
			result.Error = pipelineCtx.Err()
			result.Success = false
			result.EndTime = time.Now()
			// Mark remaining stages as skipped
			for j := i; j < len(pipeline.Stages); j++ {
				if result.StageResults[j] == nil {
					result.StageResults[j] = &StageResult{
						StageID:    pipeline.Stages[j].ID,
						StageIndex: j,
						Skipped:    true,
					}
				}
			}
			return result, result.Error
		default:
		}

		// Execute stage
		stageResult := pe.executeStage(pipelineCtx, pipeline, stage, i, currentInput)
		result.StageResults[i] = stageResult

		// Call handler
		if handler != nil {
			handler(i, stage.ID, stageResult)
		}

		// Check for failure
		if stageResult.Error != nil || stageResult.ExitCode != 0 {
			result.Success = false
			failOnError := stage.FailOnError || (stage.FailOnError == false && i == 0) // Default true for first stage
			if stopOnError && failOnError {
				result.Error = fmt.Errorf("pipeline failed at stage %d (%s): %v", i, stage.ID, stageResult.Error)
				// Mark remaining stages as skipped
				for j := i + 1; j < len(pipeline.Stages); j++ {
					result.StageResults[j] = &StageResult{
						StageID:    pipeline.Stages[j].ID,
						StageIndex: j,
						Skipped:    true,
					}
				}
				break
			}
		}

		// Pass output to next stage (apply transform if specified)
		currentInput = stageResult.Output
		if stage.Transform != nil && stageResult.Error == nil {
			transformed, err := stage.Transform(currentInput)
			if err != nil {
				stageResult.Error = fmt.Errorf("transform failed: %w", err)
				result.Success = false
				if stopOnError {
					result.Error = stageResult.Error
					break
				}
			} else {
				currentInput = transformed
			}
		}
	}

	result.FinalOutput = currentInput
	result.EndTime = time.Now()

	return result, result.Error
}

// executeStage executes a single pipeline stage
func (pe *PipelineExecutor) executeStage(ctx context.Context, pipeline *Pipeline, stage *PipelineStage, index int, input []byte) *StageResult {
	result := &StageResult{
		StageID:    stage.ID,
		StageIndex: index,
		Input:      input,
		StartTime:  time.Now(),
	}

	// Merge environment variables (global + stage-specific)
	env := make(map[string]string)
	for k, v := range pipeline.Env {
		env[k] = v
	}
	for k, v := range stage.Env {
		env[k] = v
	}

	// Add pipeline metadata to environment
	env["PIPELINE_ID"] = pipeline.ID
	env["STAGE_ID"] = stage.ID
	env["STAGE_INDEX"] = fmt.Sprintf("%d", index)

	// Create execute request
	req := &ExecuteRequest{
		CommandID:  fmt.Sprintf("%s-%s-%d", pipeline.ID, stage.ID, index),
		Command:    stage.Command,
		Args:       stage.Args,
		Shell:      stage.Shell,
		ShellType:  stage.ShellType,
		Env:        env,
		WorkingDir: stage.WorkingDir,
		Timeout:    stage.Timeout,
	}

	// If we have input from previous stage, we need to pipe it
	if len(input) > 0 {
		// For shell commands, we use stdin redirection
		// For direct execution, we'd need a different approach
		// We'll handle this via a wrapper approach
		result = pe.executeWithInput(ctx, req, input)
	} else {
		cmdResult, _ := pe.executor.Execute(ctx, req, nil)
		result.Output = cmdResult.Stdout
		result.Stderr = cmdResult.Stderr
		result.ExitCode = cmdResult.ExitCode
		result.Error = cmdResult.Error
		result.EndTime = cmdResult.EndTime
	}

	return result
}

// executeWithInput executes a command with stdin input
func (pe *PipelineExecutor) executeWithInput(ctx context.Context, req *ExecuteRequest, input []byte) *StageResult {
	result := &StageResult{
		StageID:   req.CommandID,
		Input:     input,
		StartTime: time.Now(),
	}

	// For commands that support stdin, we create a wrapper
	// that provides the input via stdin
	var stdoutBuf, stderrBuf bytes.Buffer

	// Create the shell command that reads from stdin
	shell := req.Shell
	if shell == nil && req.ShellType != "" {
		var err error
		shell, err = GetShell(req.ShellType)
		if err != nil {
			result.Error = err
			result.EndTime = time.Now()
			return result
		}
	}

	var cmd string
	var args []string
	if shell != nil {
		cmd, args = shell.Command(req.Command)
	} else {
		cmd = req.Command
		args = req.Args
	}

	// Create command context with timeout
	cmdCtx := ctx
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	// Use exec.CommandContext with stdin
	execCmd := exec.CommandContext(cmdCtx, cmd, args...)
	execCmd.Stdin = bytes.NewReader(input)
	execCmd.Stdout = &stdoutBuf
	execCmd.Stderr = &stderrBuf

	if req.WorkingDir != "" {
		execCmd.Dir = req.WorkingDir
	}

	err := execCmd.Run()
	result.EndTime = time.Now()
	result.Output = stdoutBuf.Bytes()
	result.Stderr = stderrBuf.Bytes()

	if err != nil {
		if exitErr, ok := err.(interface{ ExitCode() int }); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
			result.Error = err
		}
	}

	return result
}

// CancelPipeline cancels a running pipeline
func (pe *PipelineExecutor) CancelPipeline(pipelineID string) error {
	pe.mu.RLock()
	running, exists := pe.running[pipelineID]
	pe.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pipeline %s not found", pipelineID)
	}

	if running.cancel != nil {
		running.cancel()
	}

	return nil
}

// GetRunningPipelines returns IDs of currently running pipelines
func (pe *PipelineExecutor) GetRunningPipelines() []string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	ids := make([]string, 0, len(pe.running))
	for id := range pe.running {
		ids = append(ids, id)
	}
	return ids
}

// PipelineBuilder provides a fluent interface for building pipelines
type PipelineBuilder struct {
	pipeline *Pipeline
}

// NewPipelineBuilder creates a new pipeline builder
func NewPipelineBuilder(id string) *PipelineBuilder {
	return &PipelineBuilder{
		pipeline: &Pipeline{
			ID:          id,
			Stages:      make([]*PipelineStage, 0),
			StopOnError: true,
		},
	}
}

// AddStage adds a stage to the pipeline
func (pb *PipelineBuilder) AddStage(stage *PipelineStage) *PipelineBuilder {
	if stage.ID == "" {
		stage.ID = fmt.Sprintf("stage-%d", len(pb.pipeline.Stages))
	}
	pb.pipeline.Stages = append(pb.pipeline.Stages, stage)
	return pb
}

// AddCommand adds a simple command stage
func (pb *PipelineBuilder) AddCommand(id, command string) *PipelineBuilder {
	return pb.AddStage(&PipelineStage{
		ID:      id,
		Command: command,
	})
}

// AddShellCommand adds a shell command stage
func (pb *PipelineBuilder) AddShellCommand(id string, shellType ShellType, command string) *PipelineBuilder {
	return pb.AddStage(&PipelineStage{
		ID:        id,
		Command:   command,
		ShellType: shellType,
	})
}

// WithGlobalTimeout sets the global pipeline timeout
func (pb *PipelineBuilder) WithGlobalTimeout(timeout time.Duration) *PipelineBuilder {
	pb.pipeline.GlobalTimeout = timeout
	return pb
}

// WithEnv sets global environment variables
func (pb *PipelineBuilder) WithEnv(env map[string]string) *PipelineBuilder {
	pb.pipeline.Env = env
	return pb
}

// StopOnError configures whether to stop on first error
func (pb *PipelineBuilder) StopOnError(stop bool) *PipelineBuilder {
	pb.pipeline.StopOnError = stop
	return pb
}

// ContinueOnError configures whether to continue collecting errors
func (pb *PipelineBuilder) ContinueOnError(cont bool) *PipelineBuilder {
	pb.pipeline.ContinueOnError = cont
	return pb
}

// Build returns the configured pipeline
func (pb *PipelineBuilder) Build() *Pipeline {
	return pb.pipeline
}

// PipelineResult methods

// Duration returns the total execution time
func (pr *PipelineResult) Duration() time.Duration {
	return pr.EndTime.Sub(pr.StartTime)
}

// SuccessfulStages returns the number of successful stages
func (pr *PipelineResult) SuccessfulStages() int {
	count := 0
	for _, stage := range pr.StageResults {
		if stage != nil && !stage.Skipped && stage.Error == nil && stage.ExitCode == 0 {
			count++
		}
	}
	return count
}

// FailedStages returns the number of failed stages
func (pr *PipelineResult) FailedStages() int {
	count := 0
	for _, stage := range pr.StageResults {
		if stage != nil && !stage.Skipped && (stage.Error != nil || stage.ExitCode != 0) {
			count++
		}
	}
	return count
}

// SkippedStages returns the number of skipped stages
func (pr *PipelineResult) SkippedStages() int {
	count := 0
	for _, stage := range pr.StageResults {
		if stage != nil && stage.Skipped {
			count++
		}
	}
	return count
}

// GetStageResult returns the result for a specific stage by ID
func (pr *PipelineResult) GetStageResult(stageID string) *StageResult {
	for _, stage := range pr.StageResults {
		if stage != nil && stage.StageID == stageID {
			return stage
		}
	}
	return nil
}

// CollectErrors returns all errors from the pipeline
func (pr *PipelineResult) CollectErrors() []error {
	var errors []error
	if pr.Error != nil {
		errors = append(errors, pr.Error)
	}
	for _, stage := range pr.StageResults {
		if stage != nil && stage.Error != nil {
			errors = append(errors, fmt.Errorf("stage %s: %w", stage.StageID, stage.Error))
		}
	}
	return errors
}

// StageResult methods

// Duration returns the stage execution time
func (sr *StageResult) Duration() time.Duration {
	return sr.EndTime.Sub(sr.StartTime)
}

// OutputString returns the output as a string
func (sr *StageResult) OutputString() string {
	return string(sr.Output)
}

// StderrString returns stderr as a string
func (sr *StageResult) StderrString() string {
	return string(sr.Stderr)
}

// InputString returns input as a string
func (sr *StageResult) InputString() string {
	return string(sr.Input)
}

// StreamingPipeline provides a pipeline that streams data between stages
type StreamingPipeline struct {
	ID          string
	Stages      []*StreamingStage
	Timeout     time.Duration
	StopOnError bool
}

// StreamingStage represents a stage in a streaming pipeline
type StreamingStage struct {
	ID        string
	Command   string
	Args      []string
	ShellType ShellType
	Env       map[string]string
	WorkDir   string
}

// StreamingPipelineResult represents the result of a streaming pipeline
type StreamingPipelineResult struct {
	PipelineID string
	Output     io.Reader
	Stderr     io.Reader
	Error      error
	Wait       func() error
}

// CreateStreamingPipeline creates connected commands using io.Pipe
func (pe *PipelineExecutor) CreateStreamingPipeline(ctx context.Context, pipeline *StreamingPipeline) (*StreamingPipelineResult, error) {
	if pipeline == nil || len(pipeline.Stages) == 0 {
		return nil, fmt.Errorf("pipeline must have at least one stage")
	}

	result := &StreamingPipelineResult{
		PipelineID: pipeline.ID,
	}

	// Create pipes between stages
	var readers []io.Reader
	var writers []io.Writer
	for i := 0; i < len(pipeline.Stages)-1; i++ {
		r, w := io.Pipe()
		readers = append(readers, r)
		writers = append(writers, w)
	}

	// Final output buffer
	var finalStdout, finalStderr bytes.Buffer

	// Error collection
	var mu sync.Mutex
	var firstError error
	var wg sync.WaitGroup

	// Start all stages
	for i, stage := range pipeline.Stages {
		wg.Add(1)
		go func(index int, s *StreamingStage) {
			defer wg.Done()

			shell, _ := GetShell(s.ShellType)
			var cmd string
			var args []string
			if shell != nil {
				cmd, args = shell.Command(s.Command)
			} else {
				cmd = s.Command
				args = s.Args
			}

			execCmd := exec.CommandContext(ctx, cmd, args...)

			// Set up input
			if index > 0 {
				execCmd.Stdin = readers[index-1]
			}

			// Set up output
			if index < len(pipeline.Stages)-1 {
				execCmd.Stdout = writers[index]
			} else {
				execCmd.Stdout = &finalStdout
			}
			execCmd.Stderr = &finalStderr

			if s.WorkDir != "" {
				execCmd.Dir = s.WorkDir
			}

			err := execCmd.Run()
			if err != nil {
				mu.Lock()
				if firstError == nil {
					firstError = fmt.Errorf("stage %s failed: %w", s.ID, err)
				}
				mu.Unlock()
			}

			// Close write end of pipe when done
			if index < len(pipeline.Stages)-1 {
				if closer, ok := writers[index].(io.Closer); ok {
					closer.Close()
				}
			}
		}(i, stage)
	}

	// Wait function
	result.Wait = func() error {
		wg.Wait()
		return firstError
	}

	result.Output = &finalStdout
	result.Stderr = &finalStderr

	return result, nil
}
