// Package main provides documentation validation tooling for Keystone Core.
// It inventories documentation, checks godoc coverage, validates links,
// and generates reports on documentation health.
package main

import (
	"encoding/json"
	"flag"
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

// DocFile represents a documentation file
type DocFile struct {
	Path        string   `json:"path"`
	RelPath     string   `json:"rel_path"`
	Title       string   `json:"title"`
	Category    string   `json:"category"`
	Epics       []int    `json:"epics,omitempty"`
	LineCount   int      `json:"line_count"`
	WordCount   int      `json:"word_count"`
	HasExamples bool     `json:"has_examples"`
	Links       []string `json:"links,omitempty"`
}

// Package represents a Go package for documentation analysis
type Package struct {
	Path            string  `json:"path"`
	Name            string  `json:"name"`
	Epic            []int   `json:"epics,omitempty"`
	HasPackageDoc   bool    `json:"has_package_doc"`
	ExportedTypes   int     `json:"exported_types"`
	DocumentedTypes int     `json:"documented_types"`
	ExportedFuncs   int     `json:"exported_funcs"`
	DocumentedFuncs int     `json:"documented_funcs"`
	Coverage        float64 `json:"coverage_percent"`
}

// DocInventory holds the complete documentation inventory
type DocInventory struct {
	DocsRoot     string             `json:"docs_root"`
	PkgRoot      string             `json:"pkg_root"`
	DocFiles     []DocFile          `json:"doc_files"`
	Packages     []Package          `json:"packages"`
	EpicCoverage map[int][]string   `json:"epic_coverage"`
	Summary      InventorySummary   `json:"summary"`
}

// InventorySummary provides summary statistics
type InventorySummary struct {
	TotalDocs           int     `json:"total_docs"`
	TotalPackages       int     `json:"total_packages"`
	DocsWithExamples    int     `json:"docs_with_examples"`
	AvgGodocCoverage    float64 `json:"avg_godoc_coverage"`
	PackagesFullyCovered int    `json:"packages_fully_covered"`
	PackagesNoCoverage  int     `json:"packages_no_coverage"`
	TotalExportedSymbols int    `json:"total_exported_symbols"`
	TotalDocumentedSymbols int  `json:"total_documented_symbols"`
}

// Epic to package mapping
var epicPackageMap = map[int][]string{
	1:  {"agent", "controlplane", "state", "config", "nats", "security"},
	2:  {"execution", "targeting", "plugin"},
	3:  {"statemgmt"},
	4:  {"events"},
	5:  {"gitops"},
	6:  {"policy"},
	7:  {"metrics", "logging", "tracing", "health", "profiling", "query", "visualization"},
	8:  {"k8s", "platform", "cloud", "edge", "hardware", "container", "servicemesh"},
	9:  {"module"},
	11: {"cluster"},
	14: {"nats"},
	15: {"audit", "logging"},
	17: {"identity"},
	19: {"gateway"},
	21: {"proxy", "credentials", "protocols", "vendors"},
	22: {"files"},
	23: {"bootstrap", "backup", "selfmgmt", "upgrade"},
}

// Epic to doc mapping (concept pages)
var epicDocMap = map[int][]string{
	1:  {"control-plane.md", "agents.md", "message-bus.md"},
	2:  {"remote-execution.md"},
	3:  {"state-management.md", "modules.md"},
	4:  {"events.md", "reactors.md"},
	5:  {"gitops.md"},
	6:  {"policy.md"},
	7:  {"observability.md"},
	9:  {"modules.md"},
	14: {"nats-mesh.md"},
	17: {"identity.md"},
	21: {"proxy-agents.md"},
	22: {"file-distribution.md"},
}

func main() {
	// Check for subcommand
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "links":
			runLinksCommand(os.Args[2:])
			return
		case "examples":
			runExamplesCommand(os.Args[2:])
			return
		case "godoc":
			runGodocCommand(os.Args[2:])
			return
		case "drift":
			runDriftCommand(os.Args[2:])
			return
		case "sync":
			runSyncCommand(os.Args[2:])
			return
		case "blueprints":
			runBlueprintsCommand(os.Args[2:])
			return
		case "all":
			runAllCommand(os.Args[2:])
			return
		case "help", "-h", "--help":
			printUsage()
			return
		}
	}

	// Default: run inventory
	runInventoryCommand(os.Args[1:])
}

