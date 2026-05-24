// SPDX-License-Identifier: Apache-2.0

package gitops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
)

// newRollbackEngine builds a rollback engine over the --store SQLite
// path (or :memory: for the in-memory default) with the seam set
// supplied via Deps + the CLI's executor-specific flags. argoFactory
// is invoked lazily so a rollback that doesn't use ArgoCD doesn't
// require --argo-server.
func newRollbackEngine(d Deps, storePath, argoServer, argoToken string) (*rollback.Engine, func() error, error) {
	store, err := rollback.NewSQLiteStore(storePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open rollback store at %q: %w", storePath, err)
	}
	e := rollback.NewEngine(store)
	var argo rollback.ArgoClient
	if d.NewArgoClient != nil && argoServer != "" {
		argo = d.NewArgoClient(argoServer, argoToken)
	}
	for _, x := range []rollback.Executor{
		rollback.GitRevertExecutor{Client: d.GitClient},
		rollback.ArgoCDExecutor{Client: argo},
		rollback.K8sRolloutExecutor{Client: d.K8sClient},
	} {
		if err := e.RegisterExecutor(x); err != nil {
			_ = store.Close()
			return nil, nil, err
		}
	}
	return e, store.Close, nil
}

// rollbackFlags is the shared flag set for the rollback execute /
// approve commands.
type rollbackFlags struct {
	app             string
	executor        string
	strategy        string
	revision        string
	reason          string
	requireApproval bool
	approver        string

	repoURL    string
	branch     string
	authToken  string
	argoServer string
	argoToken  string
	argoApp    string
	namespace  string
	deployment string

	store  string
	output string
}

func (f *rollbackFlags) bindCommon(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.store, "store", "./.kscore-gitops.db",
		"path to the rollback SQLite store (use ':memory:' for ephemeral)")
	cmd.Flags().StringVar(&f.output, "output", "text", "output format: text|json")
}

func (f *rollbackFlags) bindExecute(cmd *cobra.Command) {
	f.bindCommon(cmd)
	cmd.Flags().StringVar(&f.app, "app", "", "application name")
	cmd.Flags().StringVar(&f.executor, "executor", "git", "rollback executor: git|argocd|k8s")
	cmd.Flags().StringVar(&f.strategy, "strategy", "previous", "rollback strategy: previous|specific|last-known-good")
	cmd.Flags().StringVar(&f.revision, "revision", "", "explicit target revision (strategy=specific)")
	cmd.Flags().StringVar(&f.reason, "reason", "", "operator-supplied audit reason")
	cmd.Flags().BoolVar(&f.requireApproval, "require-approval", false, "create the rollback at Pending and wait for `rollback approve`")

	cmd.Flags().StringVar(&f.repoURL, "repo-url", "", "[git] repository URL")
	cmd.Flags().StringVar(&f.branch, "branch", "main", "[git] branch")
	cmd.Flags().StringVar(&f.authToken, "auth-token", "", "[git] HTTP token for push (also persisted on the record — see v1.0 secret note)")

	cmd.Flags().StringVar(&f.argoServer, "argo-server", "", "[argocd] API server base URL")
	cmd.Flags().StringVar(&f.argoToken, "argo-token", "", "[argocd] bearer token")
	cmd.Flags().StringVar(&f.argoApp, "argo-app", "", "[argocd] application name (defaults to --app)")

	cmd.Flags().StringVar(&f.namespace, "namespace", "default", "[k8s] namespace")
	cmd.Flags().StringVar(&f.deployment, "deployment", "", "[k8s] deployment name")
}

// buildConfig assembles the executor-specific rollback.Config from
// the relevant flags based on --executor.
func (f *rollbackFlags) buildConfig() rollback.Config {
	cfg := rollback.Config{}
	switch f.executor {
	case "git":
		if f.repoURL != "" {
			cfg["repo_url"] = f.repoURL
		}
		if f.branch != "" {
			cfg["branch"] = f.branch
		}
		if f.authToken != "" {
			cfg["auth_token"] = f.authToken
		}
	case "argocd":
		if f.argoApp != "" {
			cfg["app"] = f.argoApp
		}
	case "k8s":
		if f.namespace != "" {
			cfg["namespace"] = f.namespace
		}
		if f.deployment != "" {
			cfg["deployment"] = f.deployment
		}
	}
	return cfg
}

