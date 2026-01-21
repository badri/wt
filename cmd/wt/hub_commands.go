package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/handoff"
	"github.com/badri/wt/internal/hub"
	"github.com/badri/wt/internal/monitor"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/tmux"
)

// cmdHub creates or attaches to the dedicated hub session
func cmdHub(cfg *config.Config, args []string) error {
	opts := parseHubFlags(args)
	return hub.Run(cfg, opts)
}

// cmdHubHelp shows detailed help for the hub command
func cmdHubHelp() error {
	help := `wt hub - Manage the hub orchestration session

USAGE:
    wt hub [options]

DESCRIPTION:
    The hub is a dedicated tmux session for orchestrating worker sessions.
    It includes a watch pane showing live status of all workers.

OPTIONS:
    (none)              Create hub (with watch) or attach to existing hub
    -w, --watch         Add watch pane when attaching to existing hub
    --no-watch          Create hub without watch pane
    -d, --detach        Detach from hub (return to previous session)
    -s, --status        Show hub status without attaching
    -k, --kill          Kill the hub session
    -f, --force         Skip confirmation when killing
    -h, --help          Show this help

EXAMPLES:
    wt hub                  Create or attach to hub
    wt hub --no-watch       Create hub without watch pane
    wt hub --watch          Attach and add watch pane if missing
    wt hub --status         Check if hub is running
    wt hub --kill           Terminate hub session
    wt hub --detach         Switch back to previous session
`
	fmt.Print(help)
	return nil
}

// hasHelpFlag checks if args contain -h or --help
func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func parseHubFlags(args []string) *hub.Options {
	opts := &hub.Options{}
	for _, arg := range args {
		switch arg {
		case "-d", "--detach":
			opts.Detach = true
		case "-s", "--status":
			opts.Status = true
		case "-k", "--kill":
			opts.Kill = true
		case "-f", "--force":
			opts.Force = true
		case "-w", "--watch":
			opts.Watch = true
		case "--no-watch":
			opts.NoWatch = true
		}
	}
	return opts
}

// cmdWatchHelp shows detailed help for the watch command
func cmdWatchHelp() error {
	help := `wt watch - Live dashboard of all worker sessions

USAGE:
    wt watch [options]

DESCRIPTION:
    Shows a real-time TUI dashboard displaying all active worker sessions
    with their status, bead, project, and idle time.

    When run from the hub session, the TUI runs directly.
    When run from a worker session, it opens as a tmux popup overlay.

KEYBOARD:
    ↑/k, ↓/j           Navigate between sessions
    Enter              Switch to selected session (watch keeps running)
    r                  Refresh session list
    q, Ctrl+C          Quit watch

STATUS INDICATORS:
    ● green            Working - actively processing
    ● yellow           Idle - no recent activity
    ● bright green     Ready - waiting for review
    ● red              Blocked or Error

OPTIONS:
    -h, --help         Show this help

EXAMPLES:
    wt watch           Start the watch dashboard
    wt hub --watch     Attach to hub and ensure watch pane exists
`
	fmt.Print(help)
	return nil
}

// cmdWatch displays a live dashboard of all sessions using the TUI.
// When run from a worker session (not hub), it uses tmux popup to show the watch.
func cmdWatch(cfg *config.Config) error {
	// If we're already in a popup context, run directly
	if os.Getenv("WT_WATCH_POPUP") == "1" {
		return cmdWatchTUI(cfg)
	}

	// Not in tmux at all - run TUI directly
	if os.Getenv("TMUX") == "" {
		return cmdWatchTUI(cfg)
	}

	// Check if we're in the hub session
	if hub.IsInHub() {
		// In hub - run TUI directly
		return cmdWatchTUI(cfg)
	}

	// Check if we're in a wt worker session
	state, err := session.LoadState(cfg)
	if err != nil {
		// Can't determine - run TUI directly
		return cmdWatchTUI(cfg)
	}

	// Get current tmux session name
	currentSession := tmux.CurrentSession()
	if currentSession == "" {
		// Not in a named session - run TUI directly
		return cmdWatchTUI(cfg)
	}

	// Check if current session is a wt worker
	if _, isWorker := state.Sessions[currentSession]; isWorker {
		// In worker session - use tmux popup for overlay
		return cmdWatchPopup()
	}

	// In some other tmux session (not wt-related) - run TUI directly
	return cmdWatchTUI(cfg)
}

