package netconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubtreeFilter(t *testing.T) {
	f := SubtreeFilter("<interfaces/>")
	require.NotNil(t, f)
	assert.Equal(t, "subtree", f.Type)
	assert.Equal(t, "<interfaces/>", f.Content)
}

func TestXPathFilter(t *testing.T) {
	f := XPathFilter("/interfaces/interface[name='eth0']")
	require.NotNil(t, f)
	assert.Equal(t, "xpath", f.Type)
	assert.Equal(t, "/interfaces/interface[name='eth0']", f.Content)
}

func TestPathToSubtree(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "single element",
			path:     "interfaces",
			expected: "<interfaces/>",
		},
		{
			name:     "two elements",
			path:     "interfaces/interface",
			expected: "<interfaces><interface/></interfaces>",
		},
		{
			name:     "three elements",
			path:     "system/dns/servers",
			expected: "<system><dns><servers/></dns></system>",
		},
		{
			name:     "leading slash",
			path:     "/interfaces/interface",
			expected: "<interfaces><interface/></interfaces>",
		},
		{
			name:     "trailing slash",
			path:     "interfaces/interface/",
			expected: "<interfaces><interface/></interfaces>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := PathToSubtree(tc.path)
			require.NotNil(t, f)
			assert.Equal(t, "subtree", f.Type)
			assert.Equal(t, tc.expected, f.Content)
		})
	}
}

func TestPathToSubtree_Empty(t *testing.T) {
	assert.Nil(t, PathToSubtree(""))
	assert.Nil(t, PathToSubtree("/"))
}
