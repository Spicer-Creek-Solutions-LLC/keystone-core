// Package winrm provides a WinRM protocol adapter for proxy agents.
package winrm

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileManager provides file operations over WinRM.
type FileManager struct {
	adapter *Adapter

	// ChunkSize is the size of chunks for file transfer (default 64KB).
	ChunkSize int
}

// NewFileManager creates a new file manager.
func (a *Adapter) NewFileManager() *FileManager {
	return &FileManager{
		adapter:   a,
		ChunkSize: 64 * 1024, // 64KB chunks
	}
}

// UploadFile uploads a local file to a remote path.
func (m *FileManager) UploadFile(ctx context.Context, localPath, remotePath string) error {
	// Read local file
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}

	return m.UploadBytes(ctx, data, remotePath)
}

// UploadBytes uploads byte data to a remote path.
func (m *FileManager) UploadBytes(ctx context.Context, data []byte, remotePath string) error {
	// Escape remote path for PowerShell
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")

	// For small files, upload directly
	if len(data) <= m.ChunkSize {
		return m.uploadSmallFile(ctx, data, escapedPath)
	}

	// For large files, upload in chunks
	return m.uploadLargeFile(ctx, data, escapedPath)
}

// uploadSmallFile uploads a small file in a single operation.
func (m *FileManager) uploadSmallFile(ctx context.Context, data []byte, remotePath string) error {
	encoded := base64.StdEncoding.EncodeToString(data)

	script := fmt.Sprintf(`
$bytes = [System.Convert]::FromBase64String('%s')
[System.IO.File]::WriteAllBytes('%s', $bytes)
`, encoded, remotePath)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("upload failed: %s", stderr)
	}

	return nil
}

// uploadLargeFile uploads a large file in chunks.
func (m *FileManager) uploadLargeFile(ctx context.Context, data []byte, remotePath string) error {
	// Create temp file path
	tempPath := remotePath + ".tmp"

	// Clear temp file if exists
	clearScript := fmt.Sprintf(`
if (Test-Path '%s') { Remove-Item '%s' -Force }
`, tempPath, tempPath)
	m.adapter.RunPowerShell(ctx, clearScript)

	// Upload in chunks
	for i := 0; i < len(data); i += m.ChunkSize {
		end := i + m.ChunkSize
		if end > len(data) {
			end = len(data)
		}

		chunk := data[i:end]
		encoded := base64.StdEncoding.EncodeToString(chunk)

		script := fmt.Sprintf(`
$bytes = [System.Convert]::FromBase64String('%s')
$fs = [System.IO.File]::Open('%s', [System.IO.FileMode]::Append)
$fs.Write($bytes, 0, $bytes.Length)
$fs.Close()
`, encoded, tempPath)

		_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
		if err != nil {
			return fmt.Errorf("chunk upload failed: %w", err)
		}

		if exitCode != 0 {
			return fmt.Errorf("chunk upload failed: %s", stderr)
		}
	}

	// Move temp file to final location
	moveScript := fmt.Sprintf(`
if (Test-Path '%s') { Remove-Item '%s' -Force }
Move-Item '%s' '%s' -Force
`, remotePath, remotePath, tempPath, remotePath)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, moveScript)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("failed to move file: %s", stderr)
	}

	return nil
}

// UploadReader uploads data from a reader to a remote path.
func (m *FileManager) UploadReader(ctx context.Context, reader io.Reader, remotePath string) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}
	return m.UploadBytes(ctx, data, remotePath)
}

// DownloadFile downloads a remote file to a local path.
func (m *FileManager) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	data, err := m.DownloadBytes(ctx, remotePath)
	if err != nil {
		return err
	}

	// Create parent directory if needed
	dir := filepath.Dir(localPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return os.WriteFile(localPath, data, 0644)
}

