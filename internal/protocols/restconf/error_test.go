package restconf

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError_Error(t *testing.T) {
	e := &Error{
		ErrorType:    "protocol",
		ErrorTag:     "invalid-value",
		ErrorMessage: "Bad input",
		ErrorPath:    "/interfaces/interface=eth0",
	}

	s := e.Error()
	assert.Contains(t, s, "protocol")
	assert.Contains(t, s, "invalid-value")
	assert.Contains(t, s, "Bad input")
	assert.Contains(t, s, "/interfaces/interface=eth0")
}

func TestError_ErrorNoMessage(t *testing.T) {
	e := &Error{
		ErrorType: "application",
		ErrorTag:  "data-missing",
	}

	s := e.Error()
	assert.Contains(t, s, "application")
	assert.Contains(t, s, "data-missing")
	assert.NotContains(t, s, "path:")
}

func TestErrors_Single(t *testing.T) {
	errs := &Errors{Errors: []Error{{
		ErrorType: "protocol",
		ErrorTag:  "invalid-value",
	}}}
	s := errs.Error()
	assert.Contains(t, s, "invalid-value")
	assert.NotContains(t, s, "restconf errors:")
}

func TestErrors_Multiple(t *testing.T) {
	errs := &Errors{Errors: []Error{
		{ErrorType: "protocol", ErrorTag: "error-one"},
		{ErrorType: "application", ErrorTag: "error-two"},
	}}
	s := errs.Error()
	assert.Contains(t, s, "2 restconf errors")
	assert.Contains(t, s, "error-one")
	assert.Contains(t, s, "error-two")
}

func TestErrors_HasError(t *testing.T) {
	assert.True(t, (&Errors{Errors: []Error{{ErrorTag: "test"}}}).HasError())
	assert.False(t, (&Errors{}).HasError())
}

func TestParseErrorResponse_JSON(t *testing.T) {
	data, err := os.ReadFile("testdata/error_response.json")
	require.NoError(t, err)

	errs, err := ParseErrorResponse(data, "application/yang-data+json")
	require.NoError(t, err)
	require.Len(t, errs.Errors, 1)
	assert.Equal(t, "protocol", errs.Errors[0].ErrorType)
	assert.Equal(t, "invalid-value", errs.Errors[0].ErrorTag)
	assert.Equal(t, "Invalid input data", errs.Errors[0].ErrorMessage)
	assert.Contains(t, errs.Errors[0].ErrorPath, "eth99")
}

func TestParseErrorResponse_XML(t *testing.T) {
	data, err := os.ReadFile("testdata/error_response.xml")
	require.NoError(t, err)

	errs, err := ParseErrorResponse(data, "application/yang-data+xml")
	require.NoError(t, err)
	require.Len(t, errs.Errors, 1)
	assert.Equal(t, "application", errs.Errors[0].ErrorType)
	assert.Equal(t, "data-missing", errs.Errors[0].ErrorTag)
	assert.Contains(t, errs.Errors[0].ErrorMessage, "data model content does not exist")
}

func TestParseErrorResponse_Empty(t *testing.T) {
	_, err := ParseErrorResponse(nil, "application/json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseErrorResponse_InvalidJSON(t *testing.T) {
	_, err := ParseErrorResponse([]byte("not json"), "application/json")
	assert.Error(t, err)
}

func TestIsErrorResponse(t *testing.T) {
	assert.True(t, IsErrorResponse(400))
	assert.True(t, IsErrorResponse(404))
	assert.True(t, IsErrorResponse(500))
	assert.False(t, IsErrorResponse(200))
	assert.False(t, IsErrorResponse(201))
	assert.False(t, IsErrorResponse(204))
}
