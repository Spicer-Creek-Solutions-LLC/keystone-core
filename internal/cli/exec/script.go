package exec

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func scriptCmd(g *globals) *cobra.Command {
	var flags dispatchFlags
	cmd := &cobra.Command{
		Use:   "script <file>",
		Short: "Read a script file and dispatch its contents to agents",
		Long: "Loads <file> as a single shell script and dispatches it to " +
			"agents matching --target. Implicitly uses --shell bash unless " +
			"--shell is set otherwise. Stream output is rendered like " +
			"`exec run`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			body, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
			if err != nil {
				return fmt.Errorf("exec script: read %s: %w", path, err)
			}
			if len(body) == 0 {
				return fmt.Errorf("exec script: %s is empty", path)
			}
			// Default to bash if --shell not specified — a raw script
			// payload without a shell wouldn't make sense as an
			// argv-style exec.
			if flags.Shell == "" {
				flags.Shell = "bash"
			}
			return runDispatch(cmd, g, &flags, string(body), nil, false)
		},
	}
	bindDispatchFlags(cmd.Flags(), &flags)
	return cmd
}
