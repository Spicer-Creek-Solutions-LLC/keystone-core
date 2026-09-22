package sockprobe

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// pair connects a client to a listener this test owns, and returns both ends.
// The server end is whatever the test does with it: the instrument is proven
// against a peer whose reads the test controls exactly.
func pair(t *testing.T) (client, server int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Close(l) })
	client, err = Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Close(client) })
	server, _, err = syscall.Accept(l)
	if err != nil {
		t.Fatal(err)
	}
	return client, server
}

func TestObserveReadsFramesAndEOF(t *testing.T) {
	c, s := pair(t)
	resp := append(Frame([]byte(`{"error":"a"}`)), Frame([]byte(`{"error":"b"}`))...)
	if _, err := syscall.Write(s, resp); err != nil {
		t.Fatal(err)
	}
	syscall.Close(s)
	w, err := Observe(c, time.Second, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Frames) != 2 || w.Frames[0] != `{"error":"a"}` || w.Frames[1] != `{"error":"b"}` {
		t.Fatalf("frames: %q", w.Frames)
	}
	if w.Ended != EndedEOF || w.EOFMS < 0 || w.Trailing != 0 {
		t.Fatalf("%+v", w)
	}
}

func TestObserveStopsAtTheBudgetOnAnOpenConnection(t *testing.T) {
	c, s := pair(t)
	t.Cleanup(func() { syscall.Close(s) })
	w, err := Observe(c, 100*time.Millisecond, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.Ended != EndedOpen || w.EOFMS != -1 {
		t.Fatalf("%+v", w)
	}
}

func TestErrnoNameIsStable(t *testing.T) {
	_, err := Dial(filepath.Join(t.TempDir(), "absent"))
	if got := ErrnoName(err); got != "ENOENT" {
		t.Fatalf("dialling an absent path: %q", got)
	}
}
