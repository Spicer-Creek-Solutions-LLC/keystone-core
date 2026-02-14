package policy

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Sentinel errors returned by the policy client.
var (
	ErrNotFound         = errors.New("not found")
	ErrAccessDenied     = errors.New("access denied")
	ErrInvalidArgument  = errors.New("invalid argument")
	ErrUnavailable      = errors.New("service unavailable")
	ErrUnauthenticated  = errors.New("unauthenticated")
	ErrUnimplemented    = errors.New("not implemented")
	ErrAlreadyExists    = errors.New("already exists")
	ErrResourceExhausted = errors.New("resource exhausted")
)

// grpcStatusToError converts a gRPC status error to a public sentinel error.
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
		return ErrNotFound
	case codes.PermissionDenied:
		return ErrAccessDenied
	case codes.InvalidArgument:
		return ErrInvalidArgument
	case codes.Unavailable:
		return ErrUnavailable
	case codes.Unauthenticated:
		return ErrUnauthenticated
	case codes.Unimplemented:
		return ErrUnimplemented
	case codes.AlreadyExists:
		return ErrAlreadyExists
	case codes.ResourceExhausted:
		return ErrResourceExhausted
	default:
		return err
	}
}
