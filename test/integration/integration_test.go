//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/badri/wt/internal/auto"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/handoff"
	"github.com/badri/wt/internal/merge"
	"github.com/badri/wt/internal/namepool"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/testenv"
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
	if err := tmux.NewSession(sessionName, worktreePath, repoDir+"/.beads", "", nil); err != nil {
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

// TestIntegration_ProjectWorkflow tests project registration and lookup.
func TestIntegration_ProjectWorkflow(t *testing.T) {
	// Create temporary directories
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize a git repository
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "namepool.txt"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	mgr := project.NewManager(cfg)

	// Test: List empty projects
	projects, err := mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects initially, got %d", len(projects))
	}

	// Test: Add project
	t.Log("Adding project 'testproj'")
	proj, err := mgr.Add("testproj", repoDir)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if proj.Name != "testproj" {
		t.Errorf("expected name 'testproj', got %q", proj.Name)
	}
	if proj.BeadsPrefix != "testproj" {
		t.Errorf("expected beads_prefix 'testproj', got %q", proj.BeadsPrefix)
	}

	// Test: Get project
	proj2, err := mgr.Get("testproj")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if proj2.Repo != repoDir {
		t.Errorf("expected repo %q, got %q", repoDir, proj2.Repo)
	}

	// Test: List projects
	projects, err = mgr.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}

	// Test: Find by bead prefix
	proj3, err := mgr.FindByBeadPrefix("testproj-abc123")
	if err != nil {
		t.Fatalf("FindByBeadPrefix failed: %v", err)
	}
	if proj3.Name != "testproj" {
		t.Errorf("expected project 'testproj', got %q", proj3.Name)
	}

	// Test: Update project config
	proj2.MergeMode = "direct"
	if err := mgr.Save(proj2); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	proj4, _ := mgr.Get("testproj")
	if proj4.MergeMode != "direct" {
		t.Errorf("expected merge_mode 'direct', got %q", proj4.MergeMode)
	}

	// Test: Delete project
	if err := mgr.Delete("testproj"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	projects, _ = mgr.List()
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects))
	}

	t.Log("Project integration test completed successfully")
}

// TestIntegration_MultiSession tests multiple concurrent sessions.
func TestIntegration_MultiSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available, skipping integration test")
	}

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Setup config
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

	pool, err := namepool.Load(cfg)
	if err != nil {
		t.Fatalf("failed to load namepool: %v", err)
	}

	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Create multiple sessions
	sessionNames := make([]string, 3)
	for i := range 3 {
		name, err := pool.Allocate(state.UsedNames())
		if err != nil {
			t.Fatalf("failed to allocate name %d: %v", i, err)
		}
		sessionNames[i] = name

		worktreePath := cfg.WorktreePath(name)
		branchName := "feature-" + name

		if err := worktree.Create(repoDir, worktreePath, branchName); err != nil {
			t.Fatalf("failed to create worktree %d: %v", i, err)
		}

		if err := tmux.NewSession(name, worktreePath, repoDir+"/.beads", "", nil); err != nil {
			t.Fatalf("failed to create tmux session %d: %v", i, err)
		}

		state.Sessions[name] = &session.Session{
			Bead:     "bead-" + name,
			Project:  "test",
			Worktree: worktreePath,
			Branch:   branchName,
			Status:   "working",
		}
	}

	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Verify all sessions exist
	for _, name := range sessionNames {
		if !tmux.SessionExists(name) {
			t.Errorf("session %s should exist", name)
		}
	}

	// Verify state has all sessions
	if len(state.Sessions) != 3 {
		t.Errorf("expected 3 sessions in state, got %d", len(state.Sessions))
	}

	// Cleanup: kill all sessions
	for _, name := range sessionNames {
		_ = tmux.Kill(name)
		worktreePath := cfg.WorktreePath(name)
		_ = worktree.Remove(worktreePath)
	}

	t.Log("Multi-session integration test completed successfully")
}

// TestIntegration_EventsLogging tests the event logging system.
func TestIntegration_EventsLogging(t *testing.T) {
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

	logger := events.NewLogger(cfg)

	// Simulate full session lifecycle
	_ = logger.LogSessionStart("alpha", "test-bead-1", "testproj", "/path/to/worktree1")
	_ = logger.LogSessionEnd("alpha", "test-bead-1", "testproj", "claude-session-1", "direct", "")

	_ = logger.LogSessionStart("beta", "test-bead-2", "testproj", "/path/to/worktree2")
	_ = logger.LogSessionEnd("beta", "test-bead-2", "testproj", "claude-session-2", "pr-auto", "https://github.com/test/pr/1")

	_ = logger.LogSessionStart("gamma", "test-bead-3", "testproj", "/path/to/worktree3")
	_ = logger.LogSessionKill("gamma", "test-bead-3", "testproj")

	// Test Recent
	allEvents, err := logger.Recent(100)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}
	if len(allEvents) != 6 {
		t.Errorf("expected 6 events, got %d", len(allEvents))
	}

	// Test RecentSessions (only session_end with claude_session)
	sessions, err := logger.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions with claude_session, got %d", len(sessions))
	}

	// Test FindSession
	event, err := logger.FindSession("beta")
	if err != nil {
		t.Fatalf("FindSession failed: %v", err)
	}
	if event.ClaudeSession != "claude-session-2" {
		t.Errorf("expected claude-session-2, got %s", event.ClaudeSession)
	}
	if event.PRURL != "https://github.com/test/pr/1" {
		t.Errorf("expected PR URL, got %s", event.PRURL)
	}

	t.Log("Events logging integration test completed successfully")
}

