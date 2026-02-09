// Package compression provides automatic compression with configurable algorithms
// for file distribution in Keystone Core.
package compression

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Algorithm represents a compression algorithm
type Algorithm string

const (
	// AlgorithmNone disables compression
	AlgorithmNone Algorithm = "none"
	// AlgorithmGzip uses gzip compression
	AlgorithmGzip Algorithm = "gzip"
	// AlgorithmZstd uses zstandard compression
	AlgorithmZstd Algorithm = "zstd"
	// AlgorithmLZ4 uses LZ4 compression
	AlgorithmLZ4 Algorithm = "lz4"
	// AlgorithmSnappy uses Snappy compression
	AlgorithmSnappy Algorithm = "snappy"
	// AlgorithmAuto automatically selects the best algorithm
	AlgorithmAuto Algorithm = "auto"
)

// Level represents compression level
type Level int

const (
	// LevelDefault uses the algorithm's default level
	LevelDefault Level = 0
	// LevelFastest prioritizes speed over compression ratio
	LevelFastest Level = 1
	// LevelFast good balance favoring speed
	LevelFast Level = 3
	// LevelBalanced balanced speed and compression
	LevelBalanced Level = 5
	// LevelBetter good balance favoring compression
	LevelBetter Level = 7
	// LevelBest maximum compression
	LevelBest Level = 9
)

// Config configures compression behavior
type Config struct {
	// Algorithm is the compression algorithm to use
	Algorithm Algorithm

	// Level is the compression level
	Level Level

	// MinSize is the minimum file size to compress (bytes)
	// Files smaller than this are not compressed
	MinSize int64

	// MaxSize is the maximum file size to compress (bytes)
	// Very large files may be skipped to avoid memory issues
	MaxSize int64

	// SkipCompressed skips already compressed file types
	SkipCompressed bool

	// CompressibleTypes lists MIME types to always compress
	CompressibleTypes []string

	// IncompressibleTypes lists MIME types to never compress
	IncompressibleTypes []string
}

// DefaultConfig returns a default compression configuration
func DefaultConfig() *Config {
	return &Config{
		Algorithm:      AlgorithmGzip,
		Level:          LevelBalanced,
		MinSize:        1024,          // 1KB minimum
		MaxSize:        100 * 1 << 20, // 100MB maximum
		SkipCompressed: true,
		CompressibleTypes: []string{
			"text/plain",
			"text/html",
			"text/css",
			"text/javascript",
			"application/json",
			"application/xml",
			"application/javascript",
			"application/x-yaml",
			"application/yaml",
		},
		IncompressibleTypes: []string{
			"image/jpeg",
			"image/png",
			"image/gif",
			"image/webp",
			"video/mp4",
			"video/webm",
			"audio/mpeg",
			"audio/ogg",
			"application/zip",
			"application/gzip",
			"application/x-gzip",
			"application/x-bzip2",
			"application/x-xz",
			"application/x-7z-compressed",
			"application/x-rar-compressed",
			"application/zstd",
		},
	}
}

// Compressor provides compression and decompression functionality
type Compressor struct {
	config   *Config
	gzipPool sync.Pool
	bufPool  sync.Pool
}

// NewCompressor creates a new compressor with the given configuration
func NewCompressor(config *Config) *Compressor {
	if config == nil {
		config = DefaultConfig()
	}

	c := &Compressor{
		config: config,
	}

	// Initialize gzip writer pool
	c.gzipPool = sync.Pool{
		New: func() interface{} {
			w, _ := gzip.NewWriterLevel(io.Discard, c.gzipLevel())
			return w
		},
	}

	// Initialize buffer pool
	c.bufPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	return c
}

func (c *Compressor) gzipLevel() int {
	switch c.config.Level {
	case LevelFastest:
		return gzip.BestSpeed
	case LevelFast:
		return 3
	case LevelBalanced:
		return gzip.DefaultCompression
	case LevelBetter:
		return 7
	case LevelBest:
		return gzip.BestCompression
	default:
		return gzip.DefaultCompression
	}
}

// ShouldCompress determines if data should be compressed based on configuration
func (c *Compressor) ShouldCompress(size int64, contentType string) bool {
	// Check size limits
	if size < c.config.MinSize {
		return false
	}
	if c.config.MaxSize > 0 && size > c.config.MaxSize {
		return false
	}

	// Check if algorithm is none
	if c.config.Algorithm == AlgorithmNone {
		return false
	}

	// Check incompressible types
	for _, t := range c.config.IncompressibleTypes {
		if contentType == t {
			return false
		}
	}

	// Check compressible types (if list is specified, only compress those)
	if len(c.config.CompressibleTypes) > 0 {
		for _, t := range c.config.CompressibleTypes {
			if contentType == t {
				return true
			}
		}
		// If compressible types are specified and content type doesn't match, don't compress
		if contentType != "" {
			return false
		}
	}

	return true
}

