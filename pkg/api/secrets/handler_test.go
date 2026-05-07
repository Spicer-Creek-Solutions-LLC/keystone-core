package secrets_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/api/secrets"
)

func TestHandler_RoutesReturn501(t *testing.T) {
	mux := http.NewServeMux()
	secrets.NewHandler().Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cases := []struct {
		method, path string
	}{
		// KV ops with hierarchical paths.
		{"GET", "/api/v1/secrets/production/db/postgres"},
		{"PUT", "/api/v1/secrets/production/db/postgres"},
		{"DELETE", "/api/v1/secrets/production/db/postgres"},

		// Lease ops.
		{"GET", "/api/v1/leases/l-1"},
		{"GET", "/api/v1/leases"},
		{"POST", "/api/v1/leases/l-1/renew"},
		{"POST", "/api/v1/leases/l-1/revoke"},

		// Transit ops.
		{"POST", "/api/v1/transit/encrypt"},
		{"POST", "/api/v1/transit/decrypt"},
		{"POST", "/api/v1/transit/sign"},
		{"POST", "/api/v1/transit/verify"},
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