// cmdWatchPopup shows watch in a tmux popup overlay
func cmdWatchPopup() error {
	// Use tmux popup to show watch as a floating overlay
	// -E closes popup when command exits
	// -w and -h set the size
	// Set WT_WATCH_POPUP to prevent recursion
	cmd := exec.Command("tmux", "popup", "-E", "-w", "50%", "-h", "80%", "-e", "WT_WATCH_POPUP=1", "wt", "watch")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cmdWatchLegacy displays the old-style dashboard (kept for reference)
func cmdWatchLegacy(cfg *config.Config) error {
	const refreshInterval = 2 * time.Second
	const idleThreshold = 5 // minutes

	// Track previous states for notifications
	prevStates := make(map[string]string)
	prevPRStates := make(map[string]string)
	prevSessions := make(map[string]bool)

	for {
		// Clear screen
		fmt.Print("\033[H\033[2J")

		state, err := session.LoadState(cfg)
		if err != nil {
			return err
		}

		// Check for ended sessions (were in prev, not in current)
		currentSessions := make(map[string]bool)
		for name := range state.Sessions {
			currentSessions[name] = true
		}
		for name := range prevSessions {
			if !currentSessions[name] {
				monitor.Notify("wt: Session Ended", fmt.Sprintf("Session '%s' has completed", name))
			}
		}

		now := time.Now().Format("15:04:05")
		fmt.Printf("┌─ wt watch ─────────────────────────────────────────────── %s ─┐\n", now)
		fmt.Println("│                                                                       │")

		if len(state.Sessions) == 0 {
			fmt.Println("│  No active sessions.                                                 │")
			fmt.Println("│                                                                       │")
			fmt.Println("│  Start one with: wt new <bead>                                       │")
		} else {
			fmt.Printf("│  %-3s %-10s %-18s %-8s %-6s %-10s %-6s │\n",
				"", "Name", "Bead", "Status", "Idle", "PR", "Project")
			fmt.Printf("│  %-3s %-10s %-18s %-8s %-6s %-10s %-6s │\n",
				"", "────", "────", "──────", "────", "──", "───────")

			for name, sess := range state.Sessions {
				// Use session status if set, otherwise detect from tmux
				status := sess.Status
				if status == "" {
					status = monitor.DetectStatus(name, idleThreshold)
				}
				idleMin := monitor.GetIdleMinutes(name)

				// Get PR status
				prStatus, _ := monitor.GetPRStatus(sess.Worktree, sess.Branch)

				// Format idle time
				idleStr := "-"
				if idleMin >= 0 {
					if idleMin < 60 {
						idleStr = fmt.Sprintf("%dm", idleMin)
					} else {
						idleStr = fmt.Sprintf("%dh%dm", idleMin/60, idleMin%60)
					}
				}

				// Format PR/message status - prefer status message if set
				prStr := "-"
				if sess.StatusMessage != "" {
					prStr = sess.StatusMessage
				} else if prStatus != "none" && prStatus != "" {
					prStr = prStatus
				}

				statusIcon := getStatusIcon(status)
				prIcon := monitor.PRStatusIcon(prStatus)
				if sess.StatusMessage != "" {
					prIcon = "" // Don't show PR icon when we have a message
				}

				fmt.Printf("│  %s %-10s %-18s %-8s %-6s %s %-8s %-6s │\n",
					statusIcon,
					truncate(name, 10),
					truncate(sess.Bead, 18),
					status,
					idleStr,
					prIcon,
					truncate(prStr, 8),
					truncate(sess.Project, 6))

				// Send notification on status change
				prevStatus, exists := prevStates[name]
				if exists && prevStatus != status {
					if status == "ready" {
						msg := fmt.Sprintf("Session '%s' is ready for review", name)
						if sess.StatusMessage != "" {
							msg = fmt.Sprintf("Session '%s': %s", name, sess.StatusMessage)
						}
						monitor.Notify("wt: Ready for Review", msg)
					} else if status == "idle" {
						monitor.Notify("wt: Session Idle", fmt.Sprintf("Session '%s' is now idle", name))
					} else if status == "error" {
						monitor.Notify("wt: Session Error", fmt.Sprintf("Session '%s' has an error", name))
					} else if status == "blocked" {
						msg := fmt.Sprintf("Session '%s' is blocked", name)
						if sess.StatusMessage != "" {
							msg = fmt.Sprintf("Session '%s': %s", name, sess.StatusMessage)
						}
						monitor.Notify("wt: Session Blocked", msg)
					}
				}
				prevStates[name] = status

				// Send notification on PR merge
				prevPR := prevPRStates[name]
				if prevPR != "" && prevPR != "merged" && prStatus == "merged" {
					monitor.Notify("wt: PR Merged", fmt.Sprintf("PR for '%s' has been merged", name))
				}
				prevPRStates[name] = prStatus
			}
		}

		// Update previous sessions for next iteration
		prevSessions = currentSessions

		fmt.Println("│                                                                       │")
		fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
		fmt.Println("\nPress Ctrl+C to exit")

		time.Sleep(refreshInterval)
	}
}

// cmdSeance allows talking to past sessions
func cmdSeance(cfg *config.Config, args []string) error {
	eventLogger := events.NewLogger(cfg)

	// No args - list recent sessions
	if len(args) == 0 {
		return cmdSeanceList(cfg, eventLogger)
	}

	// Parse flags
	sessionName := args[0]
	var prompt string
	var spawn bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-p":
			if i+1 < len(args) {
				prompt = args[i+1]
				i++
			}
		case "--spawn":
			spawn = true
		}
	}

	// Find the session
	event, err := eventLogger.FindSession(sessionName)
	if err != nil {
		return err
	}

	if event.ClaudeSession == "" {
		return fmt.Errorf("session '%s' has no Claude session ID recorded", sessionName)
	}

	if prompt != "" {
		// One-shot query
		return cmdSeanceQuery(event, prompt)
	}

	if spawn {
		// Spawn new tmux session for seance
		return cmdSeanceSpawn(cfg, event)
	}

	// Resume session (opens in new pane)
	return cmdSeanceResume(cfg, event)
}

