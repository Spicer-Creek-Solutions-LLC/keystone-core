package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// CompressionInfo contains information about a compression type
type CompressionInfo struct {
	Type       CompressionType
	Extension  string   // File extension (e.g., ".gz")
	Command    string   // Compression command
	DecompCmd  string   // Decompression command
	Args       []string // Default compression args
	DecompArgs []string // Default decompression args
}

// compressionRegistry maps compression types to their info
var compressionRegistry = map[CompressionType]CompressionInfo{
	CompressionTypeNone: {
		Type:      CompressionTypeNone,
		Extension: "",
	},
	CompressionTypeGzip: {
		Type:       CompressionTypeGzip,
		Extension:  ".gz",
		Command:    "gzip",
		DecompCmd:  "gzip",
		Args:       []string{"-c"},
		DecompArgs: []string{"-d", "-c"},
	},
	CompressionTypeBzip2: {
		Type:       CompressionTypeBzip2,
		Extension:  ".bz2",
		Command:    "bzip2",
		DecompCmd:  "bzip2",
		Args:       []string{"-c"},
		DecompArgs: []string{"-d", "-c"},
	},
	CompressionTypeXz: {
		Type:       CompressionTypeXz,
		Extension:  ".xz",
		Command:    "xz",
		DecompCmd:  "xz",
		Args:       []string{"-c"},
		DecompArgs: []string{"-d", "-c"},
	},
	CompressionTypeZstd: {
		Type:       CompressionTypeZstd,
		Extension:  ".zst",
		Command:    "zstd",
		DecompCmd:  "zstd",
		Args:       []string{"-c"},
		DecompArgs: []string{"-d", "-c"},
	},
	CompressionTypeLz4: {
		Type:       CompressionTypeLz4,
		Extension:  ".lz4",
		Command:    "lz4",
		DecompCmd:  "lz4",
		Args:       []string{"-c"},
		DecompArgs: []string{"-d", "-c"},
	},
}

// GetCompressionInfo returns information about a compression type
func GetCompressionInfo(t CompressionType) (CompressionInfo, bool) {
	info, ok := compressionRegistry[t]
	return info, ok
}

// GetFileExtension returns the file extension for a compression type
func GetFileExtension(t CompressionType) string {
	if info, ok := compressionRegistry[t]; ok {
		return info.Extension
	}
	return ""
}

// ParseCompressionType parses a compression type string
func ParseCompressionType(s string) (CompressionType, error) {
	switch strings.ToLower(s) {
	case "none", "":
		return CompressionTypeNone, nil
	case "gzip", "gz":
		return CompressionTypeGzip, nil
	case "bzip2", "bz2":
		return CompressionTypeBzip2, nil
	case "xz", "lzma":
		return CompressionTypeXz, nil
	case "zstd", "zstandard":
		return CompressionTypeZstd, nil
	case "lz4":
		return CompressionTypeLz4, nil
	default:
		return CompressionTypeNone, fmt.Errorf("unknown compression type: %s", s)
	}
}

// DetectCompressionFromFilename detects compression type from filename
func DetectCompressionFromFilename(filename string) CompressionType {
	switch {
	case strings.HasSuffix(filename, ".gz"), strings.HasSuffix(filename, ".gzip"):
		return CompressionTypeGzip
	case strings.HasSuffix(filename, ".bz2"), strings.HasSuffix(filename, ".bzip2"):
		return CompressionTypeBzip2
	case strings.HasSuffix(filename, ".xz"), strings.HasSuffix(filename, ".lzma"):
		return CompressionTypeXz
	case strings.HasSuffix(filename, ".zst"), strings.HasSuffix(filename, ".zstd"):
		return CompressionTypeZstd
	case strings.HasSuffix(filename, ".lz4"):
		return CompressionTypeLz4
	default:
		return CompressionTypeNone
	}
}

// Compressor handles compression/decompression operations
type Compressor struct {
	compressionType CompressionType
	config          CompressionConfig
	logger          Logger
}

// NewCompressor creates a new compressor
func NewCompressor(config CompressionConfig, logger Logger) *Compressor {
	if logger == nil {
		logger = &noopLogger{}
	}
	return &Compressor{
		compressionType: config.Type,
		config:          config,
		logger:          logger,
	}
}

// Compress compresses data from reader to writer using streaming
func (c *Compressor) Compress(ctx context.Context, r io.Reader, w io.Writer) error {
	if c.compressionType == CompressionTypeNone {
		_, err := io.Copy(w, r)
		return err
	}

	// For gzip, we can use Go's built-in implementation for efficiency
	if c.compressionType == CompressionTypeGzip && c.config.Level == 0 {
		gzw := gzip.NewWriter(w)
		defer gzw.Close()
		_, err := io.Copy(gzw, r)
		return err
	}

	// Use external command for other compression types
	info, ok := compressionRegistry[c.compressionType]
	if !ok {
		return fmt.Errorf("unsupported compression type: %s", c.compressionType)
	}

	args := append([]string{}, info.Args...)

	// Add compression level if specified
	if c.config.Level > 0 {
		args = append(args, fmt.Sprintf("-%d", c.config.Level))
	}

	// Add thread count for compressors that support it
	if c.config.Threads > 0 {
		switch c.compressionType {
		case CompressionTypeXz:
			args = append(args, fmt.Sprintf("--threads=%d", c.config.Threads))
		case CompressionTypeZstd:
			args = append(args, fmt.Sprintf("-T%d", c.config.Threads))
		}
	}

	cmd := exec.CommandContext(ctx, info.Command, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	cmd.Stdin = r
	cmd.Stdout = w

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	c.logger.Debug("compressing with external command", "cmd", info.Command, "args", args)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s compression failed: %w - %s", info.Command, err, stderr.String())
	}

	return nil
}

