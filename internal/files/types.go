// Package files provides file distribution over NATS for Keystone Core.
// It enables agents to retrieve packages, configurations, binaries, and other
// files without requiring additional network connections.
package files

import (
	"encoding/json"
	"time"
)

// FileRequest represents a request to retrieve a file.
type FileRequest struct {
	RequestID string            `json:"request_id"`
	Path      string            `json:"path"`
	Version   string            `json:"version,omitempty"`   // "latest", semver, or tag
	Checksum  string            `json:"checksum,omitempty"`  // Skip if matches (conditional GET)
	Range     *ByteRange        `json:"range,omitempty"`     // For resume
	ChunkSize int               `json:"chunk_size,omitempty"` // Override default
	Priority  int               `json:"priority,omitempty"`  // 0=normal, 1=high, 2=critical
	AgentID   string            `json:"agent_id"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ByteRange specifies a range of bytes for partial file retrieval.
type ByteRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end,omitempty"` // 0 = to end of file
}

// FileMetadata contains information about a file.
type FileMetadata struct {
	RequestID    string            `json:"request_id"`
	Path         string            `json:"path"`
	Version      string            `json:"version"`
	Size         int64             `json:"size"`
	Checksum     string            `json:"checksum"`      // SHA-256
	ContentType  string            `json:"content_type"`
	ModifiedTime time.Time         `json:"modified_time"`
	ChunkCount   int               `json:"chunk_count"`
	ChunkSize    int               `json:"chunk_size"`
	Tags         map[string]string `json:"tags,omitempty"`
	NotModified  bool              `json:"not_modified"` // True if checksum matched
}

// FileChunk represents a chunk of file data.
type FileChunk struct {
	RequestID  string `json:"request_id"`
	Index      int    `json:"index"`
	TotalCount int    `json:"total_count"`
	Data       []byte `json:"data"`     // Base64 encoded in JSON
	Checksum   string `json:"checksum"` // Chunk checksum (SHA-256)
	Final      bool   `json:"final"`
}

// FileAck acknowledges receipt of a file or chunk.
type FileAck struct {
	RequestID string    `json:"request_id"`
	Status    AckStatus `json:"status"`
	Message   string    `json:"message,omitempty"`
	ChunkIdx  int       `json:"chunk_idx,omitempty"` // Last successfully received chunk
}

// AckStatus represents the acknowledgment status.
type AckStatus string

const (
	AckStatusComplete AckStatus = "complete"
	AckStatusPartial  AckStatus = "partial"
	AckStatusError    AckStatus = "error"
	AckStatusRetry    AckStatus = "retry"
)

// FileError represents an error response.
type FileError struct {
	RequestID string        `json:"request_id"`
	Code      FileErrorCode `json:"code"`
	Message   string        `json:"message"`
	Details   string        `json:"details,omitempty"`
}

// FileErrorCode represents error types.
type FileErrorCode string

const (
	ErrCodeNotFound       FileErrorCode = "not_found"
	ErrCodeAccessDenied   FileErrorCode = "access_denied"
	ErrCodeInvalidRequest FileErrorCode = "invalid_request"
	ErrCodeBackendError   FileErrorCode = "backend_error"
	ErrCodeTimeout        FileErrorCode = "timeout"
	ErrCodeChecksumFailed FileErrorCode = "checksum_failed"
	ErrCodeQuotaExceeded  FileErrorCode = "quota_exceeded"
	ErrCodeFileTooLarge   FileErrorCode = "file_too_large"
	ErrCodeInternal       FileErrorCode = "internal_error"
)

// Error implements the error interface.
func (e *FileError) Error() string {
	return e.Message
}

// FileUploadRequest represents a request to upload a file.
type FileUploadRequest struct {
	RequestID   string            `json:"request_id"`
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"` // Expected SHA-256
	ContentType string            `json:"content_type,omitempty"`
	ChunkSize   int               `json:"chunk_size,omitempty"`
	AgentID     string            `json:"agent_id"`
	Tags        map[string]string `json:"tags,omitempty"`
	Overwrite   bool              `json:"overwrite,omitempty"`
}

// FileUploadResponse acknowledges an upload request.
type FileUploadResponse struct {
	RequestID  string `json:"request_id"`
	Accepted   bool   `json:"accepted"`
	UploadID   string `json:"upload_id,omitempty"` // For multi-chunk uploads
	ChunkSize  int    `json:"chunk_size"`
	ChunkCount int    `json:"chunk_count"`
	Error      string `json:"error,omitempty"`
}

