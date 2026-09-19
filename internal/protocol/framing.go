package protocol

import "encoding/binary"

// The framing of ADR-0005 § 1: a fixed sequence of nine length-prefixed fields,
// each prefix a uint32 big-endian immediately before its bytes, on all nine
// including the signature.
const (
	Fields      = 9
	PrefixBytes = 4

	// DefaultMaxEnvelope is ADR-0002 § 9's account payload cap. ADR-0005 § 1
	// deliberately points at that value rather than copying it, so this is the
	// one place the number appears in the module and Split takes it as an
	// argument rather than reading it -- a parser that hardcoded policy would
	// be a second copy to drift from the broker configuration C03 renders.
	DefaultMaxEnvelope = 1 << 20
)

// Frame concatenates nine field values with their prefixes. It is deliberately
// lower level than an envelope: it knows the prefix rule and nothing about what
// any field means.
//
// That split is what `AC-2` tests against. The hand-written vectors C01-A froze
// carry arbitrary opaque values -- a one-byte "version", a four-byte "timestamp"
// -- because they were written to demonstrate § 1's prefix and concatenation
// rule, which is all § 1 decided at the time. They are valid framings of
// arbitrary values, and the field ENCODINGS G37 and G38 later specified sit a
// layer above.
func Frame(fields [][]byte) ([]byte, bool) {
	if len(fields) != Fields {
		return nil, false
	}
	n := Fields * PrefixBytes
	for _, f := range fields {
		if uint64(len(f)) > uint64(^uint32(0)) {
			return nil, false
		}
		n += len(f)
	}
	out := make([]byte, 0, n)
	var prefix [PrefixBytes]byte
	for _, f := range fields {
		binary.BigEndian.PutUint32(prefix[:], uint32(len(f)))
		out = append(out, prefix[:]...)
		out = append(out, f...)
	}
	return out, true
}

// SignedInput is the byte string fields 1 to 8 present to the signature: their
// framed bytes, PREFIXES INCLUDED.
//
// ADR-0005 § 1 states why that matters, and it is not a detail. Sign the values
// alone and "AB"+"CD" and "A"+"BCD" are the same signed bytes, so an attacker
// moves a byte across a field boundary and the signature still verifies.
func SignedInput(fields [][]byte) ([]byte, bool) {
	if len(fields) != Fields {
		return nil, false
	}
	framed, ok := Frame(fields)
	if !ok {
		return nil, false
	}
	// Everything before field 9's prefix.
	n := 0
	for _, f := range fields[:Fields-1] {
		n += PrefixBytes + len(f)
	}
	return framed[:n], true
}

// Split parses framed bytes back into nine field values under a running budget.
//
// The order of checks is normative (ADR-0005 § 1), because the refusal code
// depends on it: § 9's set is coarse so a sender cannot learn where parsing
// stopped, and two receivers checking in different orders would return
// different codes for the same envelope -- leaking through the difference
// exactly what the coarseness protects.
//
// Per field: four bytes must remain for the prefix, else MalformedEnvelope; the
// declared length must be within the remaining budget, else PayloadTooLarge;
// the declared bytes must be present, else MalformedEnvelope. After field 9,
// any remaining bytes are MalformedEnvelope.
//
// The budget check PRECEDES the presence check deliberately, so an over-budget
// length is reported as too large whether or not the bytes are there -- and it
// precedes allocation, because four bytes can declare nearly 4 GiB and a parser
// that trusts a prefix it has not checked is a denial of service anyone who can
// publish can reach.
func Split(b []byte, maxEnvelope int) ([][]byte, error) {
	if maxEnvelope <= 0 {
		maxEnvelope = DefaultMaxEnvelope
	}
	budget := maxEnvelope
	fields := make([][]byte, 0, Fields)
	rest := b
	for i := 0; i < Fields; i++ {
		if len(rest) < PrefixBytes {
			return nil, MalformedEnvelope
		}
		if budget < PrefixBytes {
			return nil, PayloadTooLarge
		}
		budget -= PrefixBytes
		n := binary.BigEndian.Uint32(rest[:PrefixBytes])
		rest = rest[PrefixBytes:]

		// Budget before presence, and before any allocation.
		if uint64(n) > uint64(budget) {
			return nil, PayloadTooLarge
		}
		if uint64(len(rest)) < uint64(n) {
			return nil, MalformedEnvelope
		}
		budget -= int(n)
		fields = append(fields, rest[:n:n])
		rest = rest[n:]
	}
	if len(rest) != 0 {
		return nil, MalformedEnvelope
	}
	return fields, nil
}
