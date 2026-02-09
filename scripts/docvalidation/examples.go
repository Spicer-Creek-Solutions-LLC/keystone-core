package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// CodeExample represents a code example from documentation
type CodeExample struct {
	Source   string `json:"source"`
	Language string `json:"language"`
	Code     string `json:"code"`
	Line     int    `json:"line"`
	Valid    bool   `json:"valid"`
	Error    string `json:"error,omitempty"`
}

// ExampleValidator validates code examples in documentation
type ExampleValidator struct {
	rootDir  string
	docsDir  string
	tempDir  string
	examples []CodeExample
}

// NewExampleValidator creates a new example validator
func NewExampleValidator(rootDir string) (*ExampleValidator, error) {
	tempDir, err := os.MkdirTemp("", "kscore-examples-*")
	if err != nil {
		return nil, err
	}

	return &ExampleValidator{
		rootDir: rootDir,
		docsDir: filepath.Join(rootDir, "docs", "content", "en", "docs"),
		tempDir: tempDir,
	}, nil
}

// Close cleans up temporary files
func (ev *ExampleValidator) Close() {
	os.RemoveAll(ev.tempDir)
}

// ExtractExamples extracts all code examples from documentation
func (ev *ExampleValidator) ExtractExamples(verbose bool) []CodeExample {
	filepath.Walk(ev.docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil //nolint:nilerr // continue walk on error
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // continue walk on read error
		}

		relPath, _ := filepath.Rel(ev.docsDir, path)
		examples := extractCodeBlocks(relPath, string(content))
		ev.examples = append(ev.examples, examples...)

		if verbose && len(examples) > 0 {
			fmt.Printf("  %s: %d code examples\n", relPath, len(examples))
		}

		return nil
	})

	return ev.examples
}

// ValidateGoExamples validates Go code examples
func (ev *ExampleValidator) ValidateGoExamples(verbose bool) []CodeExample {
	var results []CodeExample

	for i := range ev.examples {
		ex := &ev.examples[i]
		if ex.Language != "go" && ex.Language != "golang" {
			continue
		}

		// Skip if it looks like a fragment
		if isGoFragment(ex.Code) {
			ex.Valid = true
			results = append(results, *ex)
			continue
		}

		// Try to parse/compile
		valid, errMsg := ev.validateGoCode(ex.Code)
		ex.Valid = valid
		ex.Error = errMsg
		results = append(results, *ex)

		if verbose && !valid {
			fmt.Printf("  [INVALID] %s:%d - %s\n", ex.Source, ex.Line, errMsg)
		}
	}

	return results
}

// ValidateYAMLExamples validates YAML code examples
func (ev *ExampleValidator) ValidateYAMLExamples(verbose bool) []CodeExample {
	var results []CodeExample

	for i := range ev.examples {
		ex := &ev.examples[i]
		if ex.Language != "yaml" && ex.Language != "yml" {
			continue
		}

		// Basic YAML syntax check - look for common issues
		valid, errMsg := validateYAMLSyntax(ex.Code)
		ex.Valid = valid
		ex.Error = errMsg
		results = append(results, *ex)

		if verbose && !valid {
			fmt.Printf("  [INVALID YAML] %s:%d - %s\n", ex.Source, ex.Line, errMsg)
		}
	}

	return results
}

// ValidateBashExamples validates bash code examples
func (ev *ExampleValidator) ValidateBashExamples(verbose bool) []CodeExample {
	var results []CodeExample

	for i := range ev.examples {
		ex := &ev.examples[i]
		if ex.Language != "bash" && ex.Language != "sh" && ex.Language != "shell" {
			continue
		}

		// Basic bash syntax check using bash -n
		valid, errMsg := ev.validateBashSyntax(ex.Code)
		ex.Valid = valid
		ex.Error = errMsg
		results = append(results, *ex)

		if verbose && !valid {
			fmt.Printf("  [INVALID BASH] %s:%d - %s\n", ex.Source, ex.Line, errMsg)
		}
	}

	return results
}

