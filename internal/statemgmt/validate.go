package statemgmt

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// The eight requisite keys are recognized by the Validator and by
// the resolver (Task 5). They live in Declaration.Params as the
// parser left them — a slice of single-key maps `[{module: name}]`
// matching the §4.8 DSL. The "_in" variants reverse the edge
// direction (X _in: this declaration ≡ this declaration applies
// _to_ X) but the shape is identical.
const (
	ReqRequire     = "require"
	ReqRequireIn   = "require_in"
	ReqWatch       = "watch"
	ReqWatchIn     = "watch_in"
	ReqPrereq      = "prereq"
	ReqPrereqIn    = "prereq_in"
	ReqOnChanges   = "onchanges"
	ReqOnChangesIn = "onchanges_in"
)

// RequisiteKeys lists every requisite Params key the engine
// recognises in source-iteration order. The order matters for
// deterministic issue reporting (validator emits issues per key in
// this order) and matches §4.8.
var RequisiteKeys = []string{
	ReqRequire, ReqRequireIn,
	ReqWatch, ReqWatchIn,
	ReqPrereq, ReqPrereqIn,
	ReqOnChanges, ReqOnChangesIn,
}

// ValidatableModule is the optional interface a Module implements
// when it wants declaration-specific validation beyond the engine's
// generic shape / state / requisite checks. Modules without param
// constraints pay zero cost — the engine type-asserts and skips them.
type ValidatableModule interface {
	Validate(decl *Declaration) error
}

// ValidationIssue is one problem found by the Validator. DeclID is
// empty for file-level issues (e.g. duplicate IDs). Field names the
// place that produced the issue ("Module", "State", "Params.require")
// so an operator can grep for it in their YAML.
type ValidationIssue struct {
	DeclID  string
	Field   string
	Message string
}

func (i ValidationIssue) String() string {
	switch {
	case i.DeclID == "":
		return fmt.Sprintf("%s: %s", i.Field, i.Message)
	case i.Field == "":
		return fmt.Sprintf("%s: %s", i.DeclID, i.Message)
	default:
		return fmt.Sprintf("%s (%s): %s", i.DeclID, i.Field, i.Message)
	}
}

// ValidationError aggregates every issue found in a single Validate
// call. Returning all of them at once is the v1.0 operator
// affordance — state authors want to see every issue in one pass,
// not fix-rerun-fix-rerun.
type ValidationError struct {
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "statemgmt: validation failed"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "statemgmt: validation failed (%d issue", len(e.Issues))
	if len(e.Issues) != 1 {
		sb.WriteByte('s')
	}
	sb.WriteString("):")
	for _, iss := range e.Issues {
		sb.WriteString("\n  • ")
		sb.WriteString(iss.String())
	}
	return sb.String()
}

// Validator runs every shape / state / requisite check against a
// StateFile. It is stateless — multiple Validate calls on the same
// instance are safe.
type Validator struct {
	Registry *Registry
}

// NewValidator returns a Validator backed by reg. Pass nil to use
// DefaultRegistry.
func NewValidator(reg *Registry) *Validator {
	return &Validator{Registry: reg}
}

func (v *Validator) registry() *Registry {
	if v.Registry != nil {
		return v.Registry
	}
	return DefaultRegistry
}

// Validate runs every check. Returns nil on success or a
// *ValidationError listing every issue.
func (v *Validator) Validate(sf *StateFile) error {
	if sf == nil {
		return nil
	}
	reg := v.registry()

	var issues []ValidationIssue

	// Pre-pass: build the ID set and detect duplicates. The resolver
	// needs an existence-check map and so do we — share it.
	idSet := make(map[string]int, len(sf.Declarations))
	for _, d := range sf.Declarations {
		if d == nil {
			continue
		}
		idSet[d.ID]++
	}
	for id, count := range idSet {
		if count > 1 {
			issues = append(issues, ValidationIssue{
				DeclID:  id,
				Field:   "ID",
				Message: fmt.Sprintf("declaration appears %d times; IDs must be unique", count),
			})
		}
	}

	for _, d := range sf.Declarations {
		if d == nil {
			continue
		}
		issues = append(issues, v.checkDeclaration(d, idSet, reg)...)
	}

	if len(issues) == 0 {
		return nil
	}
	sortIssues(issues)
	return &ValidationError{Issues: issues}
}

