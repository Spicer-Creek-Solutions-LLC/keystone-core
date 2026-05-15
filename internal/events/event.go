package events

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// EventType is the typed wire form of the §4.9 event type. The
// canonical shape is `<category>.<subtype>` where the category half
// is a member of [KnownCategories] and the subtype half is free-form
// non-empty (and may itself contain further dots — e.g.
// `state.apply.start` has category `state` and subtype `apply.start`).
//
// Subtypes are deliberately not closed: operators and v1.4 plugins
// (Epic 14) extend the taxonomy by emitting new subtypes within a
// known category without modifying core. [IsCanonical] reports
// whether a value matches one of the 22 v1.0 documented spellings;
// downstream routing should switch on [EventType.Category] (closed)
// rather than full-string match (open) wherever the dispatch is
// meant to be exhaustive.
type EventType string

// String returns the underlying string. Distinct method for log /
// format symmetry with other typed strings in the package.
func (t EventType) String() string {
	return string(t)
}

// ParseEventType validates `<category>.<subtype>` shape, that the
// category half is a member of [KnownCategories], and that the
// subtype half is non-empty and contains no whitespace. The split
// is on the FIRST `.` — multi-segment subtypes such as
// `state.apply.start` parse as category `state` + subtype
// `apply.start`. Errors wrap [ErrInvalidEvent].
func ParseEventType(s string) (EventType, error) {
	if s == "" {
		return "", fmt.Errorf("%w: event type is empty", ErrInvalidEvent)
	}
	idx := strings.IndexByte(s, '.')
	if idx <= 0 || idx == len(s)-1 {
		return "", fmt.Errorf("%w: event type %q must have shape <category>.<subtype>", ErrInvalidEvent, s)
	}
	category := Category(s[:idx])
	subtype := s[idx+1:]
	if !category.IsKnown() {
		return "", fmt.Errorf("%w: unknown category %q in event type %q (known: %v)", ErrInvalidEvent, string(category), s, KnownCategories())
	}
	for _, r := range subtype {
		if unicode.IsSpace(r) {
			return "", fmt.Errorf("%w: subtype %q in event type %q contains whitespace", ErrInvalidEvent, subtype, s)
		}
	}
	return EventType(s), nil
}

// Category returns the category half of the event type, or the
// empty [Category] if the receiver is not a well-formed event type.
// Cheap accessor — call sites that need a guarantee should run
// [ParseEventType] first.
func (t EventType) Category() Category {
	s := string(t)
	idx := strings.IndexByte(s, '.')
	if idx <= 0 {
		return ""
	}
	return Category(s[:idx])
}

// Subtype returns the subtype half of the event type (everything
// after the first dot), or the empty string if the receiver is not
// a well-formed event type. Multi-dot subtypes are preserved verbatim
// — `EventType("state.apply.start").Subtype() == "apply.start"`.
func (t EventType) Subtype() string {
	s := string(t)
	idx := strings.IndexByte(s, '.')
	if idx < 0 || idx == len(s)-1 {
		return ""
	}
	return s[idx+1:]
}

// IsValid reports whether the receiver round-trips through
// [ParseEventType] without error. Used by [Event.Validate].
func (t EventType) IsValid() bool {
	_, err := ParseEventType(string(t))
	return err == nil
}

