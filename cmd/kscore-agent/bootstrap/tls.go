package bootstrap

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"
)

const (
	defaultTLSCertPath          = "/etc/kscore/tls/kscore.crt"
	defaultTLSKeyPath           = "/etc/kscore/tls/kscore.key"
	defaultTLSCAPath            = "/etc/kscore/tls/ca.crt"
	defaultTLSCSRPath           = "/etc/kscore/tls/kscore.csr"
	defaultTLSRenewalScriptPath = "/etc/kscore/tls/renew.sh"
)

type TLSBundle struct {
	CertPEM []byte
	KeyPEM  []byte
	CAPEM   []byte
}

type CSRBundle struct {
	CSRPEM []byte
	KeyPEM []byte
}

func GenerateTLSBundle(hosts []string) (TLSBundle, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return TLSBundle{}, fmt.Errorf("generate ca key: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Keystone Core CA",
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return TLSBundle{}, fmt.Errorf("create ca cert: %w", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return TLSBundle{}, fmt.Errorf("generate server key: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "keystone-core",
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().AddDate(2, 0, 0),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	addHosts(serverTemplate, hosts)

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return TLSBundle{}, fmt.Errorf("create server cert: %w", err)
	}

	return TLSBundle{
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)}),
		CAPEM:   pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
	}, nil
}

func GenerateCSRBundle(hosts []string) (CSRBundle, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return CSRBundle{}, fmt.Errorf("generate key: %w", err)
	}
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "keystone-core",
		},
	}
	addHostsToCSR(template, hosts)

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return CSRBundle{}, fmt.Errorf("create csr: %w", err)
	}

	return CSRBundle{
		CSRPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}),
		KeyPEM: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
	}, nil
}

func addHosts(template *x509.Certificate, hosts []string) {
	seen := map[string]bool{}
	for _, host := range hosts {
		if host == "" {
			continue
		}
		if seen[host] {
			continue
		}
		seen[host] = true
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
}

func addHostsToCSR(template *x509.CertificateRequest, hosts []string) {
	seen := map[string]bool{}
	for _, host := range hosts {
		if host == "" {
			continue
		}
		if seen[host] {
			continue
		}
		seen[host] = true
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
}

func resolveTLSPaths(cfg *BootstrapConfig) (certPath, keyPath, caPath string) {
	certPath = cfg.TLSCertFile
	keyPath = cfg.TLSKeyFile
	caPath = cfg.TLSCAFile

	if cfg.GenerateCerts {
		if certPath == "" {
			certPath = defaultTLSCertPath
		}
		if keyPath == "" {
			keyPath = defaultTLSKeyPath
		}
		if caPath == "" {
			caPath = defaultTLSCAPath
		}
	}

	return certPath, keyPath, caPath
}

func resolveTLSCSRPath(cfg *BootstrapConfig) string {
	if cfg.TLSCSRFile != "" {
		return cfg.TLSCSRFile
	}
	return defaultTLSCSRPath
}

func resolveTLSRenewalScriptPath(cfg *BootstrapConfig) string {
	if cfg.TLSRenewalScriptPath != "" {
		return cfg.TLSRenewalScriptPath
	}
	return defaultTLSRenewalScriptPath
}

func ensureTLSCSRFiles(cfg *BootstrapConfig, output io.Writer, verbose bool) ([]string, error) {
	csrPath := resolveTLSCSRPath(cfg)
	keyPath := cfg.TLSKeyFile
	if keyPath == "" {
		keyPath = defaultTLSKeyPath
	}
	cfg.TLSCSRFile = csrPath
	cfg.TLSKeyFile = keyPath

	csrExists := fileExists(csrPath)
	keyExists := fileExists(keyPath)
	if csrExists && keyExists {
		if verbose {
			fmt.Fprintln(output, "tls csr and key already exist, skipping generation")
		}
		return nil, nil
	}
	if csrExists || keyExists {
		return nil, fmt.Errorf("partial tls csr/key already exist, refusing to overwrite")
	}

	if verbose {
		fmt.Fprintln(output, "generating tls certificate signing request")
	}
	bundle, err := GenerateCSRBundle(collectTLSHosts(cfg))
	if err != nil {
		return nil, err
	}
	if err := writeFile(csrPath, bundle.CSRPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write csr: %w", err)
	}
	if err := writeFile(keyPath, bundle.KeyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write key: %w", err)
	}
	return []string{csrPath, keyPath}, nil
}

func ensureTLSRenewalScript(cfg *BootstrapConfig, output io.Writer, verbose bool) ([]string, error) {
	if cfg.TLSRenewalCommand == "" {
		return nil, nil
	}
	scriptPath := resolveTLSRenewalScriptPath(cfg)
	cfg.TLSRenewalScriptPath = scriptPath
	if fileExists(scriptPath) {
		if verbose {
			fmt.Fprintln(output, "tls renewal script already exists, skipping")
		}
		return nil, nil
	}
	content := fmt.Sprintf("#!/bin/sh\n%s\n", cfg.TLSRenewalCommand)
	if verbose {
		fmt.Fprintf(output, "writing tls renewal script %s\n", scriptPath)
	}
	if err := writeFile(scriptPath, []byte(content), 0o750); err != nil {
		return nil, fmt.Errorf("write renewal script: %w", err)
	}
	return []string{scriptPath}, nil
}

func collectTLSHosts(cfg *BootstrapConfig) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if cfg.BindAddress != "" {
		hosts = append(hosts, cfg.BindAddress)
	}
	if cfg.Advertise != "" {
		hosts = append(hosts, cfg.Advertise)
	}
	return hosts
}
