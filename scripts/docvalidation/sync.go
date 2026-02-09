package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SyncReport contains documentation sync analysis
type SyncReport struct {
	ReadmeVsDocsIssues   []SyncIssue       `json:"readme_vs_docs"`
	ClaudeMdVsDocsIssues []SyncIssue       `json:"claudemd_vs_docs"`
	DesignVsDocsIssues   []SyncIssue       `json:"design_vs_docs"`
	VersionMismatches    []VersionMismatch `json:"version_mismatches"`
	FeatureListDrift     []FeatureDrift    `json:"feature_list_drift"`
}

// SyncIssue represents a synchronization issue between files
type SyncIssue struct {
	SourceFile  string `json:"source_file"`
	TargetFile  string `json:"target_file"`
	Section     string `json:"section"`
	IssueType   string `json:"issue_type"` // missing, outdated, different
	Description string `json:"description"`
}

// VersionMismatch represents version number inconsistencies
type VersionMismatch struct {
	File     string `json:"file"`
	Version  string `json:"version"`
	Expected string `json:"expected"`
}

// FeatureDrift represents features mentioned inconsistently
type FeatureDrift struct {
	Feature     string   `json:"feature"`
	MentionedIn []string `json:"mentioned_in"`
	MissingFrom []string `json:"missing_from"`
}

// RunSyncCheck checks documentation synchronization
func RunSyncCheck(rootDir string, verbose bool) {
	fmt.Println("Checking documentation synchronization...")

	report := &SyncReport{}

	// Check README vs user docs
	checkReadmeVsDocs(rootDir, report, verbose)

	// Check CLAUDE.md vs user docs
	checkClaudeMdVsDocs(rootDir, report, verbose)

	// Check feature consistency
	checkFeatureConsistency(rootDir, report, verbose)

	// Check version numbers
	checkVersionNumbers(rootDir, report, verbose)

	// Print report
	printSyncReport(report)

	// Write report
	writeReport := formatSyncReport(report)
	outputPath := "./scripts/docvalidation/sync-report.md"
	//nolint:gosec // G306: sync reports need to be readable by developers
	if err := os.WriteFile(outputPath, []byte(writeReport), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}
	fmt.Printf("Sync report written to %s\n", outputPath)
}

func checkReadmeVsDocs(rootDir string, report *SyncReport, verbose bool) {
	readmePath := filepath.Join(rootDir, "README.md")
	overviewPath := filepath.Join(rootDir, "docs", "content", "en", "docs", "getting-started", "overview.md")
	installPath := filepath.Join(rootDir, "docs", "content", "en", "docs", "getting-started", "installation.md")
	quickstartPath := filepath.Join(rootDir, "docs", "content", "en", "docs", "getting-started", "quick-start.md")

	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		if verbose {
			fmt.Printf("  Could not read README.md: %v\n", err)
		}
		return
	}

	// Check that key sections exist in both places
	keySections := []struct {
		name         string
		readmeMarker string
		docsPath     string
		docsMarker   string
	}{
		{"Features", "## Features", overviewPath, "## Key Features"},
		{"Installation", "## Installation", installPath, "## Installation"},
		{"Quick Start", "## Quick Start", quickstartPath, "## Quick Start"},
	}

	for _, section := range keySections {
		if !strings.Contains(string(readmeContent), section.readmeMarker) {
			continue // Section doesn't exist in README
		}

		docsContent, err := os.ReadFile(section.docsPath)
		if err != nil {
			report.ReadmeVsDocsIssues = append(report.ReadmeVsDocsIssues, SyncIssue{
				SourceFile:  "README.md",
				TargetFile:  section.docsPath,
				Section:     section.name,
				IssueType:   "missing",
				Description: fmt.Sprintf("Docs file not found: %s", section.docsPath),
			})
			continue
		}

		// Compare section content hashes to detect drift
		readmeSection := extractSection(string(readmeContent), section.readmeMarker)
		docsSection := extractSection(string(docsContent), section.docsMarker)

		if readmeSection != "" && docsSection != "" {
			// Normalize and compare
			readmeNorm := normalizeContent(readmeSection)
			docsNorm := normalizeContent(docsSection)

			if significantlyDifferent(readmeNorm, docsNorm) {
				report.ReadmeVsDocsIssues = append(report.ReadmeVsDocsIssues, SyncIssue{
					SourceFile:  "README.md",
					TargetFile:  filepath.Base(section.docsPath),
					Section:     section.name,
					IssueType:   "different",
					Description: fmt.Sprintf("%s section content differs significantly", section.name),
				})
				if verbose {
					fmt.Printf("  %s section differs between README.md and %s\n", section.name, filepath.Base(section.docsPath))
				}
			}
		}
	}
}

