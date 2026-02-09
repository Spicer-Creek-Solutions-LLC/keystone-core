package netconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatastore_Valid(t *testing.T) {
	tests := []struct {
		ds    Datastore
		valid bool
	}{
		{Running, true},
		{Candidate, true},
		{Startup, true},
		{"invalid", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.ds), func(t *testing.T) {
			assert.Equal(t, tc.valid, tc.ds.Valid())
		})
	}
}

func TestRPCError_Error(t *testing.T) {
	e := &RPCError{
		Type:     ErrorTypeApplication,
		Tag:      "invalid-value",
		Severity: SeverityError,
		Message:  "Bad value",
		Path:     "/interfaces/interface",
	}

	s := e.Error()
	assert.Contains(t, s, "invalid-value")
	assert.Contains(t, s, "Bad value")
	assert.Contains(t, s, "/interfaces/interface")
}

func TestRPCError_ErrorNoMessage(t *testing.T) {
	e := &RPCError{
		Type:     ErrorTypeProtocol,
		Tag:      "lock-denied",
		Severity: SeverityError,
	}

	s := e.Error()
	assert.Contains(t, s, "lock-denied")
	assert.NotContains(t, s, "path:")
}

func TestRPCErrors_Single(t *testing.T) {
	errs := RPCErrors{{
		Tag:      "data-missing",
		Severity: SeverityError,
	}}
	s := errs.Error()
	assert.Contains(t, s, "data-missing")
	assert.NotContains(t, s, "netconf rpc-errors:")
}

func TestRPCErrors_Multiple(t *testing.T) {
	errs := RPCErrors{
		{Tag: "error-one", Severity: SeverityError},
		{Tag: "error-two", Severity: SeverityWarning},
	}
	s := errs.Error()
	assert.Contains(t, s, "2 netconf rpc-errors")
	assert.Contains(t, s, "error-one")
	assert.Contains(t, s, "error-two")
}

func TestRPCErrors_HasError(t *testing.T) {
	onlyWarnings := RPCErrors{{Severity: SeverityWarning}}
	assert.False(t, onlyWarnings.HasError())

	withError := RPCErrors{{Severity: SeverityError}}
	assert.True(t, withError.HasError())

	mixed := RPCErrors{
		{Severity: SeverityWarning},
		{Severity: SeverityError},
	}
	assert.True(t, mixed.HasError())

	empty := RPCErrors{}
	assert.False(t, empty.HasError())
}

func TestClientCapabilities(t *testing.T) {
	caps := ClientCapabilities()
	assert.Contains(t, caps, BaseCapability10)
	assert.Contains(t, caps, BaseCapability11)
	assert.Len(t, caps, 2)
}
