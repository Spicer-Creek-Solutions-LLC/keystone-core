// Package main implements cache management commands for kscore-files.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shawnbutts/keystone-core/internal/cli/output"
)

// newCacheCmd creates the cache command group.
func newCacheCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage file caches",
		Long:  `Commands for managing file caches (status, clear, warm, invalidate, verify, show, refresh, set-ttl, history).`,
	}

	cmd.AddCommand(newCacheStatusCmd())
	cmd.AddCommand(newCacheClearCmd())
	cmd.AddCommand(newCacheWarmCmd())
	cmd.AddCommand(newCacheListCmd())
	cmd.AddCommand(newCacheEvictCmd())
	cmd.AddCommand(newCacheStatsCmd())
	cmd.AddCommand(newCacheInvalidateCmd())
	cmd.AddCommand(newCacheVerifyCmd())
	cmd.AddCommand(newCacheShowCmd())
	cmd.AddCommand(newCacheRefreshCmd())
	cmd.AddCommand(newCacheSetTTLCmd())
	cmd.AddCommand(newCacheHistoryCmd())

	return cmd
}

// CacheStatus contains cache status information.
type CacheStatus struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Enabled     bool          `json:"enabled"`
	Size        int64         `json:"size"`
	MaxSize     int64         `json:"max_size"`
	Entries     int64         `json:"entries"`
	MaxEntries  int64         `json:"max_entries"`
	HitRate     float64       `json:"hit_rate"`
	Hits        int64         `json:"hits"`
	Misses      int64         `json:"misses"`
	Evictions   int64         `json:"evictions"`
	TTL         time.Duration `json:"ttl"`
	LastCleared time.Time     `json:"last_cleared,omitempty"`
}

// CacheEntry contains information about a cached file.
type CacheEntry struct {
	Key         string    `json:"key"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	Checksum    string    `json:"checksum"`
	CachedAt    time.Time `json:"cached_at"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	LastAccess  time.Time `json:"last_access"`
	AccessCount int64     `json:"access_count"`
}

