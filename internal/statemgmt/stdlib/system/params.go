// SPDX-License-Identifier: Apache-2.0

package system

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

// Banner kinds. The string is the value the operator writes for
// `banner:`; the file path is resolved by the Provider.
const (
	BannerMOTD     = "motd"
	BannerIssue    = "issue"
	BannerIssueNet = "issue_net"
)

var validBanners = map[string]struct{}{
	BannerMOTD:     {},
	BannerIssue:    {},
	BannerIssueNet: {},
}

// Reboot defaults.
const (
	defaultWhenFile = "/var/run/reboot-required"
	defaultDelay    = 1
	maxDelay        = 60
)

const (
	paramBanner   = "banner"
	paramContent  = "content"
	paramReboot   = "reboot"
	paramWhenFile = "when_file"
	paramDelay    = "delay"
	paramLocale   = "locale"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramBanner:   {},
	paramContent:  {},
	paramReboot:   {},
	paramWhenFile: {},
	paramDelay:    {},
	paramLocale:   {},
	paramSeverity: {},
}

// localeRE matches POSIX locale identifiers: `C`, `POSIX`, or
// `language[_TERRITORY][.codeset][@modifier]`.
var localeRE = regexp.MustCompile(`^(C|POSIX|[a-z]{2,3}(_[A-Z]{2})?(\.[A-Za-z0-9-]+)?(@[a-z]+)?)$`)

// Op identifies which operation the declaration is performing. The op
// is implied by which params are set.
type Op int

const (
	OpUnknown Op = iota
	OpBanner
	OpReboot
	OpLocale
)

func (o Op) String() string {
	switch o {
	case OpBanner:
		return "banner"
	case OpReboot:
		return "reboot"
	case OpLocale:
		return "locale"
	}
	return "unknown"
}

type params struct {
	Label string
	State string
	Op    Op

	// OpBanner
	BannerName    string
	BannerContent string

	// OpReboot
	WhenFile string
	Delay    int

	// OpLocale
	Locale string
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: banner, content, reboot, when_file, delay, locale, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State, WhenFile: defaultWhenFile, Delay: defaultDelay}

	bannerRaw, hasBanner := decl.Params[paramBanner]
	rebootRaw, hasReboot := decl.Params[paramReboot]
	localeRaw, hasLocale := decl.Params[paramLocale]
	contentRaw, hasContent := decl.Params[paramContent]
	whenRaw, hasWhen := decl.Params[paramWhenFile]
	delayRaw, hasDelay := decl.Params[paramDelay]

	set := 0
	if hasBanner {
		set++
	}
	if hasReboot {
		set++
	}
	if hasLocale {
		set++
	}
	if set != 1 {
		return nil, fmt.Errorf("exactly one of banner / reboot / locale must be set (got %d)", set)
	}

	if hasBanner {
		s, ok := bannerRaw.(string)
		if !ok {
			return nil, fmt.Errorf("banner: expected string, got %T", bannerRaw)
		}
		p.Op = OpBanner
		p.BannerName = strings.TrimSpace(s)
		// content must accompany banner (may be empty for state=absent
		// callers that use the default — we still require the key to be
		// present so the operator's intent is explicit).
		if !hasContent {
			return nil, fmt.Errorf("banner requires content (use \"\" for an empty file)")
		}
		c, ok := contentRaw.(string)
		if !ok {
			return nil, fmt.Errorf("content: expected string, got %T", contentRaw)
		}
		p.BannerContent = c
		// when_file / delay are reboot-only
		if hasWhen || hasDelay {
			return nil, fmt.Errorf("when_file / delay are only valid with reboot")
		}
	}
	if hasReboot {
		b, ok := rebootRaw.(bool)
		if !ok {
			return nil, fmt.Errorf("reboot: expected bool, got %T", rebootRaw)
		}
		if !b {
			return nil, fmt.Errorf("reboot: must be true (false has no meaning; omit the param to skip this op)")
		}
		p.Op = OpReboot
		if hasContent {
			return nil, fmt.Errorf("content is only valid with banner")
		}
		if hasWhen {
			s, ok := whenRaw.(string)
			if !ok {
				return nil, fmt.Errorf("when_file: expected string, got %T", whenRaw)
			}
			p.WhenFile = strings.TrimSpace(s)
		}
		if hasDelay {
			d, err := coerceDelay(delayRaw)
			if err != nil {
				return nil, fmt.Errorf("delay: %w", err)
			}
			p.Delay = d
		}
	}
	if hasLocale {
		s, ok := localeRaw.(string)
		if !ok {
			return nil, fmt.Errorf("locale: expected string, got %T", localeRaw)
		}
		p.Op = OpLocale
		p.Locale = strings.TrimSpace(s)
		if hasContent || hasWhen || hasDelay {
			return nil, fmt.Errorf("locale takes no auxiliary params (content/when_file/delay are for banner/reboot)")
		}
	}
	return p, nil
}

func coerceDelay(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return int(v), nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func (p *params) validate() error {
	switch p.State {
	case StatePresent, StateAbsent:
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	switch p.Op {
	case OpBanner:
		if _, ok := validBanners[p.BannerName]; !ok {
			return fmt.Errorf("banner: must be one of motd, issue, issue_net; got %q", p.BannerName)
		}
	case OpReboot:
		if p.State != StatePresent {
			return fmt.Errorf("reboot supports state=present only (got %q) — to cancel a scheduled reboot use `shutdown -c` directly", p.State)
		}
		if p.WhenFile == "" {
			return fmt.Errorf("when_file: must be a non-empty absolute path")
		}
		if !strings.HasPrefix(p.WhenFile, "/") {
			return fmt.Errorf("when_file: %q must be an absolute path", p.WhenFile)
		}
		if p.Delay < 0 || p.Delay > maxDelay {
			return fmt.Errorf("delay: must be between 0 and %d (minutes); got %d", maxDelay, p.Delay)
		}
	case OpLocale:
		if p.State != StatePresent {
			return fmt.Errorf("locale supports state=present only (got %q)", p.State)
		}
		if !localeRE.MatchString(p.Locale) {
			return fmt.Errorf("locale: %q is not a valid POSIX locale identifier", p.Locale)
		}
	default:
		return fmt.Errorf("internal: no op selected")
	}
	return nil
}
