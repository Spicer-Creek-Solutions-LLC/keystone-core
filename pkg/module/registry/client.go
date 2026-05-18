package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
	"go.keystone-core.io/keystone-core/pkg/module/verify"
	"go.keystone-core.io/keystone-core/pkg/semver"
)

// Client talks to a remote kscore-registry over the Go module-proxy
// HTTP endpoints (task 9). It implements the task-6 resolver.Source
// so the resolver resolves against a remote registry, and adds
// fetch/publish for the kscore-module CLI (task 14).
type Client struct {
	base string
	hc   *http.Client
}

// NewClient returns a Client for base (e.g. "http://localhost:8181").
// A nil http.Client uses http.DefaultClient.
func NewClient(base string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{base: strings.TrimRight(base, "/"), hc: hc}
}

func (c *Client) get(ctx context.Context, p string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+p, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

// ListVersions implements resolver.Source via /@v/list + /@v/<v>.info.
func (c *Client) ListVersions(ctx context.Context, module string) ([]resolver.ModuleVersion, error) {
	body, code, err := c.get(ctx, "/"+module+"/@v/list")
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, module)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("registry: list %s: status %d", module, code)
	}
	var out []resolver.ModuleVersion
	for _, line := range strings.Fields(string(body)) {
		v, perr := semver.Parse(line)
		if perr != nil {
			continue
		}
		ib, ic, ierr := c.get(ctx, "/"+module+"/@v/"+line+".info")
		if ierr != nil || ic != http.StatusOK {
			return nil, fmt.Errorf("registry: info %s@%s: status %d", module, line, ic)
		}
		var info struct {
			Hash string `json:"hash"`
		}
		_ = json.Unmarshal(ib, &info)
		// The HTTP .info contract is {Version,Time}; Hash comes from
		// the content the client itself stores. When the server does
		// not expose Hash, fall back to hashing the fetched ZIP.
		if info.Hash == "" {
			zip, ferr := c.FetchZip(ctx, module, v)
			if ferr != nil {
				return nil, ferr
			}
			info.Hash = cas.HashBytes(zip)
		}
		out = append(out, resolver.ModuleVersion{Version: v, Hash: info.Hash})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %s (no versions)", ErrNotFound, module)
	}
	return out, nil
}

// GetManifest implements resolver.Source via /@v/<v>.mod.
func (c *Client) GetManifest(ctx context.Context, module string, v semver.Version) (*manifest.Manifest, error) {
	b, code, err := c.get(ctx, "/"+module+"/@v/"+v.String()+".mod")
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s@%s", ErrNotFound, module, v)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("registry: mod %s@%s: status %d", module, v, code)
	}
	return manifest.UnmarshalManifest(b)
}

// FetchZip downloads the module ZIP for a version.
func (c *Client) FetchZip(ctx context.Context, module string, v semver.Version) ([]byte, error) {
	b, code, err := c.get(ctx, "/"+module+"/@v/"+v.String()+".zip")
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s@%s zip", ErrNotFound, module, v)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("registry: zip %s@%s: status %d", module, v, code)
	}
	return b, nil
}

// FetchSignature downloads the detached signature for a version.
// ok=false (no error) when the module was published unsigned.
func (c *Client) FetchSignature(ctx context.Context, module string, v semver.Version) (verify.Signature, bool, error) {
	b, code, err := c.get(ctx, "/"+module+"/@v/"+v.String()+".sig")
	if err != nil {
		return verify.Signature{}, false, err
	}
	if code == http.StatusNotFound {
		return verify.Signature{}, false, nil
	}
	if code != http.StatusOK {
		return verify.Signature{}, false, fmt.Errorf("registry: sig %s@%s: status %d", module, v, code)
	}
	sig, perr := verify.UnmarshalSignature(b)
	if perr != nil {
		return verify.Signature{}, false, perr
	}
	return sig, true, nil
}

// Publish POSTs a multipart publish (manifest + module ZIP +
// optional signature) to the registry.
func (c *Client) Publish(ctx context.Context, manifestYAML, zip, sig []byte) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	add := func(field, fname string, data []byte) error {
		fwr, err := mw.CreateFormFile(field, fname)
		if err != nil {
			return err
		}
		_, err = fwr.Write(data)
		return err
	}
	if err := add("manifest", "manifest.yaml", manifestYAML); err != nil {
		return err
	}
	if err := add("module", "module.zip", zip); err != nil {
		return err
	}
	if len(sig) > 0 {
		if err := add("signature", "module.sig", sig); err != nil {
			return err
		}
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/publish", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusConflict:
		return fmt.Errorf("%w (remote)", ErrVersionExists)
	case http.StatusBadRequest:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s", ErrInvalidModule, strings.TrimSpace(string(body)))
	default:
		return fmt.Errorf("registry: publish status %d", resp.StatusCode)
	}
}

var _ resolver.Source = (*Client)(nil)
