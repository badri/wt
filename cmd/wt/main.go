package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/badri/wt/internal/auto"
	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/doctor"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/handoff"
	"github.com/badri/wt/internal/hub"
	"github.com/badri/wt/internal/merge"
	"github.com/badri/wt/internal/monitor"
	"github.com/badri/wt/internal/namepool"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/testenv"
	"github.com/badri/wt/internal/tmux"
	"github.com/badri/wt/internal/worktree"
)

// Version information - set via ldflags at build time
// Example: go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	args := os.Args[1:]

	// No args or "list" → list sessions
	if len(args) == 0 || args[0] == "list" {
		return cmdList(cfg)
	}

	switch args[0] {
	case "new":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt new <bead-id> [--repo <path>] [--name <name>] [--no-switch] [--switch] [--no-test-env]")
		}
		return cmdNew(cfg, args[1:])
	case "kill":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt kill <name> [--keep-worktree]")
		}
		return cmdKill(cfg, args[1], parseKillFlags(args[2:]))
	case "close":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt close <name>")
		}
		return cmdClose(cfg, args[1])
	case "done":
		return cmdDone(cfg, parseDoneFlags(args[1:]))
	case "status":
		return cmdStatus(cfg)
	case "abandon":
		return cmdAbandon(cfg)
	case "watch":
		return cmdWatch(cfg)
	case "seance":
		return cmdSeance(cfg, args[1:])
	case "projects":
		return cmdProjects(cfg)
	case "ready":
		var projectFilter string
		if len(args) > 1 {
			projectFilter = args[1]
		}
		return cmdReady(cfg, projectFilter)
	case "create":
		if len(args) < 3 {
			return fmt.Errorf("usage: wt create <project> <title> [--description <desc>] [--priority <0-3>] [--type <type>]")
		}
		return cmdCreate(cfg, args[1], args[2:])
	case "beads":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt beads <project> [--status <status>]")
		}
		return cmdBeads(cfg, args[1], parseBeadsFlags(args[2:]))
	case "project":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt project <add|config|remove> ...")
		}
		return cmdProject(cfg, args[1:])
	case "auto":
		return cmdAuto(cfg, args[1:])
	case "events":
		return cmdEvents(cfg, args[1:])
	case "doctor":
		return doctor.Run(cfg)
	case "config":
		return cmdConfig(cfg, args[1:])
	case "pick":
		return cmdPick(cfg)
	case "keys":
		return cmdKeys()
	case "completion":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt completion <bash|zsh|fish>")
		}
		return cmdCompletion(args[1])
	case "version", "--version", "-v":
		return cmdVersion()
	case "help", "--help", "-h":
		return cmdHelp()
	case "handoff":
		return cmdHandoff(cfg, args[1:])
	case "prime":
		return cmdPrime(cfg, args[1:])
	case "hub":
		return cmdHub(cfg, args[1:])
	default:
		// Assume it's a session name or bead ID to switch to
		return cmdSwitch(cfg, args[0])
	}
}

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

	// Send initial work prompt to Claude after a short delay
	go func() {
		time.Sleep(2 * time.Second) // Wait for Claude to start
		prompt := buildInitialPrompt(beadID, beadInfo.Title, proj)
		sendPromptCmd := exec.Command("tmux", "send-keys", "-t", sessionName, prompt, "Enter")
		sendPromptCmd.Run()
	}()

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
		sb.WriteString("\nWhen finished, notify that work is complete. Do NOT run `wt done` - the hub will handle cleanup.")
	case "pr-auto":
		sb.WriteString("4. Create a PR with `gh pr create`\n")
		sb.WriteString("\nWhen finished, notify that PR is ready. Do NOT run `wt done` - the hub will handle cleanup after merge.")
	case "pr-review":
		sb.WriteString("4. Create a PR with `gh pr create`\n")
		sb.WriteString("\nWhen finished, notify that PR is ready for review. Do NOT run `wt done` - the hub will handle cleanup after review.")
	}

	return sb.String()
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatDuration(t string) string {
	if t == "" {
		return "unknown"
	}
	// TODO: Parse time and format as "2m ago", "1h ago", etc.
	return "recently"
}

func collectUsedOffsets(state *session.State) []int {
	var offsets []int
	for _, sess := range state.Sessions {
		if sess.PortOffset > 0 {
			offsets = append(offsets, sess.PortOffset)
		}
	}
	return offsets
}

func cmdProjects(cfg *config.Config) error {
	mgr := project.NewManager(cfg)
	projects, err := mgr.List()
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		fmt.Println("No projects registered.")
		fmt.Println("\nRegister a project: wt project add <name> <path>")
		return nil
	}

	// Count active sessions per project
	state, _ := session.LoadState(cfg)
	sessionCount := make(map[string]int)
	for _, sess := range state.Sessions {
		sessionCount[sess.Project]++
	}

	fmt.Println("┌─ Projects ──────────────────────────────────────────────────────────────┐")
	fmt.Println("│                                                                         │")
	fmt.Printf("│  %-14s %-24s %-12s %-16s │\n", "Name", "Repo", "Merge Mode", "Active Sessions")
	fmt.Printf("│  %-14s %-24s %-12s %-16s │\n", "────", "────", "──────────", "───────────────")

	for _, proj := range projects {
		count := sessionCount[proj.Name]
		countStr := fmt.Sprintf("%d", count)
		if count == 0 {
			countStr = "-"
		}
		fmt.Printf("│  %-14s %-24s %-12s %-16s │\n",
			truncate(proj.Name, 14),
			truncate(proj.Repo, 24),
			proj.MergeMode,
			countStr)
	}

	fmt.Println("│                                                                         │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────┘")

	return nil
}