func checkClaudeMdVsDocs(rootDir string, report *SyncReport, verbose bool) {
	claudePath := filepath.Join(rootDir, "CLAUDE.md")
	conceptsPath := filepath.Join(rootDir, "docs", "content", "en", "docs", "concepts")

	claudeContent, err := os.ReadFile(claudePath)
	if err != nil {
		return
	}

	// Extract epic status from CLAUDE.md
	epicRe := regexp.MustCompile(`Epic (\d+).*?([✅⏳❌])`)
	claudeEpics := make(map[string]string)
	matches := epicRe.FindAllStringSubmatch(string(claudeContent), -1)
	for _, match := range matches {
		if len(match) > 2 {
			claudeEpics[match[1]] = match[2]
		}
	}

	// Check if concepts docs mention completed epics
	entries, _ := os.ReadDir(conceptsPath)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(conceptsPath, entry.Name()))
		if err != nil {
			continue
		}

		// Check for outdated status mentions
		if strings.Contains(string(content), "coming soon") || strings.Contains(string(content), "not yet implemented") {
			// Cross-reference with CLAUDE.md epic status
			for epicNum, status := range claudeEpics {
				if status == "✅" && strings.Contains(string(content), fmt.Sprintf("Epic %s", epicNum)) {
					report.ClaudeMdVsDocsIssues = append(report.ClaudeMdVsDocsIssues, SyncIssue{
						SourceFile:  "CLAUDE.md",
						TargetFile:  entry.Name(),
						Section:     fmt.Sprintf("Epic %s", epicNum),
						IssueType:   "outdated",
						Description: fmt.Sprintf("Epic %s is complete but %s still says 'coming soon' or 'not implemented'", epicNum, entry.Name()),
					})
					if verbose {
						fmt.Printf("  %s may have outdated status for Epic %s\n", entry.Name(), epicNum)
					}
				}
			}
		}
	}
}

func checkFeatureConsistency(rootDir string, report *SyncReport, verbose bool) {
	// Key features that should be consistently mentioned
	keyFeatures := []string{
		"NATS",
		"GitOps",
		"Policy",
		"Remote Execution",
		"State Management",
		"Observability",
		"Kubernetes",
		"SPIFFE",
	}

	files := []string{
		filepath.Join(rootDir, "README.md"),
		filepath.Join(rootDir, "docs", "project", "DESIGN.md"),
		filepath.Join(rootDir, "docs", "content", "en", "docs", "_index.md"),
		filepath.Join(rootDir, "docs", "content", "en", "docs", "getting-started", "overview.md"),
	}

	for _, feature := range keyFeatures {
		var mentionedIn []string
		var missingFrom []string

		for _, file := range files {
			content, err := os.ReadFile(file)
			if err != nil {
				continue
			}

			if strings.Contains(strings.ToLower(string(content)), strings.ToLower(feature)) {
				mentionedIn = append(mentionedIn, filepath.Base(file))
			} else {
				missingFrom = append(missingFrom, filepath.Base(file))
			}
		}

		// Report if feature mentioned in some but not all key files
		if len(mentionedIn) > 0 && len(missingFrom) > 0 {
			report.FeatureListDrift = append(report.FeatureListDrift, FeatureDrift{
				Feature:     feature,
				MentionedIn: mentionedIn,
				MissingFrom: missingFrom,
			})
			if verbose {
				fmt.Printf("  Feature '%s' inconsistently mentioned\n", feature)
			}
		}
	}
}

func checkVersionNumbers(rootDir string, report *SyncReport, verbose bool) {
	// Check for version number consistency
	versionRe := regexp.MustCompile(`v?\d+\.\d+\.\d+`)

	files := []string{
		filepath.Join(rootDir, "README.md"),
		filepath.Join(rootDir, "version.go"),
		filepath.Join(rootDir, "pkg", "version", "version.go"),
	}

	versions := make(map[string][]string)

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		matches := versionRe.FindAllString(string(content), -1)
		for _, v := range matches {
			// Skip common versions like 1.0.0, 0.0.0
			if v == "1.0.0" || v == "0.0.0" || v == "v1.0.0" {
				continue
			}
			versions[v] = append(versions[v], filepath.Base(file))
		}
	}

	// Report version inconsistencies
	if len(versions) > 1 && verbose {
		fmt.Println("  Multiple version numbers found across files:")
		for ver, files := range versions {
			fmt.Printf("    %s in: %s\n", ver, strings.Join(files, ", "))
		}
	}
}

func extractSection(content, marker string) string {
	idx := strings.Index(content, marker)
	if idx == -1 {
		return ""
	}

	// Find next section (## header)
	rest := content[idx+len(marker):]
	nextSection := strings.Index(rest, "\n## ")
	if nextSection == -1 {
		nextSection = strings.Index(rest, "\n# ")
	}
	if nextSection == -1 {
		return rest
	}
	return rest[:nextSection]
}

func normalizeContent(content string) string {
	// Remove markdown formatting, normalize whitespace
	content = strings.ToLower(content)
	content = regexp.MustCompile(`[#*_\[\](){}]`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`\s+`).ReplaceAllString(content, " ")
	return strings.TrimSpace(content)
}

