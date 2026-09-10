// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Allowlist is the exact set of issues an apply may touch. Nothing outside it
// is read, commented, labelled or closed. It is derived from a reviewed
// snapshot and carries that snapshot's hash, so an allowlist cannot be applied
// against tracker state nobody reviewed.
type Allowlist struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	SnapshotHash  string `json:"snapshot_hash"`
	Cutoff        string `json:"cutoff_utc"`
	Forge         Forge  `json:"forge"`

	// Generation names which generation's issues these are. Apply refuses to
	// touch an issue whose recorded generation is not this one, so a Generation
	// 2 issue can never be swept up by a Generation 1 retirement.
	Generation string `json:"generation"`

	Entries []AllowEntry `json:"entries"`

	// SHA256 covers the canonical entry list. The dry run prints it, the
	// reviewer approves it, and apply refuses to run against an allowlist whose
	// entries no longer hash to the approved value.
	SHA256 string `json:"entries_sha256"`
}

// AllowEntry is one issue an apply may touch, pinned to the state it was
// reviewed in.
type AllowEntry struct {
	Number    int    `json:"number"`
	Identity  string `json:"identity"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
	Title     string `json:"title"`
	Kind      string `json:"kind"` // "leaf" or "tracker"
}

// trackerTitleSuffix is the exact shape tools/trackerctl/tracker.go gives a
// roll-up issue: `<bucket> — release tracker`. Matching the generator's format
// rather than a substring matters — "event lifecycle tracking" and "in-memory
// tracking only" are ordinary leaf issues in this tracker, and a looser rule
// would close them in the wrong order.
const trackerTitleSuffix = " — release tracker"

func classify(title string) string {
	if strings.HasSuffix(title, trackerTitleSuffix) {
		return "tracker"
	}
	return "leaf"
}

// buildAllowlist derives the allowlist from a snapshot. Only open issues are
// eligible; anything that appeared after the cutoff is excluded by
// construction, because it is not in the snapshot's issue list.
func buildAllowlist(snap *Snapshot, snapshotBytes []byte, generation string, exclude map[int]string) (*Allowlist, error) {
	if generation == "" {
		return nil, fmt.Errorf("generation must be named explicitly")
	}
	al := &Allowlist{
		Schema:        "keystone-core/tracker-allowlist",
		SchemaVersion: 1,
		SnapshotHash:  hashOf(snapshotBytes),
		Cutoff:        snap.Cutoff,
		Forge:         snap.Forge,
		Generation:    generation,
	}
	for _, is := range snap.Issues {
		if is.State != "open" {
			continue
		}
		if _, skip := exclude[is.Number]; skip {
			continue
		}
		al.Entries = append(al.Entries, AllowEntry{
			Number: is.Number, Identity: is.Identity, State: is.State,
			UpdatedAt: is.UpdatedAt, Title: is.Title, Kind: classify(is.Title),
		})
	}
	sort.Slice(al.Entries, func(i, j int) bool { return al.Entries[i].Number < al.Entries[j].Number })
	al.SHA256 = al.entriesHash()
	return al, nil
}

// entriesHash is a stable digest of the entry list. It covers identity and
// updated_at as well as the number, so an allowlist whose issues have moved on
// since review does not silently hash the same.
func (a *Allowlist) entriesHash() string {
	h := sha256.New()
	entries := append([]AllowEntry(nil), a.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Number < entries[j].Number })
	for _, e := range entries {
		fmt.Fprintf(h, "%d\x00%s\x00%s\x00%s\x00%s\n", e.Number, e.Identity, e.State, e.UpdatedAt, e.Kind)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// verifySelf reports whether the allowlist still hashes to its recorded value.
func (a *Allowlist) verifySelf() error {
	if got := a.entriesHash(); got != a.SHA256 {
		return fmt.Errorf("allowlist entries hash to %s but the file records %s; it was edited after review", got[:12], a.SHA256[:12])
	}
	return nil
}

// order returns the entries in the order an apply must process them: leaves
// first, then trackers, each ascending by number. Closing a tracker before its
// leaves would leave the roll-up claiming work is done that is still open.
func (a *Allowlist) order() []AllowEntry {
	var leaves, trackers []AllowEntry
	for _, e := range a.Entries {
		if e.Kind == "tracker" {
			trackers = append(trackers, e)
		} else {
			leaves = append(leaves, e)
		}
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].Number < leaves[j].Number })
	sort.Slice(trackers, func(i, j int) bool { return trackers[i].Number < trackers[j].Number })
	return append(leaves, trackers...)
}

func (a *Allowlist) marshal() ([]byte, error) {
	b, err := json.MarshalIndent(a, "", " ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (a *Allowlist) contains(number int) bool {
	for _, e := range a.Entries {
		if e.Number == number {
			return true
		}
	}
	return false
}

func (a *Allowlist) summary() string {
	leaves, trackers := 0, 0
	for _, e := range a.Entries {
		if e.Kind == "tracker" {
			trackers++
		} else {
			leaves++
		}
	}
	return "targets: " + strconv.Itoa(len(a.Entries)) + " (" +
		strconv.Itoa(leaves) + " leaf, " + strconv.Itoa(trackers) + " tracker)" +
		"\nallowlist sha256: " + a.SHA256 +
		"\nsnapshot sha256:  " + a.SnapshotHash +
		"\ngeneration:       " + a.Generation +
		"\nforge:            " + a.Forge.Host + " " + a.Forge.repo() + " (id " + strconv.FormatInt(a.Forge.RepoID, 10) + ")"
}
