// SPDX-License-Identifier: Apache-2.0

package files

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/files"
)

func putCmd(g *globals) *cobra.Command {
	var (
		contentType string
		tagFlags    []string
	)
	cmd := &cobra.Command{
		Use:   "put <local-file> <remote-path>",
		Short: "Upload a local file via NATS chunks",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localPath := args[0]
			remote, err := parseRemotePath(args[1])
			if err != nil {
				return err
			}

			body, err := os.ReadFile(localPath) //nolint:gosec // operator-supplied path
			if err != nil {
				return fmt.Errorf("read %s: %w", localPath, err)
			}

			tags, err := parseTagFlags(tagFlags)
			if err != nil {
				return err
			}

			c, closer, err := g.connect()
			if err != nil {
				return err
			}
			defer closer()

			meta, err := c.Put(cmd.Context(), files.FileMetadata{
				Path:        remote,
				ContentType: contentType,
				Tags:        tags,
			}, body)
			if err != nil {
				return fmt.Errorf("put: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"uploaded %s -> %s (size=%d hash=%s version=%d)\n",
				localPath, remote, meta.Size, meta.Hash, meta.Version,
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&contentType, "content-type", "", "Content-Type for the uploaded file")
	cmd.Flags().StringSliceVar(&tagFlags, "tag", nil, "Tag in key=value form; repeat for multiple")
	return cmd
}

// parseTagFlags decodes the --tag key=value strings cobra collected
// into a map. Empty input yields nil so the transport client
// doesn't emit an empty tags object.
func parseTagFlags(in []string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(in))
	for _, kv := range in {
		i := strings.Index(kv, "=")
		if i <= 0 {
			return nil, fmt.Errorf("invalid --tag %q (want key=value)", kv)
		}
		out[kv[:i]] = kv[i+1:]
	}
	return out, nil
}
