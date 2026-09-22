package sockprobe

import (
	"bytes"
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
	t.Cleanup(func() { syscall.Close(server) })
	return client, server
}

// The instrument's negative: a peer that reads nothing leaves the count where
// the payload put it. Without this, a count that never moves would satisfy
// every ordering case.
func TestAPeerThatReadsNothingLeavesTheCountUnchanged(t *testing.T) {
	c, _ := pair(t)
	if err := SendSingly(c, []byte("sixteen-bytes-ok")); err != nil {
		t.Fatal(err)
	}
	w, err := Observe(c, 200*time.Millisecond, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.InitialOutQ == 0 {
		t.Fatal("the count is zero with sixteen unread bytes queued; SIOCOUTQ is not measuring the peer's consumption")
	}
	if w.FirstDropMS != -1 {
		t.Fatalf("the count dropped at %dms though the peer read nothing: %+v", w.FirstDropMS, w.Samples)
	}
}

// The instrument's positive, at the resolution the ordering cases need: ONE
// byte read by the peer moves the count, and it moves when the read happens.
func TestReadingASingleByteMovesTheCount(t *testing.T) {
	c, s := pair(t)
	if err := SendSingly(c, []byte("sixteen-bytes-ok")); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		syscall.Read(s, make([]byte, 1))
	}()
	w, err := Observe(c, 600*time.Millisecond, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.FirstDropMS < 0 {
		t.Fatalf("a one-byte read did not move the count: %+v", w)
	}
	if w.FirstDropMS < 100 {
		t.Fatalf("the count dropped at %dms, before the read at 150ms", w.FirstDropMS)
	}
}

// Why SendSingly exists. A partial read of one multi-byte write consumes no
// whole skb, so the count cannot see it; a probe that sent its payload in one
// write would miss a server that read one byte before authorizing.
func TestAPartialReadOfOneLargeWriteIsInvisible(t *testing.T) {
	c, s := pair(t)
	if _, err := syscall.Write(c, bytes.Repeat([]byte("x"), 100)); err != nil {
		t.Fatal(err)
	}
	if _, err := syscall.Read(s, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	w, err := Observe(c, 100*time.Millisecond, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.FirstDropMS != -1 || w.InitialOutQ == 0 {
		t.Fatalf("a partial read became visible (%+v); SendSingly's premise changed and the package comment is wrong", w)
	}
}

// The stated limit, asserted so that it cannot silently stop being true in
// either direction: a peek consumes nothing and the count cannot see it.
func TestAPeekIsInvisible(t *testing.T) {
	c, s := pair(t)
	if err := SendSingly(c, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := syscall.Recvfrom(s, make([]byte, 1), syscall.MSG_PEEK); err != nil {
		t.Fatal(err)
	}
	w, err := Observe(c, 100*time.Millisecond, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if w.FirstDropMS != -1 {
		t.Fatalf("a peek moved the count (%+v); the instrument can now see peeks and C04-A's stated limit is stale", w)
	}
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
	if w.EOFMS < 0 || w.Trailing != 0 {
		t.Fatalf("eof %d, trailing %d", w.EOFMS, w.Trailing)
	}
}

func TestErrnoNameIsStable(t *testing.T) {
	_, err := Dial(filepath.Join(t.TempDir(), "absent"))
	if got := ErrnoName(err); got != "ENOENT" {
		t.Fatalf("dialling an absent path: %q", got)
	}
}
