package rotation

import (
	"context"
	"errors"
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

// SNMPRotationProvider rotates SNMP v2c community strings and v3 credentials.
type SNMPRotationProvider struct {
	// CommunityLength is the length of generated community strings (default 24).
	CommunityLength int
	// PasswordLength is the length of generated SNMPv3 passwords (default 32).
	PasswordLength int
	// Commander executes commands on devices for applying SNMP config changes.
	Commander SNMPCommander
}

// SNMPCommander executes SNMP configuration commands on devices.
type SNMPCommander interface {
	SetCommunity(ctx context.Context, device DeviceInfo, oldCred credentials.Credential, newCommunity string) error
	SetSNMPv3Credentials(ctx context.Context, device DeviceInfo, oldCred credentials.Credential, username, authPass, privPass string) error
	TestSNMPAccess(ctx context.Context, device DeviceInfo, cred credentials.Credential) error
}

// NewSNMPRotationProvider creates a new SNMP rotation provider.
func NewSNMPRotationProvider(commander SNMPCommander) *SNMPRotationProvider {
	return &SNMPRotationProvider{
		CommunityLength: 24,
		PasswordLength:  32,
		Commander:       commander,
	}
}

func (p *SNMPRotationProvider) SupportsType(credType credentials.CredentialType) bool {
	return credType == credentials.CredentialTypeSNMPv2c || credType == credentials.CredentialTypeSNMPv3
}

func (p *SNMPRotationProvider) ValidateOld(ctx context.Context, device DeviceInfo, cred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SNMP commander configured")
	}
	return p.Commander.TestSNMPAccess(ctx, device, cred)
}

func (p *SNMPRotationProvider) Generate(_ context.Context, _ DeviceInfo, oldCred credentials.Credential) (credentials.Credential, error) {
	switch old := oldCred.(type) {
	case *credentials.SNMPv2cCredential:
		return p.generateV2c(old)
	case *credentials.SNMPv3Credential:
		return p.generateV3(old)
	default:
		return nil, fmt.Errorf("unsupported credential type for SNMP rotation: %T", oldCred)
	}
}

func (p *SNMPRotationProvider) Apply(ctx context.Context, device DeviceInfo, oldCred, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SNMP commander configured")
	}

	switch newC := newCred.(type) {
	case *credentials.SNMPv2cCredential:
		return p.Commander.SetCommunity(ctx, device, oldCred, newC.Community)
	case *credentials.SNMPv3Credential:
		return p.Commander.SetSNMPv3Credentials(ctx, device, oldCred, newC.Username, newC.AuthPassword, newC.PrivacyPassword)
	default:
		return fmt.Errorf("unsupported credential type: %T", newCred)
	}
}

func (p *SNMPRotationProvider) Verify(ctx context.Context, device DeviceInfo, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SNMP commander configured")
	}
	return p.Commander.TestSNMPAccess(ctx, device, newCred)
}

func (p *SNMPRotationProvider) Rollback(ctx context.Context, device DeviceInfo, oldCred, _ credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no SNMP commander configured")
	}

	switch oldC := oldCred.(type) {
	case *credentials.SNMPv2cCredential:
		return p.Commander.SetCommunity(ctx, device, oldCred, oldC.Community)
	case *credentials.SNMPv3Credential:
		return p.Commander.SetSNMPv3Credentials(ctx, device, oldCred, oldC.Username, oldC.AuthPassword, oldC.PrivacyPassword)
	default:
		return fmt.Errorf("unsupported credential type: %T", oldCred)
	}
}

func (p *SNMPRotationProvider) Cleanup(_ context.Context, _ DeviceInfo, _ credentials.Credential) error {
	return nil
}

func (p *SNMPRotationProvider) generateV2c(old *credentials.SNMPv2cCredential) (*credentials.SNMPv2cCredential, error) {
	community, err := generateRandomPassword(p.CommunityLength)
	if err != nil {
		return nil, fmt.Errorf("failed to generate community string: %w", err)
	}

	return &credentials.SNMPv2cCredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   old.CredentialID,
			CredentialType: credentials.CredentialTypeSNMPv2c,
			Description:    old.Description,
			Metadata:       old.Metadata,
		},
		Community: community,
	}, nil
}

func (p *SNMPRotationProvider) generateV3(old *credentials.SNMPv3Credential) (*credentials.SNMPv3Credential, error) {
	newCred := &credentials.SNMPv3Credential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   old.CredentialID,
			CredentialType: credentials.CredentialTypeSNMPv3,
			Description:    old.Description,
			Metadata:       old.Metadata,
		},
		Username:         old.Username,
		SecurityLevel:    old.SecurityLevel,
		AuthProtocol:     old.AuthProtocol,
		PrivacyProtocol:  old.PrivacyProtocol,
		ContextName:      old.ContextName,
		ContextEngineID:  old.ContextEngineID,
		SecurityEngineID: old.SecurityEngineID,
	}

	// Generate new auth password if auth is used
	if old.SecurityLevel == credentials.SNMPv3SecurityAuthNoPriv || old.SecurityLevel == credentials.SNMPv3SecurityAuthPriv {
		authPass, err := generateRandomPassword(p.PasswordLength)
		if err != nil {
			return nil, fmt.Errorf("failed to generate auth password: %w", err)
		}
		newCred.AuthPassword = authPass
	}

	// Generate new privacy password if privacy is used
	if old.SecurityLevel == credentials.SNMPv3SecurityAuthPriv {
		privPass, err := generateRandomPassword(p.PasswordLength)
		if err != nil {
			return nil, fmt.Errorf("failed to generate privacy password: %w", err)
		}
		newCred.PrivacyPassword = privPass
	}

	return newCred, nil
}

var _ RotationProvider = (*SNMPRotationProvider)(nil)
