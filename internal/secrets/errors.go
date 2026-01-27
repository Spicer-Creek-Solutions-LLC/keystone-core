package secrets

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

// ErrorCode represents a standardized error code.
type ErrorCode string

const (
	// ErrorCodeNotFound indicates the secret was not found.
	ErrorCodeNotFound ErrorCode = "NOT_FOUND"

	// ErrorCodeAccessDenied indicates access was denied.
	ErrorCodeAccessDenied ErrorCode = "ACCESS_DENIED"

	// ErrorCodeInvalidRequest indicates the request was invalid.
	ErrorCodeInvalidRequest ErrorCode = "INVALID_REQUEST"

	// ErrorCodeBackendUnavailable indicates the backend is unavailable.
	ErrorCodeBackendUnavailable ErrorCode = "BACKEND_UNAVAILABLE"

	// ErrorCodeRateLimit indicates the rate limit was exceeded.
	ErrorCodeRateLimit ErrorCode = "RATE_LIMIT"

	// ErrorCodeTimeout indicates the operation timed out.
	ErrorCodeTimeout ErrorCode = "TIMEOUT"

	// ErrorCodeNetwork indicates a network error occurred.
	ErrorCodeNetwork ErrorCode = "NETWORK_ERROR"

	// ErrorCodeAuthentication indicates an authentication error.
	ErrorCodeAuthentication ErrorCode = "AUTHENTICATION_ERROR"

	// ErrorCodeAuthorization indicates an authorization error.
	ErrorCodeAuthorization ErrorCode = "AUTHORIZATION_ERROR"

	// ErrorCodeVersionNotFound indicates the secret version was not found.
	ErrorCodeVersionNotFound ErrorCode = "VERSION_NOT_FOUND"

	// ErrorCodeLeaseError indicates a lease-related error.
	ErrorCodeLeaseError ErrorCode = "LEASE_ERROR"

	// ErrorCodeEncryption indicates an encryption error.
	ErrorCodeEncryption ErrorCode = "ENCRYPTION_ERROR"

	// ErrorCodeConfiguration indicates a configuration error.
	ErrorCodeConfiguration ErrorCode = "CONFIGURATION_ERROR"

	// ErrorCodeInternal indicates an internal error.
	ErrorCodeInternal ErrorCode = "INTERNAL_ERROR"
)

