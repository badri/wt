// Package handoff provides hub session persistence across compaction and restarts.
// It allows cycling to a fresh Claude instance while preserving work context.
package handoff

import (
	"bytes"
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
	// HandoffMarkerFile is the name of the marker file written before respawn
	HandoffMarkerFile = "handoff_marker"

	// HandoffBeadTitle is the title used for the pinned handoff bead
	HandoffBeadTitle = "Hub Handoff"

	// RuntimeDir is the directory for runtime state files
	RuntimeDir = ".wt"
)

// Options configures handoff behavior
type Options struct {
	Message     string // Custom message/context
	AutoCollect bool   // Auto-collect state (ready beads, sessions, etc.)
	DryRun      bool   // Show what would happen without doing it
}

// Result contains the outcome of a handoff operation
type Result struct {
	MarkerWritten bool
	BeadUpdated   bool
	Message       string
}

// Run executes the handoff process
func Run(cfg *config.Config, opts *Options) (*Result, error) {
	result := &Result{}

	// 1. Collect context
	context, err := collectContext(cfg, opts)
	if err != nil {
		return nil, fmt.Errorf("collecting context: %w", err)
	}
	result.Message = context

	// Dry run - just show what would happen
	if opts.DryRun {
		fmt.Println("=== Handoff Dry Run ===")
		fmt.Println()
		fmt.Println("Would collect the following context:")
		fmt.Println()
		fmt.Println(context)
		fmt.Println("Would:")
		fmt.Println("  1. Update/create handoff bead with above content")
		fmt.Println("  2. Write handoff marker to", filepath.Join(cfg.ConfigDir(), RuntimeDir, HandoffMarkerFile))
		fmt.Println("  3. Clear tmux history")
		fmt.Println("  4. Respawn Claude via tmux respawn-pane")
		return result, nil
	}

	// 2. Update or create handoff bead
	if err := updateHandoffBead(context); err != nil {
		// Non-fatal - continue with handoff even if bead update fails
		fmt.Fprintf(os.Stderr, "Warning: could not update handoff bead: %v\n", err)
	} else {
		result.BeadUpdated = true
	}

	// 3. Write handoff marker
	if err := writeMarker(cfg); err != nil {
		return nil, fmt.Errorf("writing marker: %w", err)
	}
	result.MarkerWritten = true

	// 4. Log hub handoff event for seance
	claudeSession := getClaudeSession()
	if claudeSession != "" {
		logger := events.NewLogger(cfg)
		if err := logger.LogHubHandoff(claudeSession, opts.Message); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not log hub handoff: %v\n", err)
		}
	}

	// 5. Clear tmux history (if in tmux)
	clearTmuxHistory()

	// 6. Respawn Claude via tmux
	if err := respawnClaude(cfg); err != nil {
		return nil, fmt.Errorf("respawning Claude: %w", err)
	}

	return result, nil
}

