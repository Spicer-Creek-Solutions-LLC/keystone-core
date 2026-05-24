// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

// SecretRefScheme is the URI scheme marking a parameter value as a
// secret reference: secret://<path>[#<field>].
const SecretRefScheme = "secret://"

// ErrSecretResolverRequired is returned by Apply when the manifest has
// a source:secret parameter with a secret:// input but no
// SecretResolver is configured.
var ErrSecretResolverRequired = errors.New("blueprint: secret resolver required for source:secret parameter")

// SecretResolver turns a secret:// reference into its cleartext value.
type SecretResolver interface {
	ResolveSecret(ctx context.Context, ref string) (string, error)
}

// secretGetter is the subset of *secrets.Broker the broker adapter
// needs. *secrets.Broker satisfies it.
type secretGetter interface {
	GetSecret(ctx context.Context, req secrets.GetSecretRequest) (*secrets.Secret, error)
}

// BrokerSecretResolver adapts a secrets broker to SecretResolver.
// A reference is secret://<path>[#<field>]; <field> selects a key in
// the secret's Data. When omitted: a single-key secret uses that key;
// otherwise a "value" key is used; otherwise it is an error.
type BrokerSecretResolver struct {
	Broker secretGetter
}

// NewBrokerSecretResolver wraps b.
func NewBrokerSecretResolver(b secretGetter) *BrokerSecretResolver {
	return &BrokerSecretResolver{Broker: b}
}

// ResolveSecret implements SecretResolver.
func (r *BrokerSecretResolver) ResolveSecret(ctx context.Context, ref string) (string, error) {
	if r == nil || r.Broker == nil {
		return "", ErrSecretResolverRequired
	}
	path, field, err := parseSecretRef(ref)
	if err != nil {
		return "", err
	}
	sec, err := r.Broker.GetSecret(ctx, secrets.GetSecretRequest{Path: path})
	if err != nil {
		return "", fmt.Errorf("blueprint: resolve secret %q: %w", path, err)
	}
	val, err := pickSecretField(sec, field)
	if err != nil {
		return "", fmt.Errorf("blueprint: secret %q: %w", path, err)
	}
	return val, nil
}

// IsSecretRef reports whether v is a secret:// reference.
func IsSecretRef(v string) bool {
	return strings.HasPrefix(v, SecretRefScheme)
}

func parseSecretRef(ref string) (path, field string, err error) {
	if !IsSecretRef(ref) {
		return "", "", fmt.Errorf("blueprint: not a secret reference: %q", ref)
	}
	rest := strings.TrimPrefix(ref, SecretRefScheme)
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		path, field = rest[:i], rest[i+1:]
	} else {
		path = rest
	}
	if path == "" {
		return "", "", fmt.Errorf("blueprint: secret reference has empty path: %q", ref)
	}
	return path, field, nil
}

func pickSecretField(sec *secrets.Secret, field string) (string, error) {
	if sec == nil || len(sec.Data) == 0 {
		return "", errors.New("secret has no data")
	}
	if field != "" {
		v, ok := sec.Data[field]
		if !ok {
			return "", fmt.Errorf("field %q not present", field)
		}
		return fmt.Sprint(v), nil
	}
	if len(sec.Data) == 1 {
		for _, v := range sec.Data {
			return fmt.Sprint(v), nil
		}
	}
	if v, ok := sec.Data["value"]; ok {
		return fmt.Sprint(v), nil
	}
	return "", errors.New("ambiguous secret: specify #field (multi-key secret, no \"value\" key)")
}
