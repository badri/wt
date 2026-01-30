package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"

	"github.com/badri/wt/internal/auto"
	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/handoff"
	"github.com/badri/wt/internal/merge"
	"github.com/badri/wt/internal/namepool"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/testenv"
	"github.com/badri/wt/internal/tmux"
	"github.com/badri/wt/internal/worktree"
)

type newFlags struct {
	repo        string
	name        string
	project     string // Explicit project to use (for multi-branch repos)
	noSwitch    bool
	forceSwitch bool
	noTestEnv   bool
	shell       bool // Start with shell only, don't launch Claude
	noPrompt    bool // Start Claude but don't send initial prompt (for wt auto)
	force       bool // Override safety checks (e.g., epic guard)
}

// cmdNewHelp shows detailed help for the new command
func cmdNewHelp() error {
	help := `wt new - Create a new worker session for a bead

USAGE:
    wt new <bead-id> [options]

DESCRIPTION:
    Creates a git worktree, tmux session, and starts Claude Code
    to work on the specified bead.

ARGUMENTS:
    <bead-id>           The bead ID to work on (e.g., wt-123, myproject-abc)

OPTIONS:
    --repo <path>       Use specific repository path (default: auto-detect from bead)
    --name <name>       Custom session name (default: generated from namepool)
    -p, --project <name>  Use specific project (required when repo has multiple
                        branch registrations with same bead prefix)
    --no-switch         Don't switch to the new session after creation
    --switch            Force switch even when running from hub
    --no-test-env       Skip test environment setup
    --shell             Create session with shell only (don't start Claude)
    --no-prompt         Start Claude but don't send initial prompt (for wt auto)
    --force             Override safety checks (e.g., allow spawning on epics)
    -h, --help          Show this help

EXAMPLES:
    wt new wt-123                     Start working on bead wt-123
    wt new wt-123 --name mybranch     Use custom session name
    wt new wt-123 --no-switch         Create but stay in current session
    wt new proj-456 --repo ~/code/proj  Specify repo path
    wt new proj-456 -p proj-feature   Use project with specific branch config
`
	fmt.Print(help)
	return nil
}

// cmdListHelp shows help for the list command
func cmdListHelp() error {
	help := `wt list - List active sessions

USAGE:
    wt list [options]

DESCRIPTION:
    Shows all active worktree sessions with their status, bead, and duration.

OPTIONS:
    --all               Show all sessions including completed ones
    -h, --help          Show this help

EXAMPLES:
    wt list             List active sessions
    wt list --all       List all sessions including completed
`
	fmt.Print(help)
	return nil
}

// cmdKillHelp shows help for the kill command
func cmdKillHelp() error {
	help := `wt kill - Terminate a session

USAGE:
    wt kill <name> [options]

DESCRIPTION:
    Terminates the tmux session and optionally removes the worktree.
    The bead remains open for future work.

ARGUMENTS:
    <name>              Session name to kill

OPTIONS:
    --keep-worktree     Keep the git worktree (only kill tmux session)
    -h, --help          Show this help

EXAMPLES:
    wt kill mysession               Kill session and remove worktree
    wt kill mysession --keep-worktree  Kill session, keep worktree
`
	fmt.Print(help)
	return nil
}

// cmdCloseHelp shows help for the close command
func cmdCloseHelp() error {
	help := `wt close - Complete a session and close the bead

USAGE:
    wt close <name>

DESCRIPTION:
    Terminates the session, removes the worktree, and closes the bead.
    Use this when work on a bead is complete.

ARGUMENTS:
    <name>              Session name to close

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt close mysession  Complete and close the session
`
	fmt.Print(help)
	return nil
}

// cmdDoneHelp shows help for the done command
func cmdDoneHelp() error {
	help := `wt done - Complete current session with merge

USAGE:
    wt done [options]

DESCRIPTION:
    Completes work in the current session by:
    1. Committing any uncommitted changes
    2. Rebasing on the target branch
    3. Merging or creating a PR (based on merge mode)
    4. Closing the bead
    5. Cleaning up the session

OPTIONS:
    -m, --merge-mode <mode>  Merge mode: direct, pr-auto, pr-review
    -h, --help               Show this help

MERGE MODES:
    direct      Merge directly to the target branch
    pr-auto     Create PR and auto-merge if checks pass
    pr-review   Create PR for review (no auto-merge)

EXAMPLES:
    wt done                     Complete with default merge mode
    wt done --merge-mode direct Complete with direct merge
    wt done -m pr-review        Create PR for review
`
	fmt.Print(help)
	return nil
}

// cmdStatusHelp shows help for the status command
func cmdStatusHelp() error {
	help := `wt status - Show current session status

USAGE:
    wt status

DESCRIPTION:
    Displays detailed information about the current worktree session,
    including bead info, git status, and session metadata.

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt status           Show current session status
`
	fmt.Print(help)
	return nil
}

// cmdSignalHelp shows help for the signal command
func cmdSignalHelp() error {
	help := `wt signal - Update session status

USAGE:
    wt signal <status> [message]

DESCRIPTION:
    Updates the status of the current session. This is used to communicate
    progress to the hub or other monitoring tools.

ARGUMENTS:
    <status>            Status: ready, blocked, error, working, idle, bead-done

STATUS VALUES:
    ready       Work is complete, ready for review/merge
    blocked     Waiting on external dependency or decision
    error       An error occurred that needs attention
    working     Actively working on the task
    idle        Paused but not blocked
    bead-done   Bead completed in batch mode (include summary for next bead)

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt signal ready               Mark session as ready
    wt signal blocked "Waiting on API access"  Mark blocked with reason
    wt signal error "Tests failing"            Mark as error with message
    wt signal bead-done "Added new feature X with tests"  Batch bead complete
`
	fmt.Print(help)
	return nil
}

// cmdAbandonHelp shows help for the abandon command
func cmdAbandonHelp() error {
	help := `wt abandon - Abandon current session without merge

USAGE:
    wt abandon

DESCRIPTION:
    Abandons the current session without merging changes.
    The worktree is removed but the bead remains open.

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt abandon          Abandon current session
`
	fmt.Print(help)
	return nil
}

func parseNewFlags(args []string) (beadID string, flags newFlags) {
	beadID = args[0]
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			if i+1 < len(args) {
				flags.repo = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				flags.name = args[i+1]
				i++
			}
		case "-p", "--project":
			if i+1 < len(args) {
				flags.project = args[i+1]
				i++
			}
		case "--no-switch":
			flags.noSwitch = true
		case "--switch":
			flags.forceSwitch = true
		case "--no-test-env":
			flags.noTestEnv = true
		case "--shell":
			flags.shell = true
		case "--no-prompt":
			flags.noPrompt = true
		case "--force":
			flags.force = true
		}
	}
	return
}

