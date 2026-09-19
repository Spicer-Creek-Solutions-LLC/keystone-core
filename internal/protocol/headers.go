package protocol

import "strconv"

// The complete header set of ADR-0005 § 8. No other header is set by Keystone.
//
// A header is readable by ACT-5 by definition, so the test for whether a field
// belongs here is not "is it sensitive" but "is the broker already entitled to
// it" -- and only routing metadata is. None carries a secret, a token, or
// payload plaintext.
const (
	HeaderMsgID   = "Nats-Msg-Id"
	HeaderVersion = "Keystone-Version"
	HeaderClass   = "Keystone-Class"
	HeaderJobID   = "Keystone-Job-Id"
)

// Headers for an envelope. The version and class duplicate fields 1 and 2 so a
// broker-side consumer can route without parsing; the job identifier is here
// because an operator-facing audit record correlates on it.
//
// Only the three classes stored in a stream carry Nats-Msg-Id, and only the
// three that carry a job identifier carry its header.
func Headers(e Envelope, msgID string) map[string]string {
	h := map[string]string{
		HeaderVersion: strconv.FormatUint(uint64(e.Version), 10),
		HeaderClass:   string(e.Class),
	}
	switch e.Class {
	case ClassCommand, ClassCancellation, ClassResult:
		h[HeaderJobID] = e.JobID
		if msgID != "" {
			h[HeaderMsgID] = msgID
		}
	}
	return h
}

// CheckHeaders applies § 9's forward-compatibility rule to a received header
// set, and § 7's agreement rule as amended at G38.
//
// An UNKNOWN header is tolerated: a receiver that refused one could not be sent
// a header it does not yet use, which is the property forward compatibility
// exists to provide.
//
// A KNOWN header that disagrees with the signed field is refused. Field 2 is
// signed and the header is not, so this is not an authenticity control -- the
// signature already decides the class. It exists so a broker-side consumer
// routing on the header and a receiver acting on the signed field can never act
// on different classes for the same message.
func CheckHeaders(e Envelope, h map[string]string) error {
	if v, ok := h[HeaderVersion]; ok {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil || uint32(n) != e.Version {
			return MalformedEnvelope
		}
	}
	if c, ok := h[HeaderClass]; ok && !HeaderAgrees(e, c) {
		return MalformedEnvelope
	}
	if j, ok := h[HeaderJobID]; ok && j != e.JobID {
		return MalformedEnvelope
	}
	// Anything else is a header this build does not use, and is tolerated.
	return nil
}