func printUsage() {
	fmt.Print(`Keystone Core Documentation Validation Tool

Usage:
  docvalidation [command] [flags]

Commands:
  (default)   Run documentation inventory
  links       Check documentation links
  examples    Validate code examples
  godoc       Generate godoc coverage report
  drift       Detect documentation drift from code changes
  sync        Check README/CLAUDE.md/docs synchronization
  blueprints  Validate blueprint example files
  all         Run all validations

Flags:
  -root       Root directory of keystone-core (default ".")
  -output     Output file (stdout if empty)
  -format     Output format: text, json, markdown (default "text")
  -verbose    Verbose output
  -external   Check external links (links command only)

Examples:
  docvalidation                           # Run inventory
  docvalidation links -verbose            # Check links with verbose output
  docvalidation examples                  # Validate code examples
  docvalidation godoc                     # Generate godoc coverage report
  docvalidation drift                     # Detect code/docs drift
  docvalidation blueprints -verbose       # Validate blueprints
  docvalidation all -root /path/to/repo   # Run all validations
`)
}

func runInventoryCommand(args []string) {
	fs := flag.NewFlagSet("inventory", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	outputFile := fs.String("output", "", "Output file for report")
	format := fs.String("format", "text", "Output format: text, json, markdown")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	docsRoot := filepath.Join(*rootDir, "docs", "content", "en", "docs")
	pkgRoot := filepath.Join(*rootDir, "pkg")
	runbooksRoot := filepath.Join(*rootDir, "docs", "runbooks")

	inventory := &DocInventory{
		DocsRoot:     docsRoot,
		PkgRoot:      pkgRoot,
		EpicCoverage: make(map[int][]string),
	}

	// Inventory documentation files
	if err := inventoryDocs(docsRoot, inventory, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error inventorying docs: %v\n", err)
		os.Exit(1)
	}

	// Inventory runbooks
	if err := inventoryRunbooks(runbooksRoot, inventory, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error inventorying runbooks: %v\n", err)
		os.Exit(1)
	}

	// Inventory packages
	if err := inventoryPackages(pkgRoot, inventory, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error inventorying packages: %v\n", err)
		os.Exit(1)
	}

	// Build epic coverage map
	buildEpicCoverage(inventory)

	// Calculate summary
	calculateSummary(inventory)

	// Output results
	var output string
	switch *format {
	case "json":
		data, _ := json.MarshalIndent(inventory, "", "  ")
		output = string(data)
	case "markdown":
		output = formatMarkdown(inventory)
	default:
		output = formatText(inventory)
	}

	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Report written to %s\n", *outputFile)
	} else {
		fmt.Println(output)
	}
}