func cmdReady(cfg *config.Config, projectFilter string) error {
	mgr := project.NewManager(cfg)

	var allBeads []bead.ReadyBead

	if projectFilter != "" {
		// Single project - get beads from that project's .beads dir
		proj, err := mgr.Get(projectFilter)
		if err != nil {
			return fmt.Errorf("project '%s' not found. Register with: wt project add %s <path>", projectFilter, projectFilter)
		}
		beadsDir := proj.RepoPath() + "/.beads"
		beads, err := bead.ReadyInDir(beadsDir)
		if err != nil {
			return err
		}
		allBeads = beads
	} else {
		// No filter - aggregate across all registered projects
		projects, err := mgr.List()
		if err != nil {
			return err
		}

		if len(projects) == 0 {
			// Fall back to current directory beads
			beads, err := bead.Ready()
			if err != nil {
				return err
			}
			allBeads = beads
		} else {
			// Collect beads from all projects
			for _, proj := range projects {
				beadsDir := proj.RepoPath() + "/.beads"
				beads, err := bead.ReadyInDir(beadsDir)
				if err != nil {
					// Skip projects without beads
					continue
				}
				allBeads = append(allBeads, beads...)
			}
		}
	}

	if len(allBeads) == 0 {
		if projectFilter != "" {
			fmt.Printf("No ready beads for project '%s'.\n", projectFilter)
		} else {
			fmt.Println("No ready beads across all projects.")
		}
		fmt.Println("\nAll caught up!")
		return nil
	}

	title := "Ready Work (all projects)"
	if projectFilter != "" {
		title = fmt.Sprintf("Ready Work (%s)", projectFilter)
	}

	fmt.Printf("┌─ %s ", title)
	padding := 71 - len(title) - 4
	for i := 0; i < padding; i++ {
		fmt.Print("─")
	}
	fmt.Println("┐")
	fmt.Println("│                                                                       │")
	fmt.Printf("│  %-14s %-36s %-6s %-8s │\n", "Bead", "Title", "Type", "Priority")
	fmt.Printf("│  %-14s %-36s %-6s %-8s │\n", "────", "─────", "────", "────────")

	for _, b := range allBeads {
		priority := fmt.Sprintf("P%d", b.Priority)
		fmt.Printf("│  %-14s %-36s %-6s %-8s │\n",
			truncate(b.ID, 14),
			truncate(b.Title, 36),
			truncate(b.IssueType, 6),
			priority)
	}

	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
	fmt.Printf("\n%d bead(s) ready. Start with: wt new <bead>\n", len(allBeads))

	return nil
}

func cmdProject(cfg *config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt project <add|config|remove> ...")
	}

	mgr := project.NewManager(cfg)

	switch args[0] {
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: wt project add <name> <path>")
		}
		return cmdProjectAdd(mgr, args[1], args[2])
	case "config":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt project config <name>")
		}
		return cmdProjectConfig(mgr, args[1])
	case "remove", "rm", "delete":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt project remove <name>")
		}
		return cmdProjectRemove(cfg, mgr, args[1])
	default:
		return fmt.Errorf("unknown project command: %s", args[0])
	}
}

func cmdProjectAdd(mgr *project.Manager, name, path string) error {
	proj, err := mgr.Add(name, path)
	if err != nil {
		return err
	}

	fmt.Printf("Project '%s' registered.\n", proj.Name)
	fmt.Printf("  Repo:         %s\n", proj.Repo)
	fmt.Printf("  Beads prefix: %s\n", proj.BeadsPrefix)
	fmt.Printf("  Merge mode:   %s (default)\n", proj.MergeMode)
	fmt.Printf("\nConfigure with: wt project config %s\n", proj.Name)

	return nil
}

