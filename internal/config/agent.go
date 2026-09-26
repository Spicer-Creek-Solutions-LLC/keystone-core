package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Agent is the agent's configuration (D-C05-4): where the broker is and how to
// verify it, and where each piece of the agent's durable state lives. Each is
// an explicit path; none is derived from another.
type Agent struct {
	NATS   AgentNATS
	Agent  AgentPaths
	Faults AgentFaults
}

type AgentNATS struct {
	URL        string
	CAFile     string
	ServerName string
}

type AgentPaths struct {
	IdentityPath    string
	PrivateKeysPath string
	LedgerPath      string
}

// AgentFaults are the agent's fault points (ADR-0010 § 5): configuration in the
// shipped binary, inert when absent, and recorded durably in the journal at
// RecordPath when enabled. Each hold stops the agent at one enrollment
// boundary for its duration.
type AgentFaults struct {
	RecordPath               string
	HoldBeforeRequest        time.Duration
	HoldAfterCredentialReply time.Duration
	HoldAfterIdentityWrite   time.Duration
	HoldAfterPermanentProof  time.Duration
	HoldBeforeConfirmation   time.Duration
}

// AgentHolds names each hold by its configuration key, in stage order.
func (f AgentFaults) AgentHolds() []struct {
	Key  string
	Hold time.Duration
} {
	return []struct {
		Key  string
		Hold time.Duration
	}{
		{"enrollment_agent_hold_before_request_ms", f.HoldBeforeRequest},
		{"enrollment_agent_hold_after_credential_reply_ms", f.HoldAfterCredentialReply},
		{"enrollment_agent_hold_after_identity_write_ms", f.HoldAfterIdentityWrite},
		{"enrollment_agent_hold_after_permanent_proof_ms", f.HoldAfterPermanentProof},
		{"enrollment_agent_hold_before_confirmation_ms", f.HoldBeforeConfirmation},
	}
}

// LoadAgent reads the agent's file. Every key but the fault journal is
// required; every file path must be absolute.
func LoadAgent(path string) (Agent, error) {
	f, err := os.Open(path)
	if err != nil {
		return Agent{}, err
	}
	defer f.Close()
	var c Agent
	targets := map[string]*string{
		"nats.url":                &c.NATS.URL,
		"nats.ca_file":            &c.NATS.CAFile,
		"nats.server_name":        &c.NATS.ServerName,
		"agent.identity_path":     &c.Agent.IdentityPath,
		"agent.private_keys_path": &c.Agent.PrivateKeysPath,
		"agent.ledger_path":       &c.Agent.LedgerPath,
		"faults.record_path":      &c.Faults.RecordPath,
	}
	holds := map[string]*time.Duration{
		"faults.enrollment_agent_hold_before_request_ms":         &c.Faults.HoldBeforeRequest,
		"faults.enrollment_agent_hold_after_credential_reply_ms": &c.Faults.HoldAfterCredentialReply,
		"faults.enrollment_agent_hold_after_identity_write_ms":   &c.Faults.HoldAfterIdentityWrite,
		"faults.enrollment_agent_hold_after_permanent_proof_ms":  &c.Faults.HoldAfterPermanentProof,
		"faults.enrollment_agent_hold_before_confirmation_ms":    &c.Faults.HoldBeforeConfirmation,
	}
	seen := map[string]bool{}
	section := ""
	s := bufio.NewScanner(f)
	for line := 1; s.Scan(); line++ {
		raw := strings.TrimSpace(stripComment(s.Text()))
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			section = strings.TrimSpace(raw[1 : len(raw)-1])
			switch section {
			case "nats", "agent", "faults":
			default:
				return Agent{}, fmt.Errorf("line %d: unknown table %q", line, section)
			}
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 || section == "" {
			return Agent{}, fmt.Errorf("line %d: invalid configuration", line)
		}
		q := section + "." + strings.TrimSpace(parts[0])
		if seen[q] {
			return Agent{}, fmt.Errorf("line %d: duplicate key %q", line, q)
		}
		seen[q] = true
		if hold, ok := holds[q]; ok {
			n, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			if err != nil || n <= 0 || n > MaxFaultDelay.Milliseconds() {
				return Agent{}, fmt.Errorf("line %d: %s must be a positive number of milliseconds up to %d", line, q, MaxFaultDelay.Milliseconds())
			}
			*hold = time.Duration(n) * time.Millisecond
			continue
		}
		target, ok := targets[q]
		if !ok {
			return Agent{}, fmt.Errorf("line %d: unknown key %q", line, q)
		}
		v, err := strconv.Unquote(strings.TrimSpace(parts[1]))
		if err != nil || v == "" {
			return Agent{}, fmt.Errorf("line %d: %s must be a non-empty quoted string", line, q)
		}
		if q != "nats.url" && q != "nats.server_name" && !filepath.IsAbs(v) {
			return Agent{}, fmt.Errorf("%w: %s=%q", ErrRelativePath, q, v)
		}
		*target = v
	}
	if err := s.Err(); err != nil {
		return Agent{}, err
	}
	for q := range targets {
		if !seen[q] && q != "faults.record_path" {
			return Agent{}, fmt.Errorf("%s is required", q)
		}
	}
	// A hold with nowhere to record that it was reached is a hold a harness
	// cannot act on and an audit cannot see.
	for _, h := range c.Faults.AgentHolds() {
		if h.Hold > 0 && c.Faults.RecordPath == "" {
			return Agent{}, fmt.Errorf("faults.%s needs faults.record_path", h.Key)
		}
	}
	return c, nil
}
