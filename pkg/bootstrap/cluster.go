package bootstrap

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ClusterFormation handles the formation of a new Keystone Core cluster
type ClusterFormation struct {
	config    *SeedConfig
	registry  *InstallerRegistry
	logger    Logger
	mu        sync.RWMutex
	status    *ClusterFormationStatus
	certDir   string
	dataDir   string
}

// ClusterFormationStatus tracks the status of cluster formation
type ClusterFormationStatus struct {
	Phase         string            `json:"phase"`
	NodesReady    int               `json:"nodes_ready"`
	TotalNodes    int               `json:"total_nodes"`
	LeaderElected bool              `json:"leader_elected"`
	LeaderNode    string            `json:"leader_node"`
	Errors        []string          `json:"errors,omitempty"`
	NodeStatuses  map[string]string `json:"node_statuses"`
}

// NewClusterFormation creates a new cluster formation handler
func NewClusterFormation(config *SeedConfig, registry *InstallerRegistry, logger Logger) *ClusterFormation {
	return &ClusterFormation{
		config:   config,
		registry: registry,
		logger:   logger,
		certDir:  "/etc/kscore/certs",
		dataDir:  "/var/lib/kscore",
		status: &ClusterFormationStatus{
			Phase:        "initializing",
			TotalNodes:   len(config.ControlPlane.Nodes),
			NodeStatuses: make(map[string]string),
		},
	}
}

// FormCluster orchestrates the formation of a new cluster
func (cf *ClusterFormation) FormCluster(ctx context.Context) error {
	cf.logger.Info("Starting cluster formation", "cluster", cf.config.Cluster.Name)

	// Step 1: Generate certificates
	cf.updatePhase("generating_certificates")
	if err := cf.generateCertificates(ctx); err != nil {
		return fmt.Errorf("failed to generate certificates: %w", err)
	}

	// Step 2: Initialize single node or coordinate multi-node
	if len(cf.config.ControlPlane.Nodes) == 1 {
		return cf.formSingleNodeCluster(ctx)
	}
	return cf.formMultiNodeCluster(ctx)
}

func (cf *ClusterFormation) updatePhase(phase string) {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	cf.status.Phase = phase
}

func (cf *ClusterFormation) updateNodeStatus(node, status string) {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	cf.status.NodeStatuses[node] = status
}

// GetStatus returns the current cluster formation status
func (cf *ClusterFormation) GetStatus() *ClusterFormationStatus {
	cf.mu.RLock()
	defer cf.mu.RUnlock()
	return cf.status
}

// generateCertificates creates the CA and server certificates
func (cf *ClusterFormation) generateCertificates(ctx context.Context) error {
	if !cf.config.ControlPlane.API.TLS.AutoGenerate {
		cf.logger.Info("TLS auto-generation disabled, skipping certificate generation")
		return nil
	}

	cf.logger.Info("Generating cluster certificates")

	if err := os.MkdirAll(cf.certDir, 0700); err != nil {
		return fmt.Errorf("failed to create cert directory: %w", err)
	}

	// Generate CA
	caKey, caCert, err := cf.generateCA()
	if err != nil {
		return fmt.Errorf("failed to generate CA: %w", err)
	}

	// Save CA
	if err := cf.saveCertAndKey(filepath.Join(cf.certDir, "ca.crt"), filepath.Join(cf.certDir, "ca.key"), caCert, caKey); err != nil {
		return fmt.Errorf("failed to save CA: %w", err)
	}

	// Generate server certificate
	serverKey, serverCert, err := cf.generateServerCert(caKey, caCert)
	if err != nil {
		return fmt.Errorf("failed to generate server certificate: %w", err)
	}

	// Save server cert
	if err := cf.saveCertAndKey(filepath.Join(cf.certDir, "server.crt"), filepath.Join(cf.certDir, "server.key"), serverCert, serverKey); err != nil {
		return fmt.Errorf("failed to save server certificate: %w", err)
	}

	cf.logger.Info("Certificates generated successfully")
	return nil
}

func (cf *ClusterFormation) generateCA() (*rsa.PrivateKey, *x509.Certificate, error) {
	// Generate key
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Keystone Core"},
			CommonName:   fmt.Sprintf("Keystone Core CA - %s", cf.config.Cluster.Name),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}

	// Self-sign
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	return key, cert, nil
}

