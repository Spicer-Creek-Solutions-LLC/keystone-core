package dns

import (
	"fmt"
	"sort"
	"strings"
)

// ChangeType represents the type of change to a DNS record.
type ChangeType string

// ChangeType constants define the supported types.
const (
	ChangeTypeCreate ChangeType = "create"
	ChangeTypeUpdate ChangeType = "update"
	ChangeTypeDelete ChangeType = "delete"
	ChangeTypeNoop   ChangeType = "noop"
)

// RecordChange represents a planned change to a DNS record.
type RecordChange struct {
	// Type is the type of change
	Type ChangeType `json:"type"`

	// Record is the target record state
	Record *Record `json:"record"`

	// Current is the current record state (for updates/deletes)
	Current *Record `json:"current,omitempty"`

	// Diff describes what fields changed (for updates)
	Diff map[string]FieldDiff `json:"diff,omitempty"`
}

// FieldDiff describes a field-level change.
type FieldDiff struct {
	Old interface{} `json:"old"`
	New interface{} `json:"new"`
}

// Plan represents a complete change plan for a zone.
type Plan struct {
	// Zone is the zone name
	Zone string `json:"zone"`

	// Changes is the list of planned changes
	Changes []RecordChange `json:"changes"`
}

// Summary returns a summary of the plan.
func (p *Plan) Summary() PlanSummary {
	summary := PlanSummary{Zone: p.Zone}
	for _, change := range p.Changes {
		switch change.Type {
		case ChangeTypeCreate:
			summary.Create++
		case ChangeTypeUpdate:
			summary.Update++
		case ChangeTypeDelete:
			summary.Delete++
		case ChangeTypeNoop:
			summary.Noop++
		}
	}
	return summary
}

// HasChanges returns true if there are any changes to make.
func (p *Plan) HasChanges() bool {
	for _, change := range p.Changes {
		if change.Type != ChangeTypeNoop {
			return true
		}
	}
	return false
}

// PlanSummary provides a summary of changes.
type PlanSummary struct {
	Zone   string `json:"zone"`
	Create int    `json:"create"`
	Update int    `json:"update"`
	Delete int    `json:"delete"`
	Noop   int    `json:"noop"`
}

// String returns a human-readable summary.
func (s PlanSummary) String() string {
	return fmt.Sprintf("Zone %s: %d create, %d update, %d delete, %d unchanged",
		s.Zone, s.Create, s.Update, s.Delete, s.Noop)
}

// Differ computes differences between desired and current DNS records.
type Differ struct {
	// Zone is the zone being diffed
	Zone string

	// IgnoreTTL ignores TTL differences
	IgnoreTTL bool

	// IgnoreProxied ignores proxied field differences
	IgnoreProxied bool
}

// NewDiffer creates a new differ for a zone.
func NewDiffer(zone string) *Differ {
	return &Differ{Zone: zone}
}

// Diff computes the changes needed to reach the desired state.
func (d *Differ) Diff(desired, current []Record) *Plan {
	plan := &Plan{
		Zone:    d.Zone,
		Changes: []RecordChange{},
	}

	// Build maps for efficient lookup
	desiredMap := d.buildRecordMap(desired)
	currentMap := d.buildRecordMap(current)

	// Find records to create or update
	for key, desiredRecord := range desiredMap {
		currentRecord, exists := currentMap[key]
		if !exists {
			// Record doesn't exist - create
			plan.Changes = append(plan.Changes, RecordChange{
				Type:   ChangeTypeCreate,
				Record: desiredRecord,
			})
		} else {
			// Record exists - check if update needed
			diff := d.compareRecords(desiredRecord, currentRecord)
			if len(diff) > 0 {
				// Preserve the ID from current record
				recordWithID := *desiredRecord
				recordWithID.ID = currentRecord.ID
				plan.Changes = append(plan.Changes, RecordChange{
					Type:    ChangeTypeUpdate,
					Record:  &recordWithID,
					Current: currentRecord,
					Diff:    diff,
				})
			} else {
				// No changes needed
				plan.Changes = append(plan.Changes, RecordChange{
					Type:    ChangeTypeNoop,
					Record:  desiredRecord,
					Current: currentRecord,
				})
			}
		}
	}

	// Find records to delete
	for key, currentRecord := range currentMap {
		if _, exists := desiredMap[key]; !exists {
			plan.Changes = append(plan.Changes, RecordChange{
				Type:    ChangeTypeDelete,
				Record:  currentRecord,
				Current: currentRecord,
			})
		}
	}

	// Sort changes for deterministic output
	d.sortChanges(plan.Changes)

	return plan
}