// CacheItemDetail contains detailed information about a specific cached item.
type CacheItemDetail struct {
	Key         string            `json:"key"`
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"`
	Algorithm   string            `json:"algorithm"`
	CachedAt    time.Time         `json:"cached_at"`
	ExpiresAt   time.Time         `json:"expires_at,omitempty"`
	LastAccess  time.Time         `json:"last_access"`
	AccessCount int64             `json:"access_count"`
	TTL         time.Duration     `json:"ttl"`
	Source      string            `json:"source"`
	Namespace   string            `json:"namespace,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CacheInvalidateResult contains the result of a cache invalidation operation.
type CacheInvalidateResult struct {
	Invalidated int64 `json:"invalidated"`
	Errors      int64 `json:"errors"`
}

// CacheVerifyResult contains the result of a cache verification.
type CacheVerifyResult struct {
	TotalEntries   int64            `json:"total_entries"`
	ValidEntries   int64            `json:"valid_entries"`
	CorruptEntries int64            `json:"corrupt_entries"`
	MissingEntries int64            `json:"missing_entries"`
	Details        []CacheVerifyErr `json:"details,omitempty"`
}

// CacheVerifyErr describes a single verification failure.
type CacheVerifyErr struct {
	Key    string `json:"key"`
	Reason string `json:"reason"`
}

// CacheRefreshResult contains the result of a cache refresh operation.
type CacheRefreshResult struct {
	Refreshed int64         `json:"refreshed"`
	Errors    int64         `json:"errors"`
	Duration  time.Duration `json:"duration"`
}

// CacheHistoryEntry represents a single cache operation log entry.
type CacheHistoryEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Operation string    `json:"operation"`
	Key       string    `json:"key,omitempty"`
	Result    string    `json:"result"`
	Details   string    `json:"details,omitempty"`
}

// CacheStats contains detailed cache statistics.
type CacheStats struct {
	TotalHits       int64         `json:"total_hits"`
	TotalMisses     int64         `json:"total_misses"`
	HitRate         float64       `json:"hit_rate"`
	TotalEvictions  int64         `json:"total_evictions"`
	BytesServed     int64         `json:"bytes_served"`
	AverageLatency  time.Duration `json:"average_latency"`
	CurrentSize     int64         `json:"current_size"`
	CurrentEntries  int64         `json:"current_entries"`
	OldestEntry     time.Time     `json:"oldest_entry,omitempty"`
	NewestEntry     time.Time     `json:"newest_entry,omitempty"`
	TopAccessedKeys []string      `json:"top_accessed_keys,omitempty"`
}

// newCacheStatusCmd creates the status command.
func newCacheStatusCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "status [name]",
		Short: "Get cache status",
		Long: `Get status information for a cache or all caches.

Examples:
  kscore-files cache status
  kscore-files cache status agent-cache
  kscore-files cache status --output json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			var caches []CacheStatus

			if len(args) == 0 {
				// Get all caches
				var err error
				caches, err = getCacheStatuses(ctx, client)
				if err != nil {
					return err
				}
			} else {
				// Get specific cache
				status, err := getCacheStatus(ctx, client, args[0])
				if err != nil {
					return err
				}
				caches = []CacheStatus{*status}
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, caches)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, caches)
			case output.FormatTable:
				if len(caches) == 0 {
					fmt.Println("No caches configured")
					return nil
				}
				rows := make([][]string, 0, len(caches))
				for i := range caches {
					cache := &caches[i]
					rows = append(rows, []string{
						cache.Name,
						cache.Type,
						fmt.Sprintf("%t", cache.Enabled),
						fmt.Sprintf("%s / %s", formatSize(cache.Size), formatSize(cache.MaxSize)),
						fmt.Sprintf("%d / %d", cache.Entries, cache.MaxEntries),
						fmt.Sprintf("%.2f%%", cache.HitRate*100),
					})
				}
				table := &output.Table{
					Headers: []string{"NAME", "TYPE", "ENABLED", "SIZE", "ENTRIES", "HIT RATE"},
					Rows:    rows,
				}
				return output.WriteTable(os.Stdout, table)
			case output.FormatText:
				for i := range caches {
					cache := &caches[i]
					fmt.Printf("Cache: %s\n", cache.Name)
					fmt.Printf("Type: %s\n", cache.Type)
					fmt.Printf("Enabled: %v\n", cache.Enabled)
					fmt.Printf("Size: %s / %s (%.1f%%)\n",
						formatSize(cache.Size),
						formatSize(cache.MaxSize),
						float64(cache.Size)/float64(cache.MaxSize)*100)
					fmt.Printf("Entries: %d / %d\n", cache.Entries, cache.MaxEntries)
					fmt.Printf("Hit Rate: %.2f%%\n", cache.HitRate*100)
					fmt.Printf("Hits/Misses: %d / %d\n", cache.Hits, cache.Misses)
					fmt.Printf("Evictions: %d\n", cache.Evictions)
					fmt.Printf("TTL: %s\n", cache.TTL)
					if !cache.LastCleared.IsZero() {
						fmt.Printf("Last Cleared: %s\n", cache.LastCleared.Format(time.RFC3339))
					}
					fmt.Println()
				}
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")

	return cmd
}

