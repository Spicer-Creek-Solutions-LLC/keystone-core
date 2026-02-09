package gnmi

import (
	"context"
	"testing"
)

func TestMetadataCredentials_GetRequestMetadata(t *testing.T) {
	creds := &metadataCredentials{
		username: "admin",
		password: "secret123",
	}

	md, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if md["username"] != "admin" {
		t.Errorf("expected username=admin, got %q", md["username"])
	}
	if md["password"] != "secret123" {
		t.Errorf("expected password=secret123, got %q", md["password"])
	}
}

func TestMetadataCredentials_RequireTransportSecurity(t *testing.T) {
	creds := &metadataCredentials{}
	if !creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity() to return true")
	}
}
