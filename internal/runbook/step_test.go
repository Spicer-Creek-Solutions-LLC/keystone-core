package runbook

import (
	"testing"
	"time"
)

func TestStepType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		stepType StepType
		want     bool
	}{
		{"command", StepTypeCommand, true},
		{"api", StepTypeAPI, true},
		{"notification", StepTypeNotification, true},
		{"wait", StepTypeWait, true},
		{"noop", StepTypeNoop, true},
		{"fail", StepTypeFail, true},
		{"if", StepTypeIf, true},              // Phase 2 - implemented
		{"switch", StepTypeSwitch, true},      // Phase 2 - implemented
		{"loop", StepTypeLoop, true},          // Phase 2 - implemented
		{"parallel", StepTypeParallel, true},  // Phase 2 - implemented
		{"runbook", StepTypeSubRunbook, true}, // Phase 2 - implemented
		{"approval", StepTypeApproval, true},  // Phase 3 - implemented
		{"state", StepTypeState, true},        // Phase 4 - implemented
		{"deploy", StepTypeDeploy, true},      // Phase 4 - implemented
		{"rollback", StepTypeRollback, true},  // Phase 4 - implemented
		{"script", StepTypeScript, true},      // Phase 4 - implemented
		{"invalid", StepType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stepType.IsValid(); got != tt.want {
				t.Errorf("StepType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepType_IsValidExtended(t *testing.T) {
	tests := []struct {
		name     string
		stepType StepType
		want     bool
	}{
		{"command", StepTypeCommand, true},
		{"api", StepTypeAPI, true},
		{"notification", StepTypeNotification, true},
		{"wait", StepTypeWait, true},
		{"noop", StepTypeNoop, true},
		{"fail", StepTypeFail, true},
		{"state", StepTypeState, true},
		{"approval", StepTypeApproval, true},
		{"if", StepTypeIf, true},
		{"switch", StepTypeSwitch, true},
		{"loop", StepTypeLoop, true},
		{"parallel", StepTypeParallel, true},
		{"runbook", StepTypeSubRunbook, true},
		{"script", StepTypeScript, true},
		{"query", StepTypeQuery, true},
		{"invalid", StepType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.stepType.IsValidExtended(); got != tt.want {
				t.Errorf("StepType.IsValidExtended() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepType_String(t *testing.T) {
	tests := []struct {
		stepType StepType
		want     string
	}{
		{StepTypeCommand, "command"},
		{StepTypeAPI, "api"},
		{StepTypeNotification, "notification"},
		{StepTypeWait, "wait"},
		{StepTypeNoop, "noop"},
		{StepTypeFail, "fail"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.stepType.String(); got != tt.want {
				t.Errorf("StepType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepState_IsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		state StepState
		want  bool
	}{
		{"pending", StepStatePending, false},
		{"running", StepStateRunning, false},
		{"completed", StepStateCompleted, true},
		{"failed", StepStateFailed, true},
		{"skipped", StepStateSkipped, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.want {
				t.Errorf("StepState.IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStepState_String(t *testing.T) {
	tests := []struct {
		state StepState
		want  string
	}{
		{StepStatePending, "pending"},
		{StepStateRunning, "running"},
		{StepStateCompleted, "completed"},
		{StepStateFailed, "failed"},
		{StepStateSkipped, "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("StepState.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackoffType_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		backoff BackoffType
		want    bool
	}{
		{"constant", BackoffConstant, true},
		{"linear", BackoffLinear, true},
		{"exponential", BackoffExponential, true},
		{"empty", BackoffType(""), true}, // empty defaults to constant
		{"invalid", BackoffType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.backoff.IsValid(); got != tt.want {
				t.Errorf("BackoffType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBackoffType_String(t *testing.T) {
	tests := []struct {
		backoff BackoffType
		want    string
	}{
		{BackoffConstant, "constant"},
		{BackoffLinear, "linear"},
		{BackoffExponential, "exponential"},
		{BackoffType(""), "constant"}, // empty defaults to constant
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.backoff.String(); got != tt.want {
				t.Errorf("BackoffType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputSource_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		source OutputSource
		want   bool
	}{
		{"stdout", OutputSourceStdout, true},
		{"stderr", OutputSourceStderr, true},
		{"exitCode", OutputSourceExitCode, true},
		{"json", OutputSourceJSON, true},
		{"header", OutputSourceHeader, true},
		{"body", OutputSourceBody, true},
		{"invalid", OutputSource("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.source.IsValid(); got != tt.want {
				t.Errorf("OutputSource.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputParser_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		parser OutputParser
		want   bool
	}{
		{"raw", OutputParserRaw, true},
		{"json", OutputParserJSON, true},
		{"regex", OutputParserRegex, true},
		{"line", OutputParserLine, true},
		{"jsonpath", OutputParserJSONPath, true},
		{"empty", OutputParser(""), true}, // empty defaults to raw
		{"invalid", OutputParser("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.parser.IsValid(); got != tt.want {
				t.Errorf("OutputParser.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOutputParser_String(t *testing.T) {
	tests := []struct {
		parser OutputParser
		want   string
	}{
		{OutputParserRaw, "raw"},
		{OutputParserJSON, "json"},
		{OutputParserRegex, "regex"},
		{OutputParserLine, "line"},
		{OutputParserJSONPath, "jsonpath"},
		{OutputParser(""), "raw"}, // empty defaults to raw
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.parser.String(); got != tt.want {
				t.Errorf("OutputParser.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryConfig_GetDelay(t *testing.T) {
	tests := []struct {
		name   string
		config *RetryConfig
		want   time.Duration
	}{
		{"nil config", nil, 0},
		{"empty delay", &RetryConfig{}, 0},
		{"valid delay", &RetryConfig{Delay: "5s"}, 5 * time.Second},
		{"invalid delay", &RetryConfig{Delay: "invalid"}, 0},
		{"1m delay", &RetryConfig{Delay: "1m"}, time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetDelay(); got != tt.want {
				t.Errorf("RetryConfig.GetDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryConfig_GetMaxDelay(t *testing.T) {
	tests := []struct {
		name   string
		config *RetryConfig
		want   time.Duration
	}{
		{"nil config", nil, 0},
		{"empty maxDelay", &RetryConfig{}, 0},
		{"valid maxDelay", &RetryConfig{MaxDelay: "30s"}, 30 * time.Second},
		{"invalid maxDelay", &RetryConfig{MaxDelay: "invalid"}, 0},
		{"5m maxDelay", &RetryConfig{MaxDelay: "5m"}, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetMaxDelay(); got != tt.want {
				t.Errorf("RetryConfig.GetMaxDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStep_GetTimeout(t *testing.T) {
	tests := []struct {
		name string
		step Step
		want time.Duration
	}{
		{"empty timeout", Step{}, 0},
		{"valid timeout", Step{Timeout: "10s"}, 10 * time.Second},
		{"invalid timeout", Step{Timeout: "invalid"}, 0},
		{"1h timeout", Step{Timeout: "1h"}, time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.step.GetTimeout(); got != tt.want {
				t.Errorf("Step.GetTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
