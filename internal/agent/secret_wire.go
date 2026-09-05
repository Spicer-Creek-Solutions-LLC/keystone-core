// SPDX-License-Identifier: Apache-2.0

package agent

import "go.keystone-core.io/keystone-core/internal/sealed"

// SecretLookupRequest is what an agent asks for. It travels as the
// Payload of a SignedRequest, so the asker's identity is established
// by certificate rather than by anything in this struct -- there is
// deliberately no agent_id field here to be tempted by.
type SecretLookupRequest struct {
	// Path is the secret's path in the store.
	Path string `json:"path"`
	// Key selects one field of the secret's data. Empty means the
	// backend's conventional single-value field.
	Key string `json:"key,omitempty"`
}

// SecretLookupResponse answers one lookup.
//
// The value is inside Box, sealed to the requesting agent's public
// key, because every agent shares one NATS credential and can read
// this subject. Error and Denied are outside it: an agent that cannot
// open the box still needs to learn why it got nothing, and neither
// field says anything a fleet member does not already know.
type SecretLookupResponse struct {
	// Nonce echoes the request's, correlating the answer to the ask.
	Nonce string      `json:"nonce"`
	Box   *sealed.Box `json:"box,omitempty"`
	// Denied means the agent is authenticated but not granted this
	// path. Distinguished from Error so an operator reading agent logs
	// can tell a policy gap from a broken lookup.
	Denied bool   `json:"denied,omitempty"`
	Error  string `json:"error,omitempty"`
}

// SecretLookupTimeout is the default wait for an answer. Rendering a
// state file blocks on this, so it is short: a control plane that
// cannot answer promptly should fail the declaration rather than stall
// a converge.
const SecretLookupTimeout = 10
