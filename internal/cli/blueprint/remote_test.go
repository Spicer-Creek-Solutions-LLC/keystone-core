// SPDX-License-Identifier: Apache-2.0

package blueprint_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc"

	clibp "go.keystone-core.io/keystone-core/internal/cli/blueprint"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// fakeBlueprintClient records the request the CLI sends and returns a
// scripted response.
type fakeBlueprintClient struct {
	v1.BlueprintServiceClient
	got  *v1.ApplyBlueprintRequest
	resp *v1.ApplyBlueprintResponse
	err  error
}

func (f *fakeBlueprintClient) ApplyBlueprint(_ context.Context, req *v1.ApplyBlueprintRequest, _ ...grpc.CallOption) (*v1.ApplyBlueprintResponse, error) {
	f.got = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func depsFor(c *fakeBlueprintClient) clibp.Deps {
	return clibp.Deps{
		Server: "127.0.0.1:5397",
		Dial: func(context.Context, string, string) (v1.BlueprintServiceClient, io.Closer, error) {
			return c, nopCloser{}, nil
		},
	}
}

func okResp() *v1.ApplyBlueprintResponse {
	return &v1.ApplyBlueprintResponse{
		RunId: "run-1", Status: "succeeded",
		Report: &v1.ApplyReport{Total: 2, Changed: 2},
	}
}

// A targeted apply must reach the control plane, carrying the target
// and the blueprint NAME -- the server applies from its own catalog.
func TestRemoteApply_SendsTargetAndName(t *testing.T) {
	c := &fakeBlueprintClient{resp: okResp()}
	out, _, err := runCLI(depsFor(c), "apply", "demo",
		"--target", "id:web-1", "--param", "port=8080", "--as", "blue")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if c.got == nil {
		t.Fatal("the CLI never called the control plane")
	}
	if c.got.GetName() != "demo" {
		t.Errorf("name = %q, want %q", c.got.GetName(), "demo")
	}
	if ids := c.got.GetTarget().GetAgentIds(); len(ids) != 1 || ids[0] != "web-1" {
		t.Errorf("target agent ids = %v, want [web-1]", ids)
	}
	if c.got.GetParams()["port"] != "8080" {
		t.Errorf("params = %v, want port=8080", c.got.GetParams())
	}
	if c.got.GetAs() != "blue" {
		t.Errorf("as = %q, want %q", c.got.GetAs(), "blue")
	}
	if !strings.Contains(out, "run-1") || !strings.Contains(out, "succeeded") {
		t.Errorf("output does not report the run:\n%s", out)
	}
}

// A run that completed but ended failed still has to exit non-zero, or
// a failed fleet apply looks successful to a script.
func TestRemoteApply_FailedRunIsAnError(t *testing.T) {
	c := &fakeBlueprintClient{resp: &v1.ApplyBlueprintResponse{
		RunId: "run-2", Status: "failed",
		Report: &v1.ApplyReport{Total: 2, Failed: 1},
	}}
	out, _, err := runCLI(depsFor(c), "apply", "demo", "--target", "id:web-1")
	if err == nil {
		t.Fatal("a failed run exited zero")
	}
	// The report is still printed: an operator needs to see what
	// happened, not only that something did.
	if !strings.Contains(out, "run-2") {
		t.Errorf("a failed run printed no report:\n%s", out)
	}
}

func TestRemoteApply_TransportErrorSurfaces(t *testing.T) {
	c := &fakeBlueprintClient{err: errors.New("unavailable")}
	if _, _, err := runCLI(depsFor(c), "apply", "demo", "--target", "id:web-1"); err == nil {
		t.Fatal("a transport failure exited zero")
	}
}

// A local apply must not dial anything -- the whole point of the
// distinction is that one runs here and the other does not.
func TestRemoteApply_LocalTargetDoesNotDial(t *testing.T) {
	c := &fakeBlueprintClient{resp: okResp()}
	d := depsFor(c)
	// No executor wired, so a local apply fails -- but it must fail
	// locally rather than by reaching the control plane.
	_, _, err := runCLI(d, "apply", t.TempDir(), "--target", "localhost")
	if err == nil {
		t.Fatal("expected the local path to fail with no executor")
	}
	if c.got != nil {
		t.Error("a localhost apply dialled the control plane")
	}
}

// A targeted apply needs the blueprint name, not a directory: the
// server applies from its catalog and cannot see this machine's disk.
func TestRemoteApply_RequiresAName(t *testing.T) {
	c := &fakeBlueprintClient{resp: okResp()}
	_, _, err := runCLI(depsFor(c), "apply", "--target", "id:web-1")
	if err == nil {
		t.Fatal("a targeted apply with no name succeeded")
	}
	if c.got != nil {
		t.Error("the CLI dialled without a blueprint name")
	}
}