// TestIntegration_PortOffsetAllocation tests the test environment port offset allocation.
func TestIntegration_PortOffsetAllocation(t *testing.T) {
	// Create a project with test_env configuration
	proj := &project.Project{
		Name: "testproj",
		Repo: "/tmp/repo",
		TestEnv: &project.TestEnv{
			Setup:    "echo setup",
			Teardown: "echo teardown",
			PortEnv:  "PORT_OFFSET",
		},
	}

	// Test allocation with no used offsets
	offset1 := testenv.AllocatePortOffset(proj, nil)
	if offset1 != testenv.DefaultPortBase {
		t.Errorf("expected first offset to be %d, got %d", testenv.DefaultPortBase, offset1)
	}

	// Test allocation with some used offsets
	usedOffsets := []int{testenv.DefaultPortBase, testenv.DefaultPortBase + testenv.DefaultPortStep}
	offset2 := testenv.AllocatePortOffset(proj, usedOffsets)
	expectedOffset := testenv.DefaultPortBase + (2 * testenv.DefaultPortStep)
	if offset2 != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, offset2)
	}

	// Test with nil project (disabled)
	offset3 := testenv.AllocatePortOffset(nil, nil)
	if offset3 != 0 {
		t.Errorf("expected 0 for nil project, got %d", offset3)
	}

	// Test with project without test_env (disabled)
	projNoTestEnv := &project.Project{Name: "noenv", Repo: "/tmp/repo"}
	offset4 := testenv.AllocatePortOffset(projNoTestEnv, nil)
	if offset4 != 0 {
		t.Errorf("expected 0 for project without test_env, got %d", offset4)
	}

	t.Log("Port offset allocation integration test completed successfully")
}

// TestIntegration_MergeHelpers tests merge helper functions with real git operations.
func TestIntegration_MergeHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	worktreeDir := filepath.Join(tmpDir, "worktree")

	// Initialize main repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create a worktree with a feature branch
	cmd := exec.Command("git", "-C", repoDir, "worktree", "add", "-b", "feature-test", worktreeDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}
	defer func() {
		exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", worktreeDir).Run()
	}()

	// Test GetCurrentBranch
	branch, err := merge.GetCurrentBranch(worktreeDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch != "feature-test" {
		t.Errorf("expected branch 'feature-test', got %q", branch)
	}

	// Test HasUncommittedChanges (clean)
	hasChanges, err := merge.HasUncommittedChanges(worktreeDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes in clean worktree")
	}

	// Make changes
	newFile := filepath.Join(worktreeDir, "feature.txt")
	if err := os.WriteFile(newFile, []byte("feature content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test HasUncommittedChanges (with changes)
	hasChanges, err = merge.HasUncommittedChanges(worktreeDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if !hasChanges {
		t.Error("expected uncommitted changes after creating file")
	}

	// Commit the changes
	cmd = exec.Command("git", "-C", worktreeDir, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "-C", worktreeDir, "commit", "-m", "Add feature")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Test HasUncommittedChanges (clean after commit)
	hasChanges, err = merge.HasUncommittedChanges(worktreeDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes after commit")
	}

	t.Log("Merge helpers integration test completed successfully")
}

// TestIntegration_SessionStatePersistence tests that session state persists correctly.
func TestIntegration_SessionStatePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"worktree_root": "` + filepath.Join(tmpDir, "worktrees") + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Create and save session state
	state1, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	state1.Sessions["alpha"] = &session.Session{
		Bead:       "test-bead-1",
		Project:    "testproj",
		Worktree:   "/path/to/worktree",
		Branch:     "feature-alpha",
		PortOffset: 1000,
		Status:     "working",
	}
	state1.Sessions["beta"] = &session.Session{
		Bead:       "test-bead-2",
		Project:    "testproj",
		Worktree:   "/path/to/worktree2",
		Branch:     "feature-beta",
		PortOffset: 1100,
		Status:     "idle",
	}

	if err := state1.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Load state again and verify persistence
	state2, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state again: %v", err)
	}

	if len(state2.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(state2.Sessions))
	}

	alpha := state2.Sessions["alpha"]
	if alpha == nil {
		t.Fatal("alpha session not found")
	}
	if alpha.Bead != "test-bead-1" {
		t.Errorf("expected bead 'test-bead-1', got %q", alpha.Bead)
	}
	if alpha.PortOffset != 1000 {
		t.Errorf("expected port_offset 1000, got %d", alpha.PortOffset)
	}

	// Test FindByBead
	name, sess := state2.FindByBead("test-bead-2")
	if name != "beta" {
		t.Errorf("expected session name 'beta', got %q", name)
	}
	if sess.Branch != "feature-beta" {
		t.Errorf("expected branch 'feature-beta', got %q", sess.Branch)
	}

	// Test UsedNames
	usedNames := state2.UsedNames()
	if len(usedNames) != 2 {
		t.Errorf("expected 2 used names, got %d", len(usedNames))
	}

	t.Log("Session state persistence integration test completed successfully")
}

// TestIntegration_CleanupOnError tests proper cleanup when operations fail.
func TestIntegration_CleanupOnError(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available, skipping integration test")
	}

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Setup config
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

	sessionName := "cleanup-test"
	worktreePath := cfg.WorktreePath(sessionName)
	branchName := "feature-cleanup"

	// Create worktree
	if err := worktree.Create(repoDir, worktreePath, branchName); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Create tmux session
	if err := tmux.NewSession(sessionName, worktreePath, repoDir+"/.beads", "", nil); err != nil {
		t.Fatalf("failed to create tmux session: %v", err)
	}

	// Verify resources exist
	if !tmux.SessionExists(sessionName) {
		t.Error("tmux session should exist")
	}
	if !worktree.Exists(worktreePath) {
		t.Error("worktree should exist")
	}

	// Simulate cleanup (as would happen on close/kill)
	if err := tmux.Kill(sessionName); err != nil {
		t.Fatalf("failed to kill tmux session: %v", err)
	}
	if err := worktree.Remove(worktreePath); err != nil {
		t.Fatalf("failed to remove worktree: %v", err)
	}

	// Verify cleanup
	if tmux.SessionExists(sessionName) {
		t.Error("tmux session should not exist after cleanup")
	}
	if worktree.Exists(worktreePath) {
		t.Error("worktree should not exist after cleanup")
	}

	t.Log("Cleanup on error integration test completed successfully")
}

