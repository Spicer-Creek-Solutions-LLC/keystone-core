// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
)

// AgentCredentials (issued here) and agent.Credentials (stored there)
// are the same wire object described twice, because internal/
// controlplane imports internal/agent and the reverse edge would be a
// cycle. The duplication is only safe while the JSON stays identical,
// so this test pins it — from this side, which is the one that can see
// both types.
//
// If this fails, the bootstrap protocol has silently forked: the
// server will issue a field the agent drops on the floor.
func TestAgentCredentials_WireShapeMatchesAgent(t *testing.T) {
	issued := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	server := AgentCredentials{
		APIKey:         "api-key-1",
		AgentID:        "agent-1",
		IssuedAt:       issued,
		CertChainPEM:   "chain",
		PrivateKeyPEM:  "key",
		TrustBundlePEM: "bundle",
	}

	encoded, err := json.Marshal(server)
	if err != nil {
		t.Fatalf("marshal server credentials: %v", err)
	}

	var stored agent.Credentials
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatalf("agent cannot decode server credentials: %v", err)
	}

	if stored.APIKey != server.APIKey {
		t.Errorf("APIKey = %q, want %q", stored.APIKey, server.APIKey)
	}
	if stored.AgentID != server.AgentID {
		t.Errorf("AgentID = %q, want %q", stored.AgentID, server.AgentID)
	}
	if !stored.IssuedAt.Equal(server.IssuedAt) {
		t.Errorf("IssuedAt = %v, want %v", stored.IssuedAt, server.IssuedAt)
	}
	if stored.CertChainPEM != server.CertChainPEM {
		t.Errorf("CertChainPEM = %q, want %q", stored.CertChainPEM, server.CertChainPEM)
	}
	if stored.PrivateKeyPEM != server.PrivateKeyPEM {
		t.Errorf("PrivateKeyPEM = %q, want %q", stored.PrivateKeyPEM, server.PrivateKeyPEM)
	}
	if stored.TrustBundlePEM != server.TrustBundlePEM {
		t.Errorf("TrustBundlePEM = %q, want %q", stored.TrustBundlePEM, server.TrustBundlePEM)
	}

	// Re-encoding from the agent side must produce the same JSON, so a
	// field added to one struct and not the other is caught even when
	// the round-trip above happens to ignore it.
	reencoded, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal agent credentials: %v", err)
	}
	var fromServer, fromAgent map[string]any
	if err := json.Unmarshal(encoded, &fromServer); err != nil {
		t.Fatalf("decode server json: %v", err)
	}
	if err := json.Unmarshal(reencoded, &fromAgent); err != nil {
		t.Fatalf("decode agent json: %v", err)
	}
	if !reflect.DeepEqual(fromServer, fromAgent) {
		t.Errorf("JSON shapes diverged:\n server = %s\n agent  = %s", encoded, reencoded)
	}
}

// The omitempty contract matters on its own: an API-key-only issuer
// must produce exactly the pre-SVID payload, so an older agent decodes
// it byte-identically.
func TestAgentCredentials_APIKeyOnlyOmitsSVIDFields(t *testing.T) {
	encoded, err := json.Marshal(AgentCredentials{
		APIKey:   "api-key-1",
		AgentID:  "agent-1",
		IssuedAt: time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, absent := range []string{"cert_chain_pem", "private_key_pem", "trust_bundle_pem"} {
		if _, ok := fields[absent]; ok {
			t.Errorf("field %q present in an API-key-only credential: %s", absent, encoded)
		}
	}

	var stored agent.Credentials
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatalf("agent decode: %v", err)
	}
	if stored.HasSVID() {
		t.Error("agent reports an SVID for an API-key-only credential")
	}
	if !stored.Valid(time.Now()) {
		t.Error("an API-key-only credential must still be valid")
	}
}
