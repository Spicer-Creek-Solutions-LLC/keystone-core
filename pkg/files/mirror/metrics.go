// Package mirror provides Prometheus metrics for mirror operations.
package mirror

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// MirrorMetrics collects metrics for mirror operations.
type MirrorMetrics struct {
	mu sync.RWMutex

	// Group metrics
	groupCount int

	// Mirror metrics by group and mirror
	mirrorHealth map[string]map[string]MirrorState

	// Operation counts
	readOperations    map[string]int64 // by group
	writeOperations   map[string]int64 // by group
	readBytes         map[string]int64 // by group
	writeBytes        map[string]int64 // by group
	readErrors        map[string]int64 // by group
	writeErrors       map[string]int64 // by group

	// Sync metrics
	syncOperationsTotal     map[string]int64   // by group
	syncOperationsActive    map[string]int64   // by group
	syncOperationsSucceeded map[string]int64   // by group
	syncOperationsFailed    map[string]int64   // by group
	syncBytesTotal          map[string]int64   // by group
	syncFilesTotal          map[string]int64   // by group
	syncConflicts           map[string]int64   // by group
	syncLatencySum          map[string]float64 // by group (seconds)
	syncLatencyCount        map[string]int64   // by group

	// Latency histograms
	readLatencyBuckets  map[string][]int64 // bucket counts by group
	writeLatencyBuckets map[string][]int64 // bucket counts by group
	syncLatencyBuckets  map[string][]int64 // bucket counts by group

	// Latency bucket boundaries (in milliseconds)
	latencyBuckets []float64
}

// NewMirrorMetrics creates a new metrics collector.
func NewMirrorMetrics() *MirrorMetrics {
	return &MirrorMetrics{
		mirrorHealth:            make(map[string]map[string]MirrorState),
		readOperations:          make(map[string]int64),
		writeOperations:         make(map[string]int64),
		readBytes:               make(map[string]int64),
		writeBytes:              make(map[string]int64),
		readErrors:              make(map[string]int64),
		writeErrors:             make(map[string]int64),
		syncOperationsTotal:     make(map[string]int64),
		syncOperationsActive:    make(map[string]int64),
		syncOperationsSucceeded: make(map[string]int64),
		syncOperationsFailed:    make(map[string]int64),
		syncBytesTotal:          make(map[string]int64),
		syncFilesTotal:          make(map[string]int64),
		syncConflicts:           make(map[string]int64),
		syncLatencySum:          make(map[string]float64),
		syncLatencyCount:        make(map[string]int64),
		readLatencyBuckets:      make(map[string][]int64),
		writeLatencyBuckets:     make(map[string][]int64),
		syncLatencyBuckets:      make(map[string][]int64),
		latencyBuckets:          []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}, // ms
	}
}

// SetGroupCount sets the number of mirror groups.
func (m *MirrorMetrics) SetGroupCount(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groupCount = count
}

// SetMirrorHealth sets the health state for a mirror.
func (m *MirrorMetrics) SetMirrorHealth(groupID, mirrorID string, state MirrorState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mirrorHealth[groupID] == nil {
		m.mirrorHealth[groupID] = make(map[string]MirrorState)
	}
	m.mirrorHealth[groupID][mirrorID] = state
}

// RecordRead records a read operation.
func (m *MirrorMetrics) RecordRead(groupID string, bytes int64, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.readOperations[groupID]++
	m.readBytes[groupID] += bytes
	if err != nil {
		m.readErrors[groupID]++
	}

	m.recordLatencyBucket(m.readLatencyBuckets, groupID, latency)
}

// RecordWrite records a write operation.
func (m *MirrorMetrics) RecordWrite(groupID string, bytes int64, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.writeOperations[groupID]++
	m.writeBytes[groupID] += bytes
	if err != nil {
		m.writeErrors[groupID]++
	}

	m.recordLatencyBucket(m.writeLatencyBuckets, groupID, latency)
}

// RecordSyncOperation records a sync operation.
func (m *MirrorMetrics) RecordSyncOperation(groupID string, succeeded bool, bytes int64, files int, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.syncOperationsTotal[groupID]++
	if succeeded {
		m.syncOperationsSucceeded[groupID]++
	} else {
		m.syncOperationsFailed[groupID]++
	}
	m.syncBytesTotal[groupID] += bytes
	m.syncFilesTotal[groupID] += int64(files)

	m.syncLatencySum[groupID] += latency.Seconds()
	m.syncLatencyCount[groupID]++

	m.recordLatencyBucket(m.syncLatencyBuckets, groupID, latency)
}

