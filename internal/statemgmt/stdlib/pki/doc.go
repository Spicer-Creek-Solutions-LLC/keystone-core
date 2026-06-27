// SPDX-License-Identifier: Apache-2.0

package pki

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the x509 module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Certificates",
		Summary: "Manages a TLS certificate and its private key on disk (the declaration " +
			"name is the certificate path) using crypto/x509 — no shelling out. Idempotent: " +
			"a key and certificate that already match the declaration (right subject, validity, " +
			"and signer) report no change; anything stale or expiring is regenerated.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "A private key and a matching certificate exist, the certificate is currently valid with at least `renew_days` left, and it is self-signed or signed by the declared `ca_cert`. Stale or expiring material is regenerated."},
			{Name: "absent", Desc: "The certificate file and the key file are removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "key_path", Type: "string", Required: true, Desc: "Private-key path; must differ from the certificate path (combined cert+key PEM is not supported)."},
			{Name: "common_name", Type: "string", Desc: "Subject common name (CN). For state `present`, one of `common_name` or `subject_alt_names` is required."},
			{Name: "subject_alt_names", Type: "list", Desc: "Subject alternative names; a single string or a list of strings. IP vs DNS is auto-detected. One of `common_name` or `subject_alt_names` is required for `present`."},
			{Name: "organization", Type: "string", Desc: "Subject organization (O)."},
			{Name: "days", Type: "int", Default: "365", Desc: "Validity window in days for a newly generated certificate."},
			{Name: "renew_days", Type: "int", Default: "30", Desc: "Regenerate when fewer than this many days of validity remain; 0 disables expiry-proximity renewal."},
			{Name: "key_type", Type: "string", Default: "rsa", Desc: "Private-key algorithm: `rsa`, `ecdsa`, or `ed25519`."},
			{Name: "rsa_bits", Type: "int", Default: "2048", Desc: "RSA key size (minimum 2048); applies when `key_type` is `rsa`."},
			{Name: "ecdsa_curve", Type: "string", Default: "p256", Desc: "ECDSA curve: `p256`, `p384`, or `p521`; applies when `key_type` is `ecdsa`."},
			{Name: "is_ca", Type: "bool", Default: "false", Desc: "Mark the certificate as a CA (sets the basic-constraints CA flag)."},
			{Name: "ca_cert", Type: "string", Desc: "Signing CA certificate path; set together with `ca_key`. Omit both for a self-signed certificate."},
			{Name: "ca_key", Type: "string", Desc: "Signing CA private-key path; set together with `ca_cert`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Self-signed leaf certificate",
				YAML: `x509:
  /etc/ssl/myapp/server.crt:
    state: present
    key_path: /etc/ssl/myapp/server.key
    common_name: host.example.com
    subject_alt_names:
      - host.example.com
      - 10.1.2.3
    organization: Keystone
    days: 365
    renew_days: 30`,
			},
			{
				Title: "A CA, then a leaf signed by it",
				Desc:  "The `require` requisite orders the leaf after its signing CA.",
				YAML: `x509:
  /etc/pki/ca.crt:
    state: present
    key_path: /etc/pki/ca.key
    common_name: Keystone Root CA
    is_ca: true
    key_type: ecdsa
    ecdsa_curve: p384
  /etc/pki/server.crt:
    state: present
    key_path: /etc/pki/server.key
    common_name: host.example.com
    ca_cert: /etc/pki/ca.crt
    ca_key: /etc/pki/ca.key
    require:
      - x509: /etc/pki/ca.crt`,
			},
			{
				Title: "Remove a certificate and its key",
				YAML: `x509:
  /etc/ssl/old/server.crt:
    state: absent
    key_path: /etc/ssl/old/server.key`,
			},
		},
		Notes: []string{
			"OS-agnostic: certificate and key material is generated in-process with crypto/x509 — no openssl binary required.",
			"`ca_cert` and `ca_key` must be set together; omit both for a self-signed certificate.",
			"`days` only affects newly generated certificates; on regeneration a still-valid key is reused.",
			"New keys are written 0600 and new certificates 0644; rewrites preserve the existing mode.",
			"Out of scope (v0.x candidates, #110): combined cert+key PEM in one file, encrypted (passphrase-protected) keys, OpenSSL-style SAN prefixes and email/URI SANs, additional Subject fields, explicit mode/owner params, and CRL/OCSP/PKCS#12/ACME issuance.",
		},
	}
}
