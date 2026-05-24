// SPDX-License-Identifier: Apache-2.0

// Package argoexec is the stdlib-REST implementation of the rollback
// [rollback.ArgoClient] seam. It talks to the ArgoCD API server's
// grpc-gateway REST surface with net/http only — deliberately no
// argo-cd/v3 dependency (that module drags controller-runtime +
// k8s.io/* and version-lockstep pain for two API calls).
package argoexec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
)

// Client implements [rollback.ArgoClient] against an ArgoCD API
// server. BaseURL is the server root (e.g. https://argocd.example.com);
// Token is a bearer token. HTTP is optional (defaults to a 30s client).
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

var _ rollback.ArgoClient = (*Client)(nil)

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method,
		strings.TrimRight(c.BaseURL, "/")+path, rdr)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("argocd %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

type appResponse struct {
	Status struct {
		Sync struct {
			Revision string `json:"revision"`
		} `json:"sync"`
		History []struct {
			ID       int64  `json:"id"`
			Revision string `json:"revision"`
		} `json:"history"`
	} `json:"status"`
}

// GetApplication implements [rollback.ArgoClient].
func (c *Client) GetApplication(ctx context.Context, name string) (rollback.ArgoApp, error) {
	raw, err := c.do(ctx, http.MethodGet, "/api/v1/applications/"+name, nil)
	if err != nil {
		return rollback.ArgoApp{}, err
	}
	var ar appResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return rollback.ArgoApp{}, fmt.Errorf("decode application: %w", err)
	}
	app := rollback.ArgoApp{Name: name, SyncRevision: ar.Status.Sync.Revision}
	for _, h := range ar.Status.History {
		app.History = append(app.History, rollback.ArgoHistoryEntry{ID: h.ID, Revision: h.Revision})
	}
	return app, nil
}

// SyncToRevision implements [rollback.ArgoClient].
func (c *Client) SyncToRevision(ctx context.Context, name, revision string) error {
	_, err := c.do(ctx, http.MethodPost, "/api/v1/applications/"+name+"/sync",
		map[string]any{"revision": revision, "prune": false})
	return err
}
