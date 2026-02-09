package backend

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFilesystemBackend(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		config  *FilesystemConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &FilesystemConfig{
				Config: Config{
					Name: "test",
					Type: TypeFilesystem,
				},
				Root: tmpDir,
			},
			wantErr: false,
		},
		{
			name: "missing root path",
			config: &FilesystemConfig{
				Config: Config{
					Name: "test",
					Type: TypeFilesystem,
				},
			},
			wantErr: true,
		},
		{
			name: "non-existent path with create",
			config: &FilesystemConfig{
				Config: Config{
					Name: "test",
					Type: TypeFilesystem,
				},
				Root:       filepath.Join(tmpDir, "new-dir"),
				CreateDirs: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewFilesystemBackend(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFilesystemBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if b.Name() != tt.config.Name {
					t.Errorf("Name() = %v, want %v", b.Name(), tt.config.Name)
				}
				if b.Type() != TypeFilesystem {
					t.Errorf("Type() = %v, want %v", b.Type(), TypeFilesystem)
				}
				b.Close()
			}
		})
	}
}

func TestFilesystemBackendPutGet(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config: Config{
			Name: "test",
			Type: TypeFilesystem,
		},
		Root:       tmpDir,
		CreateDirs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	testPath := "/test/file.txt"
	testContent := []byte("hello world")

	// Put file
	putResult, err := b.Put(ctx, testPath, bytes.NewReader(testContent), &PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if putResult.Size != int64(len(testContent)) {
		t.Errorf("Put() size = %v, want %v", putResult.Size, len(testContent))
	}
	if putResult.Checksum == "" {
		t.Error("Put() checksum is empty")
	}

	// Get file
	getResult, err := b.Get(ctx, testPath, nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer getResult.Reader.Close()

	gotContent, err := io.ReadAll(getResult.Reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if !bytes.Equal(gotContent, testContent) {
		t.Errorf("Get() content = %q, want %q", gotContent, testContent)
	}
	if getResult.Info.Checksum != putResult.Checksum {
		t.Errorf("Get() checksum = %v, want %v", getResult.Info.Checksum, putResult.Checksum)
	}
}

func TestFilesystemBackendExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config:     Config{Name: "test"},
		Root:       tmpDir,
		CreateDirs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	testPath := "/test/file.txt"

	// File doesn't exist
	exists, err := b.Exists(ctx, testPath)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}

	// Create file
	_, err = b.Put(ctx, testPath, bytes.NewReader([]byte("test")), nil)
	if err != nil {
		t.Fatal(err)
	}

	// File exists
	exists, err = b.Exists(ctx, testPath)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}
}

func TestFilesystemBackendDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config:     Config{Name: "test"},
		Root:       tmpDir,
		CreateDirs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	testPath := "/test/file.txt"

	// Create file
	_, err = b.Put(ctx, testPath, bytes.NewReader([]byte("test")), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Delete file
	if err := b.Delete(ctx, testPath); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	exists, _ := b.Exists(ctx, testPath)
	if exists {
		t.Error("File still exists after Delete()")
	}
}

func TestFilesystemBackendStat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config:     Config{Name: "test"},
		Root:       tmpDir,
		CreateDirs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	testPath := "/test/file.txt"
	testContent := []byte("hello world")

	// Create file
	_, err = b.Put(ctx, testPath, bytes.NewReader(testContent), &PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Stat file
	info, err := b.Stat(ctx, testPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	if info.Size != int64(len(testContent)) {
		t.Errorf("Stat() size = %v, want %v", info.Size, len(testContent))
	}
	if info.Name != "file.txt" {
		t.Errorf("Stat() name = %v, want %v", info.Name, "file.txt")
	}
	if info.Path != testPath {
		t.Errorf("Stat() path = %v, want %v", info.Path, testPath)
	}
	if info.IsDirectory {
		t.Error("Stat() IsDirectory = true, want false")
	}
}

func TestFilesystemBackendList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config:     Config{Name: "test"},
		Root:       tmpDir,
		CreateDirs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()

	// Create some files
	files := []string{
		"/dir1/file1.txt",
		"/dir1/file2.txt",
		"/dir2/file3.txt",
	}
	for _, f := range files {
		if _, err := b.Put(ctx, f, bytes.NewReader([]byte("test")), nil); err != nil {
			t.Fatal(err)
		}
	}

	// List dir1
	result, err := b.List(ctx, "/dir1", nil)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(result.Files) != 2 {
		t.Errorf("List() returned %d files, want 2", len(result.Files))
	}

	// List recursively
	result, err = b.List(ctx, "/", &ListOptions{Recursive: true})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Count non-directory files only
	fileCount := 0
	for _, f := range result.Files {
		if !f.IsDirectory {
			fileCount++
		}
	}
	if fileCount != 3 {
		t.Errorf("List() recursive returned %d files (excluding dirs), want 3", fileCount)
	}
}

func TestFilesystemBackendRangeRequest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config:     Config{Name: "test"},
		Root:       tmpDir,
		CreateDirs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	testPath := "/test/file.txt"
	testContent := []byte("0123456789")

	// Create file
	_, err = b.Put(ctx, testPath, bytes.NewReader(testContent), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Get range
	result, err := b.Get(ctx, testPath, &GetOptions{
		Range: &ByteRange{Start: 3, End: 7},
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer result.Reader.Close()

	gotContent, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatal(err)
	}

	expected := "3456"
	if string(gotContent) != expected {
		t.Errorf("Get() range content = %q, want %q", gotContent, expected)
	}
}

func TestFilesystemBackendConditionalGet(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config:     Config{Name: "test"},
		Root:       tmpDir,
		CreateDirs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()
	testPath := "/test/file.txt"
	testContent := []byte("hello world")

	// Create file
	putResult, err := b.Put(ctx, testPath, bytes.NewReader(testContent), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Get with matching checksum (not modified)
	result, err := b.Get(ctx, testPath, &GetOptions{
		IfNoneMatch: putResult.Checksum,
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if !result.NotModified {
		t.Error("Get() NotModified = false, want true")
	}

	// Get with different checksum (modified)
	result, err = b.Get(ctx, testPath, &GetOptions{
		IfNoneMatch: "differentchecksum",
	})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer result.Reader.Close()

	if result.NotModified {
		t.Error("Get() NotModified = true, want false")
	}
}

func TestFilesystemBackendHealth(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config: Config{Name: "test"},
		Root:   tmpDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if err := b.Health(context.Background()); err != nil {
		t.Errorf("Health() error = %v", err)
	}
}

func TestFilesystemBackendReadOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filesystem-backend-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file first
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := NewFilesystemBackend(&FilesystemConfig{
		Config: Config{
			Name:     "test",
			ReadOnly: true,
		},
		Root: tmpDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	ctx := context.Background()

	// Put should fail
	_, err = b.Put(ctx, "/newfile.txt", bytes.NewReader([]byte("test")), nil)
	if err == nil {
		t.Error("Put() should fail in read-only mode")
	}

	// Delete should fail
	err = b.Delete(ctx, "/test.txt")
	if err == nil {
		t.Error("Delete() should fail in read-only mode")
	}

	// Get should work
	_, err = b.Get(ctx, "/test.txt", nil)
	if err != nil {
		t.Errorf("Get() should work in read-only mode, got error: %v", err)
	}
}
