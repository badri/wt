package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDir_CreatesDefaultFiles(t *testing.T) {
	tmpDir := t.TempDir()

	cfg, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}

	// Check default values
	if cfg.WorktreeRoot != "~/worktrees" {
		t.Errorf("expected WorktreeRoot '~/worktrees', got %q", cfg.WorktreeRoot)
	}
	if cfg.EditorCmd != "claude --dangerously-skip-permissions" {
		t.Errorf("expected EditorCmd 'claude --dangerously-skip-permissions', got %q", cfg.EditorCmd)
	}
	if cfg.DefaultMergeMode != "pr-review" {
		t.Errorf("expected DefaultMergeMode 'pr-review', got %q", cfg.DefaultMergeMode)
	}

	// Check that namepool.txt was created
	namepoolPath := filepath.Join(tmpDir, "namepool.txt")
	if _, err := os.Stat(namepoolPath); os.IsNotExist(err) {
		t.Error("namepool.txt was not created")
	}

	// Check that sessions.json was created
	sessionsPath := filepath.Join(tmpDir, "sessions.json")
	if _, err := os.Stat(sessionsPath); os.IsNotExist(err) {
		t.Error("sessions.json was not created")
	}

	// Verify sessions.json content
	data, err := os.ReadFile(sessionsPath)
	if err != nil {
		t.Fatalf("failed to read sessions.json: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("expected sessions.json to be '{}', got %q", string(data))
	}
}

func TestLoadFromDir_LoadsExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create custom config
	customConfig := Config{
		WorktreeRoot:     "/custom/worktrees",
		EditorCmd:        "vim",
		DefaultMergeMode: "direct",
	}
	data, _ := json.Marshal(customConfig)
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	cfg, err := LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed: %v", err)
	}

	if cfg.WorktreeRoot != "/custom/worktrees" {
		t.Errorf("expected WorktreeRoot '/custom/worktrees', got %q", cfg.WorktreeRoot)
	}
	if cfg.EditorCmd != "vim" {
		t.Errorf("expected EditorCmd 'vim', got %q", cfg.EditorCmd)
	}
	if cfg.DefaultMergeMode != "direct" {
		t.Errorf("expected DefaultMergeMode 'direct', got %q", cfg.DefaultMergeMode)
	}
}

func TestConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, _ := LoadFromDir(tmpDir)

	if cfg.ConfigDir() != tmpDir {
		t.Errorf("expected ConfigDir %q, got %q", tmpDir, cfg.ConfigDir())
	}
}

func TestNamepoolPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, _ := LoadFromDir(tmpDir)

	expected := filepath.Join(tmpDir, "namepool.txt")
	if cfg.NamepoolPath() != expected {
		t.Errorf("expected NamepoolPath %q, got %q", expected, cfg.NamepoolPath())
	}
}

func TestSessionsPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, _ := LoadFromDir(tmpDir)

	expected := filepath.Join(tmpDir, "sessions.json")
	if cfg.SessionsPath() != expected {
		t.Errorf("expected SessionsPath %q, got %q", expected, cfg.SessionsPath())
	}
}

func TestWorktreePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config with absolute worktree root
	configData := `{"worktree_root": "/tmp/test-worktrees"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(configData), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _ := LoadFromDir(tmpDir)

	expected := "/tmp/test-worktrees/mysession"
	if cfg.WorktreePath("mysession") != expected {
		t.Errorf("expected WorktreePath %q, got %q", expected, cfg.WorktreePath("mysession"))
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/foo", filepath.Join(home, "foo")},
		{"~/foo/bar", filepath.Join(home, "foo/bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		result := expandPath(tt.input)
		if result != tt.expected {
			t.Errorf("expandPath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestEnsureNamepool_CreatesDefaultNames(t *testing.T) {
	tmpDir := t.TempDir()
	cfg, _ := LoadFromDir(tmpDir)

	data, err := os.ReadFile(cfg.NamepoolPath())
	if err != nil {
		t.Fatalf("failed to read namepool: %v", err)
	}

	content := string(data)
	expectedNames := []string{"toast", "shadow", "obsidian", "quartz", "steel"}
	for _, name := range expectedNames {
		if !contains(content, name) {
			t.Errorf("namepool missing expected name %q", name)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
