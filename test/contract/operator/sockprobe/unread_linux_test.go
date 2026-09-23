//go:build linux

package sockprobe

import (
	"syscall"
	"testing"
	"time"
)

// These assert, on the running kernel, every property unread_linux.go's
// ConsumptionObservable claims. The ordering case rests on them; a kernel that
// changed any of them must fail here, not pass that case.

const denial = `{"error":"authorization_denied"}`

// refuse is a server that answers and closes, having first done read to the
// client's bytes.
func refuse(t *testing.T, s int, read func(int)) {
	t.Helper()
	time.Sleep(20 * time.Millisecond)
	read(s)
	if _, err := syscall.Write(s, Frame([]byte(denial))); err != nil {
		t.Fatal(err)
	}
	syscall.Close(s)
}

func observe(t *testing.T, payload string, read func(int)) Watch {
	t.Helper()
	c, s := pair(t)
	if _, err := syscall.Write(c, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	refuse(t, s, read)
	w, err := Observe(c, time.Second, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Frames) != 1 || w.Frames[0] != denial {
		t.Fatalf("the frame written before close was not delivered: %+v", w)
	}
	return w
}

func TestAnUnconsumedByteEndsInReset(t *testing.T) {
	if w := observe(t, "x", func(int) {}); w.Ended != EndedReset {
		t.Fatalf("a close with one unread byte ended %q, not %q", w.Ended, EndedReset)
	}
}

func TestAConsumedByteEndsInEOF(t *testing.T) {
	w := observe(t, "x", func(s int) { syscall.Read(s, make([]byte, 1)) })
	if w.Ended != EndedEOF {
		t.Fatalf("a close with nothing unread ended %q, not %q", w.Ended, EndedEOF)
	}
}

// Why the ordering case sends ONE byte: a server that consumes a prefix of a
// longer payload leaves the rest queued, and the reset hides the read.
func TestAPartialReadOfALongerPayloadStillEndsInReset(t *testing.T) {
	w := observe(t, "\x00\x00\x00\x05hello", func(s int) { syscall.Read(s, make([]byte, 4)) })
	if w.Ended != EndedReset {
		t.Fatalf("ended %q; the premise for sending one byte has changed", w.Ended)
	}
}

// The stated limit, asserted so that it cannot silently stop being true in
// either direction: a peek consumes nothing and ends in a reset.
func TestAPeekIsInvisible(t *testing.T) {
	w := observe(t, "x", func(s int) { syscall.Recvfrom(s, make([]byte, 1), syscall.MSG_PEEK) })
	if w.Ended != EndedReset {
		t.Fatalf("a peek ended %q; the instrument can now see peeks and C04-A's stated limit is stale", w.Ended)
	}
}

func TestConsumptionIsObservableHere(t *testing.T) {
	if !ConsumptionObservable() {
		t.Fatal("unread_linux.go is built but reports no instrument")
	}
}