type killFlags struct {
	keepWorktree bool
}

func parseKillFlags(args []string) killFlags {
	var flags killFlags
	for _, arg := range args {
		if arg == "--keep-worktree" {
			flags.keepWorktree = true
		}
	}
	return flags
}

type doneFlags struct {
	mergeMode string
	noRebase  bool
}

type listFlags struct {
	all     bool   // Include past sessions
	project string // Filter by project
	since   string // Filter by time (e.g., "1d", "1w")
	status  string // Filter by status (completed, killed, abandoned)
}

func parseListFlags(args []string) listFlags {
	var flags listFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all", "-a":
			flags.all = true
		case "--project", "-p":
			if i+1 < len(args) {
				flags.project = args[i+1]
				i++
			}
		case "--since", "-s":
			if i+1 < len(args) {
				flags.since = args[i+1]
				i++
			}
		case "--status":
			if i+1 < len(args) {
				flags.status = args[i+1]
				i++
			}
		}
	}
	return flags
}

func parseDoneFlags(args []string) doneFlags {
	var flags doneFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--merge-mode", "-m":
			if i+1 < len(args) {
				flags.mergeMode = args[i+1]
				i++
			}
		case "--no-rebase":
			flags.noRebase = true
		}
	}
	return flags
}

// SessionJSON is the JSON output format for a session
type SessionJSON struct {
	Name                string `json:"name"`
	Type                string `json:"type"` // "bead" or "task"
	Bead                string `json:"bead,omitempty"`
	TaskDescription     string `json:"task_description,omitempty"`
	CompletionCondition string `json:"completion_condition,omitempty"`
	Project             string `json:"project"`
	Worktree            string `json:"worktree"`
	Branch              string `json:"branch"`
	Status              string `json:"status"`
	StatusMessage       string `json:"status_message,omitempty"`
	CreatedAt           string `json:"created_at"`
	LastActivity        string `json:"last_activity"`
}

// ListSessionEntry represents a session for display in the list
type ListSessionEntry struct {
	Name      string
	Type      string // "bead", "task", or "past"
	Status    string
	Title     string
	Project   string
	CreatedAt string
	EndedAt   string // For past sessions
	Duration  string // Formatted duration
	IsPast    bool
	MergeMode string // For past sessions (how it ended)
}

func cmdList(cfg *config.Config, args []string) error {
	flags := parseListFlags(args)

	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	// Build unified list of sessions
	var entries []ListSessionEntry

	// Add active sessions
	for name, sess := range state.Sessions {
		// Apply project filter
		if flags.project != "" && sess.Project != flags.project {
			continue
		}

		// Apply since filter to active sessions
		if flags.since != "" {
			duration, err := parseDurationString(flags.since)
			if err == nil {
				if sess.CreatedAt != "" {
					createdAt, err := time.Parse(time.RFC3339, sess.CreatedAt)
					if err == nil && time.Since(createdAt) > duration {
						continue
					}
				}
			}
		}

		// Status filter doesn't apply to active sessions in "completed/killed/abandoned" mode
		if flags.status != "" && (flags.status == "completed" || flags.status == "killed" || flags.status == "abandoned") {
			continue
		}

		status := sess.Status
		if status == "" {
			status = "working"
		}

		// Determine type and title based on session type
		sessionType := "bead"
		title := ""
		if sess.IsTask() {
			sessionType = "task"
			title = sess.TaskDescription
		} else {
			// Get bead title (use BeadsDir to find correct project)
			if beadInfo, err := bead.ShowInDir(sess.Bead, sess.BeadsDir); err == nil && beadInfo != nil {
				title = beadInfo.Title
			}
		}

		// Calculate duration
		durationStr := ""
		if sess.CreatedAt != "" {
			durationStr = formatSessionDuration(sess.CreatedAt, "")
		}

		entries = append(entries, ListSessionEntry{
			Name:      name,
			Type:      sessionType,
			Status:    status,
			Title:     title,
			Project:   sess.Project,
			CreatedAt: sess.CreatedAt,
			Duration:  durationStr,
			IsPast:    false,
		})
	}

	// Add past sessions if --all flag is set
	if flags.all {
		eventLogger := events.NewLogger(cfg)
		var historyEvents []events.Event

		if flags.since != "" {
			duration, err := parseDurationString(flags.since)
			if err != nil {
				return fmt.Errorf("invalid duration format: %s (use 1d, 1w, 2h, etc.)", flags.since)
			}
			historyEvents, err = eventLogger.Since(duration)
			if err != nil {
				return fmt.Errorf("reading events: %w", err)
			}
		} else {
			// Default to last 100 events when no since filter
			historyEvents, err = eventLogger.Recent(100)
			if err != nil {
				return fmt.Errorf("reading events: %w", err)
			}
		}

		// Build a map of session start times from session_start events
		startTimes := make(map[string]string)
		for _, e := range historyEvents {
			if e.Type == events.EventSessionStart {
				startTimes[e.Session] = e.Time
			}
		}

		// Process session_end events for past sessions
		for _, e := range historyEvents {
			if e.Type != events.EventSessionEnd {
				continue
			}

			// Skip sessions that are still active
			if _, active := state.Sessions[e.Session]; active {
				continue
			}

			// Apply project filter
			if flags.project != "" && e.Project != flags.project {
				continue
			}

			// Determine status based on merge mode
			status := "completed"
			if e.MergeMode == "killed" {
				status = "killed"
			} else if e.MergeMode == "abandoned" {
				status = "abandoned"
			} else if e.MergeMode == "closed" {
				status = "closed"
			}

			// Apply status filter
			if flags.status != "" && status != flags.status {
				continue
			}

			// Calculate duration
			startTime := startTimes[e.Session]
			durationStr := formatSessionDuration(startTime, e.Time)

			title := e.Bead
			if e.PRURL != "" {
				title = e.Bead + " (PR)"
			}

			entries = append(entries, ListSessionEntry{
				Name:      e.Session,
				Type:      "past",
				Status:    status,
				Title:     title,
				Project:   e.Project,
				CreatedAt: startTime,
				EndedAt:   e.Time,
				Duration:  durationStr,
				IsPast:    true,
				MergeMode: e.MergeMode,
			})
		}
	}

	if len(entries) == 0 {
		if flags.all {
			printEmptyMessage("No sessions found.", "Try: wt list --all --since 1w")
		} else {
			printEmptyMessage("No active sessions.", "Commands: wt new <bead> | wt list --all")
		}
		return nil
	}

	// JSON output
	if outputJSON {
		type ListSessionJSON struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			Status    string `json:"status"`
			Title     string `json:"title"`
			Project   string `json:"project"`
			CreatedAt string `json:"created_at,omitempty"`
			EndedAt   string `json:"ended_at,omitempty"`
			Duration  string `json:"duration,omitempty"`
			IsPast    bool   `json:"is_past"`
			MergeMode string `json:"merge_mode,omitempty"`
		}
		var jsonEntries []ListSessionJSON
		for _, e := range entries {
			jsonEntries = append(jsonEntries, ListSessionJSON{
				Name:      e.Name,
				Type:      e.Type,
				Status:    e.Status,
				Title:     e.Title,
				Project:   e.Project,
				CreatedAt: e.CreatedAt,
				EndedAt:   e.EndedAt,
				Duration:  e.Duration,
				IsPast:    e.IsPast,
				MergeMode: e.MergeMode,
			})
		}
		printJSON(jsonEntries)
		return nil
	}

	// Define columns with Duration
	columns := []table.Column{
		{Title: "Name", Width: 18},
		{Title: "Type", Width: 6},
		{Title: "Status", Width: 10},
		{Title: "Duration", Width: 10},
		{Title: "Title", Width: 26},
		{Title: "Project", Width: 12},
	}

	// Build rows
	var rows []table.Row
	for _, entry := range entries {
		rows = append(rows, table.Row{
			entry.Name,
			entry.Type,
			entry.Status,
			entry.Duration,
			truncate(entry.Title, 26),
			truncate(entry.Project, 12),
		})
	}

	title := "Active Sessions"
	if flags.all {
		title = "Sessions (Active + Past)"
	}
	printTable(title, columns, rows)

	if flags.all {
		fmt.Println("\nCommands: wt <name> (switch) | wt seance <name> (resume past)")
	} else {
		fmt.Println("\nCommands: wt <name> (switch) | wt new <bead> | wt list --all")
	}

	return nil
}