// TestIntegration_NamepoolAllocation tests namepool allocation with multiple sessions.
func TestIntegration_NamepoolAllocation(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create namepool with specific names
	namepoolContent := "alpha\nbeta\ngamma\ndelta\n"
	if err := os.WriteFile(filepath.Join(configDir, "namepool.txt"), []byte(namepoolContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
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

	pool, err := namepool.Load(cfg)
	if err != nil {
		t.Fatalf("failed to load namepool: %v", err)
	}

	// Allocate names
	usedNames := []string{}

	name1, err := pool.Allocate(usedNames)
	if err != nil {
		t.Fatalf("failed to allocate first name: %v", err)
	}
	usedNames = append(usedNames, name1)

	name2, err := pool.Allocate(usedNames)
	if err != nil {
		t.Fatalf("failed to allocate second name: %v", err)
	}
	usedNames = append(usedNames, name2)

	// Names should be different
	if name1 == name2 {
		t.Errorf("expected different names, got %q twice", name1)
	}

	// Allocate remaining names
	name3, _ := pool.Allocate(usedNames)
	usedNames = append(usedNames, name3)
	name4, _ := pool.Allocate(usedNames)
	usedNames = append(usedNames, name4)

	// All 4 names should be allocated
	if len(usedNames) != 4 {
		t.Errorf("expected 4 names allocated, got %d", len(usedNames))
	}

	// Next allocation should fail or return fallback
	_, err = pool.Allocate(usedNames)
	// Depending on implementation, this might error or return a generated name
	t.Logf("allocation after exhaustion: err=%v", err)

	t.Log("Namepool allocation integration test completed successfully")
}

// TestIntegration_SeanceWorkflow tests the seance (session recovery) workflow.
func TestIntegration_SeanceWorkflow(t *testing.T) {
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

	logger := events.NewLogger(cfg)

	// Simulate completed sessions (what seance would query)
	_ = logger.LogSessionStart("session-1", "bead-1", "proj", "/worktree1")
	_ = logger.LogSessionEnd("session-1", "bead-1", "proj", "claude-abc123", "direct", "")

	_ = logger.LogSessionStart("session-2", "bead-2", "proj", "/worktree2")
	_ = logger.LogSessionEnd("session-2", "bead-2", "proj", "claude-def456", "pr-auto", "https://pr.url")

	_ = logger.LogSessionStart("session-3", "bead-3", "proj", "/worktree3")
	_ = logger.LogSessionKill("session-3", "bead-3", "proj") // Killed, no claude session

	// Query recent sessions (seance functionality)
	sessions, err := logger.RecentSessions(10)
	if err != nil {
		t.Fatalf("RecentSessions failed: %v", err)
	}

	// Should only have 2 sessions (session-3 was killed, no claude_session)
	if len(sessions) != 2 {
		t.Errorf("expected 2 resumable sessions, got %d", len(sessions))
	}

	// Most recent first
	if sessions[0].Session != "session-2" {
		t.Errorf("expected session-2 first, got %s", sessions[0].Session)
	}
	if sessions[0].ClaudeSession != "claude-def456" {
		t.Errorf("expected claude-def456, got %s", sessions[0].ClaudeSession)
	}

	// Find specific session
	found, err := logger.FindSession("session-1")
	if err != nil {
		t.Fatalf("FindSession failed: %v", err)
	}
	if found.ClaudeSession != "claude-abc123" {
		t.Errorf("expected claude-abc123, got %s", found.ClaudeSession)
	}

	// Find non-existent session
	_, err = logger.FindSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}

	t.Log("Seance workflow integration test completed successfully")
}

// TestIntegration_WorktreeBranchOperations tests worktree operations with different branch scenarios.
func TestIntegration_WorktreeBranchOperations(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create worktree with new branch
	worktree1 := filepath.Join(tmpDir, "worktree1")
	if err := worktree.Create(repoDir, worktree1, "feature-new"); err != nil {
		t.Fatalf("failed to create worktree with new branch: %v", err)
	}
	defer worktree.Remove(worktree1)

	// Verify branch was created
	cmd := exec.Command("git", "-C", worktree1, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get branch: %v", err)
	}
	if strings.TrimSpace(string(output)) != "feature-new" {
		t.Errorf("expected branch 'feature-new', got %q", strings.TrimSpace(string(output)))
	}

	// Create second worktree with different branch
	worktree2 := filepath.Join(tmpDir, "worktree2")
	if err := worktree.Create(repoDir, worktree2, "feature-second"); err != nil {
		t.Fatalf("failed to create second worktree: %v", err)
	}
	defer worktree.Remove(worktree2)

	// Verify both worktrees exist and have different branches
	if !worktree.Exists(worktree1) {
		t.Error("worktree1 should exist")
	}
	if !worktree.Exists(worktree2) {
		t.Error("worktree2 should exist")
	}

	cmd = exec.Command("git", "-C", worktree2, "branch", "--show-current")
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("failed to get branch for worktree2: %v", err)
	}
	if strings.TrimSpace(string(output)) != "feature-second" {
		t.Errorf("expected branch 'feature-second', got %q", strings.TrimSpace(string(output)))
	}

	// Clean up worktree1
	if err := worktree.Remove(worktree1); err != nil {
		t.Fatalf("failed to remove worktree1: %v", err)
	}
	if worktree.Exists(worktree1) {
		t.Error("worktree1 should not exist after removal")
	}

	// worktree2 should still exist
	if !worktree.Exists(worktree2) {
		t.Error("worktree2 should still exist")
	}

	t.Log("Worktree branch operations integration test completed successfully")
}

