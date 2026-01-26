// Package ssh provides an SSH protocol adapter for proxy agents.
package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/sftp"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// SFTPAdapter implements the FileTransferAdapter interface for SSH/SFTP.
type SFTPAdapter struct {
	*Adapter
	sftpClient *sftp.Client
	sftpMu     sync.Mutex
}

// NewSFTPAdapter creates a new SFTP adapter.
func NewSFTPAdapter(config *Config) *SFTPAdapter {
	return &SFTPAdapter{
		Adapter: NewAdapter(config),
	}
}

// Connect establishes an SSH connection and starts an SFTP client.
func (a *SFTPAdapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	// First establish SSH connection
	if err := a.Adapter.Connect(ctx, device, cred); err != nil {
		return err
	}

	// Create SFTP client
	a.sftpMu.Lock()
	defer a.sftpMu.Unlock()

	client := a.Adapter.Client()
	if client == nil {
		return fmt.Errorf("SSH client not available")
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}

	a.sftpClient = sftpClient
	return nil
}

// Disconnect closes the SFTP client and SSH connection.
func (a *SFTPAdapter) Disconnect(ctx context.Context) error {
	a.sftpMu.Lock()
	if a.sftpClient != nil {
		_ = a.sftpClient.Close()
		a.sftpClient = nil
	}
	a.sftpMu.Unlock()

	return a.Adapter.Disconnect(ctx)
}

// SFTPClient returns the underlying SFTP client.
func (a *SFTPAdapter) SFTPClient() *sftp.Client {
	a.sftpMu.Lock()
	defer a.sftpMu.Unlock()
	return a.sftpClient
}

// Upload uploads a file to the device.
func (a *SFTPAdapter) Upload(ctx context.Context, req *protocols.UploadRequest) (*protocols.TransferResult, error) {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected")
	}

	result := &protocols.TransferResult{}
	start := time.Now()

	// Get content reader
	var reader io.Reader
	if req.Content != nil {
		reader = req.Content
	} else if req.LocalPath != "" {
		f, err := os.Open(req.LocalPath)
		if err != nil {
			result.Error = fmt.Sprintf("failed to open local file: %v", err)
			result.Duration = time.Since(start)
			return result, err
		}
		defer f.Close()
		reader = f
	} else {
		return nil, fmt.Errorf("either Content or LocalPath must be provided")
	}

	// Create parent directories if needed
	if req.CreateDirs {
		dir := filepath.Dir(req.RemotePath)
		if err := a.mkdirAll(sftpClient, dir); err != nil {
			result.Error = fmt.Sprintf("failed to create directories: %v", err)
			result.Duration = time.Since(start)
			return result, err
		}
	}

	// Check if file exists
	if !req.Overwrite {
		if _, err := sftpClient.Stat(req.RemotePath); err == nil {
			result.Error = "file already exists and overwrite is disabled"
			result.Duration = time.Since(start)
			return result, fmt.Errorf("file already exists: %s", req.RemotePath)
		}
	}

	// Create remote file
	remoteFile, err := sftpClient.Create(req.RemotePath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create remote file: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}
	defer remoteFile.Close()

	// Copy content with checksum
	hasher := sha256.New()
	writer := io.MultiWriter(remoteFile, hasher)

	written, err := io.Copy(writer, reader)
	if err != nil {
		result.Error = fmt.Sprintf("failed to copy content: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	result.BytesTransferred = written
	result.Duration = time.Since(start)
	result.Checksum = hex.EncodeToString(hasher.Sum(nil))
	result.ChecksumType = "sha256"

	// Set file permissions
	if req.Mode != 0 {
		if err := sftpClient.Chmod(req.RemotePath, os.FileMode(req.Mode)); err != nil {
			// Log but don't fail the upload
			result.Error = fmt.Sprintf("warning: failed to set permissions: %v", err)
		}
	}

	return result, nil
}

// Download downloads a file from the device.
func (a *SFTPAdapter) Download(ctx context.Context, req *protocols.DownloadRequest) (*protocols.TransferResult, error) {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected")
	}

	result := &protocols.TransferResult{}
	start := time.Now()

	// Open remote file
	remoteFile, err := sftpClient.Open(req.RemotePath)
	if err != nil {
		result.Error = fmt.Sprintf("failed to open remote file: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}
	defer remoteFile.Close()

	// Get writer
	var writer io.Writer
	var localFile *os.File

	if req.Writer != nil {
		writer = req.Writer
	} else if req.LocalPath != "" {
		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(req.LocalPath), 0755); err != nil {
			result.Error = fmt.Sprintf("failed to create local directories: %v", err)
			result.Duration = time.Since(start)
			return result, err
		}

		localFile, err = os.Create(req.LocalPath)
		if err != nil {
			result.Error = fmt.Sprintf("failed to create local file: %v", err)
			result.Duration = time.Since(start)
			return result, err
		}
		defer localFile.Close()
		writer = localFile
	} else {
		return nil, fmt.Errorf("either Writer or LocalPath must be provided")
	}

	// Copy content with checksum
	hasher := sha256.New()
	multiWriter := io.MultiWriter(writer, hasher)

	written, err := io.Copy(multiWriter, remoteFile)
	if err != nil {
		result.Error = fmt.Sprintf("failed to copy content: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	result.BytesTransferred = written
	result.Duration = time.Since(start)
	result.Checksum = hex.EncodeToString(hasher.Sum(nil))
	result.ChecksumType = "sha256"

	return result, nil
}

// Stat returns file information.
func (a *SFTPAdapter) Stat(ctx context.Context, path string) (*protocols.FileInfo, error) {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected")
	}

	info, err := sftpClient.Stat(path)
	if err != nil {
		return nil, err
	}

	return a.convertFileInfo(path, info), nil
}

// Lstat returns file information without following symlinks.
func (a *SFTPAdapter) Lstat(ctx context.Context, path string) (*protocols.FileInfo, error) {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected")
	}

	info, err := sftpClient.Lstat(path)
	if err != nil {
		return nil, err
	}

	fileInfo := a.convertFileInfo(path, info)

	// Check if symlink
	if info.Mode()&os.ModeSymlink != 0 {
		fileInfo.IsSymlink = true
		target, err := sftpClient.ReadLink(path)
		if err == nil {
			fileInfo.LinkTarget = target
		}
	}

	return fileInfo, nil
}

// ReadDir lists directory contents.
func (a *SFTPAdapter) ReadDir(ctx context.Context, path string) ([]protocols.FileInfo, error) {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected")
	}

	entries, err := sftpClient.ReadDir(path)
	if err != nil {
		return nil, err
	}

	result := make([]protocols.FileInfo, 0, len(entries))
	for _, entry := range entries {
		fullPath := filepath.Join(path, entry.Name())
		result = append(result, *a.convertFileInfo(fullPath, entry))
	}

	return result, nil
}

