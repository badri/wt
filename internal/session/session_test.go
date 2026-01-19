package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/badri/wt/internal/config"
)

func TestLoadState_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty sessions.json
	if err := os.WriteFile(filepath.Join(tmpDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "namepool.txt"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.LoadFromDir(tmpDir)
	state, err := LoadState(cfg)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if len(state.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(state.Sessions))
	}
}

func TestLoadState_WithSessions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sessions.json with data
	sessions := map[string]*Session{
		"toast": {
			Bead:     "proj-abc",
			Project:  "proj",
			Worktree: "/tmp/worktrees/toast",
			Branch:   "proj-abc",
			Status:   "working",
		},
	}
	data, _ := json.Marshal(sessions)
	if err := os.WriteFile(filepath.Join(tmpDir, "sessions.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "namepool.txt"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.LoadFromDir(tmpDir)
	state, err := LoadState(cfg)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if len(state.Sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(state.Sessions))
	}

	sess, exists := state.Sessions["toast"]
	if !exists {
		t.Fatal("session 'toast' not found")
	}
	if sess.Bead != "proj-abc" {
		t.Errorf("expected bead 'proj-abc', got %q", sess.Bead)
	}
}

func TestSave(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial empty sessions.json
	sessionsPath := filepath.Join(tmpDir, "sessions.json")
	if err := os.WriteFile(sessionsPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "namepool.txt"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.LoadFromDir(tmpDir)
	state, _ := LoadState(cfg)

	// Add a session
	state.Sessions["shadow"] = &Session{
		Bead:    "proj-xyz",
		Project: "proj",
		Status:  "idle",
	}

	if err := state.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload and verify
	state2, _ := LoadState(cfg)
	if len(state2.Sessions) != 1 {
		t.Errorf("expected 1 session after save, got %d", len(state2.Sessions))
	}

	sess, exists := state2.Sessions["shadow"]
	if !exists {
		t.Fatal("session 'shadow' not found after reload")
	}
	if sess.Bead != "proj-xyz" {
		t.Errorf("expected bead 'proj-xyz', got %q", sess.Bead)
	}
}

func TestUsedNames(t *testing.T) {
	state := &State{
		Sessions: map[string]*Session{
			"toast":  {},
			"shadow": {},
		},
	}

	names := state.UsedNames()
	if len(names) != 2 {
		t.Errorf("expected 2 used names, got %d", len(names))
	}

	// Check both names are present (order not guaranteed)
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}
	if !found["toast"] || !found["shadow"] {
		t.Errorf("expected toast and shadow in used names, got %v", names)
	}
}

func TestFindByBead(t *testing.T) {
	state := &State{
		Sessions: map[string]*Session{
			"toast":  {Bead: "proj-abc"},
			"shadow": {Bead: "proj-xyz"},
		},
	}

	name, sess := state.FindByBead("proj-xyz")
	if name != "shadow" {
		t.Errorf("expected name 'shadow', got %q", name)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.Bead != "proj-xyz" {
		t.Errorf("expected bead 'proj-xyz', got %q", sess.Bead)
	}
}

func TestFindByBead_NotFound(t *testing.T) {
	state := &State{
		Sessions: map[string]*Session{
			"toast": {Bead: "proj-abc"},
		},
	}

	name, sess := state.FindByBead("nonexistent")
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
	if sess != nil {
		t.Error("expected nil session, got non-nil")
	}
}

func TestUpdateActivity(t *testing.T) {
	sess := &Session{}

	before := time.Now().UTC().Truncate(time.Second)
	sess.UpdateActivity()
	after := time.Now().UTC().Add(time.Second).Truncate(time.Second)

	// Parse the last activity time
	activityTime, err := time.Parse(time.RFC3339, sess.LastActivity)
	if err != nil {
		t.Fatalf("failed to parse LastActivity: %v", err)
	}

	if activityTime.Before(before) || activityTime.After(after) {
		t.Errorf("LastActivity %v not between %v and %v", activityTime, before, after)
	}
}

func TestNow(t *testing.T) {
	before := time.Now().UTC().Truncate(time.Second)
	nowStr := Now()
	after := time.Now().UTC().Add(time.Second).Truncate(time.Second)

	nowTime, err := time.Parse(time.RFC3339, nowStr)
	if err != nil {
		t.Fatalf("Now() returned invalid RFC3339: %v", err)
	}

	if nowTime.Before(before) || nowTime.After(after) {
		t.Errorf("Now() %v not between %v and %v", nowTime, before, after)
	}
}
