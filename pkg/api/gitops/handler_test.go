package gitops_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/api/gitops"
)

func TestHandler_RoutesReturn501(t *testing.T) {
	mux := http.NewServeMux()
	gitops.NewHandler().Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cases := []struct {
		method, path string
	}{
		{"GET", "/api/v1/gitops/repos"},
		{"POST", "/api/v1/gitops/repos"},
		{"DELETE", "/api/v1/gitops/repos/r-1"},
		{"POST", "/api/v1/gitops/sync"},
		{"POST", "/api/v1/gitops/webhook"},
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
			if resp.StatusCode != http.StatusNotImplemented {
				t.Errorf("status = %d, want 501", resp.StatusCode)
			}
		})
	}
}
