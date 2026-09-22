package natsauth

// grants is the generator's statement of ADR-0004 §§ 4 and 6. It is
// deliberately a SECOND statement: the contract freezes its own transcription,
// and GEN-3 and GEN-4 compare the JWTs this produces against that one in both
// directions. Two independent readings of the same accepted table are what make
// a disagreement visible; deriving one from the other would make them agree by
// construction and prove nothing.
type grants struct {
	pub []string
	sub []string
}

func serviceGrants(p Principal) grants {
	switch p {
	case CommandPublisher:
		return grants{
			pub: []string{anyAgentCommand(), anyAgentCancellation()},
			sub: []string{inboxSubtree(string(CommandPublisher))},
		}
	case EnrollmentService:
		return grants{
			pub: []string{anyEnrollment("reply")},
			sub: []string{anyEnrollment("request")},
		}
	case ResultConsumer:
		return grants{
			pub: []string{
				consumerNext(ResultStream, ResultConsumerName),
				consumerAck(ResultStream, ResultConsumerName),
			},
			sub: []string{inboxSubtree(string(ResultConsumer))},
		}
	case PresenceConsumer:
		return grants{sub: []string{anyAgentOut("presence")}}
	case MonitoringRole:
		return grants{sub: append([]string{anyAgentOut("event")}, advisorySubjects()...)}
	}
	return grants{}
}

// agentGrants is the per-agent template with one identifier substituted. Every
// entry is scoped to that agent: ARCH-NATS-003 allows a documented wildcard
// only for a service role, and none of these is one.
func agentGrants(id string) grants {
	consumer := CommandConsumer(id)
	return grants{
		pub: []string{
			agentResult(id),
			agentEvent(id),
			agentPresence(id),
			consumerNext(CommandStream, consumer),
			consumerAck(CommandStream, consumer),
		},
		sub: []string{
			agentCommandPrefix(id),
			inboxSubtree(id),
		},
	}
}

// bootstrapGrants is one token's pair and nothing else. ADR-0003 § 8's expiry
// is applied to the claim rather than expressed here.
func bootstrapGrants(token string) grants {
	return grants{
		pub: []string{tokenEnrollment(token, "request")},
		sub: []string{tokenEnrollment(token, "reply")},
	}
}