// parseDurationString parses duration strings like "1d", "2w", "3h", "30m"
func parseDurationString(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	// Check for standard Go duration format first
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Parse custom formats: Nd (days), Nw (weeks)
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	numStr := s[:len(s)-1]
	unit := s[len(s)-1:]

	var num int
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	switch unit {
	case "d":
		return time.Duration(num) * 24 * time.Hour, nil
	case "w":
		return time.Duration(num) * 7 * 24 * time.Hour, nil
	case "h":
		return time.Duration(num) * time.Hour, nil
	case "m":
		return time.Duration(num) * time.Minute, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %s (use d, w, h, m)", unit)
	}
}

// formatSessionDuration formats the duration between start and end times
func formatSessionDuration(startTime, endTime string) string {
	if startTime == "" {
		return "-"
	}

	start, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return "-"
	}

	var end time.Time
	if endTime == "" {
		end = time.Now()
	} else {
		end, err = time.Parse(time.RFC3339, endTime)
		if err != nil {
			return "-"
		}
	}

	duration := end.Sub(start)

	// Handle negative or zero durations (shouldn't happen but be safe)
	if duration <= 0 {
		return "-"
	}

	// Format based on duration size
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		hours := int(duration.Hours())
		mins := int(duration.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("%dh%dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	} else {
		days := int(duration.Hours() / 24)
		hours := int(duration.Hours()) % 24
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
}

func cmdNew(cfg *config.Config, args []string) error {
	beadID, flags := parseNewFlags(args)

	// Load state
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	// Check if session already exists for this bead
	for name, sess := range state.Sessions {
		if sess.Bead == beadID {
			return fmt.Errorf("session '%s' already exists for bead %s", name, beadID)
		}
	}

	// Determine source repo and project config
	repoPath := flags.repo
	var proj *project.Project
	mgr := project.NewManager(cfg)

	// If explicit project specified, use it
	if flags.project != "" {
		var err error
		proj, err = mgr.Get(flags.project)
		if err != nil {
			return fmt.Errorf("project '%s' not found", flags.project)
		}
	} else {
		// Try to find project by bead prefix
		proj, _ = mgr.FindByBeadPrefix(beadID)

		// Check for multiple projects with same bead prefix (multi-branch repos)
		if proj != nil && proj.RepoURL != "" {
			matches, _ := mgr.FindByRepoURL(proj.RepoURL)
			if len(matches) > 1 {
				var projectNames []string
				for _, m := range matches {
					projectNames = append(projectNames, fmt.Sprintf("  %s (branch: %s)", m.Name, m.DefaultBranch))
				}
				return fmt.Errorf("multiple projects share bead prefix '%s'. Use -p <project> to specify:\n%s",
					proj.BeadsPrefix, strings.Join(projectNames, "\n"))
			}
		}
	}

	if repoPath == "" {
		if proj != nil {
			repoPath = proj.RepoPath()
		} else {
			// Fall back to current directory
			repoPath, err = worktree.FindGitRoot()
			if err != nil {
				return fmt.Errorf("not in a git repository and no project registered for bead prefix. Use --repo <path> or register a project")
			}
		}
	}

	// Validate bead exists and get full info (using project's beads directory if known)
	beadsDir := repoPath + "/.beads"
	beadInfo, err := bead.ShowFullInDir(beadID, beadsDir)
	if err != nil {
		return fmt.Errorf("bead not found: %s", beadID)
	}

	// Guard: block spawning workers on epics unless --force
	if beadInfo.IssueType == "epic" && !flags.force {
		return fmt.Errorf("cannot spawn worker for epic '%s'. Use one of:\n  wt auto --epic %s    # process all children sequentially\n  wt new <child-id>       # spawn a specific child bead\n  wt new %s --force    # override (advanced)", beadID, beadID, beadID)
	}

	// Allocate name from themed pool
	var pool *namepool.Pool
	projectName := ""
	if proj != nil {
		projectName = proj.Name
	} else if beadInfo.Project != "" {
		projectName = beadInfo.Project
	}

	if projectName != "" {
		// Use themed namepool based on project name
		pool, err = namepool.LoadForProject(projectName)
		if err != nil {
			return err
		}
		fmt.Printf("Using theme: %s\n", pool.Theme())
	} else {
		// Fall back to file-based namepool
		pool, err = namepool.Load(cfg)
		if err != nil {
			return err
		}
	}

	sessionName := flags.name
	var themeName string // Track allocated name for namepool deduplication
	if sessionName == "" {
		var err error
		themeName, err = pool.Allocate(state.UsedNames())
		if err != nil {
			return err
		}
		// Prefix with project name for easier identification
		if projectName != "" {
			sessionName = projectName + "-" + themeName
		} else {
			sessionName = themeName
		}
	}

	// Create worktree using bead ID to guarantee unique paths
	worktreePath := cfg.WorktreePath(beadID)
	fmt.Printf("Creating git worktree at %s...\n", worktreePath)

	// Determine base branch for worktree creation
	baseBranch := "main"
	if proj != nil && proj.DefaultBranch != "" {
		baseBranch = proj.DefaultBranch
	}

	// Create worktree from the project's base branch
	if err := worktree.CreateFromBranch(repoPath, worktreePath, beadID, baseBranch); err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}
	if baseBranch != "main" {
		fmt.Printf("  Created from branch: %s\n", baseBranch)
	}

	// Symlink .claude/ from main repo for project-specific configs (MCP servers, hooks, settings)
	if err := worktree.SymlinkClaudeDir(repoPath, worktreePath); err != nil {
		fmt.Printf("Warning: could not symlink .claude/: %v\n", err)
	}

	// beadsDir already set above when validating the bead

	// Allocate port offset if test env is configured
	var portOffset int
	var portEnv string
	if proj != nil && proj.TestEnv != nil {
		usedOffsets := collectUsedOffsets(state)
		portOffset = testenv.AllocatePortOffset(proj, usedOffsets)
		portEnv = proj.TestEnv.PortEnv
		if portEnv == "" {
			portEnv = "PORT_OFFSET"
		}
		fmt.Printf("Allocated %s=%d\n", portEnv, portOffset)
	}

	// Create tmux session
	fmt.Printf("Creating tmux session '%s'...\n", sessionName)
	tmuxOpts := &tmux.SessionOptions{
		PortOffset: portOffset,
		PortEnv:    portEnv,
	}
	// When --shell flag is set, don't start Claude (pass empty editorCmd)
	editorCmd := cfg.EditorCmd
	if flags.shell {
		editorCmd = ""
	}
	if err := tmux.NewSession(sessionName, worktreePath, beadsDir, editorCmd, tmuxOpts); err != nil {
		// Cleanup worktree on failure
		worktree.Remove(worktreePath)
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Run test env setup if configured and not skipped
	if proj != nil && proj.TestEnv != nil && proj.TestEnv.Setup != "" && !flags.noTestEnv {
		fmt.Println("Running test environment setup...")
		if err := testenv.RunSetup(proj, worktreePath, portOffset); err != nil {
			fmt.Printf("Warning: test env setup failed: %v\n", err)
		}

		// Wait for health check if configured
		if proj.TestEnv.HealthCheck != "" {
			fmt.Println("Waiting for test environment to be ready...")
			if err := testenv.WaitForHealthy(proj, worktreePath, portOffset, 30*time.Second); err != nil {
				fmt.Printf("Warning: health check failed: %v\n", err)
			}
		}
	} else if flags.noTestEnv && proj != nil && proj.TestEnv != nil {
		fmt.Println("Skipping test environment setup (--no-test-env)")
	}

	// Run on_create hooks if configured
	if proj != nil && proj.Hooks != nil && len(proj.Hooks.OnCreate) > 0 {
		fmt.Println("Running on_create hooks...")
		if err := testenv.RunOnCreateHooks(proj, worktreePath, portOffset, portEnv); err != nil {
			fmt.Printf("Warning: on_create hook failed: %v\n", err)
		}
	}

	// Determine project name (may have been set earlier for namepool)
	if projectName == "" {
		projectName = beadInfo.Project
		if proj != nil {
			projectName = proj.Name
		}
	}

	// Save session state
	sess := &session.Session{
		Bead:       beadID,
		Project:    projectName,
		Worktree:   worktreePath,
		Branch:     beadID,
		PortOffset: portOffset,
		BeadsDir:   beadsDir,
		Status:     "working",
		CreatedAt:  session.Now(),
		ThemeName:  themeName, // Track allocated name for namepool deduplication
	}
	sess.UpdateActivity()

	state.Sessions[sessionName] = sess
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Log session start event
	eventLogger := events.NewLogger(cfg)
	eventLogger.LogSessionStart(sessionName, beadID, projectName, worktreePath)

	fmt.Printf("\nSession '%s' ready.\n", sessionName)
	fmt.Printf("  Bead:     %s\n", beadID)
	fmt.Printf("  Worktree: %s\n", worktreePath)
	fmt.Printf("  Branch:   %s\n", beadID)

	// Skip Claude initialization when --shell flag is used
	if !flags.shell {
		// Wait for Claude to actually be running before sending prompt
		fmt.Println("Waiting for Claude to start...")
		if err := tmux.WaitForClaude(sessionName, 60*time.Second); err != nil {
			fmt.Printf("Warning: %v (sending prompt anyway)\n", err)
		}

		// Accept the bypass permissions warning dialog if present
		if err := tmux.AcceptBypassPermissionsWarning(sessionName); err != nil {
			fmt.Printf("Warning: could not accept bypass warning: %v\n", err)
		}

		// Additional delay for Claude to fully initialize its UI
		time.Sleep(2 * time.Second)

		// Send initial work prompt using reliable nudge pattern
		// Skip if --no-prompt is used (wt auto sends its own batch-aware prompt)
		if !flags.noPrompt {
			fmt.Println("Sending initial prompt to worker...")
			prompt := buildInitialPrompt(beadID, beadInfo.Title, beadInfo.Description, sessionName, proj)
			if err := tmux.NudgeSession(sessionName, prompt); err != nil {
				fmt.Printf("Warning: could not send initial prompt: %v\n", err)
			}
		}
	}

	// Determine if we should switch
	// Default to no-switch if running from hub (WT_HUB=1), unless --switch is used
	shouldSwitch := !flags.noSwitch
	if os.Getenv("WT_HUB") == "1" && !flags.forceSwitch {
		shouldSwitch = false
		fmt.Println("\n(Running from hub - staying in hub. Use 'wt <name>' or --switch to attach)")
	}

	// Switch to session unless --no-switch or in hub
	if shouldSwitch {
		fmt.Println("\nSwitching...")
		return tmux.Attach(sessionName)
	}

	return nil
}

func cmdKill(cfg *config.Config, name string, flags killFlags) error {
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	sess, exists := state.Sessions[name]
	if !exists {
		return fmt.Errorf("session '%s' not found", name)
	}

	fmt.Printf("Killing session '%s'...\n", name)

	// Run teardown hooks if configured
	mgr := project.NewManager(cfg)
	if proj, _ := mgr.Get(sess.Project); proj != nil {
		// Run test env teardown
		if proj.TestEnv != nil && proj.TestEnv.Teardown != "" {
			fmt.Println("  Running test environment teardown...")
			if err := testenv.RunTeardown(proj, sess.Worktree, sess.PortOffset); err != nil {
				fmt.Printf("  Warning: teardown failed: %v\n", err)
			}
		}

		// Run on_close hooks
		if proj.Hooks != nil && len(proj.Hooks.OnClose) > 0 {
			fmt.Println("  Running on_close hooks...")
			portEnv := ""
			if proj.TestEnv != nil {
				portEnv = proj.TestEnv.PortEnv
			}
			if err := testenv.RunOnCloseHooks(proj, sess.Worktree, sess.PortOffset, portEnv); err != nil {
				fmt.Printf("  Warning: on_close hook failed: %v\n", err)
			}
		}
	}

	// Kill tmux session
	fmt.Println("  Terminating tmux session...")
	if err := tmux.Kill(name); err != nil {
		fmt.Printf("  Warning: %v\n", err)
	}

	// Remove worktree (unless --keep-worktree)
	if !flags.keepWorktree {
		fmt.Printf("  Removing worktree: %s\n", sess.Worktree)
		if err := worktree.Remove(sess.Worktree); err != nil {
			fmt.Printf("  Warning: %v\n", err)
		}
	}

	// Log session end event (for seance resumption)
	eventLogger := events.NewLogger(cfg)
	claudeSession := getClaudeSessionID(sess.Worktree)
	eventLogger.LogSessionEnd(name, sess.Bead, sess.Project, claudeSession, "killed", "")

	// Remove from state
	delete(state.Sessions, name)
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Printf("\nDone. Bead %s still open.\n", sess.Bead)
	return nil
}

func cmdClose(cfg *config.Config, name string) error {
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	sess, exists := state.Sessions[name]
	if !exists {
		return fmt.Errorf("session '%s' not found", name)
	}

	fmt.Printf("Closing session '%s'...\n", name)
	fmt.Printf("  Bead: %s\n", sess.Bead)

	// Get project config for teardown hooks and default branch
	mgr := project.NewManager(cfg)
	proj, _ := mgr.Get(sess.Project)

	// Run teardown hooks if configured
	if proj != nil {
		// Run test env teardown
		if proj.TestEnv != nil && proj.TestEnv.Teardown != "" {
			fmt.Println("  Running test environment teardown...")
			if err := testenv.RunTeardown(proj, sess.Worktree, sess.PortOffset); err != nil {
				fmt.Printf("  Warning: teardown failed: %v\n", err)
			}
		}

		// Run on_close hooks
		if proj.Hooks != nil && len(proj.Hooks.OnClose) > 0 {
			fmt.Println("  Running on_close hooks...")
			portEnv := ""
			if proj.TestEnv != nil {
				portEnv = proj.TestEnv.PortEnv
			}
			if err := testenv.RunOnCloseHooks(proj, sess.Worktree, sess.PortOffset, portEnv); err != nil {
				fmt.Printf("  Warning: on_close hook failed: %v\n", err)
			}
		}
	}

	// Only close the bead if the branch has been merged to main
	// This ensures beads stay open when there's unfinished work
	defaultBranch := "main"
	if proj != nil && proj.DefaultBranch != "" {
		defaultBranch = proj.DefaultBranch
	}

	branch := sess.Branch
	if branch == "" {
		branch = sess.Bead
	}

	if worktree.IsBranchMerged(sess.Worktree, branch, defaultBranch) {
		fmt.Println("\n  Branch merged to", defaultBranch, "- closing bead...")
		if err := bead.Close(sess.Bead); err != nil {
			fmt.Printf("  Warning: could not close bead: %v\n", err)
		}
	} else {
		fmt.Printf("\n  Branch not merged to %s - keeping bead %s open.\n", defaultBranch, sess.Bead)
		fmt.Println("  Use 'wt done' to merge and close, or 'bd close' to close manually.")
	}

	// Kill tmux session
	fmt.Println("  Terminating tmux session...")
	if err := tmux.Kill(name); err != nil {
		fmt.Printf("  Warning: %v\n", err)
	}

	// Remove worktree
	fmt.Printf("  Removing worktree: %s\n", sess.Worktree)
	if err := worktree.Remove(sess.Worktree); err != nil {
		fmt.Printf("  Warning: %v\n", err)
	}

	// Log session end event (for seance resumption)
	eventLogger := events.NewLogger(cfg)
	claudeSession := getClaudeSessionID(sess.Worktree)
	eventLogger.LogSessionEnd(name, sess.Bead, sess.Project, claudeSession, "closed", "")

	// Remove from state
	delete(state.Sessions, name)
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Println("\nDone.")
	return nil
}

// cmdDone completes the current session's work and merges based on mode
func cmdDone(cfg *config.Config, flags doneFlags) error {
	// Find current session by checking if we're in a worktree
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	// Find session that matches current directory
	var sessionName string
	var sess *session.Session
	for name, s := range state.Sessions {
		if s.Worktree == cwd {
			sessionName = name
			sess = s
			break
		}
	}

	if sess == nil {
		return fmt.Errorf("not in a wt session. Run this from inside a session worktree")
	}

	// Handle task sessions differently
	if sess.IsTask() {
		return cmdDoneTask(cfg, state, sessionName, sess, cwd)
	}

	// Check for uncommitted changes
	hasChanges, err := merge.HasUncommittedChanges(cwd)
	if err != nil {
		return err
	}
	if hasChanges {
		return fmt.Errorf("you have uncommitted changes. Commit or stash them first")
	}

	// Check if a rebase is already in progress
	if merge.IsRebaseInProgress(cwd) {
		return fmt.Errorf("a rebase is in progress. Resolve conflicts with:\n  1. Edit conflicted files to resolve conflicts\n  2. Stage resolved files: git add <file>\n  3. Continue rebase: git rebase --continue\n  4. Or abort: git rebase --abort")
	}

	// Get project config
	mgr := project.NewManager(cfg)
	proj, err := mgr.Get(sess.Project)
	if err != nil {
		// Fallback to finding by bead prefix
		proj, err = mgr.FindByBeadPrefix(sess.Bead)
		if err != nil {
			return fmt.Errorf("project not found for session: %w", err)
		}
	}

	// Determine merge mode (flag overrides project config)
	mergeMode := proj.MergeMode
	if flags.mergeMode != "" {
		mergeMode = flags.mergeMode
	}

	defaultBranch := proj.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	branch := sess.Branch
	if branch == "" {
		branch = sess.Bead
	}

	// Get bead info for PR title
	beadInfo, err := bead.Show(sess.Bead)
	if err != nil {
		return fmt.Errorf("getting bead info: %w", err)
	}
	prTitle := beadInfo.Title
	if prTitle == "" {
		prTitle = sess.Bead
	}

	fmt.Printf("Completing session '%s'...\n", sessionName)
	fmt.Printf("  Bead:       %s\n", sess.Bead)
	fmt.Printf("  Branch:     %s\n", branch)
	fmt.Printf("  Merge mode: %s\n", mergeMode)

	// Auto-rebase on main unless disabled
	shouldRebase := !flags.noRebase && proj.AutoRebaseMode() != "false"

	if shouldRebase {
		fmt.Printf("\nFetching latest %s...\n", defaultBranch)
		if err := merge.FetchMain(cwd, defaultBranch); err != nil {
			return fmt.Errorf("fetching %s: %w", defaultBranch, err)
		}

		// Check if we're behind main
		behind, err := merge.CommitsBehind(cwd, defaultBranch)
		if err != nil {
			fmt.Printf("Warning: could not check commits behind: %v\n", err)
		} else if behind > 0 {
			fmt.Printf("Branch is %d commits behind %s. Rebasing...\n", behind, defaultBranch)

			result, err := merge.RebaseOnMain(cwd, defaultBranch)
			if err != nil {
				return fmt.Errorf("rebase failed: %w", err)
			}

			if result.HasConflicts {
				// Build conflict information message
				var conflictMsg strings.Builder
				conflictMsg.WriteString(fmt.Sprintf("Merge conflicts detected in %d file(s):\n", len(result.ConflictedFiles)))
				for _, file := range result.ConflictedFiles {
					conflictMsg.WriteString(fmt.Sprintf("  - %s\n", file))
				}
				conflictMsg.WriteString("\nTo resolve:\n")
				conflictMsg.WriteString("  1. Edit each conflicted file and resolve the conflict markers\n")
				conflictMsg.WriteString("  2. Stage resolved files: git add <file>\n")
				conflictMsg.WriteString("  3. Continue rebase: git rebase --continue\n")
				conflictMsg.WriteString("  4. Run 'wt done' again\n")
				conflictMsg.WriteString("\nOr abort with: git rebase --abort\n")
				conflictMsg.WriteString("\nConflict resolution guidelines:\n")
				conflictMsg.WriteString("  - Auto-resolve: trivial conflicts (whitespace, imports, non-overlapping changes)\n")
				conflictMsg.WriteString("  - Escalate: semantic conflicts, deletion conflicts, business logic changes\n")
				conflictMsg.WriteString("  - Run tests after resolving to verify correctness\n")

				return fmt.Errorf("%s", conflictMsg.String())
			}

			fmt.Println("Rebase successful.")
		} else {
			fmt.Printf("Branch is up-to-date with %s.\n", defaultBranch)
		}
	} else if flags.noRebase {
		fmt.Println("\nSkipping rebase (--no-rebase flag)")
	}

	var prURL string

	switch mergeMode {
	case "direct":
		fmt.Println("\nMerging directly to", defaultBranch, "...")
		if err := merge.DirectMerge(cwd, branch, defaultBranch); err != nil {
			return fmt.Errorf("direct merge failed: %w", err)
		}
		fmt.Println("Merged and pushed successfully.")

	case "pr-auto":
		fmt.Println("\nCreating PR with auto-merge...")
		var err error
		prURL, err = merge.CreatePR(cwd, branch, defaultBranch, prTitle)
		if err != nil {
			return fmt.Errorf("creating PR: %w", err)
		}
		fmt.Printf("PR created: %s\n", prURL)

		if err := merge.EnableAutoMerge(cwd, prURL); err != nil {
			fmt.Printf("Warning: could not enable auto-merge: %v\n", err)
			fmt.Println("PR created but you'll need to merge manually.")
		} else {
			fmt.Println("Auto-merge enabled. PR will merge when checks pass.")
		}

	case "pr-review":
		fmt.Println("\nCreating PR for review...")
		var err error
		prURL, err = merge.CreatePR(cwd, branch, defaultBranch, prTitle)
		if err != nil {
			return fmt.Errorf("creating PR: %w", err)
		}
		fmt.Printf("PR created: %s\n", prURL)
		fmt.Println("Waiting for review.")

	default:
		return fmt.Errorf("unknown merge mode: %s", mergeMode)
	}

	// Close the bead
	fmt.Println("\nClosing bead...")
	if err := bead.Close(sess.Bead); err != nil {
		fmt.Printf("Warning: could not close bead: %v\n", err)
	}

	// Check for batch mode marker (wt auto creates this to signal we shouldn't clean up)
	batchMarkerPath := filepath.Join(sess.Worktree, ".wt-batch-mode")
	isBatchMode := false
	if _, err := os.Stat(batchMarkerPath); err == nil {
		isBatchMode = true
		fmt.Println("\nBatch mode detected - keeping session alive for next bead.")
	}

	if !isBatchMode {
		// Run teardown hooks if configured
		if proj.TestEnv != nil && proj.TestEnv.Teardown != "" {
			fmt.Println("Running test environment teardown...")
			if err := testenv.RunTeardown(proj, sess.Worktree, sess.PortOffset); err != nil {
				fmt.Printf("Warning: teardown failed: %v\n", err)
			}
		}

		if proj.Hooks != nil && len(proj.Hooks.OnClose) > 0 {
			fmt.Println("Running on_close hooks...")
			portEnv := ""
			if proj.TestEnv != nil {
				portEnv = proj.TestEnv.PortEnv
			}
			if err := testenv.RunOnCloseHooks(proj, sess.Worktree, sess.PortOffset, portEnv); err != nil {
				fmt.Printf("Warning: on_close hook failed: %v\n", err)
			}
		}

		// Kill tmux session
		fmt.Println("Terminating tmux session...")
		if err := tmux.Kill(sessionName); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}

		// Remove worktree
		fmt.Printf("Removing worktree: %s\n", sess.Worktree)
		if err := worktree.Remove(sess.Worktree); err != nil {
			fmt.Printf("Warning: %v\n", err)
		}

		// Remove from state
		delete(state.Sessions, sessionName)
		if err := state.Save(); err != nil {
			return fmt.Errorf("saving state: %w", err)
		}
	}

	// Log session end event
	eventLogger := events.NewLogger(cfg)
	claudeSession := getClaudeSessionID(sess.Worktree)
	eventLogger.LogSessionEnd(sessionName, sess.Bead, sess.Project, claudeSession, mergeMode, prURL)

	fmt.Println("\nDone!")
	return nil
}

