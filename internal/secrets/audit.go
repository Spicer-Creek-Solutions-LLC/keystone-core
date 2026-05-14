package secrets

import (
	"context"
	"log/slog"
	"time"
)

// Action enumerates the broker-level operations every [Broker] method
// records in its [SecretAccessEvent]. String values are stable wire
// constants — Epic 12's audit-log filter UI and the `kscore-secrets
// audit` CLI grep these.
const (
	ActionGetSecret          = "get_secret"
	ActionWriteSecret        = "write_secret"
	ActionListSecrets        = "list_secrets"
	ActionDeleteSecret       = "delete_secret"
	ActionIssueDynamicSecret = "issue_dynamic_secret"
	ActionRenewLease         = "renew_lease"
	ActionRevokeLease        = "revoke_lease"
)

// CacheBackendLabel is the value [SecretAccessEvent.Backend] carries
// when a [GetSecret] call was served from the cache rather than a
// real backend. Constant so the audit query layer (Epic 12) has a
// known token to filter on.
const CacheBackendLabel = "cache"

// Principal is the caller-attribution view the broker stamps onto
// every audit event. Populated by [BrokerConfig.ExtractPrincipal]
// (the gRPC service wires `pkg/api/auth.PrincipalFromContext` in
// task 9; the default is a zero-value extractor so the package stays
// independent of the API auth boundary).
//
// AgentID is the §4.6 agent identifier (e.g. `agent-1`); SPIFFEID is
// the canonical SPIFFE URI from mTLS peer extraction (Epic 09 task
// 13); User is the human operator when the call came in via API key
// or JWT instead of mTLS. Empty fields are normal — operator calls
// have no AgentID/SPIFFEID; in-cluster agents have no User.
type Principal struct {
	AgentID  string `json:"agent_id,omitempty"`
	SPIFFEID string `json:"spiffe_id,omitempty"`
	User     string `json:"user,omitempty"`
}

// SecretAccessEvent is the audit envelope emitted on every broker
// operation per PROJECT-DETAILS §4.11. Epic 12's `AuditStore` (and
// the audit-mode policy engine) consume these.
//
// Action is one of the `Action*` constants. Path is the requested
// secret path (or the lease ID, for lease ops — Path is empty in
// that case and LeaseID is set). Backend is the matched backend
// name (or [CacheBackendLabel] when a [GetSecret] hit the cache).
//
// Allowed reports whether the dispatch succeeded — `false` covers
// every failure mode: validation rejection, capability refusal,
// backend error, ctx cancellation. ErrorReason carries a redacted
// summary safe for an audit row (the broker uses `err.Error()` from
// the wrapped sentinel; backend-specific cleartext never enters here
// because backend methods wrap errors above the sensitive layer).
//
// Duration is the wall-clock time from "dispatch about to start" to
// "result returned" — cache-hit durations are near-zero by
// construction and round-trip times to Vault dominate the rest.
//
// MaskedPayload is the [Secret.MaskForLog] result for write / issue
// ops where the data shape (key names) is operationally useful but
// the bytes themselves must never appear. Nil on read / list /
// delete / lease ops where there's no payload to mask.
type SecretAccessEvent struct {
	Timestamp     time.Time      `json:"timestamp"`
	Action        string         `json:"action"`
	Path          string         `json:"path,omitempty"`
	LeaseID       string         `json:"lease_id,omitempty"`
	Backend       string         `json:"backend,omitempty"`
	Principal     Principal      `json:"principal"`
	Allowed       bool           `json:"allowed"`
	ErrorReason   string         `json:"error_reason,omitempty"`
	Duration      time.Duration  `json:"duration"`
	MaskedPayload map[string]any `json:"masked_payload,omitempty"`
}

// Auditor is the seam the [Broker] writes audit events to. Epic 12's
// `AuditStore` will satisfy it; until then [LogAuditor] is the
// production-acceptable fallback (operators get the events in their
// log pipeline) and [NoopAuditor] is the test default.
//
// Emit MUST NOT block — the broker calls it inline on every
// operation. Implementations that need durability (the SQLite store
// in Epic 12) hand off to a background writer; the contract here is
// "fire and forget; never error back to the caller." A dropped
// event is a bug in the auditor implementation, not in the broker.
type Auditor interface {
	Emit(ctx context.Context, event SecretAccessEvent)
}

// NoopAuditor discards every event. Default when [BrokerConfig.Auditor]
// is nil. Tests assert behavior through fake auditors that record
// events into a slice.
type NoopAuditor struct{}

// Emit discards the event.
func (NoopAuditor) Emit(context.Context, SecretAccessEvent) {}

// Compile-time assertion that NoopAuditor implements [Auditor].
var _ Auditor = NoopAuditor{}

// LogAuditor writes audit events to a [log/slog.Logger] at INFO
// level. The default fallback when no real audit store is wired —
// operators still get the events in their log pipeline.
//
// Cleartext never reaches the logger. [SecretAccessEvent.MaskedPayload]
// is already passed through [Secret.MaskForLog] by the broker before
// emission, and [LogAuditor.Emit] flattens it through slog's
// structured-attr API without further serialisation, so a misbehaving
// log encoder cannot accidentally stringify cleartext.
type LogAuditor struct {
	Logger *slog.Logger
}

// NewLogAuditor returns a [LogAuditor] wrapping logger. A nil logger
// is replaced by [slog.Default] so the type is always usable.
func NewLogAuditor(logger *slog.Logger) *LogAuditor {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogAuditor{Logger: logger}
}

// Emit writes one INFO line per event. Attributes mirror
// [SecretAccessEvent]; MaskedPayload is logged as the masked map so
// operators see the *shape* of the secret without the bytes.
func (a *LogAuditor) Emit(ctx context.Context, event SecretAccessEvent) {
	attrs := []slog.Attr{
		slog.Time("ts", event.Timestamp),
		slog.String("action", event.Action),
		slog.String("backend", event.Backend),
		slog.Bool("allowed", event.Allowed),
		slog.Duration("duration", event.Duration),
		slog.String("agent_id", event.Principal.AgentID),
		slog.String("spiffe_id", event.Principal.SPIFFEID),
		slog.String("user", event.Principal.User),
	}
	if event.Path != "" {
		attrs = append(attrs, slog.String("path", event.Path))
	}
	if event.LeaseID != "" {
		attrs = append(attrs, slog.String("lease_id", event.LeaseID))
	}
	if event.ErrorReason != "" {
		attrs = append(attrs, slog.String("error", event.ErrorReason))
	}
	if event.MaskedPayload != nil {
		attrs = append(attrs, slog.Any("masked_payload", event.MaskedPayload))
	}
	a.Logger.LogAttrs(ctx, slog.LevelInfo, "secret.access", attrs...)
}

// Compile-time assertion that *LogAuditor implements [Auditor].
var _ Auditor = (*LogAuditor)(nil)

// DefaultAuditor returns the canonical [NoopAuditor]. Provided so
// [BrokerConfig] zero values resolve to a known no-op without a
// per-broker allocation.
func DefaultAuditor() Auditor { return NoopAuditor{} }
