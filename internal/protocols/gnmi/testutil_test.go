package gnmi

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/proxy"
	"github.com/shawnbutts/keystone-core/internal/security"
)

// testCerts holds PEM-encoded certificates for testing.
type testCerts struct {
	CA            *security.CertificateAuthority
	CAPem         []byte
	ServerCert    tls.Certificate
	ClientCertPem []byte
	ClientKeyPem  []byte
}

// generateTestCerts creates a CA, server cert, and client cert for testing.
func generateTestCerts(t *testing.T) *testCerts {
	t.Helper()

	ca, err := security.GenerateCA("test-ca", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate CA: %v", err)
	}

	serverCert, serverKey, err := ca.GenerateServerCert("localhost", []string{"localhost"}, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate server cert: %v", err)
	}

	clientCert, clientKey, err := ca.GenerateClientCert("test-client", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate client cert: %v", err)
	}

	caPem := encodeCert(ca.Certificate)
	serverTLSCert, err := tls.X509KeyPair(encodeCert(serverCert), encodeKey(serverKey))
	if err != nil {
		t.Fatalf("failed to load server TLS cert: %v", err)
	}

	return &testCerts{
		CA:            ca,
		CAPem:         caPem,
		ServerCert:    serverTLSCert,
		ClientCertPem: encodeCert(clientCert),
		ClientKeyPem:  encodeKey(clientKey),
	}
}

func encodeCert(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

func encodeKey(key *ecdsa.PrivateKey) []byte {
	der, _ := x509.MarshalECPrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})
}

// mockGNMIServer wraps a gRPC server implementing the gNMI service.
type mockGNMIServer struct {
	gnmipb.UnimplementedGNMIServer
	server   *grpc.Server
	listener net.Listener
	addr     string

	CapabilitiesFunc func(*gnmipb.CapabilityRequest) (*gnmipb.CapabilityResponse, error)
	GetFunc          func(*gnmipb.GetRequest) (*gnmipb.GetResponse, error)
	SetFunc          func(*gnmipb.SetRequest) (*gnmipb.SetResponse, error)
	SubscribeFunc    func(gnmipb.GNMI_SubscribeServer) error
}

func (m *mockGNMIServer) Capabilities(_ context.Context, req *gnmipb.CapabilityRequest) (*gnmipb.CapabilityResponse, error) {
	if m.CapabilitiesFunc != nil {
		return m.CapabilitiesFunc(req)
	}
	return &gnmipb.CapabilityResponse{GNMIVersion: "0.10.0"}, nil
}

func (m *mockGNMIServer) Get(_ context.Context, req *gnmipb.GetRequest) (*gnmipb.GetResponse, error) {
	if m.GetFunc != nil {
		return m.GetFunc(req)
	}
	return &gnmipb.GetResponse{}, nil
}

func (m *mockGNMIServer) Set(_ context.Context, req *gnmipb.SetRequest) (*gnmipb.SetResponse, error) {
	if m.SetFunc != nil {
		return m.SetFunc(req)
	}
	return &gnmipb.SetResponse{}, nil
}

func (m *mockGNMIServer) Subscribe(stream gnmipb.GNMI_SubscribeServer) error {
	if m.SubscribeFunc != nil {
		return m.SubscribeFunc(stream)
	}
	return nil
}

// startMockServer creates and starts a mock gNMI server with TLS.
func startMockServer(t *testing.T, certs *testCerts) *mockGNMIServer {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	clientCAPool := x509.NewCertPool()
	clientCAPool.AddCert(certs.CA.Certificate)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certs.ServerCert},
		ClientCAs:    clientCAPool,
		ClientAuth:   tls.VerifyClientCertIfGiven,
		MinVersion:   tls.VersionTLS12,
	}

	grpcServer := grpc.NewServer(grpc.Creds(grpccreds.NewTLS(tlsConfig)))
	mock := &mockGNMIServer{
		server:   grpcServer,
		listener: lis,
		addr:     lis.Addr().String(),
	}
	gnmipb.RegisterGNMIServer(grpcServer, mock)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})

	return mock
}

// makeTestCredential creates a GNMICredential from test certs.
func makeTestCredential(certs *testCerts) *credentials.GNMICredential {
	return &credentials.GNMICredential{
		BaseCredential: credentials.BaseCredential{
			CredentialID:   "test-gnmi-cred",
			CredentialType: credentials.CredentialTypeGNMI,
		},
		CACert:     certs.CAPem,
		ClientCert: certs.ClientCertPem,
		ClientKey:  certs.ClientKeyPem,
	}
}

// makeTestDevice creates a ProxiedDevice pointing at the mock server address.
// Uses "localhost" as the address to match the server cert DNS SAN.
func makeTestDevice(addr string) *proxy.ProxiedDevice {
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	return &proxy.ProxiedDevice{
		ID:           "test-gnmi-device",
		ProxyAgentID: "test-proxy",
		Name:         "test-device",
		Type:         proxy.DeviceTypeRouter,
		Protocol:     proxy.ProtocolGNMI,
		Address:      "localhost",
		Port:         port,
		ProfileID:    "test-profile",
	}
}
