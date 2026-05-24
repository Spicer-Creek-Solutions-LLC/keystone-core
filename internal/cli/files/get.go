// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/transport"
)

func getCmd(g *globals) *cobra.Command {
	var maxRetries int
	cmd := &cobra.Command{
		Use:   "get <remote-path> <local-dest>",
		Short: "Download a file via NATS chunks; verifies SHA-256",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, err := parseRemotePath(args[0])
			if err != nil {
				return err
			}
			localDest := args[1]

			c, closer, err := g.connect()
			if err != nil {
				return err
			}
			defer closer()

			body, meta, err := getWithResume(cmd, c, remote, maxRetries, g.logger)
			if err != nil {
				return err
			}

			// 0o600 — operator-supplied destination, default to
			// owner-read/write only. Operators who want broader
			// permissions chmod the file after.
			if err := os.WriteFile(localDest, body, 0o600); err != nil {
				return fmt.Errorf("write %s: %w", localDest, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"downloaded %s -> %s (size=%d hash=%s version=%d)\n",
				remote, localDest, meta.Size, meta.Hash, meta.Version,
			)
			return nil
		},
	}
	cmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum resume attempts on partial-receive errors")
	return cmd
}

// getWithResume drives transport.Client.Get with bounded retry
// on partial-receive errors. The current v1.0 client returns the
// whole body or an error; T11's chunk timeout produces a typed
// error string ("chunk timeout"). We retry up to maxRetries by
// re-issuing a full Get — chunk-offset resume from the client side
// requires the client to track partial-receive state, which v1.0
// does not surface.
func getWithResume(cmd *cobra.Command, c *transport.Client, remote string, maxRetries int, logger interface{ Warn(string, ...any) }) ([]byte, files.FileMetadata, error) {
	attempt := 0
	for {
		meta, body, err := c.Get(cmd.Context(), remote, transport.GetOptions{})
		if err == nil {
			return body, meta, nil
		}
		if !isResumableErr(err) || attempt >= maxRetries {
			return nil, files.FileMetadata{}, fmt.Errorf("get: %w", err)
		}
		attempt++
		logger.Warn("get: resumable error, retrying", "attempt", attempt, "err", err)
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
}

// isResumableErr returns true if err is one of the partial-receive
// flavours T11's client emits — chunk timeout or assembled-hash
// mismatch (the latter shouldn't happen but a retry would re-fetch
// fresh bytes from the backend on the chance of a wire flip).
func isResumableErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "chunk timeout"):
		return true
	case strings.Contains(s, "response timeout"):
		return true
	default:
		return false
	}
}

