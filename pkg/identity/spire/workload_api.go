package spire

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc"
)

// SPIFFE Workload API types and client implementation.
// This implements the client side of the SPIFFE Workload API specification.
// See: https://github.com/spiffe/spiffe/blob/main/standards/SPIFFE_Workload_API.md

// X509SVIDRequest is the request for FetchX509SVID.
type X509SVIDRequest struct{}

// X509SVIDResponse is the response from FetchX509SVID.
type X509SVIDResponse struct {
	SVIDs         []*X509SVIDData
	FederatedCA   map[string][]byte // trust_domain -> bundle
	CRL           []byte
	FederatedBdls map[string][]byte // trust_domain -> bundle (deprecated)
}

// X509SVIDData contains a single X.509 SVID.
type X509SVIDData struct {
	SPIFFEID    string // SPIFFE ID URI
	X509SVID    []byte // DER-encoded certificate chain
	X509SVIDKey []byte // DER-encoded private key
	Bundle      []byte // DER-encoded CA bundle
	Hint        string // Optional hint
}

// JWTSVIDRequest is the request for FetchJWTSVID.
type JWTSVIDRequest struct {
	Audience []string
	SpiffeID string // Optional: request SVID for specific ID
}

// JWTSVIDResponse is the response from FetchJWTSVID.
type JWTSVIDResponse struct {
	SVIDs []*JWTSVIDData
}

// JWTSVIDData contains a single JWT SVID.
type JWTSVIDData struct {
	SPIFFEID  string // SPIFFE ID URI
	Token     string // JWT token
	ExpiresAt int64  // Unix timestamp
	IssuedAt  int64  // Unix timestamp
	Hint      string // Optional hint
}

// X509BundleRequest is the request for FetchX509Bundles.
type X509BundleRequest struct{}

// X509BundleResponse is the response from FetchX509Bundles.
type X509BundleResponse struct {
	Bundles map[string][]byte // trust_domain -> DER-encoded bundle
}

// ValidateSVIDRequest is the request for ValidateJWTSVID.
type ValidateSVIDRequest struct {
	Audience string
	Token    string
}

// ValidateSVIDResponse is the response from ValidateJWTSVID.
type ValidateSVIDResponse struct {
	SPIFFEID string
	Claims   map[string]interface{}
}

// SpiffeWorkloadClient is a client for the SPIFFE Workload API.
type SpiffeWorkloadClient struct {
	conn *grpc.ClientConn
}

// NewSpiffeWorkloadClient creates a new Workload API client.
func NewSpiffeWorkloadClient(conn *grpc.ClientConn) *SpiffeWorkloadClient {
	return &SpiffeWorkloadClient{conn: conn}
}

// FetchX509SVID fetches X.509 SVIDs from the SPIRE Agent.
func (c *SpiffeWorkloadClient) FetchX509SVID(ctx context.Context) (*X509SVIDResponse, error) {
	stream, err := c.StreamX509SVID(ctx)
	if err != nil {
		return nil, err
	}

	// Get the first response
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive SVID: %w", err)
	}

	return resp, nil
}

// FetchJWTSVID fetches JWT SVIDs from the SPIRE Agent.
func (c *SpiffeWorkloadClient) FetchJWTSVID(ctx context.Context, audience []string) (*JWTSVIDResponse, error) {
	// Call the FetchJWTSVID RPC
	req := &JWTSVIDRequest{
		Audience: audience,
	}

	resp, err := c.doFetchJWTSVID(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// FetchX509Bundles fetches X.509 trust bundles from the SPIRE Agent.
func (c *SpiffeWorkloadClient) FetchX509Bundles(ctx context.Context) (*X509BundleResponse, error) {
	stream, err := c.StreamX509Bundles(ctx)
	if err != nil {
		return nil, err
	}

	// Get the first response
	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive bundle: %w", err)
	}

	return resp, nil
}

// ValidateJWTSVID validates a JWT SVID.
func (c *SpiffeWorkloadClient) ValidateJWTSVID(ctx context.Context, audience, token string) (*ValidateSVIDResponse, error) {
	req := &ValidateSVIDRequest{
		Audience: audience,
		Token:    token,
	}
	return c.doValidateJWTSVID(ctx, req)
}

// X509SVIDStream is a stream of X.509 SVID updates.
type X509SVIDStream interface {
	Recv() (*X509SVIDResponse, error)
	CloseSend() error
}

// X509BundleStream is a stream of X.509 bundle updates.
type X509BundleStream interface {
	Recv() (*X509BundleResponse, error)
	CloseSend() error
}

// StreamX509SVID returns a stream of X.509 SVID updates.
func (c *SpiffeWorkloadClient) StreamX509SVID(ctx context.Context) (X509SVIDStream, error) {
	// Create a gRPC client stream for the SpiffeWorkload/FetchX509SVID method
	stream, err := c.conn.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "FetchX509SVID",
		ServerStreams: true,
	}, "/SpiffeWorkloadAPI/FetchX509SVID")
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// Send the request
	if err := stream.SendMsg(&X509SVIDRequest{}); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return &x509SVIDStreamImpl{stream: stream}, nil
}

// StreamX509Bundles returns a stream of X.509 bundle updates.
func (c *SpiffeWorkloadClient) StreamX509Bundles(ctx context.Context) (X509BundleStream, error) {
	stream, err := c.conn.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "FetchX509Bundles",
		ServerStreams: true,
	}, "/SpiffeWorkloadAPI/FetchX509Bundles")
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	if err := stream.SendMsg(&X509BundleRequest{}); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return &x509BundleStreamImpl{stream: stream}, nil
}

// Stream implementations

type x509SVIDStreamImpl struct {
	stream grpc.ClientStream
}

func (s *x509SVIDStreamImpl) Recv() (*X509SVIDResponse, error) {
	resp := &X509SVIDResponse{}
	if err := s.stream.RecvMsg(resp); err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, fmt.Errorf("failed to receive message: %w", err)
	}
	return resp, nil
}

func (s *x509SVIDStreamImpl) CloseSend() error {
	return s.stream.CloseSend()
}

type x509BundleStreamImpl struct {
	stream grpc.ClientStream
}

func (s *x509BundleStreamImpl) Recv() (*X509BundleResponse, error) {
	resp := &X509BundleResponse{}
	if err := s.stream.RecvMsg(resp); err != nil {
		if err == io.EOF {
			return nil, err
		}
		return nil, fmt.Errorf("failed to receive message: %w", err)
	}
	return resp, nil
}

func (s *x509BundleStreamImpl) CloseSend() error {
	return s.stream.CloseSend()
}

// doFetchJWTSVID performs the FetchJWTSVID RPC.
func (c *SpiffeWorkloadClient) doFetchJWTSVID(ctx context.Context, req *JWTSVIDRequest) (*JWTSVIDResponse, error) {
	resp := &JWTSVIDResponse{}
	err := c.conn.Invoke(ctx, "/SpiffeWorkloadAPI/FetchJWTSVID", req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// doValidateJWTSVID performs the ValidateJWTSVID RPC.
func (c *SpiffeWorkloadClient) doValidateJWTSVID(ctx context.Context, req *ValidateSVIDRequest) (*ValidateSVIDResponse, error) {
	resp := &ValidateSVIDResponse{}
	err := c.conn.Invoke(ctx, "/SpiffeWorkloadAPI/ValidateJWTSVID", req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
