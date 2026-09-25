package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// AgentFaults names the agent's fault-record journal. The holds themselves are
// C05-I4's.
type AgentFaults struct {
	RecordPath string
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
		target, ok := targets[q]
		if !ok {
			return Agent{}, fmt.Errorf("line %d: unknown key %q", line, q)
		}
		if seen[q] {
			return Agent{}, fmt.Errorf("line %d: duplicate key %q", line, q)
		}
		seen[q] = true
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
	return c, nil
}
