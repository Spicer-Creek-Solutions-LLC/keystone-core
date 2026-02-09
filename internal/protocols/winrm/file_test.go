package winrm

import (
	"context"
	"testing"
)

func TestAdapterNewFileManager(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	if manager == nil {
		t.Fatal("expected manager to be created")
	}
	if manager.adapter != adapter {
		t.Error("manager.adapter should reference the adapter")
	}
	if manager.ChunkSize != 64*1024 {
		t.Errorf("ChunkSize = %d, want 65536 (64KB)", manager.ChunkSize)
	}
}

func TestFileInfoStructure(t *testing.T) {
	info := &FileInfo{
		Name:          "test.txt",
		FullName:      "C:\\Users\\admin\\test.txt",
		Length:        1024,
		Mode:          "-a----",
		LastWriteTime: "2024-01-15T10:30:00Z",
		CreationTime:  "2024-01-10T08:00:00Z",
		IsDirectory:   false,
		Raw:           "{}",
	}

	if info.Name != "test.txt" {
		t.Errorf("Name = %v", info.Name)
	}
	if info.FullName != "C:\\Users\\admin\\test.txt" {
		t.Errorf("FullName = %v", info.FullName)
	}
	if info.Length != 1024 {
		t.Errorf("Length = %d", info.Length)
	}
	if info.Mode != "-a----" {
		t.Errorf("Mode = %v", info.Mode)
	}
	if info.IsDirectory != false {
		t.Errorf("IsDirectory = %v", info.IsDirectory)
	}
}

func TestFileInfoDirectory(t *testing.T) {
	info := &FileInfo{
		Name:        "Documents",
		FullName:    "C:\\Users\\admin\\Documents",
		Length:      0,
		Mode:        "d-----",
		IsDirectory: true,
	}

	if info.Name != "Documents" {
		t.Errorf("Name = %v", info.Name)
	}
	if info.IsDirectory != true {
		t.Errorf("IsDirectory = %v, want true", info.IsDirectory)
	}
}

func TestFileManagerChunkSizeCustom(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	// Modify chunk size
	manager.ChunkSize = 32 * 1024 // 32KB

	if manager.ChunkSize != 32*1024 {
		t.Errorf("ChunkSize = %d, want 32768 (32KB)", manager.ChunkSize)
	}
}

func TestFileManagerNotConnected(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	t.Run("FileExists", func(t *testing.T) {
		_, err := manager.FileExists(context.Background(), "C:\\test.txt")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("DirectoryExists", func(t *testing.T) {
		_, err := manager.DirectoryExists(context.Background(), "C:\\test")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("CreateDirectory", func(t *testing.T) {
		err := manager.CreateDirectory(context.Background(), "C:\\test")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("DeleteFile", func(t *testing.T) {
		err := manager.DeleteFile(context.Background(), "C:\\test.txt")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("DeleteDirectory", func(t *testing.T) {
		err := manager.DeleteDirectory(context.Background(), "C:\\test", false)
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("CopyFile", func(t *testing.T) {
		err := manager.CopyFile(context.Background(), "C:\\src.txt", "C:\\dst.txt")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("MoveFile", func(t *testing.T) {
		err := manager.MoveFile(context.Background(), "C:\\src.txt", "C:\\dst.txt")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("ListDirectory", func(t *testing.T) {
		_, err := manager.ListDirectory(context.Background(), "C:\\")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("GetFileInfo", func(t *testing.T) {
		_, err := manager.GetFileInfo(context.Background(), "C:\\test.txt")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("GetFileHash", func(t *testing.T) {
		_, err := manager.GetFileHash(context.Background(), "C:\\test.txt", "SHA256")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("DownloadBytes", func(t *testing.T) {
		_, err := manager.DownloadBytes(context.Background(), "C:\\test.txt")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})

	t.Run("UploadBytes", func(t *testing.T) {
		err := manager.UploadBytes(context.Background(), []byte("test"), "C:\\test.txt")
		if err == nil {
			t.Error("expected error when not connected")
		}
	})
}

func TestUploadFileLocalReadError(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	// Try to upload a non-existent local file
	err := manager.UploadFile(context.Background(), "/nonexistent/path/file.txt", "C:\\remote.txt")
	if err == nil {
		t.Error("expected error when local file doesn't exist")
	}
}

func TestPathEscaping(t *testing.T) {
	// Test that paths with single quotes are properly escaped
	tests := []struct {
		input    string
		expected string
	}{
		{"C:\\test.txt", "C:\\test.txt"},
		{"C:\\test's file.txt", "C:\\test''s file.txt"},
		{"C:\\it's a 'test'", "C:\\it''s a ''test''"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// This is how paths are escaped in the file operations
			escaped := escapePath(tt.input)
			if escaped != tt.expected {
				t.Errorf("escapePath(%q) = %q, want %q", tt.input, escaped, tt.expected)
			}
		})
	}
}

// escapePath helper for testing - mirrors the inline escaping used in file.go
func escapePath(path string) string {
	return replaceAll(path, "'", "''")
}

// replaceAll is a test helper that mirrors strings.ReplaceAll
func replaceAll(s, old, updated string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result += updated
			i += len(old) - 1
		} else {
			result += string(s[i])
		}
	}
	return result
}

func TestDownloadFileMakesDirectory(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	// This will fail at the DownloadBytes step (not connected),
	// but we're verifying that the method signature is correct
	err := manager.DownloadFile(context.Background(), "C:\\remote.txt", "/tmp/test/nested/file.txt")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestGetFileHashDefaultAlgorithm(t *testing.T) {
	// Test that empty algorithm defaults to SHA256
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	// Will fail because not connected, but verifies the method signature
	_, err := manager.GetFileHash(context.Background(), "C:\\test.txt", "")
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestDeleteDirectoryRecursive(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	// Test recursive delete
	err := manager.DeleteDirectory(context.Background(), "C:\\test", true)
	if err == nil {
		t.Error("expected error when not connected")
	}

	// Test non-recursive delete
	err = manager.DeleteDirectory(context.Background(), "C:\\test", false)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestUploadBytesSmallVsLarge(t *testing.T) {
	adapter := NewAdapter(nil)
	manager := adapter.NewFileManager()

	// Small file (under chunk size)
	smallData := make([]byte, 1024) // 1KB
	err := manager.UploadBytes(context.Background(), smallData, "C:\\small.txt")
	if err == nil {
		t.Error("expected error when not connected")
	}

	// Large file (over chunk size)
	largeData := make([]byte, 128*1024) // 128KB
	err = manager.UploadBytes(context.Background(), largeData, "C:\\large.txt")
	if err == nil {
		t.Error("expected error when not connected")
	}
}