func runLinksCommand(args []string) {
	fs := flag.NewFlagSet("links", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	verbose := fs.Bool("verbose", false, "Verbose output")
	external := fs.Bool("external", false, "Check external links")
	fs.Parse(args)

	RunLinkCheck(*rootDir, *external, *verbose)
}

func runExamplesCommand(args []string) {
	fs := flag.NewFlagSet("examples", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	RunExampleValidation(*rootDir, *verbose)
}

func runGodocCommand(args []string) {
	fs := flag.NewFlagSet("godoc", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	GodocCoverageReport(*rootDir, *verbose)
}

func runDriftCommand(args []string) {
	fs := flag.NewFlagSet("drift", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	RunDriftDetection(*rootDir, *verbose)
}

func runSyncCommand(args []string) {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	RunSyncCheck(*rootDir, *verbose)
}

func runBlueprintsCommand(args []string) {
	fs := flag.NewFlagSet("blueprints", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	verbose := fs.Bool("verbose", false, "Verbose output")
	fs.Parse(args)

	RunBlueprintValidation(*rootDir, *verbose)
}

func runAllCommand(args []string) {
	fs := flag.NewFlagSet("all", flag.ExitOnError)
	rootDir := fs.String("root", ".", "Root directory of keystone-core")
	verbose := fs.Bool("verbose", false, "Verbose output")
	external := fs.Bool("external", false, "Check external links")
	fs.Parse(args)

	fmt.Println("=== Running Full Documentation Validation ===")
	fmt.Println()

	// Run inventory
	fmt.Println("--- Documentation Inventory ---")
	runInventoryCommand([]string{"-root", *rootDir, "-format", "text"})
	fmt.Println()

	// Run link check
	fmt.Println("--- Link Validation ---")
	RunLinkCheck(*rootDir, *external, *verbose)
	fmt.Println()

	// Run example validation
	fmt.Println("--- Code Example Validation ---")
	RunExampleValidation(*rootDir, *verbose)
	fmt.Println()

	// Run godoc coverage
	fmt.Println("--- Godoc Coverage Report ---")
	GodocCoverageReport(*rootDir, *verbose)
	fmt.Println()

	// Run drift detection
	fmt.Println("--- Documentation Drift Detection ---")
	RunDriftDetection(*rootDir, *verbose)
	fmt.Println()

	// Run sync check
	fmt.Println("--- Documentation Sync Check ---")
	RunSyncCheck(*rootDir, *verbose)
	fmt.Println()

	// Run blueprint validation
	fmt.Println("--- Blueprint Validation ---")
	RunBlueprintValidation(*rootDir, *verbose)
	fmt.Println()

	fmt.Println("=== Validation Complete ===")
}

func inventoryDocs(root string, inv *DocInventory, verbose bool) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(root, path)
		doc := DocFile{
			Path:    path,
			RelPath: relPath,
		}

		// Determine category from path
		parts := strings.Split(relPath, string(os.PathSeparator))
		if len(parts) > 0 {
			doc.Category = parts[0]
		}

		// Parse file content
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(content), "\n")
		doc.LineCount = len(lines)
		doc.WordCount = len(strings.Fields(string(content)))

		// Extract title from frontmatter or first heading
		doc.Title = extractTitle(lines)

		// Check for code examples
		doc.HasExamples = strings.Contains(string(content), "```")

		// Extract links
		doc.Links = extractLinks(string(content))

		// Map to epics
		doc.Epics = mapDocToEpics(relPath)

		inv.DocFiles = append(inv.DocFiles, doc)

		if verbose {
			fmt.Printf("  Doc: %s (%d lines, %d words)\n", relPath, doc.LineCount, doc.WordCount)
		}

		return nil
	})
}

func inventoryRunbooks(root string, inv *DocInventory, verbose bool) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil // Runbooks directory doesn't exist
	}

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(filepath.Dir(root), path)
		doc := DocFile{
			Path:     path,
			RelPath:  relPath,
			Category: "runbooks",
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lines := strings.Split(string(content), "\n")
		doc.LineCount = len(lines)
		doc.WordCount = len(strings.Fields(string(content)))
		doc.Title = extractTitle(lines)
		doc.HasExamples = strings.Contains(string(content), "```")
		doc.Links = extractLinks(string(content))

		inv.DocFiles = append(inv.DocFiles, doc)

		if verbose {
			fmt.Printf("  Runbook: %s (%d lines)\n", relPath, doc.LineCount)
		}

		return nil
	})
}

func inventoryPackages(root string, inv *DocInventory, verbose bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pkgPath := filepath.Join(root, entry.Name())
		pkg := analyzePackage(pkgPath, entry.Name())
		pkg.Epic = mapPackageToEpics(entry.Name())

		inv.Packages = append(inv.Packages, pkg)

		if verbose {
			fmt.Printf("  Package: %s (coverage: %.1f%%)\n", entry.Name(), pkg.Coverage)
		}
	}

	return nil
}