// DownloadBytes downloads a remote file as bytes.
func (m *FileManager) DownloadBytes(ctx context.Context, remotePath string) ([]byte, error) {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")

	// Get file size first
	sizeScript := fmt.Sprintf(`(Get-Item '%s').Length`, escapedPath)
	sizeOut, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, sizeScript)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("failed to get file size: %s", stderr)
	}

	var fileSize int64
	fmt.Sscanf(strings.TrimSpace(sizeOut), "%d", &fileSize)

	// For small files, download directly
	if fileSize <= int64(m.ChunkSize) {
		return m.downloadSmallFile(ctx, escapedPath)
	}

	// For large files, download in chunks
	return m.downloadLargeFile(ctx, escapedPath, fileSize)
}

// downloadSmallFile downloads a small file in a single operation.
func (m *FileManager) downloadSmallFile(ctx context.Context, remotePath string) ([]byte, error) {
	script := fmt.Sprintf(`
$bytes = [System.IO.File]::ReadAllBytes('%s')
[System.Convert]::ToBase64String($bytes)
`, remotePath)

	stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("download failed: %s", stderr)
	}

	// Decode base64
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout))
	if err != nil {
		return nil, fmt.Errorf("failed to decode file data: %w", err)
	}

	return data, nil
}

// downloadLargeFile downloads a large file in chunks.
func (m *FileManager) downloadLargeFile(ctx context.Context, remotePath string, fileSize int64) ([]byte, error) {
	var result []byte

	for offset := int64(0); offset < fileSize; offset += int64(m.ChunkSize) {
		length := int64(m.ChunkSize)
		if offset+length > fileSize {
			length = fileSize - offset
		}

		script := fmt.Sprintf(`
$fs = [System.IO.File]::Open('%s', [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read)
$fs.Seek(%d, [System.IO.SeekOrigin]::Begin) | Out-Null
$bytes = New-Object byte[] %d
$fs.Read($bytes, 0, %d) | Out-Null
$fs.Close()
[System.Convert]::ToBase64String($bytes)
`, remotePath, offset, length, length)

		stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
		if err != nil {
			return nil, fmt.Errorf("chunk download failed: %w", err)
		}

		if exitCode != 0 {
			return nil, fmt.Errorf("chunk download failed: %s", stderr)
		}

		chunk, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout))
		if err != nil {
			return nil, fmt.Errorf("failed to decode chunk: %w", err)
		}

		result = append(result, chunk...)
	}

	return result, nil
}

// FileExists checks if a file exists on the remote machine.
func (m *FileManager) FileExists(ctx context.Context, remotePath string) (bool, error) {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	script := fmt.Sprintf(`Test-Path '%s'`, escapedPath)

	stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return false, err
	}

	if exitCode != 0 {
		return false, fmt.Errorf("file exists check failed: %s", stderr)
	}

	return strings.TrimSpace(stdout) == "True", nil
}

// DirectoryExists checks if a directory exists on the remote machine.
func (m *FileManager) DirectoryExists(ctx context.Context, remotePath string) (bool, error) {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	script := fmt.Sprintf(`Test-Path '%s' -PathType Container`, escapedPath)

	stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return false, err
	}

	if exitCode != 0 {
		return false, fmt.Errorf("directory exists check failed: %s", stderr)
	}

	return strings.TrimSpace(stdout) == "True", nil
}

// CreateDirectory creates a directory on the remote machine.
func (m *FileManager) CreateDirectory(ctx context.Context, remotePath string) error {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	script := fmt.Sprintf(`New-Item -Path '%s' -ItemType Directory -Force | Out-Null`, escapedPath)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("create directory failed: %s", stderr)
	}

	return nil
}

// DeleteFile deletes a file on the remote machine.
func (m *FileManager) DeleteFile(ctx context.Context, remotePath string) error {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	script := fmt.Sprintf(`Remove-Item '%s' -Force`, escapedPath)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("delete file failed: %s", stderr)
	}

	return nil
}

