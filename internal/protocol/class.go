package protocol

// Class is a message class of ADR-0005 § 7, and its value is the canonical
// ASCII token G38 assigned it. Field 2 carries these bytes, and so does § 8's
// class header -- the same bytes in both, which is what lets a receiver check
// that a broker-side router and itself agree about what a message is.
type Class string

const (
	ClassEnrollmentRequest Class = "enrollment-request"
	ClassEnrollmentReply   Class = "enrollment-reply"
	ClassCommand           Class = "command"
	ClassCancellation      Class = "cancellation"
	ClassResult            Class = "result"
	ClassLifecycleEvent    Class = "lifecycle-event"
	ClassPresence          Class = "presence"
)

// classes is the closed set. A token outside it is UnknownClass, which is why
// § 9 carries that code and why this is a set rather than a parse.
var classes = map[Class]bool{
	ClassEnrollmentRequest: true,
	ClassEnrollmentReply:   true,
	ClassCommand:           true,
	ClassCancellation:      true,
	ClassResult:            true,
	ClassLifecycleEvent:    true,
	ClassPresence:          true,
}

// Known reports whether c is one of the seven.
func Known(c Class) bool { return classes[c] }

// Encrypted reports whether a class has an encryption recipient (ADR-0005 § 5).
// Four of the seven do not: their field 8 is cleartext, signed but not sealed.
func Encrypted(c Class) bool {
	switch c {
	case ClassCommand, ClassCancellation, ClassResult:
		return true
	default:
		return false
	}
}
