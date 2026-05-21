package s3client

import (
	"errors"
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name: "ok minimum",
			cfg: Config{
				AccessKey: "AKIA",
				SecretKey: "secret",
			},
		},
		{
			name: "ok custom endpoint + region",
			cfg: Config{
				AccessKey: "ID",
				SecretKey: "key",
				Endpoint:  "s3.us-west-002.backblazeb2.com",
				Region:    "us-west-002",
				UseSSL:    true,
			},
		},
		{
			name: "missing access key",
			cfg: Config{
				SecretKey: "secret",
			},
			wantErr: ErrMissingCredentials,
		},
		{
			name: "missing secret key",
			cfg: Config{
				AccessKey: "AKIA",
			},
			wantErr: ErrMissingCredentials,
		},
		{
			name:    "empty config",
			cfg:     Config{},
			wantErr: ErrMissingCredentials,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient(tc.cfg)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if client != nil {
					t.Errorf("client = %v, want nil on error", client)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if client == nil {
				t.Fatal("client = nil, want non-nil")
			}
		})
	}
}

func TestNewClient_BadEndpoint(t *testing.T) {
	// minio.New itself rejects malformed endpoints; the wrapper
	// returns the wrapped error so callers see s3client: minio.New: ...
	_, err := NewClient(Config{
		AccessKey: "ID",
		SecretKey: "key",
		Endpoint:  "http://has-scheme.example.com", // scheme is not allowed
	})
	if err == nil {
		t.Fatal("expected error from minio.New on bad endpoint")
	}
}