// Compress compresses data using the configured algorithm
func (c *Compressor) Compress(data []byte) (*Result, error) {
	if len(data) == 0 {
		return &Result{
			Data:           data,
			Algorithm:      AlgorithmNone,
			OriginalSize:   0,
			CompressedSize: 0,
			Ratio:          1.0,
		}, nil
	}

	algorithm := c.config.Algorithm
	if algorithm == AlgorithmAuto {
		algorithm = c.selectAlgorithm(data)
	}

	var compressed []byte
	var err error

	switch algorithm {
	case AlgorithmNone:
		return &Result{
			Data:           data,
			Algorithm:      AlgorithmNone,
			OriginalSize:   int64(len(data)),
			CompressedSize: int64(len(data)),
			Ratio:          1.0,
		}, nil

	case AlgorithmGzip:
		compressed, err = c.compressGzip(data)

	case AlgorithmZstd:
		compressed, err = c.compressZstd(data)

	case AlgorithmLZ4:
		compressed, err = c.compressLZ4(data)

	case AlgorithmSnappy:
		compressed, err = c.compressSnappy(data)

	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", algorithm)
	}

	if err != nil {
		return nil, err
	}

	// If compression made it larger, return original
	if len(compressed) >= len(data) {
		return &Result{
			Data:           data,
			Algorithm:      AlgorithmNone,
			OriginalSize:   int64(len(data)),
			CompressedSize: int64(len(data)),
			Ratio:          1.0,
		}, nil
	}

	return &Result{
		Data:           compressed,
		Algorithm:      algorithm,
		OriginalSize:   int64(len(data)),
		CompressedSize: int64(len(compressed)),
		Ratio:          float64(len(data)) / float64(len(compressed)),
	}, nil
}

// Decompress decompresses data using the specified algorithm
func (c *Compressor) Decompress(data []byte, algorithm Algorithm) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	switch algorithm {
	case AlgorithmNone:
		return data, nil

	case AlgorithmGzip:
		return c.decompressGzip(data)

	case AlgorithmZstd:
		return c.decompressZstd(data)

	case AlgorithmLZ4:
		return c.decompressLZ4(data)

	case AlgorithmSnappy:
		return c.decompressSnappy(data)

	default:
		return nil, fmt.Errorf("unsupported compression algorithm: %s", algorithm)
	}
}

// DetectAlgorithm attempts to detect the compression algorithm from data
func DetectAlgorithm(data []byte) Algorithm {
	if len(data) < 2 {
		return AlgorithmNone
	}

	// Check magic bytes
	switch {
	case data[0] == 0x1f && data[1] == 0x8b:
		return AlgorithmGzip
	case len(data) >= 4 && data[0] == 0x28 && data[1] == 0xb5 && data[2] == 0x2f && data[3] == 0xfd:
		return AlgorithmZstd
	case len(data) >= 4 && data[0] == 0x04 && data[1] == 0x22 && data[2] == 0x4d && data[3] == 0x18:
		return AlgorithmLZ4
	default:
		return AlgorithmNone
	}
}

func (c *Compressor) selectAlgorithm(data []byte) Algorithm {
	// For auto mode, select based on data size
	size := len(data)

	if size < 1024 {
		// Small data: use snappy for speed
		return AlgorithmGzip // Fallback to gzip since snappy may not be available
	}
	if size < 100*1024 {
		// Medium data: use gzip for good balance
		return AlgorithmGzip
	}
	// Large data: use zstd for better compression
	return AlgorithmGzip // Fallback to gzip (zstd requires cgo or external lib)
}