func (v *Validator) checkDeclaration(d *Declaration, idSet map[string]int, reg *Registry) []ValidationIssue {
	var issues []ValidationIssue

	// 1-3: required scalar fields.
	if d.Module == "" {
		issues = append(issues, ValidationIssue{DeclID: d.ID, Field: "Module", Message: "cannot be empty"})
	}
	if d.Name == "" {
		issues = append(issues, ValidationIssue{DeclID: d.ID, Field: "Name", Message: "cannot be empty"})
	}
	if d.State == "" {
		issues = append(issues, ValidationIssue{DeclID: d.ID, Field: "State", Message: "cannot be empty"})
	}

	// 4: ID matches Module:Name.
	if d.Module != "" && d.Name != "" {
		want := d.Module + ":" + d.Name
		if d.ID != want {
			issues = append(issues, ValidationIssue{
				DeclID:  d.ID,
				Field:   "ID",
				Message: fmt.Sprintf("%q does not match %q (Module:Name)", d.ID, want),
			})
		}
	}

	// 5: module registered.
	var mod Module
	if d.Module != "" {
		m, err := reg.Get(d.Module)
		switch {
		case errors.Is(err, ErrModuleNotFound):
			issues = append(issues, ValidationIssue{
				DeclID:  d.ID,
				Field:   "Module",
				Message: fmt.Sprintf("module %q not registered", d.Module),
			})
		case err != nil:
			issues = append(issues, ValidationIssue{
				DeclID:  d.ID,
				Field:   "Module",
				Message: fmt.Sprintf("registry lookup failed: %v", err),
			})
		default:
			mod = m
		}
	}

	// 6: state in module's ValidStates set.
	if mod != nil && d.State != "" {
		valid := mod.ValidStates()
		if !contains(valid, d.State) {
			issues = append(issues, ValidationIssue{
				DeclID:  d.ID,
				Field:   "State",
				Message: fmt.Sprintf("%q not valid for module %q (valid: %s)", d.State, d.Module, strings.Join(valid, ", ")),
			})
		}
	}

	// 7+8: requisite refs — shape + existence.
	for _, key := range RequisiteKeys {
		raw, ok := d.Params[key]
		if !ok {
			continue
		}
		refs, errs := parseRequisiteList(raw)
		for _, e := range errs {
			issues = append(issues, ValidationIssue{
				DeclID:  d.ID,
				Field:   "Params." + key,
				Message: e,
			})
		}
		for _, ref := range refs {
			if _, exists := idSet[ref]; !exists {
				issues = append(issues, ValidationIssue{
					DeclID:  d.ID,
					Field:   "Params." + key,
					Message: fmt.Sprintf("reference %q not found in state file", ref),
				})
			}
		}
	}

	// 10: opt-in per-module validation. Modules see Params without
	// the engine-reserved requisite keys (those are the resolver's
	// concern, not theirs).
	if vm, ok := mod.(ValidatableModule); ok {
		if err := vm.Validate(d.moduleView()); err != nil {
			issues = append(issues, ValidationIssue{
				DeclID:  d.ID,
				Field:   "Params",
				Message: err.Error(),
			})
		}
	}

	return issues
}

// parseRequisiteList accepts the YAML-decoded shape:
//
//	[{ module1: name1 }, { module2: name2 }, ...]
//
// and returns the canonical declaration IDs ["module1:name1", ...].
// Shape errors accumulate alongside refs so the Validator can emit
// one issue per malformed entry instead of bailing at the first.
func parseRequisiteList(value any) ([]string, []string) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, []string{fmt.Sprintf("expected a list of single-key maps, got %T", value)}
	}
	var refs []string
	var errs []string
	for i, entry := range list {
		m, ok := entry.(map[string]any)
		if !ok {
			errs = append(errs, fmt.Sprintf("entry %d: expected a single-key map, got %T", i, entry))
			continue
		}
		if len(m) != 1 {
			errs = append(errs, fmt.Sprintf("entry %d: expected exactly one key, got %d", i, len(m)))
			continue
		}
		for module, name := range m {
			nameStr, ok := name.(string)
			if !ok {
				errs = append(errs, fmt.Sprintf("entry %d: value for key %q must be a string, got %T", i, module, name))
				continue
			}
			if module == "" || nameStr == "" {
				errs = append(errs, fmt.Sprintf("entry %d: module and name must be non-empty", i))
				continue
			}
			refs = append(refs, module+":"+nameStr)
		}
	}
	return refs, errs
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// sortIssues orders by (DeclID, Field, Message) so the ValidationError
// output is stable across runs — operators diffing fix attempts get
// a clean diff.
func sortIssues(is []ValidationIssue) {
	sort.SliceStable(is, func(i, j int) bool {
		if is[i].DeclID != is[j].DeclID {
			return is[i].DeclID < is[j].DeclID
		}
		if is[i].Field != is[j].Field {
			return is[i].Field < is[j].Field
		}
		return is[i].Message < is[j].Message
	})
}
