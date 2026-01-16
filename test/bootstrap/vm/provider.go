package vm

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Provider manages VM lifecycle and access.
type Provider interface {
	Setup(ctx context.Context) error
	GetNode(name string) (*Node, error)
	ListNodes() []*Node
	Cleanup(ctx context.Context) error
}

// Node represents a VM reachable over SSH.
type Node struct {
	Name     string
	Host     string
	Port     int
	User     string
	OS       string
	Role     string
	KeyFile  string
	Password string

	SSHClient *ssh.Client
}

// ExecResult captures SSH command output.
type ExecResult struct {
	Stdout   string
	ExitCode int
	Stderr   string
}

// Exec executes a command on the node.
func (n *Node) Exec(ctx context.Context, command string, args ...string) (*ExecResult, error) {
	if n.SSHClient == nil {
		return nil, fmt.Errorf("ssh client not initialized")
	}
	session, err := n.SSHClient.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	cmd := strings.TrimSpace(strings.Join(append([]string{command}, args...), " "))
	output, err := runSession(ctx, session, cmd)
	if err != nil {
		return &ExecResult{Stdout: string(output), ExitCode: 1, Stderr: err.Error()}, err
	}
	return &ExecResult{Stdout: string(output), ExitCode: 0}, nil
}

// CopyFile copies a local file to the node via SSH.
func (n *Node) CopyFile(ctx context.Context, localPath, remotePath string, mode os.FileMode) error {
	if n.SSHClient == nil {
		return fmt.Errorf("ssh client not initialized")
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return err
	}

	session, err := n.SSHClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	if err := session.RequestPty("xterm", 80, 40, ssh.TerminalModes{}); err != nil {
		return err
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("install -m %o /dev/stdin %s", mode, remotePath)
	if err := session.Start(cmd); err != nil {
		return err
	}

	if _, err := io.Copy(stdin, strings.NewReader(string(data))); err != nil {
		_ = session.Close()
		return err
	}
	_ = stdin.Close()
	return session.Wait()
}

// FetchFile retrieves a remote file to a local path.
func (n *Node) FetchFile(ctx context.Context, remotePath, localPath string) error {
	if n.SSHClient == nil {
		return fmt.Errorf("ssh client not initialized")
	}
	session, err := n.SSHClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	output, err := runSession(ctx, session, fmt.Sprintf("cat %s", remotePath))
	if err != nil {
		return err
	}
	return os.WriteFile(localPath, output, 0o600)
}

// WaitForSSH waits for SSH connectivity.
func (n *Node) WaitForSSH(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client, err := dialSSH(n)
		if err == nil {
			n.SSHClient = client
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for ssh")
}

func runSession(ctx context.Context, session *ssh.Session, cmd string) ([]byte, error) {
	done := make(chan struct{})
	var output []byte
	var err error

	go func() {
		output, err = session.CombinedOutput(cmd)
		close(done)
	}()

	select {
	case <-ctx.Done():
		_ = session.Close()
		return output, ctx.Err()
	case <-done:
		return output, err
	}
}

func dialSSH(node *Node) (*ssh.Client, error) {
	auths, err := sshAuthMethods(node)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User:            node.User,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	address := fmt.Sprintf("%s:%d", node.Host, node.Port)
	return ssh.Dial("tcp", address, config)
}

func sshAuthMethods(node *Node) ([]ssh.AuthMethod, error) {
	if node.KeyFile != "" {
		keyData, err := os.ReadFile(node.KeyFile)
		if err != nil {
			return nil, err
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	if node.Password != "" {
		return []ssh.AuthMethod{ssh.Password(node.Password)}, nil
	}
	return nil, fmt.Errorf("no ssh authentication configured")
}
