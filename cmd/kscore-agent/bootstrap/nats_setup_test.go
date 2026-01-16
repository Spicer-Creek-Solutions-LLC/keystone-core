package bootstrap

import "testing"

func TestValidateNATSURLs(t *testing.T) {
	valid := []string{"nats://nats1:4222", "tls://nats2:4222"}
	if err := validateNATSURLs(valid); err != nil {
		t.Fatalf("expected valid urls, got error: %v", err)
	}

	invalid := []string{"http://example.com"}
	if err := validateNATSURLs(invalid); err == nil {
		t.Fatal("expected error for invalid nats url scheme")
	}
}
