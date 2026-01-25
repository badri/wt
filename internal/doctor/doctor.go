package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/tmux"
	"github.com/badri/wt/internal/worktree"
)

type CheckResult struct {
	Name    string
	Status  string // "ok", "warn", "error"
	Message string
	Details []string
}

func Run(cfg *config.Config) error {
	fmt.Println("┌─ wt doctor ───────────────────────────────────────────────────────────┐")
	fmt.Println("│                                                                       │")

	var results []CheckResult

	// 1. Check tmux
	results = append(results, checkTmux())

	// 2. Check git
	results = append(results, checkGit())

	// 3. Check beads (bd command)
	results = append(results, checkBeads())

	// 4. Check worktree root directory
	results = append(results, checkWorktreeRoot(cfg))

	// 5. Check config
	results = append(results, checkConfig(cfg))

	// 6. Check for orphaned sessions/worktrees
	orphanResults := checkOrphans(cfg)
	results = append(results, orphanResults...)

	// 7. Check CLAUDE.md configuration
	claudeResults := checkClaudeMD()
	results = append(results, claudeResults...)

	// Print results
	var hasErrors, hasWarnings bool
	for _, r := range results {
		icon := "✓"
		if r.Status == "warn" {
			icon = "!"
			hasWarnings = true
		} else if r.Status == "error" {
			icon = "✗"
			hasErrors = true
		}

		fmt.Printf("│  [%s] %-65s │\n", icon, truncate(r.Name+": "+r.Message, 65))
		for _, detail := range r.Details {
			fmt.Printf("│      %-63s │\n", truncate(detail, 63))
		}
	}

	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")

	// Summary
	if hasErrors {
		fmt.Println("\nSome checks failed. Please fix the errors above.")
		return fmt.Errorf("doctor found errors")
	} else if hasWarnings {
		fmt.Println("\nSome warnings found. Review the items above.")
	} else {
		fmt.Println("\nAll checks passed!")
	}

	return nil
}

func checkTmux() CheckResult {
	// Check if tmux is installed
	path, err := exec.LookPath("tmux")
	if err != nil {
		return CheckResult{
			Name:    "tmux",
			Status:  "error",
			Message: "not installed",
			Details: []string{"Install tmux: brew install tmux (macOS) or apt install tmux (Linux)"},
		}
	}

	// Check tmux version
	cmd := exec.Command("tmux", "-V")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:    "tmux",
			Status:  "warn",
			Message: fmt.Sprintf("installed at %s but version unknown", path),
		}
	}

	version := strings.TrimSpace(string(output))

	// Check if tmux server is running
	sessions, err := tmux.ListSessions()
	serverStatus := "server running"
	if err != nil || len(sessions) == 0 {
		serverStatus = "no active sessions"
	} else {
		serverStatus = fmt.Sprintf("server running (%d sessions)", len(sessions))
	}

	return CheckResult{
		Name:    "tmux",
		Status:  "ok",
		Message: fmt.Sprintf("%s, %s", version, serverStatus),
	}
}

func checkGit() CheckResult {
	// Check if git is installed
	path, err := exec.LookPath("git")
	if err != nil {
		return CheckResult{
			Name:    "git",
			Status:  "error",
			Message: "not installed",
			Details: []string{"Install git: brew install git (macOS) or apt install git (Linux)"},
		}
	}

	// Check git version
	cmd := exec.Command("git", "--version")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:    "git",
			Status:  "warn",
			Message: fmt.Sprintf("installed at %s but version unknown", path),
		}
	}

	version := strings.TrimSpace(string(output))
	version = strings.TrimPrefix(version, "git version ")

	return CheckResult{
		Name:    "git",
		Status:  "ok",
		Message: fmt.Sprintf("version %s", version),
	}
}

func checkBeads() CheckResult {
	// Check if bd command is installed
	path, err := exec.LookPath("bd")
	if err != nil {
		return CheckResult{
			Name:    "beads (bd)",
			Status:  "error",
			Message: "bd command not installed",
			Details: []string{"Install beads: see https://github.com/badri/beads"},
		}
	}

	// Check bd version
	cmd := exec.Command("bd", "version")
	output, err := cmd.Output()
	if err != nil {
		return CheckResult{
			Name:    "beads (bd)",
			Status:  "warn",
			Message: fmt.Sprintf("installed at %s but version unknown", path),
		}
	}

	version := strings.TrimSpace(string(output))

	return CheckResult{
		Name:    "beads (bd)",
		Status:  "ok",
		Message: version,
	}
}

