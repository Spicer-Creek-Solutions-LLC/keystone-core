// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// stubForge is a minimal Forgejo stand-in. It records every mutating request so
// a test can assert not just that the right things happened, but that nothing
// else did — which is the property R04 exists to guarantee.
type stubForge struct {
	t          *testing.T
	repoID     int64
	issues     map[int]*apiIssue
	comments   map[int][]string
	labelPosts map[int][]string
	closed     map[int]bool
	mutations  []string

	// failAfter aborts the nth mutating request with a 500, to simulate an
	// interrupted run.
	failAfter int
	nmut      int
}

func newStub(t *testing.T, issues ...*apiIssue) *stubForge {
	s := &stubForge{
		t: t, repoID: 1749683,
		issues: map[int]*apiIssue{}, comments: map[int][]string{},
		labelPosts: map[int][]string{}, closed: map[int]bool{},
	}
	for _, is := range issues {
		s.issues[is.Number] = is
	}
	return s
}

func issue(n int, title, created, updated string) *apiIssue {
	return &apiIssue{Number: n, State: "open", Title: title, CreatedAt: created, UpdatedAt: updated}
}

func (s *stubForge) server() *httptest.Server {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/api/v1/repos/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/")
		parts := strings.Split(path, "/")
		rest := parts[2:]

		if r.Method != http.MethodGet {
			s.nmut++
			if s.failAfter > 0 && s.nmut > s.failAfter {
				http.Error(w, "simulated interruption", http.StatusInternalServerError)
				return
			}
		}

		switch {
		case len(rest) == 0 && r.Method == http.MethodGet: // repo
			_ = json.NewEncoder(w).Encode(apiRepo{ID: s.repoID, FullName: parts[0] + "/" + parts[1]})

		case rest[0] == "issues" && len(rest) == 1 && r.Method == http.MethodGet: // list
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			var open []apiIssue
			for n := 1; n < 1000; n++ {
				if is, ok := s.issues[n]; ok && is.State == "open" {
					open = append(open, *is)
				}
			}
			// two per page, so paging is genuinely exercised
			lo, hi := (page-1)*2, page*2
			if lo > len(open) {
				lo = len(open)
			}
			if hi > len(open) {
				hi = len(open)
			}
			_ = json.NewEncoder(w).Encode(open[lo:hi])

		case rest[0] == "milestones" && r.Method == http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page > 1 {
				_ = json.NewEncoder(w).Encode([]apiMilestone{})
				return
			}
			_ = json.NewEncoder(w).Encode([]apiMilestone{{ID: 89201, Title: "gate-v0.5", State: "open", OpenIssues: 2}})

		case rest[0] == "issues" && len(rest) == 2 && r.Method == http.MethodGet: // one issue
			n, _ := strconv.Atoi(rest[1])
			is, ok := s.issues[n]
			if !ok {
				http.Error(w, "no such issue", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(is)

		case rest[0] == "issues" && len(rest) == 3 && rest[2] == "comments" && r.Method == http.MethodPost:
			n, _ := strconv.Atoi(rest[1])
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.comments[n] = append(s.comments[n], body["body"])
			s.mutations = append(s.mutations, fmt.Sprintf("comment:%d", n))
			s.touch(n)
			w.WriteHeader(http.StatusCreated)

		case rest[0] == "issues" && len(rest) == 3 && rest[2] == "labels" && r.Method == http.MethodPost:
			n, _ := strconv.Atoi(rest[1])
			var body map[string][]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.labelPosts[n] = append(s.labelPosts[n], body["labels"]...)
			for _, l := range body["labels"] {
				s.issues[n].Labels = append(s.issues[n].Labels, struct {
					Name string `json:"name"`
				}{Name: l})
			}
			s.mutations = append(s.mutations, fmt.Sprintf("labels:%d", n))
			s.touch(n)
			w.WriteHeader(http.StatusOK)

		case rest[0] == "issues" && len(rest) == 2 && r.Method == http.MethodPatch:
			n, _ := strconv.Atoi(rest[1])
			s.issues[n].State = "closed"
			s.closed[n] = true
			s.mutations = append(s.mutations, fmt.Sprintf("close:%d", n))
			s.touch(n)
			w.WriteHeader(http.StatusCreated)

		default:
			s.t.Errorf("stub forge got an unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	})
	return srv
}

// touch mimics the forge bumping updated_at on any change, which is what makes
// the pre-close precondition non-trivial.
func (s *stubForge) touch(n int) {
	if is, ok := s.issues[n]; ok {
		is.UpdatedAt = "2026-09-10T12:00:00Z"
	}
}

func setup(t *testing.T, stub *stubForge) (*client, *Snapshot, []byte) {
	t.Helper()
	srv := stub.server()
	t.Cleanup(srv.Close)
	c := newClient(srv.URL, "Spicer-Creek-Solutions-LLC/keystone-core", "token", 0, 0)
	c.sleep = func(time.Duration) {}
	now := func() time.Time { return time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC) }
	snap, err := takeSnapshot(c, srv.URL, now)
	if err != nil {
		t.Fatalf("takeSnapshot: %v", err)
	}
	raw, err := snap.marshal()
	if err != nil {
		t.Fatal(err)
	}
	return c, snap, raw
}

func TestFingerprintIdentifiesTheIssueNotItsState(t *testing.T) {
	base := fingerprint(17, "Add OpenRC backend", "2026-05-01T00:00:00Z")
	if base != fingerprint(17, "Add OpenRC backend", "2026-05-01T00:00:00Z") {
		t.Fatal("fingerprint is not stable")
	}
	// The things R07 itself changes must not be part of identity.
	for _, other := range []string{
		fingerprint(18, "Add OpenRC backend", "2026-05-01T00:00:00Z"),
		fingerprint(17, "Add OpenRC backends", "2026-05-01T00:00:00Z"),
		fingerprint(17, "Add OpenRC backend", "2026-05-02T00:00:00Z"),
	} {
		if other == base {
			t.Fatal("fingerprint failed to distinguish a different issue")
		}
	}
}

func TestSnapshotPagesAndDisposesOfLateIssues(t *testing.T) {
	stub := newStub(t,
		issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(2, "two", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(3, "three", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(4, "opened later", "2026-09-11T00:00:00Z", "2026-09-11T00:00:00Z"),
	)
	pr := issue(5, "a pull request", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z")
	pr.PullReq = &struct {
		Merged bool `json:"merged"`
	}{}
	stub.issues[5] = pr

	_, snap, _ := setup(t, stub)
	if len(snap.Issues) != 3 {
		t.Fatalf("got %d issues, want 3 (paging or filtering is wrong): %+v", len(snap.Issues), snap.Issues)
	}
	if len(snap.AfterCutoff) != 1 || snap.AfterCutoff[0] != 4 {
		t.Fatalf("issue opened after the cutoff was not given an explicit disposition: %v", snap.AfterCutoff)
	}
	for _, is := range snap.Issues {
		if is.Number == 5 {
			t.Fatal("a pull request was captured as an issue")
		}
	}
	if snap.Forge.RepoID != 1749683 {
		t.Errorf("repository id not captured: %d", snap.Forge.RepoID)
	}
	if snap.ResponseHash == "" {
		t.Error("no response hash recorded")
	}
}

func TestAllowlistHashDetectsAnEditAfterReview(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	_, snap, raw := setup(t, stub)
	al, err := buildAllowlist(snap, raw, "1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := al.verifySelf(); err != nil {
		t.Fatalf("freshly built allowlist does not verify: %v", err)
	}
	al.Entries = append(al.Entries, AllowEntry{Number: 99, Identity: "x", State: "open", Kind: "leaf"})
	if err := al.verifySelf(); err == nil {
		t.Fatal("an allowlist with an added entry still verified; the digest is not binding")
	}
}

func TestClassifyMatchesTheGeneratorsFormatNotASubstring(t *testing.T) {
	// The real titles in this tracker, including two leaves that contain
	// "tracking" and must not be mistaken for roll-ups.
	for title, want := range map[string]string{
		"gate-v0.5 — release tracker":                       "tracker",
		"gate-v1.0 — release tracker":                       "tracker",
		"v0.x — release tracker":                            "tracker",
		"Reactor engine + event lifecycle tracking":         "leaf",
		"Boostrap PSK consumption: in-memory tracking only": "leaf",
		"`service` stdlib module — OpenRC backends":         "leaf",
	} {
		if got := classify(title); got != want {
			t.Errorf("classify(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestAllowlistClosesLeavesBeforeTrackers(t *testing.T) {
	stub := newStub(t,
		issue(10, "gate-v0.5 — release tracker", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(11, "a leaf", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
	)
	_, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)
	order := al.order()
	if order[0].Number != 11 || order[1].Number != 10 {
		t.Fatalf("tracker issue must close after its leaves, got %d then %d", order[0].Number, order[1].Number)
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	stub := newStub(t,
		issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(2, "two", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
	)
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)
	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, true, &out); err != nil {
		t.Fatal(err)
	}
	if len(stub.mutations) != 0 {
		t.Fatalf("dry run performed mutations: %v", stub.mutations)
	}
	if !strings.Contains(out.String(), "#1") || !strings.Contains(out.String(), "#2") {
		t.Errorf("dry run did not report what it would do:\n%s", out.String())
	}
}

func TestApplyOrdersCommentThenLabelsThenClose(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)
	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, false, &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"comment:1", "labels:1", "close:1"}
	if strings.Join(stub.mutations, ",") != strings.Join(want, ",") {
		t.Fatalf("wrong order: got %v, want %v", stub.mutations, want)
	}
	if got := stub.comments[1][0]; !strings.Contains(got, "Superseded, not completed") {
		t.Errorf("comment does not say superseded rather than completed:\n%s", got)
	}
}

func TestApplyNeverDeletes(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)
	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, call := range c.calls {
		if call.Method == http.MethodDelete {
			t.Fatalf("the tool issued a DELETE: %s %s", call.Method, call.Path)
		}
	}
}

func TestInterruptedRunResumesWithoutRepeating(t *testing.T) {
	stub := newStub(t,
		issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(2, "two", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
	)
	stub.failAfter = 2 // die partway through the first issue's three steps
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)
	jpath := filepath.Join(t.TempDir(), "j.json")

	j, err := loadJournal(jpath, al)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, false, &out); err == nil {
		t.Fatal("expected the simulated interruption to surface as an error")
	}
	interrupted := append([]string(nil), stub.mutations...)
	if len(interrupted) == 0 {
		t.Fatal("nothing happened before the interruption; the test proves nothing")
	}

	// Resume with a fresh process: same journal file, same allowlist.
	stub.failAfter = 0
	j2, err := loadJournal(jpath, al)
	if err != nil {
		t.Fatalf("journal did not survive the interruption: %v", err)
	}
	var out2 strings.Builder
	if err := Apply(c, al, j2, supersededComment, false, &out2); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	for n := 1; n <= 2; n++ {
		if got := len(stub.comments[n]); got != 1 {
			t.Errorf("issue #%d got %d comments; resuming must not repeat a comment", n, got)
		}
		if !stub.closed[n] {
			t.Errorf("issue #%d was not closed after resume", n)
		}
	}
}

func TestRepeatedRunIsANoOp(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)
	jpath := filepath.Join(t.TempDir(), "j.json")

	for i := 0; i < 3; i++ {
		j, err := loadJournal(jpath, al)
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		if err := Apply(c, al, j, supersededComment, false, &out); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if got := len(stub.comments[1]); got != 1 {
		t.Fatalf("three runs produced %d comments; apply is not idempotent", got)
	}
	if n := strings.Count(strings.Join(stub.mutations, ","), "close:1"); n != 1 {
		t.Fatalf("issue closed %d times across three runs", n)
	}
}

func TestIssuesOutsideTheAllowlistAreNeverTouched(t *testing.T) {
	stub := newStub(t,
		issue(1, "in the allowlist", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(2, "excluded by the operator", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(3, "also in", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
	)
	c, snap, raw := setup(t, stub)
	al, err := buildAllowlist(snap, raw, "1", map[int]string{2: "still written by automation"})
	if err != nil {
		t.Fatal(err)
	}
	if al.contains(2) {
		t.Fatal("an excluded issue reached the allowlist")
	}
	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, m := range stub.mutations {
		if strings.HasSuffix(m, ":2") {
			t.Fatalf("issue #2 is outside the allowlist but was mutated: %v", stub.mutations)
		}
	}
	if stub.issues[2].State != "open" || len(stub.comments[2]) != 0 {
		t.Fatal("issue #2 changed despite being outside the allowlist")
	}
}

func TestDriftSinceReviewStopsTheRunBeforeAnyMutation(t *testing.T) {
	stub := newStub(t,
		issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(2, "two", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
	)
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)

	// Someone comments on #2 between review and apply.
	stub.issues[2].UpdatedAt = "2026-09-12T09:00:00Z"

	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	err := Apply(c, al, j, supersededComment, false, &out)
	if err == nil {
		t.Fatal("apply proceeded despite drift since review")
	}
	if !strings.Contains(err.Error(), "nothing applied") {
		t.Errorf("error should make clear nothing was applied: %v", err)
	}
	if len(stub.mutations) != 0 {
		t.Fatalf("drift was detected but %d mutations had already happened: %v", len(stub.mutations), stub.mutations)
	}
}

func TestIdentityDriftIsCaughtEvenWhenTimestampsMatch(t *testing.T) {
	stub := newStub(t, issue(1, "original title", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)

	// A retitled issue keeps its number; identity is what notices.
	stub.issues[1].Title = "a completely different issue"

	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, false, &out); err == nil {
		t.Fatal("a retitled issue was accepted as the reviewed one")
	}
	if len(stub.mutations) != 0 {
		t.Fatal("mutations happened despite an identity mismatch")
	}
}

func TestJournalFromAnotherAllowlistIsRefused(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	_, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)

	jpath := filepath.Join(t.TempDir(), "j.json")
	other := &Journal{AllowlistSHA: "0000000000000000", Forge: al.Forge, Steps: map[string]Step{}, path: jpath}
	if err := other.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := loadJournal(jpath, al); err == nil {
		t.Fatal("a journal from a different allowlist was accepted; a resume could apply unreviewed decisions")
	}
}

func TestVerifyReportsPostconditionsAndCollateralChange(t *testing.T) {
	stub := newStub(t,
		issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
		issue(2, "left alone", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"),
	)
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", map[int]string{2: "excluded"})
	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, false, &out); err != nil {
		t.Fatal(err)
	}
	if problems := Verify(c, al, snap, &out); len(problems) != 0 {
		t.Fatalf("verify found problems after a clean apply: %v", problems)
	}

	// Now something outside the allowlist moves; verify must notice.
	stub.issues[2].State = "closed"
	problems := Verify(c, al, snap, &out)
	if len(problems) == 0 {
		t.Fatal("verify did not notice a change to a non-allowlisted issue")
	}
	if !strings.Contains(problems[0], "#2") {
		t.Errorf("verify blamed the wrong issue: %v", problems)
	}
}

func TestForgeMismatchRefusesToAct(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, _, _ := setup(t, stub)
	wrong := Forge{Host: c.base, Owner: "Spicer-Creek-Solutions-LLC", Name: "keystone-core", RepoID: 999999}
	if err := checkForge(c, wrong); err == nil {
		t.Fatal("acted against a repository whose id differs from the reviewed manifest")
	}
	right := Forge{Host: c.base, Owner: "Spicer-Creek-Solutions-LLC", Name: "keystone-core", RepoID: 1749683}
	if err := checkForge(c, right); err != nil {
		t.Fatalf("refused the correct repository: %v", err)
	}
}

func TestSnapshotEditedAfterCaptureIsRefused(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	_, snap, _ := setup(t, stub)
	snap.Issues = append(snap.Issues, Issue{Number: 42, State: "open", Title: "smuggled in"})
	b, err := snap.marshal()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "snap.json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readSnapshot(p); err == nil {
		t.Fatal("a snapshot with an added issue was accepted; the response hash is not binding")
	}
}

func TestApplyRequiresTheReviewedDigest(t *testing.T) {
	if err := run("apply", opts{apply: true, allowPath: "x", expectSHA: ""}); err == nil {
		t.Fatal("--apply was accepted without the reviewed digest")
	}
}

func TestParseExclude(t *testing.T) {
	got, err := parseExclude("232, #246 ,")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[232] == "" || got[246] == "" {
		t.Fatalf("got %v", got)
	}
	if _, err := parseExclude("not-a-number"); err == nil {
		t.Fatal("expected an error for a non-numeric exclusion")
	}
}

func TestRateLimitWindowParsing(t *testing.T) {
	d, ok := parseRateLimitWindow([]byte(`{"message":"you have posted 5 issues in under 5 minutes"}`))
	if !ok || d != 5*time.Minute {
		t.Fatalf("got %v %v", d, ok)
	}
	if _, ok := parseRateLimitWindow([]byte("no window here")); ok {
		t.Fatal("invented a window that was not stated")
	}
}
