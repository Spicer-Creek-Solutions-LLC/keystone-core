package state_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apistate "go.keystone-core.io/keystone-core/pkg/api/state"
)

func TestHandler_RoutesReturn501(t *testing.T) {
	mux := http.NewServeMux()
	apistate.NewHandler().Register(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cases := []struct {
		method, path string
	}{
		{"POST", "/api/v1/state/apply"},
		{"POST", "/api/v1/state/check"},
		{"POST", "/api/v1/state/drift"},
		{"GET", "/api/v1/state/runs"},
		{"GET", "/api/v1/state/runs/r-1"},
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