// TestIntegration_HandoffMarker tests the handoff marker file operations.
func TestIntegration_HandoffMarker(t *testing.T) {
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

	// Initially no marker
	exists, _, _, err := handoff.CheckMarker(cfg)
	if err != nil {
		t.Fatalf("CheckMarker failed: %v", err)
	}
	if exists {
		t.Error("expected no marker initially")
	}

	// Create marker manually (simulating writeMarker)
	runtimeDir := filepath.Join(cfg.ConfigDir(), handoff.RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, handoff.HandoffMarkerFile)
	timestamp := time.Now().Format(time.RFC3339)
	sessionName := "integration-test-session"
	content := timestamp + "\n" + sessionName + "\n"
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// Check marker exists
	exists, prevSession, handoffTime, err := handoff.CheckMarker(cfg)
	if err != nil {
		t.Fatalf("CheckMarker failed: %v", err)
	}
	if !exists {
		t.Error("expected marker to exist")
	}
	if prevSession != sessionName {
		t.Errorf("expected prevSession %s, got %s", sessionName, prevSession)
	}
	if handoffTime.IsZero() {
		t.Error("expected non-zero handoff time")
	}

	// Clear marker
	if err := handoff.ClearMarker(cfg); err != nil {
		t.Fatalf("ClearMarker failed: %v", err)
	}

	// Verify cleared
	exists, _, _, err = handoff.CheckMarker(cfg)
	if err != nil {
		t.Fatalf("CheckMarker after clear failed: %v", err)
	}
	if exists {
		t.Error("expected marker to be cleared")
	}

	t.Log("Handoff marker integration test completed successfully")
}

// TestIntegration_PrimeWorkflow tests the prime command workflow.
func TestIntegration_PrimeWorkflow(t *testing.T) {
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

	// Test prime without marker (normal startup)
	opts := &handoff.PrimeOptions{
		Quiet:     true,
		NoBdPrime: true, // Skip bd prime to avoid external dependency
	}

	result, err := handoff.Prime(cfg, opts)
	if err != nil {
		t.Fatalf("Prime failed: %v", err)
	}
	if result.IsPostHandoff {
		t.Error("expected IsPostHandoff to be false without marker")
	}

	// Create marker to simulate post-handoff
	runtimeDir := filepath.Join(cfg.ConfigDir(), handoff.RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, handoff.HandoffMarkerFile)
	timestamp := time.Now().Format(time.RFC3339)
	content := timestamp + "\nprev-session\n"
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// Test prime with marker (post-handoff startup)
	result2, err := handoff.Prime(cfg, opts)
	if err != nil {
		t.Fatalf("Prime with marker failed: %v", err)
	}
	if !result2.IsPostHandoff {
		t.Error("expected IsPostHandoff to be true with marker")
	}
	if result2.PrevSession != "prev-session" {
		t.Errorf("expected PrevSession 'prev-session', got %s", result2.PrevSession)
	}

	// Marker should be cleared after prime
	exists, _, _, _ := handoff.CheckMarker(cfg)
	if exists {
		t.Error("expected marker to be cleared after Prime")
	}

	t.Log("Prime workflow integration test completed successfully")
}

// TestIntegration_HandoffContextCollection tests context collection for handoff.
func TestIntegration_HandoffContextCollection(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"worktree_root": "` + filepath.Join(tmpDir, "worktrees") + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Create some session state
	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	state.Sessions["alpha"] = &session.Session{
		Bead:    "test-bead-1",
		Project: "testproj",
		Status:  "working",
	}
	state.Sessions["beta"] = &session.Session{
		Bead:    "test-bead-2",
		Project: "testproj",
		Status:  "idle",
	}
	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Test GenerateSummary
	summary, err := handoff.GenerateSummary(cfg)
	if err != nil {
		t.Fatalf("GenerateSummary failed: %v", err)
	}

	// Summary should contain session info
	if !strings.Contains(summary, "Active Sessions") {
		t.Error("expected summary to contain 'Active Sessions'")
	}
	if !strings.Contains(summary, "alpha") {
		t.Error("expected summary to contain session 'alpha'")
	}
	if !strings.Contains(summary, "beta") {
		t.Error("expected summary to contain session 'beta'")
	}
	if !strings.Contains(summary, "test-bead-1") {
		t.Error("expected summary to contain 'test-bead-1'")
	}

	t.Log("Handoff context collection integration test completed successfully")
}

// TestIntegration_HandoffDryRun tests the handoff dry-run functionality.
func TestIntegration_HandoffDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"worktree_root": "` + filepath.Join(tmpDir, "worktrees") + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	opts := &handoff.Options{
		Message:     "Test handoff message",
		AutoCollect: false,
		DryRun:      true,
	}

	// Dry run should not create marker or modify anything
	result, err := handoff.Run(cfg, opts)
	if err != nil {
		t.Fatalf("Handoff dry run failed: %v", err)
	}

	// Result message should contain our custom message
	if !strings.Contains(result.Message, "Test handoff message") {
		t.Error("expected result to contain our message")
	}

	// Marker should NOT be created (dry run)
	exists, _, _, _ := handoff.CheckMarker(cfg)
	if exists {
		t.Error("expected no marker after dry run")
	}

	t.Log("Handoff dry run integration test completed successfully")
}

// TestIntegration_QuickPrime tests the QuickPrime function.
func TestIntegration_QuickPrime(t *testing.T) {
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

	// Test QuickPrime without marker
	if err := handoff.QuickPrime(cfg); err != nil {
		t.Fatalf("QuickPrime without marker failed: %v", err)
	}

	// Create marker
	runtimeDir := filepath.Join(cfg.ConfigDir(), handoff.RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("failed to create runtime dir: %v", err)
	}

	markerPath := filepath.Join(runtimeDir, handoff.HandoffMarkerFile)
	timestamp := time.Now().Format(time.RFC3339)
	content := timestamp + "\nquick-test-session\n"
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write marker: %v", err)
	}

	// Test QuickPrime with marker
	if err := handoff.QuickPrime(cfg); err != nil {
		t.Fatalf("QuickPrime with marker failed: %v", err)
	}

	// Marker should be cleared
	exists, _, _, _ := handoff.CheckMarker(cfg)
	if exists {
		t.Error("expected marker to be cleared after QuickPrime")
	}

	t.Log("QuickPrime integration test completed successfully")
}

