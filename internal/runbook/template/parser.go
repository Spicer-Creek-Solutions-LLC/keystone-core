package template

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Parser parses template definitions from YAML.
type Parser struct {
	strict bool
}

// NewParser creates a new template parser.
func NewParser() *Parser {
	return &Parser{strict: true}
}

// SetStrict enables or disables strict parsing mode.
func (p *Parser) SetStrict(strict bool) {
	p.strict = strict
}

// Parse parses a template from YAML bytes.
func (p *Parser) Parse(data []byte) (*Template, error) {
	var tmpl Template
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("parse template YAML: %w", err)
	}

	if p.strict {
		if err := Validate(&tmpl); err != nil {
			return nil, fmt.Errorf("validate template: %w", err)
		}
	}

	return &tmpl, nil
}

// ParseFile parses a template from a file.
func (p *Parser) ParseFile(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template file: %w", err)
	}
	return p.Parse(data)
}

// ParseReader parses a template from a reader.
func (p *Parser) ParseReader(r io.Reader) (*Template, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	return p.Parse(data)
}

// ParseMulti parses multiple templates from a multi-document YAML.
func (p *Parser) ParseMulti(data []byte) ([]*Template, error) {
	decoder := yaml.NewDecoder(nil)
	_ = decoder // Reset decoder with actual data

	// Split by YAML document separator
	var templates []*Template
	decoder = yaml.NewDecoder(nil)

	// Use a different approach - parse with Document decoder
	dec := yaml.NewDecoder(nil)
	_ = dec

	// Simple approach: split on ---
	docs := splitYAMLDocuments(data)
	for i, doc := range docs {
		if len(doc) == 0 {
			continue
		}

		tmpl, err := p.Parse(doc)
		if err != nil {
			return nil, fmt.Errorf("parse document %d: %w", i, err)
		}
		templates = append(templates, tmpl)
	}

	return templates, nil
}

// splitYAMLDocuments splits a multi-document YAML into separate documents.
func splitYAMLDocuments(data []byte) [][]byte {
	var documents [][]byte
	var current []byte
	lines := splitLines(data)

	for _, line := range lines {
		if string(line) == "---" {
			if len(current) > 0 {
				documents = append(documents, current)
				current = nil
			}
		} else {
			current = append(current, line...)
			current = append(current, '\n')
		}
	}

	if len(current) > 0 {
		documents = append(documents, current)
	}

	return documents
}

// splitLines splits data into lines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	var current []byte

	for _, b := range data {
		if b == '\n' {
			lines = append(lines, current)
			current = nil
		} else {
			current = append(current, b)
		}
	}

	if len(current) > 0 {
		lines = append(lines, current)
	}

	return lines
}

// LoadFromDirectory loads all templates from a directory.
func LoadFromDirectory(path string, registry *Registry) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	parser := NewParser()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !(hasExtension(name, ".yaml") || hasExtension(name, ".yml")) {
			continue
		}

		filePath := path + "/" + name
		tmpl, err := parser.ParseFile(filePath)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filePath, err)
		}

		if err := registry.Register(tmpl); err != nil {
			return fmt.Errorf("register %s: %w", tmpl.Metadata.Name, err)
		}
	}

	return nil
}

// hasExtension checks if a filename has the given extension.
func hasExtension(name, ext string) bool {
	if len(name) < len(ext) {
		return false
	}
	return name[len(name)-len(ext):] == ext
}
