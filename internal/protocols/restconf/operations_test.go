package restconf

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

func TestGetData(t *testing.T) {
	expected, err := os.ReadFile("testdata/get_interfaces.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/restconf/data/ietf-interfaces:interfaces", r.URL.Path)
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write(expected)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	data, err := a.GetData(context.Background(), "ietf-interfaces:interfaces", nil)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(data))
}

func TestGetData_WithQueryOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "3", r.URL.Query().Get("depth"))
		assert.Equal(t, "name;enabled", r.URL.Query().Get("fields"))
		assert.Equal(t, "config", r.URL.Query().Get("content"))
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	opts := &protocols.RestconfQueryOptions{
		Depth:   3,
		Fields:  "name;enabled",
		Content: "config",
	}
	_, err := a.GetData(context.Background(), "ietf-interfaces:interfaces", opts)
	require.NoError(t, err)
}

func TestGetData_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.GetData(context.Background(), "/test", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestPostData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/restconf/data/ietf-interfaces:interfaces", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "eth1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	err := a.PostData(context.Background(), "ietf-interfaces:interfaces", []byte(`{"interface":{"name":"eth1"}}`))
	require.NoError(t, err)
}

func TestPostData_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.PostData(context.Background(), "/test", []byte("{}"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestPutData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	err := a.PutData(context.Background(), "ietf-interfaces:interfaces/interface=eth0", []byte(`{"enabled":false}`))
	require.NoError(t, err)
}

func TestPutData_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.PutData(context.Background(), "/test", []byte("{}"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestPatchData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	err := a.PatchData(context.Background(), "ietf-interfaces:interfaces/interface=eth0", []byte(`{"enabled":true}`))
	require.NoError(t, err)
}

func TestPatchData_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.PatchData(context.Background(), "/test", []byte("{}"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestDeleteData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/restconf/data/ietf-interfaces:interfaces/interface=eth99", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	err := a.DeleteData(context.Background(), "ietf-interfaces:interfaces/interface=eth99")
	require.NoError(t, err)
}

func TestDeleteData_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	err := a.DeleteData(context.Background(), "/test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestInvokeOperation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/restconf/operations/ietf-system:restart", r.URL.Path)
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(`{"ietf-system:output":{"result":"ok"}}`))
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	data, err := a.InvokeOperation(context.Background(), "ietf-system:restart", nil)
	require.NoError(t, err)
	assert.Contains(t, string(data), "ok")
}

func TestInvokeOperation_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.InvokeOperation(context.Background(), "test", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestDoRequest_ErrorResponse(t *testing.T) {
	errJSON, err := os.ReadFile("testdata/error_response.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(errJSON)
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)
	_, err = a.GetData(context.Background(), "test", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid-value")
}

func TestExecuteCommand_Parse(t *testing.T) {
	tests := []struct {
		name    string
		command string
		wantErr string
	}{
		{name: "empty command", command: "", wantErr: "empty command"},
		{name: "unknown op", command: "foobar", wantErr: "unknown RESTCONF operation"},
		{name: "get-data missing path", command: "get-data", wantErr: "get-data requires path"},
		{name: "post-data missing args", command: "post-data /path", wantErr: "post-data requires path and body"},
		{name: "put-data missing args", command: "put-data /path", wantErr: "put-data requires path and body"},
		{name: "patch-data missing args", command: "patch-data /path", wantErr: "patch-data requires path and body"},
		{name: "delete-data missing path", command: "delete-data", wantErr: "delete-data requires path"},
		{name: "invoke missing op", command: "invoke", wantErr: "invoke requires operation"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newConnectedAdapter("http://localhost:1")
			_, err := a.executeCommand(context.Background(), tc.command)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestExecuteCommand_ValidCommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	tests := []struct {
		name    string
		command string
	}{
		{name: "get-data", command: "get-data /ietf-interfaces:interfaces"},
		{name: "post-data", command: "post-data /interfaces {\"name\":\"eth1\"}"},
		{name: "put-data", command: "put-data /interfaces/interface=eth0 {\"enabled\":true}"},
		{name: "patch-data", command: "patch-data /interfaces/interface=eth0 {\"enabled\":false}"},
		{name: "delete-data", command: "delete-data /interfaces/interface=eth99"},
		{name: "invoke", command: "invoke ietf-system:restart"},
		{name: "invoke with input", command: "invoke ietf-system:set-datetime {\"datetime\":\"2024-01-01\"}"},
		{name: "yang-library", command: "yang-library"},
		{name: "modules", command: "modules"},
		{name: "raw GET", command: "get /restconf/data/test"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newConnectedAdapter(srv.URL)
			_, err := a.executeCommand(context.Background(), tc.command)
			require.NoError(t, err)
		})
	}
}

func TestBuildDataURL_NoOpts(t *testing.T) {
	a := NewAdapter(nil)
	a.rootPath = "/restconf"
	url := a.buildDataURL("/ietf-interfaces:interfaces", nil)
	assert.Equal(t, "/restconf/data/ietf-interfaces:interfaces", url)
}

func TestBuildDataURL_WithOpts(t *testing.T) {
	a := NewAdapter(nil)
	a.rootPath = "/restconf"
	opts := &protocols.RestconfQueryOptions{
		Depth:   5,
		Fields:  "name",
		Content: "config",
	}
	url := a.buildDataURL("/interfaces", opts)
	assert.Contains(t, url, "depth=5")
	assert.Contains(t, url, "fields=name")
	assert.Contains(t, url, "content=config")
}

func TestBuildDataURL_LeadingSlash(t *testing.T) {
	a := NewAdapter(nil)
	a.rootPath = "/restconf"
	url := a.buildDataURL("ietf-interfaces:interfaces", nil)
	assert.Equal(t, "/restconf/data/ietf-interfaces:interfaces", url)
}

func TestExecute_NotConnected(t *testing.T) {
	a := NewAdapter(nil)
	_, err := a.Execute(context.Background(), &protocols.ExecuteRequest{Command: "get-data /test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestExecute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yang-data+json")
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)

	// Use the public Execute via a connected adapter stub
	a.mu.Lock()
	a.connected = true
	a.mu.Unlock()

	result, err := a.Execute(context.Background(), &protocols.ExecuteRequest{Command: "get-data /test"})
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, string(result.Stdout), "ok")
}

func TestDoRequest_MediaTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/yang-data+json", r.Header.Get("Accept"))
		if r.Method == http.MethodPost {
			assert.Equal(t, "application/yang-data+json", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	a := newConnectedAdapter(srv.URL)

	// GET should set Accept
	_, _, _, err := a.doRequest(context.Background(), "GET", "/test", nil, string(ContentTypeYANGJSON))
	require.NoError(t, err)

	// POST should set both Accept and Content-Type
	_, _, _, err = a.doRequest(context.Background(), "POST", "/test", []byte("{}"), string(ContentTypeYANGJSON))
	require.NoError(t, err)
}