// DeleteDirectory deletes a directory on the remote machine.
func (m *FileManager) DeleteDirectory(ctx context.Context, remotePath string, recurse bool) error {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	var script string
	if recurse {
		script = fmt.Sprintf(`Remove-Item '%s' -Recurse -Force`, escapedPath)
	} else {
		script = fmt.Sprintf(`Remove-Item '%s' -Force`, escapedPath)
	}

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("delete directory failed: %s", stderr)
	}

	return nil
}

// CopyFile copies a file on the remote machine.
func (m *FileManager) CopyFile(ctx context.Context, sourcePath, destPath string) error {
	escapedSource := strings.ReplaceAll(sourcePath, "'", "''")
	escapedDest := strings.ReplaceAll(destPath, "'", "''")
	script := fmt.Sprintf(`Copy-Item '%s' '%s' -Force`, escapedSource, escapedDest)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("copy file failed: %s", stderr)
	}

	return nil
}

// MoveFile moves a file on the remote machine.
func (m *FileManager) MoveFile(ctx context.Context, sourcePath, destPath string) error {
	escapedSource := strings.ReplaceAll(sourcePath, "'", "''")
	escapedDest := strings.ReplaceAll(destPath, "'", "''")
	script := fmt.Sprintf(`Move-Item '%s' '%s' -Force`, escapedSource, escapedDest)

	_, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		return fmt.Errorf("move file failed: %s", stderr)
	}

	return nil
}

// ListDirectory lists files and directories in a remote path.
func (m *FileManager) ListDirectory(ctx context.Context, remotePath string) ([]FileInfo, error) {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	script := fmt.Sprintf(`
Get-ChildItem '%s' | Select-Object Name, Length, Mode, LastWriteTime, @{N='IsDirectory';E={$_.PSIsContainer}} | ConvertTo-Json -Compress
`, escapedPath)

	stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("list directory failed: %s", stderr)
	}

	// Parse JSON response
	// In real implementation, would use json.Unmarshal
	// For now, return the raw data
	return []FileInfo{{Name: "raw", Raw: stdout}}, nil
}

// GetFileInfo gets information about a remote file.
func (m *FileManager) GetFileInfo(ctx context.Context, remotePath string) (*FileInfo, error) {
	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	script := fmt.Sprintf(`
$item = Get-Item '%s'
@{
    Name = $item.Name
    FullName = $item.FullName
    Length = $item.Length
    Mode = $item.Mode
    LastWriteTime = $item.LastWriteTime.ToString('o')
    CreationTime = $item.CreationTime.ToString('o')
    IsDirectory = $item.PSIsContainer
} | ConvertTo-Json -Compress
`, escapedPath)

	stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("get file info failed: %s", stderr)
	}

	// Parse JSON response
	return &FileInfo{Name: remotePath, Raw: stdout}, nil
}

// GetFileHash gets the hash of a remote file.
func (m *FileManager) GetFileHash(ctx context.Context, remotePath string, algorithm string) (string, error) {
	if algorithm == "" {
		algorithm = "SHA256"
	}

	escapedPath := strings.ReplaceAll(remotePath, "'", "''")
	script := fmt.Sprintf(`(Get-FileHash '%s' -Algorithm '%s').Hash`, escapedPath, algorithm)

	stdout, stderr, exitCode, err := m.adapter.RunPowerShell(ctx, script)
	if err != nil {
		return "", err
	}

	if exitCode != 0 {
		return "", fmt.Errorf("get file hash failed: %s", stderr)
	}

	return strings.TrimSpace(stdout), nil
}

// FileInfo contains remote file information.
type FileInfo struct {
	Name          string `json:"Name"`
	FullName      string `json:"FullName"`
	Length        int64  `json:"Length"`
	Mode          string `json:"Mode"`
	LastWriteTime string `json:"LastWriteTime"`
	CreationTime  string `json:"CreationTime"`
	IsDirectory   bool   `json:"IsDirectory"`
	Raw           string `json:"-"`
}