func cmdSeanceList(cfg *config.Config, logger *events.Logger) error {
	sessions, err := logger.RecentSessions(10)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		printEmptyMessage("No past sessions found.", "Sessions are recorded when they end via 'wt done', 'wt close', or 'wt handoff'.")
		return nil
	}

	// Define columns (matching wt list style)
	columns := []table.Column{
		{Title: "", Width: 2},
		{Title: "Session", Width: 18},
		{Title: "Title", Width: 36},
		{Title: "Project", Width: 14},
		{Title: "Time", Width: 16},
	}

	// Cache project configs to avoid repeated lookups
	projectMgr := project.NewManager(cfg)

	// Build rows
	var rows []table.Row
	for _, sess := range sessions {
		t, _ := time.Parse(time.RFC3339, sess.Time)
		timeStr := t.Format("2006-01-02 15:04")

		// Determine icon based on session type
		icon := "  "
		if sess.ClaudeSession != "" {
			if sess.Type == events.EventHubHandoff {
				icon = "🏠"
			} else {
				icon = "⚙️"
			}
		}

		// Get title - for workers, fetch from bead; for hub, leave empty
		title := ""
		projectDisplay := sess.Project
		if sess.Type == events.EventHubHandoff {
			projectDisplay = ""
		} else if sess.Bead != "" {
			// Try to get bead title from the correct project's beads dir
			beadsDir := ""
			if proj, err := projectMgr.Get(sess.Project); err == nil && proj != nil {
				beadsDir = proj.Repo + "/.beads"
			}
			if beadInfo, _ := bead.ShowInDir(sess.Bead, beadsDir); beadInfo != nil && beadInfo.Title != "" {
				title = beadInfo.Title
			} else {
				title = sess.Bead // Fallback to bead ID
			}
		}

		rows = append(rows, table.Row{
			icon,
			truncate(sess.Session, 18),
			truncate(title, 36),
			truncate(projectDisplay, 14),
			timeStr,
		})
	}

	printTable("Past Sessions (seance)", columns, rows)
	fmt.Println("\n⚙️ = Worker session   🏠 = Hub session")
	fmt.Println("\nCommands:")
	fmt.Println("  wt seance <name>          Resume in new pane (safe from hub)")
	fmt.Println("  wt seance <name> --spawn  Spawn new tmux session")
	fmt.Println("  wt seance <name> -p 'q'   One-shot query")

	return nil
}

