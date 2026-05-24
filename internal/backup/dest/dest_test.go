// SPDX-License-Identifier: Apache-2.0

package dest

import (
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	cfg := Config{AccessKey: "ak", SecretKey: "sk", Region: "us-east-1"}

	cases := []struct {
		name     string
		uri      string
		wantType string // "local", "s3", or "" for error
		wantSub  string // substring expected in the error
		check    func(t *testing.T, d Destination)
	}{
		{
			name:     "absolute path no scheme",
			uri:      "/tmp/backup.tar",
			wantType: "local",
			check: func(t *testing.T, d Destination) {
				if l, _ := d.(*LocalDestination); l == nil || l.Path != "/tmp/backup.tar" {
					t.Errorf("path = %#v", d)
				}
			},
		},
		{
			name:     "relative path no scheme",
			uri:      "./out/backup.tar",
			wantType: "local",
			check: func(t *testing.T, d Destination) {
				if l, _ := d.(*LocalDestination); l == nil || l.Path != "./out/backup.tar" {
					t.Errorf("path = %#v", d)
				}
			},
		},
		{
			name:     "file:// scheme",
			uri:      "file:///var/lib/kscore/backup.tar",
			wantType: "local",
			check: func(t *testing.T, d Destination) {
				if l, _ := d.(*LocalDestination); l == nil || l.Path != "/var/lib/kscore/backup.tar" {
					t.Errorf("path = %#v", d)
				}
			},
		},
		{
			name:     "s3:// scheme",
			uri:      "s3://my-bucket/path/to/backup.tar",
			wantType: "s3",
			check: func(t *testing.T, d Destination) {
				s, _ := d.(*S3Destination)
				if s == nil {
					t.Fatalf("not S3Destination: %#v", d)
				}
				if s.Bucket != "my-bucket" || s.Key != "path/to/backup.tar" {
					t.Errorf("bucket=%q key=%q", s.Bucket, s.Key)
				}
				if s.Config.AccessKey != "ak" {
					t.Errorf("Config not propagated")
				}
			},
		},
		{
			name:    "s3:// missing key",
			uri:     "s3://bucket",
			wantSub: "key must not be empty",
		},
		{
			name:    "s3:// missing bucket",
			uri:     "s3:///key-only",
			wantSub: "bucket must not be empty",
		},
		{
			name:    "unsupported scheme",
			uri:     "https://example.com/foo",
			wantSub: "unsupported scheme",
		},
		{
			name:    "empty URI",
			uri:     "",
			wantSub: "empty URI",
		},
		{
			name:    "file:// with non-localhost host",
			uri:     "file://example.com/path",
			wantSub: "host must be empty or localhost",
		},
		{
			name:     "file://localhost is fine",
			uri:      "file://localhost/path/to/file.tar",
			wantType: "local",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Resolve(tc.uri, cfg)
			if tc.wantSub != "" {
				if err == nil {
					t.Fatalf("want error, got dest=%#v", d)
				}
				if !strings.Contains(err.Error(), tc.wantSub) {
					t.Errorf("err = %v, want substring %q", err, tc.wantSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tc.wantType {
			case "local":
				if _, ok := d.(*LocalDestination); !ok {
					t.Errorf("want LocalDestination, got %T", d)
				}
			case "s3":
				if _, ok := d.(*S3Destination); !ok {
					t.Errorf("want S3Destination, got %T", d)
				}
			}
			if tc.check != nil {
				tc.check(t, d)
			}
		})
	}
}