// cmdAbandon abandons the current session without merging
func cmdAbandon(cfg *config.Config) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	// Find session that matches current directory
	var sessionName string
	var sess *session.Session
	for name, s := range state.Sessions {
		if s.Worktree == cwd {
			sessionName = name
			sess = s
			break
		}
	}

	if sess == nil {
		return fmt.Errorf("not in a wt session. Run this from inside a session worktree")
	}

	fmt.Printf("Abandoning session '%s'...\n", sessionName)
	fmt.Printf("  Bead: %s (will remain open)\n", sess.Bead)

	// Run teardown hooks if configured
	mgr := project.NewManager(cfg)
	if proj, _ := mgr.Get(sess.Project); proj != nil {
		if proj.TestEnv != nil && proj.TestEnv.Teardown != "" {
			fmt.Println("  Running test environment teardown...")
			if err := testenv.RunTeardown(proj, sess.Worktree, sess.PortOffset); err != nil {
				fmt.Printf("  Warning: teardown failed: %v\n", err)
			}
		}

		if proj.Hooks != nil && len(proj.Hooks.OnClose) > 0 {
			fmt.Println("  Running on_close hooks...")
			portEnv := ""
			if proj.TestEnv != nil {
				portEnv = proj.TestEnv.PortEnv
			}
			if err := testenv.RunOnCloseHooks(proj, sess.Worktree, sess.PortOffset, portEnv); err != nil {
				fmt.Printf("  Warning: on_close hook failed: %v\n", err)
			}
		}
	}

	// Kill tmux session
	fmt.Println("  Terminating tmux session...")
	if err := tmux.Kill(sessionName); err != nil {
		fmt.Printf("  Warning: %v\n", err)
	}

	// Remove worktree
	fmt.Printf("  Removing worktree: %s\n", sess.Worktree)
	if err := worktree.Remove(sess.Worktree); err != nil {
		fmt.Printf("  Warning: %v\n", err)
	}

	// Remove from state
	delete(state.Sessions, sessionName)
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Printf("\nSession abandoned. Bead %s is still open.\n", sess.Bead)
	return nil
}

