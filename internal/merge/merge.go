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

// RebaseResult contains the result of a rebase operation
type RebaseResult struct {
	Success         bool
	HasConflicts    bool
	ConflictedFiles []string
}

// ConflictInfo contains detailed information about a merge conflict
type ConflictInfo struct {
	File         string // Path to conflicted file
	OurChanges   string // Content from current branch (HEAD)
	TheirChanges string // Content from target branch (main)
}

// FetchMain fetches the latest changes from origin for the default branch
func FetchMain(worktreePath, defaultBranch string) error {
	cmd := exec.Command("git", "-C", worktreePath, "fetch", "origin", defaultBranch)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fetching %s: %s: %w", defaultBranch, string(output), err)
	}
	return nil
}

// CommitsBehind returns the number of commits the current branch is behind the default branch
func CommitsBehind(worktreePath, defaultBranch string) (int, error) {
	// Count commits that are in origin/defaultBranch but not in HEAD
	cmd := exec.Command("git", "-C", worktreePath, "rev-list", "--count", "HEAD..origin/"+defaultBranch)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("counting commits behind: %w", err)
	}

	var count int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("parsing commit count: %w", err)
	}
	return count, nil
}

// RebaseOnMain rebases the current branch on top of the default branch
// Returns a RebaseResult indicating success or conflict status
func RebaseOnMain(worktreePath, defaultBranch string) (*RebaseResult, error) {
	// Attempt rebase
	cmd := exec.Command("git", "-C", worktreePath, "rebase", "origin/"+defaultBranch)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if it's a conflict situation
		if strings.Contains(string(output), "CONFLICT") || strings.Contains(string(output), "could not apply") {
			conflictedFiles, _ := GetConflictedFiles(worktreePath)
			return &RebaseResult{
				Success:         false,
				HasConflicts:    true,
				ConflictedFiles: conflictedFiles,
			}, nil
		}
		return nil, fmt.Errorf("rebasing: %s: %w", string(output), err)
	}

	return &RebaseResult{
		Success:      true,
		HasConflicts: false,
	}, nil
}

// GetConflictedFiles returns the list of files with merge conflicts
func GetConflictedFiles(worktreePath string) ([]string, error) {
	cmd := exec.Command("git", "-C", worktreePath, "diff", "--name-only", "--diff-filter=U")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting conflicted files: %w", err)
	}

	lines := strings.TrimSpace(string(output))
	if lines == "" {
		return nil, nil
	}
	return strings.Split(lines, "\n"), nil
}

// IsRebaseInProgress checks if a rebase is currently in progress
func IsRebaseInProgress(worktreePath string) bool {
	cmd := exec.Command("git", "-C", worktreePath, "rev-parse", "--git-path", "rebase-merge")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Check if the rebase-merge directory exists
	rebasePath := strings.TrimSpace(string(output))
	cmd = exec.Command("test", "-d", rebasePath)
	if err := cmd.Run(); err == nil {
		return true
	}

	// Also check rebase-apply for older git versions
	cmd = exec.Command("git", "-C", worktreePath, "rev-parse", "--git-path", "rebase-apply")
	output, err = cmd.Output()
	if err != nil {
		return false
	}
	rebasePath = strings.TrimSpace(string(output))
	cmd = exec.Command("test", "-d", rebasePath)
	return cmd.Run() == nil
}

// ContinueRebase continues a rebase after conflicts have been resolved
func ContinueRebase(worktreePath string) error {
	cmd := exec.Command("git", "-C", worktreePath, "rebase", "--continue")
	cmd.Env = append(cmd.Env, "GIT_EDITOR=true") // Skip commit message editor
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("continuing rebase: %s: %w", string(output), err)
	}
	return nil
}

// AbortRebase aborts an in-progress rebase
func AbortRebase(worktreePath string) error {
	cmd := exec.Command("git", "-C", worktreePath, "rebase", "--abort")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("aborting rebase: %s: %w", string(output), err)
	}
	return nil
}

// GetConflictMarkers reads a conflicted file and extracts the conflict markers
func GetConflictMarkers(worktreePath, filePath string) ([]ConflictInfo, error) {
	cmd := exec.Command("git", "-C", worktreePath, "diff", "--", filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting diff: %w", err)
	}

	// For a more detailed view, read the file directly to see conflict markers
	fullPath := worktreePath + "/" + filePath
	cmd = exec.Command("cat", fullPath)
	content, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	// Parse conflict markers
	var conflicts []ConflictInfo
	lines := strings.Split(string(content), "\n")

	var inConflict bool
	var currentConflict ConflictInfo
	var ourLines, theirLines []string
	var inOurs bool

	for _, line := range lines {
		if strings.HasPrefix(line, "<<<<<<<") {
			inConflict = true
			inOurs = true
			currentConflict = ConflictInfo{File: filePath}
			ourLines = nil
			theirLines = nil
		} else if strings.HasPrefix(line, "=======") {
			inOurs = false
		} else if strings.HasPrefix(line, ">>>>>>>") {
			currentConflict.OurChanges = strings.Join(ourLines, "\n")
			currentConflict.TheirChanges = strings.Join(theirLines, "\n")
			conflicts = append(conflicts, currentConflict)
			inConflict = false
		} else if inConflict {
			if inOurs {
				ourLines = append(ourLines, line)
			} else {
				theirLines = append(theirLines, line)
			}
		}
	}

	// If no conflict markers found, include the diff output
	if len(conflicts) == 0 {
		conflicts = append(conflicts, ConflictInfo{
			File:         filePath,
			OurChanges:   "See git diff output",
			TheirChanges: string(output),
		})
	}

	return conflicts, nil
}

// StageResolvedFile stages a file after conflict resolution
func StageResolvedFile(worktreePath, filePath string) error {
	cmd := exec.Command("git", "-C", worktreePath, "add", filePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("staging file: %s: %w", string(output), err)
	}
	return nil
}

// GetBranchStatus returns a string indicating how far behind/ahead the branch is
func GetBranchStatus(worktreePath, defaultBranch string) (string, error) {
	// Fetch first to ensure we have latest
	if err := FetchMain(worktreePath, defaultBranch); err != nil {
		return "", err
	}

	// Get ahead/behind counts
	cmd := exec.Command("git", "-C", worktreePath, "rev-list", "--left-right", "--count", "HEAD...origin/"+defaultBranch)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("getting branch status: %w", err)
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected output format: %s", string(output))
	}

	ahead := parts[0]
	behind := parts[1]

	if ahead == "0" && behind == "0" {
		return "up-to-date", nil
	}

	var status []string
	if ahead != "0" {
		status = append(status, fmt.Sprintf("↑%s", ahead))
	}
	if behind != "0" {
		status = append(status, fmt.Sprintf("↓%s", behind))
	}

	return strings.Join(status, " "), nil
}