// collectContext gathers state information for the handoff
func collectContext(cfg *config.Config, opts *Options) (string, error) {
	var sb strings.Builder

	// Add timestamp
	sb.WriteString("## Handoff Context\n")
	sb.WriteString(fmt.Sprintf("Time: %s\n\n", time.Now().Format(time.RFC3339)))

	// Add custom message if provided
	if opts.Message != "" {
		sb.WriteString("### Notes\n")
		sb.WriteString(opts.Message)
		sb.WriteString("\n\n")
	}

	// Auto-collect state if requested
	if opts.AutoCollect {
		// Get active sessions
		state, err := session.LoadState(cfg)
		if err == nil && len(state.Sessions) > 0 {
			sb.WriteString("### Active Sessions\n")
			for name, sess := range state.Sessions {
				sb.WriteString(fmt.Sprintf("- %s: %s (%s)\n", name, sess.Bead, sess.Project))
			}
			sb.WriteString("\n")
		}

		// Get ready beads
		readyBeads, err := bead.Ready()
		if err == nil && len(readyBeads) > 0 {
			sb.WriteString("### Ready Beads\n")
			for _, b := range readyBeads {
				sb.WriteString(fmt.Sprintf("- %s: %s (P%d)\n", b.ID, b.Title, b.Priority))
			}
			sb.WriteString("\n")
		}

		// Get in-progress beads
		inProgressBeads, err := bead.List("in_progress")
		if err == nil && len(inProgressBeads) > 0 {
			sb.WriteString("### In Progress\n")
			for _, b := range inProgressBeads {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", b.ID, b.Title))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// updateHandoffBead updates or creates the pinned handoff bead
func updateHandoffBead(content string) error {
	// Find existing handoff bead
	beadID, err := findHandoffBead()
	if err != nil {
		// Create new handoff bead
		beadID, err = createHandoffBead()
		if err != nil {
			return fmt.Errorf("creating handoff bead: %w", err)
		}
	}

	// Update the bead's description with the context
	return bead.UpdateDescription(beadID, content)
}

// findHandoffBead looks for an existing handoff bead
func findHandoffBead() (string, error) {
	// Search for bead with title "Hub Handoff"
	beads, err := bead.Search(HandoffBeadTitle)
	if err != nil {
		return "", err
	}

	for _, b := range beads {
		if b.Title == HandoffBeadTitle {
			return b.ID, nil
		}
	}

	return "", fmt.Errorf("handoff bead not found")
}

// createHandoffBead creates a new pinned handoff bead
func createHandoffBead() (string, error) {
	opts := &bead.CreateOptions{
		Description: "Persistent handoff context for hub sessions",
		Type:        "task",
		Priority:    4, // Low priority, just for tracking
	}

	return bead.Create(HandoffBeadTitle, opts)
}

// writeMarker writes the handoff marker file
func writeMarker(cfg *config.Config) error {
	// Get or create runtime directory
	runtimeDir := filepath.Join(cfg.ConfigDir(), RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("creating runtime dir: %w", err)
	}

	// Write marker with session info
	markerPath := filepath.Join(runtimeDir, HandoffMarkerFile)
	content := fmt.Sprintf("%s\n%s\n", time.Now().Format(time.RFC3339), getSessionName())

	return os.WriteFile(markerPath, []byte(content), 0644)
}

// getSessionName tries to get the current Claude session name
func getSessionName() string {
	// Try environment variable first
	if name := os.Getenv("CLAUDE_SESSION"); name != "" {
		return name
	}

	// Try to get from tmux window name
	cmd := exec.Command("tmux", "display-message", "-p", "#W")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	return "unknown"
}

// getClaudeSession returns the Claude session ID for seance resumption
func getClaudeSession() string {
	// Claude Code sets this environment variable
	if id := os.Getenv("CLAUDE_SESSION_ID"); id != "" {
		return id
	}
	// Fallback to session name which can also be used
	return getSessionName()
}

// clearTmuxHistory clears the tmux pane history
func clearTmuxHistory() {
	exec.Command("tmux", "clear-history").Run()
}

// respawnClaude respawns Claude in the current tmux pane
func respawnClaude(cfg *config.Config) error {
	// Check if we're in tmux
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("not in a tmux session - cannot respawn")
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = os.Getenv("HOME")
	}

	// Use EditorCmd from config (defaults to "claude --dangerously-skip-permissions")
	editorCmd := cfg.EditorCmd
	if editorCmd == "" {
		editorCmd = "claude"
	}

	// Build respawn command - start fresh Claude with same flags
	respawnCmd := fmt.Sprintf("cd %s && exec %s", cwd, editorCmd)

	// Use tmux respawn-pane to replace current process
	cmd := exec.Command("tmux", "respawn-pane", "-k", respawnCmd)
	return cmd.Run()
}

// CheckMarker checks if a handoff marker exists and returns its content
func CheckMarker(cfg *config.Config) (exists bool, prevSession string, handoffTime time.Time, err error) {
	markerPath := filepath.Join(cfg.ConfigDir(), RuntimeDir, HandoffMarkerFile)

	data, err := os.ReadFile(markerPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", time.Time{}, nil
		}
		return false, "", time.Time{}, err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) >= 1 {
		handoffTime, _ = time.Parse(time.RFC3339, lines[0])
	}
	if len(lines) >= 2 {
		prevSession = lines[1]
	}

	return true, prevSession, handoffTime, nil
}

// ClearMarker removes the handoff marker file
func ClearMarker(cfg *config.Config) error {
	markerPath := filepath.Join(cfg.ConfigDir(), RuntimeDir, HandoffMarkerFile)
	err := os.Remove(markerPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// GetHandoffContent retrieves the content from the handoff bead
func GetHandoffContent() (string, error) {
	beadID, err := findHandoffBead()
	if err != nil {
		return "", nil // No handoff bead is not an error
	}

	info, err := bead.ShowFull(beadID)
	if err != nil {
		return "", err
	}

	return info.Description, nil
}

// ClearHandoffContent clears the handoff bead content
func ClearHandoffContent() error {
	beadID, err := findHandoffBead()
	if err != nil {
		return nil // No handoff bead to clear
	}

	return bead.UpdateDescription(beadID, "")
}

// IsInTmux checks if we're running inside tmux
func IsInTmux() bool {
	return os.Getenv("TMUX") != ""
}

// RunBdCommand runs a bd command and returns its output
func RunBdCommand(args ...string) (string, error) {
	cmd := exec.Command("bd", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