// newCacheClearCmd creates the clear command.
func newCacheClearCmd() *cobra.Command {
	var (
		pattern string
		force   bool
		older   string
		dryRun  bool
	)

	cmd := &cobra.Command{
		Use:   "clear [name]",
		Short: "Clear cache contents",
		Long: `Clear all or part of a cache.

Examples:
  kscore-files cache clear agent-cache
  kscore-files cache clear --pattern "/packages/*"
  kscore-files cache clear --older 7d
  kscore-files cache clear agent-cache --force`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheName := ""
			if len(args) > 0 {
				cacheName = args[0]
			}

			if dryRun {
				fmt.Printf("Dry run: would clear cache %s\n", cacheName)
				if pattern != "" {
					fmt.Printf("  Pattern: %s\n", pattern)
				}
				if older != "" {
					fmt.Printf("  Older than: %s\n", older)
				}
				return nil
			}

			if !force {
				msg := "Clear"
				if cacheName != "" {
					msg += " cache " + cacheName
				} else {
					msg += " all caches"
				}
				if pattern != "" {
					msg += " (pattern: " + pattern + ")"
				}
				if older != "" {
					msg += " (older than " + older + ")"
				}
				msg += "? [y/N]: "

				fmt.Print(msg)
				var confirm string
				fmt.Scanln(&confirm)
				if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
					fmt.Println("Cancelled")
					return nil
				}
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdminLong)
			defer cancel()

			// Parse older duration
			var olderThan time.Duration
			if older != "" {
				var err error
				olderThan, err = parseDuration(older)
				if err != nil {
					return fmt.Errorf("invalid duration: %w", err)
				}
			}

			result, err := clearCache(ctx, client, cacheName, pattern, olderThan)
			if err != nil {
				return err
			}

			fmt.Printf("Cleared %d entries (%s)\n", result.EntriesCleared, formatSize(result.BytesCleared))
			return nil
		},
	}

	cmd.Flags().StringVar(&pattern, "pattern", "", "Clear only entries matching pattern")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Don't prompt for confirmation")
	cmd.Flags().StringVar(&older, "older", "", "Clear entries older than duration (e.g., 7d, 24h)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be cleared without clearing")

	return cmd
}

