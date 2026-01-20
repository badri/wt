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

	// HandoffFile is the current handoff context file
	HandoffFile = "handoff.md"

	// RuntimeDir is the directory for runtime state files
	RuntimeDir = ".wt"

	// SeanceHubPrefix is the prefix for seance hub sessions
	SeanceHubPrefix = "seance-hub-"
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

	// Determine handoff file path based on session type
	handoffPath := getHandoffFilePath(cfg)

	// Dry run - just show what would happen
	if opts.DryRun {
		fmt.Println("=== Handoff Dry Run ===")
		fmt.Println()
		fmt.Println("Would collect the following context:")
		fmt.Println()
		fmt.Println(context)
		fmt.Println("Would:")
		fmt.Println("  1. Write handoff context to", handoffPath)
		fmt.Println("  2. Write handoff marker to", filepath.Join(cfg.ConfigDir(), RuntimeDir, HandoffMarkerFile))
		fmt.Println("  3. Clear tmux history")
		fmt.Println("  4. Respawn Claude via tmux respawn-pane")
		return result, nil
	}

	// 2. Write handoff context to file
	if err := writeHandoffFile(handoffPath, context); err != nil {
		return nil, fmt.Errorf("writing handoff file: %w", err)
	}
	result.BeadUpdated = true // Reusing field to indicate file written

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

// getTmuxSessionName returns the current tmux session name
func getTmuxSessionName() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#S")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// isSeanceHubSession checks if we're in a seance hub session
func isSeanceHubSession() bool {
	sessionName := getTmuxSessionName()
	return strings.HasPrefix(sessionName, SeanceHubPrefix)
}

// getSeanceTimestamp extracts timestamp from seance-hub-YYYYMMDD-HHMM session name
func getSeanceTimestamp() string {
	sessionName := getTmuxSessionName()
	if strings.HasPrefix(sessionName, SeanceHubPrefix) {
		return strings.TrimPrefix(sessionName, SeanceHubPrefix)
	}
	return ""
}

// getHandoffFilePath returns the appropriate handoff file path
// For active hub: handoff.md
// For seance hub: handoff-<timestamp>.md
func getHandoffFilePath(cfg *config.Config) string {
	if isSeanceHubSession() {
		timestamp := getSeanceTimestamp()
		if timestamp != "" {
			return filepath.Join(cfg.ConfigDir(), fmt.Sprintf("handoff-%s.md", timestamp))
		}
	}
	return filepath.Join(cfg.ConfigDir(), HandoffFile)
}

// writeHandoffFile writes handoff context to the specified file
func writeHandoffFile(path, content string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Write content with header
	header := fmt.Sprintf("# Hub Handoff Context\n\nGenerated: %s\n\n---\n\n",
		time.Now().Format("2006-01-02 15:04:05"))
	fullContent := header + content

	return os.WriteFile(path, []byte(fullContent), 0644)
}

// ReadHandoffFile reads the current handoff.md file
func ReadHandoffFile(cfg *config.Config) (string, error) {
	path := filepath.Join(cfg.ConfigDir(), HandoffFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// ArchiveHandoffFile renames handoff.md to handoff-<timestamp>.md
func ArchiveHandoffFile(cfg *config.Config) error {
	src := filepath.Join(cfg.ConfigDir(), HandoffFile)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil // No file to archive
	}

	timestamp := time.Now().Format("20060102-1504")
	dst := filepath.Join(cfg.ConfigDir(), fmt.Sprintf("handoff-%s.md", timestamp))

	return os.Rename(src, dst)
}

// ListArchivedHandoffs returns list of archived handoff files
func ListArchivedHandoffs(cfg *config.Config) ([]string, error) {
	pattern := filepath.Join(cfg.ConfigDir(), "handoff-*.md")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	return matches, nil
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

// GetHandoffContent retrieves the content from handoff.md
func GetHandoffContent(cfg *config.Config) (string, error) {
	path := filepath.Join(cfg.ConfigDir(), HandoffFile)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // No handoff file is not an error
		}
		return "", err
	}
	return string(content), nil
}

// ClearHandoffContent archives the handoff file after consumption
func ClearHandoffContent(cfg *config.Config) error {
	return ArchiveHandoffFile(cfg)
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