func analyzePackage(pkgPath, name string) Package {
	pkg := Package{
		Path: pkgPath,
		Name: name,
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgPath, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return pkg
	}

	for _, p := range pkgs {
		// Check for package doc
		for _, f := range p.Files {
			if f.Doc != nil && f.Doc.Text() != "" {
				pkg.HasPackageDoc = true
				break
			}
		}

		// Count exported types and functions
		for _, f := range p.Files {
			for _, decl := range f.Decls {
				switch d := decl.(type) {
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							if ts.Name.IsExported() {
								pkg.ExportedTypes++
								if d.Doc != nil && d.Doc.Text() != "" {
									pkg.DocumentedTypes++
								}
							}
						}
					}
				case *ast.FuncDecl:
					if d.Name.IsExported() {
						pkg.ExportedFuncs++
						if d.Doc != nil && d.Doc.Text() != "" {
							pkg.DocumentedFuncs++
						}
					}
				}
			}
		}
	}

	total := pkg.ExportedTypes + pkg.ExportedFuncs
	documented := pkg.DocumentedTypes + pkg.DocumentedFuncs
	if total > 0 {
		pkg.Coverage = float64(documented) / float64(total) * 100
	}

	return pkg
}

func extractTitle(lines []string) string {
	inFrontmatter := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			inFrontmatter = !inFrontmatter
			continue
		}
		if inFrontmatter && strings.HasPrefix(line, "title:") {
			title := strings.TrimPrefix(line, "title:")
			title = strings.Trim(strings.TrimSpace(title), "\"'")
			return title
		}
		if !inFrontmatter && strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

func extractLinks(content string) []string {
	var links []string
	// Match markdown links [text](url)
	re := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 2 {
			links = append(links, m[2])
		}
	}
	return links
}

func mapDocToEpics(relPath string) []int {
	var epics []int
	filename := filepath.Base(relPath)

	for epic, docs := range epicDocMap {
		for _, doc := range docs {
			if filename == doc {
				epics = append(epics, epic)
			}
		}
	}

	// Also check by content/category
	if strings.Contains(relPath, "nats-mesh") {
		epics = appendUnique(epics, 14)
	}
	if strings.Contains(relPath, "identity") {
		epics = appendUnique(epics, 17)
	}
	if strings.Contains(relPath, "file-") || strings.Contains(relPath, "files") {
		epics = appendUnique(epics, 22)
	}
	if strings.Contains(relPath, "gateway") {
		epics = appendUnique(epics, 19)
	}
	if strings.Contains(relPath, "windows") {
		epics = appendUnique(epics, 20)
	}
	if strings.Contains(relPath, "ipv6") {
		epics = appendUnique(epics, 18)
	}
	if strings.Contains(relPath, "self-management") || strings.Contains(relPath, "bootstrap") || strings.Contains(relPath, "backup") || strings.Contains(relPath, "upgrade") {
		epics = appendUnique(epics, 23)
	}

	return epics
}

func mapPackageToEpics(pkgName string) []int {
	var epics []int
	for epic, pkgs := range epicPackageMap {
		for _, pkg := range pkgs {
			if pkg == pkgName {
				epics = append(epics, epic)
			}
		}
	}
	return epics
}

func appendUnique(slice []int, val int) []int {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

func buildEpicCoverage(inv *DocInventory) {
	for _, doc := range inv.DocFiles {
		for _, epic := range doc.Epics {
			inv.EpicCoverage[epic] = append(inv.EpicCoverage[epic], doc.RelPath)
		}
	}
}

func calculateSummary(inv *DocInventory) {
	inv.Summary.TotalDocs = len(inv.DocFiles)
	inv.Summary.TotalPackages = len(inv.Packages)

	for _, doc := range inv.DocFiles {
		if doc.HasExamples {
			inv.Summary.DocsWithExamples++
		}
	}

	var totalCoverage float64
	for _, pkg := range inv.Packages {
		totalCoverage += pkg.Coverage
		inv.Summary.TotalExportedSymbols += pkg.ExportedTypes + pkg.ExportedFuncs
		inv.Summary.TotalDocumentedSymbols += pkg.DocumentedTypes + pkg.DocumentedFuncs
		if pkg.Coverage >= 100 {
			inv.Summary.PackagesFullyCovered++
		}
		if pkg.Coverage == 0 {
			inv.Summary.PackagesNoCoverage++
		}
	}
	if len(inv.Packages) > 0 {
		inv.Summary.AvgGodocCoverage = totalCoverage / float64(len(inv.Packages))
	}
}

func checkInternalLinks(inv *DocInventory, rootDir string, verbose bool) {
	docPaths := make(map[string]bool)
	for _, doc := range inv.DocFiles {
		docPaths[doc.RelPath] = true
	}

	for i := range inv.DocFiles {
		doc := &inv.DocFiles[i]
		var validLinks []string
		for _, link := range doc.Links {
			// Skip external links
			if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
				continue
			}
			// Skip anchors
			if strings.HasPrefix(link, "#") {
				continue
			}
			validLinks = append(validLinks, link)
		}
		doc.Links = validLinks
	}
}

