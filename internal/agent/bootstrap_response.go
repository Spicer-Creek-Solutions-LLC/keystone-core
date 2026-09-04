// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// subscribeBootstrapResponse listens for the control plane's reply to
// this agent's register publish and persists whatever it hands back.
//
// Called with a.mu held, from Start.
func (a *Agent) subscribeBootstrapResponse() error {
	subject := a.subjects.BootstrapResponse(a.cfg.AgentID)
	sub, err := a.nats.Subscribe(subject, a.handleBootstrapResponse)
	if err != nil {
		return fmt.Errorf("agent: subscribe %q: %w", subject, err)
	}
	a.bootstrapSub = sub
	return nil
}

// handleBootstrapResponse stores the issued credentials.
//
// Nothing from the payload is logged. The response carries an API key
// and, with identity enabled, a private key -- the one place in the
// protocol where that material crosses the wire in cleartext. It goes
// to a 0600 file and nowhere else.
func (a *Agent) handleBootstrapResponse(_ context.Context, subject string, env envelope.Envelope) error {
	var creds Credentials
	if err := json.Unmarshal(env.Payload, &creds); err != nil {
		a.log.Warn("agent: bootstrap response unmarshal",
			"subject", subject, "message_id", env.MessageID, "err", err)
		return nil
	}
	if creds.AgentID != "" && creds.AgentID != a.cfg.AgentID {
		// The subject is per-agent, so this should be unreachable.
		// Refusing anyway: storing another agent's credential would
		// give this host an identity it was never issued.
		a.log.Warn("agent: bootstrap response for a different agent",
			"subject", subject, "want", a.cfg.AgentID, "got", creds.AgentID)
		return nil
	}
	if creds.APIKey == "" {
		a.log.Warn("agent: bootstrap response carried no api key",
			"subject", subject, "message_id", env.MessageID)
		return nil
	}
	if creds.AgentID == "" {
		creds.AgentID = a.cfg.AgentID
	}

	a.mu.Lock()
	store := a.creds
	a.mu.Unlock()
	if store == nil {
		// No configured path. The agent still ran -- it just cannot
		// carry its identity across a restart, which is worth saying
		// out loud rather than silently degrading to the fleet key.
		a.log.Warn("agent: bootstrap credentials received but no store configured; identity will not survive restart",
			"agent_id", creds.AgentID, "has_svid", creds.HasSVID())
		return nil
	}
	if err := store.Save(&creds); err != nil {
		a.log.Error("agent: bootstrap credentials save", "err", err)
		return nil
	}

	notAfter, err := creds.LeafNotAfter()
	if err != nil {
		a.log.Warn("agent: bootstrap credentials stored but leaf is unparseable",
			"agent_id", creds.AgentID, "err", err)
		return nil
	}
	attrs := []any{"agent_id", creds.AgentID, "has_svid", creds.HasSVID()}
	if !notAfter.IsZero() {
		attrs = append(attrs, "svid_not_after", notAfter)
	}
	a.log.Info("agent: bootstrap credentials stored", attrs...)
	return nil
}

// haveValidCredentials reports whether a usable credential is already
// on disk, in which case Start skips the register publish entirely.
//
// Re-publishing would fail regardless: bootstrap PSKs are single-use
// and the server rejects a consumed one. Skipping turns a guaranteed
// warning on every restart into a debug line.
func (a *Agent) haveValidCredentials() bool {
	if a.creds == nil {
		return false
	}
	c, err := a.creds.Load()
	if errors.Is(err, ErrNoCredentials) {
		return false
	}
	if err != nil {
		a.log.Warn("agent: stored credentials unreadable; re-bootstrapping", "err", err)
		return false
	}
	if !c.Valid(time.Now()) {
		a.log.Info("agent: stored credentials expired or incomplete; re-bootstrapping",
			"agent_id", c.AgentID)
		return false
	}
	a.log.Debug("agent: using stored credentials; skipping bootstrap register",
		"agent_id", c.AgentID, "has_svid", c.HasSVID())
	return true
}
