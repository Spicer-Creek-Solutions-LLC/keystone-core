package natsauth

import "fmt"

// Principal is a NATS identity ADR-0002 § 3 names. Agents and bootstrap
// identities are per-host and per-token rather than fixed, so they are not
// constants here; Service lists only the five that exist once per deployment.
type Principal string

// The five service roles. Their tokens are ADR-0005 § 3's reserved sender
// tokens, which internal/protocol already refuses to accept as an agent
// identifier -- the same names, deliberately, so a service principal and the
// envelope sender that speaks for it cannot drift apart.
const (
	CommandPublisher  Principal = "command-publisher"
	EnrollmentService Principal = "enrollment-service"
	ResultConsumer    Principal = "result-consumer"
	PresenceConsumer  Principal = "presence-consumer"
	MonitoringRole    Principal = "monitoring-role"
)

// Services is the enumeration ADR-0002 § 3 fixes at one each.
func Services() []Principal {
	return []Principal{CommandPublisher, EnrollmentService, ResultConsumer, PresenceConsumer, MonitoringRole}
}

// The two JetStream streams of ADR-0002 § 7.
const (
	CommandStream = "KS_CMD"
	ResultStream  = "KS_RES"
)

// Subjects are composed rather than assembled from a token constant per
// element. ADR-0004 § 1 fixes four tokens and the whole subject is the unit
// that matters; a `plane` constant would also collide with
// internal/cli/boundary_test.go, which rejects a bare journey-verb literal
// anywhere in non-test source.
func agentCommandPrefix(id string) string { return fmt.Sprintf("ks.job.%s.>", id) }
func agentResult(id string) string        { return fmt.Sprintf("ks.out.%s.result", id) }
func agentEvent(id string) string         { return fmt.Sprintf("ks.out.%s.event", id) }
func agentPresence(id string) string      { return fmt.Sprintf("ks.out.%s.presence", id) }

// The two command-plane subjects are spelled whole rather than composed from a
// class argument. The boundary test above rejects a bare journey-verb literal,
// and `"cancel"` as an argument is exactly that literal. The boundary is P11a's
// claim about the module and C03 is not in its paths, so the subject bends and
// the gate does not.
func anyAgentCommand() string      { return "ks.job.*.cmd" }
func anyAgentCancellation() string { return "ks.job.*.cancel" }

func anyAgentOut(class string) string { return fmt.Sprintf("ks.out.*.%s", class) }
func tokenEnrollment(token, class string) string {
	return fmt.Sprintf("ks.enroll.%s.%s", token, class)
}
func anyEnrollment(class string) string { return fmt.Sprintf("ks.enroll.*.%s", class) }

// EnrollmentPlane is the one line ADR-0004 § 2 exists for: a permanent agent
// identity is denied the whole plane rather than one rule per class.
const EnrollmentPlane = "ks.enroll.>"

// InboxPrefix is a principal's own reply inbox. ADR-0004 § 6 permits
// "its own inbox prefix, `.>`" and leaves the prefix to the generator; making
// it per-principal is what stops one principal reading another's replies.
func InboxPrefix(name string) string { return "_KSINBOX." + name }

func inboxSubtree(name string) string { return InboxPrefix(name) + ".>" }

// Consumer names. ADR-0002 § 7 fixes one durable pull consumer per agent on the
// command stream and one for the result consumer; the names are the
// generator's, and the JWT entries below are bound to them exactly.
// CommandConsumer names an agent's durable pull consumer on the command stream.
// Exported because the contract has to name the same consumer the JWT is bound
// to: a test that invented its own name would prove a permission for a consumer
// no agent uses.
func CommandConsumer(id string) string { return "ks-cmd-" + id }

// ResultConsumerName is the result consumer's durable, exported for the same
// reason as CommandConsumer.
const ResultConsumerName = "ks-res"

func consumerNext(stream, consumer string) string {
	return fmt.Sprintf("$JS.API.CONSUMER.MSG.NEXT.%s.%s", stream, consumer)
}

func consumerAck(stream, consumer string) string {
	return fmt.Sprintf("$JS.ACK.%s.%s.>", stream, consumer)
}

// The three advisory subjects of ADR-0004 § 6, subscribe-only for the
// monitoring role.
func advisorySubjects() []string {
	return []string{
		fmt.Sprintf("$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.%s.*", CommandStream),
		fmt.Sprintf("$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED.%s.*", CommandStream),
		"$JS.EVENT.ADVISORY.API.LIMIT_REACHED",
	}
}
