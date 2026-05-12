// Package pki implements the `x509` stdlib state module — managing a
// TLS certificate + private-key pair on disk with crypto/x509 (no
// shelling out), per PROJECT-DETAILS §4.8 (Certificates category).
// The Go package is named `pki` because `x509` would collide with
// the stdlib `crypto/x509` import; it registers under the operator-
// facing name "x509" (mirroring kmod → kernel_module).
package pki

import (
	"fmt"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

// Key types.
const (
	KeyRSA     = "rsa"
	KeyECDSA   = "ecdsa"
	KeyEd25519 = "ed25519"
)

// ECDSA curves.
const (
	CurveP256 = "p256"
	CurveP384 = "p384"
	CurveP521 = "p521"
)

const (
	defaultDays      = 365
	defaultRenewDays = 30
	defaultRSABits   = 2048
	minRSABits       = 2048
)

const (
	paramKeyPath      = "key_path"
	paramCommonName   = "common_name"
	paramSANs         = "subject_alt_names"
	paramOrganization = "organization"
	paramDays         = "days"
	paramRenewDays    = "renew_days"
	paramKeyType      = "key_type"
	paramRSABits      = "rsa_bits"
	paramECDSACurve   = "ecdsa_curve"
	paramIsCA         = "is_ca"
	paramCACert       = "ca_cert"
	paramCAKey        = "ca_key"
	paramSeverity     = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramKeyPath:      {},
	paramCommonName:   {},
	paramSANs:         {},
	paramOrganization: {},
	paramDays:         {},
	paramRenewDays:    {},
	paramKeyType:      {},
	paramRSABits:      {},
	paramECDSACurve:   {},
	paramIsCA:         {},
	paramCACert:       {},
	paramCAKey:        {},
	paramSeverity:     {},
}

// absentAllowedKeys are the only params an `absent` declaration may
// carry — the cert-shaping params are meaningless when removing.
var absentAllowedKeys = map[string]struct{}{
	paramKeyPath:  {},
	paramSeverity: {},
}

type params struct {
	CertPath     string // Declaration.Name
	State        string
	KeyPath      string
	CommonName   string
	SANs         []string
	Organization string
	Days         int
	RenewDays    int
	KeyType      string // rsa|ecdsa|ed25519
	RSABits      int
	ECDSACurve   string // p256|p384|p521
	IsCA         bool
	CACertPath   string
	CAKeyPath    string

	seen map[string]struct{}
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	seen := make(map[string]struct{}, len(decl.Params))
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q", k)
		}
		seen[k] = struct{}{}
	}
	p := &params{
		CertPath:   decl.Name,
		State:      decl.State,
		Days:       defaultDays,
		RenewDays:  defaultRenewDays,
		KeyType:    KeyRSA,
		RSABits:    defaultRSABits,
		ECDSACurve: CurveP256,
		seen:       seen,
	}
	str := func(key string) (string, bool, error) {
		raw, ok := decl.Params[key]
		if !ok {
			return "", false, nil
		}
		s, ok := raw.(string)
		if !ok {
			return "", false, fmt.Errorf("%s: expected string, got %T", key, raw)
		}
		return s, true, nil
	}
	var err error
	if p.KeyPath, _, err = str(paramKeyPath); err != nil {
		return nil, err
	}
	if p.CommonName, _, err = str(paramCommonName); err != nil {
		return nil, err
	}
	if p.Organization, _, err = str(paramOrganization); err != nil {
		return nil, err
	}
	if p.CACertPath, _, err = str(paramCACert); err != nil {
		return nil, err
	}
	if p.CAKeyPath, _, err = str(paramCAKey); err != nil {
		return nil, err
	}
	if s, ok, err := str(paramKeyType); err != nil {
		return nil, err
	} else if ok && s != "" {
		p.KeyType = strings.ToLower(s)
	}
	if s, ok, err := str(paramECDSACurve); err != nil {
		return nil, err
	} else if ok && s != "" {
		p.ECDSACurve = strings.ToLower(s)
	}
	if raw, ok := decl.Params[paramSANs]; ok {
		sans, err := parseSANs(raw)
		if err != nil {
			return nil, fmt.Errorf("subject_alt_names: %w", err)
		}
		p.SANs = sans
	}
	if raw, ok := decl.Params[paramDays]; ok {
		n, err := parseInt(raw)
		if err != nil {
			return nil, fmt.Errorf("days: %w", err)
		}
		p.Days = n
	}
	if raw, ok := decl.Params[paramRenewDays]; ok {
		n, err := parseInt(raw)
		if err != nil {
			return nil, fmt.Errorf("renew_days: %w", err)
		}
		p.RenewDays = n
	}
	if raw, ok := decl.Params[paramRSABits]; ok {
		n, err := parseInt(raw)
		if err != nil {
			return nil, fmt.Errorf("rsa_bits: %w", err)
		}
		p.RSABits = n
	}
	if raw, ok := decl.Params[paramIsCA]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("is_ca: expected bool, got %T", raw)
		}
		p.IsCA = b
	}
	return p, nil
}

func parseSANs(raw any) ([]string, error) {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("empty SAN")
		}
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("element %d: expected string, got %T", i, e)
			}
			if strings.TrimSpace(s) == "" {
				return nil, fmt.Errorf("element %d: empty", i)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected a string or a list of strings, got %T", raw)
	}
}

func parseInt(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return int(v), nil
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func (p *params) validate() error {
	if strings.TrimSpace(p.CertPath) == "" {
		return fmt.Errorf("certificate path (the declaration name) is required")
	}
	if strings.TrimSpace(p.KeyPath) == "" {
		return fmt.Errorf("key_path is required")
	}
	if p.KeyPath == p.CertPath {
		return fmt.Errorf("key_path must differ from the certificate path (combined cert+key PEM is not supported in v1.0)")
	}
	switch p.State {
	case StatePresent:
		if p.CommonName == "" && len(p.SANs) == 0 {
			return fmt.Errorf("state=present requires common_name or subject_alt_names")
		}
		switch p.KeyType {
		case KeyRSA:
			if p.RSABits < minRSABits {
				return fmt.Errorf("rsa_bits: must be >= %d, got %d", minRSABits, p.RSABits)
			}
		case KeyECDSA:
			switch p.ECDSACurve {
			case CurveP256, CurveP384, CurveP521:
			default:
				return fmt.Errorf("ecdsa_curve: must be %s, %s or %s, got %q", CurveP256, CurveP384, CurveP521, p.ECDSACurve)
			}
		case KeyEd25519:
		default:
			return fmt.Errorf("key_type: must be %s, %s or %s, got %q", KeyRSA, KeyECDSA, KeyEd25519, p.KeyType)
		}
		if p.Days < 1 {
			return fmt.Errorf("days: must be >= 1, got %d", p.Days)
		}
		if p.RenewDays < 0 {
			return fmt.Errorf("renew_days: must be >= 0, got %d", p.RenewDays)
		}
		if (p.CACertPath == "") != (p.CAKeyPath == "") {
			return fmt.Errorf("ca_cert and ca_key must be set together (or both omitted for a self-signed certificate)")
		}
	case StateAbsent:
		var leaked []string
		for k := range p.seen {
			if _, ok := absentAllowedKeys[k]; !ok {
				leaked = append(leaked, k)
			}
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry certificate params: %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
