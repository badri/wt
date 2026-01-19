//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/namepool"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/tmux"
	"github.com/badri/wt/internal/worktree"
)

// TestIntegration_FullWorkflow tests the complete wt workflow.
// Run with: go test -tags=integration ./test/integration/...
func TestIntegration_FullWorkflow(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available, skipping integration test")
	}

	// Create temporary directories
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize a git repository
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Load namepool
	pool, err := namepool.Load(cfg)
	if err != nil {
		t.Fatalf("failed to load namepool: %v", err)
	}

	// Load session state
	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Allocate a name
	sessionName, err := pool.Allocate(state.UsedNames())
	if err != nil {
		t.Fatalf("failed to allocate name: %v", err)
	}

	// Create worktree
	worktreePath := cfg.WorktreePath(sessionName)
	branchName := "test-branch"

	t.Logf("Creating worktree at %s", worktreePath)
	if err := worktree.Create(repoDir, worktreePath, branchName); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Verify worktree exists
	if !worktree.Exists(worktreePath) {
		t.Error("worktree should exist after creation")
	}

	// Create tmux session
	t.Logf("Creating tmux session %s", sessionName)
	if err := tmux.NewSession(sessionName, worktreePath, repoDir+"/.beads", ""); err != nil {
		t.Fatalf("failed to create tmux session: %v", err)
	}

	// Verify tmux session exists
	if !tmux.SessionExists(sessionName) {
		t.Error("tmux session should exist after creation")
	}

	// Save session state
	state.Sessions[sessionName] = &session.Session{
		Bead:     "test-bead",
		Project:  "test",
		Worktree: worktreePath,
		Branch:   branchName,
		Status:   "working",
	}
	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Cleanup: kill tmux session
	t.Log("Cleaning up: killing tmux session")
	if err := tmux.Kill(sessionName); err != nil {
		t.Logf("Warning: failed to kill tmux session: %v", err)
	}

	// Cleanup: remove worktree
	t.Log("Cleaning up: removing worktree")
	if err := worktree.Remove(worktreePath); err != nil {
		t.Logf("Warning: failed to remove worktree: %v", err)
	}

	t.Log("Integration test completed successfully")
}

// TestIntegration_Worktree tests git worktree operations without tmux.
func TestIntegration_Worktree(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	worktreeDir := filepath.Join(tmpDir, "worktree")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create worktree with new branch
	if err := worktree.Create(repoDir, worktreeDir, "feature-branch"); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Verify worktree exists
	if !worktree.Exists(worktreeDir) {
		t.Error("worktree should exist")
	}

	// Verify it's a git directory
	gitDir := filepath.Join(worktreeDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		t.Errorf("worktree should have .git: %v", err)
	}

	// Verify branch
	cmd := exec.Command("git", "-C", worktreeDir, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get branch: %v", err)
	}
	branch := string(output)
	if branch != "feature-branch\n" {
		t.Errorf("expected branch 'feature-branch', got %q", branch)
	}

	// Remove worktree
	if err := worktree.Remove(worktreeDir); err != nil {
		t.Fatalf("failed to remove worktree: %v", err)
	}

	// Verify worktree removed
	if worktree.Exists(worktreeDir) {
		t.Error("worktree should not exist after removal")
	}
}

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func initGitRepo(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	// git init
	cmd := exec.Command("git", "init", path)
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configure git user for commits
	cmd = exec.Command("git", "-C", path, "config", "user.email", "test@test.com")
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("git", "-C", path, "config", "user.name", "Test")
	if err := cmd.Run(); err != nil {
		return err
	}

	// Create initial commit
	dummyFile := filepath.Join(path, "README.md")
	if err := os.WriteFile(dummyFile, []byte("# Test\n"), 0644); err != nil {
		return err
	}

	cmd = exec.Command("git", "-C", path, "add", ".")
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "-C", path, "commit", "-m", "Initial commit")
	return cmd.Run()
}