func cmdProjectConfig(mgr *project.Manager, name string) error {
	// Check project exists
	if _, err := mgr.Get(name); err != nil {
		return err
	}

	configPath := mgr.ConfigPath(name)

	// Get editor from environment
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func cmdProjectRemove(cfg *config.Config, mgr *project.Manager, name string) error {
	// Check project exists first
	proj, err := mgr.Get(name)
	if err != nil {
		return err
	}

	// Check for active sessions
	state, _ := session.LoadState(cfg)
	var activeSessions []string
	for sessName, sess := range state.Sessions {
		if sess.Project == name {
			activeSessions = append(activeSessions, sessName)
		}
	}

	if len(activeSessions) > 0 {
		return fmt.Errorf("cannot remove project '%s': %d active session(s): %v\nClose or kill sessions first", name, len(activeSessions), activeSessions)
	}

	fmt.Printf("Removing project '%s'...\n", name)
	fmt.Printf("  This will:\n")
	fmt.Printf("    - Remove project registration from wt\n")
	fmt.Printf("  This will NOT:\n")
	fmt.Printf("    - Delete the repository at %s\n", proj.RepoPath())
	fmt.Printf("    - Delete any beads (managed by bd)\n")
	fmt.Printf("    - Delete any open PRs on GitHub\n")
	fmt.Printf("    - Clean up orphaned worktrees (use: git worktree prune)\n")

	if err := mgr.Delete(name); err != nil {
		return err
	}

	fmt.Printf("\nProject '%s' removed.\n", name)
	fmt.Println("To re-register: wt project add", name, proj.Repo)
	return nil
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
	fmt.Println("\nCommands: wt done | wt abandon")

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

// cmdWatch displays a live dashboard of all sessions
func cmdWatch(cfg *config.Config) error {
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
				// Detect status from tmux activity
				status := monitor.DetectStatus(name, idleThreshold)
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

				// Format PR status
				prStr := "-"
				if prStatus != "none" && prStatus != "" {
					prStr = prStatus
				}

				statusIcon := monitor.StatusIcon(status)
				prIcon := monitor.PRStatusIcon(prStatus)

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
					if status == "idle" {
						monitor.Notify("wt: Session Idle", fmt.Sprintf("Session '%s' is now idle", name))
					} else if status == "error" {
						monitor.Notify("wt: Session Error", fmt.Sprintf("Session '%s' has an error", name))
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
		return cmdSeanceList(eventLogger)
	}

	// Parse flags
	sessionName := args[0]
	var prompt string
	for i := 1; i < len(args); i++ {
		if args[i] == "-p" && i+1 < len(args) {
			prompt = args[i+1]
			break
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

	// Resume session
	return cmdSeanceResume(event)
}

func cmdSeanceList(logger *events.Logger) error {
	sessions, err := logger.RecentSessions(10)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No past sessions found.")
		fmt.Println("\nSessions are recorded when they end via 'wt done', 'wt close', or 'wt handoff'.")
		return nil
	}

	fmt.Println("┌─ Past Sessions (seance) ─────────────────────────────────────────────┐")
	fmt.Println("│                                                                       │")
	fmt.Printf("│  %-10s %-18s %-12s %-24s │\n", "Session", "Bead", "Project", "Time")
	fmt.Printf("│  %-10s %-18s %-12s %-24s │\n", "───────", "────", "───────", "────")

	for _, sess := range sessions {
		t, _ := time.Parse(time.RFC3339, sess.Time)
		timeStr := t.Format("2006-01-02 15:04")

		// Determine icon based on session type
		icon := "  "
		if sess.ClaudeSession != "" {
			if sess.Type == events.EventHubHandoff {
				icon = "🏠"
			} else {
				icon = "💬"
			}
		}

		// For hub sessions, show special values
		beadDisplay := sess.Bead
		projectDisplay := sess.Project
		if sess.Type == events.EventHubHandoff {
			beadDisplay = "(hub)"
			projectDisplay = "(orchestrator)"
		}

		fmt.Printf("│  %s %-8s %-18s %-12s %-24s │\n",
			icon,
			truncate(sess.Session, 8),
			truncate(beadDisplay, 18),
			truncate(projectDisplay, 12),
			timeStr)
	}

	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
	fmt.Println("\n💬 = Worker session   🏠 = Hub session")
	fmt.Println("\nCommands:")
	fmt.Println("  wt seance <name>           Resume conversation")
	fmt.Println("  wt seance <name> -p 'msg'  One-shot query")
	fmt.Println("  wt seance hub              Resume last hub session")

	return nil
}

func cmdSeanceResume(event *events.Event) error {
	if event.Type == events.EventHubHandoff {
		fmt.Printf("Resuming hub session...\n")
	} else {
		fmt.Printf("Resuming Claude session for '%s' (bead: %s)...\n", event.Session, event.Bead)
	}

	// Run claude with --resume flag
	cmd := exec.Command("claude", "--resume", event.ClaudeSession)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func cmdSeanceQuery(event *events.Event, prompt string) error {
	fmt.Printf("Querying Claude session for '%s'...\n", event.Session)

	// Run claude with --resume and --print for one-shot
	cmd := exec.Command("claude", "--resume", event.ClaudeSession, "--print", prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// cmdCreate creates a bead in a specific project
func cmdCreate(cfg *config.Config, projectName string, args []string) error {
	mgr := project.NewManager(cfg)
	proj, err := mgr.Get(projectName)
	if err != nil {
		return fmt.Errorf("project '%s' not found. Register with: wt project add %s <path>", projectName, projectName)
	}

	// Parse title and flags
	if len(args) == 0 {
		return fmt.Errorf("title required")
	}

	title := args[0]
	opts := parseCreateFlags(args[1:])

	// Get beads directory for project
	beadsDir := proj.RepoPath() + "/.beads"

	// Create the bead
	beadID, err := bead.CreateInDir(beadsDir, title, opts)
	if err != nil {
		return err
	}

	fmt.Printf("Created bead in %s:\n", projectName)
	fmt.Printf("  ID:    %s\n", beadID)
	fmt.Printf("  Title: %s\n", title)
	if opts.Description != "" {
		fmt.Printf("  Desc:  %s\n", truncate(opts.Description, 50))
	}
	if opts.Type != "" {
		fmt.Printf("  Type:  %s\n", opts.Type)
	}
	if opts.Priority >= 0 {
		fmt.Printf("  Priority: P%d\n", opts.Priority)
	}
	fmt.Printf("\nSpawn worker: wt new %s\n", beadID)

	return nil
}

type createFlags struct {
	description string
	priority    int
	issueType   string
}

func parseCreateFlags(args []string) *bead.CreateOptions {
	opts := &bead.CreateOptions{Priority: -1} // -1 means not set
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--description", "-d":
			if i+1 < len(args) {
				opts.Description = args[i+1]
				i++
			}
		case "--priority", "-p":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.Priority)
				i++
			}
		case "--type", "-t":
			if i+1 < len(args) {
				opts.Type = args[i+1]
				i++
			}
		}
	}
	return opts
}

type beadsFlags struct {
	status string
}

func parseBeadsFlags(args []string) beadsFlags {
	var flags beadsFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--status", "-s":
			if i+1 < len(args) {
				flags.status = args[i+1]
				i++
			}
		}
	}
	return flags
}