// StatusJSON is the JSON output format for session status
type StatusJSON struct {
	Session       string `json:"session"`
	Bead          string `json:"bead"`
	Title         string `json:"title"`
	Project       string `json:"project"`
	Branch        string `json:"branch"`
	MergeMode     string `json:"merge_mode"`
	Worktree      string `json:"worktree"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message,omitempty"`
	HasChanges    bool   `json:"has_uncommitted_changes"`
	PortOffset    int    `json:"port_offset,omitempty"`
	CreatedAt     string `json:"created_at"`
	LastActivity  string `json:"last_activity"`
}

// cmdStatus shows the status of the current session
func cmdStatus(cfg *config.Config) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	// Find session that matches current directory
	var sessionName string
	var sess *session.Session
	for name, s := range state.Sessions {
		if s.Worktree == cwd {
			sessionName = name
			sess = s
			break
		}
	}

	if sess == nil {
		return fmt.Errorf("not in a wt session. Run this from inside a session worktree")
	}

	// Get bead info
	beadInfo, err := bead.Show(sess.Bead)
	if err != nil {
		return fmt.Errorf("getting bead info: %w", err)
	}

	// Get git status
	hasChanges, _ := merge.HasUncommittedChanges(cwd)
	branch, _ := merge.GetCurrentBranch(cwd)

	// Get project info
	mgr := project.NewManager(cfg)
	proj, _ := mgr.Get(sess.Project)

	mergeMode := "pr-review"
	if proj != nil && proj.MergeMode != "" {
		mergeMode = proj.MergeMode
	}

	status := sess.Status
	if status == "" {
		status = "working"
	}

	// JSON output
	if outputJSON {
		result := StatusJSON{
			Session:       sessionName,
			Bead:          sess.Bead,
			Title:         beadInfo.Title,
			Project:       sess.Project,
			Branch:        branch,
			MergeMode:     mergeMode,
			Worktree:      sess.Worktree,
			Status:        status,
			StatusMessage: sess.StatusMessage,
			HasChanges:    hasChanges,
			PortOffset:    sess.PortOffset,
			CreatedAt:     sess.CreatedAt,
			LastActivity:  sess.LastActivity,
		}
		printJSON(result)
		return nil
	}

	fmt.Println("┌─ Session Status ─────────────────────────────────────────────────────┐")
	fmt.Println("│                                                                       │")
	fmt.Printf("│  Session:    %-55s │\n", sessionName)
	fmt.Printf("│  Bead:       %-55s │\n", sess.Bead)
	fmt.Printf("│  Title:      %-55s │\n", truncate(beadInfo.Title, 55))
	fmt.Printf("│  Project:    %-55s │\n", sess.Project)
	fmt.Printf("│  Branch:     %-55s │\n", branch)
	fmt.Printf("│  Merge mode: %-55s │\n", mergeMode)
	fmt.Println("│                                                                       │")

	if hasChanges {
		fmt.Println("│  Git:        ⚠ Uncommitted changes                                    │")
	} else {
		fmt.Println("│  Git:        ✓ Clean                                                  │")
	}

	if sess.PortOffset > 0 {
		portInfo := fmt.Sprintf("Port offset: %d", sess.PortOffset)
		fmt.Printf("│  %-67s │\n", portInfo)
	}

	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
	fmt.Println("\nCommands: wt done | wt abandon | wt signal <status>")

	return nil
}