func formatText(inv *DocInventory) string {
	var sb strings.Builder

	sb.WriteString("=" + strings.Repeat("=", 70) + "\n")
	sb.WriteString("KEYSTONE CORE DOCUMENTATION INVENTORY\n")
	sb.WriteString("=" + strings.Repeat("=", 70) + "\n\n")

	// Summary
	sb.WriteString("SUMMARY\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n")
	sb.WriteString(fmt.Sprintf("Total Documentation Files:    %d\n", inv.Summary.TotalDocs))
	sb.WriteString(fmt.Sprintf("Docs with Examples:           %d (%.1f%%)\n",
		inv.Summary.DocsWithExamples,
		float64(inv.Summary.DocsWithExamples)/float64(inv.Summary.TotalDocs)*100))
	sb.WriteString(fmt.Sprintf("Total Packages:               %d\n", inv.Summary.TotalPackages))
	sb.WriteString(fmt.Sprintf("Average Godoc Coverage:       %.1f%%\n", inv.Summary.AvgGodocCoverage))
	sb.WriteString(fmt.Sprintf("Packages Fully Documented:    %d\n", inv.Summary.PackagesFullyCovered))
	sb.WriteString(fmt.Sprintf("Packages No Documentation:    %d\n", inv.Summary.PackagesNoCoverage))
	sb.WriteString(fmt.Sprintf("Total Exported Symbols:       %d\n", inv.Summary.TotalExportedSymbols))
	sb.WriteString(fmt.Sprintf("Total Documented Symbols:     %d\n", inv.Summary.TotalDocumentedSymbols))
	sb.WriteString("\n")

	// Documentation by category
	sb.WriteString("DOCUMENTATION BY CATEGORY\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n")
	categories := make(map[string][]DocFile)
	for _, doc := range inv.DocFiles {
		categories[doc.Category] = append(categories[doc.Category], doc)
	}

	catNames := make([]string, 0, len(categories))
	for cat := range categories {
		catNames = append(catNames, cat)
	}
	sort.Strings(catNames)

	for _, cat := range catNames {
		docs := categories[cat]
		sb.WriteString(fmt.Sprintf("\n%s (%d files)\n", strings.ToUpper(cat), len(docs)))
		for _, doc := range docs {
			examples := ""
			if doc.HasExamples {
				examples = " [has examples]"
			}
			sb.WriteString(fmt.Sprintf("  - %s (%d lines)%s\n", doc.RelPath, doc.LineCount, examples))
		}
	}
	sb.WriteString("\n")

	// Package godoc coverage
	sb.WriteString("PACKAGE GODOC COVERAGE\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n")

	// Sort by coverage ascending (worst first)
	pkgs := make([]Package, len(inv.Packages))
	copy(pkgs, inv.Packages)
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Coverage < pkgs[j].Coverage
	})

	sb.WriteString(fmt.Sprintf("%-20s %8s %8s %8s %8s\n", "Package", "Types", "Funcs", "DocTypes", "DocFuncs"))
	sb.WriteString(strings.Repeat("-", 60) + "\n")
	for _, pkg := range pkgs {
		status := "⚠️"
		if pkg.Coverage >= 80 {
			status = "✅"
		} else if pkg.Coverage == 0 {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("%-20s %8d %8d %8d %8d  %s %.1f%%\n",
			pkg.Name, pkg.ExportedTypes, pkg.ExportedFuncs,
			pkg.DocumentedTypes, pkg.DocumentedFuncs, status, pkg.Coverage))
	}
	sb.WriteString("\n")

	// Epic coverage
	sb.WriteString("EPIC DOCUMENTATION COVERAGE\n")
	sb.WriteString(strings.Repeat("-", 40) + "\n")

	epics := make([]int, 0, len(inv.EpicCoverage))
	for epic := range inv.EpicCoverage {
		epics = append(epics, epic)
	}
	sort.Ints(epics)

	for _, epic := range epics {
		docs := inv.EpicCoverage[epic]
		sb.WriteString(fmt.Sprintf("Epic %d: %d doc(s)\n", epic, len(docs)))
		for _, doc := range docs {
			sb.WriteString(fmt.Sprintf("  - %s\n", doc))
		}
	}

	return sb.String()
}