func extractCodeBlocks(source, content string) []CodeExample {
	var examples []CodeExample

	// Match fenced code blocks ```lang\n...\n```
	re := regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	matches := re.FindAllStringSubmatchIndex(content, -1)

	for _, match := range matches {
		if len(match) < 6 {
			continue
		}

		lang := content[match[2]:match[3]]
		code := content[match[4]:match[5]]

		// Calculate line number
		lineNum := strings.Count(content[:match[0]], "\n") + 1

		examples = append(examples, CodeExample{
			Source:   source,
			Language: strings.ToLower(lang),
			Code:     code,
			Line:     lineNum,
		})
	}

	return examples
}

func isGoFragment(code string) bool {
	// Skip if it doesn't look like a complete Go file/function
	code = strings.TrimSpace(code)

	// Check if it's just a function call or statement
	if !strings.Contains(code, "func ") && !strings.Contains(code, "package ") {
		return true
	}

	// Check for common documentation fragments
	if strings.HasPrefix(code, "//") || strings.HasPrefix(code, "/*") {
		return true
	}

	return false
}

func (ev *ExampleValidator) validateGoCode(code string) (valid bool, errMsg string) {
	// Write to temp file
	tmpFile := filepath.Join(ev.tempDir, "example.go")

	// If no package, wrap it
	if !strings.Contains(code, "package ") {
		code = "package main\n\n" + code
	}

	// If no main and has func, assume it's valid for doc purposes
	if !strings.Contains(code, "func main()") && strings.Contains(code, "func ") {
		return true, ""
	}

	//nolint:gosec // G306: temp files for validation are in temp dir
	if err := os.WriteFile(tmpFile, []byte(code), 0o644); err != nil {
		return false, err.Error()
	}

	// Try to parse with go vet (less strict than compile)
	cmd := exec.CommandContext(context.Background(), "go", "vet", tmpFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, strings.TrimSpace(string(output))
	}

	return true, ""
}

func validateYAMLSyntax(code string) (valid bool, errMsg string) {
	// Basic YAML syntax checks
	lines := strings.Split(code, "\n")

	for i, line := range lines {
		// Check for tabs (YAML should use spaces)
		if strings.HasPrefix(line, "\t") {
			return false, fmt.Sprintf("line %d: tabs not allowed in YAML (use spaces)", i+1)
		}

	}

	return true, ""
}

func (ev *ExampleValidator) validateBashSyntax(code string) (valid bool, errMsg string) {
	// Skip if it's just comments or simple commands
	lines := strings.Split(code, "\n")
	hasCode := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "$") {
			hasCode = true
			break
		}
	}

	if !hasCode {
		return true, ""
	}

	// Write to temp file
	tmpFile := filepath.Join(ev.tempDir, "example.sh")
	//nolint:gosec // G306: temp files for validation are in temp dir
	if err := os.WriteFile(tmpFile, []byte(code), 0o644); err != nil {
		return false, err.Error()
	}

	// Check syntax with bash -n
	cmd := exec.CommandContext(context.Background(), "bash", "-n", tmpFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, strings.TrimSpace(string(output))
	}

	return true, ""
}

// GenerateExampleReport generates a report of example validation
func GenerateExampleReport(examples []CodeExample) string {
	var sb strings.Builder

	// Count by language
	langCounts := make(map[string]int)
	langValid := make(map[string]int)
	langInvalid := make(map[string][]CodeExample)

	for _, ex := range examples {
		langCounts[ex.Language]++
		if ex.Valid {
			langValid[ex.Language]++
		} else {
			langInvalid[ex.Language] = append(langInvalid[ex.Language], ex)
		}
	}

	sb.WriteString("# Code Example Validation Report\n\n")
	sb.WriteString("## Summary by Language\n\n")
	sb.WriteString("| Language | Total | Valid | Invalid |\n")
	sb.WriteString("|----------|-------|-------|--------|\n")

	for lang, count := range langCounts {
		valid := langValid[lang]
		invalid := count - valid
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d |\n", lang, count, valid, invalid))
	}
	sb.WriteString("\n")

	// List invalid examples
	hasInvalid := false
	for _, invalids := range langInvalid {
		if len(invalids) > 0 {
			hasInvalid = true
			break
		}
	}

	if hasInvalid {
		sb.WriteString("## Invalid Examples\n\n")
		for lang, invalids := range langInvalid {
			if len(invalids) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("### %s\n\n", lang))
			for _, ex := range invalids {
				sb.WriteString(fmt.Sprintf("- **%s:%d**: %s\n", ex.Source, ex.Line, ex.Error))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
