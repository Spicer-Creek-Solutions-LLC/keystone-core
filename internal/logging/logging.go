// Package logging configures the project's structured logger.
//
// The implementation wraps log/slog: JSON output via JSONHandler, logfmt
// output via the standard TextHandler, and a "text" variant of TextHandler
// with shorter RFC3339 timestamps for terminal use. A correlation-ID
// injecting handler sits in front of all three so that any record logged
// against a context carrying a correlation ID emits it as the
// "correlation_id" attribute.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Options configures the logger constructed by New.
type Options struct {
	Level  string    // debug | info | warn | error
	Format string    // json | logfmt | text
	Output io.Writer // defaults to os.Stdout
}

// New returns a *slog.Logger configured per opts.
func New(opts Options) (*slog.Logger, error) {
	level, err := parseLevel(opts.Level)
	if err != nil {
		return nil, err
	}
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}

	hopts := &slog.HandlerOptions{Level: level}

	var base slog.Handler
	switch strings.ToLower(opts.Format) {
	case "json":
		base = slog.NewJSONHandler(out, hopts)
	case "logfmt":
		base = slog.NewTextHandler(out, hopts)
	case "text":
		base = slog.NewTextHandler(out, withTextReplaceAttr(hopts))
	default:
		return nil, fmt.Errorf("format: %q (must be json, logfmt, or text)", opts.Format)
	}
	return slog.New(&correlationHandler{Handler: base}), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("level: %q (must be debug, info, warn, or error)", s)
	}
}

// withTextReplaceAttr returns HandlerOptions whose ReplaceAttr formats
// the time attr as RFC3339 (no sub-second precision) for slightly cleaner
// terminal output. Other attrs pass through unchanged.
func withTextReplaceAttr(opts *slog.HandlerOptions) *slog.HandlerOptions {
	out := *opts
	out.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) > 0 {
			return a
		}
		if a.Key == slog.TimeKey {
			return slog.String(slog.TimeKey, a.Value.Time().UTC().Format(time.RFC3339))
		}
		return a
	}
	return &out
}
