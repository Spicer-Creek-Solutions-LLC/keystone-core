// Package rotation provides credential rotation for proxy-managed network devices.
//
// It supports rotating SSH passwords/keys, SNMP community strings/v3 credentials,
// REST API tokens, and TLS certificates on devices managed by proxy agents.
// The rotation engine uses a state machine to orchestrate the workflow:
// validate old credential, generate new, apply to device, verify, store, and cleanup.
// On failure at any stage, automatic rollback restores the previous credential.
//
// Rotation can be triggered manually or via policy-based scheduling with cron expressions.
package rotation
