package netconf

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeRPC_GetConfig(t *testing.T) {
	op := &rpcGetConfig{
		Source: datastoreElement(Running),
	}
	data, err := encodeRPC("1", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), `message-id="1"`)
	assert.Contains(t, string(data), "<get-config>")
	assert.Contains(t, string(data), "<running></running>")
}

func TestEncodeRPC_GetConfigWithFilter(t *testing.T) {
	op := &rpcGetConfig{
		Source: datastoreElement(Running),
		Filter: &rpcFilter{
			Type:    "subtree",
			Content: "<interfaces/>",
		},
	}
	data, err := encodeRPC("2", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), `type="subtree"`)
	assert.Contains(t, string(data), "<interfaces/>")
}

func TestEncodeRPC_EditConfig(t *testing.T) {
	op := &rpcEditConfig{
		Target:           datastoreElement(Candidate),
		DefaultOperation: "merge",
		Config:           rawXML{Content: "<interface><name>eth0</name></interface>"},
	}
	data, err := encodeRPC("3", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<edit-config>")
	assert.Contains(t, string(data), "<candidate></candidate>")
	assert.Contains(t, string(data), "<name>eth0</name>")
}

func TestEncodeRPC_Lock(t *testing.T) {
	op := &rpcLock{Target: datastoreElement(Running)}
	data, err := encodeRPC("4", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<lock>")
	assert.Contains(t, string(data), "<running></running>")
}

func TestEncodeRPC_Unlock(t *testing.T) {
	op := &rpcUnlock{Target: datastoreElement(Running)}
	data, err := encodeRPC("5", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<unlock>")
}

func TestEncodeRPC_Commit(t *testing.T) {
	data, err := encodeRPC("6", &rpcCommit{})
	require.NoError(t, err)
	assert.Contains(t, string(data), "<commit></commit>")
}

func TestEncodeRPC_DiscardChanges(t *testing.T) {
	data, err := encodeRPC("7", &rpcDiscardChanges{})
	require.NoError(t, err)
	assert.Contains(t, string(data), "<discard-changes></discard-changes>")
}

func TestEncodeRPC_Validate(t *testing.T) {
	op := &rpcValidate{Source: datastoreElement(Candidate)}
	data, err := encodeRPC("8", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<validate>")
	assert.Contains(t, string(data), "<candidate></candidate>")
}

func TestEncodeRPC_CopyConfig(t *testing.T) {
	op := &rpcCopyConfig{
		Source: datastoreElement(Running),
		Target: datastoreElement(Startup),
	}
	data, err := encodeRPC("9", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<copy-config>")
}

func TestEncodeRPC_DeleteConfig(t *testing.T) {
	op := &rpcDeleteConfig{Target: datastoreElement(Startup)}
	data, err := encodeRPC("10", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<delete-config>")
	assert.Contains(t, string(data), "<startup></startup>")
}

func TestEncodeRPC_CloseSession(t *testing.T) {
	data, err := encodeRPC("11", &rpcCloseSession{})
	require.NoError(t, err)
	assert.Contains(t, string(data), "<close-session></close-session>")
}

func TestEncodeRPC_KillSession(t *testing.T) {
	op := &rpcKillSession{SessionID: 42}
	data, err := encodeRPC("12", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<kill-session>")
	assert.Contains(t, string(data), "<session-id>42</session-id>")
}

func TestEncodeRPC_Get(t *testing.T) {
	op := &rpcGet{}
	data, err := encodeRPC("13", op)
	require.NoError(t, err)
	assert.Contains(t, string(data), "<get>")
}

func TestDecodeReply_OK(t *testing.T) {
	data, err := os.ReadFile("testdata/reply_ok.xml")
	require.NoError(t, err)

	reply, err := decodeReply(data)
	require.NoError(t, err)
	assert.True(t, reply.OK)
	assert.Equal(t, "1", reply.MessageID)
	assert.Empty(t, reply.Errors)
	assert.Nil(t, reply.Data)
}

func TestDecodeReply_Data(t *testing.T) {
	data, err := os.ReadFile("testdata/reply_config.xml")
	require.NoError(t, err)

	reply, err := decodeReply(data)
	require.NoError(t, err)
	assert.Equal(t, "2", reply.MessageID)
	assert.NotNil(t, reply.Data)
	assert.Contains(t, string(reply.Data), "eth0")
	assert.Contains(t, string(reply.Data), "interfaces")
}

func TestDecodeReply_Error(t *testing.T) {
	data, err := os.ReadFile("testdata/reply_error.xml")
	require.NoError(t, err)

	reply, err := decodeReply(data)
	require.NoError(t, err)
	assert.Equal(t, "3", reply.MessageID)
	assert.False(t, reply.OK)
	require.Len(t, reply.Errors, 1)
	assert.Equal(t, ErrorTypeApplication, reply.Errors[0].Type)
	assert.Equal(t, "invalid-value", reply.Errors[0].Tag)
	assert.Equal(t, SeverityError, reply.Errors[0].Severity)
	assert.Equal(t, "Invalid interface name", reply.Errors[0].Message)
	assert.True(t, reply.Errors.HasError())
}

func TestDecodeReply_MultiError(t *testing.T) {
	data := []byte(`<rpc-reply xmlns="urn:ietf:params:xml:ns:netconf:base:1.0" message-id="4">
  <rpc-error>
    <error-type>application</error-type>
    <error-tag>invalid-value</error-tag>
    <error-severity>error</error-severity>
    <error-message>Error one</error-message>
  </rpc-error>
  <rpc-error>
    <error-type>application</error-type>
    <error-tag>data-missing</error-tag>
    <error-severity>warning</error-severity>
    <error-message>Warning one</error-message>
  </rpc-error>
</rpc-reply>`)

	reply, err := decodeReply(data)
	require.NoError(t, err)
	require.Len(t, reply.Errors, 2)
	assert.True(t, reply.Errors.HasError())
	assert.Contains(t, reply.Errors.Error(), "2 netconf rpc-errors")
}

func TestHello_Marshal(t *testing.T) {
	h := newHello(ClientCapabilities())
	data, err := encodeHello(h)
	require.NoError(t, err)

	s := string(data)
	assert.True(t, strings.HasPrefix(s, xml.Header))
	assert.Contains(t, s, string(BaseCapability10))
	assert.Contains(t, s, string(BaseCapability11))
	assert.NotContains(t, s, "session-id")
}

func TestHello_Unmarshal_Junos(t *testing.T) {
	data, err := os.ReadFile("testdata/hello_junos.xml")
	require.NoError(t, err)

	h, err := parseHello(data)
	require.NoError(t, err)
	assert.Equal(t, uint32(12345), h.SessionID)
	assert.Contains(t, h.Capabilities, string(BaseCapability10))
	assert.Contains(t, h.Capabilities, string(BaseCapability11))
	assert.Contains(t, h.Capabilities, string(CandidateCapability))
}

func TestHello_Unmarshal_IOSXE(t *testing.T) {
	data, err := os.ReadFile("testdata/hello_iosxe.xml")
	require.NoError(t, err)

	h, err := parseHello(data)
	require.NoError(t, err)
	assert.Equal(t, uint32(67890), h.SessionID)
	assert.Contains(t, h.Capabilities, string(WritableRunning))
}

func TestHello_Unmarshal_SROS(t *testing.T) {
	data, err := os.ReadFile("testdata/hello_sros.xml")
	require.NoError(t, err)

	h, err := parseHello(data)
	require.NoError(t, err)
	assert.Equal(t, uint32(99001), h.SessionID)
}

func TestParseHello_NoCaps(t *testing.T) {
	data := []byte(`<hello xmlns="urn:ietf:params:xml:ns:netconf:base:1.0"><capabilities></capabilities></hello>`)
	_, err := parseHello(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no capabilities")
}

func TestParseHello_InvalidXML(t *testing.T) {
	_, err := parseHello([]byte("not xml"))
	assert.Error(t, err)
}

func TestDatastoreElement(t *testing.T) {
	tests := []struct {
		ds       Datastore
		running  bool
		cand     bool
		startup  bool
	}{
		{Running, true, false, false},
		{Candidate, false, true, false},
		{Startup, false, false, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.ds), func(t *testing.T) {
			elem := datastoreElement(tc.ds)
			assert.Equal(t, tc.running, elem.Running != nil)
			assert.Equal(t, tc.cand, elem.Candidate != nil)
			assert.Equal(t, tc.startup, elem.Startup != nil)
		})
	}
}

func TestFilterElement_Nil(t *testing.T) {
	assert.Nil(t, filterElement(nil))
}

func TestFilterElement_Subtree(t *testing.T) {
	f := filterElement(&Filter{Type: "subtree", Content: "<test/>"})
	require.NotNil(t, f)
	assert.Equal(t, "subtree", f.Type)
	assert.Equal(t, "<test/>", f.Content)
}