// cmdBeads lists beads for a specific project
func cmdBeads(cfg *config.Config, projectName string, flags beadsFlags) error {
	mgr := project.NewManager(cfg)
	proj, err := mgr.Get(projectName)
	if err != nil {
		return fmt.Errorf("project '%s' not found. Register with: wt project add %s <path>", projectName, projectName)
	}

	beadsDir := proj.RepoPath() + "/.beads"
	beads, err := bead.ListInDir(beadsDir, flags.status)
	if err != nil {
		return err
	}

	if len(beads) == 0 {
		statusMsg := ""
		if flags.status != "" {
			statusMsg = fmt.Sprintf(" with status '%s'", flags.status)
		}
		fmt.Printf("No beads%s in project '%s'.\n", statusMsg, projectName)
		return nil
	}

	title := fmt.Sprintf("Beads (%s)", projectName)
	if flags.status != "" {
		title = fmt.Sprintf("Beads (%s, %s)", projectName, flags.status)
	}

	fmt.Printf("┌─ %s ", title)
	padding := 71 - len(title) - 4
	for i := 0; i < padding; i++ {
		fmt.Print("─")
	}
	fmt.Println("┐")
	fmt.Println("│                                                                       │")
	fmt.Printf("│  %-14s %-38s %-6s %-8s │\n", "Bead", "Title", "Type", "Priority")
	fmt.Printf("│  %-14s %-38s %-6s %-8s │\n", "────", "─────", "────", "────────")

	for _, b := range beads {
		priority := fmt.Sprintf("P%d", b.Priority)
		fmt.Printf("│  %-14s %-38s %-6s %-8s │\n",
			truncate(b.ID, 14),
			truncate(b.Title, 38),
			truncate(b.IssueType, 6),
			priority)
	}

	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
	fmt.Printf("\n%d bead(s) found.\n", len(beads))

	return nil
}

// cmdAuto runs autonomous batch processing of ready beads
func cmdAuto(cfg *config.Config, args []string) error {
	opts := parseAutoFlags(args)
	runner := auto.NewRunner(cfg, opts)
	return runner.Run()
}

func parseAutoFlags(args []string) *auto.Options {
	opts := &auto.Options{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project", "-p":
			if i+1 < len(args) {
				opts.Project = args[i+1]
				i++
			}
		case "--merge-mode", "-m":
			if i+1 < len(args) {
				opts.MergeMode = args[i+1]
				i++
			}
		case "--timeout", "-t":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.Timeout)
				i++
			}
		case "--dry-run":
			opts.DryRun = true
		case "--check":
			opts.Check = true
		case "--stop":
			opts.Stop = true
		case "--force":
			opts.Force = true
		}
	}
	return opts
}

// cmdEvents shows wt events
func cmdEvents(cfg *config.Config, args []string) error {
	flags := parseEventsFlags(args)
	logger := events.NewLogger(cfg)

	// Handle --new and --clear for hook integration
	if flags.newOnly {
		evts, err := logger.NewSinceLastRead(flags.clear)
		if err != nil {
			return err
		}
		if len(evts) == 0 {
			return nil // Silent when no new events (for hooks)
		}
		for _, e := range evts {
			printEvent(e)
		}
		return nil
	}

	// Handle --tail (follow mode)
	if flags.tail {
		return tailEvents(cfg, logger)
	}

	// Handle --since
	var evts []events.Event
	var err error

	if flags.since != "" {
		duration, err := parseDuration(flags.since)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}
		evts, err = logger.Since(duration)
		if err != nil {
			return err
		}
	} else {
		// Default: recent 20 events
		evts, err = logger.Recent(20)
		if err != nil {
			return err
		}
	}

	if len(evts) == 0 {
		fmt.Println("No events.")
		return nil
	}

	fmt.Printf("Events (%d):\n\n", len(evts))
	for _, e := range evts {
		printEvent(e)
	}

	return nil
}

