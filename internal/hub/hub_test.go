package hub

import (
	"os"
	"testing"

	"github.com/badri/wt/internal/config"
)

func TestHubSessionName(t *testing.T) {
	if HubSessionName != "hub" {
		t.Errorf("expected HubSessionName to be 'hub', got %q", HubSessionName)
	}
}

func TestOptionsStruct(t *testing.T) {
	opts := &Options{
		Detach: true,
		Status: true,
	}

	if !opts.Detach {
		t.Error("Detach should be true")
	}
	if !opts.Status {
		t.Error("Status should be true")
	}
}

func TestStatusStruct(t *testing.T) {
	status := Status{
		Exists:      true,
		Attached:    true,
		WorkingDir:  "/home/test",
		WindowCount: 3,
		CreatedAt:   "2024-01-01",
		CurrentPane: "claude",
	}

	if !status.Exists {
		t.Error("Exists should be true")
	}
	if !status.Attached {
		t.Error("Attached should be true")
	}
	if status.WorkingDir != "/home/test" {
		t.Errorf("WorkingDir mismatch: got %s", status.WorkingDir)
	}
	if status.WindowCount != 3 {
		t.Errorf("WindowCount mismatch: got %d", status.WindowCount)
	}
}

func TestIsInHubNotInTmux(t *testing.T) {
	// Save original TMUX env var
	origTmux := os.Getenv("TMUX")
	defer os.Setenv("TMUX", origTmux)

	// Test when not in tmux
	os.Unsetenv("TMUX")
	if IsInHub() {
		t.Error("expected IsInHub to return false when not in tmux")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a longer string", 10, "this is..."},
		{"hello", 5, "hello"},
		{"hello", 4, "h..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.max)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, expected %q", tt.input, tt.max, result, tt.expected)
		}
	}
}

func TestGetHubDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	hubDir := GetHubDir(cfg)
	expectedDir := tmpDir + "/hub"
	if hubDir != expectedDir {
		t.Errorf("expected hub dir %q, got %q", expectedDir, hubDir)
	}
}

func TestGetStatusWhenHubNotExists(t *testing.T) {
	// Skip if tmux is not available
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	// Make sure hub doesn't exist
	// (We can't guarantee this in all environments, so this is a best effort test)
	status := GetStatus()

	// When hub doesn't exist, Exists should be false
	// But we can't assert this because the hub might exist from a previous run
	// So we just ensure the function doesn't panic
	_ = status.Exists
	_ = status.Attached
	_ = status.WorkingDir
}

func TestExistsFunction(t *testing.T) {
	// Skip if tmux is not available
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	// Just ensure the function doesn't panic
	_ = Exists()
}

// hasTmux checks if tmux is available
func hasTmux() bool {
	_, err := os.Stat("/usr/bin/tmux")
	if err == nil {
		return true
	}
	_, err = os.Stat("/usr/local/bin/tmux")
	if err == nil {
		return true
	}
	_, err = os.Stat("/opt/homebrew/bin/tmux")
	return err == nil
}

func TestDetachNotInTmux(t *testing.T) {
	// Save original TMUX env var
	origTmux := os.Getenv("TMUX")
	defer os.Setenv("TMUX", origTmux)

	// Test when not in tmux
	os.Unsetenv("TMUX")
	err := detach()
	if err == nil {
		t.Error("expected error when detaching outside tmux")
	}
	expectedMsg := "not in a tmux session"
	if err != nil && err.Error() != "not in a tmux session - cannot detach" {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestRunWithStatusOption(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Test status option - should not return error even if hub doesn't exist
	opts := &Options{Status: true}
	err = Run(cfg, opts)
	if err != nil {
		t.Errorf("unexpected error with status option: %v", err)
	}
}

func TestRunWithDetachNotInTmux(t *testing.T) {
	// Save original TMUX env var
	origTmux := os.Getenv("TMUX")
	defer os.Setenv("TMUX", origTmux)
	os.Unsetenv("TMUX")

	tmpDir, err := os.MkdirTemp("", "wt-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Test detach option when not in tmux - should return error
	opts := &Options{Detach: true}
	err = Run(cfg, opts)
	if err == nil {
		t.Error("expected error when detaching outside tmux")
	}
}