// newCacheWarmCmd creates the warm command.
func newCacheWarmCmd() *cobra.Command {
	var (
		namespace string
		recursive bool
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "warm <path>",
		Short: "Warm the cache with files",
		Long: `Pre-populate the cache with files from a path.

Examples:
  kscore-files cache warm /packages/
  kscore-files cache warm /configs/ --recursive
  kscore-files cache warm /critical/ --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdminSync)
			defer cancel()

			result, err := warmCache(ctx, client, path, namespace, recursive, dryRun)
			if err != nil {
				return err
			}

			if dryRun {
				fmt.Println("Dry run - would warm the following:")
			}

			fmt.Printf("Files: %d\n", result.FilesWarmed)
			fmt.Printf("Size: %s\n", formatSize(result.BytesWarmed))
			if result.Errors > 0 {
				fmt.Printf("Errors: %d\n", result.Errors)
			}
			if !dryRun {
				fmt.Printf("Duration: %s\n", result.Duration)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Warm recursively")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be warmed")

	return cmd
}

// newCacheListCmd creates the list command.
func newCacheListCmd() *cobra.Command {
	var (
		pattern   string
		limit     int
		sortBy    string
		outputFmt string
	)

	cmd := &cobra.Command{
		Use:   "list [name]",
		Short: "List cached entries",
		Long: `List entries in a cache.

Examples:
  kscore-files cache list agent-cache
  kscore-files cache list --pattern "/packages/*"
  kscore-files cache list --sort-by size --limit 10`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheName := ""
			if len(args) > 0 {
				cacheName = args[0]
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			entries, err := listCacheEntries(ctx, client, cacheName, pattern, sortBy, limit)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, entries)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, entries)
			case output.FormatTable, output.FormatText:
				if len(entries) == 0 {
					fmt.Println("No cached entries found")
					return nil
				}

				rows := make([][]string, 0, len(entries))
				for i := range entries {
					entry := &entries[i]
					rows = append(rows, []string{
						formatSize(entry.Size),
						fmt.Sprintf("%d", entry.AccessCount),
						entry.CachedAt.Format("2006-01-02 15:04:05"),
						entry.LastAccess.Format("2006-01-02 15:04:05"),
						entry.Path,
					})
				}
				table := &output.Table{
					Headers: []string{"SIZE", "ACCESSES", "CACHED", "LAST ACCESS", "PATH"},
					Rows:    rows,
				}
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				fmt.Printf("\nTotal: %d entries\n", len(entries))
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVar(&pattern, "pattern", "", "Filter by pattern")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum entries to show")
	cmd.Flags().StringVar(&sortBy, "sort-by", "last_access", "Sort by (size, last_access, cached_at, access_count)")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")

	return cmd
}

// newCacheEvictCmd creates the evict command.
func newCacheEvictCmd() *cobra.Command {
	var (
		force  bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "evict <path>",
		Short: "Evict an entry from cache",
		Long: `Evict a specific entry or pattern from the cache.

Examples:
  kscore-files cache evict /packages/nginx-1.20.rpm
  kscore-files cache evict "/packages/old-*" --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			if dryRun {
				fmt.Printf("Dry run: would evict %s from cache\n", path)
				return nil
			}

			if !force {
				fmt.Printf("Evict %s from cache? [y/N]: ", path)
				var confirm string
				fmt.Scanln(&confirm)
				if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
					fmt.Println("Cancelled")
					return nil
				}
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			evicted, err := evictFromCache(ctx, client, path)
			if err != nil {
				return err
			}

			fmt.Printf("Evicted %d entries\n", evicted)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Don't prompt for confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be evicted without evicting")

	return cmd
}

// newCacheStatsCmd creates the stats command.
func newCacheStatsCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "stats [name]",
		Short: "Get detailed cache statistics",
		Long: `Get detailed statistics for a cache.

Examples:
  kscore-files cache stats
  kscore-files cache stats agent-cache --output json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheName := ""
			if len(args) > 0 {
				cacheName = args[0]
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			stats, err := getCacheStats(ctx, client, cacheName)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, stats)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, stats)
			case output.FormatTable:
				table := buildKeyValueTable([][2]string{
					{"HIT RATE", fmt.Sprintf("%.2f%%", stats.HitRate*100)},
					{"TOTAL HITS", fmt.Sprintf("%d", stats.TotalHits)},
					{"TOTAL MISSES", fmt.Sprintf("%d", stats.TotalMisses)},
					{"TOTAL EVICTIONS", fmt.Sprintf("%d", stats.TotalEvictions)},
					{"BYTES SERVED", formatSize(stats.BytesServed)},
					{"AVG LATENCY", stats.AverageLatency.String()},
					{"CURRENT SIZE", formatSize(stats.CurrentSize)},
					{"CURRENT ENTRIES", fmt.Sprintf("%d", stats.CurrentEntries)},
					{"OLDEST ENTRY", formatTime(stats.OldestEntry)},
					{"NEWEST ENTRY", formatTime(stats.NewestEntry)},
				})
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				if len(stats.TopAccessedKeys) > 0 {
					rows := make([][]string, 0, len(stats.TopAccessedKeys))
					for i, key := range stats.TopAccessedKeys {
						rows = append(rows, []string{fmt.Sprintf("%d", i+1), key})
					}
					fmt.Println("\nTop Accessed:")
					return output.WriteTable(os.Stdout, &output.Table{
						Headers: []string{"RANK", "KEY"},
						Rows:    rows,
					})
				}
				return nil
			case output.FormatText:
				fmt.Printf("Cache Statistics\n")
				fmt.Println(strings.Repeat("-", 40))
				fmt.Printf("Hit Rate:        %.2f%%\n", stats.HitRate*100)
				fmt.Printf("Total Hits:      %d\n", stats.TotalHits)
				fmt.Printf("Total Misses:    %d\n", stats.TotalMisses)
				fmt.Printf("Total Evictions: %d\n", stats.TotalEvictions)
				fmt.Printf("Bytes Served:    %s\n", formatSize(stats.BytesServed))
				fmt.Printf("Avg Latency:     %s\n", stats.AverageLatency)
				fmt.Printf("Current Size:    %s\n", formatSize(stats.CurrentSize))
				fmt.Printf("Current Entries: %d\n", stats.CurrentEntries)
				if !stats.OldestEntry.IsZero() {
					fmt.Printf("Oldest Entry:    %s\n", stats.OldestEntry.Format(time.RFC3339))
				}
				if !stats.NewestEntry.IsZero() {
					fmt.Printf("Newest Entry:    %s\n", stats.NewestEntry.Format(time.RFC3339))
				}
				if len(stats.TopAccessedKeys) > 0 {
					fmt.Println("\nTop Accessed:")
					for i, key := range stats.TopAccessedKeys {
						fmt.Printf("  %d. %s\n", i+1, key)
					}
				}
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")

	return cmd
}

// newCacheInvalidateCmd creates the invalidate command.
func newCacheInvalidateCmd() *cobra.Command {
	var (
		target   string
		priority string
		force    bool
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "invalidate <path-or-pattern>",
		Short: "Invalidate specific cached items",
		Long: `Invalidate cached items by path or glob pattern.

Invalidated entries are marked stale and will be re-fetched on next access.

Examples:
  kscore-files cache invalidate /states/nginx-config
  kscore-files cache invalidate "states/web-*"
  kscore-files cache invalidate "policies/*" --target "*" --priority high
  kscore-files cache invalidate "/packages/old-*" --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := args[0]

			if dryRun {
				fmt.Printf("Dry run: would invalidate entries matching %s\n", pattern)
				if target != "" {
					fmt.Printf("  Target: %s\n", target)
				}
				return nil
			}

			if !force {
				fmt.Printf("Invalidate entries matching %s? [y/N]: ", pattern)
				var confirm string
				fmt.Scanln(&confirm)
				if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
					fmt.Println("Cancelled")
					return nil
				}
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdminLong)
			defer cancel()

			result, err := invalidateCache(ctx, client, pattern, target, priority)
			if err != nil {
				return err
			}

			fmt.Printf("Invalidated %d entries\n", result.Invalidated)
			if result.Errors > 0 {
				fmt.Printf("Errors: %d\n", result.Errors)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&target, "target", "", "Target agents for invalidation (e.g., 'region=us-west', '*')")
	cmd.Flags().StringVar(&priority, "priority", "normal", "Invalidation priority (normal, high)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Don't prompt for confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be invalidated without invalidating")

	return cmd
}

