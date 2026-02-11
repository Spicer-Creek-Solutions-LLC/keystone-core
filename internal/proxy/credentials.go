package proxy

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// EnvCredentialProvider reads credentials from environment variables.
// For a credential ref "router-creds" with prefix "KSCORE_CRED", it looks up:
//
//	KSCORE_CRED_ROUTER_CREDS_USERNAME
//	KSCORE_CRED_ROUTER_CREDS_PASSWORD
//	KSCORE_CRED_ROUTER_CREDS_TOKEN
//	KSCORE_CRED_ROUTER_CREDS_COMMUNITY
//	KSCORE_CRED_ROUTER_CREDS_TYPE
type EnvCredentialProvider struct {
	prefix string
}

// NewEnvCredentialProvider creates a provider that reads credentials from
// environment variables. The prefix defaults to "KSCORE_CRED" if empty.
func NewEnvCredentialProvider(prefix string) *EnvCredentialProvider {
	if prefix == "" {
		prefix = "KSCORE_CRED"
	}
	return &EnvCredentialProvider{prefix: prefix}
}

// GetCredential retrieves a credential by looking up environment variables
// derived from the ref name.
func (p *EnvCredentialProvider) GetCredential(_ context.Context, ref string) (*Credential, error) {
	envKey := p.envKey(ref)

	username := os.Getenv(envKey + "_USERNAME")
	password := os.Getenv(envKey + "_PASSWORD")
	token := os.Getenv(envKey + "_TOKEN")
	community := os.Getenv(envKey + "_COMMUNITY")
	credType := os.Getenv(envKey + "_TYPE")

	if username == "" && password == "" && token == "" && community == "" {
		return nil, fmt.Errorf("no credential environment variables found for ref %q (checked %s_*)", ref, envKey)
	}

	if credType == "" {
		credType = inferCredentialType(username, password, token, community)
	}

	return &Credential{
		Type:      credType,
		Username:  username,
		Password:  password,
		Token:     token,
		Community: community,
	}, nil
}

// envKey converts a credential ref to an environment variable prefix.
// "router-creds" with prefix "KSCORE_CRED" becomes "KSCORE_CRED_ROUTER_CREDS".
func (p *EnvCredentialProvider) envKey(ref string) string {
	normalized := strings.ToUpper(ref)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, ".", "_")
	return p.prefix + "_" + normalized
}

func inferCredentialType(username, password, token, community string) string {
	switch {
	case community != "":
		return "snmp_community"
	case token != "":
		return "token"
	case username != "" && password != "":
		return "password"
	default:
		return "password"
	}
}
