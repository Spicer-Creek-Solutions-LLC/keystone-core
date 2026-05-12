package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// client is a minimal Forgejo (Gitea-compatible) REST client scoped to one repo.
type client struct {
	base  string // e.g. http://192.168.10.21:3000
	repo  string // owner/name
	token string
	http  *http.Client
}

func newClient(host, repo, token string) *client {
	return &client{
		base:  strings.TrimRight(host, "/"),
		repo:  repo,
		token: token,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *client) do(method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	u := c.base + "/api/v1/repos/" + c.repo + path
	req, err := http.NewRequest(method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, u, resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// getPaged fetches a paginated collection, appending each page into out via the
// supplied accumulator until a short page is returned.
func (c *client) getPaged(path string, fetch func(page int) (int, error)) error {
	for page := 1; ; page++ {
		n, err := fetch(page)
		if err != nil {
			return err
		}
		if n < pageLimit {
			return nil
		}
	}
}

const pageLimit = 50

func pageQuery(extra url.Values, page int) string {
	v := url.Values{}
	for k, vs := range extra {
		v[k] = vs
	}
	v.Set("page", fmt.Sprint(page))
	v.Set("limit", fmt.Sprint(pageLimit))
	return "?" + v.Encode()
}

// --- labels ---

type label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Exclusive   bool   `json:"exclusive"`
	IsArchived  bool   `json:"is_archived"`
}

type labelPayload struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Exclusive   bool   `json:"exclusive"`
}

func (c *client) listLabels() ([]label, error) {
	var all []label
	err := c.getPaged("/labels", func(page int) (int, error) {
		var batch []label
		if err := c.do(http.MethodGet, "/labels"+pageQuery(nil, page), nil, &batch); err != nil {
			return 0, err
		}
		all = append(all, batch...)
		return len(batch), nil
	})
	return all, err
}

func (c *client) createLabel(p labelPayload) (label, error) {
	var l label
	err := c.do(http.MethodPost, "/labels", p, &l)
	return l, err
}

func (c *client) editLabel(id int64, p labelPayload) (label, error) {
	var l label
	err := c.do(http.MethodPatch, fmt.Sprintf("/labels/%d", id), p, &l)
	return l, err
}

// --- milestones ---

type milestone struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	DueOn       string `json:"due_on,omitempty"`
}

type milestonePayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state,omitempty"`
	DueOn       string `json:"due_on,omitempty"`
}

func (c *client) listMilestones() ([]milestone, error) {
	var all []milestone
	q := url.Values{"state": {"all"}}
	err := c.getPaged("/milestones", func(page int) (int, error) {
		var batch []milestone
		if err := c.do(http.MethodGet, "/milestones"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		all = append(all, batch...)
		return len(batch), nil
	})
	return all, err
}

func (c *client) createMilestone(p milestonePayload) (milestone, error) {
	var m milestone
	err := c.do(http.MethodPost, "/milestones", p, &m)
	return m, err
}

func (c *client) editMilestone(id int64, p milestonePayload) (milestone, error) {
	var m milestone
	err := c.do(http.MethodPatch, fmt.Sprintf("/milestones/%d", id), p, &m)
	return m, err
}

// --- issues ---

type issue struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	Labels    []label    `json:"labels"`
	Milestone *milestone `json:"milestone"`
}

type issuePayload struct {
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Labels    []int64 `json:"labels,omitempty"`
	Milestone int64   `json:"milestone,omitempty"`
}

func (c *client) listIssues() ([]issue, error) {
	var all []issue
	q := url.Values{"state": {"all"}, "type": {"issues"}}
	err := c.getPaged("/issues", func(page int) (int, error) {
		var batch []issue
		if err := c.do(http.MethodGet, "/issues"+pageQuery(q, page), nil, &batch); err != nil {
			return 0, err
		}
		all = append(all, batch...)
		return len(batch), nil
	})
	return all, err
}

func (c *client) createIssue(p issuePayload) (issue, error) {
	var is issue
	err := c.do(http.MethodPost, "/issues", p, &is)
	return is, err
}

func (c *client) editIssueBody(index int64, body string) (issue, error) {
	var is issue
	err := c.do(http.MethodPatch, fmt.Sprintf("/issues/%d", index), map[string]string{"body": body}, &is)
	return is, err
}

// editIssueMilestone sets (milestoneID > 0) or clears (milestoneID == 0) an
// issue's milestone.
func (c *client) editIssueMilestone(index, milestoneID int64) error {
	return c.do(http.MethodPatch, fmt.Sprintf("/issues/%d", index), map[string]int64{"milestone": milestoneID}, nil)
}

func (c *client) addIssueLabels(index int64, ids []int64) error {
	return c.do(http.MethodPost, fmt.Sprintf("/issues/%d/labels", index), map[string][]int64{"labels": ids}, nil)
}

func (c *client) removeIssueLabel(index, labelID int64) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/issues/%d/labels/%d", index, labelID), nil, nil)
}
