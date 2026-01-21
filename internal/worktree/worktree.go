package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Create(repoPath, worktreePath, branch string) error {
	// Ensure worktree parent directory exists
	parentDir := filepath.Dir(worktreePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("creating worktree directory: %w", err)
	}

	// Check if branch exists
	branchExists := checkBranchExists(repoPath, branch)

	var cmd *exec.Cmd
	if branchExists {
		// Use existing branch
		cmd = exec.Command("git", "-C", repoPath, "worktree", "add", worktreePath, branch)
	} else {
		// Create new branch
		cmd = exec.Command("git", "-C", repoPath, "worktree", "add", "-b", branch, worktreePath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", string(output), err)
	}

	return nil
}

func Remove(worktreePath string) error {
	// First, find the main repo that owns this worktree
	// Try to remove via git worktree remove
	cmd := exec.Command("git", "-C", worktreePath, "worktree", "remove", "--force", worktreePath)
	if err := cmd.Run(); err != nil {
		// Fallback: just remove the directory
		if err := os.RemoveAll(worktreePath); err != nil {
			return fmt.Errorf("removing worktree directory: %w", err)
		}
	}

	return nil
}

func Exists(worktreePath string) bool {
	info, err := os.Stat(worktreePath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func FindGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(output)), nil
}

func checkBranchExists(repoPath, branch string) bool {
	// Check local branches
	cmd := exec.Command("git", "-C", repoPath, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if cmd.Run() == nil {
		return true
	}

	// Check remote branches
	cmd = exec.Command("git", "-C", repoPath, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	return cmd.Run() == nil
}

// SymlinkClaudeDir creates a symlink to the main repo's .claude/ directory in the worktree.
// This allows workers to inherit project-specific MCP configs, hooks, and settings.
// If .claude/ doesn't exist in the main repo, or the symlink already exists, this is a no-op.
func SymlinkClaudeDir(repoPath, worktreePath string) error {
	srcDir := filepath.Join(repoPath, ".claude")
	dstDir := filepath.Join(worktreePath, ".claude")

	// Check if source exists
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil // No .claude/ in main repo, nothing to symlink
	}

	// Check if destination already exists
	if _, err := os.Lstat(dstDir); err == nil {
		return nil // Already exists (symlink or directory)
	}

	// Create symlink
	if err := os.Symlink(srcDir, dstDir); err != nil {
		return fmt.Errorf("creating .claude symlink: %w", err)
	}

	return nil
}

// IsBranchMerged checks if a branch has been merged into the target branch (e.g., main).
// This is used to determine whether a bead should be closed when a session closes.
func IsBranchMerged(repoPath, branch, targetBranch string) bool {
	// Use merge-base --is-ancestor to check if branch is an ancestor of target
	// This returns exit code 0 if branch is merged into target
	cmd := exec.Command("git", "-C", repoPath, "merge-base", "--is-ancestor", branch, targetBranch)
	return cmd.Run() == nil
}

// CreateFromBranch creates a worktree with a new branch starting from a base branch
func CreateFromBranch(repoPath, worktreePath, newBranch, baseBranch string) error {
	// Ensure worktree parent directory exists
	parentDir := filepath.Dir(worktreePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("creating worktree directory: %w", err)
	}

	// Check if branch already exists
	if checkBranchExists(repoPath, newBranch) {
		// Use existing branch
		cmd := exec.Command("git", "-C", repoPath, "worktree", "add", worktreePath, newBranch)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git worktree add: %s: %w", string(output), err)
		}
		return nil
	}

	// Create new branch from base
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "-b", newBranch, worktreePath, baseBranch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %s: %w", string(output), err)
	}

	return nil
}
