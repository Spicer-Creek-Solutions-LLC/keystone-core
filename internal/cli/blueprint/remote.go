// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"go.keystone-core.io/keystone-core/internal/cli/target"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// dialBlueprintService is the production dialer for a targeted apply.
//
// A remote apply cannot run in this process: the control plane holds
// the converge dispatcher and the agent registry, so the CLI asks it
// to do the work rather than reaching agents itself. Insecure for the
// same reason the state CLI is -- TLS plumbing belongs with identity.
func dialBlueprintService(_ context.Context, server, _ string) (v1.BlueprintServiceClient, io.Closer, error) {
	if server == "" {
		return nil, nil, fmt.Errorf("blueprint: --server is required for a targeted apply")
	}
	conn, err := grpc.NewClient(server,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("blueprint: dial %s: %w", server, err)
	}
	return v1.NewBlueprintServiceClient(conn), conn, nil
}

// authContext attaches the API key to outbound gRPC metadata, falling
// back to KSCORE_API_KEY. An empty key leaves the context untouched so
// an unauthenticated server still works in development.
func authContext(ctx context.Context, apiKey string) context.Context {
	if apiKey == "" {
		apiKey = os.Getenv("KSCORE_API_KEY")
	}
	if apiKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
}

// remoteApplyArgs carries one targeted apply's inputs. Grouped rather
// than passed positionally because eight string parameters in a row is
// a transposition waiting to happen.
type remoteApplyArgs struct {
	name       string
	target     string
	inputs     map[string]string
	enable     []string
	disable    []string
	as         string
	entrypoint string
	server     string
	apiKey     string
}

// runRemoteApply asks the control plane to apply a blueprint across a
// target.
//
// The blueprint is named, not uploaded: the server applies from its own
// catalog. That keeps one source of truth for what a blueprint contains
// -- an operator cannot apply a locally-edited copy to a fleet without
// it going through the catalog first, which is where review and version
// pinning live.
func runRemoteApply(cmd *cobra.Command, d Deps, a remoteApplyArgs) error {
	if a.name == "" {
		return errors.New("blueprint: a targeted apply needs the blueprint NAME, not a directory")
	}
	tgt, err := target.ParseTarget(a.target)
	if err != nil {
		return err
	}

	server := a.server
	if server == "" {
		server = d.Server
	}
	if server == "" {
		server = os.Getenv("KSCORE_SERVER")
	}
	apiKey := a.apiKey
	if apiKey == "" {
		apiKey = d.APIKey
	}

	ctx := withContext(cmd)
	client, closer, err := d.dial()(ctx, server, apiKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.ApplyBlueprint(authContext(ctx, apiKey), &v1.ApplyBlueprintRequest{
		Name:       a.name,
		Params:     a.inputs,
		Enable:     a.enable,
		Disable:    a.disable,
		As:         a.as,
		Entrypoint: a.entrypoint,
		Target:     tgt,
	})
	if err != nil {
		return err
	}
	printRemoteResult(cmd, a.name, a.target, resp)
	// A run that completed but ended failed is not a transport error;
	// it still has to exit non-zero, or a failed fleet apply looks
	// successful to a script.
	if resp.GetStatus() == "failed" {
		return fmt.Errorf("blueprint %q: apply failed", a.name)
	}
	return nil
}

// printRemoteResult renders the server's response.
func printRemoteResult(cmd *cobra.Command, name, tgt string, resp *v1.ApplyBlueprintResponse) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "blueprint %s → %s\n", name, tgt)
	fmt.Fprintf(out, "  run:    %s\n", resp.GetRunId())
	fmt.Fprintf(out, "  status: %s\n", resp.GetStatus())
	if r := resp.GetReport(); r != nil {
		fmt.Fprintf(out, "  total=%d changed=%d failed=%d\n",
			r.GetTotal(), r.GetChanged(), r.GetFailed())
	}
	for _, k := range sortedOutputKeys(resp.GetOutputs()) {
		fmt.Fprintf(out, "  output %s=%s\n", k, resp.GetOutputs()[k])
	}
}

func sortedOutputKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// blueprintName reads the blueprint NAME from the positional argument.
//
// A local apply takes a directory; a targeted apply takes a name,
// because the server applies from its own catalog rather than from
// whatever is on this machine. The argument is spelled the same either
// way, so this exists to make the difference explicit at the one call
// site where it matters.
func blueprintName(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// remoteRollbackWanted reports whether a rollback should be asked of
// the control plane rather than run here.
//
// The discriminator is an explicit server address, not a guess. A run
// applied to a fleet is recorded on the control plane and this process
// has no record of it, so there is no way to infer the right
// destination from the run id alone -- ids are opaque and the two
// stores are separate.
func remoteRollbackWanted(d Deps, server string) bool {
	return server != "" || d.Server != "" || os.Getenv("KSCORE_SERVER") != ""
}

// runRemoteRollback asks the control plane to revert a recorded run.
//
// No target is sent: which hosts to revert on comes from the run
// record on the server, so a rollback cannot be pointed at a different
// fleet than the apply it reverts.
func runRemoteRollback(cmd *cobra.Command, d Deps, runID, server, apiKey string) error {
	if server == "" {
		server = d.Server
	}
	if server == "" {
		server = os.Getenv("KSCORE_SERVER")
	}
	if apiKey == "" {
		apiKey = d.APIKey
	}

	ctx := withContext(cmd)
	client, closer, err := d.dial()(ctx, server, apiKey)
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.RollbackBlueprint(authContext(ctx, apiKey), &v1.RollbackBlueprintRequest{
		RunId: runID,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "rollback of %s\n", runID)
	fmt.Fprintf(out, "  run:    %s\n", resp.GetRunId())
	fmt.Fprintf(out, "  status: %s\n", resp.GetStatus())
	if r := resp.GetReport(); r != nil {
		fmt.Fprintf(out, "  total=%d changed=%d failed=%d\n",
			r.GetTotal(), r.GetChanged(), r.GetFailed())
	}
	if resp.GetStatus() == "failed" {
		return fmt.Errorf("blueprint rollback %s: failed", runID)
	}
	return nil
}
