package policy

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// ConflictType represents the type of policy conflict
type ConflictType string

const (
	// ConflictTypeOverlap policies have overlapping scope
	ConflictTypeOverlap ConflictType = "overlap"
	// ConflictTypeContradiction policies have contradicting rules
	ConflictTypeContradiction ConflictType = "contradiction"
	// ConflictTypePrecedence unclear which policy should take precedence
	ConflictTypePrecedence ConflictType = "precedence"
	// ConflictTypeDuplicate policies are effectively duplicates
	ConflictTypeDuplicate ConflictType = "duplicate"
)

// ConflictSeverity represents conflict severity
type ConflictSeverity string

const (
	// ConflictInfo informational only
	ConflictInfo ConflictSeverity = "info"
	// ConflictWarning potential issue
	ConflictWarning ConflictSeverity = "warning"
	// ConflictError must be resolved
	ConflictError ConflictSeverity = "error"
	// ConflictCritical blocks operations
	ConflictCritical ConflictSeverity = "critical"
)

// ResolutionStrategy defines how to resolve conflicts
type ResolutionStrategy string

const (
	// StrategyDeny deny the action when conflict exists
	StrategyDeny ResolutionStrategy = "deny"
	// StrategyAllow allow if any policy allows
	StrategyAllow ResolutionStrategy = "allow"
	// StrategyPriority use highest priority policy
	StrategyPriority ResolutionStrategy = "priority"
	// StrategyMostRestrictive use most restrictive policy
	StrategyMostRestrictive ResolutionStrategy = "most_restrictive"
	// StrategyLeastRestrictive use least restrictive policy
	StrategyLeastRestrictive ResolutionStrategy = "least_restrictive"
	// StrategyMerge merge policy decisions
	StrategyMerge ResolutionStrategy = "merge"
	// StrategyCustom use custom resolver
	StrategyCustom ResolutionStrategy = "custom"
)

// PolicyConflict represents a detected conflict between policies
type PolicyConflict struct {
	// ID is a unique conflict identifier
	ID string `json:"id"`

	// Type of conflict
	Type ConflictType `json:"type"`

	// Severity of the conflict
	Severity ConflictSeverity `json:"severity"`

	// Policies involved in the conflict
	Policies []string `json:"policies"`

	// Description of the conflict
	Description string `json:"description"`

	// Details provides specific conflict information
	Details map[string]interface{} `json:"details,omitempty"`

	// Resolution provides the recommended resolution
	Resolution string `json:"resolution,omitempty"`

	// DetectedAt when the conflict was detected
	DetectedAt time.Time `json:"detected_at"`
}

// ConflictResolution represents a resolved conflict
type ConflictResolution struct {
	// ConflictID that was resolved
	ConflictID string `json:"conflict_id"`

	// Strategy used to resolve
	Strategy ResolutionStrategy `json:"strategy"`

	// WinningPolicy if a single policy won
	WinningPolicy string `json:"winning_policy,omitempty"`

	// Outcome of the resolution
	Outcome string `json:"outcome"`

	// ResolvedAt when the conflict was resolved
	ResolvedAt time.Time `json:"resolved_at"`
}

// ConflictDetectorConfig configures conflict detection
type ConflictDetectorConfig struct {
	// EnableOverlapDetection enables scope overlap checking
	EnableOverlapDetection bool `json:"enable_overlap_detection"`

	// EnableContradictionDetection enables contradiction checking
	EnableContradictionDetection bool `json:"enable_contradiction_detection"`

	// EnableDuplicateDetection enables duplicate checking
	EnableDuplicateDetection bool `json:"enable_duplicate_detection"`

	// SeverityThreshold minimum severity to report
	SeverityThreshold ConflictSeverity `json:"severity_threshold"`
}

// DefaultConflictDetectorConfig returns default configuration
func DefaultConflictDetectorConfig() *ConflictDetectorConfig {
	return &ConflictDetectorConfig{
		EnableOverlapDetection:       true,
		EnableContradictionDetection: true,
		EnableDuplicateDetection:     true,
		SeverityThreshold:            ConflictInfo,
	}
}

