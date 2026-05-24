// SPDX-License-Identifier: Apache-2.0

package extract

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/grpc/metadata"
)

// Header returns an [Extractor] keyed by the value of the named
// header. headerName is case-insensitive on HTTP (Go normalises
// to canonical Title-Case on read). For gRPC the equivalent
// metadata key is the lowercased form because gRPC normalises
// metadata keys to lowercase per the HTTP/2 spec; the extractor
// lowercases the operator-supplied name automatically.
//
// An empty value returns ("", false) — operators get the
// "no key" outcome consistently across transports.
func Header(headerName string) Extractor {
	return headerExtractor{name: headerName, gRPCKey: strings.ToLower(headerName)}
}

type headerExtractor struct {
	name    string
	gRPCKey string
}

func (e headerExtractor) HTTP(r *http.Request) (string, bool) {
	v := strings.TrimSpace(r.Header.Get(e.name))
	if v == "" {
		return "", false
	}
	return v, true
}

func (e headerExtractor) GRPC(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	values := md.Get(e.gRPCKey)
	if len(values) == 0 {
		return "", false
	}
	v := strings.TrimSpace(values[0])
	if v == "" {
		return "", false
	}
	return v, true
}