// newCacheVerifyCmd creates the verify command.
func newCacheVerifyCmd() *cobra.Command {
	var (
		fix       bool
		outputFmt string
	)

	cmd := &cobra.Command{
		Use:   "verify [name]",
		Short: "Verify cache integrity",
		Long: `Verify cache integrity by checking checksums and detecting corruption.

Examples:
  kscore-files cache verify
  kscore-files cache verify agent-cache
  kscore-files cache verify --fix
  kscore-files cache verify --output json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheName := ""
			if len(args) > 0 {
				cacheName = args[0]
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdminLong)
			defer cancel()

			result, err := verifyCache(ctx, client, cacheName, fix)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, result)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, result)
			case output.FormatTable:
				table := buildKeyValueTable([][2]string{
					{"TOTAL ENTRIES", fmt.Sprintf("%d", result.TotalEntries)},
					{"VALID", fmt.Sprintf("%d", result.ValidEntries)},
					{"CORRUPT", fmt.Sprintf("%d", result.CorruptEntries)},
					{"MISSING", fmt.Sprintf("%d", result.MissingEntries)},
				})
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				if len(result.Details) > 0 {
					fmt.Println("\nIssues:")
					rows := make([][]string, 0, len(result.Details))
					for _, d := range result.Details {
						rows = append(rows, []string{d.Key, d.Reason})
					}
					return output.WriteTable(os.Stdout, &output.Table{
						Headers: []string{"KEY", "REASON"},
						Rows:    rows,
					})
				}
				return nil
			case output.FormatText:
				fmt.Println("Verifying cache entries...")
				fmt.Printf("Total entries: %d\n", result.TotalEntries)
				fmt.Printf("Valid:         %d\n", result.ValidEntries)
				fmt.Printf("Corrupt:       %d\n", result.CorruptEntries)
				fmt.Printf("Missing:       %d\n", result.MissingEntries)
				if len(result.Details) > 0 {
					fmt.Println("\nIssues found:")
					for _, d := range result.Details {
						fmt.Printf("  %s: %s\n", d.Key, d.Reason)
					}
				}
				if result.CorruptEntries == 0 && result.MissingEntries == 0 {
					fmt.Println("\nCache integrity OK")
				}
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().BoolVar(&fix, "fix", false, "Remove corrupt or missing entries")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")

	return cmd
}

// newCacheShowCmd creates the show command.
func newCacheShowCmd() *cobra.Command {
	var outputFmt string

	cmd := &cobra.Command{
		Use:   "show <key>",
		Short: "Show details about a cached item",
		Long: `Show detailed information about a specific cached item including
size, checksum, TTL, access count, and metadata.

Examples:
  kscore-files cache show states/nginx-config
  kscore-files cache show packages/nginx-1.20.rpm --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			item, err := getCacheItem(ctx, client, key)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, item)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, item)
			case output.FormatTable:
				pairs := [][2]string{
					{"KEY", item.Key},
					{"PATH", item.Path},
					{"SIZE", formatSize(item.Size)},
					{"CHECKSUM", item.Checksum},
					{"ALGORITHM", item.Algorithm},
					{"CACHED AT", formatTime(item.CachedAt)},
					{"EXPIRES AT", formatTime(item.ExpiresAt)},
					{"LAST ACCESS", formatTime(item.LastAccess)},
					{"ACCESS COUNT", fmt.Sprintf("%d", item.AccessCount)},
					{"TTL", item.TTL.String()},
					{"SOURCE", item.Source},
				}
				if item.Namespace != "" {
					pairs = append(pairs, [2]string{"NAMESPACE", item.Namespace})
				}
				table := buildKeyValueTable(pairs)
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				if len(item.Metadata) > 0 {
					fmt.Println("\nMetadata:")
					rows := make([][]string, 0, len(item.Metadata))
					for k, v := range item.Metadata {
						rows = append(rows, []string{k, v})
					}
					return output.WriteTable(os.Stdout, &output.Table{
						Headers: []string{"KEY", "VALUE"},
						Rows:    rows,
					})
				}
				return nil
			case output.FormatText:
				fmt.Printf("ID: %s\n", item.Key)
				fmt.Printf("Path: %s\n", item.Path)
				fmt.Printf("Size: %s\n", formatSize(item.Size))
				fmt.Printf("Checksum: %s (%s)\n", item.Checksum, item.Algorithm)
				fmt.Printf("Cached At: %s\n", formatTime(item.CachedAt))
				if !item.ExpiresAt.IsZero() {
					fmt.Printf("Expires At: %s\n", formatTime(item.ExpiresAt))
				}
				fmt.Printf("Last Access: %s\n", formatTime(item.LastAccess))
				fmt.Printf("Access Count: %d\n", item.AccessCount)
				fmt.Printf("TTL: %s\n", item.TTL)
				fmt.Printf("Source: %s\n", item.Source)
				if item.Namespace != "" {
					fmt.Printf("Namespace: %s\n", item.Namespace)
				}
				if len(item.Metadata) > 0 {
					fmt.Println("Metadata:")
					for k, v := range item.Metadata {
						fmt.Printf("  %s: %s\n", k, v)
					}
				}
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVarP(&outputFmt, "output", "o", "text", "Output format (text, json, yaml, table)")

	return cmd
}

