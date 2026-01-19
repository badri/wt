package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/namepool"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/tmux"
	"github.com/badri/wt/internal/worktree"
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
			return fmt.Errorf("usage: wt new <bead-id> [--repo <path>] [--name <name>] [--no-switch]")
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
	case "projects":
		return cmdProjects(cfg)
	case "project":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt project <add|config> ...")
		}
		return cmdProject(cfg, args[1:])
	default:
		// Assume it's a session name or bead ID to switch to
		return cmdSwitch(cfg, args[0])
	}
}

type newFlags struct {
	repo     string
	name     string
	noSwitch bool
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

	// Allocate name
	pool, err := namepool.Load(cfg)
	if err != nil {
		return err
	}

	sessionName := flags.name
	if sessionName == "" {
		sessionName, err = pool.Allocate(state.UsedNames())
		if err != nil {
			return err
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

	// Create tmux session
	fmt.Printf("Creating tmux session '%s'...\n", sessionName)
	if err := tmux.NewSession(sessionName, worktreePath, beadsDir, cfg.EditorCmd); err != nil {
		// Cleanup worktree on failure
		worktree.Remove(worktreePath)
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Determine project name
	projectName := beadInfo.Project
	if proj != nil {
		projectName = proj.Name
	}

	// Save session state
	sess := &session.Session{
		Bead:      beadID,
		Project:   projectName,
		Worktree:  worktreePath,
		Branch:    beadID,
		BeadsDir:  beadsDir,
		Status:    "working",
		CreatedAt: session.Now(),
	}
	sess.UpdateActivity()

	state.Sessions[sessionName] = sess
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Printf("\nSession '%s' ready.\n", sessionName)
	fmt.Printf("  Bead:     %s\n", beadID)
	fmt.Printf("  Worktree: %s\n", worktreePath)
	fmt.Printf("  Branch:   %s\n", beadID)

	// Switch to session unless --no-switch
	if !flags.noSwitch {
		fmt.Println("\nSwitching...")
		return tmux.Attach(sessionName)
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

func cmdProject(cfg *config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: wt project <add|config> ...")
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
	case "remove", "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt project remove <name>")
		}
		return cmdProjectRemove(mgr, args[1])
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

func cmdProjectRemove(mgr *project.Manager, name string) error {
	if err := mgr.Delete(name); err != nil {
		return err
	}

	fmt.Printf("Project '%s' removed.\n", name)
	return nil
}
