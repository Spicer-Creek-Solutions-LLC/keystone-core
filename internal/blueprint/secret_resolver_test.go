// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

type fakeGetter struct {
	sec *secrets.Secret
	err error
}

func (f fakeGetter) GetSecret(context.Context, secrets.GetSecretRequest) (*secrets.Secret, error) {
	return f.sec, f.err
}

func TestBrokerSecretResolver(t *testing.T) {
	ctx := context.Background()

	t.Run("single-key secret", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{sec: &secrets.Secret{Data: map[string]any{"k": "v1"}}})
		got, err := r.ResolveSecret(ctx, "secret://kv/db")
		if err != nil || got != "v1" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})

	t.Run("explicit #field", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{sec: &secrets.Secret{Data: map[string]any{"user": "u", "pass": "p"}}})
		got, err := r.ResolveSecret(ctx, "secret://kv/db#pass")
		if err != nil || got != "p" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})

	t.Run("value key fallback", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{sec: &secrets.Secret{Data: map[string]any{"value": "vv", "meta": "m"}}})
		got, err := r.ResolveSecret(ctx, "secret://kv/db")
		if err != nil || got != "vv" {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})

	t.Run("ambiguous multi-key", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{sec: &secrets.Secret{Data: map[string]any{"a": "1", "b": "2"}}})
		if _, err := r.ResolveSecret(ctx, "secret://kv/db"); err == nil {
			t.Fatal("expected ambiguity error")
		}
	})

	t.Run("missing field", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{sec: &secrets.Secret{Data: map[string]any{"a": "1"}}})
		if _, err := r.ResolveSecret(ctx, "secret://kv/db#zzz"); err == nil {
			t.Fatal("expected missing-field error")
		}
	})

	t.Run("not a ref", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{sec: &secrets.Secret{Data: map[string]any{"a": "1"}}})
		if _, err := r.ResolveSecret(ctx, "plain"); err == nil {
			t.Fatal("expected not-a-ref error")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{sec: &secrets.Secret{Data: map[string]any{"a": "1"}}})
		if _, err := r.ResolveSecret(ctx, "secret://"); err == nil {
			t.Fatal("expected empty-path error")
		}
	})

	t.Run("broker error", func(t *testing.T) {
		r := NewBrokerSecretResolver(fakeGetter{err: errors.New("vault down")})
		if _, err := r.ResolveSecret(ctx, "secret://kv/db"); err == nil {
			t.Fatal("expected broker error")
		}
	})

	t.Run("nil broker", func(t *testing.T) {
		r := &BrokerSecretResolver{}
		if _, err := r.ResolveSecret(ctx, "secret://x"); !errors.Is(err, ErrSecretResolverRequired) {
			t.Fatalf("err=%v", err)
		}
	})
}