// SecretError is a standardized error type for secret operations.
type SecretError struct {
	// Code is the standardized error code.
	Code ErrorCode `json:"code"`

	// Message is a human-readable error message.
	Message string `json:"message"`

	// Backend is the backend that produced the error.
	Backend BackendType `json:"backend,omitempty"`

	// Path is the secret path (if applicable).
	Path string `json:"path,omitempty"`

	// Retryable indicates if the operation can be retried.
	Retryable bool `json:"retryable"`

	// Cause is the underlying error.
	Cause error `json:"-"`

	// Details contains additional error details.
	Details map[string]string `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *SecretError) Error() string {
	if e.Backend != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Backend, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *SecretError) Unwrap() error {
	return e.Cause
}

// Is reports whether the error matches the target.
func (e *SecretError) Is(target error) bool {
	switch target {
	case ErrSecretNotFound:
		return e.Code == ErrorCodeNotFound
	case ErrAccessDenied:
		return e.Code == ErrorCodeAccessDenied || e.Code == ErrorCodeAuthorization
	case ErrBackendUnavailable:
		return e.Code == ErrorCodeBackendUnavailable
	case ErrInvalidPath:
		return e.Code == ErrorCodeInvalidRequest
	case ErrLeaseNotFound, ErrLeaseExpired, ErrLeaseNotRenewable:
		return e.Code == ErrorCodeLeaseError
	}
	return false
}

// WithDetails adds details to the error.
func (e *SecretError) WithDetails(key, value string) *SecretError {
	if e.Details == nil {
		e.Details = make(map[string]string)
	}
	e.Details[key] = value
	return e
}

// NewSecretError creates a new secret error.
func NewSecretError(code ErrorCode, message string, cause error) *SecretError {
	return &SecretError{
		Code:      code,
		Message:   message,
		Cause:     cause,
		Retryable: isRetryableCode(code),
	}
}

// isRetryableCode returns true if the error code is retryable.
func isRetryableCode(code ErrorCode) bool {
	switch code {
	case ErrorCodeBackendUnavailable, ErrorCodeRateLimit, ErrorCodeTimeout, ErrorCodeNetwork:
		return true
	default:
		return false
	}
}

// ErrorTranslator translates backend-specific errors to standardized errors.
type ErrorTranslator struct {
	backend BackendType
}

// NewErrorTranslator creates a new error translator.
func NewErrorTranslator(backend BackendType) *ErrorTranslator {
	return &ErrorTranslator{backend: backend}
}

// Translate translates an error to a standardized SecretError.
func (et *ErrorTranslator) Translate(err error, path string) *SecretError {
	if err == nil {
		return nil
	}

	// Check if already a SecretError
	var secretErr *SecretError
	if errors.As(err, &secretErr) {
		return secretErr
	}

	// Translate based on backend
	switch et.backend {
	case BackendTypeVault:
		return et.translateVaultError(err, path)
	case BackendTypeAWS:
		return et.translateAWSError(err, path)
	case BackendTypeAzure:
		return et.translateAzureError(err, path)
	case BackendTypeGCP:
		return et.translateGCPError(err, path)
	default:
		return et.translateGenericError(err, path)
	}
}

// translateVaultError translates Vault errors.
func (et *ErrorTranslator) translateVaultError(err error, path string) *SecretError {
	errStr := err.Error()

	// Check for common Vault errors
	if containsAny(errStr, "permission denied", "code=403") {
		return &SecretError{
			Code:      ErrorCodeAccessDenied,
			Message:   "permission denied",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "secret not found", "code=404", "no value found") {
		return &SecretError{
			Code:      ErrorCodeNotFound,
			Message:   "secret not found",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "invalid token", "token expired", "token not valid") {
		return &SecretError{
			Code:      ErrorCodeAuthentication,
			Message:   "authentication failed",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "rate limit", "code=429") {
		return &SecretError{
			Code:      ErrorCodeRateLimit,
			Message:   "rate limit exceeded",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "sealed", "standby") {
		return &SecretError{
			Code:      ErrorCodeBackendUnavailable,
			Message:   "vault is sealed or in standby mode",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "lease not found", "invalid lease") {
		return &SecretError{
			Code:      ErrorCodeLeaseError,
			Message:   "lease not found or invalid",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	return et.translateGenericError(err, path)
}

// translateAWSError translates AWS Secrets Manager errors.
func (et *ErrorTranslator) translateAWSError(err error, path string) *SecretError {
	errStr := err.Error()

	// Check for AWS-specific error codes
	if containsAny(errStr, "ResourceNotFoundException", "SecretNotFound") {
		return &SecretError{
			Code:      ErrorCodeNotFound,
			Message:   "secret not found",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "AccessDeniedException", "UnauthorizedAccess") {
		return &SecretError{
			Code:      ErrorCodeAccessDenied,
			Message:   "access denied",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "InvalidRequestException", "ValidationException") {
		return &SecretError{
			Code:      ErrorCodeInvalidRequest,
			Message:   "invalid request",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "LimitExceededException", "Throttling", "RequestLimitExceeded") {
		return &SecretError{
			Code:      ErrorCodeRateLimit,
			Message:   "rate limit exceeded",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "InternalServiceError", "ServiceUnavailable") {
		return &SecretError{
			Code:      ErrorCodeBackendUnavailable,
			Message:   "AWS service unavailable",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "InvalidParameterException", "MissingParameter") {
		return &SecretError{
			Code:      ErrorCodeConfiguration,
			Message:   "invalid parameter",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "ExpiredToken", "InvalidIdentityToken") {
		return &SecretError{
			Code:      ErrorCodeAuthentication,
			Message:   "authentication token expired",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	return et.translateGenericError(err, path)
}

// translateAzureError translates Azure Key Vault errors.
func (et *ErrorTranslator) translateAzureError(err error, path string) *SecretError {
	errStr := err.Error()

	// Check for Azure-specific error codes
	if containsAny(errStr, "SecretNotFound", "NotFound", "code=404") {
		return &SecretError{
			Code:      ErrorCodeNotFound,
			Message:   "secret not found",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "Forbidden", "AccessDenied", "code=403") {
		return &SecretError{
			Code:      ErrorCodeAccessDenied,
			Message:   "access denied",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "Unauthorized", "code=401") {
		return &SecretError{
			Code:      ErrorCodeAuthentication,
			Message:   "authentication failed",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "TooManyRequests", "code=429") {
		return &SecretError{
			Code:      ErrorCodeRateLimit,
			Message:   "rate limit exceeded",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "ServiceUnavailable", "code=503") {
		return &SecretError{
			Code:      ErrorCodeBackendUnavailable,
			Message:   "Azure Key Vault unavailable",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "BadRequest", "code=400") {
		return &SecretError{
			Code:      ErrorCodeInvalidRequest,
			Message:   "invalid request",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	return et.translateGenericError(err, path)
}

// translateGCPError translates GCP Secret Manager errors.
func (et *ErrorTranslator) translateGCPError(err error, path string) *SecretError {
	errStr := err.Error()

	// Check for gRPC status codes and GCP-specific errors
	if containsAny(errStr, "NotFound", "code = NotFound") {
		return &SecretError{
			Code:      ErrorCodeNotFound,
			Message:   "secret not found",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "PermissionDenied", "code = PermissionDenied") {
		return &SecretError{
			Code:      ErrorCodeAccessDenied,
			Message:   "permission denied",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "Unauthenticated", "code = Unauthenticated") {
		return &SecretError{
			Code:      ErrorCodeAuthentication,
			Message:   "authentication failed",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "ResourceExhausted", "code = ResourceExhausted", "quota") {
		return &SecretError{
			Code:      ErrorCodeRateLimit,
			Message:   "quota exceeded",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "Unavailable", "code = Unavailable") {
		return &SecretError{
			Code:      ErrorCodeBackendUnavailable,
			Message:   "GCP Secret Manager unavailable",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "InvalidArgument", "code = InvalidArgument") {
		return &SecretError{
			Code:      ErrorCodeInvalidRequest,
			Message:   "invalid argument",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	if containsAny(errStr, "DeadlineExceeded", "code = DeadlineExceeded") {
		return &SecretError{
			Code:      ErrorCodeTimeout,
			Message:   "request timed out",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "FailedPrecondition", "code = FailedPrecondition") {
		return &SecretError{
			Code:      ErrorCodeInvalidRequest,
			Message:   "precondition failed",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	return et.translateGenericError(err, path)
}

// translateGenericError translates generic errors.
func (et *ErrorTranslator) translateGenericError(err error, path string) *SecretError {
	// Check for network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		code := ErrorCodeNetwork
		if netErr.Timeout() {
			code = ErrorCodeTimeout
		}
		return &SecretError{
			Code:      code,
			Message:   "network error",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	// Check for common error messages
	errStr := err.Error()

	if containsAny(errStr, "connection refused", "connection reset", "no route to host") {
		return &SecretError{
			Code:      ErrorCodeNetwork,
			Message:   "connection error",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "timeout", "deadline exceeded") {
		return &SecretError{
			Code:      ErrorCodeTimeout,
			Message:   "request timed out",
			Backend:   et.backend,
			Path:      path,
			Retryable: true,
			Cause:     err,
		}
	}

	if containsAny(errStr, "certificate", "tls", "x509") {
		return &SecretError{
			Code:      ErrorCodeConfiguration,
			Message:   "TLS/certificate error",
			Backend:   et.backend,
			Path:      path,
			Retryable: false,
			Cause:     err,
		}
	}

	// Default to internal error
	return &SecretError{
		Code:      ErrorCodeInternal,
		Message:   err.Error(),
		Backend:   et.backend,
		Path:      path,
		Retryable: false,
		Cause:     err,
	}
}

// containsAny checks if the string contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	s = strings.ToLower(s)
	for _, sub := range substrings {
		if strings.Contains(s, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

// ErrorClassifier classifies errors for metrics and alerting.
type ErrorClassifier struct{}

// NewErrorClassifier creates a new error classifier.
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{}
}

// Classify classifies an error and returns its code and whether it's retryable.
func (ec *ErrorClassifier) Classify(err error) (ErrorCode, bool) {
	if err == nil {
		return "", false
	}

	var secretErr *SecretError
	if errors.As(err, &secretErr) {
		return secretErr.Code, secretErr.Retryable
	}

	// Check for known error types
	switch {
	case errors.Is(err, ErrSecretNotFound):
		return ErrorCodeNotFound, false
	case errors.Is(err, ErrAccessDenied):
		return ErrorCodeAccessDenied, false
	case errors.Is(err, ErrBackendUnavailable):
		return ErrorCodeBackendUnavailable, true
	case errors.Is(err, ErrInvalidPath):
		return ErrorCodeInvalidRequest, false
	case errors.Is(err, ErrLeaseNotFound), errors.Is(err, ErrLeaseExpired):
		return ErrorCodeLeaseError, false
	default:
		return ErrorCodeInternal, false
	}
}

// IsRetryable returns true if the error should be retried.
func (ec *ErrorClassifier) IsRetryable(err error) bool {
	_, retryable := ec.Classify(err)
	return retryable
}

// IsNotFound returns true if the error indicates not found.
func (ec *ErrorClassifier) IsNotFound(err error) bool {
	code, _ := ec.Classify(err)
	return code == ErrorCodeNotFound
}

// IsAccessDenied returns true if the error indicates access denied.
func (ec *ErrorClassifier) IsAccessDenied(err error) bool {
	code, _ := ec.Classify(err)
	return code == ErrorCodeAccessDenied || code == ErrorCodeAuthorization
}

// IsAuthentication returns true if the error indicates authentication failure.
func (ec *ErrorClassifier) IsAuthentication(err error) bool {
	code, _ := ec.Classify(err)
	return code == ErrorCodeAuthentication
}

// IsRateLimit returns true if the error indicates rate limiting.
func (ec *ErrorClassifier) IsRateLimit(err error) bool {
	code, _ := ec.Classify(err)
	return code == ErrorCodeRateLimit
}

// WrapError wraps an error with backend context.
func WrapError(err error, backend BackendType, path string) error {
	if err == nil {
		return nil
	}

	translator := NewErrorTranslator(backend)
	return translator.Translate(err, path)
}
