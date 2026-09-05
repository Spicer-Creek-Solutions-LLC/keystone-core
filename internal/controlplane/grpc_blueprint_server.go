// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Blueprint provider interfaces — independently nilable; an RPC whose
// provider is missing returns codes.Unavailable (the established
// grpc-server precedent; boot wiring is the deferred server
// composition, see ROADMAP "Remote / distributed blueprint apply
// wiring").
type (
	blueprintCatalog interface {
		List(ctx context.Context) ([]*bp.Manifest, error)
		Get(ctx context.Context, name string) (*bp.Manifest, error)
	}
	blueprintApplier interface {
		Apply(ctx context.Context, name string, opts bp.ApplyOptions) (*bp.ApplyResult, error)
	}
)

// BlueprintGRPCServer implements v1.BlueprintServiceServer (Epic 15
// task 12) — minimal query + apply. Parameter VALUES are never put
// on the wire (only names), so secret/sensitive inputs cannot leak.
type BlueprintGRPCServer struct {
	v1.UnimplementedBlueprintServiceServer

	Catalog blueprintCatalog
	Applier blueprintApplier

	// Resolver turns a request's Target into agent records. Nil means
	// a targeted apply is refused rather than silently applied to the
	// control-plane host -- converging the wrong machine is worse than
	// failing.
	Resolver AgentTargetResolver
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func blueprintToProto(m *bp.Manifest) *v1.BlueprintSummary {
	eps := []string{}
	if m.Entrypoints.Default != "" {
		eps = append(eps, "default")
	}
	if m.Entrypoints.Rollback != "" {
		eps = append(eps, "rollback")
	}
	eps = append(eps, sortedKeys(m.Entrypoints.Named)...)
	return &v1.BlueprintSummary{
		Name:        m.Metadata.Name,
		Version:     m.Metadata.Version,
		Description: m.Metadata.Description,
		Parameters:  sortedKeys(m.Parameters),
		Features:    sortedKeys(m.Features),
		Entrypoints: eps,
	}
}

// ListBlueprints returns every blueprint's summary.
func (s *BlueprintGRPCServer) ListBlueprints(ctx context.Context, _ *v1.ListBlueprintsRequest) (*v1.ListBlueprintsResponse, error) {
	if s.Catalog == nil {
		return nil, status.Error(codes.Unavailable, "blueprint: catalog not wired")
	}
	ms, err := s.Catalog.List(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	out := make([]*v1.BlueprintSummary, 0, len(ms))
	for _, m := range ms {
		out = append(out, blueprintToProto(m))
	}
	return &v1.ListBlueprintsResponse{Blueprints: out, TotalCount: int32(len(out))}, nil
}

// GetBlueprint returns one blueprint's summary.
func (s *BlueprintGRPCServer) GetBlueprint(ctx context.Context, req *v1.GetBlueprintRequest) (*v1.GetBlueprintResponse, error) {
	if s.Catalog == nil {
		return nil, status.Error(codes.Unavailable, "blueprint: catalog not wired")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	m, err := s.Catalog.Get(ctx, req.GetName())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &v1.GetBlueprintResponse{Blueprint: blueprintToProto(m)}, nil
}

// ApplyBlueprint applies a blueprint. A run that completes but ends
// failed is returned in the response with status="failed" (not a
// gRPC error); only a setup error with no result is codes.Internal.
func (s *BlueprintGRPCServer) ApplyBlueprint(ctx context.Context, req *v1.ApplyBlueprintRequest) (*v1.ApplyBlueprintResponse, error) {
	if s.Applier == nil {
		return nil, status.Error(codes.Unavailable, "blueprint: applier not wired")
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	opts := bp.ApplyOptions{
		Inputs:     req.GetParams(),
		Enable:     req.GetEnable(),
		Disable:    req.GetDisable(),
		As:         req.GetAs(),
		Entrypoint: req.GetEntrypoint(),
	}
	if t := req.GetTarget(); t != nil {
		agents, err := s.resolveBlueprintTargets(ctx, t)
		if err != nil {
			return nil, err
		}
		opts.Agents = agents
		opts.Target = describeTarget(t)
	}

	res, err := s.Applier.Apply(ctx, req.GetName(), opts)
	if res == nil {
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return nil, status.Error(codes.Internal, "blueprint: apply returned no result")
	}
	resp := &v1.ApplyBlueprintResponse{
		RunId:   res.RunID,
		Status:  res.Status,
		Outputs: stringifyMap(res.Outputs),
	}
	if res.Report != nil {
		resp.Report = &v1.ApplyReport{
			Total:   int32(res.Report.Total),
			Changed: int32(res.Report.Changed),
			Failed:  int32(res.Report.Failed),
		}
	}
	// A non-nil err here is the ran-but-failed signal (ErrApplyFailed);
	// the outcome is carried by resp.Status so the caller still gets
	// the run id + report rather than an opaque gRPC error.
	return resp, nil
}

func stringifyMap(m map[string]any) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// resolveBlueprintTargets turns a request Target into agent ids,
// through the same resolver a targeted `state apply` uses so both
// commands agree on what an expression selects.
//
// A target that matches nothing is an error, not an empty success. An
// operator who wrote `--target role:web` and has no web hosts has made
// a mistake worth hearing about; reporting "applied, 0 changed" would
// hide it.
func (s *BlueprintGRPCServer) resolveBlueprintTargets(ctx context.Context, t *v1.Target) ([]string, error) {
	if s.Resolver == nil {
		return nil, status.Error(codes.Unavailable,
			"blueprint: remote apply is not wired on this server")
	}
	records, err := s.Resolver.Resolve(ctx, targetFromProto(t))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if len(records) == 0 {
		return nil, status.Error(codes.NotFound, "blueprint: target matched no agents")
	}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

// describeTarget renders a Target back to the expression form an
// operator would have typed, for the AppliedRun record. It is
// descriptive only -- rollback uses the recorded agent ids, never a
// re-resolution of this string.
func describeTarget(t *v1.Target) string {
	switch {
	case t == nil:
		return ""
	case len(t.GetAgentIds()) > 0:
		return "id:" + strings.Join(t.GetAgentIds(), ",")
	case len(t.GetLabels()) > 0:
		parts := make([]string, 0, len(t.GetLabels()))
		for _, k := range sortedKeys(t.GetLabels()) {
			parts = append(parts, k+":"+t.GetLabels()[k])
		}
		return strings.Join(parts, ",")
	case t.GetHostnamePattern() != "":
		return "hostname:" + t.GetHostnamePattern()
	default:
		return ""
	}
}
