package secrets

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sentinel errors returned by the secrets client.
var (
	ErrSecretNotFound     = errors.New("secret not found")
	ErrAccessDenied       = errors.New("access denied")
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrBackendUnavailable = errors.New("backend unavailable")
	ErrLeaseExpired       = errors.New("lease expired")
	ErrAlreadyExists      = errors.New("already exists")
	ErrUnauthenticated    = errors.New("unauthenticated")
	ErrUnimplemented      = errors.New("not implemented")
	ErrResourceExhausted  = errors.New("resource exhausted")
)

// grpcStatusToError converts a gRPC status error to a public sentinel error.
// Non-gRPC errors are returned as-is.
func grpcStatusToError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	switch st.Code() {
	case codes.OK:
		return nil
	case codes.NotFound:
		return ErrSecretNotFound
	case codes.PermissionDenied:
		return ErrAccessDenied
	case codes.InvalidArgument:
		return ErrInvalidArgument
	case codes.Unavailable:
		return ErrBackendUnavailable
	case codes.FailedPrecondition:
		return ErrLeaseExpired
	case codes.AlreadyExists:
		return ErrAlreadyExists
	case codes.Unauthenticated:
		return ErrUnauthenticated
	case codes.Unimplemented:
		return ErrUnimplemented
	case codes.ResourceExhausted:
		return ErrResourceExhausted
	default:
		return err
	}
}
