// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// SignedRequest is an agent-authored message the control plane can
// attribute to one specific agent.
//
// Everything the agent sends today is authenticated -- if at all -- by
// the fleet-wide HMAC key, which proves that a sender is some member
// of the fleet and nothing more. This carries the agent's own X509
// SVID and a signature made with its private key, so the server can
// verify the chain against the CA and read the sender's identity out
// of the certificate rather than believing a self-declared field.
//
// AgentID is present for logging and early rejection only. It is NOT
// the authority on who sent this: the verifier takes the identity from
// the leaf's SPIFFE URI SAN and requires this field to match. An agent
// that claims another's id fails there.
type SignedRequest struct {
	AgentID string `json:"agent_id"`
	// CertChainPEM is [leaf, signing CA] as PEM CERTIFICATE blocks --
	// the same chain the control plane issued at bootstrap, sent back
	// so the verifier can build a path to the CA without holding
	// per-agent state.
	CertChainPEM string `json:"cert_chain_pem"`
	// IssuedAt bounds how long a captured request stays usable.
	IssuedAt time.Time `json:"issued_at"`
	// Nonce makes two requests with identical payloads produce
	// different signatures.
	Nonce string `json:"nonce"`
	// Payload is the caller's message, signed verbatim.
	Payload []byte `json:"payload"`
	// Signature is hex-encoded, over CanonicalSignedRequest.
	Signature string `json:"signature"`
}

// SVIDSigner signs outbound requests with the agent's bootstrap-issued
// private key.
//
// Constructed from stored Credentials, so an agent with an API-key-only
// credential (identity disabled server-side) simply has no signer and
// callers fall back to whatever the unauthenticated path was.
type SVIDSigner struct {
	key      crypto.Signer
	chainPEM string
	agentID  string
}

// ErrNoSVID is returned when credentials carry no certificate — an
// API-key-only bootstrap, which is what a server with identity
// disabled issues. Callers distinguish it from a malformed credential.
var ErrNoSVID = errors.New("agent: credentials carry no SVID")

// NewSVIDSigner builds a signer from stored credentials. The agent id
// comes from the certificate's SPIFFE URI SAN, not from the
// credential's AgentID field: the certificate is the thing the server
// will verify, so it is the thing that decides who this agent is.
func NewSVIDSigner(creds *Credentials) (*SVIDSigner, error) {
	if !creds.HasSVID() {
		return nil, ErrNoSVID
	}
	block, _ := pem.Decode([]byte(creds.CertChainPEM))
	if block == nil {
		return nil, errors.New("agent: svid: cert chain is not PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent: svid: parse leaf: %w", err)
	}
	agentID, err := AgentIDFromCert(leaf)
	if err != nil {
		return nil, err
	}

	keyBlock, _ := pem.Decode([]byte(creds.PrivateKeyPEM))
	if keyBlock == nil {
		return nil, errors.New("agent: svid: private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("agent: svid: parse private key: %w", err)
	}
	key, ok := parsed.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("agent: svid: private key of type %T cannot sign", parsed)
	}
	// A key that does not match the certificate would produce
	// signatures the server rejects on every request, with an error
	// pointing at the signature rather than at the mismatch. Catch it
	// once, here.
	if !publicKeysEqual(key.Public(), leaf.PublicKey) {
		return nil, errors.New("agent: svid: private key does not match the leaf certificate")
	}

	return &SVIDSigner{key: key, chainPEM: creds.CertChainPEM, agentID: agentID}, nil
}

// AgentID is the identity asserted by the certificate.
func (s *SVIDSigner) AgentID() string { return s.agentID }

