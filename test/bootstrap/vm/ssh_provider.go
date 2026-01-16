package vm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SSHProvider connects to user-provided VMs over SSH.
type SSHProvider struct {
	Config *SSHConfig
	nodes  map[string]*Node
}

// NewSSHProvider constructs a provider from config.
func NewSSHProvider(config *SSHConfig) *SSHProvider {
	return &SSHProvider{
		Config: config,
		nodes:  make(map[string]*Node),
	}
}

// Setup connects to all configured nodes.
func (p *SSHProvider) Setup(ctx context.Context) error {
	if p.Config == nil {
		return fmt.Errorf("ssh config is required")
	}
	for _, nodeCfg := range p.Config.Nodes {
		node := &Node{
			Name:     nodeCfg.Name,
			Host:     nodeCfg.Host,
			Port:     nodeCfg.Port,
			User:     nodeCfg.User,
			OS:       nodeCfg.OS,
			Role:     nodeCfg.Role,
			KeyFile:  expandPath(nodeCfg.KeyFile),
			Password: nodeCfg.Password,
		}
		if node.Port == 0 {
			node.Port = 22
		}

		if err := node.WaitForSSH(2 * time.Minute); err != nil {
			return fmt.Errorf("connect %s: %w", node.Name, err)
		}
		p.nodes[node.Name] = node
	}
	return nil
}

// GetNode returns a node by name.
func (p *SSHProvider) GetNode(name string) (*Node, error) {
	node, ok := p.nodes[name]
	if !ok {
		return nil, fmt.Errorf("node %q not found", name)
	}
	return node, nil
}

// ListNodes returns all configured nodes.
func (p *SSHProvider) ListNodes() []*Node {
	nodes := make([]*Node, 0, len(p.nodes))
	for _, node := range p.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// Cleanup closes all SSH sessions.
func (p *SSHProvider) Cleanup(ctx context.Context) error {
	for _, node := range p.nodes {
		if node.SSHClient != nil {
			_ = node.SSHClient.Close()
		}
	}
	return nil
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}
