package bootstrap

import (
	"fmt"
	"strings"

	"github.com/shawnbutts/keystone-core/pkg/platform"
)

// RepoPlan describes repository configuration changes.
type RepoPlan struct {
	Manager platform.PackageManager
	KeyURL  string
	KeyPath string
	Files   map[string]string
}

// BuildRepoPlan returns repository configuration for a package manager.
func BuildRepoPlan(manager platform.PackageManager, channel string) (RepoPlan, error) {
	if channel == "" {
		channel = "stable"
	}

	switch manager {
	case platform.PackageManagerAPT:
		return RepoPlan{
			Manager: manager,
			KeyURL:  "https://apt.keystonecore.io/gpg",
			KeyPath: "/usr/share/keyrings/kscore.gpg",
			Files: map[string]string{
				"/etc/apt/sources.list.d/kscore.list": fmt.Sprintf(
					"deb [signed-by=/usr/share/keyrings/kscore.gpg] https://apt.keystonecore.io %s main\n",
					channel,
				),
			},
		}, nil
	case platform.PackageManagerYum, platform.PackageManagerDNF:
		return RepoPlan{
			Manager: manager,
			KeyURL:  "https://yum.keystonecore.io/gpg",
			Files: map[string]string{
				"/etc/yum.repos.d/kscore.repo": fmt.Sprintf(`[kscore]
name=Keystone Core Repository
baseurl=https://yum.keystonecore.io/%s/$basearch
enabled=1
gpgcheck=1
gpgkey=https://yum.keystonecore.io/gpg
`, channel),
			},
		}, nil
	case platform.PackageManagerZypper:
		return RepoPlan{
			Manager: manager,
			KeyURL:  "https://zypper.keystonecore.io/gpg",
			Files: map[string]string{
				"/etc/zypp/repos.d/kscore.repo": fmt.Sprintf(`[kscore]
name=Keystone Core Repository
baseurl=https://zypper.keystonecore.io/%s/$basearch
enabled=1
autorefresh=1
type=rpm-md
gpgcheck=1
gpgkey=https://zypper.keystonecore.io/gpg
`, channel),
			},
		}, nil
	case platform.PackageManagerAPK:
		return RepoPlan{
			Manager: manager,
			KeyURL:  "https://apk.keystonecore.io/gpg",
			Files: map[string]string{
				"/etc/apk/repositories": fmt.Sprintf("https://apk.keystonecore.io/%s/main\n", channel),
			},
		}, nil
	default:
		return RepoPlan{}, fmt.Errorf("unsupported package manager: %s", manager)
	}
}

func renderRepoPlan(plan RepoPlan) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("repo manager: %s\n", plan.Manager))
	if plan.KeyURL != "" {
		builder.WriteString(fmt.Sprintf("gpg key: %s\n", plan.KeyURL))
	}
	if plan.KeyPath != "" {
		builder.WriteString(fmt.Sprintf("gpg key path: %s\n", plan.KeyPath))
	}
	for path, content := range plan.Files {
		builder.WriteString(fmt.Sprintf("file: %s\n%s", path, content))
	}
	return builder.String()
}
