package framework

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// VolumeMount describes a host->container volume mount.
type VolumeMount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// ExecResult captures stdout/stderr from a command execution.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
}

// DockerEnv manages Docker-based bootstrap test environments.
type DockerEnv struct {
	Registry      string
	CacheFrom     bool
	Platform      Platform
	ImageTag      string
	ContainerName string
	NetworkName   string
	ArtifactsDir  string
	BuildContext  string
	Dockerfile    string
	Volumes       []VolumeMount
	Tmpfs         []string
	Env           map[string]string
	Privileged    bool
	SkipBuild     bool
	Command       []string
	CgroupNSHost  bool
}

// NewDockerEnv creates a Docker test environment from config.
func NewDockerEnv(cfg *Config) *DockerEnv {
	registry := ""
	cacheFrom := false
	if cfg != nil {
		registry = cfg.Docker.Registry
		cacheFrom = cfg.Docker.CacheFrom
	}
	return &DockerEnv{
		Registry:   registry,
		CacheFrom:  cacheFrom,
		Privileged: true,
	}
}

// NewDockerEnvForPlatform configures a DockerEnv for a specific platform.
func NewDockerEnvForPlatform(cfg *Config, platformName, artifactsDir string) (*DockerEnv, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	platform, err := findPlatform(cfg, platformName)
	if err != nil {
		return nil, err
	}

	root, err := RepoRoot()
	if err != nil {
		return nil, fmt.Errorf("locate repo root: %w", err)
	}

	env := NewDockerEnv(cfg)
	env.Platform = platform
	env.BuildContext = filepath.Join(root, "test", "bootstrap", "containers")
	env.Dockerfile = filepath.Join(env.BuildContext, platform.Image)
	env.ImageTag = imageTag(cfg.Docker.Registry, platform.Name)
	env.SkipBuild = os.Getenv("KSCORE_SKIP_BUILD") == "1"

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	env.ContainerName = fmt.Sprintf("%s-%s", sanitizeName(platform.Name), suffix)
	env.NetworkName = fmt.Sprintf("kscore-bootstrap-%s", suffix)

	if artifactsDir != "" {
		env.ArtifactsDir = artifactsDir
		env.Volumes = append(env.Volumes, VolumeMount{
			Source:   artifactsDir,
			Target:   "/test-artifacts",
			ReadOnly: false,
		})
	}

	useSystemd := isSystemdPlatform(platform.Name)
	env.Command = defaultCommand(platform.Name)
	env.CgroupNSHost = useSystemd
	env.Volumes = append(env.Volumes, VolumeMount{
		Source:   "/sys/fs/cgroup",
		Target:   "/sys/fs/cgroup",
		ReadOnly: !useSystemd,
	})
	if useSystemd {
		env.Tmpfs = append(env.Tmpfs, "/run", "/run/lock")
		if env.Env == nil {
			env.Env = make(map[string]string)
		}
		env.Env["container"] = "docker"
	}

	return env, nil
}

// Start prepares Docker resources for testing.
func (d *DockerEnv) Start(ctx context.Context) error {
	if d.Platform.Name == "" {
		return errors.New("platform not set")
	}
	if d.BuildContext == "" || d.Dockerfile == "" {
		return errors.New("build context and dockerfile are required")
	}

	if d.ArtifactsDir != "" {
		if err := os.MkdirAll(d.ArtifactsDir, 0o755); err != nil {
			return fmt.Errorf("create artifacts dir: %w", err)
		}
	}

	if !d.SkipBuild {
		if err := d.buildImage(ctx); err != nil {
			return err
		}
	}
	if err := d.createNetwork(ctx); err != nil {
		return err
	}
	if err := d.startContainer(ctx); err != nil {
		_ = d.removeNetwork(ctx)
		return err
	}
	return nil
}

// Stop cleans up Docker resources.
func (d *DockerEnv) Stop(ctx context.Context) error {
	err := d.removeContainer(ctx)
	if err != nil {
		_ = d.removeNetwork(ctx)
		return err
	}
	return d.removeNetwork(ctx)
}

// Exec runs a command in the container.
func (d *DockerEnv) Exec(ctx context.Context, command ...string) ExecResult {
	if d.ContainerName == "" {
		return ExecResult{ExitCode: -1, Error: errors.New("container not started")}
	}

	args := append([]string{"exec", d.ContainerName}, command...)
	return runDocker(ctx, args...)
}

// CopyFile copies a file into the container.
func (d *DockerEnv) CopyFile(ctx context.Context, source, target string) error {
	if d.ContainerName == "" {
		return errors.New("container not started")
	}
	result := runDocker(ctx, "cp", source, fmt.Sprintf("%s:%s", d.ContainerName, target))
	return result.Error
}

// CopyDir copies a directory into the container.
func (d *DockerEnv) CopyDir(ctx context.Context, source, target string) error {
	return d.CopyFile(ctx, source, target)
}

