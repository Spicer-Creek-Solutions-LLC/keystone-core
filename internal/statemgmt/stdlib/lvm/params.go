// SPDX-License-Identifier: Apache-2.0

package lvm

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const (
	paramPV       = "pv"
	paramVG       = "vg"
	paramVGPVs    = "pvs"
	paramLV       = "lv"
	paramSize     = "size"
	paramExtents  = "extents"
	paramResizeFS = "resize_fs"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramPV:       {},
	paramVG:       {},
	paramVGPVs:    {},
	paramLV:       {},
	paramSize:     {},
	paramExtents:  {},
	paramResizeFS: {},
	paramSeverity: {},
}

// devicePathRE matches an absolute /dev path that may contain
// alphanumerics, `/`, and the punctuation LVM and udev use
// (`_`/`.`/`+`/`-`). Notably no whitespace, no shell metacharacters.
var devicePathRE = regexp.MustCompile(`^/dev/[A-Za-z0-9/_.+-]+$`)

// lvmNameRE matches a VG / LV name. LVM is more permissive than this
// (it allows `+`/`.` etc.) but rejects names starting with `-` or `.`
// and the literals `.` / `..` / `snapshot` / `pvmove`. v1.0 keeps the
// pattern simple and lets LVM reject illegal names at apply time.
var lvmNameRE = regexp.MustCompile(`^[A-Za-z0-9_+.-]+$`)

// sizeRE matches a human size: digits + optional unit suffix (KMGTP).
// Accepts both cases. LVM tools accept `10G`, `500M`, `1T`, plain
// digits (megabytes). v1.0 does not accept fractional units (1.5G);
// operators express that as `1500M`.
var sizeRE = regexp.MustCompile(`^\d+[KMGTPkmgtp]?$`)

// extentsRE matches `<N>%{FREE|VG|PVS|ORIGIN}`. The lvcreate `-l`
// argument accepts more (plain extent counts) but v1.0 limits to the
// percentage forms.
var extentsRE = regexp.MustCompile(`^\d+%(FREE|VG|PVS|ORIGIN)$`)

// Op identifies which LVM object this declaration manages. The op is
// implied by which of `pv` / `vg` / `lv` is set.
type Op int

const (
	OpUnknown Op = iota
	OpPV
	OpVG
	OpLV
)

func (o Op) String() string {
	switch o {
	case OpPV:
		return "pv"
	case OpVG:
		return "vg"
	case OpLV:
		return "lv"
	}
	return "unknown"
}

type params struct {
	Label string
	State string
	Op    Op

	Device   string   // PV op
	VGName   string   // VG op + LV op (parent)
	VGPVs    []string // VG op
	LVName   string   // LV op
	Size     string   // LV op (mutex with Extents)
	Extents  string   // LV op (mutex with Size)
	ResizeFS bool     // LV op — pass lvextend --resizefs when growing
}

// locator returns a short human description of the object this decl
// manages, for use in Diff / Comment strings.
func (p *params) locator() string {
	switch p.Op {
	case OpPV:
		return fmt.Sprintf("PV %s", p.Device)
	case OpVG:
		return fmt.Sprintf("VG %s", p.VGName)
	case OpLV:
		return fmt.Sprintf("LV %s/%s", p.VGName, p.LVName)
	}
	return "<unknown>"
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: pv, vg, pvs, lv, size, extents, severity)", k)
		}
	}
	pvRaw, hasPV := decl.Params[paramPV]
	vgRaw, hasVG := decl.Params[paramVG]
	lvRaw, hasLV := decl.Params[paramLV]
	pvsRaw, hasPVs := decl.Params[paramVGPVs]
	sizeRaw, hasSize := decl.Params[paramSize]
	extentsRaw, hasExtents := decl.Params[paramExtents]
	resizeFSRaw, hasResizeFS := decl.Params[paramResizeFS]

	p := &params{Label: decl.Name, State: decl.State}

	// Determine op from which primary key (pv / vg / lv) is set.
	// LV op requires both lv: and vg:; VG op is vg: without lv:; PV op
	// is pv: alone.
	switch {
	case hasLV:
		if hasPV {
			return nil, fmt.Errorf("lv and pv are mutually exclusive")
		}
		if !hasVG {
			return nil, fmt.Errorf("lv requires vg (the parent volume group)")
		}
		p.Op = OpLV
	case hasVG:
		if hasPV {
			return nil, fmt.Errorf("vg and pv are mutually exclusive")
		}
		p.Op = OpVG
	case hasPV:
		p.Op = OpPV
	default:
		return nil, fmt.Errorf("exactly one of pv / vg / lv must be set")
	}

	// Read the per-op fields.
	switch p.Op {
	case OpPV:
		s, ok := pvRaw.(string)
		if !ok {
			return nil, fmt.Errorf("pv: expected string, got %T", pvRaw)
		}
		p.Device = strings.TrimSpace(s)
		if hasPVs || hasSize || hasExtents || hasResizeFS {
			return nil, fmt.Errorf("pv takes no auxiliary params (pvs/size/extents/resize_fs are for vg/lv)")
		}
	case OpVG:
		s, ok := vgRaw.(string)
		if !ok {
			return nil, fmt.Errorf("vg: expected string, got %T", vgRaw)
		}
		p.VGName = strings.TrimSpace(s)
		if hasSize || hasExtents || hasResizeFS {
			return nil, fmt.Errorf("size / extents / resize_fs are only valid with lv")
		}
		if hasPVs {
			pvs, err := parseStringList(pvsRaw)
			if err != nil {
				return nil, fmt.Errorf("pvs: %w", err)
			}
			p.VGPVs = pvs
		}
	case OpLV:
		lvS, ok := lvRaw.(string)
		if !ok {
			return nil, fmt.Errorf("lv: expected string, got %T", lvRaw)
		}
		p.LVName = strings.TrimSpace(lvS)
		vgS, ok := vgRaw.(string)
		if !ok {
			return nil, fmt.Errorf("vg: expected string, got %T", vgRaw)
		}
		p.VGName = strings.TrimSpace(vgS)
		if hasPVs {
			return nil, fmt.Errorf("pvs is only valid with vg (a VG op), not lv")
		}
		if hasSize {
			s, ok := sizeRaw.(string)
			if !ok {
				return nil, fmt.Errorf("size: expected string, got %T", sizeRaw)
			}
			p.Size = strings.TrimSpace(s)
		}
		if hasExtents {
			s, ok := extentsRaw.(string)
			if !ok {
				return nil, fmt.Errorf("extents: expected string, got %T", extentsRaw)
			}
			p.Extents = strings.TrimSpace(s)
		}
		if hasResizeFS {
			b, ok := resizeFSRaw.(bool)
			if !ok {
				return nil, fmt.Errorf("resize_fs: expected bool, got %T", resizeFSRaw)
			}
			p.ResizeFS = b
		}
	}
	return p, nil
}

