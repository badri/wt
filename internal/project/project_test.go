package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/badri/wt/internal/config"
)

func setupTestConfig(t *testing.T) (*config.Config, string) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "namepool.txt"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, tmpDir
}

func setupTestRepo(t *testing.T) string {
	repoDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init", repoDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "-C", repoDir, "config", "user.email", "test@test.com")
	cmd.Run()
	cmd = exec.Command("git", "-C", repoDir, "config", "user.name", "Test")
	cmd.Run()

	// Create initial commit
	readme := filepath.Join(repoDir, "README.md")
	os.WriteFile(readme, []byte("# Test\n"), 0644)
	cmd = exec.Command("git", "-C", repoDir, "add", ".")
	cmd.Run()
	cmd = exec.Command("git", "-C", repoDir, "commit", "-m", "Initial commit")
	cmd.Run()

	return repoDir
}

func TestManager_List_Empty(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	projects, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestManager_Add(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	proj, err := mgr.Add("myproject", repoDir)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if proj.Name != "myproject" {
		t.Errorf("expected name 'myproject', got %q", proj.Name)
	}
	if proj.Repo != repoDir {
		t.Errorf("expected repo %q, got %q", repoDir, proj.Repo)
	}
	if proj.BeadsPrefix != "myproject" {
		t.Errorf("expected beads_prefix 'myproject', got %q", proj.BeadsPrefix)
	}
	if proj.MergeMode != "pr-review" {
		t.Errorf("expected merge_mode 'pr-review', got %q", proj.MergeMode)
	}
}

func TestManager_Add_DuplicateError(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	mgr.Add("myproject", repoDir)
	_, err := mgr.Add("myproject", repoDir)

	if err == nil {
		t.Error("expected error for duplicate project")
	}
}

func TestManager_Add_InvalidPath(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	_, err := mgr.Add("myproject", "/nonexistent/path")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestManager_Add_NotGitRepo(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	// Create a directory that's not a git repo
	notGitDir := t.TempDir()

	_, err := mgr.Add("myproject", notGitDir)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestManager_Get(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	mgr.Add("myproject", repoDir)

	proj, err := mgr.Get("myproject")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if proj.Name != "myproject" {
		t.Errorf("expected name 'myproject', got %q", proj.Name)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestManager_List(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir1 := setupTestRepo(t)
	repoDir2 := setupTestRepo(t)
	mgr := NewManager(cfg)

	mgr.Add("project1", repoDir1)
	mgr.Add("project2", repoDir2)

	projects, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestManager_Delete(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	mgr.Add("myproject", repoDir)

	err := mgr.Delete("myproject")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = mgr.Get("myproject")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestManager_Delete_NotFound(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	err := mgr.Delete("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestManager_FindByBeadPrefix(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	mgr.Add("myproject", repoDir)

	// Test with bead ID
	proj, err := mgr.FindByBeadPrefix("myproject-abc")
	if err != nil {
		t.Fatalf("FindByBeadPrefix failed: %v", err)
	}
	if proj.Name != "myproject" {
		t.Errorf("expected project 'myproject', got %q", proj.Name)
	}
}

func TestManager_FindByBeadPrefix_NotFound(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	_, err := mgr.FindByBeadPrefix("unknown-abc")
	if err == nil {
		t.Error("expected error for unknown prefix")
	}
}

func TestProject_RepoPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		repo     string
		expected string
	}{
		{"/absolute/path", "/absolute/path"},
		{"~/relative", filepath.Join(home, "relative")},
	}

	for _, tt := range tests {
		proj := &Project{Repo: tt.repo}
		if proj.RepoPath() != tt.expected {
			t.Errorf("RepoPath(%q) = %q, want %q", tt.repo, proj.RepoPath(), tt.expected)
		}
	}
}

func TestProject_BeadsDir(t *testing.T) {
	proj := &Project{Repo: "/path/to/repo"}
	expected := "/path/to/repo/.beads"
	if proj.BeadsDir() != expected {
		t.Errorf("BeadsDir() = %q, want %q", proj.BeadsDir(), expected)
	}
}

func TestExtractPrefix(t *testing.T) {
	tests := []struct {
		beadID   string
		expected string
	}{
		{"project-abc", "project"},
		{"my-project-xyz", "my-project"},
		{"simple", "simple"},
		{"a-b-c-d", "a-b-c"},
	}

	for _, tt := range tests {
		result := extractPrefix(tt.beadID)
		if result != tt.expected {
			t.Errorf("extractPrefix(%q) = %q, want %q", tt.beadID, result, tt.expected)
		}
	}
}
