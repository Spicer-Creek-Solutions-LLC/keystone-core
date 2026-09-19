package protocol

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// corpusDir holds raw byte files rather than Go's fuzz-corpus format, because
// two readers need them: this target seeds from it, and the acceptance
// contract's AC-13 feeds every entry to the parser directly. One directory,
// read the same way by both, so a crasher added here is a crasher the contract
// sees.
const corpusDir = "testdata/fuzz"

func corpus(t testing.TB) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = b
	}
	if len(out) == 0 {
		t.Fatal("the seed corpus is empty; AC-13 would pass by checking nothing")
	}
	return out
}

// FuzzDecode drives the parser. D-C01-4 schedules the seed corpus on every pull
// request and leaves the timed search owed: a fixed -fuzztime is a budget, not
// a property, and a fuzzer that crashes on one run and not the next makes every
// pull request a coin flip on a serial runner.
func FuzzDecode(f *testing.F) {
	for _, b := range corpus(f) {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		e, err := Decode(b, DefaultMaxEnvelope)
		if err != nil {
			// Every refusal must be one of § 9's codes. An error of any other
			// type would be detail the envelope's sender could learn from.
			var r Refusal
			if !errors.As(err, &r) {
				t.Fatalf("Decode returned %T (%v); every refusal is a Refusal", err, err)
			}
			return
		}
		// Anything accepted must re-encode to the same bytes. A parser that
		// accepts an input it cannot reproduce has accepted two encodings of one
		// envelope, which is the ambiguity § 1 exists to remove.
		again, ok := e.Encode()
		if !ok {
			t.Fatal("an accepted envelope failed to re-encode")
		}
		if string(again) != string(b) {
			t.Fatalf("accepted input does not round-trip:\n in %x\nout %x", b, again)
		}
	})
}
