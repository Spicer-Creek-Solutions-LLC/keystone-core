package rotation

import (
	"context"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

// Provider implements credential rotation for a specific protocol or device type.
type Provider interface {
	// SupportsType returns true if this provider can rotate the credential type.
	SupportsType(credType credentials.CredentialType) bool

	// ValidateOld validates the current credential still works on the device.
	ValidateOld(ctx context.Context, device DeviceInfo, cred credentials.Credential) error

	// Generate creates a new credential to replace the current one.
	Generate(ctx context.Context, device DeviceInfo, oldCred credentials.Credential) (credentials.Credential, error)

	// Apply applies the new credential to the device.
	Apply(ctx context.Context, device DeviceInfo, oldCred, newCred credentials.Credential) error

	// Verify verifies the new credential works on the device.
	Verify(ctx context.Context, device DeviceInfo, newCred credentials.Credential) error

	// Rollback reverts the device to the old credential.
	Rollback(ctx context.Context, device DeviceInfo, oldCred, newCred credentials.Credential) error

	// Cleanup removes old credential artifacts from the device after successful rotation.
	Cleanup(ctx context.Context, device DeviceInfo, oldCred credentials.Credential) error
}
