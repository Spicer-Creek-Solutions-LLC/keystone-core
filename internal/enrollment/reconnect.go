package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
)

// ErrNoIdentity is an agent started with no usable identity. It fails, loudly,
// and never tries to acquire one (ADR-0003 § 9): there is no path from here to
// a bootstrap credential.
var ErrNoIdentity = errors.New("no enrolled identity: the identity artifact is missing or unreadable")

// PresencePeriod is how often an enrolled agent proves it is live.
const PresencePeriod = 5 * time.Second

// Enrolled is an agent's permanent identity, restored from its two artifacts.
type Enrolled struct {
	cfg      config.Agent
	ca       []byte
	keys     AgentKeys
	identity IdentityArtifact
}

// LoadEnrolled restores the identity S2 wrote and the keys it was written for.
// Anything missing, unreadable or mismatched is ErrNoIdentity.
func LoadEnrolled(cfg config.Agent) (*Enrolled, error) {
	raw, err := readPrivate(cfg.Agent.IdentityPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoIdentity, err)
	}
	var head struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(raw, &head); err != nil || head.AgentID == "" {
		return nil, fmt.Errorf("%w: %s is malformed", ErrNoIdentity, cfg.Agent.IdentityPath)
	}
	keys, err := ReadKeyArtifact(cfg.Agent.PrivateKeysPath, head.AgentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoIdentity, err)
	}
	identity, err := ReadIdentityArtifact(cfg.Agent.IdentityPath, keys)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoIdentity, err)
	}
	ca, err := os.ReadFile(cfg.NATS.CAFile)
	if err != nil {
		return nil, err
	}
	return &Enrolled{cfg: cfg, ca: ca, keys: keys, identity: identity}, nil
}

// Run connects with the permanent identity and proves presence until ctx ends.
// The client library reconnects the same identity across broker restarts; the
// enrollment plane is never touched (ADR-0003 § 9).
func (e *Enrolled) Run(ctx context.Context) error {
	opts, err := ClientOptions(e.ca, e.cfg.NATS.ServerName, []byte(e.identity.PermanentCredentials))
	if err != nil {
		return err
	}
	c, err := nats.Connect(e.cfg.NATS.URL, append(opts, nats.Name("keystone-agent"), nats.MaxReconnects(-1))...)
	if err != nil {
		return err
	}
	defer c.Close()
	tick := time.NewTicker(PresencePeriod)
	defer tick.Stop()
	for {
		if c.IsConnected() {
			if err := Presence(c, e.identity.AgentID, e.keys.Signing, time.Now()); err != nil && !errors.Is(err, nats.ErrConnectionClosed) {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}