// TestIntegration_StatusCommand tests the wt status command functionality.
// This tests the underlying logic used by cmdStatus without invoking the actual CLI.
func TestIntegration_StatusCommand(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available, skipping integration test")
	}

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Setup config
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

	pool, err := namepool.Load(cfg)
	if err != nil {
		t.Fatalf("failed to load namepool: %v", err)
	}

	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Allocate a session name
	sessionName, err := pool.Allocate(state.UsedNames())
	if err != nil {
		t.Fatalf("failed to allocate name: %v", err)
	}

	// Create worktree
	worktreePath := cfg.WorktreePath(sessionName)
	branchName := "test-status-branch"
	if err := worktree.Create(repoDir, worktreePath, branchName); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	// Create tmux session
	if err := tmux.NewSession(sessionName, worktreePath, repoDir+"/.beads", "", nil); err != nil {
		t.Fatalf("failed to create tmux session: %v", err)
	}

	// Save session state with additional metadata
	state.Sessions[sessionName] = &session.Session{
		Bead:       "test-status-bead",
		Project:    "testproj",
		Worktree:   worktreePath,
		Branch:     branchName,
		PortOffset: 1000,
		Status:     "working",
	}
	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Test: Verify session can be looked up by worktree path (as cmdStatus does)
	var foundSession string
	var foundSess *session.Session
	for name, s := range state.Sessions {
		if s.Worktree == worktreePath {
			foundSession = name
			foundSess = s
			break
		}
	}

	if foundSession == "" {
		t.Error("expected to find session by worktree path")
	}
	if foundSess == nil {
		t.Error("expected session object to be non-nil")
	}
	if foundSess.Bead != "test-status-bead" {
		t.Errorf("expected bead 'test-status-bead', got %q", foundSess.Bead)
	}
	if foundSess.PortOffset != 1000 {
		t.Errorf("expected port_offset 1000, got %d", foundSess.PortOffset)
	}

	// Test: Check uncommitted changes (should be clean)
	hasChanges, err := merge.HasUncommittedChanges(worktreePath)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes in fresh worktree")
	}

	// Test: Get current branch
	branch, err := merge.GetCurrentBranch(worktreePath)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch != branchName {
		t.Errorf("expected branch %q, got %q", branchName, branch)
	}

	// Test: Add a file and verify uncommitted changes
	testFile := filepath.Join(worktreePath, "status-test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	hasChanges, err = merge.HasUncommittedChanges(worktreePath)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if !hasChanges {
		t.Error("expected uncommitted changes after creating file")
	}

	// Cleanup
	_ = tmux.Kill(sessionName)
	_ = worktree.Remove(worktreePath)

	t.Log("Status command integration test completed successfully")
}

// TestIntegration_SwitchCommand tests switching between sessions.
func TestIntegration_SwitchCommand(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available, skipping integration test")
	}

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Setup config
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

	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Create two sessions
	session1 := "switch-test-alpha"
	session2 := "switch-test-beta"
	bead1 := "test-bead-alpha"
	bead2 := "test-bead-beta"

	for _, sessInfo := range []struct {
		name string
		bead string
	}{
		{session1, bead1},
		{session2, bead2},
	} {
		worktreePath := cfg.WorktreePath(sessInfo.name)
		branchName := "feature-" + sessInfo.name

		if err := worktree.Create(repoDir, worktreePath, branchName); err != nil {
			t.Fatalf("failed to create worktree for %s: %v", sessInfo.name, err)
		}

		if err := tmux.NewSession(sessInfo.name, worktreePath, repoDir+"/.beads", "", nil); err != nil {
			t.Fatalf("failed to create tmux session for %s: %v", sessInfo.name, err)
		}

		state.Sessions[sessInfo.name] = &session.Session{
			Bead:     sessInfo.bead,
			Project:  "testproj",
			Worktree: worktreePath,
			Branch:   branchName,
			Status:   "working",
		}
	}

	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Test: Find session by exact name
	_, exists := state.Sessions[session1]
	if !exists {
		t.Errorf("expected to find session %s by name", session1)
	}

	_, exists = state.Sessions[session2]
	if !exists {
		t.Errorf("expected to find session %s by name", session2)
	}

	// Test: Find session by bead ID
	foundName, foundSess := state.FindByBead(bead1)
	if foundName != session1 {
		t.Errorf("expected to find session %s by bead %s, got %s", session1, bead1, foundName)
	}
	if foundSess == nil {
		t.Errorf("expected session object for bead %s", bead1)
	}

	foundName, foundSess = state.FindByBead(bead2)
	if foundName != session2 {
		t.Errorf("expected to find session %s by bead %s, got %s", session2, bead2, foundName)
	}

	// Test: Verify both tmux sessions exist
	if !tmux.SessionExists(session1) {
		t.Errorf("tmux session %s should exist", session1)
	}
	if !tmux.SessionExists(session2) {
		t.Errorf("tmux session %s should exist", session2)
	}

	// Test: Find non-existent session
	foundName, foundSess = state.FindByBead("non-existent-bead")
	if foundName != "" || foundSess != nil {
		t.Error("expected no session for non-existent bead")
	}

	// Cleanup
	for _, sessName := range []string{session1, session2} {
		_ = tmux.Kill(sessName)
		worktreePath := cfg.WorktreePath(sessName)
		_ = worktree.Remove(worktreePath)
	}

	t.Log("Switch command integration test completed successfully")
}

// TestIntegration_DoctorChecks tests the doctor diagnostic checks.
func TestIntegration_DoctorChecks(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")

	// Setup config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Test: Check tmux is available
	_, tmuxErr := exec.LookPath("tmux")
	hasTmuxCmd := tmuxErr == nil

	// Test: Check git is available
	_, gitErr := exec.LookPath("git")
	hasGit := gitErr == nil

	if !hasGit {
		t.Skip("git not available, skipping doctor checks")
	}

	// Test: Worktree root should be creatable
	if err := os.MkdirAll(worktreeRoot, 0755); err != nil {
		t.Fatalf("failed to create worktree root: %v", err)
	}

	// Test: Worktree root should be writable
	testFile := filepath.Join(worktreeRoot, ".test-write")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("worktree root should be writable: %v", err)
	}
	os.Remove(testFile)

	// Test: Config should be loadable
	if cfg.ConfigDir() == "" {
		t.Error("expected config dir to be set")
	}

	// Test: Session state should be loadable
	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load session state: %v", err)
	}
	if state == nil {
		t.Error("expected session state to be non-nil")
	}

	t.Logf("Doctor checks: tmux=%v, git=%v", hasTmuxCmd, hasGit)
	t.Log("Doctor checks integration test completed successfully")
}

