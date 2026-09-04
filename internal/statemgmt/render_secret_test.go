// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"errors"
	"strings"
	"testing"
)

func TestRenderer_SecretFunc(t *testing.T) {
	r := NewRendererWithSecrets(func(path, key string) (string, error) {
		if path == "app/db" && key == "password" {
			return "s3cret", nil
		}
		if path == "app/token" && key == "" {
			return "t0ken", nil
		}
		return "", errors.New("no such secret")
	})

	tests := []struct {
		name string
		tpl  string
		want string
	}{
		{"path and key", `{{ secret "app/db" "password" }}`, "s3cret"},
		{"path only", `{{ secret "app/token" }}`, "t0ken"},
		{"inside a larger string", `DB_PASS={{ secret "app/db" "password" }}`, "DB_PASS=s3cret"},
		{"composed with another func", `{{ secret "app/db" "password" | upper }}`, "S3CRET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.RenderString(tt.tpl, renderContext{})
			if err != nil {
				t.Fatalf("RenderString: %v", err)
			}
			if got != tt.want {
				t.Errorf("RenderString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A resolver error must fail the render. Rendering an empty string
// would write a config file with a blank password and report success.
func TestRenderer_SecretErrorFailsTheRender(t *testing.T) {
	r := NewRendererWithSecrets(func(string, string) (string, error) {
		return "", errors.New("denied: path not granted to this agent")
	})
	_, err := r.RenderString(`{{ secret "app/db" "password" }}`, renderContext{})
	if err == nil {
		t.Fatal("RenderString() error = nil, want the resolver's error")
	}
	if !strings.Contains(err.Error(), "not granted") {
		t.Errorf("error = %v, want it to carry the resolver's reason", err)
	}
}

// Without a resolver the func still exists, so the failure names the
// real problem instead of reading like a typo.
func TestRenderer_SecretWithoutResolver(t *testing.T) {
	_, err := NewRenderer().RenderString(`{{ secret "app/db" "password" }}`, renderContext{})
	if err == nil {
		t.Fatal("RenderString() error = nil, want ErrSecretsUnavailable")
	}
	if !strings.Contains(err.Error(), "not available here") {
		t.Errorf("error = %v, want the unavailable-resolver message", err)
	}
	if strings.Contains(err.Error(), "not defined") {
		t.Errorf("error reads like an undefined function: %v", err)
	}
}

func TestRenderer_SecretArgumentErrors(t *testing.T) {
	r := NewRendererWithSecrets(func(string, string) (string, error) { return "v", nil })

	t.Run("empty path", func(t *testing.T) {
		if _, err := r.RenderString(`{{ secret "" "k" }}`, renderContext{}); err == nil {
			t.Error("RenderString() error = nil for an empty path")
		}
	})

	t.Run("too many keys", func(t *testing.T) {
		if _, err := r.RenderString(`{{ secret "p" "a" "b" }}`, renderContext{}); err == nil {
			t.Error("RenderString() error = nil for two keys")
		}
	})
}

// The resolver is called once per occurrence, every time — a state
// file that references the same secret twice performs two lookups, so
// nothing is memoised behind the operator's back.
func TestRenderer_SecretIsResolvedPerUse(t *testing.T) {
	var calls int
	r := NewRendererWithSecrets(func(string, string) (string, error) {
		calls++
		return "v", nil
	})
	if _, err := r.RenderString(`{{ secret "p" "k" }}{{ secret "p" "k" }}`, renderContext{}); err != nil {
		t.Fatalf("RenderString: %v", err)
	}
	if calls != 2 {
		t.Errorf("resolver called %d times, want 2", calls)
	}
}

// The declaration name is rendered too, so a secret can appear in a
// file path -- and a failure there must fail the declaration.
func TestRenderer_SecretInStateFile(t *testing.T) {
	sf, err := Parse([]byte(`metadata:
  name: app-env
  version: "1.0"

file:
  /etc/app.env:
    state: present
    content: "DB_PASS={{ secret \"app/db\" \"password\" }}\n"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	r := NewRendererWithSecrets(func(path, key string) (string, error) {
		return "s3cret", nil
	})
	rendered, err := r.RenderStateFile(sf, map[string]any{})
	if err != nil {
		t.Fatalf("RenderStateFile: %v", err)
	}
	if len(rendered.Declarations) != 1 {
		t.Fatalf("declarations = %d, want 1", len(rendered.Declarations))
	}
	content, _ := rendered.Declarations[0].Params["content"].(string)
	if content != "DB_PASS=s3cret\n" {
		t.Errorf("content = %q, want %q", content, "DB_PASS=s3cret\n")
	}
}
