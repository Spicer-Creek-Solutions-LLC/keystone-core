package secrets

import "testing"

func TestDisabledRotationPolicy(t *testing.T) {
	t.Parallel()

	p := DisabledRotationPolicy()
	if p.Enabled {
		t.Errorf("DisabledRotationPolicy.Enabled = true, want false")
	}
	if p.Interval != 0 {
		t.Errorf("DisabledRotationPolicy.Interval = %v, want 0", p.Interval)
	}
	if p.Strategy != "" {
		t.Errorf("DisabledRotationPolicy.Strategy = %q, want \"\"", p.Strategy)
	}
}
