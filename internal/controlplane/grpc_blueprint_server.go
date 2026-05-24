// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"fmt"
	"sort"

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
	res, err := s.Applier.Apply(ctx, req.GetName(), bp.ApplyOptions{
		Inputs:     req.GetParams(),
		Enable:     req.GetEnable(),
		Disable:    req.GetDisable(),
		As:         req.GetAs(),
		Entrypoint: req.GetEntrypoint(),
	})
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
