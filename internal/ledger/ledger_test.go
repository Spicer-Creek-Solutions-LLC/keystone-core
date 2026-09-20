package ledger

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStartRequiresReceipt(t *testing.T) {
	l, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	err = l.RecordStart(context.Background(), Start{JobID: "missing", StartedAt: time.Now()})
	if err == nil {
		t.Fatal("start accepted without receipt")
	}
}
