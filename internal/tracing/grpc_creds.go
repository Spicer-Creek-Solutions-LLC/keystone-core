// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"crypto/tls"

	"google.golang.org/grpc/credentials"
)

// credentialsTransport is a tiny alias so exporters.go can keep
// credentials.TransportCredentials out of its surface.
type credentialsTransport = credentials.TransportCredentials

// newGRPCTransportTLS builds gRPC TransportCredentials from a *tls.Config.
// Isolated here so the credentials import stays out of exporters.go.
func newGRPCTransportTLS(cfg *tls.Config) credentials.TransportCredentials {
	return credentials.NewTLS(cfg)
}
