package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	clierrors "github.com/shawnbutts/keystone-core/internal/cli/errors"
	"github.com/shawnbutts/keystone-core/internal/cli/output"
	"github.com/shawnbutts/keystone-core/internal/files/mirror"
	"github.com/spf13/cobra"
)

// newMirrorsCmd creates the mirrors command group.
func newMirrorsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mirrors",
		Short: "Mirror group management",
		Long:  "Commands for managing mirror groups, sync operations, and conflict resolution.",
	}

	// List mirror groups
	listMirrorsCmd := &cobra.Command{
		Use:   "list",
		Short: "List mirror groups",
		Long:  "List all registered mirror groups and their status.",
		RunE:  runListMirrors,
	}
	listMirrorsCmd.Flags().StringP("output", "o", "table", "Output format (table, text, json, yaml)")
	cmd.AddCommand(listMirrorsCmd)

	// Show mirror group details
	showMirrorCmd := &cobra.Command{
		Use:   "show <group-id>",
		Short: "Show mirror group details",
		Long:  "Display detailed information about a mirror group.",
		Args:  cobra.ExactArgs(1),
		RunE:  runShowMirror,
	}
	showMirrorCmd.Flags().StringP("output", "o", "text", "Output format (text, json, yaml, table)")
	cmd.AddCommand(showMirrorCmd)

	// Sync status
	syncStatusCmd := &cobra.Command{
		Use:   "sync-status <group-id>",
		Short: "Check sync status",
		Long:  "Display the synchronization status for a mirror group.",
		Args:  cobra.ExactArgs(1),
		RunE:  runSyncStatus,
	}
	syncStatusCmd.Flags().StringP("output", "o", "text", "Output format (text, json, yaml, table)")
	cmd.AddCommand(syncStatusCmd)

	// Trigger sync
	syncCmd := &cobra.Command{
		Use:   "sync <group-id>",
		Short: "Trigger manual sync",
		Long:  "Manually trigger a synchronization operation for a mirror group.",
		Args:  cobra.ExactArgs(1),
		RunE:  runSync,
	}
	syncCmd.Flags().String("path", "", "Sync specific path only")
	syncCmd.Flags().String("source", "", "Source mirror ID")
	syncCmd.Flags().String("target", "", "Target mirror ID")
	syncCmd.Flags().Int("priority", 0, "Sync priority (higher = more urgent)")
	syncCmd.Flags().Bool("wait", false, "Wait for sync to complete")
	syncCmd.Flags().Bool("dry-run", false, "Show what would be synchronized without syncing")
	cmd.AddCommand(syncCmd)

	// Mirror health
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Show mirror health",
		Long:  "Display health status for all mirrors across all groups.",
		RunE:  runMirrorHealth,
	}
	healthCmd.Flags().String("group", "", "Filter by group ID")
	healthCmd.Flags().StringP("output", "o", "table", "Output format (table, text, json, yaml)")
	cmd.AddCommand(healthCmd)

	// Failover
	failoverCmd := &cobra.Command{
		Use:   "failover <group-id>",
		Short: "Force failover to specific mirror",
		Long:  "Force all traffic for a mirror group to a specific mirror.",
		Args:  cobra.ExactArgs(1),
		RunE:  runFailover,
	}
	failoverCmd.Flags().String("to", "", "Target mirror ID (required)")
	failoverCmd.Flags().Bool("dry-run", false, "Show what would be done without failing over")
	failoverCmd.MarkFlagRequired("to")
	cmd.AddCommand(failoverCmd)

	// Latency matrix
	latencyCmd := &cobra.Command{
		Use:   "latency",
		Short: "Show latency matrix",
		Long:  "Display latency measurements between mirrors.",
		RunE:  runLatencyMatrix,
	}
	latencyCmd.Flags().String("group", "", "Filter by group ID")
	latencyCmd.Flags().StringP("output", "o", "table", "Output format (table, text, json, yaml)")
	cmd.AddCommand(latencyCmd)

	// List conflicts
	conflictsCmd := &cobra.Command{
		Use:   "conflicts",
		Short: "List conflicts",
		Long:  "List unresolved synchronization conflicts.",
		RunE:  runListConflicts,
	}
	conflictsCmd.Flags().String("group", "", "Filter by group ID")
	conflictsCmd.Flags().StringP("output", "o", "table", "Output format (table, text, json, yaml)")
	cmd.AddCommand(conflictsCmd)

	// Resolve conflict
	resolveCmd := &cobra.Command{
		Use:   "resolve-conflict <conflict-id>",
		Short: "Resolve a conflict",
		Long:  "Resolve a synchronization conflict using a specified strategy.",
		Args:  cobra.ExactArgs(1),
		RunE:  runResolveConflict,
	}
	resolveCmd.Flags().String("strategy", "newest-wins", "Resolution strategy: newest-wins, largest-wins, source, target")
	resolveCmd.Flags().Bool("dry-run", false, "Show what would be done without resolving")
	cmd.AddCommand(resolveCmd)

	// Sync history
	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "Show sync history",
		Long:  "Display recent synchronization history.",
		RunE:  runSyncHistory,
	}
	historyCmd.Flags().Int("limit", 20, "Maximum number of entries to show")
	historyCmd.Flags().String("group", "", "Filter by group ID")
	historyCmd.Flags().StringP("output", "o", "table", "Output format (table, text, json, yaml)")
	cmd.AddCommand(historyCmd)

	return cmd
}

