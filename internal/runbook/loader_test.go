package runbook

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const validRunbookYAML = `
metadata:
  name: db-restart
  version: 1.0.0
spec:
  inputs:
    - name: agent_id
      required: true
  steps:
    - type: noop
      name: stop
    - type: noop
      name: start
      depends_on: [stop]
`

func writeRunbook(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rb.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		rb, err := Load(writeRunbook(t, validRunbookYAML))
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if rb.Metadata.Name != "db-restart" || len(rb.Spec.Steps) != 2 {
			t.Fatalf("parsed wrong: %+v", rb)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err=%v want ErrNotFound", err)
		}
	})

	t.Run("strict unknown field", func(t *testing.T) {
		_, err := Load(writeRunbook(t, validRunbookYAML+"\nbogus: 1\n"))
		if err == nil || errors.Is(err, ErrNotFound) {
			t.Fatalf("expected strict decode error, got %v", err)
		}
	})

	t.Run("invalid runbook", func(t *testing.T) {
		_, err := Load(writeRunbook(t, "metadata:\n  name: x\nspec:\n  steps: []\n"))
		if !errors.Is(err, ErrInvalidRunbook) {
			t.Fatalf("err=%v want ErrInvalidRunbook", err)
		}
	})
}
