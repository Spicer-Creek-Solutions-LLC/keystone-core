package protocol

// Identifiers follow ADR-0004's subject-token grammar, which ADR-0005 § 3
// points at rather than restating: lowercase a-z, digits, and '-', non-empty
// and at most 64 characters. A '.' would create subject structure the supplier
// controls; '*' and '>' are NATS wildcards.
const MaxIdentifier = 64

// ValidIdentifier reports whether s is a well-formed identifier. An empty
// string is NOT valid here: emptiness is a property of a FIELD (fields 3 and 4
// are empty on classes that carry none), not of an identifier.
func ValidIdentifier(s string) bool {
	if s == "" || len(s) > MaxIdentifier {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
		default:
			return false
		}
	}
	return true
}

// Service principals, ADR-0005 § 3 as amended at G38. Field 5 carries either an
// agent's identifier or one of these.
const (
	SenderCommandPublisher  = "command-publisher"
	SenderEnrollmentService = "enrollment-service"
	SenderResultConsumer    = "result-consumer"
	SenderPresenceConsumer  = "presence-consumer"
	SenderMonitoringRole    = "monitoring-role"
)

// reservedSenders is the collision guard, and the reason it exists is worth
// keeping next to it: every token above is a VALID agent identifier under the
// grammar -- lowercase, digits, hyphens. Without a reservation an agent could be
// assigned `command-publisher` and sign as one, and nothing in the grammar would
// notice. ADR-0005 § 3 states the reservation; this is where a caller can ask.
var reservedSenders = map[string]bool{
	SenderCommandPublisher:  true,
	SenderEnrollmentService: true,
	SenderResultConsumer:    true,
	SenderPresenceConsumer:  true,
	SenderMonitoringRole:    true,
}

// ReservedSender reports whether s is a service-principal token, and therefore
// a token no agent identifier may be assigned.
func ReservedSender(s string) bool { return reservedSenders[s] }