// MirrorGroupInfo contains mirror group information for display.
type MirrorGroupInfo struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	MirrorCount  int             `json:"mirror_count"`
	ReadStrategy string          `json:"read_strategy"`
	WritePolicy  string          `json:"write_policy"`
	PathPrefixes []string        `json:"path_prefixes,omitempty"`
	Namespaces   []string        `json:"namespaces,omitempty"`
	Mirrors      []MirrorInfo    `json:"mirrors"`
	SyncStatus   *SyncStatusInfo `json:"sync_status,omitempty"`
}

// MirrorInfo contains mirror information for display.
type MirrorInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ClusterID string `json:"cluster_id"`
	State     string `json:"state"`
	Latency   string `json:"latency"`
	Priority  int    `json:"priority"`
	Weight    int    `json:"weight"`
	Primary   bool   `json:"primary"`
	ReadOnly  bool   `json:"read_only"`
	Enabled   bool   `json:"enabled"`
	Location  string `json:"location,omitempty"`
}

// SyncStatusInfo contains sync status for display.
type SyncStatusInfo struct {
	State            string `json:"state"`
	LastSyncAt       string `json:"last_sync_at,omitempty"`
	LastSyncDuration string `json:"last_sync_duration,omitempty"`
	LastSyncStatus   string `json:"last_sync_status,omitempty"`
	NextSyncAt       string `json:"next_sync_at,omitempty"`
	PendingOps       int    `json:"pending_ops"`
	ActiveOps        int    `json:"active_ops"`
	ConflictCount    int    `json:"conflict_count"`
}

// ConflictInfo contains conflict information for display.
type ConflictInfo struct {
	ID           string `json:"id"`
	GroupID      string `json:"group_id"`
	Path         string `json:"path"`
	SourceMirror string `json:"source_mirror"`
	TargetMirror string `json:"target_mirror"`
	SourceSize   int64  `json:"source_size"`
	TargetSize   int64  `json:"target_size"`
	DetectedAt   string `json:"detected_at"`
}

// HistoryInfo contains sync history for display.
type HistoryInfo struct {
	OperationID      string `json:"operation_id"`
	GroupID          string `json:"group_id"`
	SourceMirror     string `json:"source_mirror"`
	TargetMirror     string `json:"target_mirror"`
	StartedAt        string `json:"started_at"`
	Duration         string `json:"duration"`
	FilesCompleted   int    `json:"files_completed"`
	FilesFailed      int    `json:"files_failed"`
	BytesTransferred string `json:"bytes_transferred"`
	Status           string `json:"status"`
}

