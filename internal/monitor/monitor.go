package monitor

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/badri/wt/internal/tmux"
)

// SessionStatus represents the current status of a session
type SessionStatus struct {
	Name        string
	Bead        string
	Project     string
	Status      string // working, idle, error
	LastOutput  time.Time
	IdleMinutes int
	PRStatus    string // open, merged, closed, none
	PRURL       string
}

// GetTmuxLastActivity gets the last activity time for a tmux session
func GetTmuxLastActivity(sessionName string) (time.Time, error) {
	// Get the activity time of the session
	cmd := exec.Command("tmux", "display-message", "-t", sessionName, "-p", "#{session_activity}")
	output, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}

	// Parse the Unix timestamp
	timestamp := strings.TrimSpace(string(output))
	if timestamp == "" {
		return time.Time{}, fmt.Errorf("no activity timestamp")
	}

	var unixTime int64
	if _, err := fmt.Sscanf(timestamp, "%d", &unixTime); err != nil {
		return time.Time{}, err
	}

	return time.Unix(unixTime, 0), nil
}

// GetIdleMinutes returns how many minutes a session has been idle
func GetIdleMinutes(sessionName string) int {
	lastActivity, err := GetTmuxLastActivity(sessionName)
	if err != nil {
		return -1 // Unknown
	}

	idle := time.Since(lastActivity)
	return int(idle.Minutes())
}

// DetectStatus determines if a session is working, idle, or error
func DetectStatus(sessionName string, idleThresholdMinutes int) string {
	idleMinutes := GetIdleMinutes(sessionName)
	if idleMinutes < 0 {
		return "unknown"
	}
	if idleMinutes >= idleThresholdMinutes {
		return "idle"
	}
	return "working"
}

// GetPRStatus checks the PR status for a branch using gh CLI
func GetPRStatus(worktreePath, branch string) (status, url string) {
	cmd := exec.Command("gh", "pr", "view", branch, "--json", "state,url", "-q", ".state + \" \" + .url")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return "none", ""
	}

	parts := strings.SplitN(strings.TrimSpace(string(output)), " ", 2)
	if len(parts) >= 1 {
		status = strings.ToLower(parts[0])
	}
	if len(parts) >= 2 {
		url = parts[1]
	}
	return status, url
}

// StuckState describes why a session is stuck
type StuckState struct {
	Type    string // "interrupted", "idle", "none"
	Minutes int    // idle minutes (relevant for "idle" type)
}

// DetectStuckState checks if a session is stuck (interrupted or idle).
func DetectStuckState(sessionName string, idleThresholdMinutes int) StuckState {
	// Check pane content for "Interrupted" keyword
	content, err := tmux.CapturePane(sessionName, 50)
	if err == nil && strings.Contains(content, "Interrupted") {
		return StuckState{Type: "interrupted"}
	}

	// Fall back to idle detection
	idleMinutes := GetIdleMinutes(sessionName)
	if idleMinutes >= idleThresholdMinutes {
		return StuckState{Type: "idle", Minutes: idleMinutes}
	}

	return StuckState{Type: "none"}
}

// StatusIcon returns an icon for the status
func StatusIcon(status string) string {
	switch status {
	case "working":
		return "🟢"
	case "idle":
		return "🟡"
	case "error":
		return "🔴"
	default:
		return "⚪"
	}
}

// PRStatusIcon returns an icon for PR status
func PRStatusIcon(status string) string {
	switch status {
	case "open":
		return "🔵"
	case "merged":
		return "🟣"
	case "closed":
		return "⚫"
	default:
		return "  "
	}
}
