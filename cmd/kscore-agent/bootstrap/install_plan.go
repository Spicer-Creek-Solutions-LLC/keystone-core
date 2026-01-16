package bootstrap

import (
	"fmt"
	"strings"

	"github.com/shawnbutts/keystone-core/pkg/platform"
)

// CommandPlan describes a command to execute.
type CommandPlan struct {
	Name string
	Args []string
}

// PackagePlan captures repository configuration and package installation steps.
type PackagePlan struct {
	Manager  platform.PackageManager
	RepoPlan RepoPlan
	Packages []string
	Commands []CommandPlan
}

// BuildPackagePlan constructs repository and install commands for a package manager.
func BuildPackagePlan(manager platform.PackageManager, channel string, packages []string, version string) (PackagePlan, error) {
	if len(packages) == 0 {
		return PackagePlan{}, fmt.Errorf("no packages specified")
	}

	repoPlan, err := BuildRepoPlan(manager, channel)
	if err != nil {
		return PackagePlan{}, err
	}

	packages = formatPackageNames(manager, packages, version)
	plan := PackagePlan{
		Manager:  manager,
		RepoPlan: repoPlan,
		Packages: append([]string(nil), packages...),
	}

	switch manager {
	case platform.PackageManagerAPT:
		plan.Commands = []CommandPlan{
			{Name: "apt-get", Args: []string{"update"}},
			{Name: "apt-get", Args: append([]string{"install", "-y"}, packages...)},
		}
	case platform.PackageManagerDNF, platform.PackageManagerYum:
		plan.Commands = []CommandPlan{
			{Name: manager.String(), Args: append([]string{"install", "-y"}, packages...)},
		}
	case platform.PackageManagerZypper:
		plan.Commands = []CommandPlan{
			{Name: "zypper", Args: append([]string{"--non-interactive", "install", "-y"}, packages...)},
		}
	case platform.PackageManagerAPK:
		plan.Commands = []CommandPlan{
			{Name: "apk", Args: append([]string{"add"}, packages...)},
		}
	default:
		return PackagePlan{}, fmt.Errorf("unsupported package manager: %s", manager)
	}

	return plan, nil
}

func formatPackageNames(manager platform.PackageManager, packages []string, version string) []string {
	if version == "" {
		return append([]string(nil), packages...)
	}
	formatted := make([]string, 0, len(packages))
	for _, pkg := range packages {
		formatted = append(formatted, formatPackageName(manager, pkg, version))
	}
	return formatted
}

func formatPackageName(manager platform.PackageManager, pkg, version string) string {
	switch manager {
	case platform.PackageManagerAPT, platform.PackageManagerAPK:
		return fmt.Sprintf("%s=%s", pkg, version)
	case platform.PackageManagerDNF, platform.PackageManagerYum, platform.PackageManagerZypper:
		return fmt.Sprintf("%s-%s", pkg, version)
	default:
		return pkg
	}
}

func renderPackagePlan(plan PackagePlan) string {
	var builder strings.Builder
	builder.WriteString(renderRepoPlan(plan.RepoPlan))
	builder.WriteString("\npackages:\n")
	for _, pkg := range plan.Packages {
		builder.WriteString(fmt.Sprintf("- %s\n", pkg))
	}
	builder.WriteString("commands:\n")
	for _, cmd := range plan.Commands {
		builder.WriteString(fmt.Sprintf("- %s %s\n", cmd.Name, strings.Join(cmd.Args, " ")))
	}
	return builder.String()
}

func packagesForRole(role string) []string {
	switch strings.ToLower(role) {
	case "control-plane":
		return []string{"kscore-server"}
	case "agent":
		return []string{"kscore-agent"}
	case "both":
		return []string{"kscore-server", "kscore-agent"}
	default:
		return nil
	}
}
