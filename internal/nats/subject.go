package nats

import (
	"errors"
	"fmt"
	"strings"
)

// subjectRoot is the non-negotiable top-level token for every Keystone
// Core NATS subject. PROJECT-DETAILS §4.2: "every subject prefixed
// with kscore.{cluster}. from day one … lets v2 supercluster slide in
// without refactoring all subscriptions."
const subjectRoot = "kscore"

// SubjectBuilder constructs and validates the v1.0 subject hierarchy.
// All public-facing subjects flow through this type — typed
// constructors prevent prefix typos, and Validate is the safety-net
// interceptor wired into Manager.Publish so any caller bypassing the
// constructors still cannot publish on a non-prefixed subject.
//
// Cluster name is fixed at construction time (per PROJECT-DETAILS §4.2,
// runtime cluster rename is out of scope for v1.0). The builder is
// safe for concurrent use — all state is immutable after New.
type SubjectBuilder struct {
	cluster string
	prefix  string // "kscore.{cluster}"
}

// NewSubjectBuilder builds a SubjectBuilder for the given cluster
// name. Cluster name must be non-empty and contain only NATS-safe
// tokens (alphanumeric, dash, underscore — no dots, wildcards, or
// whitespace). The cluster name is validated upstream by config.Load
// but we assert it here too because the builder is reachable from
// tests that construct configs by hand.
func NewSubjectBuilder(cluster string) (*SubjectBuilder, error) {
	if cluster == "" {
		return nil, errors.New("nats: cluster name must not be empty")
	}
	if err := validateToken(cluster); err != nil {
		return nil, fmt.Errorf("nats: cluster name: %w", err)
	}
	return &SubjectBuilder{
		cluster: cluster,
		prefix:  subjectRoot + "." + cluster,
	}, nil
}

// Cluster returns the configured cluster name. Used by callers (and
// tests) that need the name for log fields or to reconstruct a
// subject via a different path.
func (b *SubjectBuilder) Cluster() string { return b.cluster }

// Prefix returns "kscore.{cluster}" — the namespace every subject
// lives under. Subscribers wanting to consume the entire cluster's
// traffic use Prefix() + ".>".
func (b *SubjectBuilder) Prefix() string { return b.prefix }

// AgentRegister is the well-known subject all agents publish their
// initial registration message to. Server-side handler subscribes
// (Task 9 wires the bootstrap variant; this one is the post-
// bootstrap registration path).
func (b *SubjectBuilder) AgentRegister() string {
	return b.prefix + ".agent.register"
}

// AgentHeartbeat is the well-known heartbeat subject. All agents
// publish here on the cadence configured in their runtime.
func (b *SubjectBuilder) AgentHeartbeat() string {
	return b.prefix + ".agent.heartbeat"
}

// AgentCommand is the per-agent inbound command subject. The control
// plane publishes here; the named agent subscribes.
func (b *SubjectBuilder) AgentCommand(agentID string) string {
	return b.prefix + ".agent." + agentID + ".command"
}

// AgentResponse is the per-agent command-response subject. Agents
// publish results; the control plane subscribes via the command
// dispatcher.
func (b *SubjectBuilder) AgentResponse(agentID string) string {
	return b.prefix + ".agent." + agentID + ".response"
}

// AgentResponsePattern is the wildcard form of AgentResponse — the
// server-side response subscriber (Epic 07 task 12 / 9d) fans every
// agent's responses into a correlation map keyed by CorrelationID.
func (b *SubjectBuilder) AgentResponsePattern() string {
	return b.prefix + ".agent.*.response"
}

// AgentState is the per-agent state-publication subject (status,
// resource snapshot). Agents publish; observers subscribe.
func (b *SubjectBuilder) AgentState(agentID string) string {
	return b.prefix + ".agent." + agentID + ".state"
}