func printEvent(e events.Event) {
	// Parse time for display
	t, _ := time.Parse(time.RFC3339, e.Time)
	timeStr := t.Format("15:04:05")

	switch e.Type {
	case events.EventSessionStart:
		fmt.Printf("[%s] %s session '%s' started for bead %s\n",
			timeStr, e.Project, e.Session, e.Bead)
	case events.EventSessionEnd:
		extra := ""
		if e.PRURL != "" {
			extra = fmt.Sprintf(" (PR: %s)", e.PRURL)
		}
		fmt.Printf("[%s] %s session '%s' completed bead %s%s\n",
			timeStr, e.Project, e.Session, e.Bead, extra)
	case events.EventSessionKill:
		fmt.Printf("[%s] %s session '%s' killed (bead: %s)\n",
			timeStr, e.Project, e.Session, e.Bead)
	case events.EventPRCreated:
		fmt.Printf("[%s] %s PR created for %s: %s\n",
			timeStr, e.Project, e.Bead, e.PRURL)
	case events.EventPRMerged:
		fmt.Printf("[%s] %s PR merged for %s: %s\n",
			timeStr, e.Project, e.Bead, e.PRURL)
	default:
		fmt.Printf("[%s] %s: %s (%s)\n", timeStr, e.Type, e.Session, e.Bead)
	}
}

func tailEvents(cfg *config.Config, logger *events.Logger) error {
	fmt.Printf("Watching events (Ctrl+C to stop)...\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	eventsCh := make(chan events.Event)

	go func() {
		logger.Tail(ctx, eventsCh)
		close(eventsCh)
	}()

	for e := range eventsCh {
		printEvent(e)
	}

	return nil
}

type eventsFlags struct {
	tail    bool
	since   string
	newOnly bool
	clear   bool
}

func parseEventsFlags(args []string) eventsFlags {
	var flags eventsFlags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tail", "-f":
			flags.tail = true
		case "--since", "-s":
			if i+1 < len(args) {
				flags.since = args[i+1]
				i++
			}
		case "--new", "-n":
			flags.newOnly = true
		case "--clear", "-c":
			flags.clear = true
		}
	}
	return flags
}

// parseDuration parses duration strings like "5m", "1h", "30s"
func parseDuration(s string) (time.Duration, error) {
	// Handle shorthand
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration")
	}

	// Try standard Go duration
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Handle "5m" style without explicit unit parsing issues
	return time.ParseDuration(s)
}

// pickerEntry represents a session for the picker UI
type pickerEntry struct {
	name    string
	bead    string
	project string
}

// cmdPick provides an interactive session picker
func cmdPick(cfg *config.Config) error {
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	if len(state.Sessions) == 0 {
		fmt.Println("No active sessions.")
		return nil
	}

	// Build list of sessions
	var entries []pickerEntry
	for name, sess := range state.Sessions {
		entries = append(entries, pickerEntry{
			name:    name,
			bead:    sess.Bead,
			project: sess.Project,
		})
	}

	// Try fzf first
	if hasFzf() {
		return pickWithFzf(entries)
	}

	// Fall back to simple numbered selection
	return pickWithPrompt(entries)
}

// hasFzf checks if fzf is available
func hasFzf() bool {
	_, err := exec.LookPath("fzf")
	return err == nil
}

// pickWithFzf uses fzf for selection
func pickWithFzf(entries []pickerEntry) error {
	// Build input for fzf
	var input strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&input, "%s\t%s\t%s\n", e.name, e.bead, e.project)
	}

	cmd := exec.Command("fzf",
		"--height=40%",
		"--reverse",
		"--header=Select session (name / bead / project)",
		"--delimiter=\t",
		"--with-nth=1,2,3",
	)
	cmd.Stdin = strings.NewReader(input.String())
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		// User cancelled (Ctrl+C)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return nil
		}
		return err
	}

	// Parse selection - first field is the session name
	selected := strings.TrimSpace(string(output))
	if selected == "" {
		return nil
	}

	parts := strings.Split(selected, "\t")
	if len(parts) > 0 {
		return tmux.Attach(parts[0])
	}
	return nil
}

// pickWithPrompt uses simple numbered selection
func pickWithPrompt(entries []pickerEntry) error {
	fmt.Println("Select a session:")
	fmt.Println()
	for i, e := range entries {
		fmt.Printf("  [%d] %s  (%s, %s)\n", i+1, e.name, e.bead, e.project)
	}
	fmt.Println()
	fmt.Print("Enter number: ")

	var choice int
	_, err := fmt.Scanf("%d", &choice)
	if err != nil {
		return fmt.Errorf("invalid selection")
	}

	if choice < 1 || choice > len(entries) {
		return fmt.Errorf("selection out of range")
	}

	return tmux.Attach(entries[choice-1].name)
}

// cmdKeys outputs tmux keybinding configuration
func cmdKeys() error {
	keybindings := `# wt tmux keybindings
# Add these to your ~/.tmux.conf

# C-b s - Pick wt session with fzf in popup (requires tmux 3.2+)
bind-key s display-popup -E "wt pick"

# Alternative: use new-window if popup not available (tmux < 3.2)
# bind-key s new-window "wt pick"

# Keep C-b S for the default tmux session chooser
bind-key S choose-tree -s

# Reload tmux config after adding:
#   tmux source-file ~/.tmux.conf
`
	fmt.Print(keybindings)
	return nil
}

