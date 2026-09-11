// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// pubStub is a Forgejo stand-in for the publication path. It is separate from
// stubForge rather than an extension of it: R07's stub serves only open issues
// and carries no bodies, and publishing has to read closed Generation 1 issues
// and their bodies to prove it did not collide with any of them.
type pubStub struct {
	t          *testing.T
	repoID     int64
	nextNumber int
	issues     []*pubIssueView
	labels     map[string]int64
	nextLabel  int64
	milestones map[int64]*apiMilestone
	nextMS     int64
	releases   map[int64]*apiRelease
	mutations  []string

	pinRefused bool
	failAfter  int
	nmut       int
}

func newPubStub(t *testing.T) *pubStub {
	return &pubStub{
		t: t, repoID: 1749683, nextNumber: 268,
		labels:     map[string]int64{"kind/bug": 1, "status/superseded": 2, "generation/1": 3},
		nextLabel:  10,
		milestones: map[int64]*apiMilestone{89201: {ID: 89201, Title: "gate-v0.5", State: "closed"}},
		nextMS:     90000,
		releases: map[int64]*apiRelease{
			9667643:  {ID: 9667643, Tag: "v0.1.0", Body: "Original v0.1.0 notes.\n\n- a change\n"},
			10425107: {ID: 10425107, Tag: "v0.5.0", Body: "Original v0.5.0 notes.\n"},
		},
	}
}

// addG1 seeds a closed Generation 1 issue: no task marker, which is what makes
// it a Generation 1 issue as far as this tool is concerned.
func (s *pubStub) addG1(n int, title string) *pubIssueView {
	is := &pubIssueView{Number: n, State: "closed", Title: title, Body: "Generation 1 issue body."}
	s.issues = append(s.issues, is)
	if n >= s.nextNumber {
		s.nextNumber = n + 1
	}
	return is
}

