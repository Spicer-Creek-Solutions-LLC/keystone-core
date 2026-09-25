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

const (
	DefaultMaxUnauthorizedConnections = 32
	DefaultAuthorizationTimeout       = 5 * time.Second
	DefaultMaxAuthorizedConnections   = 64
	DefaultMaxFrameBytes              = 64 * 1024
	MaxUnauthorizedConnections        = 1024
	MaxAuthorizationTimeout           = 5 * time.Minute
	MaxAuthorizedConnections          = 4096
	MaxFrameBytes                     = 16 * 1024 * 1024
	MaxFaultDelay                     = 5 * time.Minute
)

type Server struct {
	Operator Operator
	Store    Store
	Faults   Faults
	NATS     NATS
	Service  Service
}

type Operator struct {
	AdminGroup                 string
	MaxUnauthorizedConnections int
	AuthorizationTimeout       time.Duration
	MaxAuthorizedConnections   int
	MaxFrameBytes              uint32
}

type Store struct{ Path string }

type Faults struct {
	OperatorHoldBeforeDecision time.Duration
	OperatorHoldBeforeResponse time.Duration
}

// NATS is how the server reaches the broker and which credential it uses for
// each role. D-C05-4: one explicit path per credential, never a directory, so
// the server reads the files it was told to and no other.
type NATS struct {
	URL                   string
	CAFile                string
	ServerName            string
	EnrollmentCredentials string
	PresenceCredentials   string
	AccountSigningSeed    string
}

// Service names the service keys ADR-0005 assigns the server: the envelope
// signing private key (AST-7) and the result service's encryption public half
// (AST-8). Their public halves reach agents in the token bundle.
type Service struct {
	SigningPrivateKeyFile         string
	ResultEncryptionPublicKeyFile string
}

// Enrollment reports whether the server is configured to enroll agents. The
// [nats] and [service] tables are all or nothing: a server with neither is the
// operator substrate alone, as C04 left it, and one with part of them is a
// configuration error rather than a partly working service.
func (c Server) Enrollment() bool { return c.NATS != (NATS{}) }

// enrollmentKeys are the keys that must all be present once any is.
var enrollmentKeys = []string{
	"nats.url", "nats.ca_file", "nats.server_name",
	"nats.enrollment_credentials", "nats.presence_credentials", "nats.account_signing_seed",
	"service.signing_private_key_file", "service.result_encryption_public_key_file",
}

func LoadServer(path string) (Server, error) {
	f, err := os.Open(path)
	if err != nil {
		return Server{}, err
	}
	defer f.Close()

	c := Server{Operator: Operator{
		MaxUnauthorizedConnections: DefaultMaxUnauthorizedConnections,
		AuthorizationTimeout:       DefaultAuthorizationTimeout,
		MaxAuthorizedConnections:   DefaultMaxAuthorizedConnections,
		MaxFrameBytes:              DefaultMaxFrameBytes,
	}}
	seen := make(map[string]bool)
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
			case "operator", "store", "faults", "nats", "service":
			default:
				return Server{}, fmt.Errorf("line %d: unknown table %q", line, section)
			}
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 || section == "" {
			return Server{}, fmt.Errorf("line %d: invalid configuration", line)
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		qualified := section + "." + key
		if seen[qualified] {
			return Server{}, fmt.Errorf("line %d: duplicate key %q", line, qualified)
		}
		seen[qualified] = true
		if err := setServerValue(&c, section, key, value); err != nil {
			return Server{}, fmt.Errorf("line %d: %w", line, err)
		}
	}
	if err := s.Err(); err != nil {
		return Server{}, err
	}
	if c.Operator.AdminGroup == "" {
		return Server{}, fmt.Errorf("operator.admin_group is required")
	}
	if c.Store.Path == "" {
		return Server{}, fmt.Errorf("store.path is required")
	}
	var present, missing []string
	for _, k := range enrollmentKeys {
		if seen[k] {
			present = append(present, k)
		} else {
			missing = append(missing, k)
		}
	}
	if len(present) > 0 && len(missing) > 0 {
		return Server{}, fmt.Errorf("enrollment is configured by %s but %s is missing", strings.Join(present, ", "), strings.Join(missing, ", "))
	}
	return c, nil
}