// ServerAnnounce is the server-to-server announcement subject (Epic
// 13 clustering). v1.0 single-server installs publish here on boot
// for future-proofing the subject layout.
func (b *SubjectBuilder) ServerAnnounce() string {
	return b.prefix + ".server.announce"
}

// ServerControl is the server-to-server control-plane subject
// (leader election handshakes, future cluster ops).
func (b *SubjectBuilder) ServerControl() string {
	return b.prefix + ".server.control"
}

// BootstrapRegister is the per-agent bootstrap registration request
// subject. Agents publish identity proof here under the short-lived
// bootstrap credential. Task 9 wires the server-side handler.
func (b *SubjectBuilder) BootstrapRegister(agentID string) string {
	return b.prefix + ".bootstrap." + agentID + ".register"
}

// BootstrapResponse is the per-agent bootstrap response subject. The
// server publishes full credentials here once registration is
// validated.
func (b *SubjectBuilder) BootstrapResponse(agentID string) string {
	return b.prefix + ".bootstrap." + agentID + ".response"
}

// BootstrapRegisterPattern is the wildcard subject the server-side
// bootstrap handler subscribes to. Per-agent register publishes
// (kscore.{cluster}.bootstrap.{id}.register) fan in here.
//
// '*' is a NATS subscriber wildcard — legal in subscribe patterns,
// rejected in publish by SubjectBuilder.Validate.
func (b *SubjectBuilder) BootstrapRegisterPattern() string {
	return b.prefix + ".bootstrap.*.register"
}

// Discovery is the cluster-wide discovery subject (peer enumeration,
// endpoint advertisement). Reserved for post-v1.0 K8s discovery; the
// subject exists today so subscribers can register without churn.
func (b *SubjectBuilder) Discovery() string {
	return b.prefix + ".discovery"
}

// Validate enforces four rules on a publish subject:
//
//  1. Subject is non-empty.
//  2. Subject equals Prefix() exactly OR begins with Prefix() + ".".
//  3. Subject contains no NATS subscriber wildcards ('*' or '>') —
//     those are legal in subscribe patterns, never in publish.
//  4. Subject contains only printable ASCII (0x20–0x7E) and is free
//     of whitespace. The "no non-printable" rule is defense-in-depth
//     against hash-input ambiguity in the dedup cache (Task 6) — a
//     subject containing 0x00 could collide with a different
//     (subject, messageID) pair under naive concatenation. Length-
//     prefixed hashing makes this safe regardless, but rejecting
//     malformed subjects at the boundary is the right place.
//
// This is the interceptor referenced in epic 05's acceptance bullet:
// any caller bypassing the typed constructors fails here at Publish.
func (b *SubjectBuilder) Validate(subject string) error {
	if subject == "" {
		return errors.New("nats: subject must not be empty")
	}
	if subject != b.prefix && !strings.HasPrefix(subject, b.prefix+".") {
		return fmt.Errorf("nats: subject %q must start with %q", subject, b.prefix+".")
	}
	if strings.ContainsAny(subject, "*>") {
		return fmt.Errorf("nats: subject %q contains wildcard ('*' or '>'); illegal in publish", subject)
	}
	for i := 0; i < len(subject); i++ {
		c := subject[i]
		if c <= 0x20 || c >= 0x7F {
			return fmt.Errorf("nats: subject %q contains non-printable byte at index %d", subject, i)
		}
	}
	return nil
}

// validateToken rejects tokens unsuitable for use inside a NATS
// subject — empty, dot-bearing (would create extra hierarchy levels),
// wildcard-bearing (collides with subscribe semantics), or
// whitespace-bearing (NATS rejects them at protocol level).
func validateToken(token string) error {
	if token == "" {
		return errors.New("token must not be empty")
	}
	if strings.ContainsAny(token, ". *>") {
		return fmt.Errorf("token %q contains forbidden character", token)
	}
	if strings.ContainsAny(token, "\t\r\n") {
		return fmt.Errorf("token %q contains whitespace", token)
	}
	return nil
}
