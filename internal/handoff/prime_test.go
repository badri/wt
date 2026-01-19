package handoff

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/badri/wt/internal/config"
)

func TestPrimeOptionsStruct(t *testing.T) {
	opts := &PrimeOptions{
		Quiet:     true,
		NoBdPrime: true,
	}

	if !opts.Quiet {
		t.Error("Quiet should be true")
	}
	if !opts.NoBdPrime {
		t.Error("NoBdPrime should be true")
	}
}

func TestPrimeResultStruct(t *testing.T) {
	now := time.Now()
	result := &PrimeResult{
		IsPostHandoff:  true,
		PrevSession:    "test-session",
		HandoffTime:    now,
		HandoffContent: "test content",
		BdPrimeOutput:  "bd output",
	}

	if !result.IsPostHandoff {
		t.Error("IsPostHandoff should be true")
	}
	if result.PrevSession != "test-session" {
		t.Errorf("expected PrevSession 'test-session', got '%s'", result.PrevSession)
	}
	if !result.HandoffTime.Equal(now) {
		t.Error("HandoffTime not set correctly")
	}
	if result.HandoffContent != "test content" {
		t.Error("HandoffContent not set correctly")
	}
	if result.BdPrimeOutput != "bd output" {
		t.Error("BdPrimeOutput not set correctly")
	}
}

func TestPrimeNoMarker(t *testing.T) {
	// Create a temp directory for config
	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	opts := &PrimeOptions{
		Quiet:     true,
		NoBdPrime: true, // Skip bd prime to avoid external dependency
	}

	result, err := Prime(cfg, opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result.IsPostHandoff {
		t.Error("expected IsPostHandoff to be false when no marker exists")
	}
	if result.PrevSession != "" {
		t.Errorf("expected empty PrevSession, got '%s'", result.PrevSession)
	}
}

func TestPrimeWithMarker(t *testing.T) {
	// Create a temp directory for config
	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Create marker
	runtimeDir := filepath.Join(cfg.ConfigDir(), RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, HandoffMarkerFile)
	timestamp := time.Now().Format(time.RFC3339)
	sessionName := "prev-session"
	content := timestamp + "\n" + sessionName + "\n"
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	opts := &PrimeOptions{
		Quiet:     true,
		NoBdPrime: true,
	}

	result, err := Prime(cfg, opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !result.IsPostHandoff {
		t.Error("expected IsPostHandoff to be true when marker exists")
	}
	if result.PrevSession != sessionName {
		t.Errorf("expected PrevSession '%s', got '%s'", sessionName, result.PrevSession)
	}

	// Verify marker was cleared
	exists, _, _, _ := CheckMarker(cfg)
	if exists {
		t.Error("expected marker to be cleared after Prime")
	}
}

func TestQuickPrimeNoMarker(t *testing.T) {
	// Create a temp directory for config
	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// QuickPrime should not error when no marker exists
	if err := QuickPrime(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQuickPrimeWithMarker(t *testing.T) {
	// Create a temp directory for config
	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Create marker
	runtimeDir := filepath.Join(cfg.ConfigDir(), RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, HandoffMarkerFile)
	timestamp := time.Now().Format(time.RFC3339)
	content := timestamp + "\ntest-session\n"
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// QuickPrime should clear the marker
	if err := QuickPrime(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify marker was cleared
	exists, _, _, _ := CheckMarker(cfg)
	if exists {
		t.Error("expected marker to be cleared after QuickPrime")
	}
}

func TestGenerateSummary(t *testing.T) {
	// Create a temp directory for config
	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	summary, err := GenerateSummary(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should contain at least the header
	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Should contain "Current State Summary" header
	if !containsString(summary, "Current State Summary") {
		t.Error("expected summary to contain 'Current State Summary'")
	}

	// Should contain "Active Sessions" section
	if !containsString(summary, "Active Sessions") {
		t.Error("expected summary to contain 'Active Sessions'")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestOutputPrimeResultNotPostHandoff(t *testing.T) {
	result := &PrimeResult{
		IsPostHandoff:  false,
		HandoffContent: "",
		BdPrimeOutput:  "",
	}

	// This should not panic - just calling to verify it runs
	OutputPrimeResult(result)
}

func TestOutputPrimeResultPostHandoff(t *testing.T) {
	result := &PrimeResult{
		IsPostHandoff:  true,
		PrevSession:    "old-session",
		HandoffTime:    time.Now(),
		HandoffContent: "Some context",
		BdPrimeOutput:  "",
	}

	// This should not panic - just calling to verify it runs
	OutputPrimeResult(result)
}
