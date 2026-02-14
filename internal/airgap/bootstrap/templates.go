package bootstrap

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// ConfigParams holds parameters for rendering configuration templates.
type ConfigParams struct {
	Version       string
	ListenAddress string
	DataDir       string
	NATSMode      string
	DatabaseType  string
	ClusterName   string
	ServerAddress string
	NATSUrl       string
}

// DefaultConfigParams returns ConfigParams with sensible defaults for air-gapped installs.
func DefaultConfigParams(version string) ConfigParams {
	return ConfigParams{
		Version:       version,
		ListenAddress: "0.0.0.0:8443",
		DataDir:       "/var/lib/keystone-core",
		NATSMode:      "embedded",
		DatabaseType:  "sqlite",
		ClusterName:   "default",
		ServerAddress: "https://localhost:8443",
		NATSUrl:       "nats://localhost:4222",
	}
}

// RenderServerConfig renders the server configuration template.
func RenderServerConfig(params ConfigParams) ([]byte, error) {
	return renderTemplate("templates/server.yaml.tmpl", params)
}

// RenderAgentConfig renders the agent configuration template.
func RenderAgentConfig(params ConfigParams) ([]byte, error) {
	return renderTemplate("templates/agent.yaml.tmpl", params)
}

// RenderInstallScript renders the install script template.
func RenderInstallScript(version, platform string) ([]byte, error) {
	data := struct {
		Version  string
		Platform string
	}{
		Version:  version,
		Platform: platform,
	}
	return renderTemplate("templates/install.sh.tmpl", data)
}

// BundleConfigTemplates renders config templates and the install script into
// stagingDir/config/ and returns content entries with checksums.
func BundleConfigTemplates(params ConfigParams, platform Platform, stagingDir string) ([]ContentEntry, error) {
	configDir := filepath.Join(stagingDir, "config")
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}

	type templateFile struct {
		name     string
		render   func() ([]byte, error)
		destDir  string
		prefix   string
		fileMode os.FileMode
	}

	files := []templateFile{
		{
			name:     "server.yaml.tmpl",
			render:   func() ([]byte, error) { return RenderServerConfig(params) },
			destDir:  configDir,
			prefix:   "config",
			fileMode: 0o640,
		},
		{
			name:     "agent.yaml.tmpl",
			render:   func() ([]byte, error) { return RenderAgentConfig(params) },
			destDir:  configDir,
			prefix:   "config",
			fileMode: 0o640,
		},
		{
			name:   "install.sh",
			render: func() ([]byte, error) { return RenderInstallScript(params.Version, platform.String()) },
			destDir: stagingDir,
			prefix:  "",
			//nolint:gosec // G302: install script needs to be executable
			fileMode: 0o750,
		},
	}

	var entries []ContentEntry
	for _, tf := range files {
		data, err := tf.render()
		if err != nil {
			return nil, fmt.Errorf("rendering %s: %w", tf.name, err)
		}

		destPath := filepath.Join(tf.destDir, tf.name)
		if err := os.WriteFile(destPath, data, tf.fileMode); err != nil {
			return nil, fmt.Errorf("writing %s: %w", tf.name, err)
		}

		hash, err := HashFile(destPath)
		if err != nil {
			return nil, fmt.Errorf("hashing %s: %w", tf.name, err)
		}

		entryPath := tf.name
		if tf.prefix != "" {
			entryPath = tf.prefix + "/" + tf.name
		}

		entries = append(entries, ContentEntry{
			Name:   tf.name,
			Path:   entryPath,
			SHA256: hash,
			Size:   int64(len(data)),
		})
	}

	return entries, nil
}

func renderTemplate(name string, data any) ([]byte, error) {
	content, err := templateFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", name, err)
	}

	tmpl, err := template.New(filepath.Base(name)).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing template %s: %w", name, err)
	}

	return buf.Bytes(), nil
}