// ConflictDetector detects policy conflicts
type ConflictDetector struct {
	config    *ConflictDetectorConfig
	registry  *Registry
	conflicts []*PolicyConflict
	mu        sync.RWMutex
}

// NewConflictDetector creates a new conflict detector
func NewConflictDetector(registry *Registry, config *ConflictDetectorConfig) *ConflictDetector {
	if config == nil {
		config = DefaultConflictDetectorConfig()
	}

	return &ConflictDetector{
		config:    config,
		registry:  registry,
		conflicts: make([]*PolicyConflict, 0),
	}
}

// DetectAll detects all conflicts in the registry
func (d *ConflictDetector) DetectAll() []*PolicyConflict {
	policies := d.registry.ListPolicies()

	d.mu.Lock()
	d.conflicts = make([]*PolicyConflict, 0)
	d.mu.Unlock()

	// Check pairwise conflicts
	for i := 0; i < len(policies); i++ {
		for j := i + 1; j < len(policies); j++ {
			conflicts := d.detectBetween(policies[i], policies[j])
			d.addConflicts(conflicts)
		}
	}

	return d.GetConflicts()
}

// detectBetween detects conflicts between two policies
func (d *ConflictDetector) detectBetween(p1, p2 *Policy) []*PolicyConflict {
	var conflicts []*PolicyConflict

	// Check for overlap
	if d.config.EnableOverlapDetection {
		if overlap := d.detectOverlap(p1, p2); overlap != nil {
			conflicts = append(conflicts, overlap)
		}
	}

	// Check for contradiction
	if d.config.EnableContradictionDetection {
		if contradiction := d.detectContradiction(p1, p2); contradiction != nil {
			conflicts = append(conflicts, contradiction)
		}
	}

	// Check for duplicates
	if d.config.EnableDuplicateDetection {
		if duplicate := d.detectDuplicate(p1, p2); duplicate != nil {
			conflicts = append(conflicts, duplicate)
		}
	}

	return conflicts
}

// detectOverlap checks if two policies have overlapping scope
func (d *ConflictDetector) detectOverlap(p1, p2 *Policy) *PolicyConflict {
	// Check category overlap
	if p1.Category != p2.Category {
		return nil // Different categories rarely conflict
	}

	// Check enforcement mode conflict
	if p1.EnforcementMode != p2.EnforcementMode {
		return &PolicyConflict{
			ID:       fmt.Sprintf("overlap_%s_%s", p1.ID, p2.ID),
			Type:     ConflictTypeOverlap,
			Severity: ConflictWarning,
			Policies: []string{p1.ID, p2.ID},
			Description: fmt.Sprintf("Policies '%s' and '%s' have same category but different enforcement modes",
				p1.Name, p2.Name),
			Details: map[string]interface{}{
				"category":           p1.Category,
				"enforcement_mode_1": p1.EnforcementMode,
				"enforcement_mode_2": p2.EnforcementMode,
			},
			Resolution: "Consider aligning enforcement modes or defining precedence rules",
			DetectedAt: time.Now(),
		}
	}

	return nil
}

// detectContradiction checks if two policies have contradicting rules
func (d *ConflictDetector) detectContradiction(p1, p2 *Policy) *PolicyConflict {
	// Check for same category with different severities (potential contradiction)
	if p1.Category == p2.Category && p1.Severity != p2.Severity {
		// If the difference is significant
		sevDiff := severityDistance(p1.Severity, p2.Severity)
		if sevDiff >= 2 { // More than 1 level difference
			return &PolicyConflict{
				ID:       fmt.Sprintf("contradiction_%s_%s", p1.ID, p2.ID),
				Type:     ConflictTypeContradiction,
				Severity: ConflictWarning,
				Policies: []string{p1.ID, p2.ID},
				Description: fmt.Sprintf("Policies '%s' and '%s' may contradict - significant severity difference",
					p1.Name, p2.Name),
				Details: map[string]interface{}{
					"category":   p1.Category,
					"severity_1": p1.Severity,
					"severity_2": p2.Severity,
				},
				Resolution: "Review policies to ensure consistent severity classification",
				DetectedAt: time.Now(),
			}
		}
	}

	return nil
}