// cmdSignal updates the session status with an optional message
func cmdSignal(cfg *config.Config, args []string) error {
	status := args[0]

	// Validate status
	validStatuses := map[string]bool{
		"working":   true,
		"ready":     true,
		"blocked":   true,
		"error":     true,
		"idle":      true,
		"bead-done": true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s\nvalid statuses: working, ready, blocked, error, idle, bead-done", status)
	}

	// Get optional message
	message := ""
	if len(args) > 1 {
		message = strings.Join(args[1:], " ")
	}

	// Find current session
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	var sessionName string
	var sess *session.Session
	for name, s := range state.Sessions {
		if s.Worktree == cwd {
			sessionName = name
			sess = s
			break
		}
	}

	if sess == nil {
		return fmt.Errorf("not in a wt session. Run this from inside a session worktree")
	}

	// Special handling for bead-done in auto mode
	if status == "bead-done" {
		epicState, inAutoMode := auto.IsInAutoMode(cfg, cwd)
		if inAutoMode {
			fmt.Printf("✓ Bead complete in auto mode. Transitioning to next bead...\n")
			if err := auto.HandleBeadDone(cfg, epicState, message); err != nil {
				return fmt.Errorf("handling bead-done in auto mode: %w", err)
			}
			// Don't update session status to "bead-done" - let auto mode handle it
			return nil
		}
		// Not in auto mode - just update status normally
		fmt.Println("Note: Not in auto mode. Use 'wt done' to complete the session.")
	}

	// Update status
	sess.Status = status
	sess.StatusMessage = message
	sess.UpdateActivity()

	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Display confirmation
	statusIcon := getStatusIcon(status)
	fmt.Printf("%s Session '%s' status: %s\n", statusIcon, sessionName, status)
	if message != "" {
		fmt.Printf("   Message: %s\n", message)
	}

	return nil
}

