package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"
)

// MockS3Client implements S3Client for testing.
type MockS3Client struct {
	objects map[string]*mockS3Object
}

type mockS3Object struct {
	data        []byte
	contentType string
	metadata    map[string]string
	etag        string
	modified    time.Time
}

func newMockS3Client() *MockS3Client {
	return &MockS3Client{
		objects: make(map[string]*mockS3Object),
	}
}

func (c *MockS3Client) GetObject(ctx context.Context, bucket, key string, opts *S3GetOptions) (*S3Object, error) {
	obj, ok := c.objects[key]
	if !ok {
		return nil, &BackendError{Op: "get", Path: key, Err: ErrNotFound}
	}

	data := obj.data
	if opts != nil && opts.Range != nil {
		start := opts.Range.Start
		end := opts.Range.End
		if end == 0 || end > int64(len(data)) {
			end = int64(len(data))
		}
		data = data[start:end]
	}

	return &S3Object{
		Body:         io.NopCloser(bytes.NewReader(data)),
		ContentType:  obj.contentType,
		Size:         int64(len(obj.data)),
		ETag:         obj.etag,
		LastModified: obj.modified,
		Metadata:     obj.metadata,
	}, nil
}

func (c *MockS3Client) PutObject(ctx context.Context, bucket, key string, body io.Reader, opts *S3PutOptions) (*S3PutResult, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]string)
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			metadata[k] = v
		}
	}

	contentType := "application/octet-stream"
	if opts != nil && opts.ContentType != "" {
		contentType = opts.ContentType
	}

	c.objects[key] = &mockS3Object{
		data:        data,
		contentType: contentType,
		metadata:    metadata,
		etag:        "mock-etag",
		modified:    time.Now(),
	}

	return &S3PutResult{ETag: "mock-etag"}, nil
}

func (c *MockS3Client) DeleteObject(ctx context.Context, bucket, key string) error {
	delete(c.objects, key)
	return nil
}

func (c *MockS3Client) HeadObject(ctx context.Context, bucket, key string) (*S3ObjectInfo, error) {
	obj, ok := c.objects[key]
	if !ok {
		return nil, &BackendError{Op: "head", Path: key, Err: ErrNotFound}
	}

	return &S3ObjectInfo{
		ContentType:  obj.contentType,
		Size:         int64(len(obj.data)),
		ETag:         obj.etag,
		LastModified: obj.modified,
		Metadata:     obj.metadata,
	}, nil
}

func (c *MockS3Client) ListObjects(ctx context.Context, bucket, prefix string, opts *S3ListOptions) (*S3ListResult, error) {
	var objects []S3ListObject
	for key, obj := range c.objects {
		if len(prefix) == 0 || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			objects = append(objects, S3ListObject{
				Key:          key,
				Size:         int64(len(obj.data)),
				ETag:         obj.etag,
				LastModified: obj.modified,
			})
		}
	}

	return &S3ListResult{
		Contents:    objects,
		IsTruncated: false,
	}, nil
}

func (c *MockS3Client) HeadBucket(ctx context.Context, bucket string) error {
	return nil
}

func TestNewS3Backend(t *testing.T) {
	tests := []struct {
		name    string
		config  *S3Config
		wantErr bool
	}{
		{
			name: "valid config with region",
			config: &S3Config{
				Config: Config{Name: "test"},
				Bucket: "test-bucket",
				Region: "us-west-2",
			},
			wantErr: false,
		},
		{
			name: "valid config with endpoint",
			config: &S3Config{
				Config:   Config{Name: "test"},
				Bucket:   "test-bucket",
				Endpoint: "http://localhost:9000",
			},
			wantErr: false,
		},
		{
			name: "missing bucket",
			config: &S3Config{
				Config: Config{Name: "test"},
				Region: "us-west-2",
			},
			wantErr: true,
		},
		{
			name: "missing region and endpoint",
			config: &S3Config{
				Config: Config{Name: "test"},
				Bucket: "test-bucket",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewS3Backend(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewS3Backend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if b.Name() != tt.config.Name {
					t.Errorf("Name() = %v, want %v", b.Name(), tt.config.Name)
				}
				if b.Type() != BackendTypeS3 {
					t.Errorf("Type() = %v, want %v", b.Type(), BackendTypeS3)
				}
			}
		})
	}
}