func runListMirrors(cmd *cobra.Command, args []string) error {
	outputFmt, _ := cmd.Flags().GetString("output")

	registry, _, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	groups := registry.List()

	var infos []MirrorGroupInfo
	for _, g := range groups {
		info := MirrorGroupInfo{
			ID:           g.ID(),
			Name:         g.Name(),
			MirrorCount:  len(g.GetMirrors()),
			ReadStrategy: string(g.Config().ReadStrategy),
			WritePolicy:  string(g.Config().WritePolicy),
		}
		infos = append(infos, info)
	}

	format, err := output.ParseFormat(outputFmt)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, infos)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, infos)
	case output.FormatTable, output.FormatText:
		// continue below
	default:
		return fmt.Errorf("unsupported output format: %s", outputFmt)
	}

	if len(infos) == 0 {
		fmt.Println("No mirror groups configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tMIRRORS\tSTRATEGY\tPOLICY")
	fmt.Fprintln(w, "--\t----\t-------\t--------\t------")
	for _, info := range infos {
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			info.ID, info.Name, info.MirrorCount, info.ReadStrategy, info.WritePolicy)
	}
	return w.Flush()
}

func runShowMirror(cmd *cobra.Command, args []string) error {
	groupID := args[0]
	outputFmt, _ := cmd.Flags().GetString("output")

	registry, syncEngine, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	group, ok := registry.Get(groupID)
	if !ok {
		return clierrors.New(clierrors.KindNotFound, fmt.Sprintf("mirror group not found: %s", groupID))
	}

	info := MirrorGroupInfo{
		ID:           group.ID(),
		Name:         group.Name(),
		MirrorCount:  len(group.GetMirrors()),
		ReadStrategy: string(group.Config().ReadStrategy),
		WritePolicy:  string(group.Config().WritePolicy),
	}

	for _, m := range group.GetMirrors() {
		health, _ := group.GetHealth(m.ID)
		mi := MirrorInfo{
			ID:        m.ID,
			Name:      m.Name,
			ClusterID: m.ClusterID,
			Priority:  m.Priority,
			Weight:    m.Weight,
			Primary:   m.IsPrimary,
			ReadOnly:  m.ReadOnly,
			Enabled:   m.Enabled,
		}
		if health != nil {
			mi.State = string(health.State)
			mi.Latency = formatDuration(health.AvgLatency)
		} else {
			mi.State = "unknown"
		}
		if m.Location != nil {
			mi.Location = formatLocation(m.Location)
		}
		info.Mirrors = append(info.Mirrors, mi)
	}

	if syncEngine != nil {
		status := syncEngine.GetGroupStatus(groupID)
		info.SyncStatus = &SyncStatusInfo{
			State:         string(status.State),
			PendingOps:    status.PendingOps,
			ActiveOps:     status.ActiveOps,
			ConflictCount: status.ConflictCount,
		}
		if !status.LastSyncAt.IsZero() {
			info.SyncStatus.LastSyncAt = status.LastSyncAt.Format(time.RFC3339)
			info.SyncStatus.LastSyncDuration = formatDuration(status.LastSyncDuration)
			info.SyncStatus.LastSyncStatus = string(status.LastSyncStatus)
		}
		if !status.NextSyncAt.IsZero() {
			info.SyncStatus.NextSyncAt = status.NextSyncAt.Format(time.RFC3339)
		}
	}

	format, err := output.ParseFormat(outputFmt)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, info)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, info)
	case output.FormatTable:
		table := buildKeyValueTable([][2]string{
			{"GROUP", info.ID},
			{"NAME", info.Name},
			{"STRATEGY", info.ReadStrategy},
			{"POLICY", info.WritePolicy},
		})
		if err := output.WriteTable(os.Stdout, table); err != nil {
			return err
		}
		fmt.Println("\nMirrors:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  ID\tCLUSTER\tSTATE\tLATENCY\tPRIORITY\tPRIMARY\tENABLED")
		for _, m := range info.Mirrors {
			primary := ""
			if m.Primary {
				primary = "yes"
			}
			enabled := ""
			if m.Enabled {
				enabled = "yes"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%d\t%s\t%s\n",
				m.ID, m.ClusterID, m.State, m.Latency, m.Priority, primary, enabled)
		}
		w.Flush()
		if info.SyncStatus != nil {
			fmt.Println("\nSync Status:")
			status := buildKeyValueTable([][2]string{
				{"STATE", info.SyncStatus.State},
				{"LAST SYNC", fmt.Sprintf("%s (%s, %s)", info.SyncStatus.LastSyncAt, info.SyncStatus.LastSyncDuration, info.SyncStatus.LastSyncStatus)},
				{"NEXT SYNC", info.SyncStatus.NextSyncAt},
				{"PENDING", fmt.Sprintf("%d", info.SyncStatus.PendingOps)},
				{"ACTIVE", fmt.Sprintf("%d", info.SyncStatus.ActiveOps)},
				{"CONFLICTS", fmt.Sprintf("%d", info.SyncStatus.ConflictCount)},
			})
			return output.WriteTable(os.Stdout, status)
		}
		return nil
	case output.FormatText:
		// continue below
	default:
		return fmt.Errorf("unsupported output format: %s", outputFmt)
	}

	// Text output
	fmt.Printf("Mirror Group: %s\n", info.ID)
	fmt.Printf("Name:         %s\n", info.Name)
	fmt.Printf("Strategy:     %s\n", info.ReadStrategy)
	fmt.Printf("Policy:       %s\n", info.WritePolicy)
	fmt.Println()

	fmt.Println("Mirrors:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  ID\tCLUSTER\tSTATE\tLATENCY\tPRIORITY\tPRIMARY\tENABLED")
	for _, m := range info.Mirrors {
		primary := ""
		if m.Primary {
			primary = "yes"
		}
		enabled := ""
		if m.Enabled {
			enabled = "yes"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			m.ID, m.ClusterID, m.State, m.Latency, m.Priority, primary, enabled)
	}
	w.Flush()

	if info.SyncStatus != nil {
		fmt.Println()
		fmt.Println("Sync Status:")
		fmt.Printf("  State:     %s\n", info.SyncStatus.State)
		if info.SyncStatus.LastSyncAt != "" {
			fmt.Printf("  Last Sync: %s (%s, %s)\n",
				info.SyncStatus.LastSyncAt, info.SyncStatus.LastSyncDuration, info.SyncStatus.LastSyncStatus)
		}
		if info.SyncStatus.NextSyncAt != "" {
			fmt.Printf("  Next Sync: %s\n", info.SyncStatus.NextSyncAt)
		}
		fmt.Printf("  Pending:   %d\n", info.SyncStatus.PendingOps)
		fmt.Printf("  Active:    %d\n", info.SyncStatus.ActiveOps)
		if info.SyncStatus.ConflictCount > 0 {
			fmt.Printf("  Conflicts: %d\n", info.SyncStatus.ConflictCount)
		}
	}

	return nil
}

