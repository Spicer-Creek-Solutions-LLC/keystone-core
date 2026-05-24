// SPDX-License-Identifier: Apache-2.0

package age

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	upstream "filippo.io/age"
)

func newIdentity(t *testing.T) *upstream.X25519Identity {
	t.Helper()
	id, err := upstream.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity: %v", err)
	}
	return id
}

func encryptDecrypt(t *testing.T, payload []byte, enc *Encrypter, dec *Decrypter) []byte {
	t.Helper()
	var buf bytes.Buffer
	wc, err := enc.Wrap(&buf)
	if err != nil {
		t.Fatalf("enc.Wrap: %v", err)
	}
	if _, err := wc.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := dec.Wrap(&buf)
	if err != nil {
		t.Fatalf("dec.Wrap: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return got
}

func TestRoundTrip(t *testing.T) {
	id := newIdentity(t)
	enc := &Encrypter{Recipients: []upstream.Recipient{id.Recipient()}}
	dec := &Decrypter{Identities: []upstream.Identity{id}}

	payload := []byte("Keystone Core backup payload")
	got := encryptDecrypt(t, payload, enc, dec)
	if !bytes.Equal(got, payload) {
		t.Errorf("round-trip got %q, want %q", got, payload)
	}
}

func TestRoundTrip_Empty(t *testing.T) {
	id := newIdentity(t)
	enc := &Encrypter{Recipients: []upstream.Recipient{id.Recipient()}}
	dec := &Decrypter{Identities: []upstream.Identity{id}}

	got := encryptDecrypt(t, []byte{}, enc, dec)
	if len(got) != 0 {
		t.Errorf("round-trip of empty: got %d bytes, want 0", len(got))
	}
}

func TestMultiRecipient(t *testing.T) {
	idA := newIdentity(t)
	idB := newIdentity(t)
	enc := &Encrypter{Recipients: []upstream.Recipient{idA.Recipient(), idB.Recipient()}}

	payload := []byte("multi-recipient artifact")

	// Either identity should decrypt.
	for name, id := range map[string]*upstream.X25519Identity{"A": idA, "B": idB} {
		t.Run(name, func(t *testing.T) {
			dec := &Decrypter{Identities: []upstream.Identity{id}}
			got := encryptDecrypt(t, payload, enc, dec)
			if !bytes.Equal(got, payload) {
				t.Errorf("got %q, want %q", got, payload)
			}
		})
	}
}

func TestWrongIdentity(t *testing.T) {
	idA := newIdentity(t)
	idB := newIdentity(t)
	enc := &Encrypter{Recipients: []upstream.Recipient{idA.Recipient()}}
	dec := &Decrypter{Identities: []upstream.Identity{idB}}

	var buf bytes.Buffer
	wc, err := enc.Wrap(&buf)
	if err != nil {
		t.Fatalf("enc.Wrap: %v", err)
	}
	if _, err := wc.Write([]byte("secret")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := dec.Wrap(&buf); err == nil {
		t.Fatal("Wrap with wrong identity: want error")
	}
}

func TestStreamingChunks(t *testing.T) {
	id := newIdentity(t)
	enc := &Encrypter{Recipients: []upstream.Recipient{id.Recipient()}}
	dec := &Decrypter{Identities: []upstream.Identity{id}}

	var buf bytes.Buffer
	wc, err := enc.Wrap(&buf)
	if err != nil {
		t.Fatalf("enc.Wrap: %v", err)
	}

	want := bytes.Repeat([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ\n"), 200) // ~5.4 KB
	chunkSize := 128
	for off := 0; off < len(want); off += chunkSize {
		end := off + chunkSize
		if end > len(want) {
			end = len(want)
		}
		if _, err := wc.Write(want[off:end]); err != nil {
			t.Fatalf("Write at %d: %v", off, err)
		}
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := dec.Wrap(&buf)
	if err != nil {
		t.Fatalf("dec.Wrap: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("streamed round-trip mismatch (len got=%d want=%d)", len(got), len(want))
	}
}

func TestEncrypter_NoRecipients(t *testing.T) {
	enc := &Encrypter{}
	if _, err := enc.Wrap(io.Discard); err == nil {
		t.Fatal("want error for empty Recipients")
	}
}

func TestDecrypter_NoIdentities(t *testing.T) {
	dec := &Decrypter{}
	if _, err := dec.Wrap(strings.NewReader("anything")); err == nil {
		t.Fatal("want error for empty Identities")
	}
}

// ---- key file helpers ----

func writeIdentityFile(t *testing.T, id *upstream.X25519Identity) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "key.txt")
	body := "# created: 2026-05-20T19:30:00Z\n# public key: " + id.Recipient().String() + "\n" + id.String() + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeRecipientsFile(t *testing.T, recs ...upstream.Recipient) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "recipients.txt")
	var b strings.Builder
	b.WriteString("# kscore-server backup recipients\n")
	for _, r := range recs {
		b.WriteString(r.(*upstream.X25519Recipient).String())
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadIdentityFile(t *testing.T) {
	id := newIdentity(t)
	path := writeIdentityFile(t, id)
	ids, err := LoadIdentityFile(path)
	if err != nil {
		t.Fatalf("LoadIdentityFile: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("got %d identities, want 1", len(ids))
	}
	got, ok := ids[0].(*upstream.X25519Identity)
	if !ok {
		t.Fatalf("identity type = %T, want *X25519Identity", ids[0])
	}
	if got.String() != id.String() {
		t.Errorf("identity mismatch")
	}
}

func TestLoadIdentityFile_Missing(t *testing.T) {
	_, err := LoadIdentityFile(filepath.Join(t.TempDir(), "nope.key"))
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", err)
	}
}

func TestLoadIdentityFile_Malformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(path, []byte("not an age key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadIdentityFile(path)
	if err == nil {
		t.Fatal("want parse error")
	}
}

func TestLoadIdentityFile_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.key")
	if err := os.WriteFile(path, []byte("# only a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadIdentityFile(path)
	if err == nil {
		t.Fatal("want 'no identities' error")
	}
	if !strings.Contains(err.Error(), "no identities") {
		t.Errorf("err = %v, want 'no identities'", err)
	}
}

func TestLoadRecipientsFile(t *testing.T) {
	idA := newIdentity(t)
	idB := newIdentity(t)
	path := writeRecipientsFile(t, idA.Recipient(), idB.Recipient())
	recs, err := LoadRecipientsFile(path)
	if err != nil {
		t.Fatalf("LoadRecipientsFile: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d recipients, want 2", len(recs))
	}
}

func TestLoadRecipientsFile_Missing(t *testing.T) {
	_, err := LoadRecipientsFile(filepath.Join(t.TempDir(), "nope.pub"))
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", err)
	}
}

func TestLoadRecipientsFile_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.pub")
	if err := os.WriteFile(path, []byte("# only a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRecipientsFile(path)
	if err == nil {
		t.Fatal("want 'no recipients' error")
	}
	if !strings.Contains(err.Error(), "no recipients") {
		t.Errorf("err = %v, want 'no recipients'", err)
	}
}

func TestRecipientsFromIdentities(t *testing.T) {
	idA := newIdentity(t)
	idB := newIdentity(t)
	recs, err := RecipientsFromIdentities([]upstream.Identity{idA, idB})
	if err != nil {
		t.Fatalf("RecipientsFromIdentities: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d recipients, want 2", len(recs))
	}
	// First recipient must round-trip equal to idA.Recipient().
	if recs[0].(*upstream.X25519Recipient).String() != idA.Recipient().String() {
		t.Error("recs[0] != idA.Recipient")
	}
}

func TestRecipientsFromIdentities_Empty(t *testing.T) {
	if _, err := RecipientsFromIdentities(nil); err == nil {
		t.Fatal("want error for empty identity list")
	}
}

// nonX25519Identity is a fake non-X25519 Identity for the
// "RecipientsFromIdentities rejects scrypt-like identities" case.
type nonX25519Identity struct{}

func (nonX25519Identity) Unwrap([]*upstream.Stanza) ([]byte, error) {
	return nil, errors.New("unimplemented")
}

func TestRecipientsFromIdentities_NonX25519(t *testing.T) {
	id := newIdentity(t)
	_, err := RecipientsFromIdentities([]upstream.Identity{id, nonX25519Identity{}})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "not an X25519Identity") {
		t.Errorf("err = %v, want 'not an X25519Identity'", err)
	}
}
