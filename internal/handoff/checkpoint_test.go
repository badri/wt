package handoff

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/badri/wt/internal/config"
)

func TestCheckpointStruct(t *testing.T) {
	cp := &Checkpoint{
		CreatedAt:    "2024-01-15T10:30:00Z",
		Session:      "toast",
		Bead:         "myproject-abc",
		Project:      "myproject",
		Worktree:     "/path/to/worktree",
		GitBranch:    "myproject-abc",
		GitDiffStat:  "2 files changed, 10 insertions(+)",
		GitStatus:    "M file.go",
		BeadTitle:    "Implement feature X",
		BeadDesc:     "Description of feature X",
		BeadPriority: 2,
		BeadStatus:   "in_progress",
		Notes:        "Working on implementation",
		Trigger:      "manual",
	}

	if cp.Session != "toast" {
		t.Errorf("expected Session 'toast', got '%s'", cp.Session)
	}
	if cp.Bead != "myproject-abc" {
		t.Errorf("expected Bead 'myproject-abc', got '%s'", cp.Bead)
	}
	if cp.BeadPriority != 2 {
		t.Errorf("expected BeadPriority 2, got %d", cp.BeadPriority)
	}
}

func TestCheckpointOptions(t *testing.T) {
	opts := &CheckpointOptions{
		Notes:   "test notes",
		Trigger: "auto",
		Quiet:   true,
		Clear:   false,
	}

	if opts.Notes != "test notes" {
		t.Errorf("expected Notes 'test notes', got '%s'", opts.Notes)
	}
	if opts.Trigger != "auto" {
		t.Errorf("expected Trigger 'auto', got '%s'", opts.Trigger)
	}
	if !opts.Quiet {
		t.Error("expected Quiet to be true")
	}
	if opts.Clear {
		t.Error("expected Clear to be false")
	}
}