func (cf *ClusterFormation) generateServerCert(caKey *rsa.PrivateKey, caCert *x509.Certificate) (*rsa.PrivateKey, *x509.Certificate, error) {
	// Generate key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	// Collect all hostnames and IPs
	var dnsNames []string
	var ipAddresses []net.IP

	dnsNames = append(dnsNames, "localhost")
	ipAddresses = append(ipAddresses, net.ParseIP("127.0.0.1"))
	ipAddresses = append(ipAddresses, net.ParseIP("::1"))

	// Add configured nodes
	for _, node := range cf.config.ControlPlane.Nodes {
		if ip := net.ParseIP(node.Host); ip != nil {
			ipAddresses = append(ipAddresses, ip)
		} else {
			dnsNames = append(dnsNames, node.Host)
		}
	}

	// Add cluster domain
	if cf.config.Cluster.Domain != "" {
		dnsNames = append(dnsNames, cf.config.Cluster.Domain)
		dnsNames = append(dnsNames, "*."+cf.config.Cluster.Domain)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Keystone Core"},
			CommonName:   fmt.Sprintf("Keystone Core Server - %s", cf.config.Cluster.Name),
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0), // 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	// Sign with CA
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}

	return key, cert, nil
}

func (cf *ClusterFormation) saveCertAndKey(certPath, keyPath string, cert *x509.Certificate, key *rsa.PrivateKey) error {
	// Save certificate
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return err
	}

	// Save key
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return os.WriteFile(keyPath, keyPEM, 0600)
}

// formSingleNodeCluster handles single-node cluster formation
func (cf *ClusterFormation) formSingleNodeCluster(ctx context.Context) error {
	cf.logger.Info("Forming single-node cluster")

	node := cf.config.ControlPlane.Nodes[0]
	cf.updateNodeStatus(node.Host, "initializing")

	// Install NATS if needed
	if cf.config.NATS.Mode == NATSModeEmbedded {
		cf.logger.Info("Using embedded NATS, skipping standalone installation")
	} else {
		cf.updatePhase("installing_nats")
		if err := cf.installNATS(ctx); err != nil {
			return fmt.Errorf("failed to install NATS: %w", err)
		}
	}

	// Install kscore-server
	cf.updatePhase("installing_server")
	if err := cf.installServer(ctx, node); err != nil {
		return fmt.Errorf("failed to install server: %w", err)
	}

	// Configure and start
	cf.updatePhase("configuring_server")
	if err := cf.configureServer(ctx, node, true); err != nil {
		return fmt.Errorf("failed to configure server: %w", err)
	}

	cf.updatePhase("starting_server")
	if err := cf.startServer(ctx, node); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Wait for health
	cf.updatePhase("verifying")
	if err := cf.waitForHealth(ctx, node); err != nil {
		return fmt.Errorf("server failed health check: %w", err)
	}

	cf.updateNodeStatus(node.Host, "ready")
	cf.mu.Lock()
	cf.status.NodesReady = 1
	cf.status.LeaderElected = true
	cf.status.LeaderNode = node.Host
	cf.mu.Unlock()

	cf.updatePhase("complete")
	cf.logger.Info("Single-node cluster formation complete")

	return nil
}