func setServerValue(c *Server, section, key, value string) error {
	q := section + "." + key
	if target := stringKey(c, q); target != nil {
		v, err := strconv.Unquote(value)
		if err != nil || v == "" {
			return fmt.Errorf("%s must be a non-empty quoted string", q)
		}
		if pathKey(q) && !filepath.IsAbs(v) {
			return fmt.Errorf("%w: %s=%q", ErrRelativePath, q, v)
		}
		*target = v
		return nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("%s must be a positive integer", q)
	}
	switch q {
	case "operator.max_unauthorized_connections":
		if n > MaxUnauthorizedConnections {
			return fmt.Errorf("%s exceeds maximum %d", q, MaxUnauthorizedConnections)
		}
		c.Operator.MaxUnauthorizedConnections = int(n)
	case "operator.authorization_timeout_ms":
		if n > MaxAuthorizationTimeout.Milliseconds() {
			return fmt.Errorf("%s exceeds maximum %d milliseconds", q, MaxAuthorizationTimeout.Milliseconds())
		}
		c.Operator.AuthorizationTimeout = time.Duration(n) * time.Millisecond
	case "operator.max_authorized_connections":
		if n > MaxAuthorizedConnections {
			return fmt.Errorf("%s exceeds maximum %d", q, MaxAuthorizedConnections)
		}
		c.Operator.MaxAuthorizedConnections = int(n)
	case "operator.max_frame_bytes":
		if n > MaxFrameBytes {
			return fmt.Errorf("%s exceeds maximum %d", q, MaxFrameBytes)
		}
		c.Operator.MaxFrameBytes = uint32(n)
	case "faults.operator_hold_before_decision_ms", "faults.operator_hold_before_response_ms":
		if n > MaxFaultDelay.Milliseconds() {
			return fmt.Errorf("%s exceeds maximum %d milliseconds", q, MaxFaultDelay.Milliseconds())
		}
		d := time.Duration(n) * time.Millisecond
		if q == "faults.operator_hold_before_decision_ms" {
			c.Faults.OperatorHoldBeforeDecision = d
		} else {
			c.Faults.OperatorHoldBeforeResponse = d
		}
	default:
		return fmt.Errorf("unknown key %q", q)
	}
	return nil
}

// stringKey returns where a quoted-string key is stored, or nil for a key that
// is not one.
func stringKey(c *Server, q string) *string {
	switch q {
	case "operator.admin_group":
		return &c.Operator.AdminGroup
	case "store.path":
		return &c.Store.Path
	case "nats.url":
		return &c.NATS.URL
	case "nats.ca_file":
		return &c.NATS.CAFile
	case "nats.server_name":
		return &c.NATS.ServerName
	case "nats.enrollment_credentials":
		return &c.NATS.EnrollmentCredentials
	case "nats.presence_credentials":
		return &c.NATS.PresenceCredentials
	case "nats.account_signing_seed":
		return &c.NATS.AccountSigningSeed
	case "service.signing_private_key_file":
		return &c.Service.SigningPrivateKeyFile
	case "service.result_encryption_public_key_file":
		return &c.Service.ResultEncryptionPublicKeyFile
	}
	return nil
}

// pathKey reports whether a string key names a file. Every such path must be
// absolute, for the reason ErrRelativePath gives about the configuration file
// itself: a relative one resolves against the working directory.
func pathKey(q string) bool {
	switch q {
	case "operator.admin_group", "nats.url", "nats.server_name":
		return false
	}
	return true
}

func stripComment(line string) string {
	quoted, escaped := false, false
	for i, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if quoted && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			continue
		}
		if r == '#' && !quoted {
			return line[:i]
		}
	}
	return line
}
