// Package hub provides dedicated orchestration session management.
// The hub session is a persistent tmux session for managing worker sessions.
package hub

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/badri/wt/internal/config"
)

const (
	// HubSessionName is the name of the hub tmux session.
	HubSessionName = "hub"
)

// Options configures hub behavior.
type Options struct {
	Detach  bool // Detach from hub (return to previous session)
	Status  bool // Show hub status without attaching
	Kill    bool // Kill the hub session
	Force   bool // Skip confirmation prompt
	NoWatch bool // Don't start wt watch pane (on create)
	Watch   bool // Add wt watch pane (when attaching to existing hub)
}

// Status contains hub session status information.
type Status struct {
	Exists      bool   // Whether hub session exists
	Attached    bool   // Whether we're currently attached to hub
	WorkingDir  string // Hub's working directory
	WindowCount int    // Number of tmux windows
	CreatedAt   string // Session creation time (if available)
	CurrentPane string // Current pane info
}

// Run executes the hub command based on options.
func Run(cfg *config.Config, opts *Options) error {
	if opts.Status {
		return showStatus()
	}

	if opts.Kill {
		return killWithConfirmation(opts.Force)
	}

	if opts.Detach {
		return detach()
	}

	// Default: create or attach to hub
	return createOrAttach(cfg, opts)
}

// showStatus displays hub status without attaching.
func showStatus() error {
	status := GetStatus()

	if !status.Exists {
		fmt.Println("Hub session does not exist.")
		fmt.Println("\nCreate with: wt hub")
		return nil
	}

	fmt.Println("┌─ Hub Status ──────────────────────────────────────────────────────────┐")
	fmt.Println("│                                                                       │")

	statusStr := "exists (detached)"
	if status.Attached {
		statusStr = "exists (attached)"
	}
	fmt.Printf("│  Status:      %-55s │\n", statusStr)
	fmt.Printf("│  Working Dir: %-55s │\n", truncate(status.WorkingDir, 55))
	fmt.Printf("│  Windows:     %-55d │\n", status.WindowCount)
	if status.CurrentPane != "" {
		fmt.Printf("│  Current:     %-55s │\n", truncate(status.CurrentPane, 55))
	}
	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")

	if !status.Attached {
		fmt.Println("\nAttach with: wt hub")
	}

	return nil
}

// GetStatus returns the current hub status.
func GetStatus() Status {
	status := Status{}

	// Check if hub session exists
	cmd := exec.Command("tmux", "has-session", "-t", HubSessionName)
	if err := cmd.Run(); err != nil {
		return status
	}
	status.Exists = true

	// Check if we're currently in the hub session
	if tmuxEnv := os.Getenv("TMUX"); tmuxEnv != "" {
		currentSession := getCurrentSession()
		status.Attached = currentSession == HubSessionName
	}

	// Get working directory
	cmd = exec.Command("tmux", "display-message", "-t", HubSessionName, "-p", "#{pane_current_path}")
	if output, err := cmd.Output(); err == nil {
		status.WorkingDir = strings.TrimSpace(string(output))
	}

	// Get window count
	cmd = exec.Command("tmux", "list-windows", "-t", HubSessionName, "-F", "#{window_id}")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		status.WindowCount = len(lines)
	}

	// Get current pane info
	cmd = exec.Command("tmux", "display-message", "-t", HubSessionName, "-p", "#{pane_current_command}")
	if output, err := cmd.Output(); err == nil {
		status.CurrentPane = strings.TrimSpace(string(output))
	}

	return status
}

// createOrAttach creates a new hub session or attaches to existing one.
func createOrAttach(cfg *config.Config, opts *Options) error {
	if Exists() {
		// Hub exists - optionally add watch pane before attaching
		if opts.Watch {
			if err := addWatchPane(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not add watch pane: %v\n", err)
			}
		}
		return attach()
	}

	// Create new hub session
	return create(cfg, opts)
}

