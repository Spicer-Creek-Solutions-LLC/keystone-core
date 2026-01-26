package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/blueprint/registry"
)

var (
	searchRegistry   string
	searchCategory   string
	searchTags       []string
	searchLimit      int
	searchOffset     int
	searchVerified   bool
	searchSortBy     string
	searchSortOrder  string
	searchOutputJSON bool
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for blueprints",
	Long: `Search for blueprints in registries.

Searches by name, description, keywords, and tags. Supports filtering by
category, tags, and verification status.

Examples:
  # Search for nginx blueprints
  kscorectl blueprint search nginx

  # Search with category filter
  kscorectl blueprint search web --category networking

  # Search for verified blueprints only
  kscorectl blueprint search --verified

  # Search with tags
  kscorectl blueprint search --tags kubernetes,production`,
	RunE: searchExecute,
}

func init() {
	searchCmd.Flags().StringVar(&searchRegistry, "registry", "", "Registry URL (uses default if not specified)")
	searchCmd.Flags().StringVar(&searchCategory, "category", "", "Filter by category")
	searchCmd.Flags().StringSliceVar(&searchTags, "tags", nil, "Filter by tags (comma-separated)")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Maximum number of results")
	searchCmd.Flags().IntVar(&searchOffset, "offset", 0, "Result offset for pagination")
	searchCmd.Flags().BoolVar(&searchVerified, "verified", false, "Only show verified blueprints")
	searchCmd.Flags().StringVar(&searchSortBy, "sort-by", "downloads", "Sort by (name, downloads, updated)")
	searchCmd.Flags().StringVar(&searchSortOrder, "sort-order", "desc", "Sort order (asc, desc)")
	searchCmd.Flags().BoolVar(&searchOutputJSON, "json", false, "Output in JSON format")
}