func checkWorktreeRoot(cfg *config.Config) CheckResult {
	root := expandPath(cfg.WorktreeRoot)

	// Check if directory exists
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(root, 0755); err != nil {
			return CheckResult{
				Name:    "worktree root",
				Status:  "error",
				Message: fmt.Sprintf("cannot create %s", root),
				Details: []string{fmt.Sprintf("Error: %v", err)},
			}
		}
		return CheckResult{
			Name:    "worktree root",
			Status:  "ok",
			Message: fmt.Sprintf("created %s", root),
		}
	} else if err != nil {
		return CheckResult{
			Name:    "worktree root",
			Status:  "error",
			Message: fmt.Sprintf("cannot access %s", root),
			Details: []string{fmt.Sprintf("Error: %v", err)},
		}
	}

	if !info.IsDir() {
		return CheckResult{
			Name:    "worktree root",
			Status:  "error",
			Message: fmt.Sprintf("%s exists but is not a directory", root),
		}
	}

	// Check if writable
	testFile := filepath.Join(root, ".wt-doctor-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return CheckResult{
			Name:    "worktree root",
			Status:  "error",
			Message: fmt.Sprintf("%s is not writable", root),
			Details: []string{fmt.Sprintf("Error: %v", err)},
		}
	}
	os.Remove(testFile)

	return CheckResult{
		Name:    "worktree root",
		Status:  "ok",
		Message: fmt.Sprintf("%s exists and is writable", root),
	}
}

func checkConfig(cfg *config.Config) CheckResult {
	configDir := cfg.ConfigDir()
	configPath := filepath.Join(configDir, "config.json")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return CheckResult{
			Name:    "config",
			Status:  "ok",
			Message: "using defaults (no config.json)",
		}
	}

	// Validate config values
	var warnings []string

	if cfg.EditorCmd == "" {
		warnings = append(warnings, "editor_cmd is empty")
	}

	if cfg.WorktreeRoot == "" {
		warnings = append(warnings, "worktree_root is empty")
	}

	if len(warnings) > 0 {
		return CheckResult{
			Name:    "config",
			Status:  "warn",
			Message: configPath,
			Details: warnings,
		}
	}

	return CheckResult{
		Name:    "config",
		Status:  "ok",
		Message: fmt.Sprintf("loaded from %s", configPath),
	}
}

func checkOrphans(cfg *config.Config) []CheckResult {
	var results []CheckResult

	state, err := session.LoadState(cfg)
	if err != nil {
		results = append(results, CheckResult{
			Name:    "sessions",
			Status:  "error",
			Message: "cannot load session state",
			Details: []string{fmt.Sprintf("Error: %v", err)},
		})
		return results
	}

	// Get tmux sessions
	tmuxSessions, err := tmux.ListSessions()
	if err != nil {
		tmuxSessions = nil
	}
	tmuxSet := make(map[string]bool)
	for _, s := range tmuxSessions {
		tmuxSet[s] = true
	}

	// Check for orphaned sessions (in state but no tmux session)
	var orphanedSessions []string
	for name := range state.Sessions {
		if !tmuxSet[name] {
			orphanedSessions = append(orphanedSessions, name)
		}
	}

	if len(orphanedSessions) > 0 {
		results = append(results, CheckResult{
			Name:    "orphaned sessions",
			Status:  "warn",
			Message: fmt.Sprintf("%d session(s) without tmux", len(orphanedSessions)),
			Details: []string{fmt.Sprintf("Sessions: %s", strings.Join(orphanedSessions, ", ")),
				"Fix with: wt kill <name> for each orphaned session"},
		})
	}

	// Check for orphaned worktrees (directory exists but no session)
	root := expandPath(cfg.WorktreeRoot)
	entries, err := os.ReadDir(root)
	if err == nil {
		var orphanedWorktrees []string
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
				if _, ok := state.Sessions[entry.Name()]; !ok {
					// Check if it's actually a git worktree
					gitDir := filepath.Join(root, entry.Name(), ".git")
					if _, err := os.Stat(gitDir); err == nil {
						orphanedWorktrees = append(orphanedWorktrees, entry.Name())
					}
				}
			}
		}

		if len(orphanedWorktrees) > 0 {
			results = append(results, CheckResult{
				Name:    "orphaned worktrees",
				Status:  "warn",
				Message: fmt.Sprintf("%d worktree(s) without session", len(orphanedWorktrees)),
				Details: []string{fmt.Sprintf("Worktrees: %s", strings.Join(orphanedWorktrees, ", ")),
					fmt.Sprintf("Location: %s", root)},
			})
		}
	}

	// Check for missing worktrees (session exists but no worktree directory)
	var missingWorktrees []string
	for name, sess := range state.Sessions {
		if !worktree.Exists(sess.Worktree) {
			missingWorktrees = append(missingWorktrees, name)
		}
	}

	if len(missingWorktrees) > 0 {
		results = append(results, CheckResult{
			Name:    "missing worktrees",
			Status:  "warn",
			Message: fmt.Sprintf("%d session(s) with missing worktree", len(missingWorktrees)),
			Details: []string{fmt.Sprintf("Sessions: %s", strings.Join(missingWorktrees, ", ")),
				"Fix with: wt kill <name> for each affected session"},
		})
	}

	// If no orphan issues found, report all clear
	if len(orphanedSessions) == 0 && len(results) == 0 {
		results = append(results, CheckResult{
			Name:    "sessions",
			Status:  "ok",
			Message: fmt.Sprintf("%d active session(s), no orphans", len(state.Sessions)),
		})
	}

	return results
}