func TestCheckpointSaveAndLoad(t *testing.T) {
	// Create temp directory as fake worktree
	tmpDir, err := os.MkdirTemp("", "wt-checkpoint-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo so git commands work
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	// Run git init
	runGitInit(t, tmpDir)

	// Create config
	configDir := filepath.Join(tmpDir, ".config", "wt")
	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	opts := &CheckpointOptions{
		Notes:   "test checkpoint",
		Trigger: "manual",
		Quiet:   true,
	}

	// Save checkpoint
	cp, err := SaveCheckpoint(cfg, opts)
	if err != nil {
		t.Fatalf("failed to save checkpoint: %v", err)
	}

	// On macOS, /var/folders may be symlinked to /private/var/folders
	// Resolve both paths to compare
	resolvedTmpDir, _ := filepath.EvalSymlinks(tmpDir)
	resolvedWorktree, _ := filepath.EvalSymlinks(cp.Worktree)
	if resolvedWorktree != resolvedTmpDir {
		t.Errorf("expected Worktree '%s', got '%s'", resolvedTmpDir, resolvedWorktree)
	}
	if cp.Notes != "test checkpoint" {
		t.Errorf("expected Notes 'test checkpoint', got '%s'", cp.Notes)
	}
	if cp.Trigger != "manual" {
		t.Errorf("expected Trigger 'manual', got '%s'", cp.Trigger)
	}

	// Verify checkpoint exists
	if !CheckpointExists() {
		t.Error("expected checkpoint to exist")
	}

	// Load checkpoint
	loaded, err := LoadCheckpoint()
	if err != nil {
		t.Fatalf("failed to load checkpoint: %v", err)
	}

	if loaded == nil {
		t.Fatal("expected non-nil checkpoint")
	}
	if loaded.Notes != cp.Notes {
		t.Errorf("loaded Notes mismatch: expected '%s', got '%s'", cp.Notes, loaded.Notes)
	}

	// Clear checkpoint
	if err := ClearCheckpoint(); err != nil {
		t.Errorf("failed to clear checkpoint: %v", err)
	}

	// Verify checkpoint is gone
	if CheckpointExists() {
		t.Error("expected checkpoint to be cleared")
	}

	// Loading should return nil (not error) when no checkpoint
	loaded, err = LoadCheckpoint()
	if err != nil {
		t.Errorf("unexpected error loading non-existent checkpoint: %v", err)
	}
	if loaded != nil {
		t.Error("expected nil checkpoint when not exists")
	}
}

func TestCheckpointPath(t *testing.T) {
	path := getCheckpointPath("/path/to/worktree")
	expected := "/path/to/worktree/.wt/checkpoint.json"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func TestCheckpointExistsIn(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wt-checkpoint-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Should not exist initially
	if CheckpointExistsIn(tmpDir) {
		t.Error("expected checkpoint to not exist initially")
	}

	// Create checkpoint directory and file
	cpDir := filepath.Join(tmpDir, WorktreeCheckpointDir)
	if err := os.MkdirAll(cpDir, 0755); err != nil {
		t.Fatalf("failed to create checkpoint dir: %v", err)
	}

	cpPath := filepath.Join(cpDir, CheckpointFile)
	if err := os.WriteFile(cpPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write checkpoint file: %v", err)
	}

	// Should exist now
	if !CheckpointExistsIn(tmpDir) {
		t.Error("expected checkpoint to exist")
	}
}

func TestFormatCheckpointForRecovery(t *testing.T) {
	cp := &Checkpoint{
		CreatedAt:    "2024-01-15T10:30:00Z",
		Session:      "toast",
		Bead:         "myproject-abc",
		Project:      "myproject",
		GitBranch:    "myproject-abc",
		GitDiffStat:  "2 files changed",
		GitStatus:    "M file.go",
		BeadTitle:    "Test feature",
		BeadDesc:     "Description",
		BeadPriority: 2,
		BeadStatus:   "in_progress",
		Notes:        "Test notes",
		Trigger:      "manual",
	}

	output := FormatCheckpointForRecovery(cp)

	// Check that output contains expected sections
	if !strings.Contains(output, "Context Recovery") {
		t.Error("expected output to contain 'Context Recovery'")
	}
	if !strings.Contains(output, "Current Task") {
		t.Error("expected output to contain 'Current Task'")
	}
	if !strings.Contains(output, "myproject-abc") {
		t.Error("expected output to contain bead ID")
	}
	if !strings.Contains(output, "Git State") {
		t.Error("expected output to contain 'Git State'")
	}
	if !strings.Contains(output, "Test notes") {
		t.Error("expected output to contain notes")
	}
}

func TestCheckpointJSONSerialization(t *testing.T) {
	cp := &Checkpoint{
		CreatedAt:    "2024-01-15T10:30:00Z",
		Session:      "toast",
		Bead:         "test-123",
		Project:      "test",
		Worktree:     "/path/to/worktree",
		GitBranch:    "test-123",
		BeadPriority: 1,
		Trigger:      "manual",
	}

	// Marshal to JSON
	data, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("failed to marshal checkpoint: %v", err)
	}

	// Unmarshal back
	var loaded Checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal checkpoint: %v", err)
	}

	if loaded.Session != cp.Session {
		t.Errorf("Session mismatch: expected '%s', got '%s'", cp.Session, loaded.Session)
	}
	if loaded.Bead != cp.Bead {
		t.Errorf("Bead mismatch: expected '%s', got '%s'", cp.Bead, loaded.Bead)
	}
	if loaded.BeadPriority != cp.BeadPriority {
		t.Errorf("BeadPriority mismatch: expected %d, got %d", cp.BeadPriority, loaded.BeadPriority)
	}
}

func TestClearCheckpointNonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "wt-checkpoint-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	// Clearing non-existent checkpoint should not error
	if err := ClearCheckpoint(); err != nil {
		t.Errorf("unexpected error clearing non-existent checkpoint: %v", err)
	}
}

// Helper to init a git repo for testing
func runGitInit(t *testing.T, dir string) {
	t.Helper()

	// Create a minimal git repo
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	// Create HEAD file
	headFile := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(headFile, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatalf("failed to create HEAD: %v", err)
	}

	// Create refs/heads directory
	refsDir := filepath.Join(gitDir, "refs", "heads")
	if err := os.MkdirAll(refsDir, 0755); err != nil {
		t.Fatalf("failed to create refs dir: %v", err)
	}
}
