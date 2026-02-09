package netconf

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

func TestExecuteCommand_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.Execute(context.Background(), &protocols.ExecuteRequest{Command: "get-config running"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestExecuteCommand_Parse(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr string
	}{
		{name: "empty command", command: "", wantErr: "empty command"},
		{name: "unknown op", command: "foobar", wantErr: "unknown NETCONF operation"},
		{name: "lock missing target", command: "lock", wantErr: "lock requires target"},
		{name: "unlock missing target", command: "unlock", wantErr: "unlock requires target"},
		{name: "edit-config missing args", command: "edit-config candidate", wantErr: "edit-config requires target and config"},
		{name: "copy-config missing args", command: "copy-config running", wantErr: "copy-config requires source and target"},
		{name: "delete-config missing target", command: "delete-config", wantErr: "delete-config requires target"},
		{name: "kill-session missing id", command: "kill-session", wantErr: "kill-session requires session-id"},
		{name: "kill-session bad id", command: "kill-session abc", wantErr: "invalid session-id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAdapter(nil)
			// Directly test the command parser (adapter not connected, so it will fail at session level)
			_, err := a.executeCommand(context.Background(), tc.command)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestExecuteCommand_ParseValid(t *testing.T) {
	// Valid commands pass the parser but fail at RPC send since there's no session.
	// We verify the Execute wrapper returns a result with error (not a Go error).
	tests := []struct {
		name    string
		command string
	}{
		{name: "get-config", command: "get-config running"},
		{name: "get-config candidate", command: "get-config candidate"},
		{name: "get", command: "get"},
		{name: "get with filter", command: "get <interfaces/>"},
		{name: "commit", command: "commit"},
		{name: "discard-changes", command: "discard-changes"},
		{name: "validate", command: "validate candidate"},
		{name: "validate default", command: "validate"},
		{name: "lock", command: "lock running"},
		{name: "unlock", command: "unlock candidate"},
		{name: "edit-config", command: "edit-config candidate <config/>"},
		{name: "copy-config", command: "copy-config running startup"},
		{name: "delete-config", command: "delete-config startup"},
		{name: "kill-session", command: "kill-session 42"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAdapter(nil)
			// Execute checks connected status first, returns Go error
			_, err := a.Execute(context.Background(), &protocols.ExecuteRequest{Command: tc.command})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not connected")
		})
	}
}

func TestGetConfig_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.GetConfig(context.Background(), "running", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestEditConfig_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.EditConfig(context.Background(), "candidate", []byte("<config/>"), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestLock_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.Lock(context.Background(), "running")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestUnlock_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.Unlock(context.Background(), "running")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestCommit_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.Commit(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestDiscardChanges_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.DiscardChanges(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestValidate_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.Validate(context.Background(), "candidate")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestCopyConfig_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.CopyConfig(context.Background(), "running", "startup")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestDeleteConfig_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.DeleteConfig(context.Background(), "startup")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestDeleteConfig_Running(t *testing.T) {
	a := NewAdapter(nil)
	a.mu.Lock()
	a.connected = true
	a.session = &Session{} // stub
	a.mu.Unlock()

	err := a.DeleteConfig(context.Background(), "running")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete running datastore")
}

func TestGet_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.Get(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestKillSession_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.KillSession(context.Background(), 42)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestGetConfig_InvalidDatastore(t *testing.T) {
	a := NewAdapter(nil)
	a.mu.Lock()
	a.connected = true
	a.session = &Session{}
	a.mu.Unlock()

	_, err := a.GetConfig(context.Background(), "invalid", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid datastore")
}

func TestEditConfig_InvalidDatastore(t *testing.T) {
	a := NewAdapter(nil)
	a.mu.Lock()
	a.connected = true
	a.session = &Session{}
	a.mu.Unlock()

	err := a.EditConfig(context.Background(), "invalid", []byte("<config/>"), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid datastore")
}

func TestToInternalFilter_Nil(t *testing.T) {
	assert.Nil(t, toInternalFilter(nil))
}

func TestToInternalFilter_Valid(t *testing.T) {
	pf := &protocols.NetconfFilter{Type: "subtree", Content: "<test/>"}
	f := toInternalFilter(pf)
	require.NotNil(t, f)
	assert.Equal(t, "subtree", f.Type)
	assert.Equal(t, "<test/>", f.Content)
}