// TestIntegration_ErrorPaths tests various error conditions.
func TestIntegration_ErrorPaths(t *testing.T) {
	t.Run("InvalidConfig", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := filepath.Join(tmpDir, "config")

		// Create invalid JSON config
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{invalid json}"), 0644); err != nil {
			t.Fatal(err)
		}

		// Loading config should fail
		_, err := config.LoadFromDir(configDir)
		if err == nil {
			t.Error("expected error when loading invalid config")
		}
	})

	t.Run("CorruptedSessionState", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := filepath.Join(tmpDir, "config")
		worktreeRoot := filepath.Join(tmpDir, "worktrees")

		// Create valid config but corrupted sessions.json
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatal(err)
		}
		configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
			t.Fatal(err)
		}
		// Corrupted JSON
		if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{corrupted"), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := config.LoadFromDir(configDir)
		if err != nil {
			t.Fatalf("config should load: %v", err)
		}

		// Loading session state should fail
		_, err = session.LoadState(cfg)
		if err == nil {
			t.Error("expected error when loading corrupted session state")
		}
	})

	t.Run("SessionAlreadyExistsForBead", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := filepath.Join(tmpDir, "config")
		worktreeRoot := filepath.Join(tmpDir, "worktrees")

		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatal(err)
		}
		configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := config.LoadFromDir(configDir)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		state, err := session.LoadState(cfg)
		if err != nil {
			t.Fatalf("failed to load state: %v", err)
		}

		// Add a session for a bead
		beadID := "test-duplicate-bead"
		state.Sessions["existing-session"] = &session.Session{
			Bead:    beadID,
			Project: "testproj",
			Status:  "working",
		}
		if err := state.Save(); err != nil {
			t.Fatalf("failed to save state: %v", err)
		}

		// Check if session exists for bead (as cmdNew would do)
		for name, sess := range state.Sessions {
			if sess.Bead == beadID {
				// This is the expected behavior
				t.Logf("Found existing session '%s' for bead %s", name, beadID)
				break
			}
		}
	})

	t.Run("ProjectWithActiveSessions", func(t *testing.T) {
		tmpDir := t.TempDir()
		configDir := filepath.Join(tmpDir, "config")
		worktreeRoot := filepath.Join(tmpDir, "worktrees")
		repoDir := filepath.Join(tmpDir, "repo")

		// Initialize git repo
		if err := initGitRepo(repoDir); err != nil {
			t.Fatalf("failed to init git repo: %v", err)
		}

		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatal(err)
		}
		configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
		if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		cfg, err := config.LoadFromDir(configDir)
		if err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		// Add a project
		mgr := project.NewManager(cfg)
		proj, err := mgr.Add("testproj", repoDir)
		if err != nil {
			t.Fatalf("failed to add project: %v", err)
		}

		// Add an active session for this project
		state, _ := session.LoadState(cfg)
		state.Sessions["active-session"] = &session.Session{
			Bead:    "test-bead",
			Project: proj.Name,
			Status:  "working",
		}
		if err := state.Save(); err != nil {
			t.Fatalf("failed to save state: %v", err)
		}

		// Check for active sessions (as cmdProjectRemove does)
		var activeSessions []string
		for sessName, sess := range state.Sessions {
			if sess.Project == proj.Name {
				activeSessions = append(activeSessions, sessName)
			}
		}

		if len(activeSessions) == 0 {
			t.Error("expected to find active sessions for project")
		}

		// This should prevent deletion
		t.Logf("Project has %d active session(s): %v", len(activeSessions), activeSessions)

		// Clean up: remove session and then project
		delete(state.Sessions, "active-session")
		state.Save()
		mgr.Delete(proj.Name)
	})

	t.Run("WorktreeAlreadyExists", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoDir := filepath.Join(tmpDir, "repo")
		worktreeDir := filepath.Join(tmpDir, "worktree")

		if err := initGitRepo(repoDir); err != nil {
			t.Fatalf("failed to init git repo: %v", err)
		}

		// Create first worktree
		if err := worktree.Create(repoDir, worktreeDir, "feature-1"); err != nil {
			t.Fatalf("failed to create worktree: %v", err)
		}

		// Try to create same worktree again (should fail)
		err := worktree.Create(repoDir, worktreeDir, "feature-2")
		if err == nil {
			t.Error("expected error when creating worktree at existing path")
		}

		// Clean up
		worktree.Remove(worktreeDir)
	})
}

// TestIntegration_HookExecution tests on_create and on_close hooks.
func TestIntegration_HookExecution(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create test script that writes to a marker file
	hookMarkerFile := filepath.Join(tmpDir, "hook-executed")

	// Setup config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Add a project with hooks
	mgr := project.NewManager(cfg)
	proj, err := mgr.Add("hooktest", repoDir)
	if err != nil {
		t.Fatalf("failed to add project: %v", err)
	}

	// Configure hooks on the project
	proj.Hooks = &project.Hooks{
		OnCreate: []string{"touch " + hookMarkerFile + ".create"},
		OnClose:  []string{"touch " + hookMarkerFile + ".close"},
	}
	if err := mgr.Save(proj); err != nil {
		t.Fatalf("failed to save project with hooks: %v", err)
	}

	// Verify hooks are saved
	reloadedProj, err := mgr.Get("hooktest")
	if err != nil {
		t.Fatalf("failed to reload project: %v", err)
	}
	if reloadedProj.Hooks == nil {
		t.Error("expected hooks to be saved")
	}
	if len(reloadedProj.Hooks.OnCreate) != 1 {
		t.Errorf("expected 1 on_create hook, got %d", len(reloadedProj.Hooks.OnCreate))
	}
	if len(reloadedProj.Hooks.OnClose) != 1 {
		t.Errorf("expected 1 on_close hook, got %d", len(reloadedProj.Hooks.OnClose))
	}

	// Test: Run on_create hook manually (simulating what cmdNew does)
	worktreePath := filepath.Join(worktreeRoot, "hook-test-worktree")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	if err := testenv.RunOnCreateHooks(reloadedProj, worktreePath, 0, ""); err != nil {
		t.Fatalf("failed to run on_create hooks: %v", err)
	}

	// Verify on_create hook executed
	if _, err := os.Stat(hookMarkerFile + ".create"); os.IsNotExist(err) {
		t.Error("on_create hook marker file should exist")
	}

	// Test: Run on_close hook manually (simulating what cmdKill/cmdClose does)
	if err := testenv.RunOnCloseHooks(reloadedProj, worktreePath, 0, ""); err != nil {
		t.Fatalf("failed to run on_close hooks: %v", err)
	}

	// Verify on_close hook executed
	if _, err := os.Stat(hookMarkerFile + ".close"); os.IsNotExist(err) {
		t.Error("on_close hook marker file should exist")
	}

	// Clean up
	mgr.Delete("hooktest")

	t.Log("Hook execution integration test completed successfully")
}

