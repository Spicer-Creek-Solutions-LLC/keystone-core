package framework

import "testing"

func TestSanitizeName(t *testing.T) {
	tests := map[string]string{
		"ubuntu-22.04":  "ubuntu-22.04",
		"rhel/9":        "rhel-9",
		"alpine latest": "alpine-latest",
	}

	for input, expected := range tests {
		if got := sanitizeName(input); got != expected {
			t.Fatalf("sanitizeName(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestImageTag(t *testing.T) {
	tag := imageTag("", "ubuntu-22.04")
	if tag != "kscore-bootstrap:ubuntu-22.04" {
		t.Fatalf("unexpected tag: %s", tag)
	}

	tag = imageTag("ghcr.io/shawnbutts/keystone-core/", "ubuntu-22.04")
	if tag != "ghcr.io/shawnbutts/keystone-core/kscore-bootstrap:ubuntu-22.04" {
		t.Fatalf("unexpected tag with registry: %s", tag)
	}
}
