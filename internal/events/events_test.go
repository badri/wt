package events

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/badri/wt/internal/config"
)

func setupTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configJSON := `{"worktree_root": "` + filepath.Join(tmpDir, "worktrees") + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	return cfg
}

func TestLogger_LogSessionStart(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	err := logger.LogSessionStart("test-session", "test-bead", "test-project", "/path/to/worktree")
	if err != nil {
		t.Fatalf("LogSessionStart failed: %v", err)
	}

	// Verify event was logged
	events, err := logger.Recent(10)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Type != EventSessionStart {
		t.Errorf("expected type %s, got %s", EventSessionStart, e.Type)
	}
	if e.Session != "test-session" {
		t.Errorf("expected session 'test-session', got %q", e.Session)
	}
	if e.Bead != "test-bead" {
		t.Errorf("expected bead 'test-bead', got %q", e.Bead)
	}
	if e.Project != "test-project" {
		t.Errorf("expected project 'test-project', got %q", e.Project)
	}
	if e.WorktreePath != "/path/to/worktree" {
		t.Errorf("expected worktree '/path/to/worktree', got %q", e.WorktreePath)
	}
	if e.Time == "" {
		t.Error("expected time to be set")
	}
}

func TestLogger_LogSessionEnd(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	err := logger.LogSessionEnd("test-session", "test-bead", "test-project", "claude-123", "direct", "")
	if err != nil {
		t.Fatalf("LogSessionEnd failed: %v", err)
	}

	events, err := logger.Recent(10)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.Type != EventSessionEnd {
		t.Errorf("expected type %s, got %s", EventSessionEnd, e.Type)
	}
	if e.ClaudeSession != "claude-123" {
		t.Errorf("expected claude_session 'claude-123', got %q", e.ClaudeSession)
	}
	if e.MergeMode != "direct" {
		t.Errorf("expected merge_mode 'direct', got %q", e.MergeMode)
	}
}

func TestLogger_LogSessionKill(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	err := logger.LogSessionKill("test-session", "test-bead", "test-project")
	if err != nil {
		t.Fatalf("LogSessionKill failed: %v", err)
	}

	events, err := logger.Recent(10)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Type != EventSessionKill {
		t.Errorf("expected type %s, got %s", EventSessionKill, events[0].Type)
	}
}

func TestLogger_Recent(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	// Log multiple events
	for i := range 5 {
		_ = logger.LogSessionStart("session-"+string(rune('a'+i)), "bead", "proj", "/path")
	}

	// Get recent 3
	events, err := logger.Recent(3)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Should be last 3
	if events[0].Session != "session-c" {
		t.Errorf("expected session-c, got %s", events[0].Session)
	}
	if events[2].Session != "session-e" {
		t.Errorf("expected session-e, got %s", events[2].Session)
	}
}

func TestLogger_RecentSessions(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	// Log various events
	_ = logger.LogSessionStart("session-1", "bead-1", "proj", "/path")
	_ = logger.LogSessionEnd("session-1", "bead-1", "proj", "claude-1", "direct", "")
	_ = logger.LogSessionStart("session-2", "bead-2", "proj", "/path")
	_ = logger.LogSessionKill("session-2", "bead-2", "proj") // no claude session
	_ = logger.LogSessionStart("session-3", "bead-3", "proj", "/path")
	_ = logger.LogSessionEnd("session-3", "bead-3", "proj", "claude-3", "pr-auto", "https://pr")

	// Get recent sessions (only session_end with claude_session)
	sessions, err := logger.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Most recent first
	if sessions[0].Session != "session-3" {
		t.Errorf("expected session-3 first, got %s", sessions[0].Session)
	}
	if sessions[1].Session != "session-1" {
		t.Errorf("expected session-1 second, got %s", sessions[1].Session)
	}
}

func TestLogger_FindSession(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	_ = logger.LogSessionEnd("alpha", "bead-1", "proj", "claude-1", "direct", "")
	_ = logger.LogSessionEnd("beta", "bead-2", "proj", "claude-2", "pr-auto", "https://pr")

	// Find existing session
	event, err := logger.FindSession("alpha")
	if err != nil {
		t.Fatalf("FindSession failed: %v", err)
	}
	if event.ClaudeSession != "claude-1" {
		t.Errorf("expected claude-1, got %s", event.ClaudeSession)
	}

	// Find non-existing session
	_, err = logger.FindSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestLogger_EmptyFile(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	// Recent on empty file should return nil
	events, err := logger.Recent(10)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty events, got %d", len(events))
	}

	sessions, err := logger.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty sessions, got %d", len(sessions))
	}
}

func TestLogger_AppendOnly(t *testing.T) {
	cfg := setupTestConfig(t)
	logger := NewLogger(cfg)

	// Log first event
	_ = logger.LogSessionStart("session-1", "bead", "proj", "/path")

	// Create new logger instance
	logger2 := NewLogger(cfg)

	// Log second event
	_ = logger2.LogSessionStart("session-2", "bead", "proj", "/path")

	// Both events should be present
	events, _ := logger.Recent(10)
	if len(events) != 2 {
		t.Errorf("expected 2 events (append-only), got %d", len(events))
	}
}