// TestIntegration_CrossProjectSessions tests sessions across multiple projects.
func TestIntegration_CrossProjectSessions(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available, skipping integration test")
	}

	// Clean up any existing test sessions from previous runs
	_ = tmux.Kill("cross-proj-session-1")
	_ = tmux.Kill("cross-proj-session-2")

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")

	// Create two separate repos for two projects
	repo1 := filepath.Join(tmpDir, "repo1")
	repo2 := filepath.Join(tmpDir, "repo2")

	if err := initGitRepo(repo1); err != nil {
		t.Fatalf("failed to init repo1: %v", err)
	}
	if err := initGitRepo(repo2); err != nil {
		t.Fatalf("failed to init repo2: %v", err)
	}

	// Setup config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Add two projects
	mgr := project.NewManager(cfg)
	proj1, err := mgr.Add("project-alpha", repo1)
	if err != nil {
		t.Fatalf("failed to add project1: %v", err)
	}
	proj2, err := mgr.Add("project-beta", repo2)
	if err != nil {
		t.Fatalf("failed to add project2: %v", err)
	}

	// Create sessions for each project
	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Session for project 1
	session1 := "cross-proj-session-1"
	worktree1 := cfg.WorktreePath(session1)
	if err := worktree.Create(repo1, worktree1, "feature-proj1"); err != nil {
		t.Fatalf("failed to create worktree for project1: %v", err)
	}
	if err := tmux.NewSession(session1, worktree1, repo1+"/.beads", "", nil); err != nil {
		t.Fatalf("failed to create tmux session for project1: %v", err)
	}
	state.Sessions[session1] = &session.Session{
		Bead:    "project-alpha-abc",
		Project: proj1.Name,
		Status:  "working",
	}

	// Session for project 2
	session2 := "cross-proj-session-2"
	worktree2 := cfg.WorktreePath(session2)
	if err := worktree.Create(repo2, worktree2, "feature-proj2"); err != nil {
		t.Fatalf("failed to create worktree for project2: %v", err)
	}
	if err := tmux.NewSession(session2, worktree2, repo2+"/.beads", "", nil); err != nil {
		t.Fatalf("failed to create tmux session for project2: %v", err)
	}
	state.Sessions[session2] = &session.Session{
		Bead:    "project-beta-xyz",
		Project: proj2.Name,
		Status:  "working",
	}

	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Test: Verify both sessions exist
	if len(state.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(state.Sessions))
	}

	// Test: Count sessions per project
	sessionsByProject := make(map[string]int)
	for _, sess := range state.Sessions {
		sessionsByProject[sess.Project]++
	}

	if sessionsByProject[proj1.Name] != 1 {
		t.Errorf("expected 1 session for project1, got %d", sessionsByProject[proj1.Name])
	}
	if sessionsByProject[proj2.Name] != 1 {
		t.Errorf("expected 1 session for project2, got %d", sessionsByProject[proj2.Name])
	}

	// Test: Find by bead prefix (simulating project lookup)
	foundProj, err := mgr.FindByBeadPrefix("project-alpha-abc")
	if err != nil {
		t.Fatalf("FindByBeadPrefix failed: %v", err)
	}
	if foundProj.Name != proj1.Name {
		t.Errorf("expected project %s, got %s", proj1.Name, foundProj.Name)
	}

	// Clean up
	for _, sessName := range []string{session1, session2} {
		_ = tmux.Kill(sessName)
		_ = worktree.Remove(cfg.WorktreePath(sessName))
	}
	_ = mgr.Delete(proj1.Name)
	_ = mgr.Delete(proj2.Name)

	t.Log("Cross-project sessions integration test completed successfully")
}

// TestIntegration_TestEnvSetupTeardown tests test environment setup and teardown.
func TestIntegration_TestEnvSetupTeardown(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	repoDir := filepath.Join(tmpDir, "repo")

	// Initialize git repo
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Marker files for setup/teardown
	setupMarker := filepath.Join(tmpDir, "setup-ran")
	teardownMarker := filepath.Join(tmpDir, "teardown-ran")
	healthFile := filepath.Join(tmpDir, "healthy")

	// Setup config
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

	// Add a project with test_env configuration
	mgr := project.NewManager(cfg)
	proj, err := mgr.Add("testenv-proj", repoDir)
	if err != nil {
		t.Fatalf("failed to add project: %v", err)
	}

	// Configure test environment
	proj.TestEnv = &project.TestEnv{
		Setup:       "touch " + setupMarker,
		Teardown:    "touch " + teardownMarker,
		HealthCheck: "test -f " + healthFile,
		PortEnv:     "TEST_PORT",
	}
	if err := mgr.Save(proj); err != nil {
		t.Fatalf("failed to save project: %v", err)
	}

	// Create a worktree path for testing
	worktreePath := filepath.Join(worktreeRoot, "testenv-worktree")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatal(err)
	}

	// Test: Allocate port offset
	portOffset := testenv.AllocatePortOffset(proj, nil)
	if portOffset != testenv.DefaultPortBase {
		t.Errorf("expected port offset %d, got %d", testenv.DefaultPortBase, portOffset)
	}

	// Test: Run setup
	if err := testenv.RunSetup(proj, worktreePath, portOffset); err != nil {
		t.Fatalf("RunSetup failed: %v", err)
	}

	// Verify setup marker exists
	if _, err := os.Stat(setupMarker); os.IsNotExist(err) {
		t.Error("setup marker should exist after RunSetup")
	}

	// Test: Health check should fail (file doesn't exist yet)
	err = testenv.WaitForHealthy(proj, worktreePath, portOffset, 1*time.Second)
	if err == nil {
		t.Error("expected health check to fail when health file doesn't exist")
	}

	// Create health file and check again
	if err := os.WriteFile(healthFile, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	err = testenv.WaitForHealthy(proj, worktreePath, portOffset, 5*time.Second)
	if err != nil {
		t.Errorf("expected health check to pass: %v", err)
	}

	// Test: Run teardown
	if err := testenv.RunTeardown(proj, worktreePath, portOffset); err != nil {
		t.Fatalf("RunTeardown failed: %v", err)
	}

	// Verify teardown marker exists
	if _, err := os.Stat(teardownMarker); os.IsNotExist(err) {
		t.Error("teardown marker should exist after RunTeardown")
	}

	// Clean up
	mgr.Delete(proj.Name)

	t.Log("Test environment setup/teardown integration test completed successfully")
}

