// SPDX-License-Identifier: Apache-2.0

package version

import "testing"

func TestGet(t *testing.T) {
	got := Get()
	if got.Version != Version {
		t.Errorf("Get().Version = %q, want %q", got.Version, Version)
	}
	if got.GitCommit != GitCommit {
		t.Errorf("Get().GitCommit = %q, want %q", got.GitCommit, GitCommit)
	}
	if got.BuildDate != BuildDate {
		t.Errorf("Get().BuildDate = %q, want %q", got.BuildDate, BuildDate)
	}
}

func TestDefaults(t *testing.T) {
	if Version == "" {
		t.Error("Version must have a non-empty default for builds without -ldflags")
	}
	if GitCommit == "" {
		t.Error("GitCommit must have a non-empty default for builds without -ldflags")
	}
	if BuildDate == "" {
		t.Error("BuildDate must have a non-empty default for builds without -ldflags")
	}
}
