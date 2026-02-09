package rotation

import (
	"context"
	"errors"
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

// RESTRotationProvider rotates REST API credentials (basic auth, bearer, API key, OAuth2).
type RESTRotationProvider struct {
	// PasswordLength is the length of generated passwords/tokens (default 48).
	PasswordLength int
	// APIKeyLength is the length of generated API keys (default 64).
	APIKeyLength int
	// Commander executes REST API operations for credential rotation.
	Commander RESTCommander
}

// RESTCommander executes REST API credential management operations.
type RESTCommander interface {
	RotatePassword(ctx context.Context, device DeviceInfo, oldCred *credentials.RESTBasicCredential) (string, error)
	RotateToken(ctx context.Context, device DeviceInfo, oldCred *credentials.RESTBearerCredential) (string, error)
	RotateAPIKey(ctx context.Context, device DeviceInfo, oldCred *credentials.RESTAPIKeyCredential) (string, error)
	RotateOAuth2Secret(ctx context.Context, device DeviceInfo, oldCred *credentials.RESTOAuth2Credential) (string, error)
	TestRESTAccess(ctx context.Context, device DeviceInfo, cred credentials.Credential) error
	RevokeCredential(ctx context.Context, device DeviceInfo, cred credentials.Credential) error
}

// NewRESTRotationProvider creates a new REST rotation provider.
func NewRESTRotationProvider(commander RESTCommander) *RESTRotationProvider {
	return &RESTRotationProvider{
		PasswordLength: 48,
		APIKeyLength:   64,
		Commander:      commander,
	}
}

func (p *RESTRotationProvider) SupportsType(credType credentials.CredentialType) bool {
	switch credType {
	case credentials.CredentialTypeRESTBasic,
		credentials.CredentialTypeRESTBearer,
		credentials.CredentialTypeRESTAPIKey,
		credentials.CredentialTypeRESTOAuth2:
		return true
	default:
		return false
	}
}

func (p *RESTRotationProvider) ValidateOld(ctx context.Context, device DeviceInfo, cred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no REST commander configured")
	}
	return p.Commander.TestRESTAccess(ctx, device, cred)
}

func (p *RESTRotationProvider) Generate(ctx context.Context, device DeviceInfo, oldCred credentials.Credential) (credentials.Credential, error) {
	if p.Commander == nil {
		return nil, errors.New("no REST commander configured")
	}

	switch old := oldCred.(type) {
	case *credentials.RESTBasicCredential:
		newPass, err := p.Commander.RotatePassword(ctx, device, old)
		if err != nil {
			return nil, fmt.Errorf("failed to rotate password: %w", err)
		}
		return &credentials.RESTBasicCredential{
			BaseCredential: credentials.BaseCredential{
				CredentialID:   old.CredentialID,
				CredentialType: credentials.CredentialTypeRESTBasic,
				Description:    old.Description,
				Metadata:       old.Metadata,
			},
			Username: old.Username,
			Password: newPass,
		}, nil

	case *credentials.RESTBearerCredential:
		newToken, err := p.Commander.RotateToken(ctx, device, old)
		if err != nil {
			return nil, fmt.Errorf("failed to rotate token: %w", err)
		}
		return &credentials.RESTBearerCredential{
			BaseCredential: credentials.BaseCredential{
				CredentialID:   old.CredentialID,
				CredentialType: credentials.CredentialTypeRESTBearer,
				Description:    old.Description,
				Metadata:       old.Metadata,
			},
			Token: newToken,
		}, nil

	case *credentials.RESTAPIKeyCredential:
		newKey, err := p.Commander.RotateAPIKey(ctx, device, old)
		if err != nil {
			return nil, fmt.Errorf("failed to rotate API key: %w", err)
		}
		return &credentials.RESTAPIKeyCredential{
			BaseCredential: credentials.BaseCredential{
				CredentialID:   old.CredentialID,
				CredentialType: credentials.CredentialTypeRESTAPIKey,
				Description:    old.Description,
				Metadata:       old.Metadata,
			},
			APIKey:     newKey,
			HeaderName: old.HeaderName,
			QueryParam: old.QueryParam,
		}, nil

	case *credentials.RESTOAuth2Credential:
		newSecret, err := p.Commander.RotateOAuth2Secret(ctx, device, old)
		if err != nil {
			return nil, fmt.Errorf("failed to rotate OAuth2 secret: %w", err)
		}
		return &credentials.RESTOAuth2Credential{
			BaseCredential: credentials.BaseCredential{
				CredentialID:   old.CredentialID,
				CredentialType: credentials.CredentialTypeRESTOAuth2,
				Description:    old.Description,
				Metadata:       old.Metadata,
			},
			ClientID:     old.ClientID,
			ClientSecret: newSecret,
			TokenURL:     old.TokenURL,
			Scopes:       old.Scopes,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported credential type for REST rotation: %T", oldCred)
	}
}

func (p *RESTRotationProvider) Apply(_ context.Context, _ DeviceInfo, _, _ credentials.Credential) error {
	// For REST credentials, the generation step already applied the change on the API side.
	return nil
}

func (p *RESTRotationProvider) Verify(ctx context.Context, device DeviceInfo, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no REST commander configured")
	}
	return p.Commander.TestRESTAccess(ctx, device, newCred)
}

func (p *RESTRotationProvider) Rollback(ctx context.Context, device DeviceInfo, oldCred, newCred credentials.Credential) error {
	if p.Commander == nil {
		return errors.New("no REST commander configured")
	}
	// Revoke the new credential; old one should still work since generation is atomic on the API side
	return p.Commander.RevokeCredential(ctx, device, newCred)
}

func (p *RESTRotationProvider) Cleanup(ctx context.Context, device DeviceInfo, oldCred credentials.Credential) error {
	if p.Commander == nil {
		return nil
	}
	// Revoke the old credential now that the new one is verified
	return p.Commander.RevokeCredential(ctx, device, oldCred)
}

var _ RotationProvider = (*RESTRotationProvider)(nil)
