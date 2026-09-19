package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func nine(payload []byte) [][]byte {
	f := make([][]byte, Fields)
	for i := range f {
		f[i] = []byte{}
	}
	f[7] = payload
	return f
}

func TestFrameRejectsWrongFieldCount(t *testing.T) {
	for _, n := range []int{0, 1, 8, 10} {
		if _, ok := Frame(make([][]byte, n)); ok {
			t.Errorf("Frame accepted %d fields; ADR-0005 § 1 fixes nine", n)
		}
	}
}

// The prefix is part of the signed input, and ADR-0005 § 1 gives the reason:
// sign the values alone and two different field splits are the same bytes.
func TestSignedInputBindsTheFieldSplit(t *testing.T) {
	a := nine(nil)
	a[2], a[3] = []byte("ab"), []byte("cd")
	b := nine(nil)
	b[2], b[3] = []byte("a"), []byte("bcd")

	sa, ok := SignedInput(a)
	if !ok {
		t.Fatal("SignedInput refused a well-formed field set")
	}
	sb, _ := SignedInput(b)
	if bytes.Equal(sa, sb) {
		t.Error("two different field splits produced identical signed input; the prefix is not bound")
	}
	if bytes.Equal(append([]byte("ab"), "cd"...), append([]byte("a"), "bcd"...)) != true {
		t.Fatal("fixture is wrong: the values-only concatenations should be equal")
	}
}

func TestSignedInputStopsBeforeFieldNine(t *testing.T) {
	f := nine([]byte("payload"))
	f[8] = []byte("SIGNATURE")
	si, ok := SignedInput(f)
	if !ok {
		t.Fatal("SignedInput refused")
	}
	if bytes.Contains(si, []byte("SIGNATURE")) {
		t.Error("field 9 is inside the signed input; it must cover fields 1 to 8 only")
	}
	full, _ := Frame(f)
	if len(si) != len(full)-PrefixBytes-len("SIGNATURE") {
		t.Errorf("signed input is %d bytes; want the framing less field 9", len(si))
	}
}

// The check order is normative because the refusal code depends on it.
func TestSplitCheckOrder(t *testing.T) {
	framed, _ := Frame(nine([]byte("hello")))

	for _, tc := range []struct {
		name string
		in   []byte
		max  int
		want Refusal
	}{
		{"truncated prefix", framed[:2], DefaultMaxEnvelope, MalformedEnvelope},
		{"truncated field", framed[:len(framed)-2], DefaultMaxEnvelope, MalformedEnvelope},
		{"trailing bytes after field 9", append(append([]byte{}, framed...), 0x00), DefaultMaxEnvelope, MalformedEnvelope},
		{"budget exhausted", framed, 8, PayloadTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Split(tc.in, tc.max)
			var got Refusal
			if !errors.As(err, &got) {
				t.Fatalf("err = %v, want a Refusal", err)
			}
			if got != tc.want {
				t.Errorf("refusal = %v, want %v", got, tc.want)
			}
		})
	}
}

// An over-budget length is PayloadTooLarge whether or not the bytes are there,
// because the budget check precedes the presence check.
func TestOverBudgetLengthIsTooLargeEvenWhenBytesAreAbsent(t *testing.T) {
	var b []byte
	var p [PrefixBytes]byte
	binary.BigEndian.PutUint32(p[:], 1<<30) // declares 1 GiB, supplies nothing
	b = append(b, p[:]...)
	_, err := Split(b, DefaultMaxEnvelope)
	var got Refusal
	if !errors.As(err, &got) || got != PayloadTooLarge {
		t.Fatalf("err = %v, want payload too large", err)
	}
}

// A parser that allocated on the strength of an unchecked prefix would be a
// denial of service reachable by anyone who can publish.
func TestSplitDoesNotAllocateOnAnUncheckedPrefix(t *testing.T) {
	var b []byte
	var p [PrefixBytes]byte
	binary.BigEndian.PutUint32(p[:], ^uint32(0)) // nearly 4 GiB
	b = append(b, p[:]...)
	before := testing.AllocsPerRun(1, func() { _, _ = Split(b, DefaultMaxEnvelope) })
	if before > 8 {
		t.Errorf("Split made %v allocations refusing a 4 GiB claim", before)
	}
}

func TestZeroLengthFieldIsRepresentableAndDistinctFromAbsent(t *testing.T) {
	f := nine(nil)
	framed, ok := Frame(f)
	if !ok {
		t.Fatal("Frame refused all-empty fields")
	}
	if len(framed) != Fields*PrefixBytes {
		t.Fatalf("all-empty framing is %d bytes; want %d", len(framed), Fields*PrefixBytes)
	}
	back, err := Split(framed, DefaultMaxEnvelope)
	if err != nil {
		t.Fatalf("Split refused: %v", err)
	}
	for i, v := range back {
		if v == nil {
			t.Errorf("field %d decoded as nil; an empty field is present and zero-length", i+1)
		}
	}
}
