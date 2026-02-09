package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LinkCheckResult represents the result of checking a link
type LinkCheckResult struct {
	Source     string `json:"source"`
	Link       string `json:"link"`
	Type       string `json:"type"`   // internal, external, anchor
	Status     string `json:"status"` // ok, broken, timeout, skipped
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// LinkChecker checks links in documentation
type LinkChecker struct {
	rootDir       string
	docsDir       string
	results       []LinkCheckResult
	mu            sync.Mutex
	client        *http.Client
	checkExternal bool
}

// NewLinkChecker creates a new link checker
func NewLinkChecker(rootDir string, checkExternal bool) *LinkChecker {
	return &LinkChecker{
		rootDir:       rootDir,
		docsDir:       filepath.Join(rootDir, "docs", "content", "en", "docs"),
		checkExternal: checkExternal,
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// CheckAll checks all links in documentation
func (lc *LinkChecker) CheckAll(verbose bool) []LinkCheckResult {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Limit concurrent checks

	filepath.Walk(lc.docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil //nolint:nilerr // continue walk on error
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr // continue walk on read error
		}

		relPath, _ := filepath.Rel(lc.docsDir, path)
		links := extractAllLinks(string(content))

		for _, link := range links {
			wg.Add(1)
			go func(src, lnk string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				result := lc.checkLink(src, lnk)
				lc.mu.Lock()
				lc.results = append(lc.results, result)
				lc.mu.Unlock()

				if verbose && result.Status != "ok" {
					fmt.Printf("  [%s] %s -> %s: %s\n", result.Status, src, lnk, result.Error)
				}
			}(relPath, link)
		}

		return nil
	})

	wg.Wait()
	return lc.results
}

func (lc *LinkChecker) checkLink(source, link string) LinkCheckResult {
	result := LinkCheckResult{
		Source: source,
		Link:   link,
	}

	// Skip Hugo shortcodes like {{< ref "..." >}} or {{% ... %}}
	if strings.HasPrefix(link, "{{") {
		result.Type = "shortcode"
		result.Status = "skipped"
		return result
	}

	// Determine link type
	if strings.HasPrefix(link, "#") {
		result.Type = "anchor"
		result.Status = "skipped" // Anchor checking is complex
		return result
	}

	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		result.Type = "external"
		if !lc.checkExternal {
			result.Status = "skipped"
			return result
		}
		return lc.checkExternalLink(result)
	}

	// Internal link
	result.Type = "internal"
	return lc.checkInternalLink(source, result)
}

func (lc *LinkChecker) checkExternalLink(result LinkCheckResult) LinkCheckResult {
	req, err := http.NewRequestWithContext(context.Background(), "HEAD", result.Link, http.NoBody)
	if err != nil {
		result.Status = "broken"
		result.Error = err.Error()
		return result
	}
	resp, err := lc.client.Do(req)
	if err != nil {
		result.Status = "broken"
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.Status = "ok"
	} else {
		result.Status = "broken"
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return result
}

func (lc *LinkChecker) checkInternalLink(source string, result LinkCheckResult) LinkCheckResult {
	link := result.Link

	// Remove anchor
	if idx := strings.Index(link, "#"); idx >= 0 {
		link = link[:idx]
	}

	if link == "" {
		result.Status = "ok" // Just an anchor
		return result
	}

	var targetPath string

	switch {
	case strings.HasPrefix(link, "/docs/"):
		// Hugo-style absolute URL: /docs/concepts/ -> docs/content/en/docs/concepts/
		// Strip /docs/ prefix and resolve from docsDir
		hugoPath := strings.TrimPrefix(link, "/docs/")
		hugoPath = strings.TrimSuffix(hugoPath, "/")
		targetPath = filepath.Join(lc.docsDir, hugoPath)
	case strings.HasPrefix(link, "/"):
		// Other absolute paths - check from root
		targetPath = filepath.Join(lc.rootDir, link)
	default:
		// Relative paths in Hugo: a file like concepts/gitops.md has URL /docs/concepts/gitops/
		// So relative links are from that "directory", meaning we treat the source file as a directory
		// For concepts/gitops.md with link ../events/, we want concepts/events/ not concepts/../events/

		// Get the source file's directory AND include the file basename (without .md) as another directory level
		sourceDir := filepath.Dir(filepath.Join(lc.docsDir, source))
		sourceBase := strings.TrimSuffix(filepath.Base(source), ".md")

		// If source is _index.md, don't add another directory level
		if sourceBase == "_index" || sourceBase == "index" {
			targetPath = filepath.Join(sourceDir, link)
		} else {
			// Treat the source file as if it were a directory
			// concepts/gitops.md + ../events/ = concepts/gitops/../events/ = concepts/events/
			virtualDir := filepath.Join(sourceDir, sourceBase)
			targetPath = filepath.Join(virtualDir, link)
		}
	}

	// Clean the path and remove trailing slashes
	targetPath = filepath.Clean(targetPath)

	// Check various extensions and Hugo conventions
	candidates := []string{
		targetPath,
		targetPath + ".md",
		filepath.Join(targetPath, "_index.md"),
		filepath.Join(targetPath, "index.md"),
	}

	// Also check as sibling (common Hugo pattern where relative links are siblings)
	// For concepts/control-plane.md with link "state-storage/", also check concepts/state-storage.md
	sourceDir := filepath.Dir(filepath.Join(lc.docsDir, source))
	siblingPath := filepath.Join(sourceDir, strings.TrimSuffix(link, "/"))
	siblingPath = filepath.Clean(siblingPath)
	if siblingPath != targetPath {
		candidates = append(candidates,
			siblingPath,
			siblingPath+".md",
			filepath.Join(siblingPath, "_index.md"),
			filepath.Join(siblingPath, "index.md"),
		)
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			result.Status = "ok"
			return result
		}
	}

	result.Status = "broken"
	result.Error = "file not found"
	return result
}

func extractAllLinks(content string) []string {
	var links []string
	seen := make(map[string]bool)

	// Match markdown links [text](url)
	re := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	matches := re.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) > 2 {
			link := m[2]
			// Skip mailto links
			if strings.HasPrefix(link, "mailto:") {
				continue
			}
			if !seen[link] {
				seen[link] = true
				links = append(links, link)
			}
		}
	}

	return links
}

// GenerateLinkReport generates a report of link check results
func GenerateLinkReport(results []LinkCheckResult) string {
	var sb strings.Builder

	var broken, ok, skipped int
	brokenLinks := make([]LinkCheckResult, 0)

	for _, r := range results {
		switch r.Status {
		case "ok":
			ok++
		case "broken":
			broken++
			brokenLinks = append(brokenLinks, r)
		case "skipped":
			skipped++
		}
	}

	sb.WriteString("# Link Check Report\n\n")
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- Total links checked: %d\n", len(results)))
	sb.WriteString(fmt.Sprintf("- OK: %d\n", ok))
	sb.WriteString(fmt.Sprintf("- Broken: %d\n", broken))
	sb.WriteString(fmt.Sprintf("- Skipped: %d\n", skipped))
	sb.WriteString("\n")

	if len(brokenLinks) > 0 {
		sb.WriteString("## Broken Links\n\n")
		sb.WriteString("| Source | Link | Type | Error |\n")
		sb.WriteString("|--------|------|------|-------|\n")
		for _, r := range brokenLinks {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				r.Source, r.Link, r.Type, r.Error))
		}
	}

	return sb.String()
}