func (s *pubStub) server() *httptest.Server {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	mux.HandleFunc("/api/v1/repos/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/repos/")
		parts := strings.Split(path, "/")
		rest := parts[2:]

		if r.Method != http.MethodGet {
			s.nmut++
			if s.failAfter > 0 && s.nmut > s.failAfter {
				http.Error(w, "simulated interruption", http.StatusBadGateway)
				return
			}
		}

		switch {
		case len(rest) == 0 && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(apiRepo{ID: s.repoID, FullName: parts[0] + "/" + parts[1]})

		case rest[0] == "issues" && len(rest) == 1 && r.Method == http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			const per = 5
			lo, hi := (page-1)*per, page*per
			if lo > len(s.issues) {
				lo = len(s.issues)
			}
			if hi > len(s.issues) {
				hi = len(s.issues)
			}
			out := make([]pubIssueView, 0, hi-lo)
			for _, is := range s.issues[lo:hi] {
				out = append(out, *is)
			}
			_ = json.NewEncoder(w).Encode(out)

		case rest[0] == "issues" && len(rest) == 1 && r.Method == http.MethodPost:
			var body struct {
				Title     string  `json:"title"`
				Body      string  `json:"body"`
				Labels    []int64 `json:"labels"`
				Milestone int64   `json:"milestone"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			is := &pubIssueView{Number: s.nextNumber, State: "open", Title: body.Title, Body: body.Body}
			s.nextNumber++
			for _, id := range body.Labels {
				for name, lid := range s.labels {
					if lid == id {
						is.Labels = append(is.Labels, struct {
							Name string `json:"name"`
						}{Name: name})
					}
				}
			}
			if body.Milestone != 0 {
				m := s.milestones[body.Milestone]
				if m == nil {
					s.t.Errorf("issue created against milestone %d, which does not exist", body.Milestone)
				} else {
					is.Milestone = &struct {
						ID    int64  `json:"id"`
						Title string `json:"title"`
					}{ID: m.ID, Title: m.Title}
				}
			}
			s.issues = append(s.issues, is)
			s.mutations = append(s.mutations, "create:"+body.Title)
			_ = json.NewEncoder(w).Encode(apiCreatedIssue{Number: is.Number})

		case rest[0] == "issues" && len(rest) == 3 && rest[2] == "pin" && r.Method == http.MethodPost:
			if s.pinRefused {
				http.Error(w, "user does not have permission to pin", http.StatusForbidden)
				return
			}
			s.mutations = append(s.mutations, "pin:"+rest[1])
			w.WriteHeader(http.StatusNoContent)

		case rest[0] == "labels" && len(rest) == 1 && r.Method == http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page > 1 {
				_ = json.NewEncoder(w).Encode([]apiLabel{})
				return
			}
			var out []apiLabel
			for n, id := range s.labels {
				out = append(out, apiLabel{ID: id, Name: n})
			}
			_ = json.NewEncoder(w).Encode(out)

		case rest[0] == "labels" && len(rest) == 1 && r.Method == http.MethodPost:
			var spec LabelSpec
			_ = json.NewDecoder(r.Body).Decode(&spec)
			if _, dup := s.labels[spec.Name]; dup {
				s.t.Errorf("tool tried to create label %q, which already exists", spec.Name)
			}
			s.nextLabel++
			s.labels[spec.Name] = s.nextLabel
			s.mutations = append(s.mutations, "label:"+spec.Name)
			_ = json.NewEncoder(w).Encode(apiLabel{ID: s.nextLabel, Name: spec.Name})

		case rest[0] == "milestones" && len(rest) == 1 && r.Method == http.MethodGet:
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page > 1 {
				_ = json.NewEncoder(w).Encode([]apiMilestone{})
				return
			}
			var out []apiMilestone
			for _, m := range s.milestones {
				out = append(out, *m)
			}
			_ = json.NewEncoder(w).Encode(out)

		case rest[0] == "milestones" && len(rest) == 1 && r.Method == http.MethodPost:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.nextMS++
			m := &apiMilestone{ID: s.nextMS, Title: body["title"], State: "open"}
			s.milestones[m.ID] = m
			s.mutations = append(s.mutations, "milestone:"+body["title"])
			_ = json.NewEncoder(w).Encode(m)

		case rest[0] == "releases" && len(rest) == 3 && rest[1] == "tags" && r.Method == http.MethodGet:
			for _, rel := range s.releases {
				if rel.Tag == rest[2] {
					_ = json.NewEncoder(w).Encode(rel)
					return
				}
			}
			http.Error(w, "no such release", http.StatusNotFound)

		case rest[0] == "releases" && len(rest) == 2 && r.Method == http.MethodGet:
			id, _ := strconv.ParseInt(rest[1], 10, 64)
			rel, ok := s.releases[id]
			if !ok {
				http.Error(w, "no such release", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(rel)

		case rest[0] == "releases" && len(rest) == 2 && r.Method == http.MethodPatch:
			id, _ := strconv.ParseInt(rest[1], 10, 64)
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["body"]; !ok || len(body) != 1 {
				s.t.Errorf("release PATCH sent %v; it must edit the body and nothing else", keysOf(body))
			}
			s.releases[id].Body = body["body"]
			s.mutations = append(s.mutations, "release:"+s.releases[id].Tag)
			w.WriteHeader(http.StatusOK)

		default:
			s.t.Errorf("stub forge got an unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	})
	return srv
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func pubSetup(t *testing.T, s *pubStub) *client {
	t.Helper()
	srv := s.server()
	t.Cleanup(srv.Close)
	c := newClient(srv.URL, "Spicer-Creek-Solutions-LLC/keystone-core", "token", 0, 0)
	c.sleep = func(time.Duration) {}
	return c
}

func pubForge(c *client, s *pubStub) Forge {
	return Forge{Host: strings.TrimRight(c.base, "/"), Owner: "Spicer-Creek-Solutions-LLC", Name: "keystone-core", RepoID: s.repoID}
}

// testDoc is a miniature execution plan: two workstreams and a gate table.
const testDoc = `## Stage P — Product and architecture foundation

### P00 — Product charter and canonical journeys

Define target operator, fleet profile, problems, and measurable success.

### P01 — Threat model

Model operator, server service, agent, and supply-chain attacker.

### P-stage dependency gates

| Workstream | Must follow |
|---|---|
| P00 | R10 |
| P01 | P00 |

## Stage C — First command-and-control release
`

func testReleases() []ReleaseNote {
	return []ReleaseNote{
		{Tag: "v0.1.0", ID: 9667643, Notice: releaseNotice("v0.1.0")},
		{Tag: "v0.5.0", ID: 10425107, Notice: releaseNotice("v0.5.0")},
	}
}

func testPlan(t *testing.T, c *client, s *pubStub) *PubPlan {
	t.Helper()
	now := func() time.Time { return time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC) }
	p, err := buildPubPlan(testDoc, pubForge(c, s), testReleases(), now, 2)
	if err != nil {
		t.Fatalf("buildPubPlan: %v", err)
	}
	return p
}

func TestExtractSectionsReadsEveryWorkstreamAndItsGate(t *testing.T) {
	// Against the real document, so a change to its shape breaks this test
	// rather than silently publishing twelve issues with the wrong text.
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "project", "REBOOT-EXECUTION-PLAN.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := extractSections(string(b))
	if err != nil {
		t.Fatalf("extractSections: %v", err)
	}
	if len(got) != 12 {
		t.Fatalf("got %d workstreams, want 12", len(got))
	}
	if got[0].ID != "P00" || got[11].ID != "P11" {
		t.Errorf("boundaries are %s..%s, want P00..P11", got[0].ID, got[11].ID)
	}
	if got[0].Requires != "R10" {
		t.Errorf("P00 must follow %q, want R10", got[0].Requires)
	}
	for _, s := range got {
		if len(s.Prose) < 40 {
			t.Errorf("%s prose is %d chars, too short to be the real description", s.ID, len(s.Prose))
		}
		if strings.Contains(s.Prose, "Must follow") {
			t.Errorf("%s swallowed the dependency-gate table", s.ID)
		}
		if strings.Contains(s.Prose, "###") {
			t.Errorf("%s ran past its own section", s.ID)
		}
	}
}

func TestExtractSectionsRejectsAWorkstreamWithNoGateRow(t *testing.T) {
	doc := strings.Replace(testDoc, "| P01 | P00 |\n", "", 1)
	if _, err := extractSections(doc); err == nil || !strings.Contains(err.Error(), "P01") {
		t.Fatalf("got %v, want an error naming P01", err)
	}
}

func TestExtractSectionsStopsAtTheNextHeading(t *testing.T) {
	got, err := extractSections(testDoc)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Model operator, server service, agent, and supply-chain attacker."; got[1].Prose != want {
		t.Errorf("P01 prose = %q, want %q", got[1].Prose, want)
	}
}

func TestPlanCarriesTheReviewedBodiesAndHashesThem(t *testing.T) {
	s := newPubStub(t)
	c := pubSetup(t, s)
	p := testPlan(t, c, s)

	if len(p.Issues) != 4 {
		t.Fatalf("got %d issues, want 4 (announcement, tracker, P00, P01)", len(p.Issues))
	}
	if p.Issues[0].Kind != "announcement" || !p.Issues[0].Pinned {
		t.Errorf("the announcement must come first and be pinned, got %+v", p.Issues[0].Kind)
	}
	if p.Issues[0].Milestone {
		t.Error("the announcement is a statement of record; it must not carry the release milestone")
	}
	for _, is := range p.Issues[1:] {
		if !is.Milestone {
			t.Errorf("%s is not assigned to the milestone", is.TaskID)
		}
	}
	if !strings.Contains(p.Issues[2].Body, "Define target operator") {
		t.Error("P00's body does not carry its text from the execution plan")
	}
	if !strings.Contains(p.Issues[2].Body, "**Must follow:** R10") {
		t.Error("P00's body does not state its dependency gate")
	}
	if err := p.verifySelf(); err != nil {
		t.Fatalf("a freshly built plan must verify: %v", err)
	}
}

func TestPlanHashDetectsAnEditedBody(t *testing.T) {
	s := newPubStub(t)
	c := pubSetup(t, s)
	p := testPlan(t, c, s)

	p.Issues[2].Body += "\n\nand one more thing"
	err := p.verifySelf()
	if err == nil || !strings.Contains(err.Error(), "edited after review") {
		t.Fatalf("got %v, want an edited-after-review error", err)
	}
}

func TestPlanRejectsInconsistencyBeforeItReachesTheForge(t *testing.T) {
	base := func(t *testing.T) *PubPlan {
		t.Helper()
		s := newPubStub(t)
		c := pubSetup(t, s)
		return testPlan(t, c, s)
	}
	cases := []struct {
		name   string
		mutate func(*PubPlan)
		want   string
	}{
		{"duplicate task id", func(p *PubPlan) { p.Issues[3].TaskID = p.Issues[2].TaskID }, "appears twice"},
		{"body lost its marker", func(p *PubPlan) {
			p.Issues[2].Body = strings.Replace(p.Issues[2].Body, taskMarker("P00"), "", 1)
		}, "task marker"},
		{"label the plan never creates", func(p *PubPlan) {
			p.Issues[2].Labels = append(p.Issues[2].Labels, "area/invented")
		}, "not in the plan's label list"},
		{"empty title", func(p *PubPlan) { p.Issues[2].Title = "  " }, "empty title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base(t)
			tc.mutate(p)
			err := p.checkSelfConsistent()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestMarkerTaskID(t *testing.T) {
	cases := []struct {
		body string
		want string
		ok   bool
	}{
		{"<!-- keystone-core-task: P00 -->\n\ntext", "P00", true},
		{"text\n<!-- keystone-core-task: TRACKER-v0.6.0 -->", "TRACKER-v0.6.0", true},
		{"a Generation 1 body with no marker", "", false},
		{"<!-- keystone-core-task: -->", "", false},
		{"<!-- keystone-core-task: P00", "", false},
	}
	for _, tc := range cases {
		got, ok := markerTaskID(tc.body)
		if got != tc.want || ok != tc.ok {
			t.Errorf("markerTaskID(%q) = %q,%v want %q,%v", tc.body, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPublishDryRunCreatesNothing(t *testing.T) {
	s := newPubStub(t)
	s.addG1(232, "Dependency freshness: direct deps with updates available")
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	j := &PubJournal{Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}

	var out strings.Builder
	if err := Publish(c, p, j, true, &out); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(s.mutations) != 0 {
		t.Fatalf("dry run mutated the forge: %v", s.mutations)
	}
	for _, want := range []string{"P00", "P01", "would be created", "would be pinned", "would be annotated"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry-run output does not mention %q:\n%s", want, out.String())
		}
	}
	if s.releases[9667643].Body != "Original v0.1.0 notes.\n\n- a change\n" {
		t.Error("dry run changed a release body")
	}
}

func TestPublishCreatesEverythingOnceAndRecordsIt(t *testing.T) {
	s := newPubStub(t)
	s.addG1(232, "Dependency freshness: direct deps with updates available")
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	j := &PubJournal{PlanSHA: p.SHA256, Forge: p.Forge, Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}

	var out strings.Builder
	if err := Publish(c, p, j, false, &out); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(j.Issues) != 4 {
		t.Fatalf("journal records %d issues, want 4", len(j.Issues))
	}
	for _, is := range p.Issues {
		st := j.Issues[is.TaskID]
		if st.Number == 0 {
			t.Errorf("%s has no recorded issue number", is.TaskID)
		}
		if st.Number <= 232 {
			t.Errorf("%s got #%d, at or below the Generation 1 range", is.TaskID, st.Number)
		}
	}
	if !j.Issues["ANNOUNCE-reboot"].Pinned {
		t.Error("the announcement was not pinned")
	}

	// Verification must pass against the state publish just produced.
	var vout strings.Builder
	if problems := VerifyPublication(c, p, &vout); len(problems) != 0 {
		t.Fatalf("verify found problems on a clean publication: %v", problems)
	}
}

func TestPublishAnnotatesReleasesWithoutLosingTheOriginalNotes(t *testing.T) {
	s := newPubStub(t)
	s.addG1(232, "Dependency freshness: direct deps with updates available")
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	j := &PubJournal{PlanSHA: p.SHA256, Forge: p.Forge, Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}

	var out strings.Builder
	if err := Publish(c, p, j, false, &out); err != nil {
		t.Fatal(err)
	}
	got := s.releases[9667643].Body
	if !strings.Contains(got, releaseNoticeMarker) {
		t.Error("release body does not carry the notice")
	}
	if !strings.Contains(got, "Original v0.1.0 notes.") {
		t.Error("annotating the release destroyed its original notes")
	}
	if !strings.HasPrefix(got, ">") {
		t.Error("the notice must come first, where a visitor sees it before the artifacts")
	}

	// A second run must not stack a second banner on a public page.
	before := s.releases[9667643].Body
	var out2 strings.Builder
	if err := Publish(c, p, j, false, &out2); err != nil {
		t.Fatal(err)
	}
	if s.releases[9667643].Body != before {
		t.Error("a repeated publish changed the release body again")
	}
	if strings.Count(s.releases[9667643].Body, releaseNoticeMarker) != 1 {
		t.Errorf("notice appears %d times", strings.Count(s.releases[9667643].Body, releaseNoticeMarker))
	}
}

func TestAnnotateRefusesAReleaseWhoseTagMoved(t *testing.T) {
	s := newPubStub(t)
	c := pubSetup(t, s)
	j := &PubJournal{Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}

	var out strings.Builder
	err := annotateRelease(c, ReleaseNote{Tag: "v9.9.9", ID: 9667643, Notice: "x"}, j, false, &out)
	if err == nil || !strings.Contains(err.Error(), "tagged v0.1.0") {
		t.Fatalf("got %v, want a tag-mismatch refusal", err)
	}
	if len(s.mutations) != 0 {
		t.Errorf("a refused annotation still mutated: %v", s.mutations)
	}
}

func TestInterruptedPublishResumesWithoutDuplicating(t *testing.T) {
	s := newPubStub(t)
	s.addG1(232, "Dependency freshness: direct deps with updates available")
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	jpath := filepath.Join(t.TempDir(), "j.json")

	// Fail partway: labels, milestone, then two issue creations, then stop.
	s.failAfter = 7
	j, err := loadPubJournal(jpath, p)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := Publish(c, p, j, false, &out); err == nil {
		t.Fatal("expected the simulated interruption to surface")
	}
	partial := len(j.Issues)
	if partial == 0 || partial == len(p.Issues) {
		t.Fatalf("interruption left %d of %d issues; wanted a genuine partial", partial, len(p.Issues))
	}

	// Resume from the journal on disk.
	s.failAfter = 0
	j2, err := loadPubJournal(jpath, p)
	if err != nil {
		t.Fatalf("reload journal: %v", err)
	}
	if len(j2.Issues) != partial {
		t.Fatalf("reloaded journal has %d issues, want %d", len(j2.Issues), partial)
	}
	var out2 strings.Builder
	if err := Publish(c, p, j2, false, &out2); err != nil {
		t.Fatalf("resume: %v", err)
	}

	seen := map[string]int{}
	for _, is := range s.issues {
		if id, ok := markerTaskID(is.Body); ok {
			seen[id]++
		}
	}
	for _, is := range p.Issues {
		if seen[is.TaskID] != 1 {
			t.Errorf("%s exists %d times after resume, want exactly 1", is.TaskID, seen[is.TaskID])
		}
	}
}

func TestPublishAdoptsAnIssueAlreadyOnTheForgeWhenTheJournalIsLost(t *testing.T) {
	s := newPubStub(t)
	c := pubSetup(t, s)
	p := testPlan(t, c, s)

	// An earlier run created P00 and the journal was lost afterwards.
	existing := s.addG1(300, "P00 — Product charter and canonical journeys")
	existing.State = "open"
	existing.Body = p.Issues[2].Body

	j := &PubJournal{PlanSHA: p.SHA256, Forge: p.Forge, Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Publish(c, p, j, false, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already exists as #300") {
		t.Errorf("publish did not report adopting the existing issue:\n%s", out.String())
	}
	created := 0
	for _, m := range s.mutations {
		if strings.HasPrefix(m, "create:") {
			created++
		}
	}
	if created != len(p.Issues)-1 {
		t.Errorf("created %d issues, want %d — the existing one must not be recreated", created, len(p.Issues)-1)
	}
}

func TestJournalFromAnotherPlanIsRefused(t *testing.T) {
	s := newPubStub(t)
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	jpath := filepath.Join(t.TempDir(), "j.json")

	j := &PubJournal{PlanSHA: "0000", Forge: p.Forge, Issues: map[string]PubStep{"P00": {Number: 9}}, Releases: map[string]bool{}, path: jpath}
	if err := j.save(); err != nil {
		t.Fatal(err)
	}
	_, err := loadPubJournal(jpath, p)
	if err == nil || !strings.Contains(err.Error(), "belongs to plan") {
		t.Fatalf("got %v, want a plan-mismatch refusal", err)
	}
}

func TestPublishRefusesAPlanForAnotherRepository(t *testing.T) {
	s := newPubStub(t)
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	p.Forge.RepoID = 999

	if err := checkForge(c, p.Forge); err == nil || !strings.Contains(err.Error(), "refusing to act") {
		t.Fatalf("got %v, want a forge-mismatch refusal", err)
	}
}

func TestPinFailureIsReportedAndDoesNotStopThePublish(t *testing.T) {
	s := newPubStub(t)
	s.pinRefused = true
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	j := &PubJournal{PlanSHA: p.SHA256, Forge: p.Forge, Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}

	var out strings.Builder
	if err := Publish(c, p, j, false, &out); err != nil {
		t.Fatalf("a refused pin must not fail the publish: %v", err)
	}
	if !strings.Contains(out.String(), "PIN FAILED") {
		t.Errorf("a refused pin was swallowed:\n%s", out.String())
	}
	if j.Issues["ANNOUNCE-reboot"].Pinned {
		t.Error("journal claims the issue is pinned when the forge refused")
	}
	if len(j.Releases) != 2 {
		t.Error("the publish did not carry on to the release annotations")
	}
}

func TestVerifyDetectsPublicationDefects(t *testing.T) {
	build := func(t *testing.T) (*client, *pubStub, *PubPlan) {
		t.Helper()
		s := newPubStub(t)
		s.addG1(232, "Dependency freshness: direct deps with updates available")
		c := pubSetup(t, s)
		p := testPlan(t, c, s)
		j := &PubJournal{PlanSHA: p.SHA256, Forge: p.Forge, Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}
		var out strings.Builder
		if err := Publish(c, p, j, false, &out); err != nil {
			t.Fatal(err)
		}
		return c, s, p
	}

	cases := []struct {
		name   string
		damage func(*pubStub, *PubPlan)
		want   string
	}{
		{"an issue was deleted", func(s *pubStub, p *PubPlan) {
			for i, is := range s.issues {
				if id, _ := markerTaskID(is.Body); id == "P01" {
					s.issues = append(s.issues[:i], s.issues[i+1:]...)
					break
				}
			}
		}, "no issue on the tracker carries this task marker"},
		{"a body was edited after creation", func(s *pubStub, p *PubPlan) {
			for _, is := range s.issues {
				if id, _ := markerTaskID(is.Body); id == "P00" {
					is.Body += "\n\nsomeone edited this"
				}
			}
		}, "body does not match the reviewed plan"},
		{"an issue was closed", func(s *pubStub, p *PubPlan) {
			for _, is := range s.issues {
				if id, _ := markerTaskID(is.Body); id == "P00" {
					is.State = "closed"
				}
			}
		}, "expected open"},
		{"a stray task marker appeared", func(s *pubStub, p *PubPlan) {
			s.addG1(400, "something else").Body = taskMarker("P99")
		}, "not in the plan"},
		{"the milestone assignment was dropped", func(s *pubStub, p *PubPlan) {
			for _, is := range s.issues {
				if id, _ := markerTaskID(is.Body); id == "P01" {
					is.Milestone = nil
				}
			}
		}, "not assigned to milestone"},
		{"a release notice was removed", func(s *pubStub, p *PubPlan) {
			s.releases[10425107].Body = "Original v0.5.0 notes.\n"
		}, "does not carry the archived notice"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, s, p := build(t)
			tc.damage(s, p)
			var out strings.Builder
			problems := VerifyPublication(c, p, &out)
			if !containsSubstring(problems, tc.want) {
				t.Fatalf("problems %v do not mention %q", problems, tc.want)
			}
		})
	}
}

func TestVerifyCatchesAnIssueDeduplicatedAgainstGeneration1(t *testing.T) {
	s := newPubStub(t)
	// A Generation 1 issue numbered above everything the publish created, as a
	// forge that merged a new issue into an old one would leave things.
	s.addG1(232, "Dependency freshness: direct deps with updates available")
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	j := &PubJournal{PlanSHA: p.SHA256, Forge: p.Forge, Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Publish(c, p, j, false, &out); err != nil {
		t.Fatal(err)
	}
	s.addG1(9000, "a later Generation 1 issue")

	var vout strings.Builder
	problems := VerifyPublication(c, p, &vout)
	if !containsSubstring(problems, "it was not created fresh") {
		t.Fatalf("problems %v do not report the numbering collision", problems)
	}
}

func TestVerifyCatchesATitleSharedWithAClosedGeneration1Issue(t *testing.T) {
	s := newPubStub(t)
	s.addG1(232, "Dependency freshness: direct deps with updates available")
	c := pubSetup(t, s)
	p := testPlan(t, c, s)
	j := &PubJournal{PlanSHA: p.SHA256, Forge: p.Forge, Issues: map[string]PubStep{}, Releases: map[string]bool{}, path: filepath.Join(t.TempDir(), "j.json")}
	var out strings.Builder
	if err := Publish(c, p, j, false, &out); err != nil {
		t.Fatal(err)
	}
	// A closed Generation 1 issue that happens to share a Generation 2 title.
	// Identity comes from the task marker, so the publish succeeded — but the
	// collision is reported rather than left for a human to notice later.
	s.addG1(231, p.Issues[2].Title)

	var vout strings.Builder
	problems := VerifyPublication(c, p, &vout)
	if !containsSubstring(problems, "shares a title with") {
		t.Fatalf("problems %v do not report the title collision", problems)
	}
}

func TestNegativeTestProvesArchivedIssue232HasNoReplacement(t *testing.T) {
	title := "Dependency freshness: direct deps with updates available"

	clean := []pubIssueView{{Number: 232, State: "closed", Title: title}}
	if got := verifyNoReplacement(clean, 232); len(got) != 0 {
		t.Fatalf("a clean tracker reported %v", got)
	}

	replaced := append([]pubIssueView(nil), clean...)
	replaced = append(replaced, pubIssueView{Number: 268, State: "open", Title: title})
	got := verifyNoReplacement(replaced, 232)
	if !containsSubstring(got, "a replacement was created") {
		t.Fatalf("got %v, want a replacement report", got)
	}

	reopened := []pubIssueView{{Number: 232, State: "open", Title: title}}
	if got := verifyNoReplacement(reopened, 232); !containsSubstring(got, "expected closed") {
		t.Fatalf("got %v, want a reopened report", got)
	}

	if got := verifyNoReplacement(nil, 232); !containsSubstring(got, "not found") {
		t.Fatalf("got %v, want a missing-issue report", got)
	}
}

func TestBodyLinksResolveInTheWorkingTree(t *testing.T) {
	s := newPubStub(t)
	c := pubSetup(t, s)
	p := testPlan(t, c, s)

	root := filepath.Join("..", "..")
	if err := checkBodyLinks(p, root); err != nil {
		t.Fatalf("the composed bodies link to paths that do not exist: %v", err)
	}
	// Every body must actually carry links, or the check above passes vacuously.
	found := 0
	for _, is := range p.Issues {
		n := len(bodyLinkRe.FindAllString(is.Body, -1))
		if n == 0 {
			t.Errorf("%s carries no in-repository links", is.TaskID)
		}
		found += n
	}
	if found < 10 {
		t.Errorf("only %d in-repository links across the plan; the check is close to vacuous", found)
	}

	p.Issues[2].Body += "\n- [gone](" + repoMainBase + "/docs/project/NO-SUCH-FILE.md)\n"
	err := checkBodyLinks(p, root)
	if err == nil || !strings.Contains(err.Error(), "NO-SUCH-FILE.md") {
		t.Fatalf("got %v, want a broken-link report", err)
	}
}

func TestReleaseNoticeNamesItsOwnTag(t *testing.T) {
	for _, tag := range []string{"v0.1.0", "v0.5.0"} {
		n := releaseNotice(tag)
		if !strings.Contains(n, "`"+tag+"`") {
			t.Errorf("notice for %s does not name it: %s", tag, n)
		}
		if !strings.Contains(n, releaseNoticeMarker) {
			t.Errorf("notice for %s lacks the idempotence marker", tag)
		}
		for _, line := range strings.Split(strings.TrimRight(n, "\n"), "\n") {
			if !strings.HasPrefix(line, ">") {
				t.Errorf("notice line is not part of the blockquote: %q", line)
			}
		}
	}
}

func containsSubstring(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
