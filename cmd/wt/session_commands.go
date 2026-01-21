package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
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
	noSwitch    bool
	forceSwitch bool
	noTestEnv   bool
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
    --no-switch         Don't switch to the new session after creation
    --switch            Force switch even when running from hub
    --no-test-env       Skip test environment setup
    -h, --help          Show this help

EXAMPLES:
    wt new wt-123                     Start working on bead wt-123
    wt new wt-123 --name mybranch     Use custom session name
    wt new wt-123 --no-switch         Create but stay in current session
    wt new proj-456 --repo ~/code/proj  Specify repo path
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
		case "--no-switch":
			flags.noSwitch = true
		case "--switch":
			flags.forceSwitch = true
		case "--no-test-env":
			flags.noTestEnv = true
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
	Name          string `json:"name"`
	Bead          string `json:"bead"`
	Project       string `json:"project"`
	Worktree      string `json:"worktree"`
	Branch        string `json:"branch"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message,omitempty"`
	CreatedAt     string `json:"created_at"`
	LastActivity  string `json:"last_activity"`
}

func cmdList(cfg *config.Config) error {
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	if len(state.Sessions) == 0 {
		printEmptyMessage("No active sessions.", "Commands: wt new <bead> | wt <name> (switch)")
		return nil
	}

	// JSON output
	if outputJSON {
		var sessions []SessionJSON
		for name, sess := range state.Sessions {
			status := sess.Status
			if status == "" {
				status = "working"
			}
			sessions = append(sessions, SessionJSON{
				Name:          name,
				Bead:          sess.Bead,
				Project:       sess.Project,
				Worktree:      sess.Worktree,
				Branch:        sess.Branch,
				Status:        status,
				StatusMessage: sess.StatusMessage,
				CreatedAt:     sess.CreatedAt,
				LastActivity:  sess.LastActivity,
			})
		}
		printJSON(sessions)
		return nil
	}

	// Define columns
	columns := []table.Column{
		{Title: "Name", Width: 18},
		{Title: "Status", Width: 10},
		{Title: "Title", Width: 36},
		{Title: "Project", Width: 14},
	}

	// Build rows
	var rows []table.Row
	for name, sess := range state.Sessions {
		status := sess.Status
		if status == "" {
			status = "working"
		}

		// Get bead title (use BeadsDir to find correct project)
		title := ""
		if beadInfo, err := bead.ShowInDir(sess.Bead, sess.BeadsDir); err == nil && beadInfo != nil {
			title = beadInfo.Title
		}

		rows = append(rows, table.Row{
			name,
			status,
			truncate(title, 36),
			truncate(sess.Project, 14),
		})
	}

	printTable("Active Sessions", columns, rows)
	fmt.Println("\nCommands: wt <name> (switch) | wt new <bead> | wt close <name>")

	return nil
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

	// Determine source repo FIRST (needed to find the bead)
	repoPath := flags.repo
	var proj *project.Project
	mgr := project.NewManager(cfg)

	if repoPath == "" {
		// Try to find project by bead prefix
		proj, _ = mgr.FindByBeadPrefix(beadID)
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

	// Validate bead exists (using project's beads directory if known)
	beadsDir := repoPath + "/.beads"
	beadInfo, err := bead.ShowInDir(beadID, beadsDir)
	if err != nil {
		return fmt.Errorf("bead not found: %s", beadID)
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
	if sessionName == "" {
		themeName, err := pool.Allocate(state.UsedNames())
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
	if err := worktree.Create(repoPath, worktreePath, beadID); err != nil {
		return fmt.Errorf("creating worktree: %w", err)
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
	if err := tmux.NewSession(sessionName, worktreePath, beadsDir, cfg.EditorCmd, tmuxOpts); err != nil {
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
	fmt.Println("Sending initial prompt to worker...")
	prompt := buildInitialPrompt(beadID, beadInfo.Title, proj)
	if err := tmux.NudgeSession(sessionName, prompt); err != nil {
		fmt.Printf("Warning: could not send initial prompt: %v\n", err)
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

	// Log session end event
	eventLogger := events.NewLogger(cfg)
	claudeSession := getClaudeSessionID(sess.Worktree)
	eventLogger.LogSessionEnd(sessionName, sess.Bead, sess.Project, claudeSession, mergeMode, prURL)

	// Remove from state
	delete(state.Sessions, sessionName)
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

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
		"working": true,
		"ready":   true,
		"blocked": true,
		"error":   true,
		"idle":    true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s\nvalid statuses: working, ready, blocked, error, idle", status)
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
	// Claude Code sets CLAUDE_SESSION_ID when running
	// This is the most reliable way to get the session ID
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
func buildInitialPrompt(beadID, title string, proj *project.Project) string {
	var sb strings.Builder

	// Main task
	sb.WriteString(fmt.Sprintf("Work on bead %s: %s.\n\n", beadID, title))

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

	switch mergeMode {
	case "direct":
		sb.WriteString(fmt.Sprintf("%d. Push your changes\n", stepNum))
		sb.WriteString("\nWhen finished, merge and close:\n")
		sb.WriteString("  wt done\n")
		sb.WriteString("\nThis will auto-rebase on main, push, and clean up the worktree.")
	case "pr-auto":
		sb.WriteString(fmt.Sprintf("%d. Create a PR with `gh pr create`\n", stepNum))
		sb.WriteString("\nWhen finished, signal completion with the PR URL:\n")
		sb.WriteString("  wt signal ready \"PR: <paste PR URL here>\"\n")
		sb.WriteString("\nDo NOT run `wt done` - the hub will handle cleanup after merge.")
	case "pr-review":
		sb.WriteString(fmt.Sprintf("%d. Create a PR with `gh pr create`\n", stepNum))
		sb.WriteString("\nWhen finished, signal completion with the PR URL:\n")
		sb.WriteString("  wt signal ready \"PR: <paste PR URL here>\"\n")
		sb.WriteString("\nDo NOT run `wt done` - the hub will handle cleanup after review.")
	}

	// Add conflict resolution guidance
	sb.WriteString("\n\n## Conflict Resolution (if needed)\n")
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
