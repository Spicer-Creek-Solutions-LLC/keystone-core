package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type approvedDocument struct {
	Path   string `json:"path"`
	Commit string `json:"approved_commit"`
	Why    string `json:"why"`
}

type approvalManifest struct {
	Documents []approvedDocument `json:"documents"`
}

// checkApprovedDocuments compares each document with the snapshot explicitly
// recorded as approved. It is a review aid: a later document change needs a
// human decision and a new approved snapshot, so this is deliberately not part
// of the ordinary pre-merge check.
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
		if doc.Commit == "" {
			return nil, fmt.Errorf("approved document %q has no commit", doc.Path)
		}
		patch, err := gitAt(root, "diff", "--no-ext-diff", "--unified=3", "--no-color", doc.Commit, "--", doc.Path).Output()
		if err != nil {
			return nil, fmt.Errorf("compare %s with %s: %w", doc.Path, doc.Commit, err)
		}
		if len(patch) == 0 {
			continue
		}
		findings = append(findings, fmt.Sprintf("%s differs from approved commit %s:\n%s", doc.Path, doc.Commit, strings.TrimRight(string(patch), "\n")))
	}
	return findings, nil
}
