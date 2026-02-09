package bootstrap

import (
	"fmt"
	"strings"
)

const (
	systemdServerServicePath = "/etc/systemd/system/kscore-server.service"
	systemdAgentServicePath  = "/etc/systemd/system/kscore-agent.service"
)

// FilePlan describes a file to write.
type FilePlan struct {
	Path    string
	Content []byte
	Mode    int
}

// ServicePlan captures service files and commands.
type ServicePlan struct {
	InitSystem string
	Files      []FilePlan
	Commands   []CommandPlan
}

// BuildServicePlan creates a service configuration plan based on the bootstrap config and init system.
func BuildServicePlan(cfg *Config, initSystem string) (ServicePlan, error) {
	if cfg == nil {
		return ServicePlan{}, fmt.Errorf("bootstrap config is required")
	}

	if initSystem != "systemd" {
		return ServicePlan{}, fmt.Errorf("unsupported init system: %s", initSystem)
	}

	plan := ServicePlan{InitSystem: initSystem}

	switch cfg.NodeRole {
	case "control-plane":
		if err := addServerServicePlan(cfg, &plan); err != nil {
			return ServicePlan{}, err
		}
	case "agent":
		if err := addAgentServicePlan(cfg, &plan); err != nil {
			return ServicePlan{}, err
		}
	case "both":
		if err := addServerServicePlan(cfg, &plan); err != nil {
			return ServicePlan{}, err
		}
		if err := addAgentServicePlan(cfg, &plan); err != nil {
			return ServicePlan{}, err
		}
	default:
		return ServicePlan{}, fmt.Errorf("unsupported node role: %s", cfg.NodeRole)
	}

	plan.Commands = append(plan.Commands,
		CommandPlan{Name: "systemctl", Args: []string{"daemon-reload"}},
	)

	if cfg.NodeRole == "control-plane" || cfg.NodeRole == "both" {
		plan.Commands = append(plan.Commands,
			CommandPlan{Name: "systemctl", Args: []string{"enable", "kscore-server"}},
			CommandPlan{Name: "systemctl", Args: []string{"start", "kscore-server"}},
		)
	}

	if cfg.NodeRole == "agent" || cfg.NodeRole == "both" {
		plan.Commands = append(plan.Commands,
			CommandPlan{Name: "systemctl", Args: []string{"enable", "kscore-agent"}},
			CommandPlan{Name: "systemctl", Args: []string{"start", "kscore-agent"}},
		)
	}

	return plan, nil
}

func addServerServicePlan(cfg *Config, plan *ServicePlan) error {
	serverConfig, err := buildServerConfig(cfg)
	if err != nil {
		return err
	}

	plan.Files = append(plan.Files,
		FilePlan{Path: serverConfigPath, Content: serverConfig, Mode: 0o640},
		FilePlan{Path: systemdServerServicePath, Content: []byte(systemdServerService(unitBinaryPath("kscore-server"), serverConfigPath)), Mode: 0o644},
	)
	return nil
}

func addAgentServicePlan(cfg *Config, plan *ServicePlan) error {
	agentConfig, err := buildAgentConfig(cfg)
	if err != nil {
		return err
	}

	plan.Files = append(plan.Files,
		FilePlan{Path: agentConfigPath, Content: agentConfig, Mode: 0o640},
		FilePlan{Path: systemdAgentServicePath, Content: []byte(systemdAgentService(unitBinaryPath("kscore-agent"), agentConfigPath)), Mode: 0o644},
	)
	return nil
}

func systemdServerService(binaryPath, configPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Keystone Core Server
Documentation=https://docs.keystonecore.io
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --config %s
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
`, binaryPath, configPath)
}

func systemdAgentService(binaryPath, configPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Keystone Core Agent
Documentation=https://docs.keystonecore.io
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --config %s
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
`, binaryPath, configPath)
}

func unitBinaryPath(binary string) string {
	return fmt.Sprintf("/usr/bin/%s", binary)
}

func renderServicePlan(plan ServicePlan) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("init system: %s\n", plan.InitSystem))
	builder.WriteString("files:\n")
	for _, file := range plan.Files {
		builder.WriteString(fmt.Sprintf("- %s (mode %04o)\n", file.Path, file.Mode))
	}
	builder.WriteString("commands:\n")
	for _, cmd := range plan.Commands {
		builder.WriteString(fmt.Sprintf("- %s %s\n", cmd.Name, strings.Join(cmd.Args, " ")))
	}
	return builder.String()
}
