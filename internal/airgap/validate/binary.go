package validate

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var urlPattern = regexp.MustCompile(`https?://[a-zA-Z0-9._:/-]+`)

// BinaryChecker scans kscore-* binaries for embedded URLs that reference
// external hosts, which would fail in air-gapped environments.
type BinaryChecker struct {
	BinaryDir    string
	AllowedHosts []string
}

// Name returns the checker name.
func (c *BinaryChecker) Name() string { return "binary-urls" }

// Category returns the check category.
func (c *BinaryChecker) Category() CheckCategory { return CategoryBinary }

// Check scans binaries and reports external URLs.
func (c *BinaryChecker) Check(ctx context.Context) ([]Finding, error) {
	pattern := filepath.Join(c.BinaryDir, "kscore-*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}

	if len(matches) == 0 {
		return []Finding{{
			Category: CategoryBinary,
			Check:    "binary-urls",
			Severity: SeverityWarn,
			Message:  "no kscore-* binaries found",
			Detail:   fmt.Sprintf("directory: %s", c.BinaryDir),
		}}, nil
	}

	allowed := defaultAllowedHosts()
	for _, h := range c.AllowedHosts {
		allowed[h] = true
	}

	var findings []Finding
	for _, path := range matches {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		urls, err := scanFileForURLs(path)
		if err != nil {
			findings = append(findings, Finding{
				Category: CategoryBinary,
				Check:    "binary-urls",
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("could not scan %s: %v", filepath.Base(path), err),
			})
			continue
		}

		external := filterExternalURLs(urls, allowed)
		if len(external) == 0 {
			findings = append(findings, Finding{
				Category: CategoryBinary,
				Check:    "binary-urls",
				Severity: SeverityPass,
				Message:  fmt.Sprintf("%s: no external URLs", filepath.Base(path)),
			})
		} else {
			findings = append(findings, Finding{
				Category:    CategoryBinary,
				Check:       "binary-urls",
				Severity:    SeverityFail,
				Message:     fmt.Sprintf("%s: %d external URL(s) found", filepath.Base(path), len(external)),
				Detail:      strings.Join(external, ", "),
				Remediation: "Remove or replace external URLs with local equivalents",
			})
		}
	}
	return findings, nil
}

func defaultAllowedHosts() map[string]bool {
	return map[string]bool{
		"localhost":  true,
		"127.0.0.1":  true,
		"[::1]":      true,
		"golang.org": true,
		"go.dev":     true,
	}
}

func scanFileForURLs(path string) ([]string, error) {
	f, err := os.Open(path) //#nosec G304 -- path from controlled glob
	if err != nil {
		return nil, err
	}
	defer f.Close()

	const chunkSize = 64 * 1024
	const overlap = 256

	var urls []string
	seen := make(map[string]bool)
	buf := make([]byte, chunkSize+overlap)
	carry := 0

	for {
		n, err := f.Read(buf[carry:])
		if n == 0 && err != nil {
			break
		}
		data := buf[:carry+n]

		found := urlPattern.FindAll(data, -1)
		for _, u := range found {
			s := string(u)
			if !seen[s] {
				seen[s] = true
				urls = append(urls, s)
			}
		}

		// Keep last `overlap` bytes for next iteration
		if len(data) > overlap {
			copy(buf, data[len(data)-overlap:])
			carry = overlap
		} else {
			carry = 0
		}

		if err != nil {
			break
		}
	}
	return urls, nil
}

func filterExternalURLs(urls []string, allowed map[string]bool) []string {
	var external []string
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		host := parsed.Hostname()
		if host == "" {
			continue
		}
		if !allowed[host] {
			external = append(external, host)
		}
	}
	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, h := range external {
		if !seen[h] {
			seen[h] = true
			unique = append(unique, h)
		}
	}
	return unique
}