func runSyncStatus(cmd *cobra.Command, args []string) error {
	groupID := args[0]
	outputFmt, _ := cmd.Flags().GetString("output")

	registry, syncEngine, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	_, ok := registry.Get(groupID)
	if !ok {
		return clierrors.New(clierrors.KindNotFound, fmt.Sprintf("mirror group not found: %s", groupID))
	}

	if syncEngine == nil {
		return clierrors.New(clierrors.KindUnavailable, "sync engine not available")
	}

	status := syncEngine.GetGroupStatus(groupID)
	info := SyncStatusInfo{
		State:         string(status.State),
		PendingOps:    status.PendingOps,
		ActiveOps:     status.ActiveOps,
		ConflictCount: status.ConflictCount,
	}
	if !status.LastSyncAt.IsZero() {
		info.LastSyncAt = status.LastSyncAt.Format(time.RFC3339)
		info.LastSyncDuration = formatDuration(status.LastSyncDuration)
		info.LastSyncStatus = string(status.LastSyncStatus)
	}
	if !status.NextSyncAt.IsZero() {
		info.NextSyncAt = status.NextSyncAt.Format(time.RFC3339)
	}

	// Get active operations
	activeOps := syncEngine.GetActiveOperations()
	pendingOps := syncEngine.GetPendingOperations()

	result := struct {
		Status     SyncStatusInfo          `json:"status"`
		ActiveOps  []*mirror.SyncOperation `json:"active_ops,omitempty"`
		PendingOps []*mirror.SyncOperation `json:"pending_ops,omitempty"`
	}{
		Status:     info,
		ActiveOps:  filterOpsByGroup(activeOps, groupID),
		PendingOps: filterOpsByGroup(pendingOps, groupID),
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
		status := buildKeyValueTable([][2]string{
			{"STATE", info.State},
			{"LAST SYNC", info.LastSyncAt},
			{"DURATION", info.LastSyncDuration},
			{"RESULT", info.LastSyncStatus},
			{"NEXT SYNC", info.NextSyncAt},
			{"PENDING", fmt.Sprintf("%d operations", info.PendingOps)},
			{"ACTIVE", fmt.Sprintf("%d operations", info.ActiveOps)},
			{"CONFLICTS", fmt.Sprintf("%d unresolved", info.ConflictCount)},
		})
		if err := output.WriteTable(os.Stdout, status); err != nil {
			return err
		}
		if len(result.ActiveOps) > 0 {
			fmt.Println("\nActive Operations:")
			rows := make([][]string, 0, len(result.ActiveOps))
			for _, op := range result.ActiveOps {
				rows = append(rows, []string{
					op.ID,
					op.SourceMirror,
					op.TargetMirror,
					fmt.Sprintf("%.1f%%", op.Progress*100),
				})
			}
			return output.WriteTable(os.Stdout, &output.Table{
				Headers: []string{"ID", "SOURCE", "TARGET", "PROGRESS"},
				Rows:    rows,
			})
		}
		return nil
	case output.FormatText:
		// continue below
	default:
		return fmt.Errorf("unsupported output format: %s", outputFmt)
	}

	// Text output
	fmt.Printf("Sync Status for %s\n", groupID)
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("State:     %s\n", info.State)
	if info.LastSyncAt != "" {
		fmt.Printf("Last Sync: %s\n", info.LastSyncAt)
		fmt.Printf("Duration:  %s\n", info.LastSyncDuration)
		fmt.Printf("Result:    %s\n", info.LastSyncStatus)
	}
	if info.NextSyncAt != "" {
		fmt.Printf("Next Sync: %s\n", info.NextSyncAt)
	}
	fmt.Printf("Pending:   %d operations\n", info.PendingOps)
	fmt.Printf("Active:    %d operations\n", info.ActiveOps)
	if info.ConflictCount > 0 {
		fmt.Printf("Conflicts: %d unresolved\n", info.ConflictCount)
	}

	groupActiveOps := filterOpsByGroup(activeOps, groupID)
	if len(groupActiveOps) > 0 {
		fmt.Println()
		fmt.Println("Active Operations:")
		for _, op := range groupActiveOps {
			fmt.Printf("  %s: %s -> %s (%.1f%% complete)\n",
				op.ID, op.SourceMirror, op.TargetMirror, op.Progress*100)
		}
	}

	return nil
}

