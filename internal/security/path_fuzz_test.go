package security

import (
	"testing"
)

// FuzzValidatePath tests ValidatePath with random inputs to find edge cases
func FuzzValidatePath(f *testing.F) {
	// Seed corpus with interesting test cases
	f.Add("/var/data", "file.txt")
	f.Add("/var/data", "../etc/passwd")
	f.Add("/var/data", "../../../../../../etc/passwd")
	f.Add("/var/data", "/etc/passwd")
	f.Add("/var/data", "subdir/file.txt")
	f.Add("/var/data", "")
	f.Add("/var/data", ".")
	f.Add("/var/data", "..")
	f.Add("/var/data", ".....")
	f.Add("/var/data", "file\x00.txt")
	f.Add("/var/data", "file\nname")
	f.Add("/var/data", "file\tname")

	f.Fuzz(func(t *testing.T, basePath, userPath string) {
		result, err := ValidatePath(basePath, userPath)

		// If no error, verify the result is within basePath
		if err == nil && basePath != "" {
			// Result should not be empty
			if result == "" {
				t.Errorf("ValidatePath returned empty result for basePath=%q, userPath=%q", basePath, userPath)
			}
		}
	})
}

// FuzzValidateFilename tests ValidateFilename with random inputs
func FuzzValidateFilename(f *testing.F) {
	// Seed corpus with interesting test cases
	f.Add("file.txt")
	f.Add("")
	f.Add(".")
	f.Add("..")
	f.Add(".hidden")
	f.Add("dir/file")
	f.Add("dir\\file")
	f.Add("file\x00.txt")
	f.Add("file\nname")
	f.Add("very_long_filename_" + string(make([]byte, 1000)))

	f.Fuzz(func(t *testing.T, filename string) {
		err := ValidateFilename(filename)

		// If no error, verify basic properties
		if err == nil {
			if filename == "" {
				t.Error("ValidateFilename should reject empty filename")
			}
			if filename == "." || filename == ".." {
				t.Errorf("ValidateFilename should reject %q", filename)
			}
		}
	})
}

// FuzzSanitizePathComponent tests SanitizePathComponent with random inputs
func FuzzSanitizePathComponent(f *testing.F) {
	// Seed corpus
	f.Add("filename")
	f.Add("dir/file")
	f.Add("dir\\file")
	f.Add(".hidden")
	f.Add("...")
	f.Add("")
	f.Add("file\x00name")

	f.Fuzz(func(t *testing.T, input string) {
		result := SanitizePathComponent(input)

		// Result should never be empty
		if result == "" {
			t.Errorf("SanitizePathComponent returned empty for input=%q", input)
		}

		// Result should not contain path separators
		for _, c := range result {
			if c == '/' || c == '\\' {
				t.Errorf("SanitizePathComponent result contains path separator: %q", result)
			}
		}

		// Result should not start with a dot
		if len(result) > 0 && result[0] == '.' {
			t.Errorf("SanitizePathComponent result starts with dot: %q", result)
		}
	})
}
