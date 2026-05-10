package exec

import "testing"

func TestEnvMap(t *testing.T) {
	t.Parallel()
	got, err := envMap([]string{"FOO=bar", "BAZ=qux"})
	if err != nil {
		t.Fatal(err)
	}
	if got["FOO"] != "bar" || got["BAZ"] != "qux" {
		t.Errorf("got %v", got)
	}
}

func TestEnvMap_AcceptsEqualsInValue(t *testing.T) {
	t.Parallel()
	got, err := envMap([]string{"URL=https://example.com?q=1"})
	if err != nil {
		t.Fatal(err)
	}
	if got["URL"] != "https://example.com?q=1" {
		t.Errorf("URL = %q", got["URL"])
	}
}

func TestEnvMap_Errors(t *testing.T) {
	t.Parallel()
	if _, err := envMap([]string{"NOEQUALS"}); err == nil {
		t.Error("missing '=' should error")
	}
	if _, err := envMap([]string{"=value"}); err == nil {
		t.Error("empty key should error")
	}
	if _, err := envMap([]string{"K=1", "K=2"}); err == nil {
		t.Error("duplicate key should error")
	}
}

func TestEnvMap_Empty(t *testing.T) {
	t.Parallel()
	got, err := envMap(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
