package validate

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ipPattern = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)

// Known external services that should not appear in air-gapped configs.
var knownExternalServices = map[string]string{
	"pool.ntp.org":           "NTP",
	"time.google.com":        "NTP",
	"time.windows.com":       "NTP",
	"docker.io":              "container registry",
	"registry.hub.docker.com": "container registry",
	"ghcr.io":                "container registry",
	"quay.io":                "container registry",
	"gcr.io":                 "container registry",
	"registry.k8s.io":        "container registry",
	"pypi.org":               "package registry",
	"npmjs.org":              "package registry",
	"rubygems.org":           "package registry",
}

// ConfigChecker scans YAML/JSON config files for external references.
type ConfigChecker struct {
	ConfigDir    string
	InternalNets []*net.IPNet
}

// Name returns the checker name.
func (c *ConfigChecker) Name() string { return "config-external-refs" }

// Category returns the check category.
func (c *ConfigChecker) Category() CheckCategory { return CategoryConfiguration }

// Check scans configuration files for external references.
func (c *ConfigChecker) Check(ctx context.Context) ([]Finding, error) {
	patterns := []string{"*.yaml", "*.yml", "*.json", "*.toml"}
	var files []string
	for _, p := range patterns {
		matches, err := filepath.Glob(filepath.Join(c.ConfigDir, p))
		if err != nil {
			continue
		}
		files = append(files, matches...)
	}

	if len(files) == 0 {
		return []Finding{{
			Category: CategoryConfiguration,
			Check:    "config-external-refs",
			Severity: SeverityWarn,
			Message:  "no configuration files found",
			Detail:   fmt.Sprintf("directory: %s", c.ConfigDir),
		}}, nil
	}

	var findings []Finding
	for _, path := range files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		ff, err := c.scanConfigFile(path)
		if err != nil {
			findings = append(findings, Finding{
				Category: CategoryConfiguration,
				Check:    "config-external-refs",
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("could not scan %s: %v", filepath.Base(path), err),
			})
			continue
		}
		findings = append(findings, ff...)
	}

	if len(findings) == 0 {
		findings = append(findings, Finding{
			Category: CategoryConfiguration,
			Check:    "config-external-refs",
			Severity: SeverityPass,
			Message:  "no external references in configuration files",
		})
	}
	return findings, nil
}

func (c *ConfigChecker) scanConfigFile(path string) ([]Finding, error) {
	f, err := os.Open(path) //#nosec G304 -- path from controlled glob
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var findings []Finding
	base := filepath.Base(path)
	scanner := bufio.NewScanner(f)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Check for known external services
		for service, svcType := range knownExternalServices {
			if strings.Contains(line, service) {
				severity := SeverityWarn
				if svcType == "container registry" || svcType == "package registry" {
					severity = SeverityFail
				}
				findings = append(findings, Finding{
					Category:    CategoryConfiguration,
					Check:       "config-external-refs",
					Severity:    severity,
					Message:     fmt.Sprintf("%s:%d: external %s reference: %s", base, lineNum, svcType, service),
					Remediation: fmt.Sprintf("Replace %s with a local %s mirror", service, svcType),
				})
			}
		}

		// Check for IP addresses outside internal networks
		if len(c.InternalNets) > 0 {
			ipMatches := ipPattern.FindAllString(line, -1)
			for _, ipStr := range ipMatches {
				ip := net.ParseIP(ipStr)
				if ip == nil {
					continue
				}
				if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
					continue
				}
				internal := false
				for _, cidr := range c.InternalNets {
					if cidr.Contains(ip) {
						internal = true
						break
					}
				}
				if !internal {
					findings = append(findings, Finding{
						Category:    CategoryConfiguration,
						Check:       "config-external-refs",
						Severity:    SeverityFail,
						Message:     fmt.Sprintf("%s:%d: external IP address: %s", base, lineNum, ipStr),
						Remediation: "Replace with an internal network address",
					})
				}
			}
		}
	}

	return findings, scanner.Err()
}
