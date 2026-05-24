// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"net"
	"net/http"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// injectAuthHeaderAsMetadata wraps the inbound HTTP Authorization
// header in gRPC metadata so the same APIKey/JWT extractors work for
// both transports.
func injectAuthHeaderAsMetadata(ctx context.Context, authHeader string) context.Context {
	if authHeader == "" {
		return ctx
	}
	md := metadata.Pairs("authorization", authHeader)
	return metadata.NewIncomingContext(ctx, md)
}

// injectTLSStateAsPeerInfo wraps the HTTP request's TLS state in a
// gRPC peer.Peer so MTLSAuthenticator's peer.FromContext +
// VerifiedChains lookup works for HTTP requests too.
func injectTLSStateAsPeerInfo(ctx context.Context, r *http.Request) context.Context {
	if r.TLS == nil {
		return ctx
	}
	addr := tcpAddrFromString(trimSchemeFromAddr(r.RemoteAddr))
	authInfo := credentials.TLSInfo{State: *r.TLS}
	return peer.NewContext(ctx, &peer.Peer{
		Addr:     addr,
		AuthInfo: authInfo,
	})
}

// tcpAddrFromString parses host:port strings into a net.Addr. Returns
// nil on parse failure (peer.Addr may legitimately be nil).
func tcpAddrFromString(s string) net.Addr {
	if s == "" {
		return nil
	}
	addr, err := net.ResolveTCPAddr("tcp", s)
	if err != nil {
		return nil
	}
	return addr
}
