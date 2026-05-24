// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"
	"strings"
)

// kvScheme is the optional URI prefix kscore-files accepts on
// remote paths. The CLI strips it; the underlying file service
// path is just the slash-delimited tail.
const kvScheme = "kv://"

// parseRemotePath accepts both bare paths (`configs/app.yaml`)
// and `kv://` URIs (`kv://configs/app.yaml`) and returns the
// canonical slash-delimited form the file service expects.
// Empty input is rejected so subcommand error messages are
// consistent.
func parseRemotePath(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("remote path is required")
	}
	s = strings.TrimPrefix(s, kvScheme)
	if s == "" {
		return "", fmt.Errorf("remote path is required (kv:// with no path)")
	}
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("remote path must not start with %q", "/")
	}
	return s, nil
}
