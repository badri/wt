package handoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/badri/wt/internal/config"
)

func TestCheckMarkerNotExists(t *testing.T) {
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

	// Check marker when it doesn't exist
	exists, prevSession, handoffTime, err := CheckMarker(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected marker to not exist")
	}
	if prevSession != "" {
		t.Errorf("expected empty prevSession, got %s", prevSession)
	}
	if !handoffTime.IsZero() {
		t.Errorf("expected zero handoffTime, got %v", handoffTime)
	}
}

func TestWriteAndCheckMarker(t *testing.T) {
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

	// Write marker manually (simulating what writeMarker does)
	runtimeDir := filepath.Join(cfg.ConfigDir(), RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, HandoffMarkerFile)
	timestamp := time.Now().Format(time.RFC3339)
	sessionName := "test-session"
	content := timestamp + "\n" + sessionName + "\n"
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// Check marker
	exists, prevSession, handoffTime, err := CheckMarker(cfg)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected marker to exist")
	}
	if prevSession != sessionName {
		t.Errorf("expected prevSession %s, got %s", sessionName, prevSession)
	}
	if handoffTime.IsZero() {
		t.Error("expected non-zero handoffTime")
	}
}

func TestClearMarker(t *testing.T) {
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

	// Write marker
	runtimeDir := filepath.Join(cfg.ConfigDir(), RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, HandoffMarkerFile)
	if err := os.WriteFile(markerPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// Clear marker
	if err := ClearMarker(cfg); err != nil {
		t.Errorf("unexpected error clearing marker: %v", err)
	}

	// Verify it's gone
	exists, _, _, _ := CheckMarker(cfg)
	if exists {
		t.Error("expected marker to be cleared")
	}
}

func TestClearMarkerNotExists(t *testing.T) {
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

	// Clear marker that doesn't exist - should not error
	if err := ClearMarker(cfg); err != nil {
		t.Errorf("unexpected error clearing non-existent marker: %v", err)
	}
}

func TestIsInTmux(t *testing.T) {
	// Save original TMUX env var
	origTmux := os.Getenv("TMUX")
	defer os.Setenv("TMUX", origTmux)

	// Test when not in tmux
	os.Unsetenv("TMUX")
	if IsInTmux() {
		t.Error("expected IsInTmux to return false when TMUX is unset")
	}

	// Test when in tmux
	os.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	if !IsInTmux() {
		t.Error("expected IsInTmux to return true when TMUX is set")
	}
}

func TestCollectContextBasic(t *testing.T) {
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

	// Test with message only
	opts := &Options{
		Message:     "Test handoff message",
		AutoCollect: false,
	}

	context, err := collectContext(cfg, opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !strings.Contains(context, "## Handoff Context") {
		t.Error("expected context to contain header")
	}
	if !strings.Contains(context, "Test handoff message") {
		t.Error("expected context to contain message")
	}
	if !strings.Contains(context, "### Notes") {
		t.Error("expected context to contain Notes section")
	}
}

func TestCollectContextNoMessage(t *testing.T) {
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

	// Test without message
	opts := &Options{
		Message:     "",
		AutoCollect: false,
	}

	context, err := collectContext(cfg, opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !strings.Contains(context, "## Handoff Context") {
		t.Error("expected context to contain header")
	}
	if strings.Contains(context, "### Notes") {
		t.Error("expected context to NOT contain Notes section when no message")
	}
}

func TestGetSessionName(t *testing.T) {
	// Save original env vars
	origSession := os.Getenv("CLAUDE_SESSION")
	defer os.Setenv("CLAUDE_SESSION", origSession)

	// Test with CLAUDE_SESSION set
	os.Setenv("CLAUDE_SESSION", "my-test-session")
	name := getSessionName()
	if name != "my-test-session" {
		t.Errorf("expected 'my-test-session', got '%s'", name)
	}

	// Test with CLAUDE_SESSION unset (will fall back to tmux or "unknown")
	os.Unsetenv("CLAUDE_SESSION")
	name = getSessionName()
	// Should either get tmux window name or "unknown"
	if name == "" {
		t.Error("expected non-empty session name")
	}
}

func TestOptionsStruct(t *testing.T) {
	opts := &Options{
		Message:     "test message",
		AutoCollect: true,
		DryRun:      true,
	}

	if opts.Message != "test message" {
		t.Error("Message not set correctly")
	}
	if !opts.AutoCollect {
		t.Error("AutoCollect should be true")
	}
	if !opts.DryRun {
		t.Error("DryRun should be true")
	}
}

func TestResultStruct(t *testing.T) {
	result := &Result{
		MarkerWritten: true,
		BeadUpdated:   true,
		Message:       "test content",
	}

	if !result.MarkerWritten {
		t.Error("MarkerWritten should be true")
	}
	if !result.BeadUpdated {
		t.Error("BeadUpdated should be true")
	}
	if result.Message != "test content" {
		t.Error("Message not set correctly")
	}
}

func TestConstants(t *testing.T) {
	if HandoffMarkerFile != "handoff_marker" {
		t.Errorf("unexpected HandoffMarkerFile: %s", HandoffMarkerFile)
	}
	if HandoffBeadTitle != "Hub Handoff" {
		t.Errorf("unexpected HandoffBeadTitle: %s", HandoffBeadTitle)
	}
	if RuntimeDir != ".wt" {
		t.Errorf("unexpected RuntimeDir: %s", RuntimeDir)
	}
}