// cmdCompletion outputs shell completion scripts
func cmdCompletion(shell string) error {
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return fmt.Errorf("unsupported shell: %s (use bash, zsh, or fish)", shell)
	}
	return nil
}

const bashCompletion = `# wt bash completion
# Add to ~/.bashrc: eval "$(wt completion bash)"

_wt_completions() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    commands="list new kill close done status abandon watch seance projects ready create beads project auto events doctor config pick keys completion handoff prime hub help"

    case "${prev}" in
        wt)
            # Complete commands or session names
            local sessions=$(wt list 2>/dev/null | grep -E '^\│\s+[🟢🟡🔴]' | awk '{print $2}')
            COMPREPLY=( $(compgen -W "${commands} ${sessions}" -- "${cur}") )
            return 0
            ;;
        new)
            # Complete bead IDs from ready beads
            local beads=$(wt ready 2>/dev/null | grep -E '^\│\s+[a-z]+-[a-z0-9]+' | awk '{print $1}' | tr -d '│')
            COMPREPLY=( $(compgen -W "${beads}" -- "${cur}") )
            return 0
            ;;
        kill|close)
            # Complete session names
            local sessions=$(wt list 2>/dev/null | grep -E '^\│\s+[🟢🟡🔴]' | awk '{print $2}')
            COMPREPLY=( $(compgen -W "${sessions}" -- "${cur}") )
            return 0
            ;;
        seance)
            # Complete past session names
            local sessions=$(wt seance 2>/dev/null | grep -E '^\│\s+💬' | awk '{print $2}')
            COMPREPLY=( $(compgen -W "${sessions}" -- "${cur}") )
            return 0
            ;;
        project)
            COMPREPLY=( $(compgen -W "add config remove" -- "${cur}") )
            return 0
            ;;
        create|beads|ready)
            # Complete project names
            local projects=$(wt projects 2>/dev/null | grep -E '^\│\s+[a-zA-Z]' | awk '{print $1}' | tr -d '│')
            COMPREPLY=( $(compgen -W "${projects}" -- "${cur}") )
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            return 0
            ;;
        hub)
            COMPREPLY=( $(compgen -W "--status --detach --kill --force" -- "${cur}") )
            return 0
            ;;
        config)
            COMPREPLY=( $(compgen -W "show init set editor" -- "${cur}") )
            return 0
            ;;
        *)
            COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
            return 0
            ;;
    esac
}

complete -F _wt_completions wt
`

const zshCompletion = `#compdef wt
# wt zsh completion
# Add to ~/.zshrc: eval "$(wt completion zsh)"

_wt() {
    local -a commands sessions beads projects

    commands=(
        'list:List active sessions'
        'new:Create new session for a bead'
        'kill:Kill a session (keep bead open)'
        'close:Close session and bead'
        'done:Complete work and merge'
        'status:Show current session status'
        'abandon:Abandon session without merging'
        'watch:Live dashboard of sessions'
        'seance:Talk to past sessions'
        'projects:List registered projects'
        'ready:Show ready beads'
        'create:Create a new bead'
        'beads:List beads for a project'
        'project:Manage projects'
        'auto:Autonomous batch processing'
        'events:Show wt events'
        'doctor:Check wt setup'
        'pick:Interactive session picker'
        'keys:Output tmux keybindings'
        'completion:Generate shell completions'
        'handoff:Handoff to fresh Claude instance'
        'prime:Inject startup context'
        'hub:Create or attach to hub session'
        'help:Show help message'
        'config:Manage wt configuration'
    )

    _arguments -C \
        '1: :->command' \
        '*: :->args'

    case $state in
        command)
            _describe 'command' commands
            # Also complete session names
            local -a sess
            sess=(${(f)"$(wt list 2>/dev/null | grep -E '^\│\s+[🟢🟡🔴]' | awk '{print $2}')"})
            [[ -n "$sess" ]] && _describe 'session' sess
            ;;
        args)
            case $words[2] in
                new)
                    local -a beads
                    beads=(${(f)"$(wt ready 2>/dev/null | grep -E '^\│\s+[a-z]+-[a-z0-9]+' | awk '{print $1}' | tr -d '│')"})
                    _describe 'bead' beads
                    ;;
                kill|close)
                    local -a sess
                    sess=(${(f)"$(wt list 2>/dev/null | grep -E '^\│\s+[🟢🟡🔴]' | awk '{print $2}')"})
                    _describe 'session' sess
                    ;;
                seance)
                    local -a sess
                    sess=(${(f)"$(wt seance 2>/dev/null | grep -E '^\│\s+💬' | awk '{print $2}')"})
                    _describe 'session' sess
                    ;;
                project)
                    _describe 'subcommand' '(add config remove)'
                    ;;
                create|beads|ready)
                    local -a proj
                    proj=(${(f)"$(wt projects 2>/dev/null | grep -E '^\│\s+[a-zA-Z]' | awk '{print $1}' | tr -d '│')"})
                    _describe 'project' proj
                    ;;
                completion)
                    _describe 'shell' '(bash zsh fish)'
                    ;;
                hub)
                    _describe 'flag' '(--status --detach --kill --force)'
                    ;;
                config)
                    _describe 'subcommand' '(show init set editor)'
                    ;;
            esac
            ;;
    esac
}

_wt "$@"
`