func significantlyDifferent(a, b string) bool {
	// Compare SHA256 hashes of normalized content
	hashA := fmt.Sprintf("%x", sha256.Sum256([]byte(a)))
	hashB := fmt.Sprintf("%x", sha256.Sum256([]byte(b)))

	// If lengths differ by more than 50%, consider different
	if a != "" && b != "" {
		ratio := float64(len(a)) / float64(len(b))
		if ratio < 0.5 || ratio > 2.0 {
			return true
		}
	}

	return hashA != hashB
}

func printSyncReport(report *SyncReport) {
	fmt.Println()
	fmt.Println("=== Documentation Sync Report ===")
	fmt.Println()

	total := len(report.ReadmeVsDocsIssues) + len(report.ClaudeMdVsDocsIssues) +
		len(report.FeatureListDrift) + len(report.VersionMismatches)

	if total == 0 {
		fmt.Println("No synchronization issues detected!")
		return
	}

	if len(report.ReadmeVsDocsIssues) > 0 {
		fmt.Printf("README.md vs Docs issues: %d\n", len(report.ReadmeVsDocsIssues))
		for _, issue := range report.ReadmeVsDocsIssues {
			fmt.Printf("  - [%s] %s -> %s: %s\n", issue.IssueType, issue.SourceFile, issue.TargetFile, issue.Description)
		}
		fmt.Println()
	}

	if len(report.ClaudeMdVsDocsIssues) > 0 {
		fmt.Printf("CLAUDE.md vs Docs issues: %d\n", len(report.ClaudeMdVsDocsIssues))
		for _, issue := range report.ClaudeMdVsDocsIssues {
			fmt.Printf("  - [%s] %s: %s\n", issue.IssueType, issue.TargetFile, issue.Description)
		}
		fmt.Println()
	}

	if len(report.FeatureListDrift) > 0 {
		fmt.Printf("Feature mention inconsistencies: %d\n", len(report.FeatureListDrift))
		for _, drift := range report.FeatureListDrift {
			fmt.Printf("  - '%s': mentioned in %s, missing from %s\n",
				drift.Feature,
				strings.Join(drift.MentionedIn, ", "),
				strings.Join(drift.MissingFrom, ", "))
		}
		fmt.Println()
	}

	fmt.Printf("Total issues: %d\n", total)
}

func formatSyncReport(report *SyncReport) string {
	var sb strings.Builder

	sb.WriteString("# Documentation Sync Report\n\n")

	total := len(report.ReadmeVsDocsIssues) + len(report.ClaudeMdVsDocsIssues) +
		len(report.FeatureListDrift) + len(report.VersionMismatches)

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Category | Issues |\n")
	sb.WriteString("|----------|--------|\n")
	sb.WriteString(fmt.Sprintf("| README vs Docs | %d |\n", len(report.ReadmeVsDocsIssues)))
	sb.WriteString(fmt.Sprintf("| CLAUDE.md vs Docs | %d |\n", len(report.ClaudeMdVsDocsIssues)))
	sb.WriteString(fmt.Sprintf("| Feature Consistency | %d |\n", len(report.FeatureListDrift)))
	sb.WriteString(fmt.Sprintf("| Version Mismatches | %d |\n", len(report.VersionMismatches)))
	sb.WriteString(fmt.Sprintf("| **Total** | **%d** |\n", total))
	sb.WriteString("\n")

	if len(report.ReadmeVsDocsIssues) > 0 {
		sb.WriteString("## README.md vs Documentation\n\n")
		sb.WriteString("| Source | Target | Section | Issue | Description |\n")
		sb.WriteString("|--------|--------|---------|-------|-------------|\n")
		for _, issue := range report.ReadmeVsDocsIssues {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
				issue.SourceFile, issue.TargetFile, issue.Section, issue.IssueType, issue.Description))
		}
		sb.WriteString("\n")
	}

	if len(report.ClaudeMdVsDocsIssues) > 0 {
		sb.WriteString("## CLAUDE.md vs Documentation\n\n")
		sb.WriteString("| Target | Section | Issue | Description |\n")
		sb.WriteString("|--------|---------|-------|-------------|\n")
		for _, issue := range report.ClaudeMdVsDocsIssues {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				issue.TargetFile, issue.Section, issue.IssueType, issue.Description))
		}
		sb.WriteString("\n")
	}

	if len(report.FeatureListDrift) > 0 {
		sb.WriteString("## Feature Mention Inconsistencies\n\n")
		sb.WriteString("| Feature | Mentioned In | Missing From |\n")
		sb.WriteString("|---------|--------------|-------------|\n")
		for _, drift := range report.FeatureListDrift {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
				drift.Feature,
				strings.Join(drift.MentionedIn, ", "),
				strings.Join(drift.MissingFrom, ", ")))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