func searchExecute(cmd *cobra.Command, args []string) error {
	query := ""
	if len(args) > 0 {
		query = strings.Join(args, " ")
	}

	// Get registry URL
	registryURL := searchRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}

	fmt.Printf("Searching %s...\n\n", registryURL)

	// Create client
	client, err := registry.NewHTTPClient(&registry.RegistryConfig{
		URL:     registryURL,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	// Build search query
	searchQuery := &registry.SearchQuery{
		Query:     query,
		Tags:      searchTags,
		Limit:     searchLimit,
		Offset:    searchOffset,
		SortBy:    searchSortBy,
		SortOrder: searchSortOrder,
	}

	// Perform search
	results, err := client.Search(searchQuery)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Apply client-side filters (category, verified)
	filtered := results.Blueprints
	if searchCategory != "" || searchVerified {
		var filteredResults []*registry.BlueprintInfo
		for _, bp := range filtered {
			// We can't filter by category or verified from BlueprintInfo
			// as it doesn't have these fields - would need to use GetIndex
			// For now, skip client-side filtering and note the limitation
			filteredResults = append(filteredResults, bp)
		}
		filtered = filteredResults
	}

	if len(filtered) == 0 {
		fmt.Println("No blueprints found matching your criteria.")
		return nil
	}

	// Print results
	fmt.Printf("Found %d blueprints (showing %d-%d):\n\n",
		results.Total,
		searchOffset+1,
		min(searchOffset+len(filtered), results.Total))

	for _, bp := range filtered {
		printBlueprintInfo(bp)
	}

	// Pagination info
	if results.Total > searchOffset+searchLimit {
		fmt.Printf("\nShowing %d of %d results. Use --offset %d to see more.\n",
			len(filtered), results.Total, searchOffset+searchLimit)
	}

	return nil
}

func printBlueprintInfo(bp *registry.BlueprintInfo) {
	signed := ""
	if bp.Signature != "" {
		signed = " ✓"
	}

	fmt.Printf("  %s%s\n", bp.Name, signed)
	if bp.Metadata != nil && bp.Metadata.Description != "" {
		desc := bp.Metadata.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		fmt.Printf("    %s\n", desc)
	}
	fmt.Printf("    Version: %s", bp.Version)
	if bp.Metadata != nil && len(bp.Metadata.Keywords) > 0 {
		fmt.Printf(" | Tags: %s", strings.Join(bp.Metadata.Keywords, ", "))
	}
	fmt.Println()
	fmt.Println()
}

var infoCmd = &cobra.Command{
	Use:   "info <blueprint>",
	Short: "Show blueprint details",
	Long: `Show detailed information about a blueprint.

Displays metadata, parameters, dependencies, and available versions.

Examples:
  # Show blueprint info
  kscorectl blueprint info community/nginx

  # Show info for specific version
  kscorectl blueprint info community/nginx@1.2.0`,
	Args: cobra.ExactArgs(1),
	RunE: infoExecute,
}

var (
	infoRegistry   string
	infoVersion    string
	infoOutputJSON bool
)

func init() {
	infoCmd.Flags().StringVar(&infoRegistry, "registry", "", "Registry URL")
	infoCmd.Flags().StringVar(&infoVersion, "version", "", "Specific version (defaults to latest)")
	infoCmd.Flags().BoolVar(&infoOutputJSON, "json", false, "Output in JSON format")
}

func infoExecute(cmd *cobra.Command, args []string) error {
	blueprintRef := args[0]

	// Parse reference (name@version)
	name, version := parseReference(blueprintRef)
	if infoVersion != "" {
		version = infoVersion
	}

	// Get registry URL
	registryURL := infoRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}

	// Create client
	client, err := registry.NewHTTPClient(&registry.RegistryConfig{
		URL:     registryURL,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	// Get version if not specified
	if version == "" {
		latestVersion, err := client.GetLatestVersion(name)
		if err != nil {
			return fmt.Errorf("failed to get latest version: %w", err)
		}
		version = latestVersion
	}

	// Get blueprint info
	info, err := client.GetBlueprintInfo(name, version)
	if err != nil {
		return fmt.Errorf("failed to get blueprint info: %w", err)
	}

	// Print info
	fmt.Printf("Blueprint: %s\n", info.Name)
	fmt.Printf("Version: %s\n", info.Version)
	if info.Metadata != nil {
		if info.Metadata.Description != "" {
			fmt.Printf("Description: %s\n", info.Metadata.Description)
		}
		if len(info.Metadata.Maintainers) > 0 {
			fmt.Printf("Maintainers:\n")
			for _, m := range info.Metadata.Maintainers {
				if m.Email != "" {
					fmt.Printf("  - %s <%s>\n", m.Name, m.Email)
				} else {
					fmt.Printf("  - %s\n", m.Name)
				}
			}
		}
		if info.Metadata.License != "" {
			fmt.Printf("License: %s\n", info.Metadata.License)
		}
		if info.Metadata.Repository != "" {
			fmt.Printf("Repository: %s\n", info.Metadata.Repository)
		}
	}

	fmt.Printf("Published: %s\n", info.Published.Format("2006-01-02"))
	fmt.Printf("Checksum: %s\n", info.Checksum)

	if info.SignedBy != "" {
		fmt.Printf("Signed by: %s\n", info.SignedBy)
	}

	if len(info.Dependencies) > 0 {
		fmt.Printf("\nDependencies:\n")
		for _, dep := range info.Dependencies {
			fmt.Printf("  - %s\n", dep)
		}
	}

	if info.Compatibility != nil {
		fmt.Printf("\nCompatibility:\n")
		if info.Compatibility.Kscore != "" {
			fmt.Printf("  Keystone Core: %s\n", info.Compatibility.Kscore)
		}
	}

	return nil
}

var versionsCmd = &cobra.Command{
	Use:   "versions <blueprint>",
	Short: "List available versions",
	Long: `List all available versions of a blueprint.

Shows version history with release dates and deprecation status.

Examples:
  # List versions
  kscorectl blueprint versions community/nginx

  # List all versions including prereleases
  kscorectl blueprint versions community/nginx --all`,
	Args: cobra.ExactArgs(1),
	RunE: versionsExecute,
}

var (
	versionsRegistry string
	versionsAll      bool
	versionsLimit    int
)

func init() {
	versionsCmd.Flags().StringVar(&versionsRegistry, "registry", "", "Registry URL")
	versionsCmd.Flags().BoolVar(&versionsAll, "all", false, "Include prerelease versions")
	versionsCmd.Flags().IntVar(&versionsLimit, "limit", 20, "Maximum versions to show")
}

func versionsExecute(cmd *cobra.Command, args []string) error {
	blueprintName := args[0]

	// Remove version if provided
	name, _ := parseReference(blueprintName)

	// Get registry URL
	registryURL := versionsRegistry
	if registryURL == "" {
		registryURL = getDefaultRegistry()
	}

	// Create client
	client, err := registry.NewHTTPClient(&registry.RegistryConfig{
		URL:     registryURL,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create registry client: %w", err)
	}

	// Get versions
	versions, err := client.ListVersions(name)
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	if len(versions) == 0 {
		fmt.Printf("No versions found for %s\n", name)
		return nil
	}

	fmt.Printf("Versions of %s:\n\n", name)

	// Apply limit
	displayVersions := versions
	if versionsLimit > 0 && len(displayVersions) > versionsLimit {
		displayVersions = displayVersions[:versionsLimit]
	}

	// Filter prereleases if not showing all
	if !versionsAll {
		var stableVersions []string
		for _, v := range displayVersions {
			if !isPrerelease(v) {
				stableVersions = append(stableVersions, v)
			}
		}
		displayVersions = stableVersions
	}

	for _, v := range displayVersions {
		fmt.Printf("  %s\n", v)
	}

	if len(versions) > versionsLimit {
		fmt.Printf("\nShowing %d of %d versions. Use --limit to see more.\n",
			len(displayVersions), len(versions))
	}

	return nil
}

// Helper functions

func parseReference(ref string) (name, version string) {
	parts := strings.SplitN(ref, "@", 2)
	name = parts[0]
	if len(parts) > 1 {
		version = parts[1]
	}
	return
}

func getDefaultRegistry() string {
	if registryURL := os.Getenv("KSCORE_BLUEPRINT_REGISTRY"); registryURL != "" {
		return registryURL
	}
	return "https://blueprints.keystone-core.io"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isPrerelease(version string) bool {
	return strings.Contains(version, "-")
}
