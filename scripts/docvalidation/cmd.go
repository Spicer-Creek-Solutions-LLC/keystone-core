package main

import (
	"fmt"
	"os"
	"strings"
)

// RunLinkCheck runs the link checker and returns the number of broken links
func RunLinkCheck(rootDir string, checkExternal, verbose bool) int {
	fmt.Println("Checking documentation links...")
	checker := NewLinkChecker(rootDir, checkExternal)
	results := checker.CheckAll(verbose)

	report := GenerateLinkReport(results)
	outputPath := "./scripts/docvalidation/link-check-report.md"
	//nolint:gosec // G306: validation reports need to be readable by developers
	if err := os.WriteFile(outputPath, []byte(report), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return -1
	}
	fmt.Printf("Link check report written to %s\n", outputPath)

	// Summary
	var broken, brokenInternal int
	for _, r := range results {
		if r.Status == "broken" {
			broken++
			if r.Type == "internal" {
				brokenInternal++
			}
		}
	}
	if broken > 0 {
		fmt.Printf("Found %d broken links (%d internal, %d external)\n", broken, brokenInternal, broken-brokenInternal)
	} else {
		fmt.Println("All links OK")
	}

	return brokenInternal
}

// RunExampleValidation runs the example validator
func RunExampleValidation(rootDir string, verbose bool) {
	fmt.Println("Validating code examples...")
	validator, err := NewExampleValidator(rootDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		return
	}
	defer validator.Close()

	// Extract all examples
	examples := validator.ExtractExamples(verbose)
	fmt.Printf("Found %d code examples\n", len(examples))

	// Validate different languages
	fmt.Println("Validating Go examples...")
	validator.ValidateGoExamples(verbose)

	fmt.Println("Validating YAML examples...")
	validator.ValidateYAMLExamples(verbose)

	fmt.Println("Validating Bash examples...")
	validator.ValidateBashExamples(verbose)

	// Generate report
	report := GenerateExampleReport(validator.examples)
	outputPath := "./scripts/docvalidation/example-validation-report.md"
	//nolint:gosec // G306: validation reports need to be readable by developers
	if err := os.WriteFile(outputPath, []byte(report), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}
	fmt.Printf("Example validation report written to %s\n", outputPath)
}

// GodocCoverageReport generates a detailed godoc coverage report
func GodocCoverageReport(rootDir string, verbose bool) {
	fmt.Println("Generating godoc coverage report...")

	pkgRoot := rootDir + "/pkg"
	inv := &DocInventory{
		PkgRoot: pkgRoot,
	}

	if err := inventoryPackages(pkgRoot, inv, verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	var sb fmt.Stringer = &godocReport{packages: inv.Packages}
	outputPath := "./scripts/docvalidation/godoc-coverage-report.md"
	//nolint:gosec // G306: coverage reports need to be readable by developers
	if err := os.WriteFile(outputPath, []byte(sb.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}
	fmt.Printf("Godoc coverage report written to %s\n", outputPath)
}

type godocReport struct {
	packages []Package
}

func (r *godocReport) String() string {
	var sb stringBuilder

	sb.WriteString("# Godoc Coverage Report\n\n")

	// Summary statistics
	var totalSymbols, documentedSymbols int
	var fullCoverage, noCoverage int
	var totalCoverage float64

	for _, pkg := range r.packages {
		totalSymbols += pkg.ExportedTypes + pkg.ExportedFuncs
		documentedSymbols += pkg.DocumentedTypes + pkg.DocumentedFuncs
		totalCoverage += pkg.Coverage
		if pkg.Coverage >= 100 {
			fullCoverage++
		}
		if pkg.Coverage == 0 {
			noCoverage++
		}
	}

	avgCoverage := totalCoverage / float64(len(r.packages))

	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total packages**: %d\n", len(r.packages)))
	sb.WriteString(fmt.Sprintf("- **Average coverage**: %.1f%%\n", avgCoverage))
	sb.WriteString(fmt.Sprintf("- **Fully documented**: %d packages\n", fullCoverage))
	sb.WriteString(fmt.Sprintf("- **No documentation**: %d packages\n", noCoverage))
	sb.WriteString(fmt.Sprintf("- **Total exported symbols**: %d\n", totalSymbols))
	sb.WriteString(fmt.Sprintf("- **Documented symbols**: %d (%.1f%%)\n\n",
		documentedSymbols, float64(documentedSymbols)/float64(totalSymbols)*100))

	// Packages needing attention (coverage < 80%)
	sb.WriteString("## Packages Needing Attention\n\n")
	sb.WriteString("Packages with less than 80% godoc coverage:\n\n")
	sb.WriteString("| Package | Types | Documented | Funcs | Documented | Coverage |\n")
	sb.WriteString("|---------|-------|------------|-------|------------|----------|\n")

	for _, pkg := range r.packages {
		if pkg.Coverage < 80 {
			sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %.1f%% |\n",
				pkg.Name, pkg.ExportedTypes, pkg.DocumentedTypes,
				pkg.ExportedFuncs, pkg.DocumentedFuncs, pkg.Coverage))
		}
	}

	sb.WriteString("\n## All Packages\n\n")
	sb.WriteString("| Package | Has Pkg Doc | Types | Doc Types | Funcs | Doc Funcs | Coverage |\n")
	sb.WriteString("|---------|-------------|-------|-----------|-------|-----------|----------|\n")

	for _, pkg := range r.packages {
		hasPkgDoc := "No"
		if pkg.HasPackageDoc {
			hasPkgDoc = "Yes"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %d | %.1f%% |\n",
			pkg.Name, hasPkgDoc, pkg.ExportedTypes, pkg.DocumentedTypes,
			pkg.ExportedFuncs, pkg.DocumentedFuncs, pkg.Coverage))
	}

	return sb.sb.String()
}

type stringBuilder struct {
	sb strings.Builder
}

func (sb *stringBuilder) WriteString(s string) {
	sb.sb.WriteString(s)
}

// RunBlueprintValidation validates blueprint files
func RunBlueprintValidation(rootDir string, verbose bool) {
	fmt.Println("Validating blueprint files...")
	validator := NewBlueprintValidator(rootDir)
	inventory := validator.ValidateBlueprints(verbose)

	// Generate report
	report := GenerateBlueprintReport(inventory)
	outputPath := "./scripts/docvalidation/blueprint-validation-report.md"
	//nolint:gosec // G306: validation reports need to be readable by developers
	if err := os.WriteFile(outputPath, []byte(report), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}
	fmt.Printf("Blueprint validation report written to %s\n", outputPath)

	// Print summary
	fmt.Printf("\nBlueprint Validation Summary:\n")
	fmt.Printf("  Blueprints: %d\n", inventory.Summary.TotalBlueprints)
	fmt.Printf("  State Files: %d\n", inventory.Summary.TotalStateFiles)
	fmt.Printf("  Total States: %d\n", inventory.Summary.TotalStates)
	fmt.Printf("  Valid Files: %d\n", inventory.Summary.ValidFiles)
	fmt.Printf("  Invalid Files: %d\n", inventory.Summary.InvalidFiles)
	fmt.Printf("  Files with Warnings: %d\n", inventory.Summary.FilesWithWarnings)

	if inventory.Summary.InvalidFiles > 0 {
		fmt.Printf("\nWARNING: %d invalid blueprint files found\n", inventory.Summary.InvalidFiles)
	} else {
		fmt.Println("\nAll blueprint files are valid!")
	}
}
