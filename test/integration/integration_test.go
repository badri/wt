//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