// Logs returns recent container logs.
func (d *DockerEnv) Logs(ctx context.Context, tail int) (string, error) {
	if d.ContainerName == "" {
		return "", errors.New("container not started")
	}
	result := runDocker(ctx, "logs", "--tail", fmt.Sprintf("%d", tail), d.ContainerName)
	if result.Error != nil {
		return "", result.Error
	}
	return result.Stdout, nil
}

func (d *DockerEnv) buildImage(ctx context.Context) error {
	args := []string{
		"build",
		"-t", d.ImageTag,
		"-f", d.Dockerfile,
	}

	for key, value := range d.Platform.Args {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}

	if d.CacheFrom {
		args = append(args, "--cache-from", d.ImageTag)
	}

	args = append(args, d.BuildContext)
	result := runDocker(ctx, args...)
	if result.Error != nil {
		return fmt.Errorf("docker build failed: %w: %s", result.Error, result.Stderr)
	}
	return nil
}

func (d *DockerEnv) createNetwork(ctx context.Context) error {
	if d.NetworkName == "" {
		return nil
	}
	result := runDocker(ctx, "network", "create", d.NetworkName)
	if result.Error != nil {
		return fmt.Errorf("docker network create failed: %w: %s", result.Error, result.Stderr)
	}
	return nil
}

func (d *DockerEnv) startContainer(ctx context.Context) error {
	args := []string{
		"run",
		"-d",
		"--name", d.ContainerName,
	}

	if d.Privileged {
		args = append(args, "--privileged")
	}
	if d.NetworkName != "" {
		args = append(args, "--network", d.NetworkName)
	}
	if d.CgroupNSHost {
		args = append(args, "--cgroupns=host")
	}

	for _, mount := range d.Volumes {
		mode := "rw"
		if mount.ReadOnly {
			mode = "ro"
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s:%s", mount.Source, mount.Target, mode))
	}
	for _, tmpfs := range d.Tmpfs {
		args = append(args, "--tmpfs", tmpfs)
	}

	for key, value := range d.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	args = append(args, d.ImageTag)
	if len(d.Command) > 0 {
		args = append(args, d.Command...)
	}
	result := runDocker(ctx, args...)
	if result.Error != nil {
		return fmt.Errorf("docker run failed: %w: %s", result.Error, result.Stderr)
	}
	return nil
}

func (d *DockerEnv) removeContainer(ctx context.Context) error {
	if d.ContainerName == "" {
		return nil
	}
	result := runDocker(ctx, "rm", "-f", d.ContainerName)
	if result.Error != nil {
		return fmt.Errorf("docker rm failed: %w: %s", result.Error, result.Stderr)
	}
	return nil
}

func (d *DockerEnv) removeNetwork(ctx context.Context) error {
	if d.NetworkName == "" {
		return nil
	}
	result := runDocker(ctx, "network", "rm", d.NetworkName)
	if result.Error != nil {
		if strings.Contains(result.Stderr, "No such network") {
			return nil
		}
		return fmt.Errorf("docker network rm failed: %w: %s", result.Error, result.Stderr)
	}
	return nil
}

func findPlatform(cfg *Config, name string) (Platform, error) {
	for _, platform := range cfg.Platforms {
		if platform.Name == name {
			return platform, nil
		}
	}
	return Platform{}, fmt.Errorf("platform %q not found", name)
}

func imageTag(registry, platformName string) string {
	tag := fmt.Sprintf("kscore-bootstrap:%s", sanitizeName(platformName))
	if registry == "" {
		return tag
	}
	return strings.TrimSuffix(registry, "/") + "/" + tag
}

func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '-'
		}
	}, name)
}

func defaultCommand(platformName string) []string {
	switch {
	case strings.HasPrefix(platformName, "ubuntu"),
		strings.HasPrefix(platformName, "debian"):
		return []string{"/lib/systemd/systemd"}
	case strings.HasPrefix(platformName, "rhel"),
		strings.HasPrefix(platformName, "rocky"),
		strings.HasPrefix(platformName, "fedora"):
		return []string{"/usr/sbin/init"}
	case strings.HasPrefix(platformName, "alpine"):
		return []string{"sh", "-c", "tail -f /dev/null"}
	default:
		return nil
	}
}

func isSystemdPlatform(platformName string) bool {
	switch {
	case strings.HasPrefix(platformName, "ubuntu"),
		strings.HasPrefix(platformName, "debian"),
		strings.HasPrefix(platformName, "rhel"),
		strings.HasPrefix(platformName, "rocky"),
		strings.HasPrefix(platformName, "fedora"):
		return true
	default:
		return false
	}
}

// RepoRoot locates the repository root by searching for go.mod.
func RepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", os.ErrNotExist
		}
		wd = parent
	}
}

func runDocker(ctx context.Context, args ...string) ExecResult {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}

	return ExecResult{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
		Error:    err,
	}
}
