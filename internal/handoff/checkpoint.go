// Package handoff provides hub session persistence and context recovery.
package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/session"
)

const (
	// CheckpointFile is the name of the checkpoint file
	CheckpointFile = "checkpoint.json"

	// CheckpointDir is the directory for checkpoint files in the worktree
	WorktreeCheckpointDir = ".wt"
)

// Checkpoint represents saved session state for recovery after compaction
type Checkpoint struct {
	// Metadata
	CreatedAt string `json:"created_at"`
	Session   string `json:"session"`
	Bead      string `json:"bead"`
	Project   string `json:"project"`
	Worktree  string `json:"worktree"`

	// Git state
	GitBranch     string `json:"git_branch"`
	GitDiffStat   string `json:"git_diff_stat"`
	GitStatus     string `json:"git_status"`
	RecentCommits string `json:"recent_commits"`

	// Bead state
	BeadTitle    string `json:"bead_title"`
	BeadDesc     string `json:"bead_description"`
	BeadPriority int    `json:"bead_priority"`
	BeadStatus   string `json:"bead_status"`

	// Work context
	Notes string `json:"notes,omitempty"`

	// Trigger info
	Trigger string `json:"trigger"` // "manual", "auto" (pre-compaction)
}

// CheckpointOptions configures checkpoint behavior
type CheckpointOptions struct {
	Notes   string // Optional notes to include
	Trigger string // What triggered this checkpoint
	Quiet   bool   // Suppress output
	Clear   bool   // Clear existing checkpoint instead of saving
}

// SaveCheckpoint saves the current session state for recovery
func SaveCheckpoint(cfg *config.Config, opts *CheckpointOptions) (*Checkpoint, error) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting cwd: %w", err)
	}

	// Try to find session info from state
	state, err := session.LoadState(cfg)
	var sess *session.Session
	var sessionName string
	if err == nil {
		// Find session by worktree path
		for name, s := range state.Sessions {
			if s.Worktree == cwd {
				sess = s
				sessionName = name
				break
			}
		}
	}

	// Build checkpoint
	cp := &Checkpoint{
		CreatedAt: time.Now().Format(time.RFC3339),
		Worktree:  cwd,
		Trigger:   opts.Trigger,
		Notes:     opts.Notes,
	}

	// Fill session info if available
	if sess != nil {
		cp.Session = sessionName
		cp.Bead = sess.Bead
		cp.Project = sess.Project
	}

	// Collect git state
	cp.GitBranch = getGitBranch()
	cp.GitDiffStat = getGitDiffStat()
	cp.GitStatus = getGitStatusBrief()
	cp.RecentCommits = getRecentCommits(3)

	// Collect bead info if we have a bead ID
	if cp.Bead != "" {
		if beadInfo, err := bead.ShowFull(cp.Bead); err == nil {
			cp.BeadTitle = beadInfo.Title
			cp.BeadDesc = beadInfo.Description
			cp.BeadPriority = beadInfo.Priority
			cp.BeadStatus = beadInfo.Status
		}
	}

	// Save checkpoint file
	checkpointPath := getCheckpointPath(cwd)
	if err := saveCheckpointFile(checkpointPath, cp); err != nil {
		return nil, fmt.Errorf("saving checkpoint: %w", err)
	}

	// Log compaction event
	logger := events.NewLogger(cfg)
	if err := logger.LogCompaction(sessionName, cp.Bead, cp.Project, cwd); err != nil {
		// Non-fatal
		if !opts.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: could not log compaction event: %v\n", err)
		}
	}

	return cp, nil
}

// LoadCheckpoint loads a checkpoint from the current directory
func LoadCheckpoint() (*Checkpoint, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return LoadCheckpointFrom(cwd)
}

// LoadCheckpointFrom loads a checkpoint from a specific directory
func LoadCheckpointFrom(worktreePath string) (*Checkpoint, error) {
	checkpointPath := getCheckpointPath(worktreePath)

	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No checkpoint is not an error
		}
		return nil, err
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parsing checkpoint: %w", err)
	}

	return &cp, nil
}

