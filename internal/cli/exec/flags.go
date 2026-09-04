// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
)

// dispatchFlags carries the shared flag set for run / async / script.
// Bound via bindDispatchFlags onto each subcommand.
type dispatchFlags struct {
	Target            string
	Concurrency       int
	CommandTimeout    time.Duration
	ContinueOnFailure bool
	Shell             string
	WorkingDir        string
	User              string
	Env               []string // K=V strings; parsed by envMap()
	JobID             string
	DryRun            bool
	ShowOutput        bool
}

func bindDispatchFlags(fs *pflag.FlagSet, f *dispatchFlags) {
	fs.StringVar(&f.Target, "target", "",
		`target expression: id:<id>[,...] | hostname:<glob> | <label>:<value> [AND ...]`)
	fs.IntVar(&f.Concurrency, "concurrency", 10,
		"max in-flight agents (server default if 0)")
	fs.DurationVar(&f.CommandTimeout, "command-timeout", 5*time.Minute,
		"per-agent command timeout")
	fs.BoolVar(&f.ContinueOnFailure, "continue-on-failure", false,
		"keep dispatching after an agent fails (server-side honor: v1.x)")
	fs.StringVar(&f.Shell, "shell", "",
		"shell to wrap the command (bash | sh | powershell | cmd); empty = direct exec")
	fs.StringVar(&f.WorkingDir, "working-dir", "",
		"working directory on the agent")
	fs.StringVar(&f.User, "user", "",
		"user to run as on the agent (Linux uid switch)")
	fs.BoolVar(&f.ShowOutput, "show-output", false,
		"print each agent's captured stdout/stderr beneath the status table")
	fs.StringSliceVar(&f.Env, "env", nil,
		"K=V env var (repeatable)")
	fs.StringVar(&f.JobID, "job-id", "",
		"override the generated batch job ID (must be unique)")
	fs.BoolVar(&f.DryRun, "dry-run", false,
		"resolve target and print matched agents without dispatching")
}

// envMap parses --env entries into a map. Reject duplicate keys and
// malformed pairs.
func envMap(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			return nil, fmt.Errorf("exec: --env %q: missing '='", e)
		}
		k = strings.TrimSpace(k)
		if k == "" {
			return nil, fmt.Errorf("exec: --env %q: empty key", e)
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("exec: --env %q: duplicate key", k)
		}
		out[k] = v
	}
	return out, nil
}