// formMultiNodeCluster handles multi-node cluster formation
func (cf *ClusterFormation) formMultiNodeCluster(ctx context.Context) error {
	cf.logger.Info("Forming multi-node cluster", "nodes", len(cf.config.ControlPlane.Nodes))

	// Find leader node
	var leaderNode *NodeConfig
	for i := range cf.config.ControlPlane.Nodes {
		if cf.config.ControlPlane.Nodes[i].Role == NodeRoleLeader {
			leaderNode = &cf.config.ControlPlane.Nodes[i]
			break
		}
	}
	if leaderNode == nil {
		leaderNode = &cf.config.ControlPlane.Nodes[0]
		leaderNode.Role = NodeRoleLeader
	}

	// Install NATS cluster if external
	if cf.config.NATS.Mode == NATSModeCluster {
		cf.updatePhase("installing_nats_cluster")
		if err := cf.installNATSCluster(ctx); err != nil {
			return fmt.Errorf("failed to install NATS cluster: %w", err)
		}
	}

	// Bootstrap leader first
	cf.logger.Info("Bootstrapping leader node", "host", leaderNode.Host)
	cf.updateNodeStatus(leaderNode.Host, "bootstrapping")

	cf.updatePhase("installing_leader")
	if err := cf.installServer(ctx, *leaderNode); err != nil {
		return fmt.Errorf("failed to install leader server: %w", err)
	}

	if err := cf.configureServer(ctx, *leaderNode, true); err != nil {
		return fmt.Errorf("failed to configure leader server: %w", err)
	}

	if err := cf.startServer(ctx, *leaderNode); err != nil {
		return fmt.Errorf("failed to start leader server: %w", err)
	}

	if err := cf.waitForHealth(ctx, *leaderNode); err != nil {
		return fmt.Errorf("leader failed health check: %w", err)
	}

	cf.updateNodeStatus(leaderNode.Host, "ready")
	cf.mu.Lock()
	cf.status.NodesReady = 1
	cf.status.LeaderElected = true
	cf.status.LeaderNode = leaderNode.Host
	cf.mu.Unlock()

	// Join followers
	cf.updatePhase("joining_followers")
	for _, node := range cf.config.ControlPlane.Nodes {
		if node.Host == leaderNode.Host {
			continue
		}

		cf.logger.Info("Joining follower node", "host", node.Host)
		cf.updateNodeStatus(node.Host, "joining")

		if err := cf.installServer(ctx, node); err != nil {
			cf.logger.Error("Failed to install follower", "host", node.Host, "error", err)
			cf.updateNodeStatus(node.Host, "failed")
			continue
		}

		if err := cf.configureServer(ctx, node, false); err != nil {
			cf.logger.Error("Failed to configure follower", "host", node.Host, "error", err)
			cf.updateNodeStatus(node.Host, "failed")
			continue
		}

		if err := cf.startServer(ctx, node); err != nil {
			cf.logger.Error("Failed to start follower", "host", node.Host, "error", err)
			cf.updateNodeStatus(node.Host, "failed")
			continue
		}

		if err := cf.waitForHealth(ctx, node); err != nil {
			cf.logger.Error("Follower failed health check", "host", node.Host, "error", err)
			cf.updateNodeStatus(node.Host, "unhealthy")
			continue
		}

		cf.updateNodeStatus(node.Host, "ready")
		cf.mu.Lock()
		cf.status.NodesReady++
		cf.mu.Unlock()
	}

	cf.updatePhase("complete")
	cf.logger.Info("Multi-node cluster formation complete",
		"ready", cf.status.NodesReady,
		"total", cf.status.TotalNodes)

	return nil
}

func (cf *ClusterFormation) installNATS(ctx context.Context) error {
	installer, ok := cf.registry.Get(ComponentNATS)
	if !ok {
		return fmt.Errorf("NATS installer not found")
	}

	config := ComponentConfig{
		Type:    ComponentNATS,
		Version: "2.10.0",
	}

	if err := installer.Install(ctx, config); err != nil {
		return err
	}

	if err := installer.Configure(ctx, config); err != nil {
		return err
	}

	return installer.Start(ctx)
}

func (cf *ClusterFormation) installNATSCluster(ctx context.Context) error {
	// For multi-node NATS cluster, we'd need to configure each node
	// This is a simplified implementation
	cf.logger.Info("Installing NATS cluster", "nodes", cf.config.NATS.Nodes)

	installer, ok := cf.registry.Get(ComponentNATS)
	if !ok {
		return fmt.Errorf("NATS installer not found")
	}

	config := ComponentConfig{
		Type:    ComponentNATS,
		Version: "2.10.0",
		Settings: map[string]any{
			"cluster_name": cf.config.NATS.ClusterName,
			"routes":       cf.config.NATS.Nodes,
		},
	}

	if err := installer.Install(ctx, config); err != nil {
		return err
	}

	if err := installer.Configure(ctx, config); err != nil {
		return err
	}

	return installer.Start(ctx)
}

func (cf *ClusterFormation) installServer(ctx context.Context, node NodeConfig) error {
	installer, ok := cf.registry.Get(ComponentServer)
	if !ok {
		return fmt.Errorf("server installer not found")
	}

	config := ComponentConfig{
		Type:    ComponentServer,
		Version: "latest",
	}

	return installer.Install(ctx, config)
}

