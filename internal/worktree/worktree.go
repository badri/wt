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