// sizeToBytes converts an LVM human size (the `size:` param: digits +
// optional binary unit K/M/G/T/P, no suffix = MiB) to bytes. LVM uses
// powers of 1024 for these units. Fractional sizes are rejected at
// validate() time via sizeRE, so this only sees integers.
func sizeToBytes(size string) (uint64, error) {
	if size == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := uint64(1024 * 1024) // no suffix → MiB
	last := size[len(size)-1]
	digits := size
	if u, ok := unitMultiplier(last); ok {
		mult = u
		digits = size[:len(size)-1]
	}
	n, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", size, err)
	}
	return n * mult, nil
}

func unitMultiplier(c byte) (uint64, bool) {
	switch c {
	case 'K', 'k':
		return 1024, true
	case 'M', 'm':
		return 1024 * 1024, true
	case 'G', 'g':
		return 1024 * 1024 * 1024, true
	case 'T', 't':
		return 1024 * 1024 * 1024 * 1024, true
	case 'P', 'p':
		return 1024 * 1024 * 1024 * 1024 * 1024, true
	}
	return 0, false
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
		s = strings.TrimSpace(s)
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
	switch p.Op {
	case OpPV:
		if p.Device == "" {
			return fmt.Errorf("pv: empty device path")
		}
		if !devicePathRE.MatchString(p.Device) {
			return fmt.Errorf("pv: %q is not a /dev/ path or contains invalid characters", p.Device)
		}
	case OpVG:
		if p.VGName == "" {
			return fmt.Errorf("vg: empty name")
		}
		if !lvmNameRE.MatchString(p.VGName) {
			return fmt.Errorf("vg: invalid name %q", p.VGName)
		}
		if p.State == StatePresent {
			if len(p.VGPVs) == 0 {
				return fmt.Errorf("vg: present requires pvs (at least one device)")
			}
			for _, dev := range p.VGPVs {
				if !devicePathRE.MatchString(dev) {
					return fmt.Errorf("pvs: %q is not a /dev/ path or contains invalid characters", dev)
				}
			}
		}
		// state=absent: pvs is ignored (and at parse time it was accepted) — that's fine.
	case OpLV:
		if p.LVName == "" {
			return fmt.Errorf("lv: empty name")
		}
		if !lvmNameRE.MatchString(p.LVName) {
			return fmt.Errorf("lv: invalid name %q", p.LVName)
		}
		if p.VGName == "" {
			return fmt.Errorf("vg: empty parent name")
		}
		if !lvmNameRE.MatchString(p.VGName) {
			return fmt.Errorf("vg: invalid parent name %q", p.VGName)
		}
		if p.State == StatePresent {
			if p.Size == "" && p.Extents == "" {
				return fmt.Errorf("lv present requires exactly one of size / extents")
			}
			if p.Size != "" && p.Extents != "" {
				return fmt.Errorf("lv: size and extents are mutually exclusive")
			}
			if p.Size != "" && !sizeRE.MatchString(p.Size) {
				return fmt.Errorf("size: %q is not a valid LVM size (e.g. 10G, 500M, 1T, 1024)", p.Size)
			}
			if p.Extents != "" && !extentsRE.MatchString(p.Extents) {
				return fmt.Errorf("extents: %q is not a valid LVM extent spec (e.g. 50%%FREE, 100%%VG)", p.Extents)
			}
		}
	default:
		return fmt.Errorf("internal: no op selected")
	}
	return nil
}
