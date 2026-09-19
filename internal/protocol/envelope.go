package protocol

import (
	"encoding/binary"
	"time"
)

// Version is the only protocol version this build knows. ADR-0005 § 2: an
// unknown version is refused, never best-effort parsed, because a receiver that
// tries to make sense of a version it does not know is a receiver an attacker
// can teach.
const Version uint32 = 1

// Sizes ADR-0005 fixes for a v1 envelope's typed fields.
const (
	VersionBytes   = 4  // § 2, uint32 big-endian
	TimestampBytes = 8  // § 6, int64 big-endian Unix milliseconds UTC
	NonceBytes     = 16 // § 6, from a cryptographic random source
)

// Envelope is a v1 envelope's nine fields, decoded.
//
// Field 8 is opaque here and stays that way: whether it is ciphertext or
// cleartext is § 5's business per class, and a payload's schema belongs to the
// task that owns the message, not to this layer.
type Envelope struct {
	Version       uint32
	Class         Class
	JobID         string
	CorrelationID string
	Sender        string
	Timestamp     time.Time
	Nonce         []byte
	Payload       []byte
	Signature     []byte
}

// fieldValues renders an envelope as the nine byte strings § 1 frames.
func (e Envelope) fieldValues() [][]byte {
	version := make([]byte, VersionBytes)
	binary.BigEndian.PutUint32(version, e.Version)
	ts := make([]byte, TimestampBytes)
	binary.BigEndian.PutUint64(ts, uint64(e.Timestamp.UTC().UnixMilli()))
	return [][]byte{
		version,
		[]byte(e.Class),
		[]byte(e.JobID),
		[]byte(e.CorrelationID),
		[]byte(e.Sender),
		ts,
		e.Nonce,
		e.Payload,
		e.Signature,
	}
}

// SignedInput returns the bytes fields 1 to 8 present to the signature.
func (e Envelope) SignedInput() ([]byte, bool) { return SignedInput(e.fieldValues()) }

// Encode frames a complete envelope.
func (e Envelope) Encode() ([]byte, bool) { return Frame(e.fieldValues()) }

// Decode parses framed bytes into an envelope, refusing anything that is not a
// well-formed v1 envelope.
//
// The version is checked before the field shapes, because "a known version
// whose envelope does not match that version's field sequence" (§ 9) is only a
// meaningful statement once the version is known to be one this build parses.
func Decode(b []byte, maxEnvelope int) (Envelope, error) {
	fields, err := Split(b, maxEnvelope)
	if err != nil {
		return Envelope{}, err
	}
	if len(fields[0]) != VersionBytes {
		return Envelope{}, MalformedEnvelope
	}
	v := binary.BigEndian.Uint32(fields[0])
	if v != Version {
		return Envelope{}, UnknownVersion
	}

	class := Class(fields[1])
	if !Known(class) {
		return Envelope{}, UnknownClass
	}

	// The v1 field sequence. A known version whose envelope does not match it
	// is refused rather than interpreted.
	if len(fields[5]) != TimestampBytes || len(fields[6]) != NonceBytes {
		return Envelope{}, MalformedEnvelope
	}
	// Field 9 is fixed at SignatureBytes for v1 (ADR-0005 § 4 as amended at
	// G37). A different length is not a signature that fails to verify; it is
	// an envelope that does not match this version's field sequence, which § 9
	// requires a receiver to refuse.
	if len(fields[8]) != SignatureBytes {
		return Envelope{}, MalformedEnvelope
	}
	for _, id := range [][]byte{fields[2], fields[3]} {
		if len(id) != 0 && !ValidIdentifier(string(id)) {
			return Envelope{}, MalformedEnvelope
		}
	}
	sender := string(fields[4])
	if !ValidIdentifier(sender) && !ReservedSender(sender) {
		return Envelope{}, MalformedEnvelope
	}
	// Field 4 is EMPTY on encrypted classes (ADR-0005 § 1 and § 3): the
	// correlation identifier travels inside the sealed payload, because it is
	// what groups an operator's several actions and § 3 exists so an observer
	// cannot link them. § 7's accepted leakage for a command names the job
	// identifier in cleartext and NOT this one, so a non-empty field 4 here
	// leaks beyond what the class permits -- which makes it part of the version's
	// field sequence rather than a caller's business.
	if Encrypted(class) && len(fields[3]) != 0 {
		return Envelope{}, MalformedEnvelope
	}

	return Envelope{
		Version:       v,
		Class:         class,
		JobID:         string(fields[2]),
		CorrelationID: string(fields[3]),
		Sender:        sender,
		Timestamp:     time.UnixMilli(int64(binary.BigEndian.Uint64(fields[5]))).UTC(),
		Nonce:         fields[6],
		Payload:       fields[7],
		Signature:     fields[8],
	}, nil
}

// HeaderAgrees reports whether § 8's class header matches the signed field 2.
//
// ADR-0005 § 7 as amended at G38 requires a receiver to refuse an envelope
// whose header disagrees, and states the limit this function inherits: field 2
// is signed and the header is NOT, so this is not an authenticity control. The
// signature already decides the class. It exists so that a broker-side consumer
// routing on the header and a receiver acting on the signed field can never act
// on different classes for the same message. Field 2 is authoritative.
func HeaderAgrees(e Envelope, header string) bool { return string(e.Class) == header }
