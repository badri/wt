package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/session"
)

type beadFlags struct {
	description string
	priority    int
	issueType   string
}

func cmdBeadHelp() error {
	help := `wt bead - Manage beads from within a session

USAGE:
    wt bead <subcommand> [options]

SUBCOMMANDS:
    create <title>          Create a new bead in the current project

CREATE OPTIONS:
    --description, -d <text>    Description for the bead
    --priority, -p <0-4>        Priority level (0=critical, 4=backlog, default: 2)
    --type, -t <type>           Issue type (task, bug, feature, default: task)
    -h, --help                  Show this help

EXAMPLES:
    wt bead create "Fix slow query performance"
    wt bead create "Add user authentication" --type feature --priority 1
    wt bead create "Memory leak in worker" --type bug -d "Discovered during investigation"

NOTES:
    - Must be run from within a wt session worktree
    - Creates the bead in the session's project
    - For task sessions, captures context from the task description
`
	fmt.Print(help)
	return nil
}

func parseBeadCreateFlags(args []string) (title string, flags beadFlags) {
	flags.priority = 2 // default priority

	var titleParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--description", "-d":
			if i+1 < len(args) {
				flags.description = args[i+1]
				i++
			}
		case "--priority", "-p":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &flags.priority)
				i++
			}
		case "--type", "-t":
			if i+1 < len(args) {
				flags.issueType = args[i+1]
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				titleParts = append(titleParts, args[i])
			}
		}
	}

	title = strings.Join(titleParts, " ")
	if flags.issueType == "" {
		flags.issueType = "task"
	}
	return
}

func cmdBead(cfg *config.Config, args []string) error {
	if len(args) == 0 {
		return cmdBeadHelp()
	}

	switch args[0] {
	case "create":
		if len(args) < 2 || hasHelpFlag(args[1:]) {
			return cmdBeadHelp()
		}
		return cmdBeadCreate(cfg, args[1:])
	case "help", "-h", "--help":
		return cmdBeadHelp()
	default:
		return fmt.Errorf("unknown bead subcommand: %s\nRun 'wt bead help' for usage", args[0])
	}
}

func cmdBeadCreate(cfg *config.Config, args []string) error {
	title, flags := parseBeadCreateFlags(args)

	if title == "" {
		return fmt.Errorf("bead title required. Usage: wt bead create <title> [options]")
	}

	// Find current session from working directory
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

	// Ensure we have a beads directory
	if sess.BeadsDir == "" {
		return fmt.Errorf("no beads directory configured for this session")
	}

	// Build description with context from task session
	description := flags.description
	if sess.IsTask() && sess.TaskDescription != "" {
		if description == "" {
			description = fmt.Sprintf("Discovered during task: %s", sess.TaskDescription)
		} else {
			description = fmt.Sprintf("%s\n\nDiscovered during task: %s", description, sess.TaskDescription)
		}
	}

	// Create the bead
	opts := &bead.CreateOptions{
		Description: description,
		Priority:    flags.priority,
		Type:        flags.issueType,
	}

	beadID, err := bead.CreateInDir(sess.BeadsDir, title, opts)
	if err != nil {
		return fmt.Errorf("creating bead: %w", err)
	}

	fmt.Printf("Created bead: %s\n", beadID)
	fmt.Printf("  Title:       %s\n", title)
	fmt.Printf("  Type:        %s\n", flags.issueType)
	fmt.Printf("  Priority:    P%d\n", flags.priority)
	if description != "" {
		// Show truncated description
		desc := description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Printf("  Description: %s\n", desc)
	}
	fmt.Printf("  Session:     %s\n", sessionName)

	return nil
}