func runSync(cmd *cobra.Command, args []string) error {
	groupID := args[0]
	pathPrefix, _ := cmd.Flags().GetString("path")
	source, _ := cmd.Flags().GetString("source")
	target, _ := cmd.Flags().GetString("target")
	priority, _ := cmd.Flags().GetInt("priority")
	wait, _ := cmd.Flags().GetBool("wait")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if dryRun {
		if source != "" && target != "" {
			fmt.Printf("Dry run: would sync %s -> %s in group %s (priority %d)\n", source, target, groupID, priority)
		} else {
			if pathPrefix != "" {
				fmt.Printf("Dry run: would sync group %s (path %s, priority %d)\n", groupID, pathPrefix, priority)
			} else {
				fmt.Printf("Dry run: would sync group %s (priority %d)\n", groupID, priority)
			}
		}
		return nil
	}

	if wait {
		fmt.Println("Note: --wait is not implemented yet; returning after scheduling.")
	}

	registry, syncEngine, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	group, ok := registry.Get(groupID)
	if !ok {
		return clierrors.New(clierrors.KindNotFound, fmt.Sprintf("mirror group not found: %s", groupID))
	}

	if syncEngine == nil {
		return clierrors.New(clierrors.KindUnavailable, "sync engine not available")
	}

	// If source/target specified, trigger specific sync
	if source != "" && target != "" {
		op, err := syncEngine.TriggerSync(groupID, source, target, priority)
		if err != nil {
			return fmt.Errorf("failed to trigger sync: %w", err)
		}
		fmt.Printf("Sync operation triggered: %s\n", op.ID)
		fmt.Printf("  Source: %s\n", source)
		fmt.Printf("  Target: %s\n", target)
		return nil
	}

	// Otherwise schedule sync for entire group
	mirrors := group.GetMirrors()
	if len(mirrors) < 2 {
		return clierrors.New(clierrors.KindInvalidArgument, "group has fewer than 2 mirrors, nothing to sync")
	}

	err = syncEngine.ScheduleSync(groupID, pathPrefix, priority)
	if err != nil {
		return fmt.Errorf("failed to schedule sync: %w", err)
	}

	fmt.Printf("Sync scheduled for group: %s\n", groupID)
	return nil
}

