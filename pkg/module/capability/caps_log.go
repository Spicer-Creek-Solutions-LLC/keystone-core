// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"context"
	"fmt"
	"log/slog"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// slogLogger is the default Logger — adapts to the standard slog.
type slogLogger struct{ l *slog.Logger }

func (s slogLogger) Log(level, msg string, kv map[string]string) {
	args := make([]any, 0, len(kv)*2)
	for k, v := range kv {
		args = append(args, k, v)
	}
	lvl := slog.LevelInfo
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	s.l.Log(context.Background(), lvl, msg, args...)
}

// Log is the rate-limited log capability. Over-budget calls return
// ErrRateLimited (the task-2 Invoker audits the drop; the task-12
// Starlark builtin may choose to swallow it).
type Log struct {
	limiter *tokenBucket
	host    Logger
}

// NewLog builds the capability. A nil host defaults to slog.
func NewLog(cfg manifest.CapabilityConfig, host Logger) (*Log, error) {
	lim, err := newRateLimiter(cfg.RateLimit)
	if err != nil {
		return nil, fmt.Errorf("log rate_limit: %w", err)
	}
	if host == nil {
		host = slogLogger{l: slog.Default()}
	}
	return &Log{limiter: lim, host: host}, nil
}

// Emit logs msg at level with structured kv, unless rate-limited.
func (c *Log) Emit(level, msg string, kv map[string]string) error {
	if !c.limiter.allow() {
		return fmt.Errorf("%w: log", ErrRateLimited)
	}
	c.host.Log(level, msg, kv)
	return nil
}
