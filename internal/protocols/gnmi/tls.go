package gnmi

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/credentials"
)

// buildTLSConfig creates a TLS configuration from a GNMICredential.
func buildTLSConfig(cred *credentials.GNMICredential) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	if cred.SkipVerify {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // user-configured skip-verify for dev/test
	}

	if len(cred.CACert) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cred.CACert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = pool
	}

	if len(cred.ClientCert) > 0 && len(cred.ClientKey) > 0 {
		cert, err := tls.X509KeyPair(cred.ClientCert, cred.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}
