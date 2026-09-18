package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type approvedDocument struct {
	Path   string `json:"path"`
	Commit string `json:"snapshot_commit"`
	Digest string `json:"approved_sha256"`
	Why    string `json:"why"`
}

type approvalManifest struct {
	Documents []approvedDocument `json:"documents"`
}

// checkApprovedDocuments compares each document with the content digest and
// snapshot explicitly recorded as sanctioned. Updating both in the same
// change is the approval decision that makes a document change admissible.
func checkApprovedDocuments(root, manifestPath string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(root, manifestPath))
	if err != nil {
		return nil, fmt.Errorf("read approval manifest: %w", err)
	}
	var manifest approvalManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, fmt.Errorf("parse approval manifest: %w", err)
	}

	var findings []string
	for _, doc := range manifest.Documents {
		if doc.Path == "" || filepath.IsAbs(doc.Path) || filepath.Clean(doc.Path) != doc.Path || strings.HasPrefix(doc.Path, "../") {
			return nil, fmt.Errorf("invalid approved document path %q", doc.Path)
		}
		if doc.Commit == "" || doc.Digest == "" {
			return nil, fmt.Errorf("approved document %q needs a snapshot commit and content digest", doc.Path)
		}
		if _, err := hex.DecodeString(doc.Digest); err != nil || len(doc.Digest) != sha256.Size*2 {
			return nil, fmt.Errorf("approved document %q has an invalid SHA-256 digest", doc.Path)
		}
		current, err := os.ReadFile(filepath.Join(root, doc.Path))
		if err != nil {
			return nil, fmt.Errorf("read approved document %s: %w", doc.Path, err)
		}
		got := sha256.Sum256(current)
		if hex.EncodeToString(got[:]) == doc.Digest {
			continue
		}
		patch, err := gitAt(root, "diff", "--no-ext-diff", "--unified=3", "--no-color", doc.Commit, "--", doc.Path).Output()
		if err != nil {
			return nil, fmt.Errorf("compare %s with %s: %w", doc.Path, doc.Commit, err)
		}
		if len(patch) == 0 {
			continue
		}
		findings = append(findings, fmt.Sprintf("%s differs from sanctioned snapshot %s (sha256 %s, want %s):\n%s", doc.Path, doc.Commit, hex.EncodeToString(got[:]), doc.Digest, strings.TrimRight(string(patch), "\n")))
	}
	return findings, nil
}