func newRollbackCmd(d Deps) *cobra.Command {
	var f rollbackFlags
	root := &cobra.Command{
		Use:   "rollback",
		Short: "Trigger a GitOps rollback",
		Long:  "With --app, executes a rollback against the local engine; subcommands manage the lifecycle (approve / reject / get / list).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.app == "" {
				return fmt.Errorf("--app is required (or use a `rollback <subcommand>`)")
			}
			if !rollback.Strategy(f.strategy).Valid() {
				return fmt.Errorf("unknown --strategy %q (want previous|specific|last-known-good)", f.strategy)
			}
			e, closeStore, err := newRollbackEngine(d, f.store, f.argoServer, f.argoToken)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			spec := rollback.RollbackSpec{
				ExecutorType:    f.executor,
				Config:          f.buildConfig(),
				RequireApproval: f.requireApproval,
				Request: rollback.Request{
					Application: f.app,
					Strategy:    rollback.Strategy(f.strategy),
					Revision:    f.revision,
					Reason:      f.reason,
				},
			}
			rb, err := e.Execute(context.Background(), spec)
			if err != nil {
				return err
			}
			return formatRollback(cmd.OutOrStdout(), f.output, rb)
		},
	}
	f.bindExecute(root)

	root.AddCommand(newApproveCmd(d, &f))
	root.AddCommand(newRejectCmd(d, &f))
	root.AddCommand(newGetCmd(d, &f))
	root.AddCommand(newListCmd(d, &f))
	return root
}

func newApproveCmd(d Deps, parent *rollbackFlags) *cobra.Command {
	var f rollbackFlags
	cmd := &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a Pending rollback and drive it to completion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Inherit --store / --output from the parent command if
			// the subcommand-local flags weren't set.
			store, output := pickStore(&f, parent), pickOutput(&f, parent)
			e, closeStore, err := newRollbackEngine(d, store, f.argoServer, f.argoToken)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()
			rb, err := e.ApproveRollback(context.Background(), args[0], f.approver)
			if err != nil {
				if errors.Is(err, rollback.ErrRollbackNotFound) {
					return fmt.Errorf("rollback %q not found in %s", args[0], store)
				}
				return err
			}
			return formatRollback(cmd.OutOrStdout(), output, rb)
		},
	}
	f.bindCommon(cmd)
	cmd.Flags().StringVar(&f.approver, "approver", "", "approver identity (recorded on the rollback)")
	cmd.Flags().StringVar(&f.argoServer, "argo-server", "", "[argocd] API server (only if the rollback uses argocd)")
	cmd.Flags().StringVar(&f.argoToken, "argo-token", "", "[argocd] bearer token")
	return cmd
}

func newRejectCmd(d Deps, parent *rollbackFlags) *cobra.Command {
	var f rollbackFlags
	cmd := &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a Pending rollback (terminal)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, output := pickStore(&f, parent), pickOutput(&f, parent)
			e, closeStore, err := newRollbackEngine(d, store, "", "")
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()
			rb, err := e.RejectRollback(context.Background(), args[0], f.approver, f.reason)
			if err != nil {
				return err
			}
			return formatRollback(cmd.OutOrStdout(), output, rb)
		},
	}
	f.bindCommon(cmd)
	cmd.Flags().StringVar(&f.approver, "approver", "", "approver identity")
	cmd.Flags().StringVar(&f.reason, "reason", "", "reason for rejection")
	return cmd
}

func newGetCmd(d Deps, parent *rollbackFlags) *cobra.Command {
	var f rollbackFlags
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Print a stored rollback record",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, output := pickStore(&f, parent), pickOutput(&f, parent)
			e, closeStore, err := newRollbackEngine(d, store, "", "")
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()
			rb, ok, err := e.GetRollback(context.Background(), args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("rollback %q not found", args[0])
			}
			return formatRollback(cmd.OutOrStdout(), output, rb)
		},
	}
	f.bindCommon(cmd)
	return cmd
}

func newListCmd(d Deps, parent *rollbackFlags) *cobra.Command {
	var f rollbackFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored rollback records",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, output := pickStore(&f, parent), pickOutput(&f, parent)
			e, closeStore, err := newRollbackEngine(d, store, "", "")
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()
			list, err := e.ListRollbacks(context.Background())
			if err != nil {
				return err
			}
			return formatRollbackList(cmd.OutOrStdout(), output, list)
		},
	}
	f.bindCommon(cmd)
	return cmd
}

func pickStore(local, parent *rollbackFlags) string {
	if local.store != "" && local.store != "./.kscore-gitops.db" {
		return local.store
	}
	if strings.TrimSpace(parent.store) != "" {
		return parent.store
	}
	return "./.kscore-gitops.db"
}

func pickOutput(local, parent *rollbackFlags) string {
	if local.output != "" && local.output != "text" {
		return local.output
	}
	if parent.output != "" {
		return parent.output
	}
	return "text"
}
