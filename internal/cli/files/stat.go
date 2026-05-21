package files

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/files"
)

func statCmd(g *globals) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "stat <remote-path>",
		Short: "Print metadata for a remote file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote, err := parseRemotePath(args[0])
			if err != nil {
				return err
			}
			c, closer, err := g.connect()
			if err != nil {
				return err
			}
			defer closer()

			meta, err := c.Stat(cmd.Context(), remote)
			if err != nil {
				return fmt.Errorf("stat: %w", err)
			}
			return renderStat(cmd.OutOrStdout(), meta, output)
		},
	}
	cmd.Flags().StringVar(&output, "output", "table", "Output format: table | json")
	return cmd
}

func renderStat(w io.Writer, m files.FileMetadata, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(m)
	case "table", "":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "PATH:\t%s\n", m.Path)
		fmt.Fprintf(tw, "SIZE:\t%d\n", m.Size)
		fmt.Fprintf(tw, "HASH:\t%s\n", m.Hash)
		fmt.Fprintf(tw, "VERSION:\t%d\n", m.Version)
		if m.ContentType != "" {
			fmt.Fprintf(tw, "CONTENT-TYPE:\t%s\n", m.ContentType)
		}
		fmt.Fprintf(tw, "CREATED:\t%s\n", m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
		for k, v := range m.Tags {
			fmt.Fprintf(tw, "TAG[%s]:\t%s\n", k, v)
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown output format %q (want table|json)", format)
	}
}