func cmdSeanceResume(cfg *config.Config, event *events.Event) error {
	if event.Type == events.EventHubHandoff {
		fmt.Printf("Resuming hub session in new pane...\n")
	} else {
		fmt.Printf("Resuming '%s' (bead: %s) in new pane...\n", event.Session, event.Bead)
	}

	// Use EditorCmd from config (defaults to "claude --dangerously-skip-permissions")
	editorCmd := cfg.EditorCmd
	if editorCmd == "" {
		editorCmd = "claude"
	}

	// Check if we're in tmux
	if os.Getenv("TMUX") == "" {
		// Not in tmux - fall back to direct resume (replaces current process)
		fmt.Println("Note: Not in tmux, resuming in current terminal")
		// Split editorCmd and add --resume flag
		args := strings.Fields(editorCmd)
		args = append(args, "--resume", event.ClaudeSession)
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Open in new tmux window and run claude --resume
	resumeCmd := fmt.Sprintf("%s --resume %s", editorCmd, event.ClaudeSession)
	cmd := exec.Command("tmux", "new-window", "-n", "seance", resumeCmd)
	return cmd.Run()
}

func cmdSeanceSpawn(cfg *config.Config, event *events.Event) error {
	// Generate session name - prefix with "seance-" to avoid conflicts
	sessionName := "seance-" + event.Session
	if event.Type == events.EventHubHandoff {
		// Use timestamp for hub seances to allow multiple past hubs
		timestamp := time.Now().Format("20060102-1504")
		sessionName = "seance-hub-" + timestamp
	}

	// Check if session already exists (for hub, this is unlikely due to timestamp)
	if tmux.SessionExists(sessionName) {
		fmt.Printf("Seance session '%s' already exists. Switching to it.\n", sessionName)
		return tmux.Attach(sessionName)
	}

	// Get working directory
	workdir := os.Getenv("HOME")
	if event.WorktreePath != "" {
		workdir = event.WorktreePath
	}

	fmt.Printf("Spawning seance session '%s' for past session '%s'...\n", sessionName, event.Session)

	// Use EditorCmd from config (defaults to "claude --dangerously-skip-permissions")
	editorCmd := cfg.EditorCmd
	if editorCmd == "" {
		editorCmd = "claude"
	}

	// Create the seance session
	if err := tmux.NewSeanceSession(sessionName, workdir, editorCmd, event.ClaudeSession, true); err != nil {
		return err
	}

	return nil
}

func cmdSeanceQuery(event *events.Event, prompt string) error {
	fmt.Printf("Querying Claude session for '%s'...\n", event.Session)

	// Run claude with --resume and --print for one-shot
	cmd := exec.Command("claude", "--resume", event.ClaudeSession, "--print", prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// cmdHandoff performs a session handoff to a fresh Claude instance
func cmdHandoff(cfg *config.Config, args []string) error {
	opts := parseHandoffFlags(args)

	// Check if we're in tmux
	if !handoff.IsInTmux() {
		return fmt.Errorf("handoff requires running inside a tmux session")
	}

	fmt.Println("Performing handoff to fresh Claude instance...")

	result, err := handoff.Run(cfg, opts)
	if err != nil {
		return err
	}

	if opts.DryRun {
		return nil
	}

	if result.BeadUpdated {
		fmt.Println("  ✓ Handoff bead updated")
	}
	if result.MarkerWritten {
		fmt.Println("  ✓ Handoff marker written")
	}

	fmt.Println("\nRespawning Claude...")
	return nil
}

func parseHandoffFlags(args []string) *handoff.Options {
	opts := &handoff.Options{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-m", "--message":
			if i+1 < len(args) {
				opts.Message = args[i+1]
				i++
			}
		case "-c", "--collect":
			opts.AutoCollect = true
		case "--dry-run":
			opts.DryRun = true
		}
	}
	return opts
}

// cmdPrime injects context on session startup
func cmdPrime(cfg *config.Config, args []string) error {
	opts := parsePrimeFlags(args)

	result, err := handoff.Prime(cfg, opts)
	if err != nil {
		return err
	}

	handoff.OutputPrimeResult(result)

	// Archive handoff file after displaying (renames handoff.md to handoff-<timestamp>.md)
	if result.HandoffContent != "" {
		if err := handoff.ClearHandoffContent(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not archive handoff file: %v\n", err)
		}
	}

	return nil
}

func parsePrimeFlags(args []string) *handoff.PrimeOptions {
	opts := &handoff.PrimeOptions{}
	for _, arg := range args {
		switch arg {
		case "-q", "--quiet":
			opts.Quiet = true
		case "--no-bd-prime":
			opts.NoBdPrime = true
		}
	}
	return opts
}
