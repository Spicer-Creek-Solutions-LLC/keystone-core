package netconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCapabilities_YANGModels(t *testing.T) {
	caps := []Capability{
		BaseCapability10,
		"urn:ietf:params:xml:ns:yang:ietf-interfaces?module=ietf-interfaces&revision=2018-02-20",
		"http://cisco.com/ns/yang/Cisco-IOS-XE-native?module=Cisco-IOS-XE-native&revision=2023-03-01&features=feature1,feature2",
		"urn:ietf:params:xml:ns:yang:ietf-ip?module=ietf-ip&revision=2018-02-22&deviations=cisco-devs",
	}

	models := ParseCapabilities(caps)
	assert.Len(t, models, 3)

	assert.Equal(t, "ietf-interfaces", models[0].Module)
	assert.Equal(t, "2018-02-20", models[0].Revision)

	assert.Equal(t, "Cisco-IOS-XE-native", models[1].Module)
	assert.Equal(t, "2023-03-01", models[1].Revision)
	assert.Equal(t, []string{"feature1", "feature2"}, models[1].Features)

	assert.Equal(t, "ietf-ip", models[2].Module)
	assert.Equal(t, []string{"cisco-devs"}, models[2].Deviations)
}

func TestParseCapabilities_NoModules(t *testing.T) {
	caps := []Capability{
		BaseCapability10,
		BaseCapability11,
		CandidateCapability,
	}
	models := ParseCapabilities(caps)
	assert.Empty(t, models)
}

func TestHasCapability(t *testing.T) {
	caps := []Capability{
		BaseCapability10,
		BaseCapability11,
		CandidateCapability,
		"urn:ietf:params:netconf:capability:url:1.0?scheme=http,ftp",
	}

	assert.True(t, HasCapability(caps, BaseCapability10))
	assert.True(t, HasCapability(caps, BaseCapability11))
	assert.True(t, HasCapability(caps, CandidateCapability))
	assert.True(t, HasCapability(caps, URLCapability)) // matches prefix before ?
	assert.False(t, HasCapability(caps, WritableRunning))
	assert.False(t, HasCapability(caps, RollbackOnError))
}

func TestSupportsCandidate(t *testing.T) {
	assert.True(t, SupportsCandidate([]Capability{CandidateCapability}))
	assert.False(t, SupportsCandidate([]Capability{WritableRunning}))
}

func TestSupportsWritableRunning(t *testing.T) {
	assert.True(t, SupportsWritableRunning([]Capability{WritableRunning}))
	assert.False(t, SupportsWritableRunning([]Capability{CandidateCapability}))
}

func TestSupportsValidate(t *testing.T) {
	assert.True(t, SupportsValidate([]Capability{Validate10}))
	assert.True(t, SupportsValidate([]Capability{Validate11}))
	assert.False(t, SupportsValidate([]Capability{BaseCapability10}))
}

func TestSupportsRollbackOnError(t *testing.T) {
	assert.True(t, SupportsRollbackOnError([]Capability{RollbackOnError}))
	assert.False(t, SupportsRollbackOnError([]Capability{BaseCapability10}))
}

func TestSupportsConfirmedCommit(t *testing.T) {
	assert.True(t, SupportsConfirmedCommit([]Capability{ConfirmedCommit10}))
	assert.True(t, SupportsConfirmedCommit([]Capability{ConfirmedCommit11}))
	assert.False(t, SupportsConfirmedCommit([]Capability{BaseCapability10}))
}

func TestSupportsStartup(t *testing.T) {
	assert.True(t, SupportsStartup([]Capability{StartupCapability}))
	assert.False(t, SupportsStartup([]Capability{BaseCapability10}))
}

func TestSupportsXPath(t *testing.T) {
	assert.True(t, SupportsXPath([]Capability{XPathCapability}))
	assert.False(t, SupportsXPath([]Capability{BaseCapability10}))
}

func TestSupportsURL(t *testing.T) {
	assert.True(t, SupportsURL([]Capability{URLCapability}))
	assert.True(t, SupportsURL([]Capability{"urn:ietf:params:netconf:capability:url:1.0?scheme=http"}))
	assert.False(t, SupportsURL([]Capability{BaseCapability10}))
}

func TestSupportsBase11(t *testing.T) {
	assert.True(t, SupportsBase11([]Capability{BaseCapability11}))
	assert.False(t, SupportsBase11([]Capability{BaseCapability10}))
}
