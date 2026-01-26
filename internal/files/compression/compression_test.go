package compression

import (
	"bytes"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Algorithm != AlgorithmGzip {
		t.Errorf("Default algorithm = %s, want gzip", cfg.Algorithm)
	}
	if cfg.Level != LevelBalanced {
		t.Errorf("Default level = %d, want %d", cfg.Level, LevelBalanced)
	}
	if cfg.MinSize != 1024 {
		t.Errorf("Default MinSize = %d, want 1024", cfg.MinSize)
	}
	if !cfg.SkipCompressed {
		t.Error("SkipCompressed should be true by default")
	}
}

func TestNewCompressor(t *testing.T) {
	// With nil config
	c := NewCompressor(nil)
	if c == nil {
		t.Fatal("NewCompressor returned nil")
	}
	if c.config.Algorithm != AlgorithmGzip {
		t.Error("Should use default config when nil")
	}

	// With custom config
	cfg := &Config{Algorithm: AlgorithmZstd}
	c = NewCompressor(cfg)
	if c.config.Algorithm != AlgorithmZstd {
		t.Error("Should use provided config")
	}
}

func TestCompressor_ShouldCompress(t *testing.T) {
	c := NewCompressor(DefaultConfig())

	tests := []struct {
		name        string
		size        int64
		contentType string
		expected    bool
	}{
		{"small file", 100, "text/plain", false},           // Below MinSize
		{"text file", 2000, "text/plain", true},            // Compressible type
		{"json file", 5000, "application/json", true},      // Compressible type
		{"jpeg image", 5000, "image/jpeg", false},          // Incompressible type
		{"gzip file", 5000, "application/gzip", false},     // Already compressed
		{"large text", 2000, "text/html", true},            // Compressible
		{"unknown type", 2000, "", true},                   // Unknown defaults to compressible
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.ShouldCompress(tt.size, tt.contentType)
			if got != tt.expected {
				t.Errorf("ShouldCompress(%d, %q) = %v, want %v",
					tt.size, tt.contentType, got, tt.expected)
			}
		})
	}
}

func TestCompressor_ShouldCompress_NoAlgorithm(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Algorithm = AlgorithmNone
	c := NewCompressor(cfg)

	if c.ShouldCompress(5000, "text/plain") {
		t.Error("Should not compress when algorithm is none")
	}
}

func TestCompressor_ShouldCompress_MaxSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 10000
	c := NewCompressor(cfg)

	if c.ShouldCompress(20000, "text/plain") {
		t.Error("Should not compress when above MaxSize")
	}
}

func TestCompressor_Compress_Empty(t *testing.T) {
	c := NewCompressor(nil)

	result, err := c.Compress([]byte{})
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	if len(result.Data) != 0 {
		t.Error("Empty input should return empty output")
	}
	if result.Algorithm != AlgorithmNone {
		t.Errorf("Empty input algorithm = %s, want none", result.Algorithm)
	}
}

func TestCompressor_Compress_Gzip(t *testing.T) {
	c := NewCompressor(nil)

	// Create compressible data (repeated text compresses well)
	original := []byte(strings.Repeat("Hello, World! ", 1000))

	result, err := c.Compress(original)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	if result.Algorithm != AlgorithmGzip {
		t.Errorf("Algorithm = %s, want gzip", result.Algorithm)
	}
	if result.OriginalSize != int64(len(original)) {
		t.Errorf("OriginalSize = %d, want %d", result.OriginalSize, len(original))
	}
	if result.CompressedSize >= result.OriginalSize {
		t.Errorf("Compressed size %d should be less than original %d",
			result.CompressedSize, result.OriginalSize)
	}
	if result.Ratio <= 1.0 {
		t.Errorf("Ratio = %f, should be > 1.0", result.Ratio)
	}
}

func TestCompressor_Compress_Incompressible(t *testing.T) {
	c := NewCompressor(nil)

	// Random-looking data doesn't compress well
	original := make([]byte, 1000)
	for i := range original {
		original[i] = byte(i * 17 % 256) // Pseudo-random pattern
	}

	result, err := c.Compress(original)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	// If compression made it larger, should return original with AlgorithmNone
	if result.CompressedSize > result.OriginalSize {
		t.Error("Should return original if compression is ineffective")
	}
}

func TestCompressor_Decompress_Gzip(t *testing.T) {
	c := NewCompressor(nil)

	original := []byte(strings.Repeat("Test data for compression ", 100))

	// Compress
	result, err := c.Compress(original)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	// Decompress
	decompressed, err := c.Decompress(result.Data, result.Algorithm)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, original) {
		t.Error("Decompressed data doesn't match original")
	}
}

func TestCompressor_Decompress_None(t *testing.T) {
	c := NewCompressor(nil)

	original := []byte("uncompressed data")

	decompressed, err := c.Decompress(original, AlgorithmNone)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(decompressed, original) {
		t.Error("AlgorithmNone should return data unchanged")
	}
}

func TestCompressor_Decompress_Empty(t *testing.T) {
	c := NewCompressor(nil)

	decompressed, err := c.Decompress([]byte{}, AlgorithmGzip)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if len(decompressed) != 0 {
		t.Error("Empty input should return empty output")
	}
}

