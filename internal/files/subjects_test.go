// SPDX-License-Identifier: Apache-2.0

package files_test

import (
	"testing"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/nats"
)

// TestNATSSubjectBuilderSatisfiesSubjects is the structural-typing
// guard: internal/nats.SubjectBuilder must satisfy [files.Subjects]
// without an explicit interface declaration. A method-signature
// drift on either side breaks compilation.
func TestNATSSubjectBuilderSatisfiesSubjects(t *testing.T) {
	b, err := nats.NewSubjectBuilder("default")
	if err != nil {
		t.Fatalf("NewSubjectBuilder: %v", err)
	}
	var _ files.Subjects = b // compile-time assertion

	// Smoke each method to assert non-empty cluster-prefixed output.
	cases := []struct{ name, got string }{
		{"FilesRequest", b.FilesRequest("put")},
		{"FilesRequestPattern", b.FilesRequestPattern()},
		{"FilesResponse", b.FilesResponse("req-1")},
		{"FilesChunk", b.FilesChunk("transfer-1")},
		{"FilesMetadata", b.FilesMetadata()},
	}
	for _, tc := range cases {
		if tc.got == "" {
			t.Errorf("%s returned empty subject", tc.name)
		}
		if got := tc.got[:14]; got != "kscore.default" {
			t.Errorf("%s = %q, want prefix kscore.default", tc.name, tc.got)
		}
	}
}
