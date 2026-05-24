// SPDX-License-Identifier: Apache-2.0

package gitops

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"go.keystone-core.io/keystone-core/internal/gitops/verification"
)

// stepDoc / workflowDoc are the YAML on-disk shape of a workflow
// file. They map directly onto [verification.Workflow] / [Step] —
// Config is passed through as map[string]any so YAML types reach the
// verifiers via the existing cfg* accessors.
type stepDoc struct {
	Name     string         `yaml:"name"`
	Type     string         `yaml:"type"`
	Optional bool           `yaml:"optional,omitempty"`
	Timeout  string         `yaml:"timeout,omitempty"`
	Retries  int            `yaml:"retries,omitempty"`
	Config   map[string]any `yaml:"config,omitempty"`
}

type workflowDoc struct {
	Name        string    `yaml:"name"`
	Parallel    bool      `yaml:"parallel,omitempty"`
	Timeout     string    `yaml:"timeout,omitempty"`
	OnFailure   string    `yaml:"on_failure,omitempty"`
	MaxParallel int       `yaml:"max_parallel,omitempty"`
	Steps       []stepDoc `yaml:"steps"`
}

// LoadWorkflow reads a YAML workflow file from path and converts it
// to a [verification.Workflow] with its steps. Exported for tests.
func LoadWorkflow(path string) (verification.Workflow, error) {
	// G304: the path is the operator-supplied workflow file passed as
	// a CLI positional argument; reading it is the only purpose of
	// `verify <workflow-file>`.
	//nolint:gosec
	raw, err := os.ReadFile(path)
	if err != nil {
		return verification.Workflow{}, fmt.Errorf("read %s: %w", path, err)
	}
	var doc workflowDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return verification.Workflow{}, fmt.Errorf("parse %s: %w", path, err)
	}
	wf := verification.Workflow{
		Name:        doc.Name,
		Parallel:    doc.Parallel,
		OnFailure:   verification.FailurePolicy(doc.OnFailure),
		MaxParallel: doc.MaxParallel,
	}
	if doc.Timeout != "" {
		d, err := time.ParseDuration(doc.Timeout)
		if err != nil {
			return verification.Workflow{}, fmt.Errorf("workflow timeout %q: %w", doc.Timeout, err)
		}
		wf.Timeout = d
	}
	for i, s := range doc.Steps {
		step := verification.Step{
			Name:     s.Name,
			Type:     s.Type,
			Optional: s.Optional,
			Retries:  s.Retries,
			Config:   s.Config,
		}
		if s.Timeout != "" {
			d, err := time.ParseDuration(s.Timeout)
			if err != nil {
				return verification.Workflow{}, fmt.Errorf("step[%d] %s timeout %q: %w", i, s.Name, s.Timeout, err)
			}
			step.Timeout = d
		}
		wf.Steps = append(wf.Steps, step)
	}
	return wf, nil
}

func newVerifyCmd(d Deps) *cobra.Command {
	var (
		parallel bool
		timeout  time.Duration
		output   string
	)
	cmd := &cobra.Command{
		Use:   "verify <workflow-file>",
		Short: "Run a GitOps verification workflow",
		Long:  "Loads a YAML workflow and runs each step (HTTP / gRPC / command) via the local verification engine.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wf, err := LoadWorkflow(args[0])
			if err != nil {
				return err
			}
			// CLI overrides (acceptance line 104: --parallel --timeout 2m).
			if cmd.Flags().Changed("parallel") {
				wf.Parallel = parallel
			}
			if cmd.Flags().Changed("timeout") {
				wf.Timeout = timeout
			}

			httpClient := d.HTTPClient
			if httpClient == nil {
				httpClient = &http.Client{Timeout: 30 * time.Second}
			}
			reg := verification.NewDefaultRegistry(verification.Deps{
				HTTPClient:    httpClient,
				HealthCheck:   d.HealthCheck,
				CommandRunner: d.CmdRunner,
			})

			result := verification.NewEngine(reg).Run(context.Background(), wf)
			if err := formatWorkflowResult(cmd.OutOrStdout(), output, result); err != nil {
				return err
			}
			if !result.Success {
				return fmt.Errorf("verification failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&parallel, "parallel", false, "run steps in parallel (overrides workflow file)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "overall workflow timeout, e.g. 2m (overrides workflow file)")
	cmd.Flags().StringVar(&output, "output", "text", "output format: text|json")
	return cmd
}
