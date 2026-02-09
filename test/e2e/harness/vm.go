package harness

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"github.com/shawnbutts/keystone-core/test/bootstrap/vm"
)

const (
	envE2EMode   = "KSCORE_E2E_MODE"
	envE2EConfig = "KSCORE_E2E_CONFIG"
	envVMConfig  = "KSCORE_VM_CONFIG"
)

// IsVMMode reports whether VM-based E2E mode is enabled.
func IsVMMode() bool {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(envE2EMode)))
	return mode == "vm"
}

// LoadVMConfig resolves and loads the VM config path for E2E tests.
func LoadVMConfig(configPath string) (*vm.Config, error) {
	if configPath == "" {
		configPath = os.Getenv(envE2EConfig)
	}
	if configPath == "" {
		configPath = os.Getenv(envVMConfig)
	}
	if configPath == "" {
		configPath = "test/bootstrap/vm/config.yaml"
	}
	return vm.LoadConfig(configPath)
}

// ConfigFromVM builds a test harness config from a VM inventory.
func ConfigFromVM(configPath string) (cfg *Config, vmCfg *vm.Config, err error) {
	vmCfg, err = LoadVMConfig(configPath)
	if err != nil {
		return nil, nil, err
	}

	cpNodes := nodesByRole(vmCfg, "control-plane", "server")
	if len(cpNodes) == 0 {
		return nil, nil, fmt.Errorf("no control-plane nodes found in VM config")
	}
	sort.Slice(cpNodes, func(i, j int) bool {
		return cpNodes[i].Name < cpNodes[j].Name
	})
	cp := cpNodes[0]

	cfg = DefaultConfig()
	cfg.Mode = "vm"
	cfg.ComposeFile = ""
	cfg.BuildImages = false
	cfg.ServerGRPCAddr = formatHostPort(cp.Host, cfg.ServerGRPCPort)
	cfg.ServerHTTPAddr = formatHTTPAddr(cp.Host, cfg.ServerHTTPPort)
	cfg.WebhookAddr = formatHTTPAddr(cp.Host, cfg.WebhookPort)

	return cfg, vmCfg, nil
}

// HAClusterConfigFromVM builds an HA config from a VM inventory.
func HAClusterConfigFromVM(configPath string, expectedAgents int) (*HAClusterConfig, *vm.Config, vm.Provider, error) {
	vmCfg, err := LoadVMConfig(configPath)
	if err != nil {
		return nil, nil, nil, err
	}

	cpNodes := nodesByRole(vmCfg, "control-plane", "server")
	if len(cpNodes) == 0 {
		return nil, nil, nil, fmt.Errorf("no control-plane nodes found in VM config")
	}
	sort.Slice(cpNodes, func(i, j int) bool {
		return cpNodes[i].Name < cpNodes[j].Name
	})

	servers := make([]ServerInfo, 0, len(cpNodes))
	for _, node := range cpNodes {
		grpcPort := 8080
		httpPort := 8081
		servers = append(servers, ServerInfo{
			Name:     node.Name,
			GRPCAddr: formatHostPort(node.Host, grpcPort),
			HTTPAddr: formatHTTPAddr(node.Host, httpPort),
		})
	}

	cfg := DefaultHAClusterConfig()
	cfg.Mode = "vm"
	cfg.ComposeFile = ""
	cfg.BuildImages = false
	cfg.Servers = servers
	if expectedAgents > 0 {
		cfg.ExpectedAgents = expectedAgents
	}

	provider := vm.NewSSHProvider(&vmCfg.SSH)
	cfg.VMProvider = provider

	return cfg, vmCfg, provider, nil
}

func nodesByRole(cfg *vm.Config, roles ...string) []vm.SSHNode {
	if cfg == nil {
		return nil
	}
	roleSet := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		roleSet[role] = struct{}{}
	}
	var nodes []vm.SSHNode
	for _, node := range cfg.SSH.Nodes {
		if _, ok := roleSet[node.Role]; ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func formatHostPort(host string, port int) string {
	if host == "" {
		return fmt.Sprintf("localhost:%d", port)
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

func formatHTTPAddr(host string, port int) string {
	if host == "" {
		return fmt.Sprintf("http://localhost:%d", port)
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
}
