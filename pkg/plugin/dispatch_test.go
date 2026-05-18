package plugin_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/pkg/plugin"
)

type fakeExec struct {
	called bool
	gotP   plugin.Plugin
	gotARG []string
	code   int
}

func (f *fakeExec) Execute(_ context.Context, p plugin.Plugin, args []string, _ io.Reader, _, _ io.Writer) (int, error) {
	f.called = true
	f.gotP = p
	f.gotARG = args
	return f.code, nil
}

func testRoot() *cobra.Command {
	root := &cobra.Command{Use: "kscorectl", SilenceErrors: true, SilenceUsage: true,
		Run: func(*cobra.Command, []string) {}}
	root.AddCommand(&cobra.Command{Use: "exec", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "state", Run: func(*cobra.Command, []string) {}})
	return root
}

func TestDispatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kscore-module"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	cases := []struct {
		name     string
		args     []string
		wantHand bool
		wantArgs []string
	}{
		{"unknown-with-plugin", []string{"module", "install", "x@1"}, true, []string{"install", "x@1"}},
		{"registered subcommand", []string{"exec", "run"}, false, nil},
		{"another registered", []string{"state", "apply"}, false, nil},
		{"unknown-no-plugin", []string{"ghost", "x"}, false, nil},
		{"root flag", []string{"--help"}, false, nil},
		{"version flag", []string{"--version"}, false, nil},
		{"help builtin", []string{"help"}, false, nil},
		{"empty", nil, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := plugin.New("")
			fe := &fakeExec{code: 7}
			handled, code, err := plugin.Dispatch(context.Background(), testRoot(), tc.args,
				d, fe, nil, nil, nil)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if handled != tc.wantHand {
				t.Fatalf("handled = %v, want %v", handled, tc.wantHand)
			}
			if tc.wantHand {
				if code != 7 || !fe.called || fe.gotP.Name != "module" {
					t.Fatalf("dispatch = code %d called %v plugin %q", code, fe.called, fe.gotP.Name)
				}
				if len(fe.gotARG) != len(tc.wantArgs) {
					t.Fatalf("plugin args = %v, want %v", fe.gotARG, tc.wantArgs)
				}
				for i := range tc.wantArgs {
					if fe.gotARG[i] != tc.wantArgs[i] {
						t.Fatalf("plugin args = %v, want %v", fe.gotARG, tc.wantArgs)
					}
				}
			} else if fe.called {
				t.Fatal("executor must NOT run for a non-dispatched invocation")
			}
		})
	}
}