func cmdSwitch(cfg *config.Config, nameOrBead string) error {
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	// First try exact session name match
	if _, exists := state.Sessions[nameOrBead]; exists {
		return tmux.Attach(nameOrBead)
	}

	// Try bead ID match
	for name, sess := range state.Sessions {
		if sess.Bead == nameOrBead {
			return tmux.Attach(name)
		}
	}

	return fmt.Errorf("no session found for '%s'", nameOrBead)
}

// getClaudeSessionID gets the Claude session ID for seance resumption
func getClaudeSessionID(worktreePath string) string {
	// First, try reading from .wt/session_id in the worktree
	// This is persisted by wt prime --hook from Claude's SessionStart hook
	if worktreePath != "" {
		if id := handoff.ReadSessionID(worktreePath); id != "" {
			return id
		}
	}

	// Claude Code sets CLAUDE_SESSION_ID when running
	// This only works when called from within Claude's process
	if id := os.Getenv("CLAUDE_SESSION_ID"); id != "" {
		return id
	}

	// Fallback: scan ~/.claude/projects/ for session files matching the worktree
	// Claude stores sessions in directories named after the path (with - replacing /)
	if worktreePath != "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			// Convert worktree path to Claude's directory naming convention
			// e.g., /Users/lakshminp/worktrees/foo -> -Users-lakshminp-worktrees-foo
			claudeDirName := strings.ReplaceAll(worktreePath, "/", "-")
			projectDir := homeDir + "/.claude/projects/" + claudeDirName

			// Find the most recently modified .jsonl file
			entries, err := os.ReadDir(projectDir)
			if err == nil {
				var latestFile string
				var latestTime int64
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
						info, err := entry.Info()
						if err == nil && info.ModTime().Unix() > latestTime {
							latestTime = info.ModTime().Unix()
							latestFile = entry.Name()
						}
					}
				}
				if latestFile != "" {
					// Extract session ID from filename (remove .jsonl)
					return strings.TrimSuffix(latestFile, ".jsonl")
				}
			}
		}
	}

	// Last fallback: try reading from .claude/session file (legacy)
	if worktreePath != "" {
		sessionFile := worktreePath + "/.claude/session"
		data, err := os.ReadFile(sessionFile)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	return ""
}

