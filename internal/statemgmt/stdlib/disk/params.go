// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const (
	paramDevice      = "device"
	paramFstype      = "fstype"
	paramMkfsOptions = "mkfs_options"
	paramForce       = "force"
	paramResizeFS    = "resize_fs"
	paramSeverity    = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramDevice:      {},
	paramFstype:      {},
	paramMkfsOptions: {},
	paramForce:       {},
	paramResizeFS:    {},
	paramSeverity:    {},
}

// resizableFstypes is the v0.5 fs-resize support set. Resizing ext is
// device-based (`resize2fs <device>`) with a predictable, mount-free
// idempotency check; xfs / btrfs / f2fs (mount-required, different size
// math) are V1X.
var resizableFstypes = map[string]struct{}{
	"ext2": {}, "ext3": {}, "ext4": {},
}

// validFstypes is the curated v1.0 catalog. Adding entries means
// confirming the mkfs binary, blkid TYPE= name, and wipefs
// behaviour. Catalog expansion is V1X.
var validFstypes = map[string]struct{}{
	"ext2":  {},
	"ext3":  {},
	"ext4":  {},
	"xfs":   {},
	"btrfs": {},
	"f2fs":  {},
	"vfat":  {},
	"exfat": {},
	"swap":  {},
}

// KnownFstypes returns the v1.0 catalog in sorted order. Used in
// error messages and exported for documentation tooling.
func KnownFstypes() []string {
	out := make([]string, 0, len(validFstypes))
	for k := range validFstypes {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// devicePathRE matches an absolute /dev path. Same shape as the
// LVM module's check — no shell metacharacters or whitespace.
var devicePathRE = regexp.MustCompile(`^/dev/[A-Za-z0-9/_.+-]+$`)

// optionRE matches one element of mkfs_options. Permits the
// characters mkfs flags + values legitimately need (letters,
// digits, `-`, `_`, `=`, `,`, `.`, `:`, `/`, `%`); rejects shell
// metacharacters, whitespace, and control characters.
var optionRE = regexp.MustCompile(`^[A-Za-z0-9_=,./:%+-]+$`)

type params struct {
	Label       string
	State       string
	Device      string
	Fstype      string // required for present; ignored for absent
	MkfsOptions []string
	Force       bool
	ResizeFS    bool // grow the fs to fill the device (ext2/3/4, present only)
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: device, fstype, mkfs_options, force, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramDevice]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("device: expected string, got %T", raw)
		}
		p.Device = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramFstype]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("fstype: expected string, got %T", raw)
		}
		p.Fstype = strings.ToLower(strings.TrimSpace(s))
	}
	if raw, ok := decl.Params[paramMkfsOptions]; ok {
		opts, err := parseStringList(raw)
		if err != nil {
			return nil, fmt.Errorf("mkfs_options: %w", err)
		}
		p.MkfsOptions = opts
	}
	if raw, ok := decl.Params[paramForce]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("force: expected bool, got %T", raw)
		}
		p.Force = b
	}
	if raw, ok := decl.Params[paramResizeFS]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("resize_fs: expected bool, got %T", raw)
		}
		p.ResizeFS = b
	}
	return p, nil
}

func parseStringList(raw any) ([]string, error) {
	v, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list of strings, got %T", raw)
	}
	out := make([]string, 0, len(v))
	for i, e := range v {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("element %d: expected string, got %T", i, e)
		}
		if s == "" {
			return nil, fmt.Errorf("element %d: empty", i)
		}
		out = append(out, s)
	}
	return out, nil
}

func (p *params) validate() error {
	switch p.State {
	case StatePresent, StateAbsent:
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	if p.Device == "" {
		return fmt.Errorf("device: empty")
	}
	if !devicePathRE.MatchString(p.Device) {
		return fmt.Errorf("device: %q is not a /dev/ path or contains invalid characters", p.Device)
	}
	if p.State == StatePresent {
		if p.Fstype == "" {
			return fmt.Errorf("fstype: required for state=present (catalog: %s)", strings.Join(KnownFstypes(), ", "))
		}
		if _, ok := validFstypes[p.Fstype]; !ok {
			return fmt.Errorf("fstype: %q is not in the v1.0 catalog (allowed: %s)", p.Fstype, strings.Join(KnownFstypes(), ", "))
		}
	} else {
		// absent: mkfs_options has no purpose and is rejected for clarity.
		if len(p.MkfsOptions) > 0 {
			return fmt.Errorf("mkfs_options is only valid with state=present")
		}
		if p.ResizeFS {
			return fmt.Errorf("resize_fs is only valid with state=present")
		}
	}
	if p.ResizeFS {
		if _, ok := resizableFstypes[p.Fstype]; !ok {
			return fmt.Errorf("resize_fs is supported for ext2/3/4 only in v0.5 (got fstype %q); xfs/btrfs/f2fs resize is V1X", p.Fstype)
		}
	}
	for i, opt := range p.MkfsOptions {
		if !optionRE.MatchString(opt) {
			return fmt.Errorf("mkfs_options[%d]: %q contains an unsafe character (whitespace / control / shell metacharacter not allowed; use mkfs flags + values only)", i, opt)
		}
	}
	return nil
}
