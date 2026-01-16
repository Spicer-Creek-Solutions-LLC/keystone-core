package vm

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config defines VM-based bootstrap test configuration.
type Config struct {
	VMProvider string     `yaml:"vm_provider"`
	SSH        SSHConfig  `yaml:"ssh"`
	Vagrant    VagrantCfg `yaml:"vagrant"`
	Cloud      CloudCfg   `yaml:"cloud"`
}

// SSHConfig holds user-provided VM connection details.
type SSHConfig struct {
	CleanNodes bool      `yaml:"clean_nodes"`
	Nodes      []SSHNode `yaml:"nodes"`
	Postgres   *Postgres `yaml:"postgres"`
}

// SSHNode describes a VM reachable over SSH.
type SSHNode struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	KeyFile  string `yaml:"key_file"`
	Password string `yaml:"password"`
	OS       string `yaml:"os"`
	Role     string `yaml:"role"`
}

// Postgres describes an external Postgres endpoint for VM tests.
type Postgres struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// VagrantCfg holds Vagrant-based VM metadata.
type VagrantCfg struct {
	BoxPrefix string       `yaml:"box_prefix"`
	Boxes     []VagrantBox `yaml:"boxes"`
}

// VagrantBox describes a Vagrant VM box to provision.
type VagrantBox struct {
	Name   string `yaml:"name"`
	Box    string `yaml:"box"`
	Memory int    `yaml:"memory"`
	CPUs   int    `yaml:"cpus"`
	Role   string `yaml:"role"`
}

// CloudCfg holds cloud VM settings.
type CloudCfg struct {
	Region        string            `yaml:"region"`
	InstanceType  string            `yaml:"instance_type"`
	KeyName       string            `yaml:"key_name"`
	SecurityGroup string            `yaml:"security_group"`
	Subnet        string            `yaml:"subnet"`
	AMIMap        map[string]string `yaml:"ami_map"`
}

// LoadConfig reads VM test configuration from disk.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vm config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse vm config: %w", err)
	}
	return &cfg, nil
}
