// SPDX-License-Identifier: Apache-2.0

package maintenance_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/api/maintenance"
)

func TestHandler_RoutesReturn410(t *testing.T) {
	mux := http.NewServeMux()
	maintenance.NewHandler().Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wantBody := "Not part of v0.1; maintenance windows ship post-v1.0."

	cases := []struct {
		method, path string
	}{
		{"GET", "/api/v1/maintenance/windows"},
		{"POST", "/api/v1/maintenance/windows"},
		{"GET", "/api/v1/maintenance/windows/w-1"},
		{"DELETE", "/api/v1/maintenance/windows/w-1"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusGone {
				t.Errorf("status = %d, want 410", resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), wantBody) {
				t.Errorf("body = %q, want substring %q", string(body), wantBody)
			}
		})
	}
}