// buildInitialPrompt creates the prompt to send to Claude when starting work on a bead
func buildInitialPrompt(beadID, title, description, sessionName string, proj *project.Project) string {
	var sb strings.Builder

	// Main task with bead ID and title
	sb.WriteString(fmt.Sprintf("Work on bead %s: %s.\n\n", beadID, title))

	// Include bead description if available
	if description != "" {
		sb.WriteString(description)
		sb.WriteString("\n\n")
	}

	// Workflow instructions
	sb.WriteString("Workflow:\n")
	sb.WriteString("1. Implement the task\n")
	sb.WriteString("2. Commit your changes with descriptive message\n")

	// Add test instructions if configured
	stepNum := 3
	if proj != nil && proj.TestEnv != nil {
		sb.WriteString(fmt.Sprintf("%d. Run tests and fix any failures\n", stepNum))
		stepNum++
	}

	// Add merge mode instructions
	mergeMode := "pr-review" // default
	if proj != nil && proj.MergeMode != "" {
		mergeMode = proj.MergeMode
	}

	defaultBranch := "main"
	if proj != nil && proj.DefaultBranch != "" {
		defaultBranch = proj.DefaultBranch
	}

	switch mergeMode {
	case "direct":
		sb.WriteString(fmt.Sprintf("%d. Run `wt done` to complete\n", stepNum))
		sb.WriteString("\nIMPORTANT: `wt done` handles everything:\n")
		sb.WriteString(fmt.Sprintf("- Rebases on %s\n", defaultBranch))
		sb.WriteString("- Pushes your changes\n")
		sb.WriteString("- Closes the bead\n")
		sb.WriteString("- Cleans up the session\n")
		sb.WriteString("\nDo NOT run `git push` or `bd close` separately.")
	case "pr-auto":
		sb.WriteString(fmt.Sprintf("%d. Create a PR with `gh pr create --base %s`\n", stepNum, defaultBranch))
		sb.WriteString("\nWhen finished, signal completion with the PR URL:\n")
		sb.WriteString("  wt signal ready \"PR: <paste PR URL here>\"\n")
		sb.WriteString("\nDo NOT run `wt done` - the hub will handle cleanup after merge.")
	case "pr-review":
		sb.WriteString(fmt.Sprintf("%d. Create a PR with `gh pr create --base %s`\n", stepNum, defaultBranch))
		sb.WriteString("\nWhen finished, signal completion with the PR URL:\n")
		sb.WriteString("  wt signal ready \"PR: <paste PR URL here>\"\n")
		sb.WriteString("\nDo NOT run `wt done` - the hub will handle cleanup after review.")
	}

	// Add commit message format with session name
	sb.WriteString("\n\n## Commit Message Format\n")
	sb.WriteString("Include this footer in your commit messages for traceability:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<commit message>\n\n")
	sb.WriteString("Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>\n")
	sb.WriteString(fmt.Sprintf("Session: %s\n", sessionName))
	sb.WriteString("```\n")

	// Add conflict resolution guidance
	sb.WriteString("\n## Conflict Resolution (if needed)\n")
	sb.WriteString("If `wt done` reports merge conflicts:\n")
	sb.WriteString("- Auto-resolve: trivial conflicts (whitespace, imports, non-overlapping changes)\n")
	sb.WriteString("- Escalate via `wt signal blocked \"<reason>\"`: semantic conflicts, deletion conflicts, business logic\n")
	sb.WriteString("- After resolving: `git add <file>`, `git rebase --continue`, then `wt done` again\n")

	return sb.String()
}

// getStatusIcon returns an icon for the given status
func getStatusIcon(status string) string {
	switch status {
	case "ready":
		return "✅"
	case "blocked":
		return "🚫"
	case "error":
		return "❌"
	case "working":
		return "🔄"
	case "idle":
		return "💤"
	default:
		return "•"
	}
}