// detectDuplicate checks if two policies are effectively duplicates
func (d *ConflictDetector) detectDuplicate(p1, p2 *Policy) *PolicyConflict {
	// Same type, category, and enforcement mode with similar policy content
	if p1.Type != p2.Type {
		return nil
	}
	if p1.Category != p2.Category {
		return nil
	}
	if p1.EnforcementMode != p2.EnforcementMode {
		return nil
	}

	// Check if policy content is similar
	similarity := calculateSimilarity(p1.Policy, p2.Policy)
	if similarity > 0.9 { // 90% similar
		return &PolicyConflict{
			ID:       fmt.Sprintf("duplicate_%s_%s", p1.ID, p2.ID),
			Type:     ConflictTypeDuplicate,
			Severity: ConflictInfo,
			Policies: []string{p1.ID, p2.ID},
			Description: fmt.Sprintf("Policies '%s' and '%s' appear to be duplicates (%.0f%% similar)",
				p1.Name, p2.Name, similarity*100),
			Details: map[string]interface{}{
				"similarity": similarity,
				"type":       p1.Type,
			},
			Resolution: "Consider consolidating these policies into one",
			DetectedAt: time.Now(),
		}
	}

	return nil
}

// severityDistance calculates the distance between two severities
func severityDistance(s1, s2 Severity) int {
	order := map[Severity]int{
		SeverityLow:      0,
		SeverityMedium:   1,
		SeverityHigh:     2,
		SeverityCritical: 3,
	}

	diff := order[s1] - order[s2]
	if diff < 0 {
		diff = -diff
	}
	return diff
}

// calculateSimilarity calculates similarity between two strings (simplified)
func calculateSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	// Simple character-based similarity
	matches := 0
	maxLen := len1
	if len2 > maxLen {
		maxLen = len2
	}

	minLen := len1
	if len2 < minLen {
		minLen = len2
	}

	for i := 0; i < minLen; i++ {
		if s1[i] == s2[i] {
			matches++
		}
	}

	return float64(matches) / float64(maxLen)
}

// addConflicts adds conflicts to the list
func (d *ConflictDetector) addConflicts(conflicts []*PolicyConflict) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, c := range conflicts {
		if d.meetsThreshold(c.Severity) {
			d.conflicts = append(d.conflicts, c)
		}
	}
}

// meetsThreshold checks if a severity meets the threshold
func (d *ConflictDetector) meetsThreshold(severity ConflictSeverity) bool {
	order := map[ConflictSeverity]int{
		ConflictInfo:     0,
		ConflictWarning:  1,
		ConflictError:    2,
		ConflictCritical: 3,
	}

	return order[severity] >= order[d.config.SeverityThreshold]
}

// GetConflicts returns all detected conflicts
func (d *ConflictDetector) GetConflicts() []*PolicyConflict {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*PolicyConflict, len(d.conflicts))
	copy(result, d.conflicts)
	return result
}

// GetConflictsByType returns conflicts of a specific type
func (d *ConflictDetector) GetConflictsByType(conflictType ConflictType) []*PolicyConflict {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []*PolicyConflict
	for _, c := range d.conflicts {
		if c.Type == conflictType {
			result = append(result, c)
		}
	}
	return result
}

// GetConflictsBySeverity returns conflicts at or above a severity level
func (d *ConflictDetector) GetConflictsBySeverity(minSeverity ConflictSeverity) []*PolicyConflict {
	d.mu.RLock()
	defer d.mu.RUnlock()

	order := map[ConflictSeverity]int{
		ConflictInfo:     0,
		ConflictWarning:  1,
		ConflictError:    2,
		ConflictCritical: 3,
	}

	var result []*PolicyConflict
	for _, c := range d.conflicts {
		if order[c.Severity] >= order[minSeverity] {
			result = append(result, c)
		}
	}
	return result
}

// Clear clears all detected conflicts
func (d *ConflictDetector) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.conflicts = make([]*PolicyConflict, 0)
}

// ConflictResolver resolves policy conflicts
type ConflictResolver struct {
	defaultStrategy ResolutionStrategy
	policyPriority  map[string]int // Policy ID to priority
	customResolvers map[string]ResolverFunc
	mu              sync.RWMutex
}

