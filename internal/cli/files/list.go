package files

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/files"
)

func listCmd(g *globals) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "list [prefix]",
		Short: "List files at a prefix (empty = list all)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix := ""
			if len(args) == 1 {
				p, err := parseListPrefix(args[0])
				if err != nil {
					return err
				}
				prefix = p
			}

			c, closer, err := g.connect()
			if err != nil {
				return err
			}
			defer closer()

			list, err := c.List(cmd.Context(), prefix)
			if err != nil {
				return fmt.Errorf("list: %w", err)
			}
			return renderList(cmd.OutOrStdout(), list, output)
		},
	}
	cmd.Flags().StringVar(&output, "output", "table", "Output format: table | json")
	return cmd
}

// parseListPrefix accepts the same kv:// URI form as
// [parseRemotePath] but allows a trailing slash (a list-prefix
// may end at a directory boundary). Empty / kv:// alone → empty
// prefix.
func parseListPrefix(s string) (string, error) {
	s = strings.TrimPrefix(s, kvScheme)
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("prefix must not start with %q", "/")
	}
	return s, nil
}

// renderList writes list to w in the requested format. table is
// columnar via text/tabwriter; json is a top-level array of
// FileMetadata.
func renderList(w io.Writer, list []files.FileMetadata, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	case "table", "":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "PATH\tSIZE\tVERSION\tHASH")
		for _, m := range list {
			fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", m.Path, m.Size, m.Version, shortHash(m.Hash))
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown output format %q (want table|json)", format)
	}
}

// shortHash trims a 64-char hex SHA-256 to the first 12 chars for
// table-mode readability. JSON keeps the full hash.
func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
