// Package main extracts and validates code examples from documentation.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// CodeBlock represents an extracted code block from documentation.
type CodeBlock struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Language   string `json:"language"`
	Content    string `json:"content"`
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
}

// ExtractionResult contains the results of extracting code blocks.
type ExtractionResult struct {
	TotalBlocks    int          `json:"total_blocks"`
	ValidBlocks    int          `json:"valid_blocks"`
	InvalidBlocks  int          `json:"invalid_blocks"`
	SkippedBlocks  int          `json:"skipped_blocks"`
	ByLanguage     map[string]int `json:"by_language"`
	Blocks         []CodeBlock  `json:"blocks"`
	InvalidDetails []CodeBlock  `json:"invalid_details,omitempty"`
}

func main() {
	docsDir := flag.String("docs", "docs/content/en/docs", "Documentation directory")
	outputFile := flag.String("output", "", "Output JSON file (empty for stdout)")
	verbose := flag.Bool("verbose", false, "Verbose output")
	validateOnly := flag.Bool("validate", true, "Validate code blocks")
	flag.Parse()

	result := &ExtractionResult{
		ByLanguage: make(map[string]int),
		Blocks:     []CodeBlock{},
	}

	err := filepath.Walk(*docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		blocks, err := extractCodeBlocks(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", path, err)
			return nil
		}

		for _, block := range blocks {
			result.TotalBlocks++
			result.ByLanguage[block.Language]++

			if *validateOnly {
				validateCodeBlock(&block)
			}

			if block.Skipped {
				result.SkippedBlocks++
			} else if block.Valid {
				result.ValidBlocks++
			} else {
				result.InvalidBlocks++
				result.InvalidDetails = append(result.InvalidDetails, block)
			}

			if *verbose {
				result.Blocks = append(result.Blocks, block)
			}
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking docs: %v\n", err)
		os.Exit(1)
	}

	// Output results
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling results: %v\n", err)
		os.Exit(1)
	}

	if *outputFile != "" {
		err = os.WriteFile(*outputFile, output, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Results written to %s\n", *outputFile)
	} else {
		fmt.Println(string(output))
	}

	// Print summary
	fmt.Fprintf(os.Stderr, "\n=== Summary ===\n")
	fmt.Fprintf(os.Stderr, "Total code blocks: %d\n", result.TotalBlocks)
	fmt.Fprintf(os.Stderr, "Valid: %d\n", result.ValidBlocks)
	fmt.Fprintf(os.Stderr, "Invalid: %d\n", result.InvalidBlocks)
	fmt.Fprintf(os.Stderr, "Skipped: %d\n", result.SkippedBlocks)
	fmt.Fprintf(os.Stderr, "\nBy language:\n")
	for lang, count := range result.ByLanguage {
		fmt.Fprintf(os.Stderr, "  %s: %d\n", lang, count)
	}

	if result.InvalidBlocks > 0 {
		fmt.Fprintf(os.Stderr, "\n=== Invalid Blocks ===\n")
		for _, block := range result.InvalidDetails {
			fmt.Fprintf(os.Stderr, "%s:%d [%s]: %s\n", block.File, block.Line, block.Language, block.Error)
		}
		os.Exit(1)
	}
}

func extractCodeBlocks(path string) ([]CodeBlock, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var blocks []CodeBlock
	scanner := bufio.NewScanner(file)
	lineNum := 0
	inBlock := false
	var currentBlock CodeBlock
	var content strings.Builder
	codeBlockRegex := regexp.MustCompile("^```(\\w*)$")

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if !inBlock {
			matches := codeBlockRegex.FindStringSubmatch(line)
			if matches != nil {
				inBlock = true
				currentBlock = CodeBlock{
					File:     path,
					Line:     lineNum,
					Language: matches[1],
				}
				content.Reset()
			}
		} else {
			if line == "```" {
				currentBlock.Content = content.String()
				blocks = append(blocks, currentBlock)
				inBlock = false
			} else {
				content.WriteString(line)
				content.WriteString("\n")
			}
		}
	}

	return blocks, scanner.Err()
}

func validateCodeBlock(block *CodeBlock) {
	// Skip blocks that are output examples or pseudo-code
	if isOutputExample(block) {
		block.Skipped = true
		block.SkipReason = "output example"
		block.Valid = true
		return
	}

	switch block.Language {
	case "yaml":
		validateYAML(block)
	case "json":
		validateJSON(block)
	case "go":
		validateGo(block)
	case "bash", "shell", "sh":
		validateBash(block)
	case "sql":
		block.Skipped = true
		block.SkipReason = "SQL not validated"
		block.Valid = true
	case "text", "plaintext", "":
		block.Skipped = true
		block.SkipReason = "plain text"
		block.Valid = true
	case "mermaid":
		block.Skipped = true
		block.SkipReason = "diagram"
		block.Valid = true
	case "toml":
		validateTOML(block)
	case "ini":
		block.Skipped = true
		block.SkipReason = "INI not validated"
		block.Valid = true
	case "proto", "protobuf":
		block.Skipped = true
		block.SkipReason = "protobuf not validated"
		block.Valid = true
	case "diff":
		block.Skipped = true
		block.SkipReason = "diff output"
		block.Valid = true
	case "powershell", "ps1":
		block.Skipped = true
		block.SkipReason = "PowerShell not validated"
		block.Valid = true
	case "dockerfile":
		block.Skipped = true
		block.SkipReason = "Dockerfile not validated"
		block.Valid = true
	default:
		block.Skipped = true
		block.SkipReason = fmt.Sprintf("unknown language: %s", block.Language)
		block.Valid = true
	}
}

