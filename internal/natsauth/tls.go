package natsauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"path"
	"time"
)

// ADR-0002 § 12: TLS on every connection, terminating at the broker. G49.
//
// The choices below were each measured against nats:2.15.0-alpine, the broker
// C03-A pins, before they were written here:
//
//   - TLS-FIRST. With handshake_first the broker sends no byte before the
//     handshake; without it NATS sends its INFO greeting in plaintext first.
//     A raw TCP connect to a TLS-first broker reads nothing.
//   - TLS 1.3 MINIMUM. A client capped at 1.2 is refused with "protocol version
//     not supported".
//   - ABSOLUTE PATHS. The broker resolves cert_file and key_file against its
//     working directory, not against the configuration file; a relative path
//     stops it starting. So the configuration names paths under RuntimeDir.
//   - NO CLIENT CERTIFICATES. A principal's identity is its NATS JWT (ADR-0002
//     § 2); TLS authenticates the broker to the client and protects the
//     connection, and asks nothing of the client.

// Default lifetimes. Both are configuration; rotating either is C13's
// procedure, like RSK-13's for the NATS credentials.
const (
	DefaultCALifetime         = 5 * 365 * 24 * time.Hour
	DefaultBrokerCertLifetime = 365 * 24 * time.Hour

	// clockAllowance back-dates NotBefore so that a broker or client whose clock
	// runs slightly behind the generator's does not refuse a new certificate.
	// ADR-0003 § 12 bounds the skew the deployment assumes at about a minute.
	clockAllowance = 5 * time.Minute
)

// TLS file names inside the deployment directory. The broker configuration
// names them under RuntimeDir, so they are part of the generator's interface.
const (
	TLSDir            = "tls"
	CACertFile        = "ca.pem"
	BrokerCertFile    = "broker.pem"
	BrokerKeyFile     = "broker-key.pem"
	certificateMode   = 0o644
	privateKeyPEMType = "PRIVATE KEY"
)

var (
	// ErrBrokerNames is returned when no name is given for the broker's
	// certificate. There is no default: a client verifies the broker against a
	// name, and a guessed name is a certificate every client must be told to
	// accept regardless.
	ErrBrokerNames = errors.New("natsauth: at least one broker name is required for its TLS certificate")

	// ErrRuntimeDir is returned for a runtime directory that is not absolute.
	ErrRuntimeDir = errors.New("natsauth: the broker's runtime directory must be an absolute path")
)

// TLSMaterial is the broker's TLS identity and the anchor clients trust.
//
// It holds no CA private key. That is Deployment.CAKey, returned to the caller
// and never written by Write, for the reason OperatorSeed is: whoever holds it
// can issue a certificate every client accepts as the broker.
type TLSMaterial struct {
	CACertPEM     []byte
	BrokerCertPEM []byte
	BrokerKeyPEM  []byte
	BrokerNames   []string
}

// newTLS issues a CA and a broker certificate signed by it. It returns the CA
// private key separately so the caller decides where it goes.
func newTLS(names []string, caLifetime, brokerLifetime time.Duration, now time.Time) (TLSMaterial, []byte, error) {
	if len(names) == 0 {
		return TLSMaterial{}, nil, ErrBrokerNames
	}
	for _, n := range names {
		if n == "" {
			return TLSMaterial{}, nil, ErrBrokerNames
		}
	}
	if caLifetime <= 0 {
		caLifetime = DefaultCALifetime
	}
	if brokerLifetime <= 0 {
		brokerLifetime = DefaultBrokerCertLifetime
	}
	notBefore := now.Add(-clockAllowance)
	caNotAfter := now.Add(caLifetime)
	brokerNotAfter := now.Add(brokerLifetime)
	if brokerNotAfter.After(caNotAfter) {
		brokerNotAfter = caNotAfter
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return TLSMaterial{}, nil, fmt.Errorf("generate CA key: %w", err)
	}
	caSerial, err := serial()
	if err != nil {
		return TLSMaterial{}, nil, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "Keystone broker CA"},
		NotBefore:             notBefore,
		NotAfter:              caNotAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		// It issues broker certificates and nothing below them.
		MaxPathLen:     0,
		MaxPathLenZero: true,
		KeyUsage:       x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return TLSMaterial{}, nil, fmt.Errorf("sign CA certificate: %w", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return TLSMaterial{}, nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	brokerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return TLSMaterial{}, nil, fmt.Errorf("generate broker key: %w", err)
	}
	brokerSerial, err := serial()
	if err != nil {
		return TLSMaterial{}, nil, err
	}
	brokerTemplate := &x509.Certificate{
		SerialNumber: brokerSerial,
		Subject:      pkix.Name{CommonName: names[0]},
		NotBefore:    notBefore,
		NotAfter:     brokerNotAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	// net/netip, not net: it parses an address and cannot open a connection,
	// and internal/cli/boundary_test.go confines net to the operator substrate.
	for _, n := range names {
		if addr, err := netip.ParseAddr(n); err == nil {
			brokerTemplate.IPAddresses = append(brokerTemplate.IPAddresses, addr.AsSlice())
		} else {
			brokerTemplate.DNSNames = append(brokerTemplate.DNSNames, n)
		}
	}
	brokerDER, err := x509.CreateCertificate(rand.Reader, brokerTemplate, ca, &brokerKey.PublicKey, caKey)
	if err != nil {
		return TLSMaterial{}, nil, fmt.Errorf("sign broker certificate: %w", err)
	}
	brokerKeyDER, err := x509.MarshalPKCS8PrivateKey(brokerKey)
	if err != nil {
		return TLSMaterial{}, nil, fmt.Errorf("encode broker key: %w", err)
	}
	caKeyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		return TLSMaterial{}, nil, fmt.Errorf("encode CA key: %w", err)
	}

	return TLSMaterial{
		CACertPEM:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		BrokerCertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: brokerDER}),
		BrokerKeyPEM:  pem.EncodeToMemory(&pem.Block{Type: privateKeyPEMType, Bytes: brokerKeyDER}),
		BrokerNames:   append([]string(nil), names...),
	}, pem.EncodeToMemory(&pem.Block{Type: privateKeyPEMType, Bytes: caKeyDER}), nil
}

func serial() (*big.Int, error) {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, fmt.Errorf("certificate serial: %w", err)
	}
	return n, nil
}

// tlsBlock is the broker configuration's tls section, naming the files under
// the directory the broker will find them in.
func tlsBlock(runtimeDir string) string {
	dir := path.Join(runtimeDir, TLSDir)
	return fmt.Sprintf("tls {\n"+
		"  cert_file: %q\n"+
		"  key_file: %q\n"+
		"  handshake_first: true\n"+
		"  min_version: \"1.3\"\n"+
		"}\n", path.Join(dir, BrokerCertFile), path.Join(dir, BrokerKeyFile))
}

// ClientTLSConfig is how every Keystone client verifies the broker: against the
// deployment's CA and one of the names its certificate carries, at TLS 1.3 or
// above. A NATS client must ALSO enable TLS-first -- nats.TLSHandshakeFirst() --
// because the broker refuses a client that waits for a plaintext greeting.
//
// Verification is never disabled. There is no parameter that turns it off.
func ClientTLSConfig(caCertPEM []byte, brokerName string) (*tls.Config, error) {
	if brokerName == "" {
		return nil, ErrBrokerNames
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCertPEM) {
		return nil, errors.New("natsauth: no CA certificate in the supplied PEM")
	}
	return &tls.Config{
		RootCAs:    pool,
		ServerName: brokerName,
		MinVersion: tls.VersionTLS13,
	}, nil
}
