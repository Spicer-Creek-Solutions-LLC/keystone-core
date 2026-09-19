package protocol

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func valid() Envelope {
	return Envelope{
		Version:   Version,
		Class:     ClassCommand,
		JobID:     "job-01hx9yq",
		Sender:    SenderCommandPublisher,
		Timestamp: time.UnixMilli(1758240000000).UTC(),
		Nonce:     make([]byte, NonceBytes),
		Payload:   []byte("payload"),
		Signature: make([]byte, SignatureBytes),
	}
}

func TestEnvelopeRoundTripsItsFields(t *testing.T) {
	in := valid()
	b, ok := in.Encode()
	if !ok {
		t.Fatal("Encode refused a well-formed envelope")
	}
	out, err := Decode(b, DefaultMaxEnvelope)
	if err != nil {
		t.Fatalf("Decode refused: %v", err)
	}
	if out.Version != in.Version || out.Class != in.Class || out.JobID != in.JobID ||
		out.Sender != in.Sender || !out.Timestamp.Equal(in.Timestamp) {
		t.Errorf("round trip changed the envelope:\n got %+v\nwant %+v", out, in)
	}
}

// ADR-0005 § 2: an unknown version is refused, never best-effort parsed.
func TestUnknownVersionIsRefused(t *testing.T) {
	e := valid()
	e.Version = Version + 1
	b, _ := e.Encode()
	_, err := Decode(b, DefaultMaxEnvelope)
	var got Refusal
	if !errors.As(err, &got) || got != UnknownVersion {
		t.Fatalf("err = %v, want unknown version", err)
	}
}

// § 9: a KNOWN version whose envelope does not match that version's field
// sequence is refused.
func TestKnownVersionWithAWrongFieldSequenceIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		bend func(f [][]byte)
		want Refusal
	}{
		{"nonce is not 16 bytes", func(f [][]byte) { f[6] = []byte{1, 2, 3} }, MalformedEnvelope},
		{"timestamp is not 8 bytes", func(f [][]byte) { f[5] = []byte{1} }, MalformedEnvelope},
		{"version field is not 4 bytes", func(f [][]byte) { f[0] = []byte{1} }, MalformedEnvelope},
		{"job identifier breaks the grammar", func(f [][]byte) { f[2] = []byte("Job.One") }, MalformedEnvelope},
		{"sender is neither an agent nor a reserved token", func(f [][]byte) { f[4] = []byte("NOT VALID") }, MalformedEnvelope},
		{"class is outside the seven", func(f [][]byte) { f[1] = []byte("telemetry") }, UnknownClass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := valid().fieldValues()
			tc.bend(f)
			b, ok := Frame(f)
			if !ok {
				t.Fatal("Frame refused the fixture")
			}
			_, err := Decode(b, DefaultMaxEnvelope)
			var got Refusal
			if !errors.As(err, &got) {
				t.Fatalf("err = %v, want a Refusal", err)
			}
			if got != tc.want {
				t.Errorf("refusal = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAgentSenderIsAccepted(t *testing.T) {
	e := valid()
	e.Sender = "agent-7"
	b, _ := e.Encode()
	if _, err := Decode(b, DefaultMaxEnvelope); err != nil {
		t.Fatalf("an agent identifier must be a valid sender: %v", err)
	}
}

// The reservation exists because every service token is itself a valid agent
// identifier under ADR-0004's grammar.
func TestServiceTokensAreValidAgentIdentifiersAndThereforeReserved(t *testing.T) {
	for _, s := range []string{
		SenderCommandPublisher, SenderEnrollmentService, SenderResultConsumer,
		SenderPresenceConsumer, SenderMonitoringRole,
	} {
		if !ValidIdentifier(s) {
			t.Errorf("%q is not a valid identifier; the collision this guards would not exist", s)
		}
		if !ReservedSender(s) {
			t.Errorf("%q is not reserved, so an agent could be assigned it", s)
		}
	}
}

func TestHeaderMustAgreeWithFieldTwo(t *testing.T) {
	e := valid()
	if !HeaderAgrees(e, string(ClassCommand)) {
		t.Error("a matching header must agree")
	}
	if HeaderAgrees(e, string(ClassPresence)) {
		t.Error("a disagreeing header must not agree; § 7 refuses the envelope")
	}
}

func TestEmptyIdentifierFieldsAreAllowedWhereTheClassCarriesNone(t *testing.T) {
	e := valid()
	e.Class = ClassPresence
	e.JobID, e.CorrelationID = "", ""
	b, _ := e.Encode()
	if _, err := Decode(b, DefaultMaxEnvelope); err != nil {
		t.Fatalf("empty identifier fields must be accepted: %v", err)
	}
}

func TestTimestampIsUnixMilliBigEndian(t *testing.T) {
	e := valid()
	f := e.fieldValues()
	if got := int64(binary.BigEndian.Uint64(f[5])); got != 1758240000000 {
		t.Errorf("timestamp encoded as %d; want Unix milliseconds big-endian", got)
	}
}