// FileListRequest requests a list of files.
type FileListRequest struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`      // Directory path or glob pattern
	Recursive bool   `json:"recursive"` // Include subdirectories
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	AgentID   string `json:"agent_id"`
}

// FileListResponse contains a list of files.
type FileListResponse struct {
	RequestID  string         `json:"request_id"`
	Path       string         `json:"path"`
	Files      []FileInfo     `json:"files"`
	TotalCount int            `json:"total_count"`
	Truncated  bool           `json:"truncated"`
}

// FileInfo contains basic file information for listings.
type FileInfo struct {
	Path         string            `json:"path"`
	Name         string            `json:"name"`
	Size         int64             `json:"size"`
	Checksum     string            `json:"checksum"`
	ContentType  string            `json:"content_type"`
	ModifiedTime time.Time         `json:"modified_time"`
	IsDirectory  bool              `json:"is_directory"`
	Version      string            `json:"version,omitempty"`
	Tags         map[string]string `json:"tags,omitempty"`
}

// FileDeleteRequest requests deletion of a file.
type FileDeleteRequest struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Version   string `json:"version,omitempty"` // Delete specific version
	AgentID   string `json:"agent_id"`
}

// FileDeleteResponse acknowledges a delete request.
type FileDeleteResponse struct {
	RequestID string `json:"request_id"`
	Deleted   bool   `json:"deleted"`
	Error     string `json:"error,omitempty"`
}

// FileExistsRequest checks if a file exists.
type FileExistsRequest struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Version   string `json:"version,omitempty"`
	AgentID   string `json:"agent_id"`
}

// FileExistsResponse indicates if a file exists.
type FileExistsResponse struct {
	RequestID string        `json:"request_id"`
	Exists    bool          `json:"exists"`
	Metadata  *FileMetadata `json:"metadata,omitempty"` // If exists
}

// FileChangeEvent represents a file change notification.
type FileChangeEvent struct {
	EventID      string            `json:"event_id"`
	Path         string            `json:"path"`
	Action       FileAction        `json:"action"`
	Version      string            `json:"version,omitempty"`
	Checksum     string            `json:"checksum,omitempty"`
	Size         int64             `json:"size,omitempty"`
	ModifiedTime time.Time         `json:"modified_time"`
	Tags         map[string]string `json:"tags,omitempty"`
	Backend      string            `json:"backend,omitempty"`
}

// FileAction represents the type of file change.
type FileAction string

const (
	FileActionCreated FileAction = "created"
	FileActionUpdated FileAction = "updated"
	FileActionDeleted FileAction = "deleted"
)

// CacheInvalidation requests cache invalidation.
type CacheInvalidation struct {
	Paths     []string `json:"paths"`      // Paths or patterns to invalidate
	Recursive bool     `json:"recursive"`
	Reason    string   `json:"reason,omitempty"`
}

// TransferProgress reports transfer progress.
type TransferProgress struct {
	RequestID        string  `json:"request_id"`
	Path             string  `json:"path"`
	BytesTransferred int64   `json:"bytes_transferred"`
	TotalBytes       int64   `json:"total_bytes"`
	ChunksCompleted  int     `json:"chunks_completed"`
	TotalChunks      int     `json:"total_chunks"`
	PercentComplete  float64 `json:"percent_complete"`
}

// Priority levels for file requests.
const (
	PriorityNormal   = 0
	PriorityHigh     = 1
	PriorityCritical = 2
)

// Default configuration values.
const (
	DefaultChunkSize     = 1 << 20  // 1MB
	MaxChunkSize         = 10 << 20 // 10MB
	DefaultMaxFileSize   = 10 << 30 // 10GB
	DefaultCacheSize     = 1 << 30  // 1GB
	DefaultCacheTTL      = 24 * time.Hour
	DefaultRetryAttempts = 3
	DefaultRetryDelay    = 5 * time.Second
)

// NATS subject patterns for file operations.
const (
	SubjectFileRequest     = "kscore.%s.files.request.%s"     // cluster, namespace
	SubjectFileMetadata    = "kscore.%s.files.metadata.%s"    // cluster, namespace
	SubjectFileUpload      = "kscore.%s.files.upload.%s"      // cluster, namespace
	SubjectFileNotify      = "kscore.%s.files.notify.%s"      // cluster, namespace
	SubjectCacheInvalidate = "kscore.%s.files.cache.invalidate"
	SubjectFileAdmin       = "kscore.%s.files.admin.%s"       // cluster, operation
)

// MarshalJSON implements custom JSON marshaling for FileChunk.
// Data is base64 encoded automatically by encoding/json.
func (c *FileChunk) MarshalJSON() ([]byte, error) {
	type Alias FileChunk
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(c),
	})
}

// NamespaceFromPath extracts the namespace from a file path.
// For example, "/packages/nginx/1.24.deb" returns "packages".
func NamespaceFromPath(path string) string {
	if len(path) == 0 {
		return ""
	}
	// Remove leading slash
	if path[0] == '/' {
		path = path[1:]
	}
	// Find first slash
	for i, c := range path {
		if c == '/' {
			return path[:i]
		}
	}
	return path
}

// ValidatePath checks if a path is valid.
func ValidatePath(path string) error {
	if path == "" {
		return &FileError{Code: ErrCodeInvalidRequest, Message: "path is required"}
	}
	if path[0] != '/' {
		return &FileError{Code: ErrCodeInvalidRequest, Message: "path must start with /"}
	}
	// Check for path traversal
	if containsPathTraversal(path) {
		return &FileError{Code: ErrCodeInvalidRequest, Message: "path contains invalid characters"}
	}
	return nil
}

// containsPathTraversal checks for path traversal attempts.
func containsPathTraversal(path string) bool {
	// Check for ".." components
	for i := 0; i < len(path)-1; i++ {
		if path[i] == '.' && path[i+1] == '.' {
			if i == 0 || path[i-1] == '/' {
				if i+2 >= len(path) || path[i+2] == '/' {
					return true
				}
			}
		}
	}
	return false
}
