package restconf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

func TestNewAdapter_Defaults(t *testing.T) {
	a := NewAdapter(nil)
	assert.NotNil(t, a)
	assert.Equal(t, DefaultPort, a.config.Port)
	assert.Equal(t, ContentTypeYANGJSON, a.config.Encoding)
	assert.True(t, a.config.ValidateSSL)
	assert.True(t, a.config.DiscoverRoot)
	assert.NotNil(t, a.metrics)
}

func TestNewAdapter_CustomConfig(t *testing.T) {
	cfg := &Config{
		Port:         8181,
		RootPath:     "/rests",
		Encoding:     ContentTypeYANGXML,
		ValidateSSL:  false,
		DiscoverRoot: false,
	}
	a := NewAdapter(cfg)
	assert.Equal(t, 8181, a.config.Port)
	assert.Equal(t, "/rests", a.config.RootPath)
	assert.Equal(t, ContentTypeYANGXML, a.config.Encoding)
}

func TestAdapter_Type(t *testing.T) {
	a := NewAdapter(nil)
	assert.Equal(t, protocols.ProtocolRESTCONF, a.Type())
}

func TestAdapter_IsConnected(t *testing.T) {
	a := NewAdapter(nil)
	assert.False(t, a.IsConnected())

	a.mu.Lock()
	a.connected = true
	a.mu.Unlock()
	assert.True(t, a.IsConnected())
}

func TestAdapter_Metrics(t *testing.T) {
	a := NewAdapter(nil)
	m := a.Metrics()
	assert.NotNil(t, m)
	assert.Equal(t, int64(0), m.ConnectionCount)
}

func TestAdapter_Connect_NilCredential(t *testing.T) {
	// Server that responds to well-known discovery
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/host-meta" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := NewAdapter(&Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             0,
		ValidateSSL:      false,
		DiscoverRoot:     false,
	})

	device := &proxy.ProxiedDevice{
		Address: "127.0.0.1",
		Port:    0,
	}

	err := a.Connect(context.Background(), device, nil)
	require.NoError(t, err)
	assert.True(t, a.IsConnected())
	assert.Equal(t, DefaultRootPath, a.rootPath)
	assert.Equal(t, int64(1), a.metrics.ConnectionCount)
}

func TestAdapter_Connect_AlreadyConnected(t *testing.T) {
	a := NewAdapter(nil)
	a.mu.Lock()
	a.connected = true
	a.mu.Unlock()

	err := a.Connect(context.Background(), &proxy.ProxiedDevice{}, nil)
	assert.NoError(t, err)
}

func TestAdapter_Connect_UnsupportedCredential(t *testing.T) {
	a := NewAdapter(&Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		ValidateSSL:      false,
		DiscoverRoot:     false,
	})

	device := &proxy.ProxiedDevice{
		Address: "127.0.0.1",
	}

	// Use a credential type that setupAuth doesn't support
	cred := &credentials.SSHPasswordCredential{
		Username: "admin",
		Password: "secret",
	}

	err := a.Connect(context.Background(), device, cred)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported credential type")
}

func TestAdapter_Connect_ConfiguredRootPath(t *testing.T) {
	a := NewAdapter(&Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		RootPath:         "/rests",
		ValidateSSL:      false,
		DiscoverRoot:     false,
	})

	device := &proxy.ProxiedDevice{
		Address: "127.0.0.1",
	}

	err := a.Connect(context.Background(), device, nil)
	require.NoError(t, err)
	assert.Equal(t, "/rests", a.rootPath)
}

func TestAdapter_Connect_DevicePortOverride(t *testing.T) {
	a := NewAdapter(&Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             443,
		ValidateSSL:      false,
		DiscoverRoot:     false,
	})

	device := &proxy.ProxiedDevice{
		Address: "10.0.0.1",
		Port:    8443,
	}

	err := a.Connect(context.Background(), device, nil)
	require.NoError(t, err)
	// The port from device should be used (8443 not 443)
	assert.True(t, a.IsConnected())
}

func TestAdapter_Disconnect(t *testing.T) {
	a := newConnectedAdapter("http://localhost")
	assert.True(t, a.IsConnected())

	err := a.Disconnect(context.Background())
	require.NoError(t, err)
	assert.False(t, a.IsConnected())
	assert.Nil(t, a.client)
	assert.Nil(t, a.auth)
	assert.Equal(t, "", a.rootPath)
}

func TestAdapter_HealthCheck_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	result, err := a.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
	assert.Equal(t, "not connected", result.Status)
}

