package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/pkg/blueprint/registry"
)

var versionsCmd = &cobra.Command{
	Use:   "versions <blueprint>",
	Short: "List available versions",
	Long: `List all available versions of a blueprint.

Shows version history with release dates and deprecation status.

Examples:
  # List versions
  kscorectl blueprint-publish versions community/nginx

  # List all versions including prereleases
  kscorectl blueprint-publish versions community/nginx --all

  # Output as JSON
  kscorectl blueprint-publish versions community/nginx --json`,
	Args: cobra.ExactArgs(1),
	RunE: versionsExecute,
}

var (
	versionsRegistry string
	versionsAll      bool
	versionsLimit    int
	versionsJSON     bool
)

func init() {
	versionsCmd.Flags().StringVar(&versionsRegistry, "registry", "", "Registry URL")
	versionsCmd.Flags().BoolVar(&versionsAll, "all", false, "Include prerelease versions")
	versionsCmd.Flags().IntVar(&versionsLimit, "limit", 20, "Maximum versions to show")
	versionsCmd.Flags().BoolVar(&versionsJSON, "json", false, "Output in JSON format")
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

	// Apply limit
	if versionsLimit > 0 && len(versions) > versionsLimit {
		versions = versions[:versionsLimit]
	}

	if versionsJSON {
		data, _ := json.MarshalIndent(map[string]interface{}{
			"name":     name,
			"versions": versions,
		}, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Versions of %s:\n\n", name)
	for i, v := range versions {
		marker := " "
		if i == 0 {
			marker = "*" // Latest
		}
		fmt.Printf("  %s %s\n", marker, v)
	}
	fmt.Println()
	fmt.Println("  * = latest")

	return nil
}