// Event is the v1.0 record shape from PROJECT-DETAILS §4.9. Field
// order matches the spec exactly; do not reorder without an RFC.
//
// ID is a UUID stamped by [NewEvent] via [uuid.NewV7] — Unix-time-
// prefixed so events sort lexicographically by stamping time,
// giving SQL indexes (task 2) and NATS replay (task 4) better
// locality than purely-random v4 IDs. Operators stamping their own
// IDs MAY use any RFC 4122 / 9562 value; [Event.Validate] checks
// non-emptiness, not the version.
//
// Type carries the closed-category / open-subtype taxonomy
// described on [EventType].
//
// Source is operator-supplied free-form text identifying the
// emitting component — typically a hostname, an agent ID, or a
// service name (`server-1`, `agent-prod-3`, `state-runner`).
// Non-empty is the only structural rule.
//
// Time is the wall-clock instant the event happened, in UTC.
// [NewEvent] stamps `time.Now().UTC()`. Operators that emit
// historical events MAY supply an explicit value.
//
// Severity is the §4.9 level (debug / info / warn / error /
// critical). Zero value is rejected by [Event.Validate];
// [NewEvent] stamps [SeverityInfo].
//
// CorrelationID is the request-scoped trace correlator carried in
// gRPC contexts and NATS message headers (epic 05's correlation
// scheme); empty when the event is not tied to a particular
// request.
//
// Tags is the indexable label map — values stay short and
// cardinality-bounded (think Prometheus labels: `role`, `env`,
// `cluster`). Indexed by the SQL store (task 2); usable in CEL
// filters as `tags.<key>` (task 5).
//
// Data is the typed payload map — arbitrary structured detail
// about the event. NOT indexed by default; usable in CEL filters
// via nested JSON path (`data.foo.bar`).
//
// Subject is the NATS subject the event was (or will be)
// published on — `kscore.<cluster>.events.<category>.<subtype>`.
// Empty at [NewEvent] time because the cluster name is operator
// config, not construction-site context; the publisher (task 3)
// stamps it via [Event.StampSubject] at emit time.
type Event struct {
	ID            string            `json:"id"`
	Type          EventType         `json:"type"`
	Source        string            `json:"source"`
	Time          time.Time         `json:"time"`
	Severity      Severity          `json:"severity"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	Data          map[string]any    `json:"data,omitempty"`
	Subject       string            `json:"subject,omitempty"`
}

// NewEvent constructs an Event with a freshly-stamped UUIDv7 ID,
// the current UTC time, and [SeverityInfo] as the default level.
// Tags and Data are left nil; callers populate them before emit.
// Subject is left empty for the publisher to stamp via
// [Event.StampSubject].
//
// Caller-supplied typ is validated via [ParseEventType] and
// rejected with a wrapped [ErrInvalidEvent] error if malformed.
// Source must be non-empty.
func NewEvent(typ EventType, source string) (Event, error) {
	if _, err := ParseEventType(string(typ)); err != nil {
		return Event{}, err
	}
	if source == "" {
		return Event{}, fmt.Errorf("%w: source is required", ErrInvalidEvent)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Event{}, fmt.Errorf("%w: uuidv7: %v", ErrInvalidEvent, err)
	}
	return Event{
		ID:       id.String(),
		Type:     typ,
		Source:   source,
		Time:     time.Now().UTC(),
		Severity: SeverityInfo,
	}, nil
}

// MustNewEvent is the panic-on-error sibling of [NewEvent]. Test-
// only — production code should always handle the error.
func MustNewEvent(typ EventType, source string) Event {
	e, err := NewEvent(typ, source)
	if err != nil {
		panic(err)
	}
	return e
}

// IsZero reports whether the receiver is the uninitialised value.
// A successful [NewEvent] never returns a zero value. Used by
// the SQL store (task 2) and the gRPC handlers (task 6) as the
// "nothing here" sentinel when an error path also returns an
// Event.
func (e Event) IsZero() bool {
	return e.ID == "" &&
		e.Type == "" &&
		e.Source == "" &&
		e.Time.IsZero() &&
		e.Severity == SeverityUnknown &&
		e.CorrelationID == "" &&
		e.Tags == nil &&
		e.Data == nil &&
		e.Subject == ""
}

// Validate enforces the structural invariants every later task
// depends on: non-empty ID / Source / Type, well-formed
// [EventType], non-zero Time, valid [Severity]. Tags / Data /
// CorrelationID / Subject are optional. Errors wrap
// [ErrInvalidEvent].
func (e Event) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidEvent)
	}
	if e.Source == "" {
		return fmt.Errorf("%w: source is required", ErrInvalidEvent)
	}
	if _, err := ParseEventType(string(e.Type)); err != nil {
		return err
	}
	if e.Time.IsZero() {
		return fmt.Errorf("%w: time is zero", ErrInvalidEvent)
	}
	if !e.Severity.IsValid() {
		return fmt.Errorf("%w: severity %s is not a valid emission level", ErrInvalidEvent, e.Severity)
	}
	return nil
}

// Clone returns a deep copy of the receiver. Maps (Tags, Data) are
// duplicated so the caller can mutate the clone without disturbing
// the original. Used by the store (task 2) when handing cached
// records to request-scoped callers, and by the subscriber (task 4)
// before passing an event to user handlers. Primitive leaves in
// Data are copied by value; nested `map[string]any` and `[]any`
// payloads are duplicated recursively.
func (e Event) Clone() Event {
	out := e
	out.Tags = cloneStringMap(e.Tags)
	out.Data = cloneAnyMap(e.Data)
	return out
}

// StampSubject sets [Event.Subject] to the canonical NATS subject
// for the receiver's [EventType] in the given cluster. Returns the
// new value (and assigns it on the receiver) so the publisher
// (task 3) can chain `evt.StampSubject(cluster)` in its hot path.
// Empty cluster is rejected with a wrapped [ErrInvalidEvent].
func (e *Event) StampSubject(clusterName string) (string, error) {
	subj, err := SubjectFor(clusterName, e.Type)
	if err != nil {
		return "", err
	}
	e.Subject = subj
	return subj, nil
}

// cloneStringMap is the Tags-side deep copy helper. Nil round-trips
// to nil so callers can distinguish "no tags" from "empty tags."
func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// cloneAnyMap is the Data-side deep copy helper. Recurses through
// nested `map[string]any` and `[]any` values; primitives are
// duplicated by value. Mirrors the helper in `internal/secrets`
// rather than sharing one to keep packages independent.
func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneAnyValue(v)
	}
	return out
}

func cloneAnyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneAnyMap(t)
	case []any:
		dup := make([]any, len(t))
		for i, elem := range t {
			dup[i] = cloneAnyValue(elem)
		}
		return dup
	default:
		return v
	}
}