const fishCompletion = `# wt fish completion
# Add to ~/.config/fish/completions/wt.fish

# Disable file completion by default
complete -c wt -f

# Commands
complete -c wt -n __fish_use_subcommand -a list -d 'List active sessions'
complete -c wt -n __fish_use_subcommand -a new -d 'Create new session for a bead'
complete -c wt -n __fish_use_subcommand -a kill -d 'Kill a session (keep bead open)'
complete -c wt -n __fish_use_subcommand -a close -d 'Close session and bead'
complete -c wt -n __fish_use_subcommand -a done -d 'Complete work and merge'
complete -c wt -n __fish_use_subcommand -a status -d 'Show current session status'
complete -c wt -n __fish_use_subcommand -a abandon -d 'Abandon session without merging'
complete -c wt -n __fish_use_subcommand -a watch -d 'Live dashboard of sessions'
complete -c wt -n __fish_use_subcommand -a seance -d 'Talk to past sessions'
complete -c wt -n __fish_use_subcommand -a projects -d 'List registered projects'
complete -c wt -n __fish_use_subcommand -a ready -d 'Show ready beads'
complete -c wt -n __fish_use_subcommand -a create -d 'Create a new bead'
complete -c wt -n __fish_use_subcommand -a beads -d 'List beads for a project'
complete -c wt -n __fish_use_subcommand -a project -d 'Manage projects'
complete -c wt -n __fish_use_subcommand -a auto -d 'Autonomous batch processing'
complete -c wt -n __fish_use_subcommand -a events -d 'Show wt events'
complete -c wt -n __fish_use_subcommand -a doctor -d 'Check wt setup'
complete -c wt -n __fish_use_subcommand -a pick -d 'Interactive session picker'
complete -c wt -n __fish_use_subcommand -a keys -d 'Output tmux keybindings'
complete -c wt -n __fish_use_subcommand -a completion -d 'Generate shell completions'
complete -c wt -n __fish_use_subcommand -a handoff -d 'Handoff to fresh Claude instance'
complete -c wt -n __fish_use_subcommand -a prime -d 'Inject startup context'
complete -c wt -n __fish_use_subcommand -a hub -d 'Create or attach to hub session'
complete -c wt -n __fish_use_subcommand -a help -d 'Show help message'
complete -c wt -n __fish_use_subcommand -a config -d 'Manage wt configuration'

# Session names for bare wt command
complete -c wt -n __fish_use_subcommand -a "(wt list 2>/dev/null | grep -E '^\│\s+[🟢🟡🔴]' | awk '{print \$2}')" -d 'Switch to session'

# Completions for 'new' - bead IDs
complete -c wt -n '__fish_seen_subcommand_from new' -a "(wt ready 2>/dev/null | grep -E '^\│\s+[a-z]+-[a-z0-9]+' | awk '{print \$1}' | tr -d '│')" -d 'Bead'

# Completions for 'kill' and 'close' - session names
complete -c wt -n '__fish_seen_subcommand_from kill close' -a "(wt list 2>/dev/null | grep -E '^\│\s+[🟢🟡🔴]' | awk '{print \$2}')" -d 'Session'

# Completions for 'seance' - past sessions
complete -c wt -n '__fish_seen_subcommand_from seance' -a "(wt seance 2>/dev/null | grep -E '^\│\s+💬' | awk '{print \$2}')" -d 'Past session'

# Completions for 'project' subcommand
complete -c wt -n '__fish_seen_subcommand_from project' -a 'add config remove' -d 'Project subcommand'

# Completions for 'create', 'beads', 'ready' - project names
complete -c wt -n '__fish_seen_subcommand_from create beads ready' -a "(wt projects 2>/dev/null | grep -E '^\│\s+[a-zA-Z]' | awk '{print \$1}' | tr -d '│')" -d 'Project'

# Completions for 'completion' - shell types
complete -c wt -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish' -d 'Shell'

# Completions for 'hub' - flags
complete -c wt -n '__fish_seen_subcommand_from hub' -l status -s s -d 'Show hub status'
complete -c wt -n '__fish_seen_subcommand_from hub' -l detach -s d -d 'Detach from hub'
complete -c wt -n '__fish_seen_subcommand_from hub' -l kill -s k -d 'Kill hub session'
complete -c wt -n '__fish_seen_subcommand_from hub' -l force -s f -d 'Skip confirmation'

# Completions for 'config' - subcommands
complete -c wt -n '__fish_seen_subcommand_from config' -a 'show init set editor' -d 'Config subcommand'
`

// cmdVersion prints version information
func cmdVersion() error {
	fmt.Printf("wt version %s\n", version)
	fmt.Printf("  commit:  %s\n", commit)
	fmt.Printf("  built:   %s\n", date)
	fmt.Printf("  go:      %s\n", runtime.Version())
	fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return nil
}

