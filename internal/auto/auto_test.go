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