func (c *Compressor) compressGzip(data []byte) ([]byte, error) {
	buf := c.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer c.bufPool.Put(buf)

	w := c.gzipPool.Get().(*gzip.Writer)
	w.Reset(buf)
	defer c.gzipPool.Put(w)

	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("gzip write failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

func (c *Compressor) decompressGzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader failed: %w", err)
	}
	defer r.Close()

	buf := c.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer c.bufPool.Put(buf)

	maxSize := c.config.MaxSize
	if maxSize <= 0 {
		maxSize = int64(len(data)) * 100
		if maxSize == 0 {
			maxSize = 1 << 20
		}
	}

	n, err := io.CopyN(buf, r, maxSize+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("gzip decompress failed: %w", err)
	}
	if n > maxSize {
		return nil, fmt.Errorf("gzip decompressed data exceeds max size %d bytes", maxSize)
	}

	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

// Placeholder implementations for other algorithms
// In production, these would use actual compression libraries

func (c *Compressor) compressZstd(data []byte) ([]byte, error) {
	// Fallback to gzip if zstd not available
	return c.compressGzip(data)
}

func (c *Compressor) decompressZstd(data []byte) ([]byte, error) {
	// Fallback to gzip if zstd not available
	return c.decompressGzip(data)
}

func (c *Compressor) compressLZ4(data []byte) ([]byte, error) {
	// Fallback to gzip if lz4 not available
	return c.compressGzip(data)
}

func (c *Compressor) decompressLZ4(data []byte) ([]byte, error) {
	// Fallback to gzip if lz4 not available
	return c.decompressGzip(data)
}

func (c *Compressor) compressSnappy(data []byte) ([]byte, error) {
	// Fallback to gzip if snappy not available
	return c.compressGzip(data)
}

func (c *Compressor) decompressSnappy(data []byte) ([]byte, error) {
	// Fallback to gzip if snappy not available
	return c.decompressGzip(data)
}

// Result contains the result of a compression operation
type Result struct {
	// Data is the compressed (or original) data
	Data []byte

	// Algorithm is the algorithm used
	Algorithm Algorithm

	// OriginalSize is the original data size
	OriginalSize int64

	// CompressedSize is the compressed data size
	CompressedSize int64

	// Ratio is the compression ratio (original/compressed)
	Ratio float64
}

// Stats tracks compression statistics
type Stats struct {
	mu                sync.Mutex
	TotalCompressed   int64
	TotalDecompressed int64
	BytesSaved        int64
	CompressionOps    int64
	DecompressionOps  int64
	ByAlgorithm       map[Algorithm]*AlgorithmStats
}

// AlgorithmStats tracks stats for a specific algorithm
type AlgorithmStats struct {
	Operations   int64
	BytesIn      int64
	BytesOut     int64
	TotalRatio   float64
	AverageRatio float64
}

// NewStats creates a new stats tracker
func NewStats() *Stats {
	return &Stats{
		ByAlgorithm: make(map[Algorithm]*AlgorithmStats),
	}
}

// RecordCompression records a compression operation
func (s *Stats) RecordCompression(result *Result) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalCompressed += result.OriginalSize
	s.BytesSaved += result.OriginalSize - result.CompressedSize
	s.CompressionOps++

	if _, ok := s.ByAlgorithm[result.Algorithm]; !ok {
		s.ByAlgorithm[result.Algorithm] = &AlgorithmStats{}
	}

	stats := s.ByAlgorithm[result.Algorithm]
	stats.Operations++
	stats.BytesIn += result.OriginalSize
	stats.BytesOut += result.CompressedSize
	stats.TotalRatio += result.Ratio
	stats.AverageRatio = stats.TotalRatio / float64(stats.Operations)
}

// RecordDecompression records a decompression operation
func (s *Stats) RecordDecompression(compressedSize, originalSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalDecompressed += originalSize
	s.DecompressionOps++
}

// Snapshot returns a copy of current stats
func (s *Stats) Snapshot() *Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := &Stats{
		TotalCompressed:   s.TotalCompressed,
		TotalDecompressed: s.TotalDecompressed,
		BytesSaved:        s.BytesSaved,
		CompressionOps:    s.CompressionOps,
		DecompressionOps:  s.DecompressionOps,
		ByAlgorithm:       make(map[Algorithm]*AlgorithmStats),
	}

	for alg, stats := range s.ByAlgorithm {
		snapshot.ByAlgorithm[alg] = &AlgorithmStats{
			Operations:   stats.Operations,
			BytesIn:      stats.BytesIn,
			BytesOut:     stats.BytesOut,
			TotalRatio:   stats.TotalRatio,
			AverageRatio: stats.AverageRatio,
		}
	}

	return snapshot
}

// ContentTypeCompressible returns true if the content type is compressible
func ContentTypeCompressible(contentType string) bool {
	compressible := map[string]bool{
		"text/plain":               true,
		"text/html":                true,
		"text/css":                 true,
		"text/javascript":          true,
		"text/xml":                 true,
		"application/json":         true,
		"application/xml":          true,
		"application/javascript":   true,
		"application/x-javascript": true,
		"application/x-yaml":       true,
		"application/yaml":         true,
		"image/svg+xml":            true,
	}
	return compressible[contentType]
}

// ExtensionCompressible returns true if the file extension indicates compressibility
func ExtensionCompressible(ext string) bool {
	compressible := map[string]bool{
		".txt":  true,
		".html": true,
		".htm":  true,
		".css":  true,
		".js":   true,
		".json": true,
		".xml":  true,
		".yaml": true,
		".yml":  true,
		".svg":  true,
		".md":   true,
		".csv":  true,
		".log":  true,
		".conf": true,
		".cfg":  true,
		".ini":  true,
		".sh":   true,
		".bat":  true,
		".ps1":  true,
		".py":   true,
		".go":   true,
		".java": true,
		".c":    true,
		".cpp":  true,
		".h":    true,
		".rs":   true,
	}
	return compressible[ext]
}