func TestS3BackendOperations(t *testing.T) {
	config := &S3Config{
		Config: Config{Name: "test"},
		Bucket: "test-bucket",
		Region: "us-west-2",
	}

	b, err := NewS3Backend(config)
	if err != nil {
		t.Fatal(err)
	}

	mockClient := newMockS3Client()
	b.SetClient(mockClient)

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

	// Exists
	exists, err := b.Exists(ctx, testPath)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
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

	// Stat
	info, err := b.Stat(ctx, testPath)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size != int64(len(testContent)) {
		t.Errorf("Stat() size = %v, want %v", info.Size, len(testContent))
	}

	// List
	listResult, err := b.List(ctx, "/test", &ListOptions{Recursive: true})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listResult.Files) != 1 {
		t.Errorf("List() returned %d files, want 1", len(listResult.Files))
	}

	// Delete
	if err := b.Delete(ctx, testPath); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	exists, _ = b.Exists(ctx, testPath)
	if exists {
		t.Error("File still exists after Delete()")
	}

	// Health
	if err := b.Health(ctx); err != nil {
		t.Errorf("Health() error = %v", err)
	}
}

func TestS3BackendNoClient(t *testing.T) {
	config := &S3Config{
		Config: Config{Name: "test"},
		Bucket: "test-bucket",
		Region: "us-west-2",
	}

	b, err := NewS3Backend(config)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// All operations should fail without a client
	_, err = b.Get(ctx, "/test", nil)
	if err == nil {
		t.Error("Get() should fail without client")
	}

	_, err = b.Put(ctx, "/test", bytes.NewReader([]byte("test")), nil)
	if err == nil {
		t.Error("Put() should fail without client")
	}

	_, err = b.Exists(ctx, "/test")
	if err == nil {
		t.Error("Exists() should fail without client")
	}

	_, err = b.Stat(ctx, "/test")
	if err == nil {
		t.Error("Stat() should fail without client")
	}

	_, err = b.List(ctx, "/", nil)
	if err == nil {
		t.Error("List() should fail without client")
	}

	err = b.Health(ctx)
	if err == nil {
		t.Error("Health() should fail without client")
	}
}

func TestS3BackendReadOnly(t *testing.T) {
	config := &S3Config{
		Config: Config{Name: "test", ReadOnly: true},
		Bucket: "test-bucket",
		Region: "us-west-2",
	}

	b, err := NewS3Backend(config)
	if err != nil {
		t.Fatal(err)
	}

	mockClient := newMockS3Client()
	b.SetClient(mockClient)

	ctx := context.Background()

	// Put should fail
	_, err = b.Put(ctx, "/test", bytes.NewReader([]byte("test")), nil)
	if err == nil {
		t.Error("Put() should fail in read-only mode")
	}

	// Delete should fail
	err = b.Delete(ctx, "/test")
	if err == nil {
		t.Error("Delete() should fail in read-only mode")
	}
}

func TestS3BackendPrefix(t *testing.T) {
	config := &S3Config{
		Config: Config{Name: "test"},
		Bucket: "test-bucket",
		Region: "us-west-2",
		Prefix: "data",
	}

	b, err := NewS3Backend(config)
	if err != nil {
		t.Fatal(err)
	}

	// Test fullKey with prefix
	key := b.fullKey("/test/file.txt")
	expected := "data/test/file.txt"
	if key != expected {
		t.Errorf("fullKey() = %v, want %v", key, expected)
	}
}

// MockGCSClient implements GCSClient for testing.
type MockGCSClient struct {
	objects map[string]*mockGCSObject
}

type mockGCSObject struct {
	data        []byte
	contentType string
	metadata    map[string]string
	generation  int64
	updated     time.Time
}

func newMockGCSClient() *MockGCSClient {
	return &MockGCSClient{
		objects: make(map[string]*mockGCSObject),
	}
}

func (c *MockGCSClient) GetObject(ctx context.Context, bucket, object string, opts *GCSGetOptions) (*GCSObject, error) {
	obj, ok := c.objects[object]
	if !ok {
		return nil, &BackendError{Op: "get", Path: object, Err: ErrNotFound}
	}

	data := obj.data
	if opts != nil && opts.Length > 0 {
		end := opts.Offset + opts.Length
		if end > int64(len(data)) {
			end = int64(len(data))
		}
		data = data[opts.Offset:end]
	}

	return &GCSObject{
		Body:        io.NopCloser(bytes.NewReader(data)),
		ContentType: obj.contentType,
		Size:        int64(len(obj.data)),
		Generation:  obj.generation,
		Updated:     obj.updated,
		Metadata:    obj.metadata,
	}, nil
}

