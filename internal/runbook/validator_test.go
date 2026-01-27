package runbook

import (
	"strings"
	"testing"
)

func TestValidator_Validate_ValidRunbook(t *testing.T) {
	rb := &Runbook{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name: "valid-runbook",
		},
		Spec: RunbookSpec{
			Steps: []Step{
				{
					Name: "step1",
					Type: StepTypeCommand,
					Config: map[string]interface{}{
						"command": "echo hello",
					},
				},
			},
		},
	}

	v := NewValidator()
	if err := v.Validate(rb); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_Validate_MissingName(t *testing.T) {
	rb := &Runbook{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata:   Metadata{},
		Spec: RunbookSpec{
			Steps: []Step{
				{
					Name:   "step1",
					Type:   StepTypeNoop,
					Config: map[string]interface{}{},
				},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want to contain 'name is required'", err.Error())
	}
}

func TestValidator_Validate_InvalidName(t *testing.T) {
	tests := []struct {
		name    string
		runbook string
		wantErr bool
	}{
		{"valid single char", "a", false},
		{"valid simple", "my-runbook", false},
		{"valid with numbers", "test-123", false},
		{"invalid uppercase", "MyRunbook", true},
		{"invalid underscore", "my_runbook", true},
		{"invalid starts with hyphen", "-runbook", true},
		{"invalid ends with hyphen", "runbook-", true},
		{"invalid spaces", "my runbook", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: tt.runbook},
				Spec: RunbookSpec{
					Steps: []Step{
						{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_Validate_NoSteps(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "no-steps"},
		Spec:     RunbookSpec{Steps: []Step{}},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for no steps, got nil")
	}
	if !strings.Contains(err.Error(), "at least one step is required") {
		t.Errorf("error = %q, want to contain 'at least one step is required'", err.Error())
	}
}

func TestValidator_Validate_DuplicateStepNames(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "dup-steps"},
		Spec: RunbookSpec{
			Steps: []Step{
				{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
				{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for duplicate step names, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate step name") {
		t.Errorf("error = %q, want to contain 'duplicate step name'", err.Error())
	}
}

func TestValidator_Validate_InvalidStepType(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "invalid-type"},
		Spec: RunbookSpec{
			Steps: []Step{
				{Name: "step1", Type: StepType("invalid"), Config: map[string]interface{}{}},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for invalid step type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported step type") {
		t.Errorf("error = %q, want to contain 'unsupported step type'", err.Error())
	}
}

func TestValidator_Validate_FutureStepType(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "future-type"},
		Spec: RunbookSpec{
			Steps: []Step{
				{Name: "step1", Type: StepTypeQuery, Config: map[string]interface{}{}},
			},
		},
	}

	// Without AllowFutureStepTypes - StepTypeQuery is a future step type
	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for future step type, got nil")
	}

	// With AllowFutureStepTypes
	v.AllowFutureStepTypes = true
	if err := v.Validate(rb); err != nil {
		t.Errorf("Validate() with AllowFutureStepTypes error = %v, want nil", err)
	}
}

func TestValidator_Validate_InvalidTimeout(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "invalid-timeout"},
		Spec: RunbookSpec{
			Timeout: "invalid",
			Steps: []Step{
				{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for invalid timeout, got nil")
	}
	if !strings.Contains(err.Error(), "invalid duration format") {
		t.Errorf("error = %q, want to contain 'invalid duration format'", err.Error())
	}
}

func TestValidator_Validate_InvalidStepTimeout(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "invalid-step-timeout"},
		Spec: RunbookSpec{
			Steps: []Step{
				{
					Name:    "step1",
					Type:    StepTypeNoop,
					Timeout: "not-a-duration",
					Config:  map[string]interface{}{},
				},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for invalid step timeout, got nil")
	}
}

func TestValidator_Validate_MissingDependency(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "missing-dep"},
		Spec: RunbookSpec{
			Steps: []Step{
				{
					Name:      "step1",
					Type:      StepTypeNoop,
					DependsOn: []string{"nonexistent"},
					Config:    map[string]interface{}{},
				},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for missing dependency, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want to contain 'does not exist'", err.Error())
	}
}

func TestValidator_Validate_SelfDependency(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "self-dep"},
		Spec: RunbookSpec{
			Steps: []Step{
				{
					Name:      "step1",
					Type:      StepTypeNoop,
					DependsOn: []string{"step1"},
					Config:    map[string]interface{}{},
				},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for self-dependency, got nil")
	}
	if !strings.Contains(err.Error(), "cannot depend on itself") {
		t.Errorf("error = %q, want to contain 'cannot depend on itself'", err.Error())
	}
}

func TestValidator_Validate_CircularDependency(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "circular-dep"},
		Spec: RunbookSpec{
			Steps: []Step{
				{Name: "a", Type: StepTypeNoop, DependsOn: []string{"c"}, Config: map[string]interface{}{}},
				{Name: "b", Type: StepTypeNoop, DependsOn: []string{"a"}, Config: map[string]interface{}{}},
				{Name: "c", Type: StepTypeNoop, DependsOn: []string{"b"}, Config: map[string]interface{}{}},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("error = %q, want to contain 'circular dependency'", err.Error())
	}
}

func TestValidator_Validate_ValidDependencyChain(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "valid-deps"},
		Spec: RunbookSpec{
			Steps: []Step{
				{Name: "a", Type: StepTypeNoop, Config: map[string]interface{}{}},
				{Name: "b", Type: StepTypeNoop, DependsOn: []string{"a"}, Config: map[string]interface{}{}},
				{Name: "c", Type: StepTypeNoop, DependsOn: []string{"a", "b"}, Config: map[string]interface{}{}},
				{Name: "d", Type: StepTypeNoop, DependsOn: []string{"c"}, Config: map[string]interface{}{}},
			},
		},
	}

	v := NewValidator()
	if err := v.Validate(rb); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestValidator_Validate_InputValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   InputDef
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid string input",
			input:   InputDef{Name: "test", Type: InputTypeString},
			wantErr: false,
		},
		{
			name:    "missing name",
			input:   InputDef{Type: InputTypeString},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:    "invalid name",
			input:   InputDef{Name: "123invalid", Type: InputTypeString},
			wantErr: true,
			errMsg:  "must be a valid identifier",
		},
		{
			name:    "invalid type",
			input:   InputDef{Name: "test", Type: InputType("invalid")},
			wantErr: true,
			errMsg:  "invalid type",
		},
		{
			name:    "type mismatch default",
			input:   InputDef{Name: "test", Type: InputTypeInt, Default: "not an int"},
			wantErr: true,
			errMsg:  "default value type mismatch",
		},
		{
			name:    "valid int with int default",
			input:   InputDef{Name: "test", Type: InputTypeInt, Default: 42},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "input-test"},
				Spec: RunbookSpec{
					Inputs: []InputDef{tt.input},
					Steps: []Step{
						{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidator_Validate_DuplicateInputNames(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "dup-inputs"},
		Spec: RunbookSpec{
			Inputs: []InputDef{
				{Name: "input1", Type: InputTypeString},
				{Name: "input1", Type: InputTypeInt},
			},
			Steps: []Step{
				{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for duplicate input names, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate input name") {
		t.Errorf("error = %q, want to contain 'duplicate input name'", err.Error())
	}
}

func TestValidator_Validate_CommandStepConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid command",
			config:  map[string]interface{}{"command": "echo hello"},
			wantErr: false,
		},
		{
			name:    "missing command",
			config:  map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "cmd-test"},
				Spec: RunbookSpec{
					Steps: []Step{
						{Name: "step1", Type: StepTypeCommand, Config: tt.config},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_Validate_APIStepConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid api",
			config:  map[string]interface{}{"url": "http://example.com"},
			wantErr: false,
		},
		{
			name:    "valid api with method",
			config:  map[string]interface{}{"url": "http://example.com", "method": "POST"},
			wantErr: false,
		},
		{
			name:    "missing url",
			config:  map[string]interface{}{"method": "GET"},
			wantErr: true,
		},
		{
			name:    "invalid method",
			config:  map[string]interface{}{"url": "http://example.com", "method": "INVALID"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "api-test"},
				Spec: RunbookSpec{
					Steps: []Step{
						{Name: "step1", Type: StepTypeAPI, Config: tt.config},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_Validate_NotificationStepConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid notification",
			config:  map[string]interface{}{"channel": "slack", "message": "hello"},
			wantErr: false,
		},
		{
			name:    "missing channel",
			config:  map[string]interface{}{"message": "hello"},
			wantErr: true,
		},
		{
			name:    "missing message",
			config:  map[string]interface{}{"channel": "slack"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "notify-test"},
				Spec: RunbookSpec{
					Steps: []Step{
						{Name: "step1", Type: StepTypeNotification, Config: tt.config},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_Validate_WaitStepConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid duration",
			config:  map[string]interface{}{"duration": "30s"},
			wantErr: false,
		},
		{
			name:    "valid condition",
			config:  map[string]interface{}{"condition": "{{ .steps.prev.success }}"},
			wantErr: false,
		},
		{
			name:    "neither duration nor condition",
			config:  map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "invalid duration",
			config:  map[string]interface{}{"duration": "invalid"},
			wantErr: true,
		},
		{
			name:    "duration not a string",
			config:  map[string]interface{}{"duration": 123},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "wait-test"},
				Spec: RunbookSpec{
					Steps: []Step{
						{Name: "step1", Type: StepTypeWait, Config: tt.config},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_Validate_FailStepConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid fail",
			config:  map[string]interface{}{"message": "intentional failure"},
			wantErr: false,
		},
		{
			name:    "missing message",
			config:  map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "fail-test"},
				Spec: RunbookSpec{
					Steps: []Step{
						{Name: "step1", Type: StepTypeFail, Config: tt.config},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_Validate_OutputValidation(t *testing.T) {
	tests := []struct {
		name    string
		output  OutputDef
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid raw output",
			output:  OutputDef{Name: "result", Source: OutputSourceStdout},
			wantErr: false,
		},
		{
			name:    "missing name",
			output:  OutputDef{Source: OutputSourceStdout},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:    "invalid name",
			output:  OutputDef{Name: "123invalid", Source: OutputSourceStdout},
			wantErr: true,
			errMsg:  "must be a valid identifier",
		},
		{
			name:    "invalid source",
			output:  OutputDef{Name: "result", Source: OutputSource("invalid")},
			wantErr: true,
			errMsg:  "invalid output source",
		},
		{
			name:    "invalid parser",
			output:  OutputDef{Name: "result", Source: OutputSourceStdout, Parser: OutputParser("invalid")},
			wantErr: true,
			errMsg:  "invalid output parser",
		},
		{
			name:    "regex without path",
			output:  OutputDef{Name: "result", Source: OutputSourceStdout, Parser: OutputParserRegex},
			wantErr: true,
			errMsg:  "path (regex pattern) is required",
		},
		{
			name:    "regex with invalid pattern",
			output:  OutputDef{Name: "result", Source: OutputSourceStdout, Parser: OutputParserRegex, Path: "[invalid"},
			wantErr: true,
			errMsg:  "invalid regex pattern",
		},
		{
			name:    "jsonpath without path",
			output:  OutputDef{Name: "result", Source: OutputSourceStdout, Parser: OutputParserJSONPath},
			wantErr: true,
			errMsg:  "path (JSONPath expression) is required",
		},
		{
			name:    "json without path",
			output:  OutputDef{Name: "result", Source: OutputSourceStdout, Parser: OutputParserJSON},
			wantErr: true,
			errMsg:  "path (JSON key) is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "output-test"},
				Spec: RunbookSpec{
					Steps: []Step{
						{
							Name:    "step1",
							Type:    StepTypeCommand,
							Config:  map[string]interface{}{"command": "echo"},
							Outputs: []OutputDef{tt.output},
						},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidator_Validate_DuplicateOutputNames(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "dup-outputs"},
		Spec: RunbookSpec{
			Steps: []Step{
				{
					Name:   "step1",
					Type:   StepTypeCommand,
					Config: map[string]interface{}{"command": "echo"},
					Outputs: []OutputDef{
						{Name: "out1", Source: OutputSourceStdout},
						{Name: "out1", Source: OutputSourceStderr},
					},
				},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for duplicate output names, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate output name") {
		t.Errorf("error = %q, want to contain 'duplicate output name'", err.Error())
	}
}

func TestValidator_Validate_RetryConfig(t *testing.T) {
	tests := []struct {
		name    string
		retries *RetryConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid retry config",
			retries: &RetryConfig{MaxAttempts: 3, Delay: "5s", Backoff: BackoffExponential},
			wantErr: false,
		},
		{
			name:    "zero max attempts",
			retries: &RetryConfig{MaxAttempts: 0},
			wantErr: true,
			errMsg:  "maxAttempts must be at least 1",
		},
		{
			name:    "invalid delay",
			retries: &RetryConfig{MaxAttempts: 3, Delay: "invalid"},
			wantErr: true,
			errMsg:  "invalid duration format",
		},
		{
			name:    "invalid maxDelay",
			retries: &RetryConfig{MaxAttempts: 3, MaxDelay: "invalid"},
			wantErr: true,
			errMsg:  "invalid duration format",
		},
		{
			name:    "invalid backoff",
			retries: &RetryConfig{MaxAttempts: 3, Backoff: BackoffType("invalid")},
			wantErr: true,
			errMsg:  "invalid backoff type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "retry-test"},
				Spec: RunbookSpec{
					Steps: []Step{
						{
							Name:    "step1",
							Type:    StepTypeNoop,
							Retries: tt.retries,
							Config:  map[string]interface{}{},
						},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidator_Validate_NegativeMaxRetries(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "neg-retries"},
		Spec: RunbookSpec{
			MaxRetries: -1,
			Steps: []Step{
				{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	v := NewValidator()
	err := v.Validate(rb)
	if err == nil {
		t.Error("Validate() expected error for negative maxRetries, got nil")
	}
	if !strings.Contains(err.Error(), "maxRetries must be non-negative") {
		t.Errorf("error = %q, want to contain 'maxRetries must be non-negative'", err.Error())
	}
}

func TestValidator_Validate_InputValidationRules(t *testing.T) {
	minVal := float64(0)
	maxVal := float64(100)

	tests := []struct {
		name    string
		input   InputDef
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid pattern validation",
			input: InputDef{
				Name: "email",
				Type: InputTypeString,
				Validation: &InputValidation{
					Pattern: `^[a-z]+@[a-z]+\.[a-z]+$`,
				},
			},
			wantErr: false,
		},
		{
			name: "pattern on non-string",
			input: InputDef{
				Name:       "count",
				Type:       InputTypeInt,
				Validation: &InputValidation{Pattern: "[0-9]+"},
			},
			wantErr: true,
			errMsg:  "pattern validation only applies to string",
		},
		{
			name: "invalid pattern",
			input: InputDef{
				Name:       "test",
				Type:       InputTypeString,
				Validation: &InputValidation{Pattern: "[invalid"},
			},
			wantErr: true,
			errMsg:  "invalid regex pattern",
		},
		{
			name: "min/max on int",
			input: InputDef{
				Name: "percent",
				Type: InputTypeInt,
				Validation: &InputValidation{
					Min: &minVal,
					Max: &maxVal,
				},
			},
			wantErr: false,
		},
		{
			name: "min greater than max",
			input: InputDef{
				Name: "test",
				Type: InputTypeInt,
				Validation: &InputValidation{
					Min: &maxVal,
					Max: &minVal,
				},
			},
			wantErr: true,
			errMsg:  "min cannot be greater than max",
		},
		{
			name: "min/max on bool",
			input: InputDef{
				Name:       "flag",
				Type:       InputTypeBool,
				Validation: &InputValidation{Min: &minVal},
			},
			wantErr: true,
			errMsg:  "min/max validation only applies to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := &Runbook{
				Metadata: Metadata{Name: "validation-test"},
				Spec: RunbookSpec{
					Inputs: []InputDef{tt.input},
					Steps: []Step{
						{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
					},
				},
			}

			v := NewValidator()
			err := v.Validate(rb)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name string
		errs ValidationErrors
		want string
	}{
		{
			name: "empty",
			errs: ValidationErrors{},
			want: "",
		},
		{
			name: "single error",
			errs: ValidationErrors{
				{Field: "test.field", Message: "is invalid"},
			},
			want: "test.field: is invalid",
		},
		{
			name: "multiple errors",
			errs: ValidationErrors{
				{Field: "field1", Message: "error1"},
				{Field: "field2", Message: "error2"},
			},
			want: "2 validation errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.errs.Error()
			if !strings.Contains(got, tt.want) {
				t.Errorf("ValidationErrors.Error() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *ValidationError
		want string
	}{
		{
			name: "with field",
			err:  &ValidationError{Field: "test.field", Message: "is required"},
			want: "test.field: is required",
		},
		{
			name: "without field",
			err:  &ValidationError{Message: "general error"},
			want: "general error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("ValidationError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidate_ConvenienceFunction(t *testing.T) {
	rb := &Runbook{
		Metadata: Metadata{Name: "test"},
		Spec: RunbookSpec{
			Steps: []Step{
				{Name: "step1", Type: StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	if err := Validate(rb); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}