// SetActiveSyncOperations sets the number of active sync operations.
func (m *MirrorMetrics) SetActiveSyncOperations(groupID string, count int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncOperationsActive[groupID] = count
}

// RecordConflict records a sync conflict.
func (m *MirrorMetrics) RecordConflict(groupID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncConflicts[groupID]++
}

// recordLatencyBucket records a latency in the appropriate bucket.
func (m *MirrorMetrics) recordLatencyBucket(buckets map[string][]int64, groupID string, latency time.Duration) {
	if buckets[groupID] == nil {
		buckets[groupID] = make([]int64, len(m.latencyBuckets)+1)
	}

	ms := float64(latency.Milliseconds())
	for i, bucket := range m.latencyBuckets {
		if ms <= bucket {
			buckets[groupID][i]++
			return
		}
	}
	// +Inf bucket
	buckets[groupID][len(m.latencyBuckets)]++
}

// Export returns all metrics in Prometheus text format.
func (m *MirrorMetrics) Export() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder

	// Group count
	sb.WriteString("# HELP kscore_mirror_groups_total Total number of mirror groups\n")
	sb.WriteString("# TYPE kscore_mirror_groups_total gauge\n")
	fmt.Fprintf(&sb, "kscore_mirror_groups_total %d\n\n", m.groupCount)

	// Mirror health
	sb.WriteString("# HELP kscore_mirror_health Mirror health status (1=healthy, 0=unhealthy)\n")
	sb.WriteString("# TYPE kscore_mirror_health gauge\n")
	for groupID, mirrors := range m.mirrorHealth {
		for mirrorID, state := range mirrors {
			healthy := 0
			if state == MirrorStateHealthy {
				healthy = 1
			}
			fmt.Fprintf(&sb, "kscore_mirror_health{group=\"%s\",mirror=\"%s\",state=\"%s\"} %d\n",
				groupID, mirrorID, state, healthy)
		}
	}
	sb.WriteString("\n")

	// Read operations
	sb.WriteString("# HELP kscore_mirror_read_operations_total Total read operations\n")
	sb.WriteString("# TYPE kscore_mirror_read_operations_total counter\n")
	for groupID, count := range m.readOperations {
		fmt.Fprintf(&sb, "kscore_mirror_read_operations_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	// Write operations
	sb.WriteString("# HELP kscore_mirror_write_operations_total Total write operations\n")
	sb.WriteString("# TYPE kscore_mirror_write_operations_total counter\n")
	for groupID, count := range m.writeOperations {
		fmt.Fprintf(&sb, "kscore_mirror_write_operations_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	// Read bytes
	sb.WriteString("# HELP kscore_mirror_read_bytes_total Total bytes read\n")
	sb.WriteString("# TYPE kscore_mirror_read_bytes_total counter\n")
	for groupID, bytes := range m.readBytes {
		fmt.Fprintf(&sb, "kscore_mirror_read_bytes_total{group=\"%s\"} %d\n", groupID, bytes)
	}
	sb.WriteString("\n")

	// Write bytes
	sb.WriteString("# HELP kscore_mirror_write_bytes_total Total bytes written\n")
	sb.WriteString("# TYPE kscore_mirror_write_bytes_total counter\n")
	for groupID, bytes := range m.writeBytes {
		fmt.Fprintf(&sb, "kscore_mirror_write_bytes_total{group=\"%s\"} %d\n", groupID, bytes)
	}
	sb.WriteString("\n")

	// Read errors
	sb.WriteString("# HELP kscore_mirror_read_errors_total Total read errors\n")
	sb.WriteString("# TYPE kscore_mirror_read_errors_total counter\n")
	for groupID, count := range m.readErrors {
		fmt.Fprintf(&sb, "kscore_mirror_read_errors_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	// Write errors
	sb.WriteString("# HELP kscore_mirror_write_errors_total Total write errors\n")
	sb.WriteString("# TYPE kscore_mirror_write_errors_total counter\n")
	for groupID, count := range m.writeErrors {
		fmt.Fprintf(&sb, "kscore_mirror_write_errors_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	// Sync operations
	sb.WriteString("# HELP kscore_mirror_sync_operations_total Total sync operations\n")
	sb.WriteString("# TYPE kscore_mirror_sync_operations_total counter\n")
	for groupID, count := range m.syncOperationsTotal {
		fmt.Fprintf(&sb, "kscore_mirror_sync_operations_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	sb.WriteString("# HELP kscore_mirror_sync_operations_active Active sync operations\n")
	sb.WriteString("# TYPE kscore_mirror_sync_operations_active gauge\n")
	for groupID, count := range m.syncOperationsActive {
		fmt.Fprintf(&sb, "kscore_mirror_sync_operations_active{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	sb.WriteString("# HELP kscore_mirror_sync_operations_succeeded_total Successful sync operations\n")
	sb.WriteString("# TYPE kscore_mirror_sync_operations_succeeded_total counter\n")
	for groupID, count := range m.syncOperationsSucceeded {
		fmt.Fprintf(&sb, "kscore_mirror_sync_operations_succeeded_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	sb.WriteString("# HELP kscore_mirror_sync_operations_failed_total Failed sync operations\n")
	sb.WriteString("# TYPE kscore_mirror_sync_operations_failed_total counter\n")
	for groupID, count := range m.syncOperationsFailed {
		fmt.Fprintf(&sb, "kscore_mirror_sync_operations_failed_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	// Sync bytes and files
	sb.WriteString("# HELP kscore_mirror_sync_bytes_total Total bytes synced\n")
	sb.WriteString("# TYPE kscore_mirror_sync_bytes_total counter\n")
	for groupID, bytes := range m.syncBytesTotal {
		fmt.Fprintf(&sb, "kscore_mirror_sync_bytes_total{group=\"%s\"} %d\n", groupID, bytes)
	}
	sb.WriteString("\n")

	sb.WriteString("# HELP kscore_mirror_sync_files_total Total files synced\n")
	sb.WriteString("# TYPE kscore_mirror_sync_files_total counter\n")
	for groupID, count := range m.syncFilesTotal {
		fmt.Fprintf(&sb, "kscore_mirror_sync_files_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	// Conflicts
	sb.WriteString("# HELP kscore_mirror_sync_conflicts_total Total sync conflicts\n")
	sb.WriteString("# TYPE kscore_mirror_sync_conflicts_total counter\n")
	for groupID, count := range m.syncConflicts {
		fmt.Fprintf(&sb, "kscore_mirror_sync_conflicts_total{group=\"%s\"} %d\n", groupID, count)
	}
	sb.WriteString("\n")

	// Latency histograms
	m.writeHistogram(&sb, "kscore_mirror_read_latency_seconds", "Read latency histogram", m.readLatencyBuckets)
	m.writeHistogram(&sb, "kscore_mirror_write_latency_seconds", "Write latency histogram", m.writeLatencyBuckets)
	m.writeHistogram(&sb, "kscore_mirror_sync_latency_seconds", "Sync latency histogram", m.syncLatencyBuckets)

	return sb.String()
}

// writeHistogram writes a histogram in Prometheus format.
func (m *MirrorMetrics) writeHistogram(sb *strings.Builder, name, help string, buckets map[string][]int64) {
	if len(buckets) == 0 {
		return
	}

	fmt.Fprintf(sb, "# HELP %s %s\n", name, help)
	fmt.Fprintf(sb, "# TYPE %s histogram\n", name)

	groups := make([]string, 0, len(buckets))
	for g := range buckets {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	for _, groupID := range groups {
		counts := buckets[groupID]
		var sum int64
		for i, count := range counts {
			sum += count
			var le string
			if i < len(m.latencyBuckets) {
				le = fmt.Sprintf("%.3f", m.latencyBuckets[i]/1000.0) // Convert ms to seconds
			} else {
				le = "+Inf"
			}
			fmt.Fprintf(sb, "%s_bucket{group=\"%s\",le=\"%s\"} %d\n", name, groupID, le, sum)
		}
		// Note: _sum and _count would require tracking actual sum values
		// For simplicity, just output the buckets
	}
	sb.WriteString("\n")
}

// MetricsHandler returns an HTTP handler for Prometheus metrics.
func (m *MirrorMetrics) MetricsHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Write([]byte(m.Export()))
	}
}

// CollectFromRegistry collects metrics from a mirror registry.
func (m *MirrorMetrics) CollectFromRegistry(registry *Registry) {
	groups := registry.List()
	m.SetGroupCount(len(groups))

	for _, g := range groups {
		for _, mirror := range g.GetMirrors() {
			if health, ok := g.GetHealth(mirror.ID); ok {
				m.SetMirrorHealth(g.ID(), mirror.ID, health.State)
			}
		}
	}
}

// CollectFromSyncEngine collects metrics from a sync engine.
func (m *MirrorMetrics) CollectFromSyncEngine(engine *SyncEngine) {
	if engine == nil {
		return
	}

	// Count active operations per group
	activeOps := engine.GetActiveOperations()
	activeCounts := make(map[string]int64)
	for _, op := range activeOps {
		activeCounts[op.GroupID]++
	}
	for groupID, count := range activeCounts {
		m.SetActiveSyncOperations(groupID, count)
	}
}