// create creates a new hub tmux session.
func create(cfg *config.Config, opts *Options) error {
	// Initialize hub-level beads store
	if err := InitHubBeads(cfg); err != nil {
		// Non-fatal - hub can work without beads
		fmt.Fprintf(os.Stderr, "Warning: could not initialize hub beads: %v\n", err)
	}

	// Use home directory as working directory
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/"
	}

	// Create detached tmux session with WT_HUB=1 set from the start
	// Using -e flag ensures the shell inherits the env var immediately
	cmd := exec.Command("tmux", "new-session",
		"-d",                 // detached
		"-s", HubSessionName, // session name
		"-c", homeDir, // working directory
		"-e", "WT_HUB=1", // mark as hub session for child processes
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating hub session: %w", err)
	}

	// Get editor command from config
	editorCmd := cfg.EditorCmd
	if editorCmd != "" {
		// Append hub context prompt so Claude knows it's in hub mode
		hubPrompt := `You are in the **hub session**. For queries about ready/available work, use ` + "`wt ready`" + ` (not bd ready) to see work across ALL registered projects. Prefer /wt skill over /beads:ready in this context.`

		// Check if there's pending handoff content to pick up
		handoffPath := filepath.Join(cfg.ConfigDir(), "handoff.md")
		if _, err := os.Stat(handoffPath); err == nil {
			// Add handoff prompt so Claude reads the context
			handoffPrompt := "A handoff just occurred. IMPORTANT: First read the handoff context file at ~/.config/wt/handoff.md to understand what was happening in the previous session, then acknowledge the handoff to the user."
			hubPrompt = hubPrompt + " " + handoffPrompt
		}

		fullCmd := fmt.Sprintf("%s --append-system-prompt %q", editorCmd, hubPrompt)

		// Send the editor command to start
		// Prefix with space to avoid shell history
		sendCmd := exec.Command("tmux", "send-keys", "-t", HubSessionName, " "+fullCmd, "Enter")
		if err := sendCmd.Run(); err != nil {
			return fmt.Errorf("starting editor in hub: %w", err)
		}
	}

	// Create a right-side pane for wt watch (unless --no-watch)
	if !opts.NoWatch {
		splitCmd := exec.Command("tmux", "split-window", "-h", "-t", HubSessionName, "-l", "25%", "-c", homeDir)
		if err := splitCmd.Run(); err != nil {
			// Non-fatal - watch pane is optional
			fmt.Fprintf(os.Stderr, "Warning: could not create watch pane: %v\n", err)
		} else {
			// Start wt watch in a loop so it restarts if user quits
			// This ensures the watch pane stays active
			// Prefix with space to avoid shell history
			watchCmd := exec.Command("tmux", "send-keys", "-t", HubSessionName+".1", " while true; do wt watch; sleep 1; done", "Enter")
			_ = watchCmd.Run() // Non-fatal if this fails

			// Focus back on the main pane (pane 0)
			focusCmd := exec.Command("tmux", "select-pane", "-t", HubSessionName+".0")
			_ = focusCmd.Run()
		}
	}

	fmt.Printf("Created hub session.\n")
	fmt.Printf("  Working directory: %s\n", homeDir)
	if !opts.NoWatch {
		fmt.Printf("  Watch dashboard: right pane\n")
	}

	// Attach to the new session
	return attach()
}

// addWatchPane adds a watch pane to an existing hub session.
func addWatchPane() error {
	// Check how many panes exist
	cmd := exec.Command("tmux", "list-panes", "-t", HubSessionName, "-F", "#{pane_id}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("listing panes: %w", err)
	}

	panes := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(panes) > 1 {
		// Already has multiple panes, assume watch exists
		fmt.Println("Watch pane may already exist (hub has multiple panes)")
		return nil
	}

	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/"
	}

	// Create watch pane
	splitCmd := exec.Command("tmux", "split-window", "-h", "-t", HubSessionName, "-l", "25%", "-c", homeDir)
	if err := splitCmd.Run(); err != nil {
		return fmt.Errorf("creating watch pane: %w", err)
	}

	// Start wt watch in a loop
	// Prefix with space to avoid shell history
	watchCmd := exec.Command("tmux", "send-keys", "-t", HubSessionName+".1", " while true; do wt watch; sleep 1; done", "Enter")
	_ = watchCmd.Run()

	// Focus back on main pane
	focusCmd := exec.Command("tmux", "select-pane", "-t", HubSessionName+".0")
	_ = focusCmd.Run()

	fmt.Println("Added watch pane to hub")
	return nil
}

