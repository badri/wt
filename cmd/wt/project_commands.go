package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/table"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
)

// cmdProjectHelp shows detailed help for the project command
func cmdProjectHelp() error {
	help := `wt project - Manage registered projects

USAGE:
    wt project [command] [options]

COMMANDS:
    (none), list        List all registered projects
    add <name> <path>   Register a new project
    config <name>       Edit project configuration in editor
    remove <name>       Unregister a project

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt project                      List all projects
    wt project list                 Same as above
    wt project add myproj ~/code/myproj    Register project
    wt project config myproj        Edit myproj's configuration
    wt project remove myproj        Unregister myproj

SEE ALSO:
    wt ready [project]     Show beads ready to work on
    wt beads <project>     List all beads for a project
    wt create <proj> <t>   Create a new bead in project
`
	fmt.Print(help)
	return nil
}

func cmdProjects(cfg *config.Config) error {
	return printProjectsList(cfg)
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
		msg := "No ready beads across all projects."
		if projectFilter != "" {
			msg = fmt.Sprintf("No ready beads for project '%s'.", projectFilter)
		}
		printEmptyMessage(msg, "All caught up!")
		return nil
	}

	title := "Ready Work (all projects)"
	if projectFilter != "" {
		title = fmt.Sprintf("Ready Work (%s)", projectFilter)
	}

	// Define columns
	columns := []table.Column{
		{Title: "Bead", Width: 16},
		{Title: "Title", Width: 40},
		{Title: "Type", Width: 8},
		{Title: "Priority", Width: 8},
	}

	// Build rows
	var rows []table.Row
	for _, b := range allBeads {
		priority := fmt.Sprintf("P%d", b.Priority)
		rows = append(rows, table.Row{
			b.ID,
			truncate(b.Title, 40),
			b.IssueType,
			priority,
		})
	}

	printTable(title, columns, rows)
	fmt.Printf("\n%d bead(s) ready. Start with: wt new <bead>\n", len(allBeads))

	return nil
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
		printEmptyMessage(fmt.Sprintf("No beads%s in project '%s'.", statusMsg, projectName), "")
		return nil
	}

	title := fmt.Sprintf("Beads (%s)", projectName)
	if flags.status != "" {
		title = fmt.Sprintf("Beads (%s, %s)", projectName, flags.status)
	}

	// Define columns
	columns := []table.Column{
		{Title: "Bead", Width: 16},
		{Title: "Title", Width: 40},
		{Title: "Type", Width: 8},
		{Title: "Priority", Width: 8},
	}

	// Build rows
	var rows []table.Row
	for _, b := range beads {
		priority := fmt.Sprintf("P%d", b.Priority)
		rows = append(rows, table.Row{
			b.ID,
			truncate(b.Title, 40),
			b.IssueType,
			priority,
		})
	}

	printTable(title, columns, rows)
	fmt.Printf("\n%d bead(s) found.\n", len(beads))

	return nil
}
