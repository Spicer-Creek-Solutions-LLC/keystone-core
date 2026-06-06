// SPDX-License-Identifier: Apache-2.0

//go:build linux

package user

import (
	"context"
	"fmt"
	"os/user"
	"sort"
	"strconv"
)

// busyboxProvider drives BusyBox's user applets (adduser / deluser /
// addgroup / delgroup), the toolchain Alpine ships in place of
// shadow-utils. Its flags differ materially from useradd's, and there
// is no usermod equivalent — so Mod is unavailable (ErrModUnsupported).
//
// The read path (Lookup) is the shared, cross-platform osLookup; only
// the mutating paths are BusyBox-specific.
type busyboxProvider struct {
	osLookup
	adduser  string
	deluser  string
	addgroup string
	delgroup string

	// run is the exec seam; production wiring is runManaged. Tests
	// inject a recorder to assert arg formation without invoking
	// BusyBox.
	run commandRunner

	// lookupFn reads back the current account (for SetGroups' diff);
	// defaults to the embedded osLookup. groupNameByGID resolves a
	// primary GID to its group name so SetGroups never removes the
	// primary group. Both are seams for tests.
	lookupFn       func(name string) (*UserInfo, error)
	groupNameByGID func(gid int) (string, error)
}

func newBusyboxProvider(adduser string) *busyboxProvider {
	p := &busyboxProvider{
		adduser:        adduser,
		deluser:        "deluser",
		addgroup:       "addgroup",
		delgroup:       "delgroup",
		run:            runManaged,
		groupNameByGID: realGroupNameByGID,
	}
	p.lookupFn = p.Lookup
	return p
}

// Add creates the account via `adduser`. BusyBox prompts for a
// password interactively unless -D is passed, so -D is mandatory in a
// non-interactive context. Supplementary groups have no adduser flag
// (-G sets only the primary group), so each is applied afterward via
// `addgroup USER GROUP`.
func (p *busyboxProvider) Add(ctx context.Context, opts AddOptions) error {
	args := []string{"-D"} // never prompt for a password
	if opts.System {
		args = append(args, "-S")
	}
	if !opts.CreateHome {
		args = append(args, "-H")
	}
	if opts.UID != nil {
		args = append(args, "-u", strconv.Itoa(*opts.UID))
	}
	if opts.Home != "" {
		args = append(args, "-h", opts.Home)
	}
	if opts.Shell != "" {
		args = append(args, "-s", opts.Shell)
	}
	if opts.Comment != "" {
		args = append(args, "-g", opts.Comment)
	}
	// Primary group. BusyBox's -G takes a group *name*; resolve a
	// numeric GID to its name (params makes gid/group mutually
	// exclusive, so at most one of these fires).
	switch {
	case opts.Group != "":
		args = append(args, "-G", opts.Group)
	case opts.GID != nil:
		name, err := p.groupNameByGID(*opts.GID)
		if err != nil {
			return fmt.Errorf("adduser %s: resolve primary gid %d: %w", opts.Name, *opts.GID, err)
		}
		args = append(args, "-G", name)
	}
	args = append(args, opts.Name)
	if err := p.run(ctx, p.adduser, args); err != nil {
		return err
	}
	for _, g := range opts.Groups {
		if err := p.run(ctx, p.addgroup, []string{opts.Name, g}); err != nil {
			return err
		}
	}
	return nil
}

// Mod is unavailable on BusyBox: there is no usermod, so an existing
// account's scalar fields can't be changed in place.
func (p *busyboxProvider) Mod(_ context.Context, _ ModOptions) error {
	return ErrModUnsupported
}

// Del removes the account via `deluser`. BusyBox spells the
// home-removal flag --remove-home (shadow's userdel uses --remove).
func (p *busyboxProvider) Del(ctx context.Context, name string, removeHome bool) error {
	args := []string{}
	if removeHome {
		args = append(args, "--remove-home")
	}
	args = append(args, name)
	return p.run(ctx, p.deluser, args)
}

// SetGroups reconciles the supplementary group set. BusyBox has no
// usermod -G replace, so it diffs the live membership against the
// desired set and applies the delta with addgroup / delgroup. The
// primary group is excluded from both sides so it is never removed.
func (p *busyboxProvider) SetGroups(ctx context.Context, name string, groups []string) error {
	live, err := p.lookupFn(name)
	if err != nil {
		return fmt.Errorf("addgroup %s: read current groups: %w", name, err)
	}
	var current []string
	var primary string
	if live != nil {
		current = live.Groups
		if pn, err := p.groupNameByGID(live.GID); err == nil {
			primary = pn
		}
	}
	toAdd, toRemove := groupDelta(stripGroup(current, primary), stripGroup(groups, primary))
	for _, g := range toAdd {
		if err := p.run(ctx, p.addgroup, []string{name, g}); err != nil {
			return err
		}
	}
	for _, g := range toRemove {
		if err := p.run(ctx, p.delgroup, []string{name, g}); err != nil {
			return err
		}
	}
	return nil
}

// groupDelta computes the set difference between the desired and
// current supplementary group memberships: toAdd is desired-not-in-
// current, toRemove is current-not-in-desired. Both are sorted for
// deterministic command ordering (and test assertions).
func groupDelta(current, desired []string) (toAdd, toRemove []string) {
	cur := make(map[string]struct{}, len(current))
	for _, g := range current {
		cur[g] = struct{}{}
	}
	des := make(map[string]struct{}, len(desired))
	for _, g := range desired {
		des[g] = struct{}{}
	}
	for g := range des {
		if _, ok := cur[g]; !ok {
			toAdd = append(toAdd, g)
		}
	}
	for g := range cur {
		if _, ok := des[g]; !ok {
			toRemove = append(toRemove, g)
		}
	}
	sort.Strings(toAdd)
	sort.Strings(toRemove)
	return toAdd, toRemove
}

// stripGroup returns names with the single name `drop` removed (used
// to keep the primary group out of a supplementary-set diff). A blank
// drop is a no-op.
func stripGroup(names []string, drop string) []string {
	if drop == "" {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != drop {
			out = append(out, n)
		}
	}
	return out
}

func realGroupNameByGID(gid int) (string, error) {
	g, err := user.LookupGroupId(strconv.Itoa(gid))
	if err != nil {
		return "", err
	}
	return g.Name, nil
}
