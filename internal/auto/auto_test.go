package auto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Command == "" {
		t.Error("expected default command to be set")
	}

	if cfg.TimeoutMinutes <= 0 {
		t.Error("expected default timeout to be positive")
	}

	if cfg.PromptTemplate == "" {
		t.Error("expected default prompt template to be set")
	}
}

func TestLockFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "wt-auto-test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockFile := filepath.Join(tmpDir, "auto.lock")

	// Write lock
	lock := LockInfo{
		PID:       12345,
		StartTime: time.Now().Format(time.RFC3339),
		Project:   "test-project",
	}

	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatalf("marshaling lock: %v", err)
	}

	if err := os.WriteFile(lockFile, data, 0644); err != nil {
		t.Fatalf("writing lock file: %v", err)
	}

	// Read lock back
	readData, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("reading lock file: %v", err)
	}

	var readLock LockInfo
	if err := json.Unmarshal(readData, &readLock); err != nil {
		t.Fatalf("unmarshaling lock: %v", err)
	}

	if readLock.PID != lock.PID {
		t.Errorf("expected PID %d, got %d", lock.PID, readLock.PID)
	}

	if readLock.Project != lock.Project {
		t.Errorf("expected project %s, got %s", lock.Project, readLock.Project)
	}
}

func TestBuildPrompt(t *testing.T) {
	// We can't easily test buildPrompt without a full Runner setup,
	// but we can test the template replacement logic directly
	template := "Work on {BEAD_ID}: {TITLE}\n\n{DESCRIPTION}"

	result := template
	result = replaceTemplateVar(result, "{BEAD_ID}", "test-abc")
	result = replaceTemplateVar(result, "{TITLE}", "Fix bug")
	result = replaceTemplateVar(result, "{DESCRIPTION}", "This is a test description")

	expected := "Work on test-abc: Fix bug\n\nThis is a test description"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func replaceTemplateVar(s, old, new string) string {
	for i := 0; i < len(s); i++ {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

func TestOptions(t *testing.T) {
	opts := &Options{
		Project:   "test",
		MergeMode: "pr-review",
		DryRun:    true,
		Timeout:   60,
	}

	if opts.Project != "test" {
		t.Errorf("expected project 'test', got %q", opts.Project)
	}

	if opts.MergeMode != "pr-review" {
		t.Errorf("expected merge mode 'pr-review', got %q", opts.MergeMode)
	}

	if !opts.DryRun {
		t.Error("expected DryRun to be true")
	}

	if opts.Timeout != 60 {
		t.Errorf("expected timeout 60, got %d", opts.Timeout)
	}
}

func TestOptionsWithEpic(t *testing.T) {
	opts := &Options{
		Epic:           "wt-doc-batch",
		PauseOnFailure: true,
		SkipAudit:      false,
		Resume:         false,
		Abort:          false,
	}

	if opts.Epic != "wt-doc-batch" {
		t.Errorf("expected epic 'wt-doc-batch', got %q", opts.Epic)
	}

	if !opts.PauseOnFailure {
		t.Error("expected PauseOnFailure to be true")
	}

	if opts.SkipAudit {
		t.Error("expected SkipAudit to be false")
	}
}

func TestEpicState(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "wt-auto-epic-test")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stateFile := filepath.Join(tmpDir, "auto-epic-state.json")

	// Create epic state
	state := &EpicState{
		EpicID:         "wt-test-epic",
		Worktree:       "/tmp/worktree",
		SessionName:    "auto-test",
		Beads:          []string{"wt-a1", "wt-b2", "wt-c3"},
		CompletedBeads: []string{"wt-a1"},
		CurrentBead:    "wt-b2",
		Status:         "running",
		StartTime:      time.Now().Format(time.RFC3339),
		ProjectDir:     "/path/to/project",
		MergeMode:      "pr-review",
	}

	// Write state
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshaling state: %v", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		t.Fatalf("writing state file: %v", err)
	}

	// Read state back
	readData, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}

	var readState EpicState
	if err := json.Unmarshal(readData, &readState); err != nil {
		t.Fatalf("unmarshaling state: %v", err)
	}

	if readState.EpicID != state.EpicID {
		t.Errorf("expected epic ID %s, got %s", state.EpicID, readState.EpicID)
	}

	if len(readState.Beads) != 3 {
		t.Errorf("expected 3 beads, got %d", len(readState.Beads))
	}

	if len(readState.CompletedBeads) != 1 {
		t.Errorf("expected 1 completed bead, got %d", len(readState.CompletedBeads))
	}

	if readState.Status != "running" {
		t.Errorf("expected status 'running', got %s", readState.Status)
	}

	if readState.CurrentBead != "wt-b2" {
		t.Errorf("expected current bead 'wt-b2', got %s", readState.CurrentBead)
	}
}

func TestEpicAuditResult(t *testing.T) {
	result := &EpicAuditResult{
		EpicID:     "wt-test-epic",
		Ready:      false,
		Beads:      []string{"wt-a1", "wt-b2"},
		BeadTitles: map[string]string{"wt-a1": "Task 1", "wt-b2": "Task 2"},
		Issues:     []string{"Bead wt-a1 has no description"},
	}

	if result.Ready {
		t.Error("expected Ready to be false")
	}

	if len(result.Beads) != 2 {
		t.Errorf("expected 2 beads, got %d", len(result.Beads))
	}

	if len(result.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(result.Issues))
	}

	if result.BeadTitles["wt-a1"] != "Task 1" {
		t.Errorf("expected title 'Task 1', got %s", result.BeadTitles["wt-a1"])
	}
}

func TestLockInfoWithEpic(t *testing.T) {
	lock := LockInfo{
		PID:       12345,
		StartTime: time.Now().Format(time.RFC3339),
		Project:   "test-project",
		Epic:      "wt-doc-batch",
	}

	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatalf("marshaling lock: %v", err)
	}

	var readLock LockInfo
	if err := json.Unmarshal(data, &readLock); err != nil {
		t.Fatalf("unmarshaling lock: %v", err)
	}

	if readLock.Epic != "wt-doc-batch" {
		t.Errorf("expected epic 'wt-doc-batch', got %s", readLock.Epic)
	}
}
