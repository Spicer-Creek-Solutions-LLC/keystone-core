package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// G38 added checkFieldEncodings so that a field with no stated encoding fails
// the build. Its first draft matched the row shape anywhere in ADR-0005 and
// found section 1's ORIGINAL field table -- three columns, leading digit, a
// different meaning -- so blanking a cell in the encoding index left archlint
// green while blanking one in the older table failed it. A check that names one
// table and reads another is the defect it exists to prevent.
//
// G37's review made the other half of the point: that first draft was found by
// planting in a shell, and a demonstration that is not checked in guards
// nothing after the session ends. These cases are the demonstration, in the tree.

const indexHeading = "#### Every field's encoding, in one place"

// decoy is the shape that fooled the first draft: an earlier three-column table
// in the same document whose rows also begin with a digit.
const decoy = `### 1. Envelope canonicalization

| # | Field | Notes |
|---|---|---|
| 1 | Protocol version | Cleartext (§ 2) |
| 6 | Timestamp | § 6 |

`

func writeADR(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "adr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "0005-versioned-encrypted-protocol.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func completeIndex() string {
	rows := []string{
		"| 1 | Protocol version | `uint32` big-endian (§ 2) |",
		"| 2 | Message class | canonical ASCII token (§ 7) |",
		"| 3 | Job identifier | ASCII, 1 to 64 bytes (§ 3) |",
		"| 4 | Correlation identifier | as field 3 (§ 3) |",
		"| 5 | Sender identifier | agent id or reserved token (§ 3) |",
		"| 6 | Timestamp | `int64` big-endian, Unix ms (§ 6) |",
		"| 7 | Nonce | 16 bytes random (§ 6) |",
		"| 8 | Payload | KEM output or opaque bytes (§ 5) |",
		"| 9 | Signature | Ed25519 ‖ ML-DSA-65 (§ 4) |",
	}
	return decoy + indexHeading + "\n\n| # | Field | Encoding |\n|---|---|---|\n" +
		strings.Join(rows, "\n") + "\n"
}

func TestFieldEncodingIndexIsCheckedAndAnchored(t *testing.T) {
	if got := checkFieldEncodings(writeADR(t, completeIndex())); len(got) != 0 {
		t.Fatalf("a complete index must pass, got %q", got)
	}

	// The decoy must not be what the check reads. Blanking a cell in the OLDER
	// table has to leave the check green; blanking one in the index must not.
	decoyBlanked := strings.Replace(completeIndex(), "| 6 | Timestamp | § 6 |", "| 6 | Timestamp |  |", 1)
	if got := checkFieldEncodings(writeADR(t, decoyBlanked)); len(got) != 0 {
		t.Errorf("the check read the wrong table: blanking the decoy produced %q", got)
	}

	for _, tc := range []struct {
		name, from, to, want string
	}{
		{"blank encoding", "| 6 | Timestamp | `int64` big-endian, Unix ms (§ 6) |", "| 6 | Timestamp |  |", "blank encoding"},
		{"deferred encoding", "| 7 | Nonce | 16 bytes random (§ 6) |", "| 7 | Nonce | unspecified |", "defers its encoding"},
		{"TBD encoding", "| 7 | Nonce | 16 bytes random (§ 6) |", "| 7 | Nonce | TBD |", "defers its encoding"},
		{"row removed", "| 2 | Message class | canonical ASCII token (§ 7) |\n", "", "no row for field 2"},
		{"heading removed", indexHeading, "#### Something else", "field-encoding index is gone"},
		{"duplicate row", "| 9 | Signature | Ed25519 ‖ ML-DSA-65 (§ 4) |", "| 9 | Signature | Ed25519 (§ 4) |\n| 9 | Signature | ML-DSA (§ 4) |", "two rows for field 9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(completeIndex(), tc.from, tc.to, 1)
			if body == completeIndex() {
				t.Fatalf("fixture unchanged; %q did not match", tc.from)
			}
			got := checkFieldEncodings(writeADR(t, body))
			if len(got) == 0 {
				t.Fatalf("no finding; want one mentioning %q", tc.want)
			}
			if !strings.Contains(strings.Join(got, "\n"), tc.want) {
				t.Errorf("findings %q, want one mentioning %q", got, tc.want)
			}
		})
	}
}