// Mkdir creates a directory.
func (a *SFTPAdapter) Mkdir(ctx context.Context, path string, mode uint32) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	if err := sftpClient.Mkdir(path); err != nil {
		return err
	}

	if mode != 0 {
		return sftpClient.Chmod(path, os.FileMode(mode))
	}

	return nil
}

// MkdirAll creates a directory and all parent directories.
func (a *SFTPAdapter) MkdirAll(ctx context.Context, path string, mode uint32) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	return a.mkdirAll(sftpClient, path)
}

// mkdirAll creates all directories in path (helper function).
func (a *SFTPAdapter) mkdirAll(client *sftp.Client, path string) error {
	// Clean the path
	path = filepath.Clean(path)

	// Check if already exists
	_, err := client.Stat(path)
	if err == nil {
		return nil
	}

	// Create parent first
	parent := filepath.Dir(path)
	if parent != path && parent != "/" && parent != "." {
		if err := a.mkdirAll(client, parent); err != nil {
			return err
		}
	}

	// Create this directory
	if err := client.Mkdir(path); err != nil {
		// Check if someone else created it
		if _, statErr := client.Stat(path); statErr == nil {
			return nil
		}
		return err
	}

	return nil
}

// Remove removes a file or empty directory.
func (a *SFTPAdapter) Remove(ctx context.Context, path string) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	// Check if it's a directory
	info, err := sftpClient.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return sftpClient.RemoveDirectory(path)
	}

	return sftpClient.Remove(path)
}

// RemoveAll removes a file or directory recursively.
func (a *SFTPAdapter) RemoveAll(ctx context.Context, path string) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	return a.removeAll(sftpClient, path)
}

// removeAll recursively removes a path.
func (a *SFTPAdapter) removeAll(client *sftp.Client, path string) error {
	info, err := client.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return client.Remove(path)
	}

	// Remove directory contents first
	entries, err := client.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(path, entry.Name())
		if err := a.removeAll(client, entryPath); err != nil {
			return err
		}
	}

	return client.RemoveDirectory(path)
}

// Rename renames a file or directory.
func (a *SFTPAdapter) Rename(ctx context.Context, oldPath, newPath string) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	return sftpClient.Rename(oldPath, newPath)
}

// Chmod changes file permissions.
func (a *SFTPAdapter) Chmod(ctx context.Context, path string, mode uint32) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	return sftpClient.Chmod(path, os.FileMode(mode))
}

// Chown changes file ownership.
func (a *SFTPAdapter) Chown(ctx context.Context, path string, uid, gid int) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	return sftpClient.Chown(path, uid, gid)
}

// Symlink creates a symbolic link.
func (a *SFTPAdapter) Symlink(ctx context.Context, target, link string) error {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return fmt.Errorf("SFTP client not connected")
	}

	return sftpClient.Symlink(target, link)
}

// ReadLink reads the target of a symbolic link.
func (a *SFTPAdapter) ReadLink(ctx context.Context, path string) (string, error) {
	a.sftpMu.Lock()
	sftpClient := a.sftpClient
	a.sftpMu.Unlock()

	if sftpClient == nil {
		return "", fmt.Errorf("SFTP client not connected")
	}

	return sftpClient.ReadLink(path)
}

// convertFileInfo converts os.FileInfo to protocols.FileInfo.
func (a *SFTPAdapter) convertFileInfo(path string, info os.FileInfo) *protocols.FileInfo {
	return &protocols.FileInfo{
		Name:    info.Name(),
		Path:    path,
		Size:    info.Size(),
		Mode:    uint32(info.Mode().Perm()),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}
}

// NewSFTPAdapterFactory creates a file transfer adapter factory for SSH/SFTP.
func NewSFTPAdapterFactory(config *Config) protocols.FileTransferAdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.FileTransferAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultConfig()
		}
		cfg.ConnectionConfig = connConfig
		return NewSFTPAdapter(cfg), nil
	}
}

// init registers the SFTP adapter with the default registry.
func init() {
	protocols.RegisterFileTransfer(protocols.ProtocolSSH, NewSFTPAdapterFactory(nil))
}
