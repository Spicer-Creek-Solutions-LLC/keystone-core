package runbook

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Parser provides methods for parsing runbook definitions from YAML.
type Parser struct {
	// StrictMode enables strict validation during parsing.
	StrictMode bool
}

// NewParser creates a new Parser with default settings.
func NewParser() *Parser {
	return &Parser{
		StrictMode: false,
	}
}

// ParseFile reads and parses a runbook from a file.
func (p *Parser) ParseFile(path string) (*Runbook, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open runbook file: %w", err)
	}
	defer f.Close()

	return p.Parse(f)
}

// Parse reads and parses a runbook from an io.Reader.
func (p *Parser) Parse(r io.Reader) (*Runbook, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read runbook data: %w", err)
	}

	return p.ParseBytes(data)
}

// ParseBytes parses a runbook from raw YAML bytes.
func (p *Parser) ParseBytes(data []byte) (*Runbook, error) {
	// First, check API version and kind
	var header struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("parse runbook header: %w", err)
	}

	if header.Kind != Kind {
		return nil, fmt.Errorf("invalid kind %q, expected %q", header.Kind, Kind)
	}

	// Parse based on API version
	switch header.APIVersion {
	case APIVersion, "":
		return p.parseV1(data)
	default:
		return nil, fmt.Errorf("unsupported API version %q, expected %q", header.APIVersion, APIVersion)
	}
}

// parseV1 parses a v1 runbook definition.
func (p *Parser) parseV1(data []byte) (*Runbook, error) {
	var rb Runbook

	decoder := yaml.NewDecoder(nil)
	if p.StrictMode {
		decoder.KnownFields(true)
	}

	if err := yaml.Unmarshal(data, &rb); err != nil {
		return nil, fmt.Errorf("parse runbook: %w", err)
	}

	// Set defaults
	if rb.APIVersion == "" {
		rb.APIVersion = APIVersion
	}
	if rb.Kind == "" {
		rb.Kind = Kind
	}

	// Apply default values to steps
	for i := range rb.Spec.Steps {
		p.applyStepDefaults(&rb.Spec.Steps[i])
	}
	for i := range rb.Spec.OnSuccess {
		p.applyStepDefaults(&rb.Spec.OnSuccess[i])
	}
	for i := range rb.Spec.OnFailure {
		p.applyStepDefaults(&rb.Spec.OnFailure[i])
	}

	// Apply default values to inputs
	for i := range rb.Spec.Inputs {
		p.applyInputDefaults(&rb.Spec.Inputs[i])
	}

	return &rb, nil
}

// applyStepDefaults applies default values to a step.
func (p *Parser) applyStepDefaults(step *Step) {
	// Ensure Config map is initialized
	if step.Config == nil {
		step.Config = make(map[string]interface{})
	}

	// Apply defaults to retry config
	if step.Retries != nil {
		if step.Retries.Backoff == "" {
			step.Retries.Backoff = BackoffConstant
		}
		if step.Retries.MaxAttempts < 1 {
			step.Retries.MaxAttempts = 1
		}
	}

	// Apply defaults to outputs
	for i := range step.Outputs {
		if step.Outputs[i].Parser == "" {
			step.Outputs[i].Parser = OutputParserRaw
		}
	}
}

// applyInputDefaults applies default values to an input definition.
func (p *Parser) applyInputDefaults(input *InputDef) {
	if input.Type == "" {
		input.Type = InputTypeString
	}
}

// ParseFile is a convenience function that parses a runbook from a file path.
func ParseFile(path string) (*Runbook, error) {
	return NewParser().ParseFile(path)
}

// Parse is a convenience function that parses a runbook from an io.Reader.
func Parse(r io.Reader) (*Runbook, error) {
	return NewParser().Parse(r)
}

// ParseBytes is a convenience function that parses a runbook from raw YAML bytes.
func ParseBytes(data []byte) (*Runbook, error) {
	return NewParser().ParseBytes(data)
}

// ParseString is a convenience function that parses a runbook from a YAML string.
func ParseString(s string) (*Runbook, error) {
	return ParseBytes([]byte(s))
}

// ToYAML converts a runbook to YAML bytes.
func ToYAML(rb *Runbook) ([]byte, error) {
	return yaml.Marshal(rb)
}
