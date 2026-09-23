package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxUnauthorizedConnections = 32
	DefaultAuthorizationTimeout       = 5 * time.Second
	DefaultMaxAuthorizedConnections   = 64
	DefaultMaxFrameBytes              = 64 * 1024
)

type Server struct {
	Operator Operator
	Store    Store
	Faults   Faults
}

type Operator struct {
	AdminGroup                 string
	MaxUnauthorizedConnections int
	AuthorizationTimeout       time.Duration
	MaxAuthorizedConnections   int
	MaxFrameBytes              uint32
}

type Store struct{ Path string }

type Faults struct{ OperatorHoldBeforeDecision time.Duration }

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
		raw := strings.TrimSpace(strings.SplitN(s.Text(), "#", 2)[0])
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
			section = strings.TrimSpace(raw[1 : len(raw)-1])
			if section != "operator" && section != "store" && section != "faults" {
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
	return c, nil
}

func setServerValue(c *Server, section, key, value string) error {
	q := section + "." + key
	switch q {
	case "operator.admin_group", "store.path":
		v, err := strconv.Unquote(value)
		if err != nil || v == "" {
			return fmt.Errorf("%s must be a non-empty quoted string", q)
		}
		if q == "operator.admin_group" {
			c.Operator.AdminGroup = v
		} else {
			c.Store.Path = v
		}
		return nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("%s must be a positive integer", q)
	}
	switch q {
	case "operator.max_unauthorized_connections":
		c.Operator.MaxUnauthorizedConnections = int(n)
	case "operator.authorization_timeout_ms":
		c.Operator.AuthorizationTimeout = time.Duration(n) * time.Millisecond
	case "operator.max_authorized_connections":
		c.Operator.MaxAuthorizedConnections = int(n)
	case "operator.max_frame_bytes":
		if n > int64(^uint32(0)) {
			return fmt.Errorf("%s exceeds uint32 framing", q)
		}
		c.Operator.MaxFrameBytes = uint32(n)
	case "faults.operator_hold_before_decision_ms":
		c.Faults.OperatorHoldBeforeDecision = time.Duration(n) * time.Millisecond
	default:
		return fmt.Errorf("unknown key %q", q)
	}
	return nil
}