func TestDetectAlgorithm(t *testing.T) {
	c := NewCompressor(nil)

	// Compress some data to get real compressed bytes
	original := []byte(strings.Repeat("test ", 100))
	result, _ := c.Compress(original)

	tests := []struct {
		name     string
		data     []byte
		expected Algorithm
	}{
		{"gzip data", result.Data, AlgorithmGzip},
		{"empty data", []byte{}, AlgorithmNone},
		{"short data", []byte{0x00}, AlgorithmNone},
		{"plain text", []byte("hello world"), AlgorithmNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectAlgorithm(tt.data)
			if got != tt.expected {
				t.Errorf("DetectAlgorithm() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestCompressor_Levels(t *testing.T) {
	original := []byte(strings.Repeat("Level test data ", 500))

	levels := []Level{LevelFastest, LevelFast, LevelBalanced, LevelBetter, LevelBest}

	for _, level := range levels {
		cfg := DefaultConfig()
		cfg.Level = level
		c := NewCompressor(cfg)

		result, err := c.Compress(original)
		if err != nil {
			t.Errorf("Level %d compress failed: %v", level, err)
			continue
		}

		if result.CompressedSize >= result.OriginalSize {
			t.Errorf("Level %d should compress data", level)
		}
	}
}

func TestNewStats(t *testing.T) {
	s := NewStats()

	if s == nil {
		t.Fatal("NewStats returned nil")
	}
	if s.ByAlgorithm == nil {
		t.Error("ByAlgorithm map should be initialized")
	}
}

func TestStats_RecordCompression(t *testing.T) {
	s := NewStats()

	result := &Result{
		Data:           make([]byte, 500),
		Algorithm:      AlgorithmGzip,
		OriginalSize:   1000,
		CompressedSize: 500,
		Ratio:          2.0,
	}

	s.RecordCompression(result)

	if s.TotalCompressed != 1000 {
		t.Errorf("TotalCompressed = %d, want 1000", s.TotalCompressed)
	}
	if s.BytesSaved != 500 {
		t.Errorf("BytesSaved = %d, want 500", s.BytesSaved)
	}
	if s.CompressionOps != 1 {
		t.Errorf("CompressionOps = %d, want 1", s.CompressionOps)
	}

	algStats, ok := s.ByAlgorithm[AlgorithmGzip]
	if !ok {
		t.Fatal("Should have gzip stats")
	}
	if algStats.Operations != 1 {
		t.Errorf("Gzip operations = %d, want 1", algStats.Operations)
	}
	if algStats.BytesIn != 1000 {
		t.Errorf("Gzip BytesIn = %d, want 1000", algStats.BytesIn)
	}
}

func TestStats_RecordDecompression(t *testing.T) {
	s := NewStats()

	s.RecordDecompression(500, 1000)

	if s.TotalDecompressed != 1000 {
		t.Errorf("TotalDecompressed = %d, want 1000", s.TotalDecompressed)
	}
	if s.DecompressionOps != 1 {
		t.Errorf("DecompressionOps = %d, want 1", s.DecompressionOps)
	}
}

func TestStats_Snapshot(t *testing.T) {
	s := NewStats()

	result := &Result{
		Algorithm:      AlgorithmGzip,
		OriginalSize:   1000,
		CompressedSize: 500,
		Ratio:          2.0,
	}
	s.RecordCompression(result)

	snapshot := s.Snapshot()

	// Modify original
	s.RecordCompression(result)

	// Snapshot should be unchanged
	if snapshot.CompressionOps != 1 {
		t.Error("Snapshot should be independent copy")
	}
}

func TestContentTypeCompressible(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"text/plain", true},
		{"text/html", true},
		{"application/json", true},
		{"image/jpeg", false},
		{"image/png", false},
		{"application/octet-stream", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ContentTypeCompressible(tt.contentType)
		if got != tt.expected {
			t.Errorf("ContentTypeCompressible(%q) = %v, want %v",
				tt.contentType, got, tt.expected)
		}
	}
}

func TestExtensionCompressible(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".txt", true},
		{".json", true},
		{".html", true},
		{".go", true},
		{".py", true},
		{".jpg", false},
		{".png", false},
		{".zip", false},
		{".exe", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ExtensionCompressible(tt.ext)
		if got != tt.expected {
			t.Errorf("ExtensionCompressible(%q) = %v, want %v",
				tt.ext, got, tt.expected)
		}
	}
}

func TestCompressor_RoundTrip(t *testing.T) {
	c := NewCompressor(nil)

	testCases := [][]byte{
		[]byte(strings.Repeat("a", 10000)),
		[]byte(strings.Repeat("Hello, World! ", 1000)),
		[]byte(`{"key": "value", "numbers": [1,2,3,4,5]}`),
		bytes.Repeat([]byte{0x00, 0xFF}, 5000),
	}

	for i, original := range testCases {
		result, err := c.Compress(original)
		if err != nil {
			t.Errorf("Case %d: Compress failed: %v", i, err)
			continue
		}

		decompressed, err := c.Decompress(result.Data, result.Algorithm)
		if err != nil {
			t.Errorf("Case %d: Decompress failed: %v", i, err)
			continue
		}

		if !bytes.Equal(decompressed, original) {
			t.Errorf("Case %d: Round trip failed", i)
		}
	}
}

func BenchmarkCompress(b *testing.B) {
	c := NewCompressor(nil)
	data := []byte(strings.Repeat("Benchmark compression test data ", 1000))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Compress(data)
	}
}

func BenchmarkDecompress(b *testing.B) {
	c := NewCompressor(nil)
	data := []byte(strings.Repeat("Benchmark compression test data ", 1000))
	result, _ := c.Compress(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Decompress(result.Data, result.Algorithm)
	}
}