func (c *MockGCSClient) PutObject(ctx context.Context, bucket, object string, body io.Reader, opts *GCSPutOptions) (*GCSPutResult, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]string)
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			metadata[k] = v
		}
	}

	contentType := "application/octet-stream"
	if opts != nil && opts.ContentType != "" {
		contentType = opts.ContentType
	}

	c.objects[object] = &mockGCSObject{
		data:        data,
		contentType: contentType,
		metadata:    metadata,
		generation:  1,
		updated:     time.Now(),
	}

	return &GCSPutResult{Generation: 1}, nil
}

func (c *MockGCSClient) DeleteObject(ctx context.Context, bucket, object string) error {
	delete(c.objects, object)
	return nil
}

func (c *MockGCSClient) GetObjectAttrs(ctx context.Context, bucket, object string) (*GCSObjectAttrs, error) {
	obj, ok := c.objects[object]
	if !ok {
		return nil, &BackendError{Op: "attrs", Path: object, Err: ErrNotFound}
	}

	return &GCSObjectAttrs{
		Name:        object,
		ContentType: obj.contentType,
		Size:        int64(len(obj.data)),
		Generation:  obj.generation,
		Updated:     obj.updated,
		Metadata:    obj.metadata,
	}, nil
}

func (c *MockGCSClient) ListObjects(ctx context.Context, bucket string, opts *GCSListOptions) (*GCSListResult, error) {
	var objects []GCSListObject
	prefix := ""
	if opts != nil {
		prefix = opts.Prefix
	}

	for name, obj := range c.objects {
		if len(prefix) == 0 || len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			objects = append(objects, GCSListObject{
				Name:       name,
				Size:       int64(len(obj.data)),
				Generation: obj.generation,
				Updated:    obj.updated,
			})
		}
	}

	return &GCSListResult{
		Objects: objects,
	}, nil
}

func (c *MockGCSClient) BucketExists(ctx context.Context, bucket string) error {
	return nil
}

func TestNewGCSBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  *GCSConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &GCSConfig{
				Config: Config{Name: "test"},
				Bucket: "test-bucket",
			},
			wantErr: false,
		},
		{
			name: "missing bucket",
			config: &GCSConfig{
				Config: Config{Name: "test"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewGCSBackend(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGCSBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if b.Name() != tt.config.Name {
					t.Errorf("Name() = %v, want %v", b.Name(), tt.config.Name)
				}
				if b.Type() != BackendTypeGCS {
					t.Errorf("Type() = %v, want %v", b.Type(), BackendTypeGCS)
				}
			}
		})
	}
}

func TestGCSBackendOperations(t *testing.T) {
	config := &GCSConfig{
		Config: Config{Name: "test"},
		Bucket: "test-bucket",
	}

	b, err := NewGCSBackend(config)
	if err != nil {
		t.Fatal(err)
	}

	mockClient := newMockGCSClient()
	b.SetClient(mockClient)

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

	// Health
	if err := b.Health(ctx); err != nil {
		t.Errorf("Health() error = %v", err)
	}
}

// MockAzureClient implements AzureClient for testing.
type MockAzureClient struct {
	blobs map[string]*mockAzureBlob
}

type mockAzureBlob struct {
	data        []byte
	contentType string
	metadata    map[string]string
	etag        string
	modified    time.Time
}

func newMockAzureClient() *MockAzureClient {
	return &MockAzureClient{
		blobs: make(map[string]*mockAzureBlob),
	}
}

func (c *MockAzureClient) GetBlob(ctx context.Context, container, blob string, opts *AzureGetOptions) (*AzureBlob, error) {
	obj, ok := c.blobs[blob]
	if !ok {
		return nil, &BackendError{Op: "get", Path: blob, Err: ErrNotFound}
	}

	data := obj.data
	if opts != nil && opts.Count > 0 {
		end := opts.Offset + opts.Count
		if end > int64(len(data)) {
			end = int64(len(data))
		}
		data = data[opts.Offset:end]
	}

	return &AzureBlob{
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentType:   obj.contentType,
		ContentLength: int64(len(obj.data)),
		ETag:          obj.etag,
		LastModified:  obj.modified,
		Metadata:      obj.metadata,
	}, nil
}

func (c *MockAzureClient) PutBlob(ctx context.Context, container, blob string, body io.Reader, opts *AzurePutOptions) (*AzurePutResult, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]string)
	if opts != nil && opts.Metadata != nil {
		for k, v := range opts.Metadata {
			metadata[k] = v
		}
	}

	contentType := "application/octet-stream"
	if opts != nil && opts.ContentType != "" {
		contentType = opts.ContentType
	}

	c.blobs[blob] = &mockAzureBlob{
		data:        data,
		contentType: contentType,
		metadata:    metadata,
		etag:        "mock-etag",
		modified:    time.Now(),
	}

	return &AzurePutResult{ETag: "mock-etag"}, nil
}

