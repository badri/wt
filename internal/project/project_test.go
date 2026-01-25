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

	proj, err := mgr.Add("myproject", repoDir, nil)
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

	mgr.Add("myproject", repoDir, nil)
	_, err := mgr.Add("myproject", repoDir, nil)

	if err == nil {
		t.Error("expected error for duplicate project")
	}
}

func TestManager_Add_InvalidPath(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	_, err := mgr.Add("myproject", "/nonexistent/path", nil)
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestManager_Add_NotGitRepo(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	mgr := NewManager(cfg)

	// Create a directory that's not a git repo
	notGitDir := t.TempDir()

	_, err := mgr.Add("myproject", notGitDir, nil)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestManager_Get(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	mgr.Add("myproject", repoDir, nil)

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

	mgr.Add("project1", repoDir1, nil)
	mgr.Add("project2", repoDir2, nil)

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

	mgr.Add("myproject", repoDir, nil)

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

	mgr.Add("myproject", repoDir, nil)

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

func TestManager_Add_WithBranch(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	opts := &AddOptions{Branch: "feature/v2"}
	proj, err := mgr.Add("myproject", repoDir, opts)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if proj.DefaultBranch != "feature/v2" {
		t.Errorf("expected branch 'feature/v2', got %q", proj.DefaultBranch)
	}
}

func TestManager_Add_WithMergeMode(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	// Test with explicit merge mode
	opts := &AddOptions{Branch: "main", MergeMode: "direct"}
	proj, err := mgr.Add("myproject", repoDir, opts)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if proj.MergeMode != "direct" {
		t.Errorf("expected merge mode 'direct', got %q", proj.MergeMode)
	}
}

func TestManager_Add_DefaultMergeMode(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)
	mgr := NewManager(cfg)

	// Test with no merge mode specified (should default to pr-review)
	proj, err := mgr.Add("myproject", repoDir, nil)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if proj.MergeMode != "pr-review" {
		t.Errorf("expected merge mode 'pr-review', got %q", proj.MergeMode)
	}
}

func TestManager_Add_MultipleBranches(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)

	// Add remote URL to test repo
	cmd := exec.Command("git", "-C", repoDir, "remote", "add", "origin", "git@github.com:test/repo.git")
	cmd.Run()

	mgr := NewManager(cfg)

	// First registration with main branch
	proj1, err := mgr.Add("myproject", repoDir, &AddOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("Add main branch failed: %v", err)
	}
	if proj1.RepoURL != "git@github.com:test/repo.git" {
		t.Errorf("expected RepoURL to be set, got %q", proj1.RepoURL)
	}

	// Second registration with feature branch (same repo, different branch)
	proj2, err := mgr.Add("myproject-feature", repoDir, &AddOptions{Branch: "feature/v2"})
	if err != nil {
		t.Fatalf("Add feature branch failed: %v", err)
	}
	if proj2.DefaultBranch != "feature/v2" {
		t.Errorf("expected branch 'feature/v2', got %q", proj2.DefaultBranch)
	}

	// Both should share the same beads prefix
	if proj1.BeadsPrefix != proj2.BeadsPrefix {
		t.Errorf("expected same beads prefix, got %q and %q", proj1.BeadsPrefix, proj2.BeadsPrefix)
	}
}

func TestManager_Add_SameRepoBranchConflict(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)

	// Add remote URL to test repo
	cmd := exec.Command("git", "-C", repoDir, "remote", "add", "origin", "git@github.com:test/repo.git")
	cmd.Run()

	mgr := NewManager(cfg)

	// First registration
	_, err := mgr.Add("myproject", repoDir, &AddOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("Add first project failed: %v", err)
	}

	// Second registration with same branch should fail
	_, err = mgr.Add("myproject2", repoDir, &AddOptions{Branch: "main"})
	if err == nil {
		t.Error("expected error when registering same repo with same branch")
	}
}

func TestManager_FindByRepoURL(t *testing.T) {
	cfg, _ := setupTestConfig(t)
	repoDir := setupTestRepo(t)

	// Add remote URL to test repo
	cmd := exec.Command("git", "-C", repoDir, "remote", "add", "origin", "git@github.com:test/repo.git")
	cmd.Run()

	mgr := NewManager(cfg)

	// Register two projects for same repo
	mgr.Add("proj-main", repoDir, &AddOptions{Branch: "main"})
	mgr.Add("proj-feature", repoDir, &AddOptions{Branch: "feature"})

	// Find all projects for this repo
	matches, err := mgr.FindByRepoURL("git@github.com:test/repo.git")
	if err != nil {
		t.Fatalf("FindByRepoURL failed: %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}
}
