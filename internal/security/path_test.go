package security

import (
	"path/filepath"
	"testing"
)

func TestValidatePath(t *testing.T) {
	basePath := "/var/data"

	tests := []struct {
		name     string
		basePath string
		userPath string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "simple valid path",
			basePath: basePath,
			userPath: "file.txt",
			wantPath: filepath.Join(basePath, "file.txt"),
			wantErr:  false,
		},
		{
			name:     "nested valid path",
			basePath: basePath,
			userPath: "subdir/file.txt",
			wantPath: filepath.Join(basePath, "subdir/file.txt"),
			wantErr:  false,
		},
		{
			name:     "path traversal attempt",
			basePath: basePath,
			userPath: "../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "absolute path converted to relative",
			basePath: basePath,
			userPath: "/etc/passwd",
			wantPath: filepath.Join(basePath, "etc/passwd"),
			wantErr:  false, // filepath.Join makes absolute paths relative to basePath
		},
		{
			name:     "double dot in middle",
			basePath: basePath,
			userPath: "subdir/../../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "relative base path",
			basePath: "relative/path",
			userPath: "file.txt",
			wantErr:  true,
		},
		{
			name:     "empty user path",
			basePath: basePath,
			userPath: "",
			wantPath: basePath,
			wantErr:  false,
		},
		{
			name:     "dot path",
			basePath: basePath,
			userPath: ".",
			wantPath: basePath,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidatePath(tt.basePath, tt.userPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantPath {
				t.Errorf("ValidatePath() = %v, want %v", got, tt.wantPath)
			}
		})
	}
}

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "valid filename",
			filename: "file.txt",
			wantErr:  false,
		},
		{
			name:     "filename with spaces",
			filename: "my file.txt",
			wantErr:  false,
		},
		{
			name:     "empty filename",
			filename: "",
			wantErr:  true,
		},
		{
			name:     "forward slash",
			filename: "dir/file.txt",
			wantErr:  true,
		},
		{
			name:     "backslash",
			filename: "dir\\file.txt",
			wantErr:  true,
		},
		{
			name:     "dot directory",
			filename: ".",
			wantErr:  true,
		},
		{
			name:     "double dot",
			filename: "..",
			wantErr:  true,
		},
		{
			name:     "hidden file",
			filename: ".hidden",
			wantErr:  true,
		},
		{
			name:     "null byte",
			filename: "file\x00.txt",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilename() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizePathComponent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean input",
			input: "filename",
			want:  "filename",
		},
		{
			name:  "forward slash",
			input: "dir/file",
			want:  "dir_file",
		},
		{
			name:  "backslash",
			input: "dir\\file",
			want:  "dir_file",
		},
		{
			name:  "leading dot",
			input: ".hidden",
			want:  "hidden",
		},
		{
			name:  "multiple leading dots",
			input: "...file",
			want:  "file",
		},
		{
			name:  "only dots",
			input: "...",
			want:  "unnamed",
		},
		{
			name:  "empty string",
			input: "",
			want:  "unnamed",
		},
		{
			name:  "null byte",
			input: "file\x00name",
			want:  "filename",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizePathComponent(tt.input)
			if got != tt.want {
				t.Errorf("SanitizePathComponent() = %v, want %v", got, tt.want)
			}
		})
	}
}
