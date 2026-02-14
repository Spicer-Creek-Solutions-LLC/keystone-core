package bootstrap

import (
	"testing"
)

func TestPlatformString(t *testing.T) {
	p := Platform{OS: "linux", Arch: "amd64"}
	if got := p.String(); got != "linux/amd64" {
		t.Errorf("String() = %q, want %q", got, "linux/amd64")
	}
}

func TestPlatformValidate(t *testing.T) {
	tests := []struct {
		name    string
		p       Platform
		wantErr bool
	}{
		{"linux/amd64", Platform{OS: "linux", Arch: "amd64"}, false},
		{"linux/arm64", Platform{OS: "linux", Arch: "arm64"}, false},
		{"windows/amd64", Platform{OS: "windows", Arch: "amd64"}, false},
		{"empty os", Platform{OS: "", Arch: "amd64"}, true},
		{"empty arch", Platform{OS: "linux", Arch: ""}, true},
		{"unsupported os", Platform{OS: "freebsd", Arch: "amd64"}, true},
		{"unsupported arch", Platform{OS: "linux", Arch: "mips"}, true},
		{"darwin unsupported", Platform{OS: "darwin", Arch: "amd64"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Platform
		wantErr bool
	}{
		{"linux/amd64", "linux/amd64", Platform{OS: "linux", Arch: "amd64"}, false},
		{"linux/arm64", "linux/arm64", Platform{OS: "linux", Arch: "arm64"}, false},
		{"windows/amd64", "windows/amd64", Platform{OS: "windows", Arch: "amd64"}, false},
		{"no slash", "linux", Platform{}, true},
		{"empty", "", Platform{}, true},
		{"empty parts", "/", Platform{}, true},
		{"empty os", "/amd64", Platform{}, true},
		{"empty arch", "linux/", Platform{}, true},
		{"unsupported", "freebsd/amd64", Platform{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePlatform(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePlatform(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParsePlatform(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSupportedPlatforms(t *testing.T) {
	platforms := SupportedPlatforms()
	if len(platforms) < 2 {
		t.Fatalf("expected at least 2 supported platforms, got %d", len(platforms))
	}

	found := make(map[string]bool)
	for _, p := range platforms {
		found[p.String()] = true
	}
	for _, want := range []string{"linux/amd64", "linux/arm64"} {
		if !found[want] {
			t.Errorf("expected %q in supported platforms", want)
		}
	}
}