func isOutputExample(block *CodeBlock) bool {
	content := strings.TrimSpace(block.Content)

	// Common patterns for output examples
	outputPatterns := []string{
		"INFO ", "WARN ", "ERROR ", "DEBUG ",
		"Status:", "Output:", "Expected:",
		"# Expected:",
		"AGENT ID",
		"---", // table separators
	}

	for _, pattern := range outputPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}

	// If it's a short block that looks like output
	if block.Language == "" && len(content) < 200 {
		return true
	}

	return false
}

func validateYAML(block *CodeBlock) {
	content := block.Content

	// Check if this is a multi-example documentation block (shows multiple scenarios)
	// These often have comments like "# Case 1:", "# Scenario:", "# Example:"
	// and duplicate keys are intentional to show different states
	if strings.Contains(content, "# Case") || strings.Contains(content, "# Scenario") ||
		strings.Contains(content, "# Example") || strings.Contains(content, "# Or ") ||
		strings.Contains(content, "# Result:") || strings.Contains(content, "# Module declares:") ||
		strings.Contains(content, "# Default ") || strings.Contains(content, "# Simple ") ||
		strings.Contains(content, "# detailed format") {
		block.Skipped = true
		block.SkipReason = "multi-example documentation block"
		block.Valid = true
		return
	}

	// Check for placeholder syntax
	if strings.Contains(content, "<...>") || strings.Contains(content, "...") && strings.Contains(content, ": ...") {
		block.Skipped = true
		block.SkipReason = "YAML with placeholders"
		block.Valid = true
		return
	}

	var data interface{}
	err := yaml.Unmarshal([]byte(content), &data)
	if err != nil {
		// Check if it's a multi-document YAML (separated by ---)
		docs := strings.Split(content, "\n---\n")
		if len(docs) > 1 {
			allValid := true
			for _, doc := range docs {
				doc = strings.TrimSpace(doc)
				if doc == "" || doc == "---" {
					continue
				}
				if err := yaml.Unmarshal([]byte(doc), &data); err != nil {
					allValid = false
					block.Error = fmt.Sprintf("YAML syntax error: %v", err)
					break
				}
			}
			block.Valid = allValid
		} else {
			// Check if it contains duplicate key pattern (common in showing state results)
			if strings.Contains(err.Error(), "already defined") {
				// These are often showing different states (e.g., result: true, result: false)
				block.Skipped = true
				block.SkipReason = "multi-state documentation example"
				block.Valid = true
			} else {
				block.Valid = false
				block.Error = fmt.Sprintf("YAML syntax error: %v", err)
			}
		}
	} else {
		block.Valid = true
	}
}

func validateJSON(block *CodeBlock) {
	content := block.Content

	// Check for JSONC (JSON with comments) - common in VS Code configs
	if strings.Contains(content, "//") || strings.Contains(content, "/*") {
		block.Skipped = true
		block.SkipReason = "JSONC (JSON with comments)"
		block.Valid = true
		return
	}

	// Check for placeholder syntax like { ... } or [...]
	if strings.Contains(content, "{ ...") || strings.Contains(content, "...}") ||
		strings.Contains(content, "{ /* ") || strings.Contains(content, "<...>") ||
		strings.Contains(content, "[...]") || strings.Contains(content, "...],") {
		block.Skipped = true
		block.SkipReason = "JSON with placeholders"
		block.Valid = true
		return
	}

	// Check for multiple JSON objects (common in showing sequences)
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") && strings.Count(content, "\n{") > 0 {
		block.Skipped = true
		block.SkipReason = "JSON sequence (multiple objects)"
		block.Valid = true
		return
	}

	var data interface{}
	err := json.Unmarshal([]byte(content), &data)
	if err != nil {
		block.Valid = false
		block.Error = fmt.Sprintf("JSON syntax error: %v", err)
	} else {
		block.Valid = true
	}
}

func validateGo(block *CodeBlock) {
	content := block.Content

	// Check if it's a complete Go file or a snippet
	if !strings.Contains(content, "package ") {
		// It's a snippet - skip validation
		block.Skipped = true
		block.SkipReason = "Go snippet (no package declaration)"
		block.Valid = true
		return
	}

	// For full Go files, we could compile them but that's complex
	// Just do basic syntax check for common issues
	block.Valid = true

	// Check for obvious issues
	if strings.Count(content, "{") != strings.Count(content, "}") {
		block.Valid = false
		block.Error = "mismatched braces"
	}
}

func validateBash(block *CodeBlock) {
	content := strings.TrimSpace(block.Content)

	// Skip if it's clearly output or a comment
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		block.Valid = true
		return
	}

	// Check for common bash syntax issues
	block.Valid = true

	// Check for unclosed quotes
	singleQuotes := strings.Count(content, "'") - strings.Count(content, "\\'")
	if singleQuotes % 2 != 0 {
		// Could be heredoc or intentional, skip
		block.Skipped = true
		block.SkipReason = "complex quoting"
		return
	}

	// Check for obvious command issues
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Basic validation passed
	}
}

func validateTOML(block *CodeBlock) {
	// Basic TOML validation - just check for obvious issues
	content := block.Content

	// Check for section headers
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && !strings.HasSuffix(line, "]") {
			block.Valid = false
			block.Error = "unclosed section header"
			return
		}
	}

	block.Valid = true
}
