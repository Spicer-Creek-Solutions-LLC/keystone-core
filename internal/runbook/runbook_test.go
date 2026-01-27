package runbook

import (
	"testing"
)

func TestExecutionState_IsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		state ExecutionState
		want  bool
	}{
		{"pending", ExecutionStatePending, false},
		{"running", ExecutionStateRunning, false},
		{"completed", ExecutionStateCompleted, true},
		{"failed", ExecutionStateFailed, true},
		{"cancelled", ExecutionStateCancelled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.want {
				t.Errorf("ExecutionState.IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecutionState_String(t *testing.T) {
	tests := []struct {
		state ExecutionState
		want  string
	}{
		{ExecutionStatePending, "pending"},
		{ExecutionStateRunning, "running"},
		{ExecutionStateCompleted, "completed"},
		{ExecutionStateFailed, "failed"},
		{ExecutionStateCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("ExecutionState.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInputType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		inputType InputType
		want      bool
	}{
		{"string", InputTypeString, true},
		{"int", InputTypeInt, true},
		{"bool", InputTypeBool, true},
		{"float", InputTypeFloat, true},
		{"list", InputTypeList, true},
		{"map", InputTypeMap, true},
		{"invalid", InputType("invalid"), false},
		{"empty", InputType(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.inputType.IsValid(); got != tt.want {
				t.Errorf("InputType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInputType_String(t *testing.T) {
	tests := []struct {
		inputType InputType
		want      string
	}{
		{InputTypeString, "string"},
		{InputTypeInt, "int"},
		{InputTypeBool, "bool"},
		{InputTypeFloat, "float"},
		{InputTypeList, "list"},
		{InputTypeMap, "map"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.inputType.String(); got != tt.want {
				t.Errorf("InputType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
