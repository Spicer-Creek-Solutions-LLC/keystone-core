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
	labels     map[string]string // name -> description, the repo's label set
	milestones map[int64]string  // id -> state
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
		labels:     map[string]string{"kind/bug": "pre-existing, must not be touched"},
		milestones: map[int64]string{89201: "open", 89210: "open"},
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

		case rest[0] == "labels" && len(rest) == 1 && r.Method == http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page > 1 {
				_ = json.NewEncoder(w).Encode([]apiLabel{})
				return
			}
			var out []apiLabel
			var i int64
			for n, d := range s.labels {
				i++
				out = append(out, apiLabel{ID: i, Name: n, Description: d})
			}
			_ = json.NewEncoder(w).Encode(out)

		case rest[0] == "labels" && len(rest) == 1 && r.Method == http.MethodPost:
			var spec LabelSpec
			_ = json.NewDecoder(r.Body).Decode(&spec)
			if _, dup := s.labels[spec.Name]; dup {
				s.t.Errorf("tool tried to create label %q, which already exists", spec.Name)
			}
			s.labels[spec.Name] = spec.Description
			s.mutations = append(s.mutations, "label:"+spec.Name)
			_ = json.NewEncoder(w).Encode(apiLabel{ID: 900, Name: spec.Name})

		case rest[0] == "milestones" && len(rest) == 2 && r.Method == http.MethodPatch:
			id, _ := strconv.ParseInt(rest[1], 10, 64)
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.milestones[id] = body["state"]
			s.mutations = append(s.mutations, fmt.Sprintf("milestone:%d:%s", id, body["state"]))
			w.WriteHeader(http.StatusOK)

		case rest[0] == "milestones" && r.Method == http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page > 1 {
				_ = json.NewEncoder(w).Encode([]apiMilestone{})
				return
			}
			var ms []apiMilestone
			for id, st := range s.milestones {
				title := "gate-v0.5"
				if id != 89201 {
					title = "v1.x"
				}
				ms = append(ms, apiMilestone{ID: id, Title: title, State: st, OpenIssues: 2})
			}
			_ = json.NewEncoder(w).Encode(ms)

		case rest[0] == "issues" && len(rest) == 2 && r.Method == http.MethodGet: // one issue
			n, _ := strconv.Atoi(rest[1])
			is, ok := s.issues[n]
			if !ok {
				http.Error(w, "no such issue", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(is)

		case rest[0] == "issues" && len(rest) == 3 && rest[2] == "comments" && r.Method == http.MethodGet:
			n, _ := strconv.Atoi(rest[1])
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page > 1 {
				_ = json.NewEncoder(w).Encode([]apiComment{})
				return
			}
			var out []apiComment
			// Serve comments actually posted to this issue, so a resume can see
			// its own earlier write. Fall back to synthetic bodies for issues
			// that only carry a count, which is how pre-existing discussion is
			// modelled.
			bodies := s.comments[n]
			for len(bodies) < s.issues[n].Comments {
				bodies = append(bodies, fmt.Sprintf("discussion %d on issue %d", len(bodies)+1, n))
			}
			for k, b := range bodies {
				out = append(out, apiComment{
					ID: int64(1000 + k), CreatedAt: "2026-02-01T00:00:00Z", UpdatedAt: "2026-02-01T00:00:00Z",
					Body: b,
					User: &struct {
						Login string `json:"login"`
					}{Login: "someone"},
				})
			}
			_ = json.NewEncoder(w).Encode(out)

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
	// Forgejo phrases its two rate limits differently, and both must parse.
	// The comment wording omits "under"; a pattern written only for the issue
	// wording leaves --max-wait inert and stops a long apply mid-run.
	cases := map[string]time.Duration{
		`{"message":"you have posted 5 issues in under 5 minutes"}`:                                   5 * time.Minute,
		`{"message":"CreateComment: \"keystone-bot\" posted 16 comments in 5 minutes: rate limited"}`: 5 * time.Minute,
		`{"message":"posted 30 comments in 30 minutes"}`:                                              30 * time.Minute,
	}
	for body, want := range cases {
		d, ok := parseRateLimitWindow([]byte(body))
		if !ok || d != want {
			t.Errorf("parseRateLimitWindow(%.48s…) = %v %v, want %v", body, d, ok, want)
		}
	}
	if _, ok := parseRateLimitWindow([]byte("no window here")); ok {
		t.Fatal("invented a window that was not stated")
	}
}

func TestEnsureLabelsCreatesOnlyWhatIsMissing(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, _, _ := setup(t, stub)
	var out strings.Builder
	if err := EnsureLabels(c, transitionLabelSpecs, false, &out); err != nil {
		t.Fatal(err)
	}
	for _, spec := range transitionLabelSpecs {
		if _, ok := stub.labels[spec.Name]; !ok {
			t.Errorf("label %s was not created", spec.Name)
		}
	}
	// The pre-existing label must survive untouched; the stub fails the test if
	// the tool tries to recreate one that exists.
	if got := stub.labels["kind/bug"]; got != "pre-existing, must not be touched" {
		t.Errorf("an existing label was modified: %q", got)
	}
	for _, m := range stub.mutations {
		if m == "label:kind/bug" {
			t.Fatal("the tool rewrote an existing label")
		}
	}
}

func TestEnsureLabelsIsIdempotent(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, _, _ := setup(t, stub)
	var out strings.Builder
	for i := 0; i < 3; i++ {
		if err := EnsureLabels(c, transitionLabelSpecs, false, &out); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	n := 0
	for _, m := range stub.mutations {
		if strings.HasPrefix(m, "label:") {
			n++
		}
	}
	if n != len(transitionLabelSpecs) {
		t.Fatalf("three runs created %d labels, want %d", n, len(transitionLabelSpecs))
	}
}

func TestEnsureLabelsDryRunCreatesNothing(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, _, _ := setup(t, stub)
	var out strings.Builder
	if err := EnsureLabels(c, transitionLabelSpecs, true, &out); err != nil {
		t.Fatal(err)
	}
	if len(stub.mutations) != 0 {
		t.Fatalf("dry run created labels: %v", stub.mutations)
	}
	if !strings.Contains(out.String(), "would be created") {
		t.Errorf("dry run did not report what it would do:\n%s", out.String())
	}
}

func TestMilestonesAreClosedNeverDeleted(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, _ := setup(t, stub)
	var out strings.Builder
	if err := CloseMilestones(c, snap, false, &out); err != nil {
		t.Fatal(err)
	}
	for id, st := range stub.milestones {
		if st != "closed" {
			t.Errorf("milestone %d is %q, want closed", id, st)
		}
	}
	for _, call := range c.calls {
		if call.Method == http.MethodDelete {
			t.Fatalf("a milestone was deleted rather than closed: %s", call.Path)
		}
	}
	if problems := VerifyMilestones(c, snap); len(problems) != 0 {
		t.Fatalf("verification failed after a clean close: %v", problems)
	}
}

func TestMilestoneDryRunChangesNothing(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, _ := setup(t, stub)
	var out strings.Builder
	if err := CloseMilestones(c, snap, true, &out); err != nil {
		t.Fatal(err)
	}
	for id, st := range stub.milestones {
		if st != "open" {
			t.Errorf("dry run closed milestone %d", id)
		}
	}
}

func TestVerifyMilestonesFlagsADeletedMilestone(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, _ := setup(t, stub)
	var out strings.Builder
	if err := CloseMilestones(c, snap, false, &out); err != nil {
		t.Fatal(err)
	}
	// Something removes a milestone behind the tool's back. Verification must
	// call that out rather than treat a missing milestone as closed.
	delete(stub.milestones, 89201)
	problems := VerifyMilestones(c, snap)
	if len(problems) == 0 || !strings.Contains(problems[0], "never deleted") {
		t.Fatalf("a deleted milestone was not flagged: %v", problems)
	}
}

func TestSnapshotCapturesCommentBodies(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	stub.issues[1].Comments = 2
	_, snap, _ := setup(t, stub)
	if len(snap.Issues) != 1 {
		t.Fatalf("got %d issues", len(snap.Issues))
	}
	if got := len(snap.Issues[0].CommentBodies); got != 2 {
		t.Fatalf("captured %d comment bodies, want 2 — the archive must preserve discussion, not just count it", got)
	}
	if snap.Issues[0].CommentBodies[0].Body == "" {
		t.Error("comment body is empty")
	}
}

func TestRetryPolicyTreatsCommentPostsDifferently(t *testing.T) {
	const comments = "/repos/o/r/issues/7/comments"
	const labels = "/repos/o/r/issues/7/labels"
	// A 500 is ambiguous. Retrying an idempotent write is harmless; re-posting a
	// comment would leave two identical notices on a public issue.
	if retryableStatus(500, http.MethodPost, comments) {
		t.Error("a comment POST must not be retried on 500")
	}
	if !retryableStatus(500, http.MethodPost, labels) {
		t.Error("a label POST should be retried on 500; adding a label is idempotent")
	}
	if !retryableStatus(500, http.MethodPatch, "/repos/o/r/issues/7") {
		t.Error("closing an issue should be retried on 500; it is idempotent")
	}
	// Query strings must not defeat the suffix check.
	if retryableStatus(500, http.MethodPost, comments+"?page=1") {
		t.Error("a comment POST with a query string must still not be retried")
	}
	// The rate limit and gateway errors stay retryable for everything.
	for _, code := range []int{429, 502, 503, 504} {
		if !retryableStatus(code, http.MethodPost, comments) {
			t.Errorf("%d should be retryable", code)
		}
	}
	if retryableStatus(404, http.MethodPost, labels) {
		t.Error("404 is not retryable")
	}
}

func TestResumeDoesNotRepeatACommentTheJournalMissed(t *testing.T) {
	stub := newStub(t, issue(1, "one", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"))
	c, snap, raw := setup(t, stub)
	al, _ := buildAllowlist(snap, raw, "1", nil)

	// Simulate the ambiguous case: the comment landed, but the journal never
	// recorded it because the response failed.
	stub.comments[1] = []string{supersededComment(al.Entries[0])}
	stub.issues[1].Comments = 1

	j := &Journal{Steps: map[string]Step{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Apply(c, al, j, supersededComment, false, &out); err != nil {
		t.Fatal(err)
	}
	if got := len(stub.comments[1]); got != 1 {
		t.Fatalf("issue has %d comments; the resume duplicated a retirement notice", got)
	}
	if !strings.Contains(out.String(), "already present") {
		t.Errorf("resume did not report skipping the existing comment:\n%s", out.String())
	}
	if !stub.closed[1] {
		t.Error("the issue was not closed after the resume")
	}
}
