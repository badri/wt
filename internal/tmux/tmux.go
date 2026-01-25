package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// nudgeMutex serializes NudgeSession calls to prevent interleaved keystrokes
// when multiple goroutines try to send messages to sessions simultaneously.
var nudgeMutex sync.Mutex

// SessionOptions contains optional configuration for creating a tmux session.
type SessionOptions struct {
	PortOffset int
	PortEnv    string // defaults to PORT_OFFSET if empty
}

func NewSession(name, workdir, beadsDir, editorCmd string, opts *SessionOptions) error {
	// Check if session already exists
	if SessionExists(name) {
		return fmt.Errorf("tmux session '%s' already exists", name)
	}

	// Build command arguments for tmux new-session
	// Use direct command execution to eliminate send-keys race condition
	args := []string{
		"new-session",
		"-d",       // detached
		"-s", name, // session name
		"-c", workdir, // working directory
		"-e", fmt.Sprintf("BEADS_DIR=%s", beadsDir), // environment vars via -e flag
		"-e", fmt.Sprintf("WT_SESSION=%s", name),
	}

	// Add PORT_OFFSET if configured
	if opts != nil && opts.PortOffset > 0 {
		portEnv := opts.PortEnv
		if portEnv == "" {
			portEnv = "PORT_OFFSET"
		}
		args = append(args, "-e", fmt.Sprintf("%s=%d", portEnv, opts.PortOffset))
	}

	// If editorCmd is provided, run it directly as the pane process
	// This eliminates the race condition where send-keys might arrive before shell is ready
	if editorCmd != "" {
		args = append(args, editorCmd)
	}

	cmd := exec.Command("tmux", args...)
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	return nil
}

// NewSeanceSession creates a tmux session for resuming a past Claude conversation.
// It runs editorCmd --resume in a new session, optionally switching to it.
// editorCmd should be the base command (e.g., "claude --dangerously-skip-permissions")
func NewSeanceSession(name, workdir, editorCmd, claudeSessionID string, switchTo bool) error {
	// Check if session already exists
	if SessionExists(name) {
		return fmt.Errorf("tmux session '%s' already exists", name)
	}

	// Build the full resume command
	resumeCmd := fmt.Sprintf("%s --resume %s", editorCmd, claudeSessionID)

	// Create tmux session with command running directly as the pane process
	// This eliminates the send-keys race condition
	cmd := exec.Command("tmux", "new-session",
		"-d",       // detached
		"-s", name, // session name
		"-c", workdir, // working directory
		resumeCmd, // run command directly
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Switch to the session if requested
	if switchTo {
		return Attach(name)
	}

	return nil
}

func Attach(name string) error {
	// Check if we're inside tmux
	if os.Getenv("TMUX") != "" {
		// Switch to the session
		cmd := exec.Command("tmux", "switch-client", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Attach to the session
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// SwitchClient switches the tmux client to a different session.
// Unlike Attach, this doesn't need to capture stdin/stdout since
// it's meant to be called from background processes like the watch TUI.
func SwitchClient(name string) error {
	cmd := exec.Command("tmux", "switch-client", "-t", name)
	return cmd.Run()
}

// NudgeSession sends a message to a tmux session with reliable delivery.
// Uses send-keys -l (literal mode) for reliable text entry.
// Serialized via mutex to prevent interleaved keystrokes from concurrent calls.
// Based on gastown's proven implementation.
func NudgeSession(session, message string) error {
	// Serialize access to prevent concurrent nudges from interleaving
	nudgeMutex.Lock()
	defer nudgeMutex.Unlock()

	// 1. Send text in literal mode (handles special characters)
	sendCmd := exec.Command("tmux", "send-keys", "-t", session, "-l", message)
	if err := sendCmd.Run(); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	// 2. Wait for text to appear in input
	time.Sleep(1 * time.Second)

	// 3. Send Enter to submit
	// Use empty string "" followed by Enter - more reliable than just "Enter"
	enterCmd := exec.Command("tmux", "send-keys", "-t", session, "", "Enter")
	if err := enterCmd.Run(); err != nil {
		return fmt.Errorf("sending Enter: %w", err)
	}

	return nil
}

// WaitForClaude waits for Claude to be running in the session.
// Returns nil when Claude is detected, or error on timeout.
func WaitForClaude(session string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.Command("tmux", "display-message", "-t", session, "-p", "#{pane_current_command}")
		output, err := cmd.Output()
		if err == nil {
			command := strings.TrimSpace(string(output))
			// Check if it's Claude (not a shell)
			if command == "claude" || command == "node" {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for Claude to start")
}

// CapturePane captures the visible content of a tmux session's pane.
// Returns up to 'lines' lines of content from the pane.
func CapturePane(session string, lines int) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", fmt.Sprintf("-%d", lines))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capturing pane: %w", err)
	}
	return string(output), nil
}

// AcceptBypassPermissionsWarning dismisses the Claude Code bypass permissions warning dialog.
// When Claude starts with --dangerously-skip-permissions, it shows a warning dialog that
// requires pressing Down arrow to select "Yes, I accept" and then Enter to confirm.
// This function checks if the warning is present before sending keys.
//
// Call this after WaitForClaude, but before sending any prompts.
func AcceptBypassPermissionsWarning(session string) error {
	// Wait for the dialog to potentially render
	time.Sleep(1 * time.Second)

	// Check if the bypass permissions warning is present
	content, err := CapturePane(session, 30)
	if err != nil {
		return err
	}

	// Look for the characteristic warning text
	if !strings.Contains(content, "Bypass Permissions mode") {
		// Warning not present, nothing to do
		return nil
	}

	// Press Down to select "Yes, I accept" (option 2)
	downCmd := exec.Command("tmux", "send-keys", "-t", session, "Down")
	if err := downCmd.Run(); err != nil {
		return fmt.Errorf("sending Down key: %w", err)
	}

	// Small delay to let selection update
	time.Sleep(200 * time.Millisecond)

	// Press Enter to confirm
	enterCmd := exec.Command("tmux", "send-keys", "-t", session, "Enter")
	if err := enterCmd.Run(); err != nil {
		return fmt.Errorf("sending Enter key: %w", err)
	}

	return nil
}

func Kill(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	return cmd.Run()
}

// GetSessionEnv gets an environment variable from a tmux session
func GetSessionEnv(session, varName string) string {
	cmd := exec.Command("tmux", "show-environment", "-t", session, varName)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Output is "VAR=value\n", extract the value
	line := strings.TrimSpace(string(output))
	if idx := strings.Index(line, "="); idx != -1 {
		return line[idx+1:]
	}
	return ""
}

func SessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// CurrentSession returns the name of the current tmux session, or empty string if not in tmux
func CurrentSession() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// No sessions is not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var sessions []string
	for _, line := range lines {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}
