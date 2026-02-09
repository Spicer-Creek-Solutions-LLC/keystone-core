package gnmi

import "context"

// metadataCredentials implements grpc credentials.PerRPCCredentials
// for username/password authentication via gRPC metadata headers.
type metadataCredentials struct {
	username string
	password string
}

// GetRequestMetadata returns the metadata to attach to each RPC.
func (c *metadataCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"username": c.username,
		"password": c.password,
	}, nil
}

// RequireTransportSecurity indicates that transport security is required.
func (c *metadataCredentials) RequireTransportSecurity() bool {
	return true
}