func (c *MockAzureClient) DeleteBlob(ctx context.Context, container, blob string) error {
	delete(c.blobs, blob)
	return nil
}

func (c *MockAzureClient) GetBlobProperties(ctx context.Context, container, blob string) (*AzureBlobProperties, error) {
	obj, ok := c.blobs[blob]
	if !ok {
		return nil, &BackendError{Op: "props", Path: blob, Err: ErrNotFound}
	}

	return &AzureBlobProperties{
		ContentType:   obj.contentType,
		ContentLength: int64(len(obj.data)),
		ETag:          obj.etag,
		LastModified:  obj.modified,
		Metadata:      obj.metadata,
	}, nil
}

func (c *MockAzureClient) ListBlobs(ctx context.Context, container string, opts *AzureListOptions) (*AzureListResult, error) {
	var blobs []AzureListBlob
	prefix := ""
	if opts != nil {
		prefix = opts.Prefix
	}

	for name, obj := range c.blobs {
		if len(prefix) == 0 || len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			blobs = append(blobs, AzureListBlob{
				Name:         name,
				Size:         int64(len(obj.data)),
				ETag:         obj.etag,
				LastModified: obj.modified,
				Metadata:     obj.metadata,
			})
		}
	}

	return &AzureListResult{
		Blobs: blobs,
	}, nil
}

func (c *MockAzureClient) ContainerExists(ctx context.Context, container string) error {
	return nil
}

func TestNewAzureBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  *AzureConfig
		wantErr bool
	}{
		{
			name: "valid config with account",
			config: &AzureConfig{
				Config:      Config{Name: "test"},
				Container:   "test-container",
				AccountName: "testaccount",
			},
			wantErr: false,
		},
		{
			name: "valid config with connection string",
			config: &AzureConfig{
				Config:           Config{Name: "test"},
				Container:        "test-container",
				ConnectionString: "DefaultEndpointsProtocol=https;AccountName=test;AccountKey=key==",
			},
			wantErr: false,
		},
		{
			name: "missing container",
			config: &AzureConfig{
				Config:      Config{Name: "test"},
				AccountName: "testaccount",
			},
			wantErr: true,
		},
		{
			name: "missing account and connection string",
			config: &AzureConfig{
				Config:    Config{Name: "test"},
				Container: "test-container",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewAzureBackend(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAzureBackend() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if b.Name() != tt.config.Name {
					t.Errorf("Name() = %v, want %v", b.Name(), tt.config.Name)
				}
				if b.Type() != BackendTypeAzure {
					t.Errorf("Type() = %v, want %v", b.Type(), BackendTypeAzure)
				}
			}
		})
	}
}

func TestAzureBackendOperations(t *testing.T) {
	config := &AzureConfig{
		Config:      Config{Name: "test"},
		Container:   "test-container",
		AccountName: "testaccount",
	}

	b, err := NewAzureBackend(config)
	if err != nil {
		t.Fatal(err)
	}

	mockClient := newMockAzureClient()
	b.SetClient(mockClient)

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

	// Health
	if err := b.Health(ctx); err != nil {
		t.Errorf("Health() error = %v", err)
	}
}

func TestIsS3NotFound(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("NoSuchKey: key not found"), true},
		{fmt.Errorf("NotFound: bucket not found"), true},
		{fmt.Errorf("404 not found"), true},
		{fmt.Errorf("access denied"), false},
	}

	for _, tt := range tests {
		got := isS3NotFound(tt.err)
		if got != tt.want {
			t.Errorf("isS3NotFound(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestIsGCSNotFound(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("storage: object doesn't exist"), true},
		{fmt.Errorf("object doesn't exist"), true},
		{fmt.Errorf("404 not found"), true},
		{fmt.Errorf("access denied"), false},
	}

	for _, tt := range tests {
		got := isGCSNotFound(tt.err)
		if got != tt.want {
			t.Errorf("isGCSNotFound(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestIsAzureNotFound(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("BlobNotFound: blob not found"), true},
		{fmt.Errorf("blob does not exist"), true},
		{fmt.Errorf("404 not found"), true},
		{fmt.Errorf("access denied"), false},
	}

	for _, tt := range tests {
		got := isAzureNotFound(tt.err)
		if got != tt.want {
			t.Errorf("isAzureNotFound(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
