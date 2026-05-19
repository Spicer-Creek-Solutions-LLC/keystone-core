package verification

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// HealthChecker performs one gRPC health check against target for the
// given service name (empty = server-wide). It returns the
// stringified serving status (e.g. "SERVING", "NOT_SERVING") or an
// error if the check could not be performed. Injectable so the
// verifier is testable without standing up a real server.
type HealthChecker func(ctx context.Context, target, service string, useTLS bool) (string, error)

// GRPCVerifier checks a gRPC endpoint via the standard
// grpc.health.v1.Health/Check protocol. Config:
//
//	target  (required) host:port
//	service (string)   health service name; "" = server-wide
//	tls     (bool)     use TLS (system roots); default insecure
//
// Check is injectable for tests; nil uses [defaultHealthCheck].
type GRPCVerifier struct {
	Check HealthChecker
}

// Type implements [Verifier].
func (GRPCVerifier) Type() string { return "grpc" }

// Verify implements [Verifier]. Success when the health check returns
// SERVING for the requested service.
func (v GRPCVerifier) Verify(ctx context.Context, step Step) Result {
	start := time.Now()

	target, err := cfgString(step.Config, "target")
	if err != nil {
		return failf(start, err, "grpc: %v", err)
	}
	service := cfgStringOpt(step.Config, "service", "")
	useTLS := cfgBoolOpt(step.Config, "tls", false)

	check := v.Check
	if check == nil {
		check = defaultHealthCheck
	}
	status, err := check(ctx, target, service, useTLS)
	if err != nil {
		return failf(start, err, "grpc: health check failed: %v", err)
	}
	data := map[string]any{"serving_status": status}
	if status != grpc_health_v1.HealthCheckResponse_SERVING.String() {
		r := failf(start, nil, "grpc: %s is %s, want SERVING", targetLabel(target, service), status)
		r.Data = data
		return r
	}
	return Result{
		Success:  true,
		Message:  fmt.Sprintf("grpc: %s is SERVING", targetLabel(target, service)),
		Data:     data,
		Duration: time.Since(start),
	}
}

func targetLabel(target, service string) string {
	if service == "" {
		return target
	}
	return target + "/" + service
}

// defaultHealthCheck dials target with the existing grpc dependency
// (no new module) via the non-deprecated grpc.NewClient, then calls
// the standard health service. The connection is lazy; Check drives
// the dial under ctx's deadline.
func defaultHealthCheck(ctx context.Context, target, service string, useTLS bool) (string, error) {
	var creds credentials.TransportCredentials
	if useTLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		creds = insecure.NewCredentials()
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", target, err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx,
		&grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		return "", err
	}
	return resp.GetStatus().String(), nil
}