// cmdHelp prints categorized help information
func cmdHelp() error {
	help := `wt - Worktree Session Manager

Orchestrates isolated development sessions where each bead gets its own
git worktree, tmux session, and optional test environment.

Usage:
  wt [command]
  wt <session>     Switch to a session by name or bead ID

Hub Management:
  hub              Create or attach to hub session
  hub --status     Show hub status without attaching
  hub --detach     Detach from hub, return to previous session
  hub --kill       Kill hub session (with confirmation)
  hub --kill -f    Kill hub session (no confirmation)
  handoff          Hand off to fresh Claude instance, preserve context
  prime            Inject startup context (for hooks)

Session Management:
  list             List active sessions (default when no args)
  new <bead>       Spawn new worker session for a bead
                   (stays in hub by default, use --switch to attach)
  kill <name>      Kill session, keep bead open
  close <name>     Close session and bead
  done             Complete work and merge (from inside worker)
  status           Show current session status (from inside worker)
  abandon          Discard work, keep bead open (from inside worker)
  pick             Interactive session picker (uses fzf if available)

Monitoring:
  watch            Live dashboard of all sessions
  events           Show recent events
  events --tail    Follow events in real-time
  seance           List past sessions
  seance <name>    Talk to a past session

Project Management:
  projects         List registered projects
  project add      Register a new project
  project config   Edit project configuration
  project remove   Remove a project registration

Bead Operations:
  ready            Show ready beads across all projects
  ready <project>  Show ready beads for a specific project
  create           Create a new bead in a project
  beads <project>  List beads for a project

Automation:
  auto             Process ready beads autonomously
  auto --dry-run   Preview what would run
  auto --stop      Stop running auto gracefully

Configuration:
  config           Show current configuration
  config init      Create config file with defaults
  config editor    Set Claude editor command (with/without permissions)
  config set       Set a config value

Diagnostics:
  doctor           Run health checks on wt setup
  keys             Output tmux keybindings for .tmux.conf
  completion       Generate shell completions (bash/zsh/fish)
  version          Print version information
  help             Show this help message

Flags:
  -h, --help       Show help
  -v, --version    Show version

Use "wt <command> --help" for more information about a command.
`
	fmt.Print(help)
	return nil
}

// cmdConfig manages wt configuration
func cmdConfig(cfg *config.Config, args []string) error {
	// Parse subcommand
	if len(args) == 0 {
		return showConfig(cfg)
	}

	switch args[0] {
	case "show":
		return showConfig(cfg)
	case "init":
		return initConfig(cfg)
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: wt config set <key> <value>")
		}
		return setConfig(cfg, args[1], args[2])
	case "editor":
		return configEditor(cfg, args[1:])
	default:
		return fmt.Errorf("unknown config subcommand: %s\nusage: wt config [show|init|set|editor]", args[0])
	}
}

func showConfig(cfg *config.Config) error {
	fmt.Println("wt configuration")
	fmt.Println("================")
	fmt.Printf("Config file:     %s", cfg.ConfigPath())
	if cfg.ConfigExists() {
		fmt.Println(" (exists)")
	} else {
		fmt.Println(" (not created)")
	}
	fmt.Println()
	fmt.Printf("worktree_root:   %s\n", cfg.WorktreeRoot)
	fmt.Printf("editor_cmd:      %s\n", cfg.EditorCmd)
	fmt.Printf("merge_mode:      %s\n", cfg.DefaultMergeMode)
	return nil
}

func initConfig(cfg *config.Config) error {
	if cfg.ConfigExists() {
		fmt.Printf("Config file already exists: %s\n", cfg.ConfigPath())
		return nil
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}

	fmt.Printf("Created config file: %s\n", cfg.ConfigPath())
	return nil
}

func setConfig(cfg *config.Config, key, value string) error {
	switch key {
	case "worktree_root":
		cfg.WorktreeRoot = value
	case "editor_cmd":
		cfg.EditorCmd = value
	case "merge_mode", "default_merge_mode":
		cfg.DefaultMergeMode = value
	default:
		return fmt.Errorf("unknown config key: %s\nvalid keys: worktree_root, editor_cmd, merge_mode", key)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}

func configEditor(cfg *config.Config, args []string) error {
	// If no args, show current and prompt
	if len(args) == 0 {
		fmt.Println("Select Claude editor command:")
		fmt.Println()
		fmt.Println("  1) claude --dangerously-skip-permissions  (recommended for automation)")
		fmt.Println("  2) claude                                  (with permission prompts)")
		fmt.Println()
		fmt.Printf("Current: %s\n", cfg.EditorCmd)
		fmt.Println()
		fmt.Println("Usage: wt config editor [1|2|skip-permissions|with-prompts]")
		return nil
	}

	var newCmd string
	switch args[0] {
	case "1", "skip-permissions", "skip":
		newCmd = "claude --dangerously-skip-permissions"
	case "2", "with-prompts", "prompts":
		newCmd = "claude"
	default:
		return fmt.Errorf("invalid option: %s\nuse: 1, 2, skip-permissions, or with-prompts", args[0])
	}

	cfg.EditorCmd = newCmd
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Set editor_cmd = %s\n", newCmd)
	return nil
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

// cmdHub creates or attaches to the dedicated hub session
func cmdHub(cfg *config.Config, args []string) error {
	opts := parseHubFlags(args)
	return hub.Run(cfg, opts)
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
		}
	}
	return opts
}
