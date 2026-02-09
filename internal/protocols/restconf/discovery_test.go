package restconf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawnbutts/keystone-core/internal/protocols/rest"
)

func TestParseHostMeta(t *testing.T) {
	data, err := os.ReadFile("testdata/host_meta.xml")
	require.NoError(t, err)

	path := parseHostMeta(data)
	assert.Equal(t, "/rests", path)
}

func TestParseHostMeta_NoLink(t *testing.T) {
	data := []byte(`<XRD xmlns="http://docs.oasis-open.org/ns/xri/xrd-1.0"><Link rel="other" href="/foo"/></XRD>`)
	assert.Equal(t, "", parseHostMeta(data))
}

func TestParseHostMeta_InvalidXML(t *testing.T) {
	assert.Equal(t, "", parseHostMeta([]byte("not xml")))
}

func TestDiscoverRootPath_WellKnown(t *testing.T) {
	hostMeta, err := os.ReadFile("testdata/host_meta.xml")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/host-meta" {
			w.Header().Set("Content-Type", "application/xrd+xml")
			w.Write(hostMeta)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := rest.NewClient(&rest.ClientConfig{BaseURL: srv.URL})
	path := discoverRootPath(context.Background(), client, nil)
	assert.Equal(t, "/rests", path)
}

func TestDiscoverRootPath_Fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := rest.NewClient(&rest.ClientConfig{BaseURL: srv.URL})
	path := discoverRootPath(context.Background(), client, nil)
	assert.Equal(t, DefaultRootPath, path)
}

func TestYANGLibraryVersion(t *testing.T) {
	yangLib, err := os.ReadFile("testdata/yang_library.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write(yangLib)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	version, err := a.YANGLibraryVersion(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "2016-06-21", version)
}

func TestYANGLibraryVersion_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.YANGLibraryVersion(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestServerModules(t *testing.T) {
	modulesData, err := os.ReadFile("testdata/modules_state.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write(modulesData)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	modules, err := a.ServerModules(context.Background())
	require.NoError(t, err)
	require.Len(t, modules, 3)
	assert.Equal(t, "ietf-interfaces", modules[0].Name)
	assert.Equal(t, "2018-02-20", modules[0].Revision)
	assert.Equal(t, "ietf-ip", modules[1].Name)
	assert.Equal(t, "ietf-system", modules[2].Name)
}

func TestServerModules_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.ServerModules(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestRootPath(t *testing.T) {
	a := NewAdapter(nil)
	a.mu.Lock()
	a.rootPath = "/custom"
	a.mu.Unlock()
	assert.Equal(t, "/custom", a.RootPath())
}

// newConnectedAdapter creates an adapter pre-connected to a test server.
func newConnectedAdapter(baseURL string) *Adapter {
	a := NewAdapter(nil)
	a.mu.Lock()
	a.connected = true
	a.rootPath = "/restconf"
	a.client = rest.NewClient(&rest.ClientConfig{BaseURL: baseURL})
	a.auth = &rest.NoAuth{}
	a.mu.Unlock()
	return a
}