// ResolverFunc is a custom resolution function
type ResolverFunc func(conflict *PolicyConflict, policies []*Policy) *ConflictResolution

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(defaultStrategy ResolutionStrategy) *ConflictResolver {
	return &ConflictResolver{
		defaultStrategy: defaultStrategy,
		policyPriority:  make(map[string]int),
		customResolvers: make(map[string]ResolverFunc),
	}
}

// SetPolicyPriority sets the priority for a policy (higher = more important)
func (r *ConflictResolver) SetPolicyPriority(policyID string, priority int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policyPriority[policyID] = priority
}

// GetPolicyPriority gets the priority for a policy
func (r *ConflictResolver) GetPolicyPriority(policyID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.policyPriority[policyID]
}

// RegisterResolver registers a custom resolver for a conflict type
func (r *ConflictResolver) RegisterResolver(conflictType ConflictType, fn ResolverFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.customResolvers[string(conflictType)] = fn
}

// Resolve resolves a conflict
func (r *ConflictResolver) Resolve(conflict *PolicyConflict, policies []*Policy) *ConflictResolution {
	// Check for custom resolver
	r.mu.RLock()
	customResolver, hasCustom := r.customResolvers[string(conflict.Type)]
	r.mu.RUnlock()

	if hasCustom {
		return customResolver(conflict, policies)
	}

	// Use default strategy
	return r.resolveWithStrategy(conflict, policies, r.defaultStrategy)
}

// ResolveWithStrategy resolves using a specific strategy
func (r *ConflictResolver) ResolveWithStrategy(
	conflict *PolicyConflict,
	policies []*Policy,
	strategy ResolutionStrategy,
) *ConflictResolution {
	return r.resolveWithStrategy(conflict, policies, strategy)
}

// resolveWithStrategy implements resolution strategies
func (r *ConflictResolver) resolveWithStrategy(
	conflict *PolicyConflict,
	policies []*Policy,
	strategy ResolutionStrategy,
) *ConflictResolution {
	resolution := &ConflictResolution{
		ConflictID: conflict.ID,
		Strategy:   strategy,
		ResolvedAt: time.Now(),
	}

	// Get involved policies
	involved := make([]*Policy, 0)
	for _, p := range policies {
		for _, pid := range conflict.Policies {
			if p.ID == pid {
				involved = append(involved, p)
				break
			}
		}
	}

	if len(involved) == 0 {
		resolution.Outcome = "No involved policies found"
		return resolution
	}

	switch strategy {
	case StrategyDeny:
		resolution.Outcome = "Action denied due to policy conflict"

	case StrategyAllow:
		resolution.Outcome = "Action allowed despite policy conflict"

	case StrategyPriority:
		winner := r.getHighestPriorityPolicy(involved)
		if winner != nil {
			resolution.WinningPolicy = winner.ID
			resolution.Outcome = fmt.Sprintf("Using highest priority policy: %s", winner.Name)
		} else {
			resolution.Outcome = "No priority defined, using first policy"
			if len(involved) > 0 {
				resolution.WinningPolicy = involved[0].ID
			}
		}

	case StrategyMostRestrictive:
		winner := r.getMostRestrictive(involved)
		if winner != nil {
			resolution.WinningPolicy = winner.ID
			resolution.Outcome = fmt.Sprintf("Using most restrictive policy: %s", winner.Name)
		}

	case StrategyLeastRestrictive:
		winner := r.getLeastRestrictive(involved)
		if winner != nil {
			resolution.WinningPolicy = winner.ID
			resolution.Outcome = fmt.Sprintf("Using least restrictive policy: %s", winner.Name)
		}

	case StrategyMerge:
		resolution.Outcome = "Merged policy decisions from all involved policies"

	default:
		resolution.Outcome = "Unknown strategy, no resolution"
	}

	return resolution
}

// getHighestPriorityPolicy returns the policy with highest priority
func (r *ConflictResolver) getHighestPriorityPolicy(policies []*Policy) *Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var highest *Policy
	highestPrio := -1

	for _, p := range policies {
		prio := r.policyPriority[p.ID]
		if prio > highestPrio {
			highestPrio = prio
			highest = p
		}
	}

	return highest
}