func TestAdapter_HealthCheck_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(`{"ietf-restconf:yang-library-version":"2016-06-21"}`))
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	result, err := a.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Healthy)
	assert.Equal(t, "healthy", result.Status)
	assert.True(t, result.Latency > 0)
}

func TestAdapter_HealthCheck_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	result, err := a.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.False(t, result.Healthy)
	assert.Equal(t, "unhealthy", result.Status)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, DefaultPort, cfg.Port)
	assert.Equal(t, ContentTypeYANGJSON, cfg.Encoding)
	assert.True(t, cfg.ValidateSSL)
	assert.True(t, cfg.DiscoverRoot)
	assert.NotNil(t, cfg.ConnectionConfig)
}

func TestSetupAuth_BasicCredential(t *testing.T) {
	cred := &credentials.RESTBasicCredential{
		Username: "admin",
		Password: "password",
	}
	auth, err := setupAuth(cred)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestSetupAuth_BearerCredential(t *testing.T) {
	cred := &credentials.RESTBearerCredential{
		Token: "my-token",
	}
	auth, err := setupAuth(cred)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestSetupAuth_APIKeyCredential_Header(t *testing.T) {
	cred := &credentials.RESTAPIKeyCredential{
		APIKey:     "key123",
		HeaderName: "X-Custom-Key",
	}
	auth, err := setupAuth(cred)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestSetupAuth_APIKeyCredential_Query(t *testing.T) {
	cred := &credentials.RESTAPIKeyCredential{
		APIKey:     "key123",
		QueryParam: "api_key",
	}
	auth, err := setupAuth(cred)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestSetupAuth_APIKeyCredential_DefaultHeader(t *testing.T) {
	cred := &credentials.RESTAPIKeyCredential{
		APIKey: "key123",
	}
	auth, err := setupAuth(cred)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestSetupAuth_OAuth2Credential(t *testing.T) {
	cred := &credentials.RESTOAuth2Credential{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		TokenURL:     "https://auth.example.com/token",
		Scopes:       []string{"read", "write"},
	}
	auth, err := setupAuth(cred)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestSetupAuth_NilCredential(t *testing.T) {
	auth, err := setupAuth(nil)
	require.NoError(t, err)
	assert.NotNil(t, auth)
}

func TestSetupAuth_UnsupportedCredential(t *testing.T) {
	cred := &credentials.SSHPasswordCredential{
		Username: "user",
		Password: "pass",
	}
	_, err := setupAuth(cred)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported credential type")
}

func TestNewAdapterFactory(t *testing.T) {
	factory := NewAdapterFactory(nil)
	adapter, err := factory(nil)
	require.NoError(t, err)
	assert.NotNil(t, adapter)
	assert.Equal(t, protocols.ProtocolRESTCONF, adapter.Type())
}

func TestNewAdapterFactory_WithConfig(t *testing.T) {
	cfg := &Config{
		Port:     8181,
		Encoding: ContentTypeYANGXML,
	}
	factory := NewAdapterFactory(cfg)
	adapter, err := factory(nil)
	require.NoError(t, err)

	rcAdapter, ok := adapter.(*Adapter)
	require.True(t, ok)
	assert.Equal(t, 8181, rcAdapter.config.Port)
}

func TestNewRestconfAdapterFactory(t *testing.T) {
	factory := NewRestconfAdapterFactory(nil)
	adapter, err := factory(nil)
	require.NoError(t, err)
	assert.NotNil(t, adapter)
	assert.Equal(t, protocols.ProtocolRESTCONF, adapter.Type())
}

func TestRegistration(t *testing.T) {
	// Verify the init() function registered both factories
	assert.True(t, protocols.DefaultRegistry.Has(protocols.ProtocolRESTCONF))
	assert.True(t, protocols.DefaultRegistry.HasRestconf(protocols.ProtocolRESTCONF))
}

func TestCreateAdapter_ViaRegistry(t *testing.T) {
	adapter, err := protocols.DefaultRegistry.Create(protocols.ProtocolRESTCONF, nil)
	require.NoError(t, err)
	assert.NotNil(t, adapter)
	assert.Equal(t, protocols.ProtocolRESTCONF, adapter.Type())
}

func TestCreateRestconfAdapter_ViaRegistry(t *testing.T) {
	adapter, err := protocols.DefaultRegistry.CreateRestconf(protocols.ProtocolRESTCONF, nil)
	require.NoError(t, err)
	assert.NotNil(t, adapter)

	assert.NotNil(t, adapter)
}

// Interface compliance
var (
	_ protocols.ProtocolAdapter = (*Adapter)(nil)
	_ protocols.RestconfAdapter = (*Adapter)(nil)
)