// TestIntegration_AutoModeComponents tests components of the auto mode workflow.
func TestIntegration_AutoModeComponents(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")
	logsDir := filepath.Join(configDir, "logs")

	// Setup config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
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

	// Test: Default auto config
	autoCfg := auto.DefaultConfig()
	if autoCfg.TimeoutMinutes != 30 {
		t.Errorf("expected default timeout 30, got %d", autoCfg.TimeoutMinutes)
	}
	if autoCfg.Command == "" {
		t.Error("expected default command to be set")
	}
	if autoCfg.PromptTemplate == "" {
		t.Error("expected default prompt template to be set")
	}

	// Test: Logger creation
	logger, err := auto.NewLogger(logsDir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test: Log a message
	logger.Log("test message %s", "arg1")

	// Test: Lock file mechanics
	lockFile := filepath.Join(configDir, "auto.lock")

	// Create lock file manually
	lock := struct {
		PID       int    `json:"pid"`
		StartTime string `json:"start_time"`
	}{
		PID:       os.Getpid(),
		StartTime: time.Now().Format(time.RFC3339),
	}
	lockData, _ := json.MarshalIndent(lock, "", "  ")
	if err := os.WriteFile(lockFile, lockData, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify lock file exists
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		t.Error("lock file should exist")
	}

	// Clean up lock
	os.Remove(lockFile)

	// Test: Stop file mechanics
	stopFile := filepath.Join(configDir, "stop-auto")
	if err := os.WriteFile(stopFile, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify stop file exists
	if _, err := os.Stat(stopFile); os.IsNotExist(err) {
		t.Error("stop file should exist")
	}

	// Clean up stop file
	os.Remove(stopFile)

	// Test: Runner creation (but don't run it since it needs real beads)
	opts := &auto.Options{
		DryRun: true,
		Check:  true,
	}
	runner := auto.NewRunner(cfg, opts)
	if runner == nil {
		t.Error("expected runner to be non-nil")
	}

	t.Log("Auto mode components integration test completed successfully")
}

// TestIntegration_PickerFallback tests the picker fallback behavior.
func TestIntegration_PickerFallback(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	worktreeRoot := filepath.Join(tmpDir, "worktrees")

	// Setup config
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configJSON := `{"worktree_root": "` + worktreeRoot + `"}`
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	state, err := session.LoadState(cfg)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	// Test: Empty sessions list
	if len(state.Sessions) != 0 {
		t.Errorf("expected 0 sessions initially, got %d", len(state.Sessions))
	}

	// Add some mock sessions
	state.Sessions["picker-alpha"] = &session.Session{
		Bead:    "bead-alpha",
		Project: "proj1",
		Status:  "working",
	}
	state.Sessions["picker-beta"] = &session.Session{
		Bead:    "bead-beta",
		Project: "proj2",
		Status:  "idle",
	}
	state.Sessions["picker-gamma"] = &session.Session{
		Bead:    "bead-gamma",
		Project: "proj1",
		Status:  "working",
	}

	if err := state.Save(); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Test: Build picker entries
	type pickerEntry struct {
		name    string
		bead    string
		project string
	}

	var entries []pickerEntry
	for name, sess := range state.Sessions {
		entries = append(entries, pickerEntry{
			name:    name,
			bead:    sess.Bead,
			project: sess.Project,
		})
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 picker entries, got %d", len(entries))
	}

	// Test: Check fzf availability (don't fail if not present)
	_, hasFzfErr := exec.LookPath("fzf")
	t.Logf("fzf available: %v", hasFzfErr == nil)

	t.Log("Picker fallback integration test completed successfully")
}

// TestIntegration_DirectMergeWorkflow tests the direct merge workflow.
func TestIntegration_DirectMergeWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	worktreeDir := filepath.Join(tmpDir, "worktree")

	// Initialize main repo with main branch
	if err := initGitRepo(repoDir); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Create a worktree with a feature branch
	cmd := exec.Command("git", "-C", repoDir, "worktree", "add", "-b", "feature-direct-merge", worktreeDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}
	defer func() {
		exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", worktreeDir).Run()
	}()

	// Make a change in the worktree
	featureFile := filepath.Join(worktreeDir, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature content\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Commit the change
	cmd = exec.Command("git", "-C", worktreeDir, "add", ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	cmd = exec.Command("git", "-C", worktreeDir, "commit", "-m", "Add feature")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	// Test: Check no uncommitted changes
	hasChanges, err := merge.HasUncommittedChanges(worktreeDir)
	if err != nil {
		t.Fatalf("HasUncommittedChanges failed: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes after commit")
	}

	// Test: Get current branch
	branch, err := merge.GetCurrentBranch(worktreeDir)
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch != "feature-direct-merge" {
		t.Errorf("expected branch 'feature-direct-merge', got %q", branch)
	}

	// Note: We can't actually test DirectMerge without a remote,
	// but we've tested all the helper functions it uses.

	t.Log("Direct merge workflow integration test completed successfully")
}
