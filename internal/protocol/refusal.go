package protocol

// Refusal is one of ADR-0005 § 9's seven codes, and it carries nothing else.
//
// That is the design, not an omission. § 9's set is "deliberately coarse": a
// sender learns that its envelope was refused and roughly why, and never which
// key failed, whether an identifier exists, or where parsing stopped. `AC-10`
// forbids input-derived detail and `AC-11` requires three specified pairs of
// probing inputs to return *identical* refusals.
//
// A refusal that wrapped an underlying error, held an offset, or named a field
// would fail both -- and would fail them by degrees, which is the worst way,
// because the leak would depend on which path produced it. A bare code has
// nowhere for a difference to live, so the pairs are indistinguishable by
// construction rather than by discipline.
type Refusal uint8

const (
	UnknownVersion Refusal = iota + 1
	MalformedEnvelope
	SignatureInvalid
	DecryptionFailed
	ReplayRejected
	PayloadTooLarge
	UnknownClass
)

// The strings are fixed and derived from nothing. A message that interpolated
// any part of the input would be the detail § 9 refuses to disclose.
var refusalText = map[Refusal]string{
	UnknownVersion:    "unknown version",
	MalformedEnvelope: "malformed envelope",
	SignatureInvalid:  "signature invalid",
	DecryptionFailed:  "decryption failed",
	ReplayRejected:    "replay rejected",
	PayloadTooLarge:   "payload too large",
	UnknownClass:      "unknown class",
}

func (r Refusal) Error() string {
	if s, ok := refusalText[r]; ok {
		return s
	}
	// Unreachable through the constructors above. Returning a constant rather
	// than formatting the value keeps the no-input-derived-detail property true
	// even for a code that should not exist.
	return "malformed envelope"
}