func runMirrorHealth(cmd *cobra.Command, args []string) error {
	groupFilter, _ := cmd.Flags().GetString("group")
	outputFmt, _ := cmd.Flags().GetString("output")

	registry, _, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	type HealthEntry struct {
		GroupID   string `json:"group_id"`
		MirrorID  string `json:"mirror_id"`
		State     string `json:"state"`
		Latency   string `json:"latency"`
		LastCheck string `json:"last_check"`
		LastError string `json:"last_error,omitempty"`
	}

	var entries []HealthEntry

	allHealth := registry.GetAllHealth()
	for groupID, groupHealth := range allHealth {
		if groupFilter != "" && groupID != groupFilter {
			continue
		}
		for mirrorID, health := range groupHealth {
			entry := HealthEntry{
				GroupID:  groupID,
				MirrorID: mirrorID,
				State:    string(health.State),
				Latency:  formatDuration(health.AvgLatency),
			}
			if !health.LastCheck.IsZero() {
				entry.LastCheck = health.LastCheck.Format(time.RFC3339)
			}
			if health.LastError != "" {
				entry.LastError = health.LastError
			}
			entries = append(entries, entry)
		}
	}

	// Sort by group then mirror
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].GroupID != entries[j].GroupID {
			return entries[i].GroupID < entries[j].GroupID
		}
		return entries[i].MirrorID < entries[j].MirrorID
	})

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
		// continue below
	default:
		return fmt.Errorf("unsupported output format: %s", outputFmt)
	}

	if len(entries) == 0 {
		fmt.Println("No health data available.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "GROUP\tMIRROR\tSTATE\tLATENCY\tLAST CHECK\tERROR")
	fmt.Fprintln(w, "-----\t------\t-----\t-------\t----------\t-----")
	for _, e := range entries {
		errStr := ""
		if e.LastError != "" {
			if len(e.LastError) > 30 {
				errStr = e.LastError[:30] + "..."
			} else {
				errStr = e.LastError
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.GroupID, e.MirrorID, e.State, e.Latency, e.LastCheck, errStr)
	}
	return w.Flush()
}

func runFailover(cmd *cobra.Command, args []string) error {
	groupID := args[0]
	targetMirror, _ := cmd.Flags().GetString("to")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if dryRun {
		fmt.Printf("Dry run: would fail over group %s to mirror %s\n", groupID, targetMirror)
		return nil
	}

	registry, _, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	group, ok := registry.Get(groupID)
	if !ok {
		return clierrors.New(clierrors.KindNotFound, fmt.Sprintf("mirror group not found: %s", groupID))
	}

	// Verify target mirror exists
	mirrors := group.GetMirrors()
	var found bool
	for _, m := range mirrors {
		if m.ID == targetMirror {
			found = true
			break
		}
	}
	if !found {
		return clierrors.New(clierrors.KindNotFound, fmt.Sprintf("mirror not found in group: %s", targetMirror))
	}

	// Note: In a real implementation, this would update the group's routing
	// to force traffic to the specified mirror. For now, we just print.
	fmt.Printf("Failover initiated for group %s to mirror %s\n", groupID, targetMirror)
	fmt.Println("Note: This is a placeholder. Full implementation requires routing updates.")

	return nil
}

func runLatencyMatrix(cmd *cobra.Command, args []string) error {
	groupFilter, _ := cmd.Flags().GetString("group")
	outputFmt, _ := cmd.Flags().GetString("output")

	registry, _, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	type LatencyEntry struct {
		GroupID  string `json:"group_id"`
		MirrorID string `json:"mirror_id"`
		Latency  string `json:"latency"`
		Min      string `json:"min,omitempty"`
		Max      string `json:"max,omitempty"`
		P50      string `json:"p50,omitempty"`
		P95      string `json:"p95,omitempty"`
		P99      string `json:"p99,omitempty"`
	}

	var entries []LatencyEntry

	allHealth := registry.GetAllHealth()
	for groupID, groupHealth := range allHealth {
		if groupFilter != "" && groupID != groupFilter {
			continue
		}
		for mirrorID, health := range groupHealth {
			entry := LatencyEntry{
				GroupID:  groupID,
				MirrorID: mirrorID,
				Latency:  formatDuration(health.AvgLatency),
			}
			entries = append(entries, entry)
		}
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
		// continue below
	default:
		return fmt.Errorf("unsupported output format: %s", outputFmt)
	}

	if len(entries) == 0 {
		fmt.Println("No latency data available.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "GROUP\tMIRROR\tLATENCY (EMA)")
	fmt.Fprintln(w, "-----\t------\t------------")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.GroupID, e.MirrorID, e.Latency)
	}
	return w.Flush()
}

func runListConflicts(cmd *cobra.Command, args []string) error {
	groupFilter, _ := cmd.Flags().GetString("group")
	outputFmt, _ := cmd.Flags().GetString("output")

	_, syncEngine, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	if syncEngine == nil {
		return clierrors.New(clierrors.KindUnavailable, "sync engine not available")
	}

	conflicts := syncEngine.GetConflicts()

	var infos []ConflictInfo
	for _, c := range conflicts {
		if groupFilter != "" && c.GroupID != groupFilter {
			continue
		}
		info := ConflictInfo{
			ID:           c.ID,
			GroupID:      c.GroupID,
			Path:         c.Path,
			SourceMirror: c.SourceMirror,
			TargetMirror: c.TargetMirror,
			SourceSize:   c.SourceInfo.Size,
			TargetSize:   c.TargetInfo.Size,
			DetectedAt:   c.DetectedAt.Format(time.RFC3339),
		}
		infos = append(infos, info)
	}

	format, err := output.ParseFormat(outputFmt)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, infos)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, infos)
	case output.FormatTable, output.FormatText:
		// continue below
	default:
		return fmt.Errorf("unsupported output format: %s", outputFmt)
	}

	if len(infos) == 0 {
		fmt.Println("No unresolved conflicts.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tGROUP\tPATH\tSOURCE\tTARGET\tDETECTED")
	fmt.Fprintln(w, "--\t-----\t----\t------\t------\t--------")
	for _, c := range infos {
		// Truncate path if too long
		path := c.Path
		if len(path) > 30 {
			path = "..." + path[len(path)-27:]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			c.ID[:8]+"...", c.GroupID, path, c.SourceMirror, c.TargetMirror, c.DetectedAt)
	}
	return w.Flush()
}

