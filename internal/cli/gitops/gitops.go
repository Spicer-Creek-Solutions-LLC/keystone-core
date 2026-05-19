package gitops

import (
	"net/http"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/gitops/rollback"
	"go.keystone-core.io/keystone-core/internal/gitops/verification"
)

// Deps bundles the production seams the gitops CLI needs. Production
// `main` constructs them with the real implementations; tests inject
// fakes. The `kscore-server` boot path will inject the same set when
// the gate-v1.0 "GitOps rollback boot wiring" ROADMAP item lands.
type Deps struct {
	HTTPClient    *http.Client
	CmdRunner     verification.CommandRunner
	HealthCheck   verification.HealthChecker
	GitClient     rollback.GitClient
	NewArgoClient func(server, token string) rollback.ArgoClient
	K8sClient     rollback.K8sRolloutClient
}

// NewCommand returns the root `kscore-gitops` cobra command. It is
// also the entrypoint reached as `kscorectl gitops …` via the
// Epic-14 plugin dispatch.
func NewCommand(d Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "kscore-gitops",
		Short:         "GitOps verification + rollback CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVerifyCmd(d))
	root.AddCommand(newRollbackCmd(d))
	return root
}
