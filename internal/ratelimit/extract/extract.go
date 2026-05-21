package extract

import (
	"context"
	"net/http"
)

// Extractor returns a rate-limit key for an inbound request.
// Each concrete extractor implements both transport methods so
// the middleware can call the matching one without branching on
// the extractor type.
type Extractor interface {
	// HTTP returns the key for an HTTP request. ok=false means
	// no usable signal (header absent, RemoteAddr unset, etc.).
	HTTP(r *http.Request) (string, bool)

	// GRPC returns the key for a gRPC call. ok=false means no
	// usable signal (peer missing, metadata absent, etc.).
	GRPC(ctx context.Context) (string, bool)
}

// Chain returns an Extractor that consults each child in order
// and returns the first hit. With zero children it always
// returns ("", false). Use Chain for fallback policies like
// "API key if authenticated, else IP for anonymous":
//
//	rl := extract.Chain(extract.APIKey(), extract.IP(extract.IPConfig{}))
type Chain []Extractor

// HTTP runs the chain against an HTTP request.
func (c Chain) HTTP(r *http.Request) (string, bool) {
	for _, e := range c {
		if k, ok := e.HTTP(r); ok {
			return k, true
		}
	}
	return "", false
}

// GRPC runs the chain against a gRPC call.
func (c Chain) GRPC(ctx context.Context) (string, bool) {
	for _, e := range c {
		if k, ok := e.GRPC(ctx); ok {
			return k, true
		}
	}
	return "", false
}