// Sign wraps payload in a SignedRequest.
//
// The digest is SHA-256 over the canonical encoding regardless of key
// type; ECDSA and RSA keys both take a pre-hashed digest, and the CA
// issues ECDSA-P256 by default with P384/RSA configurable.
func (s *SVIDSigner) Sign(payload []byte) (*SignedRequest, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("agent: svid: nonce: %w", err)
	}
	req := &SignedRequest{
		AgentID:      s.agentID,
		CertChainPEM: s.chainPEM,
		IssuedAt:     time.Now().UTC(),
		Nonce:        hex.EncodeToString(nonce),
		Payload:      payload,
	}
	digest := sha256.Sum256(CanonicalSignedRequest(req))
	sig, err := s.key.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("agent: svid: sign: %w", err)
	}
	req.Signature = hex.EncodeToString(sig)
	return req, nil
}

// CanonicalSignedRequest is the exact byte string that gets signed.
//
// Length-prefixed, matching canonicalConverge: without it, moving a
// byte from the end of one field to the start of the next would leave
// the concatenation -- and so the signature -- unchanged. The
// signature itself is excluded, and CertChainPEM is included so a
// captured signature cannot be re-presented with a different
// certificate.
//
// Exported because it is the protocol contract, not an internal
// detail: it is what an independent implementation of an agent would
// have to reproduce byte for byte, and what the control plane's tests
// use to forge the requests a hostile agent could actually construct.
func CanonicalSignedRequest(r *SignedRequest) []byte {
	var buf []byte
	appendField := func(s string) {
		buf = append(buf, []byte(strconv.Itoa(len(s)))...)
		buf = append(buf, ':')
		buf = append(buf, []byte(s)...)
		buf = append(buf, '|')
	}
	appendField(r.AgentID)
	appendField(r.CertChainPEM)
	appendField(strconv.FormatInt(r.IssuedAt.UTC().UnixNano(), 10))
	appendField(r.Nonce)
	appendField(string(r.Payload))
	return buf
}

// AgentIDFromCert reads the agent id out of a leaf's SPIFFE URI SAN.
//
// Shared with the control-plane verifier so both ends agree on what a
// certificate says. Requires exactly one URI SAN, matching
// identity.NewX509SVID: a leaf with two identities has no single
// answer to "who is this".
func AgentIDFromCert(leaf *x509.Certificate) (string, error) {
	if leaf == nil {
		return "", errors.New("agent: svid: no leaf certificate")
	}
	if len(leaf.URIs) != 1 {
		return "", fmt.Errorf("agent: svid: leaf must carry exactly one URI SAN, got %d", len(leaf.URIs))
	}
	uri := leaf.URIs[0]
	if uri.Scheme != "spiffe" {
		return "", fmt.Errorf("agent: svid: leaf URI SAN %q is not a SPIFFE ID", uri)
	}
	// spiffe://<trust domain>/agent/<agent id>
	const prefix = "/agent/"
	path := uri.Path
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return "", fmt.Errorf("agent: svid: leaf URI SAN %q is not an agent identity", uri)
	}
	id := path[len(prefix):]
	if id == "" {
		return "", fmt.Errorf("agent: svid: leaf URI SAN %q carries an empty agent id", uri)
	}
	return id, nil
}

// publicKeysEqual compares two public keys of any supported type.
func publicKeysEqual(a, b crypto.PublicKey) bool {
	type equaler interface{ Equal(crypto.PublicKey) bool }
	if e, ok := a.(equaler); ok {
		return e.Equal(b)
	}
	return false
}

// VerifySignature checks r's signature against pub. Exported so the
// control-plane verifier reuses exactly the algorithm dispatch the
// signer used -- a second implementation is a second chance to
// disagree about what was signed.
func VerifySignature(pub crypto.PublicKey, r *SignedRequest) error {
	sig, err := hex.DecodeString(r.Signature)
	if err != nil {
		return fmt.Errorf("agent: svid: signature is not hex: %w", err)
	}
	digest := sha256.Sum256(CanonicalSignedRequest(r))
	switch key := pub.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, digest[:], sig) {
			return errors.New("agent: svid: signature does not verify")
		}
		return nil
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
			return fmt.Errorf("agent: svid: signature does not verify: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("agent: svid: unsupported public key type %T", pub)
	}
}
