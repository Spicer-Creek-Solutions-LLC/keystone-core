package restconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContentOption_Valid(t *testing.T) {
	tests := []struct {
		opt   ContentOption
		valid bool
	}{
		{ContentAll, true},
		{ContentConfig, true},
		{ContentNonconfig, true},
		{"invalid", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.opt), func(t *testing.T) {
			assert.Equal(t, tc.valid, tc.opt.Valid())
		})
	}
}

func TestWithDefaultsMode_Valid(t *testing.T) {
	tests := []struct {
		mode  WithDefaultsMode
		valid bool
	}{
		{WithDefaultsReportAll, true},
		{WithDefaultsReportAllTagged, true},
		{WithDefaultsTrim, true},
		{WithDefaultsExplicit, true},
		{"invalid", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			assert.Equal(t, tc.valid, tc.mode.Valid())
		})
	}
}

func TestContentType_String(t *testing.T) {
	assert.Equal(t, "application/yang-data+json", ContentTypeYANGJSON.String())
	assert.Equal(t, "application/yang-data+xml", ContentTypeYANGXML.String())
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 443, DefaultPort)
	assert.Equal(t, "/restconf", DefaultRootPath)
}