// getMostRestrictive returns the most restrictive policy
func (r *ConflictResolver) getMostRestrictive(policies []*Policy) *Policy {
	// Sort by enforcement mode (enforce > warn > audit) and severity
	sorted := make([]*Policy, len(policies))
	copy(sorted, policies)

	sort.Slice(sorted, func(i, j int) bool {
		// Compare enforcement mode
		modeOrder := map[EnforcementMode]int{
			ModeAudit:   0,
			ModeWarn:    1,
			ModeEnforce: 2,
		}

		if modeOrder[sorted[i].EnforcementMode] != modeOrder[sorted[j].EnforcementMode] {
			return modeOrder[sorted[i].EnforcementMode] > modeOrder[sorted[j].EnforcementMode]
		}

		// Compare severity
		sevOrder := map[Severity]int{
			SeverityLow:      0,
			SeverityMedium:   1,
			SeverityHigh:     2,
			SeverityCritical: 3,
		}

		return sevOrder[sorted[i].Severity] > sevOrder[sorted[j].Severity]
	})

	if len(sorted) > 0 {
		return sorted[0]
	}
	return nil
}

// getLeastRestrictive returns the least restrictive policy
func (r *ConflictResolver) getLeastRestrictive(policies []*Policy) *Policy {
	sorted := make([]*Policy, len(policies))
	copy(sorted, policies)

	sort.Slice(sorted, func(i, j int) bool {
		modeOrder := map[EnforcementMode]int{
			ModeAudit:   0,
			ModeWarn:    1,
			ModeEnforce: 2,
		}

		if modeOrder[sorted[i].EnforcementMode] != modeOrder[sorted[j].EnforcementMode] {
			return modeOrder[sorted[i].EnforcementMode] < modeOrder[sorted[j].EnforcementMode]
		}

		sevOrder := map[Severity]int{
			SeverityLow:      0,
			SeverityMedium:   1,
			SeverityHigh:     2,
			SeverityCritical: 3,
		}

		return sevOrder[sorted[i].Severity] < sevOrder[sorted[j].Severity]
	})

	if len(sorted) > 0 {
		return sorted[0]
	}
	return nil
}

// ConflictReport provides a summary of detected conflicts
type ConflictReport struct {
	// GeneratedAt when the report was generated
	GeneratedAt time.Time `json:"generated_at"`

	// TotalConflicts count
	TotalConflicts int `json:"total_conflicts"`

	// ConflictsByType breakdown
	ConflictsByType map[ConflictType]int `json:"conflicts_by_type"`

	// ConflictsBySeverity breakdown
	ConflictsBySeverity map[ConflictSeverity]int `json:"conflicts_by_severity"`

	// Conflicts list
	Conflicts []*PolicyConflict `json:"conflicts"`

	// Recommendations for resolution
	Recommendations []string `json:"recommendations,omitempty"`
}

// GenerateReport generates a conflict report
func (d *ConflictDetector) GenerateReport() *ConflictReport {
	conflicts := d.GetConflicts()

	report := &ConflictReport{
		GeneratedAt:         time.Now(),
		TotalConflicts:      len(conflicts),
		ConflictsByType:     make(map[ConflictType]int),
		ConflictsBySeverity: make(map[ConflictSeverity]int),
		Conflicts:           conflicts,
		Recommendations:     make([]string, 0),
	}

	for _, c := range conflicts {
		report.ConflictsByType[c.Type]++
		report.ConflictsBySeverity[c.Severity]++
	}

	// Generate recommendations
	if report.ConflictsByType[ConflictTypeDuplicate] > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("Consider consolidating %d duplicate policies", report.ConflictsByType[ConflictTypeDuplicate]))
	}

	if report.ConflictsBySeverity[ConflictError] > 0 || report.ConflictsBySeverity[ConflictCritical] > 0 {
		report.Recommendations = append(report.Recommendations,
			"Address error and critical severity conflicts before deployment")
	}

	if report.ConflictsByType[ConflictTypePrecedence] > 0 {
		report.Recommendations = append(report.Recommendations,
			"Define explicit policy priorities to resolve precedence conflicts")
	}

	return report
}