func runResolveConflict(cmd *cobra.Command, args []string) error {
	conflictID := args[0]
	strategy, _ := cmd.Flags().GetString("strategy")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	_, syncEngine, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	if syncEngine == nil {
		return clierrors.New(clierrors.KindUnavailable, "sync engine not available")
	}

	// Map strategy to resolution
	var resolution string
	switch strategy {
	case "newest-wins", "source":
		resolution = "source"
	case "largest-wins", "target":
		resolution = "target"
	default:
		return clierrors.New(clierrors.KindInvalidArgument, fmt.Sprintf("invalid strategy: %s (valid: newest-wins, largest-wins, source, target)", strategy))
	}

	if dryRun {
		fmt.Printf("Dry run: would resolve conflict %s using strategy %s\n", conflictID, strategy)
		return nil
	}

	err = syncEngine.ResolveConflict(conflictID, resolution, "cli-user")
	if err != nil {
		return fmt.Errorf("failed to resolve conflict: %w", err)
	}

	fmt.Printf("Conflict %s resolved using strategy: %s\n", conflictID, strategy)
	return nil
}

func runSyncHistory(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	groupFilter, _ := cmd.Flags().GetString("group")
	outputFmt, _ := cmd.Flags().GetString("output")

	_, syncEngine, err := getMirrorRegistry()
	if err != nil {
		return err
	}

	if syncEngine == nil {
		return fmt.Errorf("sync engine not available")
	}

	history := syncEngine.GetHistory(limit)

	var infos []HistoryInfo
	for _, h := range history {
		if groupFilter != "" && h.GroupID != groupFilter {
			continue
		}
		info := HistoryInfo{
			OperationID:      h.OperationID,
			GroupID:          h.GroupID,
			SourceMirror:     h.SourceMirror,
			TargetMirror:     h.TargetMirror,
			StartedAt:        h.StartedAt.Format(time.RFC3339),
			Duration:         formatDuration(h.Duration),
			FilesCompleted:   h.FilesCompleted,
			FilesFailed:      h.FilesFailed,
			BytesTransferred: formatBytes(h.BytesTransferred),
			Status:           string(h.Status),
		}
		infos = append(infos, info)
	}

	format, err := output.ParseFormat(outputFmt)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		return output.WriteJSON(os.Stdout, infos)
	case output.FormatYAML:
		return output.WriteYAML(os.Stdout, infos)
	case output.FormatTable, output.FormatText:
		// continue below
	default:
		return fmt.Errorf("unsupported output format: %s", outputFmt)
	}

	if len(infos) == 0 {
		fmt.Println("No sync history available.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STARTED\tGROUP\tSOURCE\tTARGET\tFILES\tBYTES\tDURATION\tSTATUS")
	fmt.Fprintln(w, "-------\t-----\t------\t------\t-----\t-----\t--------\t------")
	for _, h := range infos {
		files := fmt.Sprintf("%d", h.FilesCompleted)
		if h.FilesFailed > 0 {
			files += fmt.Sprintf(" (%d failed)", h.FilesFailed)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			h.StartedAt, h.GroupID, h.SourceMirror, h.TargetMirror,
			files, h.BytesTransferred, h.Duration, h.Status)
	}
	return w.Flush()
}

// getMirrorRegistry returns the mirror registry and sync engine.
// In a real implementation, this would connect to the file server.
func getMirrorRegistry() (*mirror.Registry, *mirror.SyncEngine, error) {
	// Create an in-memory registry for demonstration
	// In production, this would load from server configuration
	registry := mirror.NewRegistry()
	syncConfig := mirror.DefaultSyncConfig()
	syncEngine := mirror.NewSyncEngine(registry, syncConfig)

	return registry, syncEngine, nil
}

func filterOpsByGroup(ops []*mirror.SyncOperation, groupID string) []*mirror.SyncOperation {
	var result []*mirror.SyncOperation
	for _, op := range ops {
		if op.GroupID == groupID {
			result = append(result, op)
		}
	}
	return result
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatLocation(loc *mirror.Location) string {
	if loc == nil {
		return ""
	}
	parts := []string{}
	if loc.Region != "" {
		parts = append(parts, loc.Region)
	}
	if loc.Zone != "" {
		parts = append(parts, loc.Zone)
	}
	if loc.Datacenter != "" {
		parts = append(parts, loc.Datacenter)
	}
	return strings.Join(parts, "/")
}
