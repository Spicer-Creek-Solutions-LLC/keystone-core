package restconf

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/rest"
)

// discoverRootPath attempts to discover the RESTCONF root path via
// the /.well-known/host-meta resource (RFC 8040 Section 3.1).
// Falls back to DefaultRootPath if discovery fails.
func discoverRootPath(ctx context.Context, client *rest.Client, auth rest.Authenticator) string {
	req, err := client.NewRequest(ctx, http.MethodGet, "/.well-known/host-meta", nil)
	if err != nil {
		return DefaultRootPath
	}
	req.Header.Set("Accept", "application/xrd+xml")
	if auth != nil {
		if err := auth.Authenticate(req); err != nil {
			return DefaultRootPath
		}
	}

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return DefaultRootPath
	}

	r, err := rest.NewResponse(resp)
	if err != nil {
		return DefaultRootPath
	}

	path := parseHostMeta(r.Body())
	if path == "" {
		return DefaultRootPath
	}
	return path
}

// xrd is the minimal structure for parsing /.well-known/host-meta XRD.
type xrd struct {
	XMLName xml.Name  `xml:"XRD"`
	Links   []xrdLink `xml:"Link"`
}

type xrdLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

// parseHostMeta extracts the restconf link from XRD XML.
func parseHostMeta(data []byte) string {
	var doc xrd
	if err := xml.Unmarshal(data, &doc); err != nil {
		return ""
	}
	for _, link := range doc.Links {
		if link.Rel == "restconf" {
			return strings.TrimRight(link.Href, "/")
		}
	}
	return ""
}

// YANGLibraryVersion returns the YANG library version from the server.
func (a *Adapter) YANGLibraryVersion(ctx context.Context) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return "", fmt.Errorf("not connected")
	}

	url := a.rootPath + "/yang-library-version"
	body, _, _, err := a.doRequest(ctx, http.MethodGet, url, nil, string(ContentTypeYANGJSON))
	if err != nil {
		return "", err
	}

	var result struct {
		Version string `json:"yang-library-version"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil
	}
	return result.Version, nil
}

// ServerModules returns the YANG modules reported by the RESTCONF server.
func (a *Adapter) ServerModules(ctx context.Context) ([]protocols.RestconfModule, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected {
		return nil, fmt.Errorf("not connected")
	}

	url := a.rootPath + "/data/ietf-yang-library:modules-state"
	body, _, _, err := a.doRequest(ctx, http.MethodGet, url, nil, string(ContentTypeYANGJSON))
	if err != nil {
		return nil, err
	}

	var result struct {
		ModulesState struct {
			Module []Module `json:"module"`
		} `json:"ietf-yang-library:modules-state"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse modules response: %w", err)
	}

	modules := make([]protocols.RestconfModule, len(result.ModulesState.Module))
	for i, m := range result.ModulesState.Module {
		modules[i] = protocols.RestconfModule{
			Name:      m.Name,
			Revision:  m.Revision,
			Namespace: m.Namespace,
		}
	}
	return modules, nil
}

// RootPath returns the discovered or configured RESTCONF API root path.
func (a *Adapter) RootPath() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.rootPath
}
