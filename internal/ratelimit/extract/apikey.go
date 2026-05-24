// SPDX-License-Identifier: Apache-2.0

package extract

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// APIKey returns an [Extractor] keyed by the SHA-256 hash of the
// inbound Bearer token. The cleartext token never reaches the
// rate-limit bucket map — only the hash does — so a memory dump
// or rate-limit log line cannot leak credentials.
//
// Both transports look at the conventional Authorization header
// (HTTP) / authorization metadata (gRPC). The header is parsed
// the same way [pkg/api/auth.extractBearerToken] handles it: the
// "Bearer " prefix is optional for backwards compatibility with
// kscorectl scripts that send bare tokens.
func APIKey() Extractor {
	return apiKeyExtractor{}
}

type apiKeyExtractor struct{}

func (apiKeyExtractor) HTTP(r *http.Request) (string, bool) {
	token := bearerFromHeader(r.Header.Get("Authorization"))
	if token == "" {
		return "", false
	}
	return auth.HashAPIKey(token), true
}

func (apiKeyExtractor) GRPC(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", false
	}
	token := bearerFromHeader(values[0])
	if token == "" {
		return "", false
	}
	return auth.HashAPIKey(token), true
}

// bearerFromHeader returns the bearer value with the "Bearer "
// prefix stripped (case-insensitive). Bare tokens (no prefix)
// pass through trimmed. Empty string on no value.
func bearerFromHeader(header string) string {
	if header == "" {
		return ""
	}
	const bearerPrefix = "Bearer "
	// Case-insensitive prefix check — RFC 6750 names "Bearer"
	// case-insensitively though Go's standard libraries emit
	// the canonical form. Be lenient on input.
	if len(header) >= len(bearerPrefix) && strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(header[len(bearerPrefix):])
	}
	return strings.TrimSpace(header)
}
