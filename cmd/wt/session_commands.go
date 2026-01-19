package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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
		}
	}
	return flags
}

func cmdList(cfg *config.Config) error {
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	if len(state.Sessions) == 0 {
		fmt.Println("No active sessions.")
		fmt.Println("\nCommands: wt new <bead> | wt <name> (switch)")
		return nil
	}

	fmt.Println("┌─ Active Sessions ─────────────────────────────────────────────────────┐")
	fmt.Println("│                                                                       │")
	fmt.Printf("│  %-10s %-18s %-10s %-14s %-15s │\n", "Name", "Bead", "Status", "Last Activity", "Project")
	fmt.Printf("│  %-10s %-18s %-10s %-14s %-15s │\n", "────", "────", "──────", "─────────────", "───────")

	for name, sess := range state.Sessions {
		status := sess.Status
		if status == "" {
			status = "working"
		}
		statusIcon := "🟢"
		if status == "idle" {
			statusIcon = "🟡"
		} else if status == "error" {
			statusIcon = "🔴"
		}

		lastActivity := formatDuration(sess.LastActivity)
		fmt.Printf("│  %s %-8s %-18s %-10s %-14s %-15s │\n",
			statusIcon, name, truncate(sess.Bead, 18), status, lastActivity, truncate(sess.Project, 15))
	}

	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
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

	// Validate bead exists
	beadInfo, err := bead.Show(beadID)
	if err != nil {
		return fmt.Errorf("bead not found: %s", beadID)
	}

	// Determine source repo
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

	// Create worktree
	worktreePath := cfg.WorktreePath(sessionName)
	fmt.Printf("Creating git worktree at %s...\n", worktreePath)
	if err := worktree.Create(repoPath, worktreePath, beadID); err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	// Determine BEADS_DIR (main repo's .beads)
	beadsDir := repoPath + "/.beads"

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

	// Send initial work prompt to Claude after delay for startup
	// Run synchronously to ensure prompt is sent before wt exits
	fmt.Println("Waiting for Claude to start...")
	time.Sleep(8 * time.Second) // Wait for Claude Code to fully initialize
	fmt.Println("Sending initial prompt to worker...")
	prompt := buildInitialPrompt(beadID, beadInfo.Title, proj)
	// Send prompt text
	sendPromptCmd := exec.Command("tmux", "send-keys", "-t", sessionName, prompt)
	if err := sendPromptCmd.Run(); err != nil {
		fmt.Printf("Warning: could not send initial prompt: %v\n", err)
	}
	// Send Enter key separately
	sendEnterCmd := exec.Command("tmux", "send-keys", "-t", sessionName, "Enter")
	if err := sendEnterCmd.Run(); err != nil {
		fmt.Printf("Warning: could not send Enter: %v\n", err)
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

	// Log session kill event
	eventLogger := events.NewLogger(cfg)
	eventLogger.LogSessionKill(name, sess.Bead, sess.Project)

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

	// Close the bead
	fmt.Println("\n  Closing bead...")
	if err := bead.Close(sess.Bead); err != nil {
		fmt.Printf("  Warning: could not close bead: %v\n", err)
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

// getClaudeSessionID attempts to read the Claude session ID from a worktree
func getClaudeSessionID(worktreePath string) string {
	// Claude Code stores session info in .claude/session
	// Try to read it if it exists
	sessionFile := worktreePath + "/.claude/session"
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return ""
	}
	return string(data)
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
	if proj != nil && proj.TestEnv != nil {
		sb.WriteString("3. Run tests and fix any failures\n")
	}

	// Add merge mode instructions
	mergeMode := "pr-review" // default
	if proj != nil && proj.MergeMode != "" {
		mergeMode = proj.MergeMode
	}

	switch mergeMode {
	case "direct":
		sb.WriteString("4. Push your changes\n")
		sb.WriteString("\nWhen finished, signal completion:\n")
		sb.WriteString("  wt signal ready \"Changes pushed\"\n")
		sb.WriteString("\nDo NOT run `wt done` - the hub will handle cleanup.")
	case "pr-auto":
		sb.WriteString("4. Create a PR with `gh pr create`\n")
		sb.WriteString("\nWhen finished, signal completion with the PR URL:\n")
		sb.WriteString("  wt signal ready \"PR: <paste PR URL here>\"\n")
		sb.WriteString("\nDo NOT run `wt done` - the hub will handle cleanup after merge.")
	case "pr-review":
		sb.WriteString("4. Create a PR with `gh pr create`\n")
		sb.WriteString("\nWhen finished, signal completion with the PR URL:\n")
		sb.WriteString("  wt signal ready \"PR: <paste PR URL here>\"\n")
		sb.WriteString("\nDo NOT run `wt done` - the hub will handle cleanup after review.")
	}

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
