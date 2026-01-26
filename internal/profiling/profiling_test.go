package profiling

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("Expected profiling to be enabled by default")
	}

	if config.ListenAddr != "localhost:6060" {
		t.Errorf("Expected default listen address localhost:6060, got %s", config.ListenAddr)
	}

	if config.BlockProfileRate != 1 {
		t.Errorf("Expected default block profile rate 1, got %d", config.BlockProfileRate)
	}

	if config.MutexProfileFraction != 1 {
		t.Errorf("Expected default mutex profile fraction 1, got %d", config.MutexProfileFraction)
	}
}

func TestNewServer(t *testing.T) {
	config := &ProfileConfig{
		Enabled:              true,
		ListenAddr:           "localhost:16060",
		BlockProfileRate:     1,
		MutexProfileFraction: 1,
	}

	server := NewServer(config)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.config.ListenAddr != "localhost:16060" {
		t.Errorf("Expected listen address localhost:16060, got %s", server.config.ListenAddr)
	}
}

func TestServerStartStop(t *testing.T) {
	config := &ProfileConfig{
		Enabled:              true,
		ListenAddr:           "localhost:16061",
		BlockProfileRate:     1,
		MutexProfileFraction: 1,
	}

	server := NewServer(config)
	ctx := context.Background()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Test that pprof endpoint is accessible
	var resp *http.Response
	if err := helpers.WaitForTimeout(2*time.Second, 50*time.Millisecond, func() (bool, error) {
		var reqErr error
		resp, reqErr = http.Get("http://localhost:16061/debug/pprof/")
		if reqErr != nil {
			return false, nil
		}
		return true, nil
	}); err != nil {
		t.Fatalf("Failed to access pprof endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Stop(stopCtx); err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

func TestServerDisabled(t *testing.T) {
	config := &ProfileConfig{
		Enabled: false,
	}

	server := NewServer(config)
	ctx := context.Background()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start should not error when disabled: %v", err)
	}

	if err := server.Stop(ctx); err != nil {
		t.Fatalf("Stop should not error when disabled: %v", err)
	}
}

func TestCaptureHeapProfile(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:   ProfileTypeHeap,
		Output: &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture heap profile: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Heap profile is empty")
	}
}

func TestCaptureGoroutineProfile(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:   ProfileTypeGoroutine,
		Output: &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture goroutine profile: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Goroutine profile is empty")
	}
}

func TestCaptureCPUProfile(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:     ProfileTypeCPU,
		Duration: 100 * time.Millisecond,
		Output:   &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture CPU profile: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("CPU profile is empty")
	}
}

func TestCaptureBlockProfile(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:   ProfileTypeBlock,
		Output: &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture block profile: %v", err)
	}

	// Block profile might be empty if no blocking occurred
	// Just verify no error
}

func TestCaptureMutexProfile(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:   ProfileTypeMutex,
		Output: &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture mutex profile: %v", err)
	}

	// Mutex profile might be empty if no contention occurred
	// Just verify no error
}

func TestCaptureThreadCreateProfile(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:   ProfileTypeThreads,
		Output: &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture threadcreate profile: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Threadcreate profile is empty")
	}
}

func TestCaptureAllocsProfile(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:   ProfileTypeAllocs,
		Output: &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture allocs profile: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Allocs profile is empty")
	}
}

func TestCaptureTrace(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:     ProfileTypeTrace,
		Duration: 100 * time.Millisecond,
		Output:   &buf,
	}

	if err := CaptureProfile(req); err != nil {
		t.Fatalf("Failed to capture trace: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("Trace is empty")
	}
}

func TestCaptureProfileInvalidType(t *testing.T) {
	var buf bytes.Buffer

	req := &ProfileRequest{
		Type:   ProfileType("invalid"),
		Output: &buf,
	}

	if err := CaptureProfile(req); err == nil {
		t.Error("Expected error for invalid profile type")
	}
}

func TestCaptureProfileNilOutput(t *testing.T) {
	req := &ProfileRequest{
		Type:   ProfileTypeHeap,
		Output: nil,
	}

	if err := CaptureProfile(req); err == nil {
		t.Error("Expected error for nil output")
	}
}

func TestGetStats(t *testing.T) {
	stats := GetStats()

	if stats == nil {
		t.Fatal("GetStats returned nil")
	}

	if stats.NumGoroutine <= 0 {
		t.Error("Expected at least one goroutine")
	}

	if stats.NumCPU <= 0 {
		t.Error("Expected at least one CPU")
	}

	if stats.GOMAXPROCS <= 0 {
		t.Error("Expected GOMAXPROCS > 0")
	}

	// MemStats should have some data
	if stats.MemStats.Alloc == 0 && stats.MemStats.TotalAlloc == 0 {
		t.Error("Expected memory allocation stats")
	}
}