// attach attaches to the hub session.
func attach() error {
	// Check if we're inside tmux
	if os.Getenv("TMUX") != "" {
		// Switch to the hub session
		cmd := exec.Command("tmux", "switch-client", "-t", HubSessionName)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Attach to the session
	cmd := exec.Command("tmux", "attach-session", "-t", HubSessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// detach detaches from hub and returns to previous session.
func detach() error {
	// Check if we're in tmux
	if os.Getenv("TMUX") == "" {
		return fmt.Errorf("not in a tmux session - cannot detach")
	}

	// Check if we're in the hub session
	currentSession := getCurrentSession()
	if currentSession != HubSessionName {
		return fmt.Errorf("not in hub session (current: %s)", currentSession)
	}

	// Get the last session to switch to
	lastSession := getLastSession()
	if lastSession == "" || lastSession == HubSessionName {
		// No previous session, just detach
		fmt.Println("No previous session to return to. Detaching...")
		cmd := exec.Command("tmux", "detach-client")
		return cmd.Run()
	}

	// Switch to last session
	fmt.Printf("Returning to session: %s\n", lastSession)
	cmd := exec.Command("tmux", "switch-client", "-t", lastSession)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Exists returns true if the hub session exists.
func Exists() bool {
	cmd := exec.Command("tmux", "has-session", "-t", HubSessionName)
	return cmd.Run() == nil
}

// IsInHub returns true if we're currently in the hub session.
func IsInHub() bool {
	if os.Getenv("TMUX") == "" {
		return false
	}
	return getCurrentSession() == HubSessionName
}

// Kill terminates the hub session.
func Kill() error {
	if !Exists() {
		return fmt.Errorf("hub session does not exist")
	}

	cmd := exec.Command("tmux", "kill-session", "-t", HubSessionName)
	return cmd.Run()
}

// killWithConfirmation kills the hub session after showing implications and prompting.
func killWithConfirmation(force bool) error {
	if !Exists() {
		fmt.Println("Hub session does not exist.")
		return nil
	}

	// Check if we're currently in the hub
	inHub := IsInHub()

	// Show implications
	fmt.Println("WARNING: Killing the hub session will:")
	fmt.Println()
	fmt.Println("  - Terminate any running Claude session in the hub")
	fmt.Println("  - Lose any unsaved context or conversation history")
	fmt.Println("  - NOT affect worker sessions (they run independently)")
	fmt.Println()

	if inHub {
		fmt.Println("  NOTE: You are currently IN the hub session.")
		fmt.Println("        Your terminal will be disconnected.")
		fmt.Println()
	}

	// If not forced, prompt for confirmation
	if !force {
		fmt.Print("Are you sure you want to kill the hub? [y/N] ")

		var response string
		fmt.Scanln(&response)

		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Kill the hub
	if err := Kill(); err != nil {
		return fmt.Errorf("killing hub: %w", err)
	}

	fmt.Println("Hub session killed.")
	return nil
}

// getCurrentSession returns the name of the current tmux session.
func getCurrentSession() string {
	cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// getLastSession returns the name of the previously active session.
func getLastSession() string {
	// tmux stores last session in the session stack
	// We can get it via the client's last session
	cmd := exec.Command("tmux", "display-message", "-p", "#{client_last_session}")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetHubDir returns the path to hub-specific state directory.
func GetHubDir(cfg *config.Config) string {
	return filepath.Join(cfg.ConfigDir(), "hub")
}

// truncate truncates a string to max length with ellipsis.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
