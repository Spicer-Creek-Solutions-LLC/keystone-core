package blueprint

import (
	"strings"
	"testing"
)

func TestRenderState(t *testing.T) {
	ctx := NewRenderContext(
		ResolvedParams{Values: map[string]any{"name": "web", "replicas": int64(3)}},
		map[string]bool{"tls": true},
	)

	t.Run("params and features resolve", func(t *testing.T) {
		out, err := RenderState(`name={{ .Params.name }} n={{ .Params.replicas }} tls={{ .Features.tls }}`, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if out != "name=web n=3 tls=true" {
			t.Fatalf("out=%q", out)
		}
	})

	t.Run("statemgmt funcmap is available", func(t *testing.T) {
		out, err := RenderState(`{{ upper .Params.name }}|{{ trim "  x  " }}|{{ default "z" "" }}`, ctx)
		if err != nil {
			t.Fatal(err)
		}
		if out != "WEB|x|z" {
			t.Fatalf("out=%q", out)
		}
	})

	t.Run("missing key fails loudly", func(t *testing.T) {
		_, err := RenderState(`{{ .Params.nope }}`, ctx)
		if err == nil || !strings.Contains(err.Error(), "render state") {
			t.Fatalf("expected loud missing-key error, got %v", err)
		}
	})

	t.Run("parse error surfaces", func(t *testing.T) {
		if _, err := RenderState(`{{ .Params.name `, ctx); err == nil {
			t.Fatal("expected parse error")
		}
	})
}
