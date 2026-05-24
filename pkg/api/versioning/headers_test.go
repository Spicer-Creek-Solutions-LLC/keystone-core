// SPDX-License-Identifier: Apache-2.0

package versioning_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/api/versioning"
)

func TestHeadersFor_UntrackedReturnsNil(t *testing.T) {
	r := versioning.NewRegistry()
	if h := r.HeadersFor("/never-registered"); h != nil {
		t.Errorf("untracked should return nil; got %v", h)
	}
}

func TestHeadersFor_CurrentEmitsNothing(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{Method: "/svc/M", Status: versioning.StatusCurrent})
	h := r.HeadersFor("/svc/M")
	for _, name := range []string{"Deprecation", "Sunset", "Link", "Warning"} {
		if v := h.Get(name); v != "" {
			t.Errorf("current endpoint should not emit %s; got %q", name, v)
		}
	}
}

func TestHeadersFor_DeprecatedEmitsDeprecation(t *testing.T) {
	r := versioning.NewRegistry()
	deprecatedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.Register(versioning.Endpoint{
		Method:       "/svc/M",
		Status:       versioning.StatusDeprecated,
		DeprecatedAt: deprecatedAt,
	})
	h := r.HeadersFor("/svc/M")

	if v := h.Get("Deprecation"); v == "" {
		t.Error("Deprecation header missing")
	} else if !strings.Contains(v, "GMT") {
		t.Errorf("Deprecation should be HTTP-date format; got %q", v)
	}
	if w := h.Get("Warning"); !strings.Contains(w, "299") || !strings.Contains(w, "deprecated") {
		t.Errorf("Warning header missing or wrong: %q", w)
	}
	if h.Get("Sunset") != "" {
		t.Error("Sunset should not be set without SunsetAt")
	}
}

func TestHeadersFor_DeprecatedWithSunset(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{
		Method:       "/svc/M",
		Status:       versioning.StatusDeprecated,
		DeprecatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SunsetAt:     time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
	})
	h := r.HeadersFor("/svc/M")

	if h.Get("Deprecation") == "" {
		t.Error("Deprecation missing")
	}
	if h.Get("Sunset") == "" {
		t.Error("Sunset missing")
	}
	if !strings.Contains(h.Get("Warning"), "2026-12-31") {
		t.Errorf("Warning should mention sunset date: %q", h.Get("Warning"))
	}
}

func TestHeadersFor_ReplacementEmitsLink(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{
		Method:      "/svc/M",
		Status:      versioning.StatusDeprecated,
		Replacement: "/svc/M2",
	})
	link := r.HeadersFor("/svc/M").Get("Link")
	if !strings.Contains(link, "</svc/M2>") {
		t.Errorf("Link should reference replacement; got %q", link)
	}
	if !strings.Contains(link, `rel="successor-version"`) {
		t.Errorf("Link should mark successor-version; got %q", link)
	}
}

func TestHeadersFor_AlphaEmitsWarning(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{Method: "/svc/M", Status: versioning.StatusAlpha})
	w := r.HeadersFor("/svc/M").Get("Warning")
	if !strings.Contains(w, "199") || !strings.Contains(w, "alpha") {
		t.Errorf("alpha warning: %q", w)
	}
}

func TestHeadersFor_BetaEmitsWarning(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{Method: "/svc/M", Status: versioning.StatusBeta})
	w := r.HeadersFor("/svc/M").Get("Warning")
	if !strings.Contains(w, "199") || !strings.Contains(w, "beta") {
		t.Errorf("beta warning: %q", w)
	}
}

func TestHeadersFor_RetiredOverrideViaSunset(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := versioning.NewRegistry()
	r.SetClock(func() time.Time { return now })
	r.Register(versioning.Endpoint{
		Method:   "/svc/M",
		Status:   versioning.StatusDeprecated,
		SunsetAt: now.Add(-time.Hour),
	})
	w := r.HeadersFor("/svc/M").Get("Warning")
	if !strings.Contains(w, "retired") {
		t.Errorf("expired sunset should produce retired warning: %q", w)
	}
}

func TestMetadataFor_MirrorsHeaders(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{
		Method:       "/svc/M",
		Status:       versioning.StatusDeprecated,
		DeprecatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SunsetAt:     time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		Replacement:  "/svc/M2",
	})
	md := r.MetadataFor("/svc/M")
	for _, key := range []string{"deprecation", "sunset", "link", "warning"} {
		if got := md.Get(key); len(got) == 0 {
			t.Errorf("metadata key %q missing", key)
		}
	}
}

func TestMetadataFor_CurrentReturnsNil(t *testing.T) {
	r := versioning.NewRegistry()
	r.Register(versioning.Endpoint{Method: "/svc/M", Status: versioning.StatusCurrent})
	if md := r.MetadataFor("/svc/M"); md != nil {
		t.Errorf("current endpoint should return nil metadata; got %v", md)
	}
}

func TestMetadataFor_UntrackedReturnsNil(t *testing.T) {
	r := versioning.NewRegistry()
	if md := r.MetadataFor("/missing"); md != nil {
		t.Errorf("untracked should return nil; got %v", md)
	}
}

// Sanity: HTTP date format is correct (RFC 7231).
func TestHeadersFor_DeprecationDateFormat(t *testing.T) {
	r := versioning.NewRegistry()
	at := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r.Register(versioning.Endpoint{
		Method:       "/svc/M",
		Status:       versioning.StatusDeprecated,
		DeprecatedAt: at,
	})
	got := r.HeadersFor("/svc/M").Get("Deprecation")
	if _, err := http.ParseTime(got); err != nil {
		t.Errorf("Deprecation %q is not a parseable HTTP date: %v", got, err)
	}
}
