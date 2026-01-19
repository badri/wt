package merge

import (
	"fmt"
	"os/exec"
	"strings"
)

// Mode represents the merge mode for a project
type Mode string

const (
	ModeDirect   Mode = "direct"
	ModePRAuto   Mode = "pr-auto"
	ModePRReview Mode = "pr-review"
)

// DirectMerge merges the branch directly to the default branch and pushes
func DirectMerge(worktreePath, branch, defaultBranch string) error {
	// Get the main repo path from the worktree
	repoPath, err := getMainRepoPath(worktreePath)
	if err != nil {
		return fmt.Errorf("getting main repo: %w", err)
	}

	// First push the branch to remote
	if err := pushBranch(worktreePath, branch); err != nil {
		return fmt.Errorf("pushing branch: %w", err)
	}

	// Checkout default branch in main repo
	cmd := exec.Command("git", "-C", repoPath, "checkout", defaultBranch)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("checking out %s: %s: %w", defaultBranch, string(output), err)
	}

	// Pull latest
	cmd = exec.Command("git", "-C", repoPath, "pull", "--ff-only")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pulling %s: %s: %w", defaultBranch, string(output), err)
	}

	// Merge the branch
	cmd = exec.Command("git", "-C", repoPath, "merge", "--no-ff", branch, "-m", fmt.Sprintf("Merge branch '%s'", branch))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("merging %s: %s: %w", branch, string(output), err)
	}

	// Push
	cmd = exec.Command("git", "-C", repoPath, "push")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pushing: %s: %w", string(output), err)
	}

	// Delete the remote branch
	cmd = exec.Command("git", "-C", repoPath, "push", "origin", "--delete", branch)
	_ = cmd.Run() // Ignore errors, branch might not exist on remote

	// Delete the local branch
	cmd = exec.Command("git", "-C", repoPath, "branch", "-d", branch)
	_ = cmd.Run() // Ignore errors

	return nil
}

// CreatePR creates a pull request using gh CLI
func CreatePR(worktreePath, branch, defaultBranch, title string) (string, error) {
	// Push the branch first
	if err := pushBranch(worktreePath, branch); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	// Create PR using gh
	cmd := exec.Command("gh", "pr", "create",
		"--base", defaultBranch,
		"--head", branch,
		"--title", title,
		"--body", fmt.Sprintf("Closes bead: %s", branch))
	cmd.Dir = worktreePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if PR already exists
		if strings.Contains(string(output), "already exists") {
			// Get existing PR URL
			return getExistingPRURL(worktreePath, branch)
		}
		return "", fmt.Errorf("creating PR: %s: %w", string(output), err)
	}

	// Output contains the PR URL
	return strings.TrimSpace(string(output)), nil
}

// EnableAutoMerge enables auto-merge on a PR
func EnableAutoMerge(worktreePath, prURL string) error {
	cmd := exec.Command("gh", "pr", "merge", prURL, "--auto", "--merge")
	cmd.Dir = worktreePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("enabling auto-merge: %s: %w", string(output), err)
	}

	return nil
}

// HasUncommittedChanges checks if the worktree has uncommitted changes
func HasUncommittedChanges(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("checking git status: %w", err)
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// GetCurrentBranch returns the current branch name
func GetCurrentBranch(worktreePath string) (string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func pushBranch(worktreePath, branch string) error {
	cmd := exec.Command("git", "-C", worktreePath, "push", "-u", "origin", branch)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", string(output), err)
	}
	return nil
}

func getMainRepoPath(worktreePath string) (string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "rev-parse", "--path-format=absolute", "--git-common-dir")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	// Output is path to .git directory, get parent
	gitDir := strings.TrimSpace(string(output))
	// Remove trailing /.git if present
	if strings.HasSuffix(gitDir, "/.git") {
		return strings.TrimSuffix(gitDir, "/.git"), nil
	}
	return gitDir, nil
}

func getExistingPRURL(worktreePath, branch string) (string, error) {
	cmd := exec.Command("gh", "pr", "view", branch, "--json", "url", "-q", ".url")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting existing PR: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
