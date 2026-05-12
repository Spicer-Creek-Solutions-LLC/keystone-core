package main

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCheckedNumbers(t *testing.T) {
	body := `Execution order for v1.1.

- [x] #8 Schema versioning via ` + "`golang-migrate`" + `
- [ ] #9 Reactor engine
- [X] #10 state.apply.skip
- [ ] #11 something
not a checklist line #99
`
	got := checkedNumbers(body)
	want := map[int64]bool{8: true, 10: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("checkedNumbers = %v, want %v", got, want)
	}
	if checkedNumbers("") == nil {
		t.Error("checkedNumbers(\"\") should return a non-nil empty map")
	}
}

func TestOrderEntries(t *testing.T) {
	entries := []backlogEntry{
		{Title: "alpha"}, {Title: "bravo"}, {Title: "charlie"}, {Title: "delta"},
	}
	t.Run("explicit order, partial", func(t *testing.T) {
		var buf strings.Builder
		got := titlesOf(orderEntries(entries, []string{"charlie", "alpha"}, &buf))
		// charlie, alpha first; then remaining in backlog order: bravo, delta
		if want := []string{"charlie", "alpha", "bravo", "delta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("order = %v, want %v", got, want)
		}
		if !strings.Contains(buf.String(), `"bravo" is not in release-order.yaml`) {
			t.Errorf("expected appended-at-end warning, got: %q", buf.String())
		}
	})
	t.Run("unknown title in order list", func(t *testing.T) {
		var buf strings.Builder
		got := titlesOf(orderEntries(entries, []string{"ghost", "bravo"}, &buf))
		if want := []string{"bravo", "alpha", "charlie", "delta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("order = %v, want %v", got, want)
		}
		if !strings.Contains(buf.String(), `"ghost" matches no backlog entry`) {
			t.Errorf("expected unknown-title warning, got: %q", buf.String())
		}
	})
	t.Run("no order falls back to backlog order", func(t *testing.T) {
		got := titlesOf(orderEntries(entries, nil, io.Discard))
		if want := []string{"alpha", "bravo", "charlie", "delta"}; !reflect.DeepEqual(got, want) {
			t.Errorf("order = %v, want %v", got, want)
		}
	})
}

func TestBuildTrackerBody(t *testing.T) {
	body := buildTrackerBody("v1.1", []trackerItem{
		{number: 8, title: "Schema versioning", checked: true},
		{number: 9, title: "Reactor engine", checked: false},
	})
	if !strings.Contains(body, "- [x] #8 Schema versioning") {
		t.Errorf("missing checked line; body:\n%s", body)
	}
	if !strings.Contains(body, "- [ ] #9 Reactor engine") {
		t.Errorf("missing unchecked line; body:\n%s", body)
	}
	if !strings.Contains(body, "v1.1") {
		t.Error("body should mention the version")
	}
	// round-trip: rebuilding from the previous body preserves the tick
	if got := checkedNumbers(body); !got[8] || got[9] {
		t.Errorf("round-trip checkedNumbers = %v, want {8:true}", got)
	}
}

func TestLoadReleaseOrder(t *testing.T) {
	order, err := loadReleaseOrder("v1.1")
	if err != nil {
		t.Fatalf("loadReleaseOrder: %v", err)
	}
	if len(order) == 0 {
		t.Fatal("expected a v1.1 release order")
	}
	if order[0] != "Schema versioning via `golang-migrate`" {
		t.Errorf("first v1.1 item = %q, want the golang-migrate entry", order[0])
	}
	if got, _ := loadReleaseOrder("v9.9"); got != nil {
		t.Errorf("loadReleaseOrder(v9.9) = %v, want nil", got)
	}
}

func TestReleaseOrderTitlesMatchBacklog(t *testing.T) {
	// Every title listed for v1.1 must correspond to a real `####` heading in
	// V1X-BACKLOG.md, or gen-tracker would silently drop it.
	f, err := os.Open("../../docs/project/V1X-BACKLOG.md")
	if err != nil {
		t.Fatalf("open backlog: %v", err)
	}
	defer f.Close()
	parsed, err := parseBacklog(f)
	if err != nil {
		t.Fatalf("parseBacklog: %v", err)
	}
	have := map[string]bool{}
	for _, e := range selectEntries(parsed, []string{"v1.1"}) {
		have[e.Title] = true
	}
	order, _ := loadReleaseOrder("v1.1")
	for _, title := range order {
		if !have[title] {
			t.Errorf("release-order.yaml v1.1 lists %q, which is not a v1.1 backlog entry", title)
		}
	}
	for title := range have {
		found := false
		for _, o := range order {
			if o == title {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("v1.1 backlog entry %q is missing from release-order.yaml", title)
		}
	}
}

func titlesOf(es []backlogEntry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Title)
	}
	return out
}
