package netconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, DefaultPort, cfg.Port)
	assert.NotNil(t, cfg.ConnectionConfig)
}

func TestNewAdapter(t *testing.T) {
	a := NewAdapter(nil)
	require.NotNil(t, a)
	assert.Equal(t, DefaultPort, a.config.Port)
	assert.NotNil(t, a.metrics)
	assert.False(t, a.IsConnected())
}

func TestNewAdapter_WithConfig(t *testing.T) {
	cfg := &Config{Port: 22830}
	a := NewAdapter(cfg)
	assert.Equal(t, 22830, a.config.Port)
	assert.NotNil(t, a.config.ConnectionConfig)
}

func TestAdapterType(t *testing.T) {
	a := NewAdapter(nil)
	assert.Equal(t, protocols.ProtocolNETCONF, a.Type())
}

func TestAdapterIsConnected(t *testing.T) {
	a := NewAdapter(nil)
	assert.False(t, a.IsConnected())
}

func TestAdapterMetrics(t *testing.T) {
	a := NewAdapter(nil)
	m := a.Metrics()
	require.NotNil(t, m)
	assert.Equal(t, int64(0), m.ConnectionCount)
	assert.Equal(t, int64(0), m.ExecutionCount)
}

func TestAdapterSessionID_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	assert.Equal(t, uint32(0), a.SessionID())
}

func TestAdapterServerCapabilities_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	assert.Nil(t, a.ServerCapabilities())
}

func TestNewAdapterFactory(t *testing.T) {
	factory := NewAdapterFactory(nil)
	require.NotNil(t, factory)

	adapter, err := factory(protocols.DefaultConnectionConfig())
	require.NoError(t, err)
	require.NotNil(t, adapter)
	assert.Equal(t, protocols.ProtocolNETCONF, adapter.Type())
}

func TestNewNetconfAdapterFactory(t *testing.T) {
	factory := NewNetconfAdapterFactory(nil)
	require.NotNil(t, factory)

	adapter, err := factory(protocols.DefaultConnectionConfig())
	require.NoError(t, err)
	require.NotNil(t, adapter)
	assert.Equal(t, protocols.ProtocolNETCONF, adapter.Type())
}

func TestAdapterRegistered(t *testing.T) {
	assert.True(t, protocols.DefaultRegistry.Has(protocols.ProtocolNETCONF))
	assert.True(t, protocols.DefaultRegistry.HasNetconf(protocols.ProtocolNETCONF))
}

func TestAdapterCreate(t *testing.T) {
	adapter, err := protocols.DefaultRegistry.Create(protocols.ProtocolNETCONF, nil)
	require.NoError(t, err)
	assert.Equal(t, protocols.ProtocolNETCONF, adapter.Type())
}

func TestNetconfAdapterCreate(t *testing.T) {
	adapter, err := protocols.DefaultRegistry.CreateNetconf(protocols.ProtocolNETCONF, nil)
	require.NoError(t, err)
	assert.Equal(t, protocols.ProtocolNETCONF, adapter.Type())
}

func TestInterfaceCompliance(t *testing.T) {
	var _ protocols.ProtocolAdapter = (*Adapter)(nil)
	var _ protocols.NetconfAdapter = (*Adapter)(nil)
}