// claudeMDHealthCheck validates the contents of a CLAUDE.md file
func claudeMDHealthCheck(path string) (status string, message string, details []string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "warn", "cannot read", []string{fmt.Sprintf("Error: %v", err)}
	}

	size := len(content)
	trimmed := strings.TrimSpace(string(content))

	// Check if empty or only whitespace
	if size == 0 || len(trimmed) == 0 {
		return "warn", "empty file", []string{"CLAUDE.md exists but has no content"}
	}

	// Check if file is very large (> 50KB might overwhelm context)
	const maxRecommendedSize = 50 * 1024
	if size > maxRecommendedSize {
		return "warn", fmt.Sprintf("large file (%d KB)", size/1024),
			[]string{
				"Large CLAUDE.md files may overwhelm AI context",
				"Consider splitting into focused sections or removing verbose content",
			}
	}

	// Check for minimal content (< 50 bytes of actual content is suspicious)
	const minUsefulContent = 50
	if len(trimmed) < minUsefulContent {
		return "warn", "minimal content",
			[]string{
				fmt.Sprintf("Only %d bytes of content", len(trimmed)),
				"Consider adding more detailed project guidelines",
			}
	}

	// All checks passed
	return "ok", "healthy", nil
}

func checkClaudeMD() []CheckResult {
	var results []CheckResult

	// 1. Check global ~/.claude/CLAUDE.md
	home, err := os.UserHomeDir()
	if err == nil {
		globalClaudeDir := filepath.Join(home, ".claude")
		globalClaudeMD := filepath.Join(globalClaudeDir, "CLAUDE.md")

		if _, err := os.Stat(globalClaudeMD); os.IsNotExist(err) {
			results = append(results, CheckResult{
				Name:    "global CLAUDE.md",
				Status:  "ok",
				Message: "not configured (optional)",
				Details: []string{fmt.Sprintf("Create %s for global instructions", globalClaudeMD)},
			})
		} else if err != nil {
			results = append(results, CheckResult{
				Name:    "global CLAUDE.md",
				Status:  "warn",
				Message: "cannot access",
				Details: []string{fmt.Sprintf("Error: %v", err)},
			})
		} else {
			// File exists, run health checks
			status, message, details := claudeMDHealthCheck(globalClaudeMD)
			results = append(results, CheckResult{
				Name:    "global CLAUDE.md",
				Status:  status,
				Message: message,
				Details: details,
			})
		}
	}

	// 2. Check project-level CLAUDE.md and .claude/ (only if in a git repo)
	gitRoot := getGitRoot()
	if gitRoot == "" {
		// Not in a git repo, skip project checks
		return results
	}

	// Check project CLAUDE.md
	projectClaudeMD := filepath.Join(gitRoot, "CLAUDE.md")
	if _, err := os.Stat(projectClaudeMD); os.IsNotExist(err) {
		results = append(results, CheckResult{
			Name:    "project CLAUDE.md",
			Status:  "warn",
			Message: "not found",
			Details: []string{
				"Consider adding CLAUDE.md with project guidelines",
				fmt.Sprintf("Path: %s", projectClaudeMD),
			},
		})
	} else if err != nil {
		results = append(results, CheckResult{
			Name:    "project CLAUDE.md",
			Status:  "warn",
			Message: "cannot access",
			Details: []string{fmt.Sprintf("Error: %v", err)},
		})
	} else {
		// File exists, run health checks
		status, message, details := claudeMDHealthCheck(projectClaudeMD)
		results = append(results, CheckResult{
			Name:    "project CLAUDE.md",
			Status:  status,
			Message: message,
			Details: details,
		})
	}

	// Check project .claude/ directory
	projectClaudeDir := filepath.Join(gitRoot, ".claude")
	if info, err := os.Stat(projectClaudeDir); os.IsNotExist(err) {
		results = append(results, CheckResult{
			Name:    "project .claude/",
			Status:  "ok",
			Message: "not configured (optional)",
			Details: []string{"Create .claude/ for MCP configs, hooks, or settings"},
		})
	} else if err != nil {
		results = append(results, CheckResult{
			Name:    "project .claude/",
			Status:  "warn",
			Message: "cannot access",
			Details: []string{fmt.Sprintf("Error: %v", err)},
		})
	} else if !info.IsDir() {
		results = append(results, CheckResult{
			Name:    "project .claude/",
			Status:  "warn",
			Message: "exists but is not a directory",
		})
	} else {
		// Check if it's a symlink (common in worktrees)
		if info, err := os.Lstat(projectClaudeDir); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(projectClaudeDir)
				results = append(results, CheckResult{
					Name:    "project .claude/",
					Status:  "ok",
					Message: fmt.Sprintf("symlink -> %s", target),
				})
			} else {
				results = append(results, CheckResult{
					Name:    "project .claude/",
					Status:  "ok",
					Message: "directory found",
				})
			}
		}
	}

	return results
}

// getGitRoot returns the root of the current git repository, or empty string if not in a repo.
func getGitRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