// newCacheRefreshCmd creates the refresh command.
func newCacheRefreshCmd() *cobra.Command {
	var (
		force  bool
		dryRun bool
	)

	cmd := &cobra.Command{
		Use:   "refresh <path-or-pattern>",
		Short: "Refresh cached items from source",
		Long: `Refresh cached items by re-fetching them from the original source.

This re-downloads and updates cached entries while preserving cache keys.

Examples:
  kscore-files cache refresh states/nginx-config
  kscore-files cache refresh "states/*" --force
  kscore-files cache refresh "/packages/*" --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := args[0]

			if dryRun {
				fmt.Printf("Dry run: would refresh entries matching %s\n", pattern)
				return nil
			}

			if !force {
				fmt.Printf("Refresh entries matching %s? [y/N]: ", pattern)
				var confirm string
				fmt.Scanln(&confirm)
				if !strings.EqualFold(confirm, "y") && !strings.EqualFold(confirm, "yes") {
					fmt.Println("Cancelled")
					return nil
				}
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdminSync)
			defer cancel()

			result, err := refreshCache(ctx, client, pattern, force)
			if err != nil {
				return err
			}

			fmt.Printf("Refreshed %d entries\n", result.Refreshed)
			if result.Errors > 0 {
				fmt.Printf("Errors: %d\n", result.Errors)
			}
			fmt.Printf("Duration: %s\n", result.Duration)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Refresh even if entries are not expired")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be refreshed without refreshing")

	return cmd
}

// newCacheSetTTLCmd creates the set-ttl command.
func newCacheSetTTLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-ttl <key> <duration>",
		Short: "Set TTL for cached items",
		Long: `Set the time-to-live for a specific cached item or pattern.

Duration accepts Go duration strings (e.g., 1h, 30m, 24h) and day suffixes (e.g., 7d).

Examples:
  kscore-files cache set-ttl states/nginx-config 6h
  kscore-files cache set-ttl "packages/*" 7d
  kscore-files cache set-ttl policies/security 30m`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			durationStr := args[1]

			ttl, err := parseDuration(durationStr)
			if err != nil {
				return fmt.Errorf("invalid duration %q: %w", durationStr, err)
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			updated, err := setCacheTTL(ctx, client, key, ttl)
			if err != nil {
				return err
			}

			fmt.Printf("Updated TTL for %d entries to %s\n", updated, ttl)
			return nil
		},
	}

	return cmd
}

// newCacheHistoryCmd creates the history command.
func newCacheHistoryCmd() *cobra.Command {
	var (
		opType    string
		limit     int
		outputFmt string
	)

	cmd := &cobra.Command{
		Use:   "history [name]",
		Short: "Show cache operation history",
		Long: `Show the history of cache operations including hits, misses, evictions,
invalidations, and refreshes.

Examples:
  kscore-files cache history
  kscore-files cache history agent-cache
  kscore-files cache history --type invalidation --limit 50
  kscore-files cache history --type eviction --output json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheName := ""
			if len(args) > 0 {
				cacheName = args[0]
			}

			client, cleanup, err := createAdminClient()
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), filesTimeoutAdmin)
			defer cancel()

			entries, err := getCacheHistory(ctx, client, cacheName, opType, limit)
			if err != nil {
				return err
			}

			format, err := output.ParseFormat(outputFmt)
			if err != nil {
				return err
			}

			switch format {
			case output.FormatJSON:
				return output.WriteJSON(os.Stdout, entries)
			case output.FormatYAML:
				return output.WriteYAML(os.Stdout, entries)
			case output.FormatTable, output.FormatText:
				if len(entries) == 0 {
					fmt.Println("No cache history entries found")
					return nil
				}
				rows := make([][]string, 0, len(entries))
				for i := range entries {
					entry := &entries[i]
					rows = append(rows, []string{
						entry.Timestamp.Format("2006-01-02 15:04:05"),
						entry.Operation,
						entry.Key,
						entry.Result,
						entry.Details,
					})
				}
				table := &output.Table{
					Headers: []string{"TIMESTAMP", "OPERATION", "KEY", "RESULT", "DETAILS"},
					Rows:    rows,
				}
				if err := output.WriteTable(os.Stdout, table); err != nil {
					return err
				}
				fmt.Printf("\nTotal: %d entries\n", len(entries))
				return nil
			default:
				return fmt.Errorf("unsupported output format: %s", outputFmt)
			}
		},
	}

	cmd.Flags().StringVar(&opType, "type", "", "Filter by operation type (hit, miss, eviction, invalidation, refresh)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum entries to show")
	cmd.Flags().StringVarP(&outputFmt, "output", "o", "table", "Output format (table, text, json, yaml)")

	return cmd
}

