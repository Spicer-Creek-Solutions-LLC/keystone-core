package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DriftReport contains documentation drift analysis
type DriftReport struct {
	NewPackages       []string            `json:"new_packages"`        // Packages with no docs
	UndocumentedTypes []UndocumentedItem  `json:"undocumented_types"`  // New exported types without godoc
	UndocumentedFuncs []UndocumentedItem  `json:"undocumented_funcs"`  // New exported funcs without godoc
	StaleReferences   []StaleReference    `json:"stale_references"`    // Docs referencing non-existent code
	MissingEpicDocs   []int               `json:"missing_epic_docs"`   // Epics without documentation
	APIChanges        []APIChange         `json:"api_changes"`         // Potential API changes
}

// UndocumentedItem represents an exported symbol without documentation
type UndocumentedItem struct {
	Package  string `json:"package"`
	Name     string `json:"name"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// StaleReference represents a documentation reference to non-existent code
type StaleReference struct {
	DocFile   string `json:"doc_file"`
	Reference string `json:"reference"`
	Type      string `json:"type"` // function, type, package
}

// APIChange represents a potential API change that may need doc updates
type APIChange struct {
	Package     string `json:"package"`
	Symbol      string `json:"symbol"`
	ChangeType  string `json:"change_type"` // signature_change, deprecated, removed
	Description string `json:"description"`
}

// RunDriftDetection detects documentation drift from code changes
func RunDriftDetection(rootDir string, verbose bool) {
	fmt.Println("Detecting documentation drift...")

	report := &DriftReport{}

	pkgRoot := filepath.Join(rootDir, "pkg")
	docsRoot := filepath.Join(rootDir, "docs", "content", "en", "docs")

	// 1. Find packages without documentation
	report.NewPackages = findUndocumentedPackages(pkgRoot, docsRoot, verbose)

	// 2. Find undocumented exported symbols
	report.UndocumentedTypes, report.UndocumentedFuncs = findUndocumentedSymbols(pkgRoot, verbose)

	// 3. Find stale references in docs
	report.StaleReferences = findStaleReferences(docsRoot, pkgRoot, verbose)

	// 4. Find epics without documentation
	report.MissingEpicDocs = findMissingEpicDocs(docsRoot, verbose)

	// Generate report
	printDriftReport(report)

	// Write report to file
	writeReport := formatDriftReport(report)
	outputPath := "./scripts/docvalidation/drift-report.md"
	if err := os.WriteFile(outputPath, []byte(writeReport), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing report: %v\n", err)
		return
	}
	fmt.Printf("Drift report written to %s\n", outputPath)
}

func findUndocumentedPackages(pkgRoot, docsRoot string, verbose bool) []string {
	var undocumented []string

	// Get all packages
	entries, err := os.ReadDir(pkgRoot)
	if err != nil {
		return undocumented
	}

	// Known documentation mappings (package -> doc mentions)
	pkgDocMentions := make(map[string]bool)

	// Scan docs for package mentions
	filepath.Walk(docsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		text := string(content)
		for _, entry := range entries {
			if entry.IsDir() {
				// Check if package is mentioned in docs
				pkgName := entry.Name()
				if strings.Contains(text, "pkg/"+pkgName) ||
					strings.Contains(text, "`"+pkgName+"`") ||
					strings.Contains(text, "package "+pkgName) {
					pkgDocMentions[pkgName] = true
				}
			}
		}
		return nil
	})

	// Find packages not mentioned in any docs
	for _, entry := range entries {
		if entry.IsDir() {
			pkgName := entry.Name()
			if !pkgDocMentions[pkgName] {
				// Check if package has any Go files
				files, _ := filepath.Glob(filepath.Join(pkgRoot, pkgName, "*.go"))
				if len(files) > 0 {
					undocumented = append(undocumented, pkgName)
					if verbose {
						fmt.Printf("  Package not documented: %s\n", pkgName)
					}
				}
			}
		}
	}

	return undocumented
}

func findUndocumentedSymbols(pkgRoot string, verbose bool) ([]UndocumentedItem, []UndocumentedItem) {
	var undocTypes []UndocumentedItem
	var undocFuncs []UndocumentedItem

	entries, err := os.ReadDir(pkgRoot)
	if err != nil {
		return undocTypes, undocFuncs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pkgPath := filepath.Join(pkgRoot, entry.Name())
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, pkgPath, func(fi os.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, parser.ParseComments)
		if err != nil {
			continue
		}

		for _, p := range pkgs {
			for filename, f := range p.Files {
				for _, decl := range f.Decls {
					switch d := decl.(type) {
					case *ast.GenDecl:
						for _, spec := range d.Specs {
							if ts, ok := spec.(*ast.TypeSpec); ok {
								if ts.Name.IsExported() && (d.Doc == nil || d.Doc.Text() == "") {
									pos := fset.Position(ts.Pos())
									undocTypes = append(undocTypes, UndocumentedItem{
										Package: entry.Name(),
										Name:    ts.Name.Name,
										File:    filepath.Base(filename),
										Line:    pos.Line,
									})
									if verbose {
										fmt.Printf("  Undocumented type: %s.%s\n", entry.Name(), ts.Name.Name)
									}
								}
							}
						}
					case *ast.FuncDecl:
						if d.Name.IsExported() && (d.Doc == nil || d.Doc.Text() == "") {
							pos := fset.Position(d.Pos())
							undocFuncs = append(undocFuncs, UndocumentedItem{
								Package: entry.Name(),
								Name:    d.Name.Name,
								File:    filepath.Base(filename),
								Line:    pos.Line,
							})
							if verbose {
								fmt.Printf("  Undocumented func: %s.%s\n", entry.Name(), d.Name.Name)
							}
						}
					}
				}
			}
		}
	}

	return undocTypes, undocFuncs
}

func findStaleReferences(docsRoot, pkgRoot string, verbose bool) []StaleReference {
	var stale []StaleReference

	// Build set of known packages and exported symbols
	knownPackages := make(map[string]bool)
	knownSymbols := make(map[string]bool) // "pkg.Symbol" format

	entries, _ := os.ReadDir(pkgRoot)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pkgName := entry.Name()
		knownPackages[pkgName] = true

		pkgPath := filepath.Join(pkgRoot, pkgName)
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, pkgPath, func(fi os.FileInfo) bool {
			return !strings.HasSuffix(fi.Name(), "_test.go")
		}, parser.ParseComments)
		if err != nil {
			continue
		}

		for _, p := range pkgs {
			for _, f := range p.Files {
				for _, decl := range f.Decls {
					switch d := decl.(type) {
					case *ast.GenDecl:
						for _, spec := range d.Specs {
							if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
								knownSymbols[pkgName+"."+ts.Name.Name] = true
							}
						}
					case *ast.FuncDecl:
						if d.Name.IsExported() {
							knownSymbols[pkgName+"."+d.Name.Name] = true
						}
					}
				}
			}
		}
	}

	// Scan docs for references to packages/symbols
	pkgRefRe := regexp.MustCompile(`pkg/(\w+)`)
	symbolRefRe := regexp.MustCompile(`(\w+)\.([A-Z]\w+)`)

	filepath.Walk(docsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(docsRoot, path)
		text := string(content)

		// Check package references
		pkgMatches := pkgRefRe.FindAllStringSubmatch(text, -1)
		for _, match := range pkgMatches {
			if len(match) > 1 {
				pkgName := match[1]
				if !knownPackages[pkgName] {
					stale = append(stale, StaleReference{
						DocFile:   relPath,
						Reference: "pkg/" + pkgName,
						Type:      "package",
					})
					if verbose {
						fmt.Printf("  Stale reference in %s: pkg/%s\n", relPath, pkgName)
					}
				}
			}
		}

		// Check symbol references (Type.Method or pkg.Type patterns)
		symbolMatches := symbolRefRe.FindAllStringSubmatch(text, -1)
		seen := make(map[string]bool)
		for _, match := range symbolMatches {
			if len(match) > 2 {
				ref := match[1] + "." + match[2]
				if seen[ref] {
					continue
				}
				seen[ref] = true

				// Only flag if the package exists but symbol doesn't
				if knownPackages[match[1]] && !knownSymbols[ref] {
					stale = append(stale, StaleReference{
						DocFile:   relPath,
						Reference: ref,
						Type:      "symbol",
					})
					if verbose {
						fmt.Printf("  Stale symbol in %s: %s\n", relPath, ref)
					}
				}
			}
		}

		return nil
	})

	return stale
}

func findMissingEpicDocs(docsRoot string, verbose bool) []int {
	var missing []int

	// Known implemented epics
	implementedEpics := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23}

	// Check which epics are mentioned in docs
	epicMentioned := make(map[int]bool)

	filepath.Walk(docsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		text := string(content)
		for _, epic := range implementedEpics {
			pattern := fmt.Sprintf("Epic %d", epic)
			if strings.Contains(text, pattern) {
				epicMentioned[epic] = true
			}
		}
		return nil
	})

	for _, epic := range implementedEpics {
		if !epicMentioned[epic] {
			missing = append(missing, epic)
			if verbose {
				fmt.Printf("  Epic %d not mentioned in docs\n", epic)
			}
		}
	}

	return missing
}

func printDriftReport(report *DriftReport) {
	fmt.Println()
	fmt.Println("=== Documentation Drift Report ===")
	fmt.Println()

	if len(report.NewPackages) > 0 {
		fmt.Printf("Packages without documentation: %d\n", len(report.NewPackages))
		for _, pkg := range report.NewPackages {
			fmt.Printf("  - %s\n", pkg)
		}
		fmt.Println()
	}

	if len(report.UndocumentedTypes) > 0 {
		fmt.Printf("Undocumented exported types: %d\n", len(report.UndocumentedTypes))
		if len(report.UndocumentedTypes) <= 10 {
			for _, item := range report.UndocumentedTypes {
				fmt.Printf("  - %s.%s (%s:%d)\n", item.Package, item.Name, item.File, item.Line)
			}
		} else {
			// Group by package
			byPkg := make(map[string]int)
			for _, item := range report.UndocumentedTypes {
				byPkg[item.Package]++
			}
			for pkg, count := range byPkg {
				fmt.Printf("  - %s: %d undocumented types\n", pkg, count)
			}
		}
		fmt.Println()
	}

	if len(report.UndocumentedFuncs) > 0 {
		fmt.Printf("Undocumented exported functions: %d\n", len(report.UndocumentedFuncs))
		// Group by package
		byPkg := make(map[string]int)
		for _, item := range report.UndocumentedFuncs {
			byPkg[item.Package]++
		}
		pkgs := make([]string, 0, len(byPkg))
		for pkg := range byPkg {
			pkgs = append(pkgs, pkg)
		}
		sort.Strings(pkgs)
		for _, pkg := range pkgs {
			fmt.Printf("  - %s: %d undocumented functions\n", pkg, byPkg[pkg])
		}
		fmt.Println()
	}

	if len(report.StaleReferences) > 0 {
		fmt.Printf("Stale references in docs: %d\n", len(report.StaleReferences))
		for _, ref := range report.StaleReferences[:min(10, len(report.StaleReferences))] {
			fmt.Printf("  - %s: %s (%s)\n", ref.DocFile, ref.Reference, ref.Type)
		}
		if len(report.StaleReferences) > 10 {
			fmt.Printf("  ... and %d more\n", len(report.StaleReferences)-10)
		}
		fmt.Println()
	}

	if len(report.MissingEpicDocs) > 0 {
		fmt.Printf("Epics without documentation mentions: %d\n", len(report.MissingEpicDocs))
		for _, epic := range report.MissingEpicDocs {
			fmt.Printf("  - Epic %d\n", epic)
		}
		fmt.Println()
	}

	// Summary
	total := len(report.NewPackages) + len(report.UndocumentedTypes) +
		len(report.UndocumentedFuncs) + len(report.StaleReferences) +
		len(report.MissingEpicDocs)
	if total == 0 {
		fmt.Println("No documentation drift detected!")
	} else {
		fmt.Printf("Total issues: %d\n", total)
	}
}

func formatDriftReport(report *DriftReport) string {
	var sb strings.Builder

	sb.WriteString("# Documentation Drift Report\n\n")

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("| Category | Count |\n"))
	sb.WriteString("|----------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Packages without docs | %d |\n", len(report.NewPackages)))
	sb.WriteString(fmt.Sprintf("| Undocumented types | %d |\n", len(report.UndocumentedTypes)))
	sb.WriteString(fmt.Sprintf("| Undocumented functions | %d |\n", len(report.UndocumentedFuncs)))
	sb.WriteString(fmt.Sprintf("| Stale references | %d |\n", len(report.StaleReferences)))
	sb.WriteString(fmt.Sprintf("| Epics without docs | %d |\n", len(report.MissingEpicDocs)))
	sb.WriteString("\n")

	// Details
	if len(report.NewPackages) > 0 {
		sb.WriteString("## Packages Without Documentation\n\n")
		for _, pkg := range report.NewPackages {
			sb.WriteString(fmt.Sprintf("- `%s`\n", pkg))
		}
		sb.WriteString("\n")
	}

	if len(report.UndocumentedTypes) > 0 {
		sb.WriteString("## Undocumented Types\n\n")
		sb.WriteString("| Package | Type | File | Line |\n")
		sb.WriteString("|---------|------|------|------|\n")
		for _, item := range report.UndocumentedTypes {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d |\n",
				item.Package, item.Name, item.File, item.Line))
		}
		sb.WriteString("\n")
	}

	if len(report.UndocumentedFuncs) > 0 {
		sb.WriteString("## Undocumented Functions\n\n")
		// Group by package
		byPkg := make(map[string][]UndocumentedItem)
		for _, item := range report.UndocumentedFuncs {
			byPkg[item.Package] = append(byPkg[item.Package], item)
		}
		pkgs := make([]string, 0, len(byPkg))
		for pkg := range byPkg {
			pkgs = append(pkgs, pkg)
		}
		sort.Strings(pkgs)

		for _, pkg := range pkgs {
			items := byPkg[pkg]
			sb.WriteString(fmt.Sprintf("### %s (%d functions)\n\n", pkg, len(items)))
			for _, item := range items {
				sb.WriteString(fmt.Sprintf("- `%s` (%s:%d)\n", item.Name, item.File, item.Line))
			}
			sb.WriteString("\n")
		}
	}

	if len(report.StaleReferences) > 0 {
		sb.WriteString("## Stale References\n\n")
		sb.WriteString("| Doc File | Reference | Type |\n")
		sb.WriteString("|----------|-----------|------|\n")
		for _, ref := range report.StaleReferences {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
				ref.DocFile, ref.Reference, ref.Type))
		}
		sb.WriteString("\n")
	}

	if len(report.MissingEpicDocs) > 0 {
		sb.WriteString("## Epics Without Documentation\n\n")
		for _, epic := range report.MissingEpicDocs {
			sb.WriteString(fmt.Sprintf("- Epic %d\n", epic))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
