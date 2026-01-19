package merge

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")

	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// git init
	cmd := exec.Command("git", "init", repoDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "-C", repoDir, "config", "user.email", "test@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config email failed: %v", err)
	}
	cmd = exec.Command("git", "-C", repoDir, "config", "user.name", "Test")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config name failed: %v", err)
	}

	// Create initial commit
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd = exec.Command("git", "-C", repoDir, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	cmd = exec.Command("git", "-C", repoDir, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	return repoDir
}

func TestHasUncommittedChanges_Clean(t *testing.T) {
	repoDir := initTestRepo(t)

	hasChanges, err := HasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}

	if hasChanges {
		t.Error("expected no uncommitted changes in clean repo")
	}
}

func TestHasUncommittedChanges_WithChanges(t *testing.T) {
	repoDir := initTestRepo(t)

	// Create a new file
	newFile := filepath.Join(repoDir, "new.txt")
	if err := os.WriteFile(newFile, []byte("new content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	hasChanges, err := HasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}

	if !hasChanges {
		t.Error("expected uncommitted changes with new file")
	}
}

func TestHasUncommittedChanges_StagedChanges(t *testing.T) {
	repoDir := initTestRepo(t)

	// Create and stage a new file
	newFile := filepath.Join(repoDir, "staged.txt")
	if err := os.WriteFile(newFile, []byte("staged content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repoDir, "add", "staged.txt")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	hasChanges, err := HasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}

	if !hasChanges {
		t.Error("expected uncommitted changes with staged file")
	}
}

func TestHasUncommittedChanges_ModifiedFile(t *testing.T) {
	repoDir := initTestRepo(t)

	// Modify existing file
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Modified\n"), 0644); err != nil {
		t.Fatal(err)
	}

	hasChanges, err := HasUncommittedChanges(repoDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}

	if !hasChanges {
		t.Error("expected uncommitted changes with modified file")
	}
}

func TestGetCurrentBranch_Main(t *testing.T) {
	repoDir := initTestRepo(t)

	branch, err := GetCurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}

	// Default branch could be main or master depending on git version/config
	if branch != "main" && branch != "master" {
		t.Errorf("expected main or master, got %q", branch)
	}
}

func TestGetCurrentBranch_Feature(t *testing.T) {
	repoDir := initTestRepo(t)

	// Create and checkout feature branch
	cmd := exec.Command("git", "-C", repoDir, "checkout", "-b", "feature-test")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git checkout -b failed: %v", err)
	}

	branch, err := GetCurrentBranch(repoDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}

	if branch != "feature-test" {
		t.Errorf("expected feature-test, got %q", branch)
	}
}

func TestGetMainRepoPath(t *testing.T) {
	// This test requires creating a worktree to test the function
	tmpDir := t.TempDir()
	mainRepo := filepath.Join(tmpDir, "main")
	worktreeDir := filepath.Join(tmpDir, "worktree")

	// Create main repo
	if err := os.MkdirAll(mainRepo, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "init", mainRepo)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Configure and create initial commit
	cmd = exec.Command("git", "-C", mainRepo, "config", "user.email", "test@test.com")
	_ = cmd.Run()
	cmd = exec.Command("git", "-C", mainRepo, "config", "user.name", "Test")
	_ = cmd.Run()

	readme := filepath.Join(mainRepo, "README.md")
	if err := os.WriteFile(readme, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "-C", mainRepo, "add", ".")
	_ = cmd.Run()
	cmd = exec.Command("git", "-C", mainRepo, "commit", "-m", "Initial")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Create worktree
	cmd = exec.Command("git", "-C", mainRepo, "worktree", "add", "-b", "feature", worktreeDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git worktree add failed: %v", err)
	}

	// Test getMainRepoPath
	repoPath, err := getMainRepoPath(worktreeDir)
	if err != nil {
		t.Fatalf("getMainRepoPath failed: %v", err)
	}

	// Resolve symlinks (macOS /var -> /private/var)
	expectedPath, _ := filepath.EvalSymlinks(mainRepo)
	actualPath, _ := filepath.EvalSymlinks(repoPath)

	// Should return the main repo path
	if actualPath != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, actualPath)
	}
}

func TestModeConstants(t *testing.T) {
	// Test that merge mode constants are defined correctly
	if ModeDirect != "direct" {
		t.Errorf("expected ModeDirect to be 'direct', got %q", ModeDirect)
	}
	if ModePRAuto != "pr-auto" {
		t.Errorf("expected ModePRAuto to be 'pr-auto', got %q", ModePRAuto)
	}
	if ModePRReview != "pr-review" {
		t.Errorf("expected ModePRReview to be 'pr-review', got %q", ModePRReview)
	}
}
