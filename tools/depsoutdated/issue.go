// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// issueTitle is the fixed title of the single tracking issue this tool
// maintains. Matching on it keeps the sync idempotent: one persistent
// issue is reused/updated/closed rather than a new one opened each night.
const issueTitle = "Dependency freshness: direct deps with updates available"

// syncIssue creates, updates, or closes the tracking issue so the
// deps-outdated findings surface as an issue (and thus an issue
// notification) instead of living only in a CI log nobody reads.
//
// It is best-effort by design: every failure path logs to stderr and
// returns without exiting non-zero, so a missing token or an API hiccup
// can never turn the informational nightly red. If the tracking issue
// stops appearing, the warning in the job log says why (most likely the
// Actions token lacks `issues: write`).
//
// Behaviour:
//   - outdated, no open issue  → create it
//   - outdated, issue open      → update its body with the latest report
//   - up-to-date, issue open    → close it (drift cleared)
//   - up-to-date, no open issue → nothing to do
func syncIssue(outdated bool, report string) {
	api := strings.TrimRight(os.Getenv("GITHUB_API_URL"), "/")
	repo := os.Getenv("GITHUB_REPOSITORY")
	token := os.Getenv("GITHUB_TOKEN")
	if api == "" || repo == "" || token == "" {
		fmt.Fprintln(os.Stderr, "depsoutdated: --issue: GITHUB_API_URL/REPOSITORY/TOKEN not all set; skipping issue sync")
		return
	}
	c := &issueClient{base: api + "/repos/" + repo, token: token, http: &http.Client{Timeout: 30 * time.Second}}

	num, err := c.findOpen(issueTitle)
	if err != nil {
		fmt.Fprintln(os.Stderr, "depsoutdated: --issue: find tracking issue:", err)
		return
	}

	switch {
	case outdated && num == 0:
		if err := c.create(issueTitle, issueBody(report)); err != nil {
			fmt.Fprintln(os.Stderr, "depsoutdated: --issue: create:", err)
			return
		}
		fmt.Fprintln(os.Stderr, "depsoutdated: --issue: opened tracking issue")
	case outdated && num != 0:
		if err := c.patch(num, map[string]any{"body": issueBody(report)}); err != nil {
			fmt.Fprintln(os.Stderr, "depsoutdated: --issue: update:", err)
			return
		}
		fmt.Fprintf(os.Stderr, "depsoutdated: --issue: updated tracking issue #%d\n", num)
	case !outdated && num != 0:
		body := "All direct dependencies are on their latest release. Closing; will reopen automatically when drift reappears.\n"
		if err := c.patch(num, map[string]any{"body": body, "state": "closed"}); err != nil {
			fmt.Fprintln(os.Stderr, "depsoutdated: --issue: close:", err)
			return
		}
		fmt.Fprintf(os.Stderr, "depsoutdated: --issue: closed tracking issue #%d (drift cleared)\n", num)
	}
}

// issueBody wraps the plain-text report in a fenced block plus a short
// provenance note so a reader knows it is machine-maintained.
func issueBody(report string) string {
	return "Automated nightly report from `.forgejo/workflows/ci-full.yml` (deps-outdated job). " +
		"Auto-updated on each run; do not edit by hand — `make deps-outdated` reproduces it locally.\n\n" +
		"```\n" + strings.TrimRight(report, "\n") + "\n```\n"
}

// issueClient is a thin Forgejo issue-API wrapper over the run token.
type issueClient struct {
	base  string
	token string
	http  *http.Client
}

func (c *issueClient) do(method, url string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// findOpen returns the number of the open issue with the exact given
// title, or 0 if none. The `q` filter narrows server-side; the exact
// title match guards against partial hits.
func (c *issueClient) findOpen(title string) (int, error) {
	data, err := c.do(http.MethodGet, c.base+"/issues?state=open&type=issues&limit=50", nil)
	if err != nil {
		return 0, err
	}
	var issues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(data, &issues); err != nil {
		return 0, err
	}
	for _, is := range issues {
		if is.Title == title {
			return is.Number, nil
		}
	}
	return 0, nil
}

func (c *issueClient) create(title, body string) error {
	_, err := c.do(http.MethodPost, c.base+"/issues", map[string]any{"title": title, "body": body})
	return err
}

func (c *issueClient) patch(num int, fields map[string]any) error {
	_, err := c.do(http.MethodPatch, fmt.Sprintf("%s/issues/%d", c.base, num), fields)
	return err
}
