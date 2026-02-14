package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Config holds CLI configuration.
type Config struct {
	ServerAddr    string
	Timeout       time.Duration
	TLS           bool
	TLSCACert     string
	TLSCert       string
	TLSKey        string
	TLSSkipVerify bool
	TLSServerName string
	TLSMinVersion string
	OutputFormat  string
	Verbose      bool
}

// createEventClient creates a gRPC EventService client connection.
func createEventClient(cfg *Config) (pb.EventServiceClient, *grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var dialOpts []grpc.DialOption

	if cfg.TLS || cfg.TLSCACert != "" || cfg.TLSCert != "" {
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	dialOpts = append(dialOpts, grpc.WithBlock()) //nolint:staticcheck // SA1019: grpc.WithBlock is deprecated but supported throughout gRPC 1.x

	conn, err := grpc.DialContext(ctx, cfg.ServerAddr, dialOpts...) //nolint:staticcheck // SA1019: grpc.DialContext is deprecated but supported throughout gRPC 1.x
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to server at %s: %w", cfg.ServerAddr, err)
	}

	client := pb.NewEventServiceClient(conn)
	return client, conn, nil
}

// buildTLSConfig builds a TLS configuration from CLI flags.
func buildTLSConfig(cfg *Config) (*tls.Config, error) {
	minVersion, err := parseTLSMinVersion(cfg.TLSMinVersion)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{ // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- InsecureSkipVerify allowed only when KSCORE_ALLOW_INSECURE_TLS=1 is set for dev/test
		MinVersion: minVersion, // #nosec G402 -- validated to TLS 1.2+ defaults
	}

	if cfg.TLSSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("TLS skip verify requires KSCORE_ALLOW_INSECURE_TLS=1 for development/testing only")
		}
		fmt.Fprintln(os.Stderr, "WARNING: TLS certificate verification is disabled. This is insecure and should only be used for development.")
		tlsConfig.InsecureSkipVerify = true // #nosec G402 -- gated by KSCORE_ALLOW_INSECURE_TLS
	}

	if cfg.TLSServerName != "" {
		tlsConfig.ServerName = cfg.TLSServerName
	}

	if cfg.TLSCACert != "" {
		caCert, err := os.ReadFile(cfg.TLSCACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		if cfg.TLSCert == "" || cfg.TLSKey == "" {
			return nil, fmt.Errorf("both --tls-cert and --tls-key must be provided for mTLS")
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

func parseTLSMinVersion(value string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1.3", "tls1.3", "tls13":
		return tls.VersionTLS13, nil
	case "1.2", "tls1.2", "tls12":
		return tls.VersionTLS12, nil
	default:
		return 0, fmt.Errorf("unsupported TLS minimum version: %s", value)
	}
}

// severityToProto converts a severity string to proto enum.
func severityToProto(s string) pb.EventSeverity {
	switch strings.ToLower(s) {
	case "debug":
		return pb.EventSeverity_EVENT_SEVERITY_DEBUG
	case "info":
		return pb.EventSeverity_EVENT_SEVERITY_INFO
	case "warning":
		return pb.EventSeverity_EVENT_SEVERITY_WARNING
	case "error":
		return pb.EventSeverity_EVENT_SEVERITY_ERROR
	case "critical":
		return pb.EventSeverity_EVENT_SEVERITY_CRITICAL
	default:
		return pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED
	}
}

// protoToSeverity converts a proto severity enum to a string.
func protoToSeverity(s pb.EventSeverity) string {
	switch s {
	case pb.EventSeverity_EVENT_SEVERITY_DEBUG:
		return "debug"
	case pb.EventSeverity_EVENT_SEVERITY_INFO:
		return "info"
	case pb.EventSeverity_EVENT_SEVERITY_WARNING:
		return "warning"
	case pb.EventSeverity_EVENT_SEVERITY_ERROR:
		return "error"
	case pb.EventSeverity_EVENT_SEVERITY_CRITICAL:
		return "critical"
	default:
		return "unknown"
	}
}
