package manifest

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// sizeUnits maps a unit suffix to its byte multiplier. KB/MB/GB are
// treated as binary (1024-based) — the common convention for memory
// + file-size limits in ops tooling; the explicit binary suffixes
// (KiB/MiB/GiB) are accepted as synonyms. A bare number is bytes.
var sizeUnits = []struct {
	suffix string
	mult   int64
}{
	{"GIB", 1 << 30}, {"MIB", 1 << 20}, {"KIB", 1 << 10},
	{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10},
	{"B", 1},
}

// ParseSize parses a human size string ("10MB", "64MiB", "512",
// "1.5GB") into bytes. Case-insensitive; fractional values allowed.
func ParseSize(s string) (int64, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, fmt.Errorf("empty size")
	}
	u := strings.ToUpper(t)
	for _, su := range sizeUnits {
		if !strings.HasSuffix(u, su.suffix) {
			continue
		}
		num := strings.TrimSpace(u[:len(u)-len(su.suffix)])
		if num == "" {
			return 0, fmt.Errorf("size %q has a unit but no number", s)
		}
		f, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, fmt.Errorf("size %q: %w", s, err)
		}
		if f < 0 {
			return 0, fmt.Errorf("size %q must be >= 0", s)
		}
		return int64(f * float64(su.mult)), nil
	}
	// No recognised unit → bare bytes.
	n, err := strconv.ParseInt(u, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("size %q: unrecognised unit or value", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("size %q must be >= 0", s)
	}
	return n, nil
}

// ParseRate parses a "<n>/<unit>" rate ("100/s", "5/m", "10/h")
// into a count and the window duration.
func ParseRate(s string) (count int, per time.Duration, err error) {
	parts := strings.SplitN(strings.TrimSpace(s), "/", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("rate %q must be <n>/<s|m|h>", s)
	}
	n, perr := strconv.Atoi(strings.TrimSpace(parts[0]))
	if perr != nil || n < 0 {
		return 0, 0, fmt.Errorf("rate %q: count must be a non-negative integer", s)
	}
	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "s", "sec", "second":
		per = time.Second
	case "m", "min", "minute":
		per = time.Minute
	case "h", "hr", "hour":
		per = time.Hour
	default:
		return 0, 0, fmt.Errorf("rate %q: unit must be s, m, or h", s)
	}
	return n, per, nil
}