func (cf *ClusterFormation) configureServer(ctx context.Context, node NodeConfig, isLeader bool) error {
	installer, ok := cf.registry.Get(ComponentServer)
	if !ok {
		return fmt.Errorf("server installer not found")
	}

	// Build NATS URLs
	var natsURLs []string
	if cf.config.NATS.Mode == NATSModeEmbedded {
		natsURLs = []string{"nats://localhost:4222"}
	} else {
		for _, n := range cf.config.NATS.Nodes {
			natsURLs = append(natsURLs, fmt.Sprintf("nats://%s:4222", n))
		}
	}

	settings := map[string]any{
		"cluster_id": cf.config.Cluster.Name,
		"nats_urls":  natsURLs,
		"api_listen": cf.config.ControlPlane.API.Listen,
	}

	// Database config
	if cf.config.Database.Type == DatabaseTypeSQLite {
		settings["database_type"] = "sqlite"
		settings["database_path"] = cf.config.Database.Path
	} else {
		settings["database_type"] = "postgresql"
		settings["database_host"] = cf.config.Database.Host
		settings["database_port"] = cf.config.Database.Port
		settings["database_name"] = cf.config.Database.Name
		settings["database_user"] = cf.config.Database.User
	}

	// TLS config
	if cf.config.ControlPlane.API.TLS.Enabled {
		settings["tls_enabled"] = true
		if cf.config.ControlPlane.API.TLS.AutoGenerate {
			settings["tls_cert_file"] = filepath.Join(cf.certDir, "server.crt")
			settings["tls_key_file"] = filepath.Join(cf.certDir, "server.key")
			settings["tls_ca_file"] = filepath.Join(cf.certDir, "ca.crt")
		} else {
			settings["tls_cert_file"] = cf.config.ControlPlane.API.TLS.CertFile
			settings["tls_key_file"] = cf.config.ControlPlane.API.TLS.KeyFile
			settings["tls_ca_file"] = cf.config.ControlPlane.API.TLS.CAFile
		}
	}

	// Clustering config
	if len(cf.config.ControlPlane.Nodes) > 1 {
		settings["cluster_enabled"] = true
		settings["etcd_mode"] = string(cf.config.Etcd.Mode)
		if cf.config.Etcd.Mode == EtcdModeEmbedded {
			settings["etcd_data_dir"] = cf.config.Etcd.DataDir
		} else {
			settings["etcd_nodes"] = cf.config.Etcd.Nodes
		}
	}

	config := ComponentConfig{
		Type:     ComponentServer,
		Settings: settings,
	}

	return installer.Configure(ctx, config)
}

func (cf *ClusterFormation) startServer(ctx context.Context, node NodeConfig) error {
	installer, ok := cf.registry.Get(ComponentServer)
	if !ok {
		return fmt.Errorf("server installer not found")
	}

	return installer.Start(ctx)
}

func (cf *ClusterFormation) waitForHealth(ctx context.Context, node NodeConfig) error {
	cf.logger.Info("Waiting for server health", "host", node.Host)

	timeout := 2 * time.Minute
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	installer, ok := cf.registry.Get(ComponentServer)
	if !ok {
		return fmt.Errorf("server installer not found")
	}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		status, err := installer.Status(ctx)
		if err == nil && status.Running && status.Healthy {
			cf.logger.Info("Server is healthy", "host", node.Host)
			return nil
		}

		time.Sleep(interval)
	}

	return fmt.Errorf("server did not become healthy within %v", timeout)
}

// Status returns the current cluster formation status
func (cf *ClusterFormation) Status() *ClusterFormationStatus {
	cf.mu.RLock()
	defer cf.mu.RUnlock()

	// Return a copy
	status := *cf.status
	status.NodeStatuses = make(map[string]string)
	for k, v := range cf.status.NodeStatuses {
		status.NodeStatuses[k] = v
	}

	return &status
}

// GenerateClusterID generates a unique cluster ID
func GenerateClusterID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate cluster ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateAdminToken generates a secure admin token
func GenerateAdminToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate admin token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GetCAFingerprint returns the SHA256 fingerprint of the CA certificate
func GetCAFingerprint(certPath string) (string, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return "", err
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}

	// Use the raw public key info for fingerprint
	fingerprint := make([]byte, 32)
	for i, b := range cert.RawSubjectPublicKeyInfo[:32] {
		fingerprint[i] = b
	}

	return hex.EncodeToString(fingerprint), nil
}