func formatMarkdown(inv *DocInventory) string {
	var sb strings.Builder

	sb.WriteString("# Keystone Core Documentation Inventory\n\n")

	// Summary table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Documentation Files | %d |\n", inv.Summary.TotalDocs))
	sb.WriteString(fmt.Sprintf("| Docs with Examples | %d (%.1f%%) |\n",
		inv.Summary.DocsWithExamples,
		float64(inv.Summary.DocsWithExamples)/float64(inv.Summary.TotalDocs)*100))
	sb.WriteString(fmt.Sprintf("| Total Packages | %d |\n", inv.Summary.TotalPackages))
	sb.WriteString(fmt.Sprintf("| Average Godoc Coverage | %.1f%% |\n", inv.Summary.AvgGodocCoverage))
	sb.WriteString(fmt.Sprintf("| Packages Fully Documented | %d |\n", inv.Summary.PackagesFullyCovered))
	sb.WriteString(fmt.Sprintf("| Packages No Documentation | %d |\n", inv.Summary.PackagesNoCoverage))
	sb.WriteString(fmt.Sprintf("| Total Exported Symbols | %d |\n", inv.Summary.TotalExportedSymbols))
	sb.WriteString(fmt.Sprintf("| Documented Symbols | %d |\n", inv.Summary.TotalDocumentedSymbols))
	sb.WriteString("\n")

	// Package coverage table
	sb.WriteString("## Package Godoc Coverage\n\n")
	sb.WriteString("| Package | Exported Types | Documented Types | Exported Funcs | Documented Funcs | Coverage |\n")
	sb.WriteString("|---------|---------------|------------------|----------------|------------------|----------|\n")

	pkgs := make([]Package, len(inv.Packages))
	copy(pkgs, inv.Packages)
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})

	for _, pkg := range pkgs {
		status := "⚠️"
		if pkg.Coverage >= 80 {
			status = "✅"
		} else if pkg.Coverage == 0 {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %s %.1f%% |\n",
			pkg.Name, pkg.ExportedTypes, pkg.DocumentedTypes,
			pkg.ExportedFuncs, pkg.DocumentedFuncs, status, pkg.Coverage))
	}
	sb.WriteString("\n")

	// Documentation by category
	sb.WriteString("## Documentation by Category\n\n")
	categories := make(map[string][]DocFile)
	for _, doc := range inv.DocFiles {
		categories[doc.Category] = append(categories[doc.Category], doc)
	}

	catNames := make([]string, 0, len(categories))
	for cat := range categories {
		catNames = append(catNames, cat)
	}
	sort.Strings(catNames)

	for _, cat := range catNames {
		docs := categories[cat]
		sb.WriteString(fmt.Sprintf("### %s (%d files)\n\n", strings.Title(cat), len(docs)))
		sb.WriteString("| File | Lines | Has Examples |\n")
		sb.WriteString("|------|-------|-------------|\n")
		for _, doc := range docs {
			examples := "No"
			if doc.HasExamples {
				examples = "Yes"
			}
			sb.WriteString(fmt.Sprintf("| %s | %d | %s |\n", doc.RelPath, doc.LineCount, examples))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