// buildRecordMap creates a map of records keyed by their unique identifier.
func (d *Differ) buildRecordMap(records []Record) map[string]*Record {
	result := make(map[string]*Record)
	for i := range records {
		record := records[i].Normalize(d.Zone)
		key := record.Key()
		result[key] = record
	}
	return result
}

// compareRecords compares two records and returns the differences.
func (d *Differ) compareRecords(desired, current *Record) map[string]FieldDiff {
	diff := make(map[string]FieldDiff)

	// Compare TTL
	if !d.IgnoreTTL && desired.TTL != current.TTL {
		diff["ttl"] = FieldDiff{Old: current.TTL, New: desired.TTL}
	}

	// Compare priority (for MX, SRV)
	if desired.Priority != current.Priority {
		diff["priority"] = FieldDiff{Old: current.Priority, New: desired.Priority}
	}

	// Compare weight (for SRV)
	if desired.Weight != current.Weight {
		diff["weight"] = FieldDiff{Old: current.Weight, New: desired.Weight}
	}

	// Compare port (for SRV)
	if desired.Port != current.Port {
		diff["port"] = FieldDiff{Old: current.Port, New: desired.Port}
	}

	// Compare proxied (if not ignoring)
	if !d.IgnoreProxied && desired.Proxied != nil && current.Proxied != nil {
		if *desired.Proxied != *current.Proxied {
			diff["proxied"] = FieldDiff{Old: *current.Proxied, New: *desired.Proxied}
		}
	}

	return diff
}

// sortChanges sorts changes for deterministic output.
func (d *Differ) sortChanges(changes []RecordChange) {
	sort.Slice(changes, func(i, j int) bool {
		// Sort by type first (create, update, noop, delete)
		typeOrder := map[ChangeType]int{
			ChangeTypeCreate: 0,
			ChangeTypeUpdate: 1,
			ChangeTypeNoop:   2,
			ChangeTypeDelete: 3,
		}
		if typeOrder[changes[i].Type] != typeOrder[changes[j].Type] {
			return typeOrder[changes[i].Type] < typeOrder[changes[j].Type]
		}

		// Then by record type
		if changes[i].Record.Type != changes[j].Record.Type {
			return changes[i].Record.Type < changes[j].Record.Type
		}

		// Then by name
		return changes[i].Record.Name < changes[j].Record.Name
	})
}

// FormatPlan formats a plan as human-readable text.
func FormatPlan(plan *Plan) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Zone: %s\n", plan.Zone))
	sb.WriteString(fmt.Sprintf("Changes: %s\n\n", plan.Summary().String()))

	for _, change := range plan.Changes {
		switch change.Type {
		case ChangeTypeCreate:
			sb.WriteString(fmt.Sprintf("+ %s %s %s (TTL: %d)\n",
				change.Record.Type, change.Record.Name, change.Record.Value, change.Record.TTL))

		case ChangeTypeUpdate:
			sb.WriteString(fmt.Sprintf("~ %s %s %s\n",
				change.Record.Type, change.Record.Name, change.Record.Value))
			for field, diff := range change.Diff {
				sb.WriteString(fmt.Sprintf("    %s: %v -> %v\n", field, diff.Old, diff.New))
			}

		case ChangeTypeDelete:
			sb.WriteString(fmt.Sprintf("- %s %s %s\n",
				change.Record.Type, change.Record.Name, change.Record.Value))

		case ChangeTypeNoop:
			// Don't show unchanged records by default
		}
	}

	return sb.String()
}

// FormatPlanVerbose formats a plan including unchanged records.
func FormatPlanVerbose(plan *Plan) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Zone: %s\n", plan.Zone))
	sb.WriteString(fmt.Sprintf("Changes: %s\n\n", plan.Summary().String()))

	for _, change := range plan.Changes {
		switch change.Type {
		case ChangeTypeCreate:
			sb.WriteString(fmt.Sprintf("+ %s %s %s (TTL: %d)\n",
				change.Record.Type, change.Record.Name, change.Record.Value, change.Record.TTL))

		case ChangeTypeUpdate:
			sb.WriteString(fmt.Sprintf("~ %s %s %s\n",
				change.Record.Type, change.Record.Name, change.Record.Value))
			for field, diff := range change.Diff {
				sb.WriteString(fmt.Sprintf("    %s: %v -> %v\n", field, diff.Old, diff.New))
			}

		case ChangeTypeDelete:
			sb.WriteString(fmt.Sprintf("- %s %s %s\n",
				change.Record.Type, change.Record.Name, change.Record.Value))

		case ChangeTypeNoop:
			sb.WriteString(fmt.Sprintf("  %s %s %s (unchanged)\n",
				change.Record.Type, change.Record.Name, change.Record.Value))
		}
	}

	return sb.String()
}