// Decompress decompresses data from reader to writer using streaming
func (c *Compressor) Decompress(ctx context.Context, r io.Reader, w io.Writer) error {
	if c.compressionType == CompressionTypeNone {
		_, err := io.Copy(w, r)
		return err
	}

	// For gzip, we can use Go's built-in implementation for efficiency
	if c.compressionType == CompressionTypeGzip {
		gzr, err := gzip.NewReader(r)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzr.Close()
		_, err = io.Copy(w, gzr) // nosemgrep: go.lang.security.decompression_bomb.potential-dos-via-decompression-bomb -- backup artifacts are generated by trusted workflows and may exceed fixed limits
		return err
	}

	// Use external command for other compression types
	info, ok := compressionRegistry[c.compressionType]
	if !ok {
		return fmt.Errorf("unsupported compression type: %s", c.compressionType)
	}

	args := append([]string{}, info.DecompArgs...)

	cmd := exec.CommandContext(ctx, info.DecompCmd, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	cmd.Stdin = r
	cmd.Stdout = w

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	c.logger.Debug("decompressing with external command", "cmd", info.DecompCmd, "args", args)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s decompression failed: %w - %s", info.DecompCmd, err, stderr.String())
	}

	return nil
}

// CompressToWriter returns a writer that compresses data written to it
// The returned writer must be closed when done
func (c *Compressor) CompressToWriter(ctx context.Context, w io.Writer) (io.WriteCloser, error) {
	if c.compressionType == CompressionTypeNone {
		return &nopWriteCloser{w}, nil
	}

	// For gzip with default level, use Go's built-in implementation
	if c.compressionType == CompressionTypeGzip && c.config.Level == 0 {
		return gzip.NewWriter(w), nil
	}

	// For other compression types, we need to use a pipe with external command
	info, ok := compressionRegistry[c.compressionType]
	if !ok {
		return nil, fmt.Errorf("unsupported compression type: %s", c.compressionType)
	}

	pr, pw := io.Pipe()

	args := append([]string{}, info.Args...)
	if c.config.Level > 0 {
		args = append(args, fmt.Sprintf("-%d", c.config.Level))
	}
	if c.config.Threads > 0 {
		switch c.compressionType {
		case CompressionTypeXz:
			args = append(args, fmt.Sprintf("--threads=%d", c.config.Threads))
		case CompressionTypeZstd:
			args = append(args, fmt.Sprintf("-T%d", c.config.Threads))
		}
	}

	cmd := exec.CommandContext(ctx, info.Command, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- command execution is intentional and inputs are validated/controlled
	cmd.Stdin = pr
	cmd.Stdout = w

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("failed to start %s: %w", info.Command, err)
	}

	return &cmdWriteCloser{
		pw:     pw,
		pr:     pr,
		cmd:    cmd,
		stderr: &stderr,
	}, nil
}

// nopWriteCloser wraps a writer with a no-op close
type nopWriteCloser struct {
	io.Writer
}

func (n *nopWriteCloser) Close() error {
	return nil
}

// cmdWriteCloser wraps a pipe writer that feeds a compression command
type cmdWriteCloser struct {
	pw     *io.PipeWriter
	pr     *io.PipeReader
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func (c *cmdWriteCloser) Write(p []byte) (n int, err error) {
	return c.pw.Write(p)
}

func (c *cmdWriteCloser) Close() error {
	// Close the write end of the pipe to signal EOF to the command
	c.pw.Close()

	// Wait for command to finish
	if err := c.cmd.Wait(); err != nil {
		c.pr.Close()
		return fmt.Errorf("compression command failed: %w - %s", err, c.stderr.String())
	}

	c.pr.Close()
	return nil
}

// IsCompressionAvailable checks if a compression type's command is available
func IsCompressionAvailable(t CompressionType) bool {
	if t == CompressionTypeNone {
		return true
	}
	if t == CompressionTypeGzip {
		// gzip is always available via Go's stdlib
		return true
	}

	info, ok := compressionRegistry[t]
	if !ok {
		return false
	}

	_, err := exec.LookPath(info.Command)
	return err == nil
}

// ListAvailableCompression returns a list of available compression types
func ListAvailableCompression() []CompressionType {
	available := []CompressionType{CompressionTypeNone, CompressionTypeGzip}

	for _, t := range []CompressionType{CompressionTypeBzip2, CompressionTypeXz, CompressionTypeZstd, CompressionTypeLz4} {
		if IsCompressionAvailable(t) {
			available = append(available, t)
		}
	}

	return available
}
