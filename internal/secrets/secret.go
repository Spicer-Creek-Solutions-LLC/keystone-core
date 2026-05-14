package secrets

import (
	"time"
)

// MaskedValue is the literal that [Secret.MaskForLog] writes in place
// of every leaf value in [Secret.Data]. Constant so audit consumers
// and tests grep for the same string.
const MaskedValue = "***"

// Secret is the value type every [SecretBackend] returns from
// [SecretBackend.GetSecret] / [SecretBackend.WriteSecret] /
// [SecretBackend.IssueDynamicSecret], and that the broker hands back
// to [pkg/api/secrets] / [pkg/secrets] consumers.
//
// Path is the canonical store path — slash-separated, no scheme, no
// leading slash. The broker's path-prefix router (task 2) operates on
// this field.
//
// Data carries the payload. Backends serialise it however they like
// (the encrypted-file backend uses JSON; Vault KV returns whatever the
// underlying engine has stored). Values are `any` so structured
// secrets (TLS keypairs, multi-field DB credentials) survive round
// trips without flattening to strings.
//
// Metadata is operator-visible info — labels, owner, version notes —
// never cleartext secret material. Backends MAY expose metadata in
// list responses; cleartext Data is never listed.
//
// Version is the backend-native version number for stores that
// support it (Vault KV v2; encrypted-file with WAL is task 4's call).
// 0 means "unversioned / not applicable."
//
// LeaseID + LeaseDuration + Renewable + LeaseRenewable are populated
// for dynamic secrets only. [Secret.IsDynamic] is the canonical check.
// For static KV reads they are zero.
//
// CreatedAt / UpdatedAt are wall-clock timestamps in UTC; backends
// that don't track one or both leave them zero.
type Secret struct {
	Path          string         `json:"path"`
	Data          map[string]any `json:"data,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Version       uint64         `json:"version,omitempty"`
	LeaseID       string         `json:"lease_id,omitempty"`
	LeaseDuration time.Duration  `json:"lease_duration,omitempty"`
	Renewable     bool           `json:"renewable,omitempty"`
	CreatedAt     time.Time      `json:"created_at,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at,omitempty"`
}

// IsDynamic reports whether this secret carries a lease — i.e. it was
// issued by [SecretBackend.IssueDynamicSecret] (or by a Vault dynamic
// engine) and the lease manager (task 6) needs to track it.
func (s Secret) IsDynamic() bool {
	return s.LeaseID != ""
}

// MaskForLog returns a copy of the secret with every value in Data
// replaced by [MaskedValue]. The structure (keys, nested-map shape)
// is preserved so a malformed payload still stands out in the audit
// trail. Metadata is preserved verbatim — it's operator-visible by
// contract. The original receiver is never mutated.
//
// Nested maps are masked recursively. Slices of `any` (a common shape
// for "list of credentials") are walked element-wise; primitive leaves
// become [MaskedValue]. Any other type — including structs — is
// masked at the leaf so a backend that hands back a custom type
// doesn't slip past the masker.
func (s Secret) MaskForLog() Secret {
	out := s
	out.Data = maskMap(s.Data)
	out.Metadata = copyStringMap(s.Metadata)
	return out
}

// Clone returns a deep copy of the secret. Maps (Data, Metadata) are
// duplicated so the caller can mutate the clone without disturbing
// the cached / backend-owned original. Used by the cache (task 8) and
// by every gRPC handler that needs to hand a copy to a request-scoped
// caller. Slice and primitive leaves are duplicated.
func (s Secret) Clone() Secret {
	out := s
	out.Data = cloneAnyMap(s.Data)
	out.Metadata = copyStringMap(s.Metadata)
	return out
}

// maskMap walks the input and returns a structurally-identical map
// whose primitive leaves are replaced by [MaskedValue]. Nested maps
// and `[]any` slices are recursed. Nil input round-trips to nil so
// the caller can distinguish "no data" from "data was masked."
func maskMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = maskValue(v)
	}
	return out
}

func maskValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return maskMap(t)
	case []any:
		masked := make([]any, len(t))
		for i, elem := range t {
			masked[i] = maskValue(elem)
		}
		return masked
	default:
		return MaskedValue
	}
}

// cloneAnyMap performs a structural deep copy of a `map[string]any`.
// Nested maps and `[]any` slices are duplicated; primitives and
// anything else are copied by value (which is correct for strings,
// numbers, bools — and acceptable for backend-owned interface values
// since callers should treat them as immutable once handed to them).
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

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
