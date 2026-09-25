// Package enrollment is ADR-0003's one-use enrollment, as RFC 0005 amended it:
// the server's token issuance and enrollment service, and the agent's stages.
//
// It is the only package permitted to connect to NATS (C05.md § 3.3).
package enrollment
