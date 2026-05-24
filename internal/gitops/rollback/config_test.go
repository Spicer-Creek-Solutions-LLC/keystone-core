// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"errors"
	"testing"
)

func TestCfgString(t *testing.T) {
	t.Parallel()
	if _, err := cfgString(Config{}, "repo_url"); !errors.Is(err, ErrConfig) {
		t.Errorf("missing key err = %v, want ErrConfig", err)
	}
	if _, err := cfgString(Config{"repo_url": 1}, "repo_url"); !errors.Is(err, ErrConfig) {
		t.Errorf("wrong type err = %v, want ErrConfig", err)
	}
	if _, err := cfgString(Config{"repo_url": ""}, "repo_url"); !errors.Is(err, ErrConfig) {
		t.Errorf("empty string err = %v, want ErrConfig", err)
	}
	got, err := cfgString(Config{"repo_url": "https://x"}, "repo_url")
	if err != nil || got != "https://x" {
		t.Errorf("cfgString = %q,%v", got, err)
	}
}

func TestCfgStringOpt(t *testing.T) {
	t.Parallel()
	if v := cfgStringOpt(Config{}, "branch", "main"); v != "main" {
		t.Errorf("default not returned: %q", v)
	}
	if v := cfgStringOpt(Config{"branch": ""}, "branch", "main"); v != "main" {
		t.Errorf("empty value should fall back to default: %q", v)
	}
	if v := cfgStringOpt(Config{"branch": "dev"}, "branch", "main"); v != "dev" {
		t.Errorf("set value not returned: %q", v)
	}
}