// ClearCheckpoint removes the checkpoint file
func ClearCheckpoint() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	return ClearCheckpointFrom(cwd)
}

// ClearCheckpointFrom removes the checkpoint file from a specific directory
func ClearCheckpointFrom(worktreePath string) error {
	checkpointPath := getCheckpointPath(worktreePath)
	err := os.Remove(checkpointPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// CheckpointExists returns true if a checkpoint exists in the current directory
func CheckpointExists() bool {
	cwd, _ := os.Getwd()
	return CheckpointExistsIn(cwd)
}

// CheckpointExistsIn returns true if a checkpoint exists in the given directory
func CheckpointExistsIn(worktreePath string) bool {
	checkpointPath := getCheckpointPath(worktreePath)
	_, err := os.Stat(checkpointPath)
	return err == nil
}

// FormatCheckpointForRecovery formats checkpoint data for Claude context injection
func FormatCheckpointForRecovery(cp *Checkpoint) string {
	var sb strings.Builder

	sb.WriteString("## 🔄 Context Recovery - Session Compaction Detected\n\n")
	sb.WriteString(fmt.Sprintf("**Checkpoint saved**: %s\n", cp.CreatedAt))
	sb.WriteString(fmt.Sprintf("**Trigger**: %s\n\n", cp.Trigger))

	// Bead context
	if cp.Bead != "" {
		sb.WriteString("### Current Task\n")
		sb.WriteString(fmt.Sprintf("**Bead**: %s\n", cp.Bead))
		if cp.BeadTitle != "" {
			sb.WriteString(fmt.Sprintf("**Title**: %s\n", cp.BeadTitle))
		}
		if cp.BeadDesc != "" {
			sb.WriteString(fmt.Sprintf("**Description**: %s\n", cp.BeadDesc))
		}
		sb.WriteString(fmt.Sprintf("**Priority**: P%d | **Status**: %s\n", cp.BeadPriority, cp.BeadStatus))
		sb.WriteString("\n")
	}

	// Git state
	sb.WriteString("### Git State\n")
	sb.WriteString(fmt.Sprintf("**Branch**: %s\n", cp.GitBranch))

	if cp.GitDiffStat != "" {
		sb.WriteString("\n**Changes (diff stat)**:\n```\n")
		sb.WriteString(cp.GitDiffStat)
		sb.WriteString("```\n")
	}

	if cp.GitStatus != "" {
		sb.WriteString("\n**Status**:\n```\n")
		sb.WriteString(cp.GitStatus)
		sb.WriteString("```\n")
	}

	if cp.RecentCommits != "" {
		sb.WriteString("\n**Recent commits**:\n```\n")
		sb.WriteString(cp.RecentCommits)
		sb.WriteString("```\n")
	}

	// Custom notes
	if cp.Notes != "" {
		sb.WriteString("\n### Notes\n")
		sb.WriteString(cp.Notes)
		sb.WriteString("\n")
	}

	sb.WriteString("\n---\n")
	sb.WriteString("*Resume your work from where you left off. Check `git diff` for current changes.*\n")

	return sb.String()
}

// getCheckpointPath returns the checkpoint file path for a worktree
func getCheckpointPath(worktreePath string) string {
	return filepath.Join(worktreePath, WorktreeCheckpointDir, CheckpointFile)
}

// saveCheckpointFile writes checkpoint to disk
func saveCheckpointFile(path string, cp *Checkpoint) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Git helpers

func getGitBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getGitDiffStat() string {
	cmd := exec.Command("git", "diff", "--stat", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()
	return strings.TrimSpace(out.String())
}

func getGitStatusBrief() string {
	cmd := exec.Command("git", "status", "-s")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()
	status := strings.TrimSpace(out.String())
	// Limit to first 20 lines
	lines := strings.Split(status, "\n")
	if len(lines) > 20 {
		status = strings.Join(lines[:20], "\n") + fmt.Sprintf("\n... and %d more files", len(lines)-20)
	}
	return status
}

func getRecentCommits(n int) string {
	cmd := exec.Command("git", "log", "--oneline", fmt.Sprintf("-%d", n))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Run()
	return strings.TrimSpace(out.String())
}