// Helper functions

type CacheClearResult struct {
	EntriesCleared int64 `json:"entries_cleared"`
	BytesCleared   int64 `json:"bytes_cleared"`
}

type CacheWarmResult struct {
	FilesWarmed int64         `json:"files_warmed"`
	BytesWarmed int64         `json:"bytes_warmed"`
	Errors      int64         `json:"errors"`
	Duration    time.Duration `json:"duration"`
}

func getCacheStatuses(ctx context.Context, client *AdminClient) ([]CacheStatus, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.list", client.clusterID)

	msg, err := client.nc.RequestWithContext(ctx, subject, nil)
	if err != nil {
		return nil, err
	}

	var caches []CacheStatus
	if err := json.Unmarshal(msg.Data, &caches); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return caches, nil
}

func getCacheStatus(ctx context.Context, client *AdminClient, name string) (*CacheStatus, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.status", client.clusterID)

	req := map[string]string{"name": name}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var status CacheStatus
	if err := json.Unmarshal(msg.Data, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &status, nil
}

func clearCache(ctx context.Context, client *AdminClient, name, pattern string, olderThan time.Duration) (*CacheClearResult, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.clear", client.clusterID)

	req := map[string]interface{}{
		"name":       name,
		"pattern":    pattern,
		"older_than": olderThan.String(),
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var result CacheClearResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func warmCache(ctx context.Context, client *AdminClient, path, namespace string, recursive, dryRun bool) (*CacheWarmResult, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.warm", client.clusterID)

	req := map[string]interface{}{
		"path":      path,
		"namespace": namespace,
		"recursive": recursive,
		"dry_run":   dryRun,
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var result CacheWarmResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func listCacheEntries(ctx context.Context, client *AdminClient, name, pattern, sortBy string, limit int) ([]CacheEntry, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.entries", client.clusterID)

	req := map[string]interface{}{
		"name":    name,
		"pattern": pattern,
		"sort_by": sortBy,
		"limit":   limit,
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var entries []CacheEntry
	if err := json.Unmarshal(msg.Data, &entries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return entries, nil
}

func evictFromCache(ctx context.Context, client *AdminClient, path string) (int64, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.evict", client.clusterID)

	req := map[string]string{"path": path}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return 0, err
	}

	var result struct {
		Evicted int64 `json:"evicted"`
	}
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result.Evicted, nil
}

func getCacheStats(ctx context.Context, client *AdminClient, name string) (*CacheStats, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.stats", client.clusterID)

	req := map[string]string{"name": name}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var stats CacheStats
	if err := json.Unmarshal(msg.Data, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &stats, nil
}

func invalidateCache(ctx context.Context, client *AdminClient, pattern, target, priority string) (*CacheInvalidateResult, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.invalidate", client.clusterID)

	req := map[string]interface{}{
		"pattern":  pattern,
		"target":   target,
		"priority": priority,
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var result CacheInvalidateResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func verifyCache(ctx context.Context, client *AdminClient, name string, fix bool) (*CacheVerifyResult, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.verify", client.clusterID)

	req := map[string]interface{}{
		"name": name,
		"fix":  fix,
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var result CacheVerifyResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func getCacheItem(ctx context.Context, client *AdminClient, key string) (*CacheItemDetail, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.show", client.clusterID)

	req := map[string]string{"key": key}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var item CacheItemDetail
	if err := json.Unmarshal(msg.Data, &item); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &item, nil
}

func refreshCache(ctx context.Context, client *AdminClient, pattern string, force bool) (*CacheRefreshResult, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.refresh", client.clusterID)

	req := map[string]interface{}{
		"pattern": pattern,
		"force":   force,
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var result CacheRefreshResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func setCacheTTL(ctx context.Context, client *AdminClient, key string, ttl time.Duration) (int64, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.set-ttl", client.clusterID)

	req := map[string]interface{}{
		"key": key,
		"ttl": ttl.String(),
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return 0, err
	}

	var result struct {
		Updated int64 `json:"updated"`
	}
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return 0, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result.Updated, nil
}

func getCacheHistory(ctx context.Context, client *AdminClient, name, opType string, limit int) ([]CacheHistoryEntry, error) {
	subject := fmt.Sprintf("kscore.%s.files.admin.cache.history", client.clusterID)

	req := map[string]interface{}{
		"name":  name,
		"type":  opType,
		"limit": limit,
	}
	reqData, _ := json.Marshal(req)

	msg, err := client.nc.RequestWithContext(ctx, subject, reqData)
	if err != nil {
		return nil, err
	}

	var entries []CacheHistoryEntry
	if err := json.Unmarshal(msg.Data, &entries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return entries, nil
}

func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, err
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
