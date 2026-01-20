package main

import (
	"fmt"
	"os"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/doctor"
)

// Version information - set via ldflags at build time
// Example: go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// Global output format flag
var outputJSON bool

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

	// Parse global --json flag
	args = parseGlobalFlags(args)

	// No args or "list" → list sessions
	if len(args) == 0 || args[0] == "list" {
		return cmdList(cfg)
	}

	switch args[0] {
	case "new":
		if len(args) < 2 || hasHelpFlag(args[1:]) {
			return cmdNewHelp()
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
	case "signal":
		if len(args) < 2 {
			return fmt.Errorf("usage: wt signal <status> [message]\n  status: ready, blocked, error, working")
		}
		return cmdSignal(cfg, args[1:])
	case "abandon":
		return cmdAbandon(cfg)
	case "watch":
		if hasHelpFlag(args[1:]) {
			return cmdWatchHelp()
		}
		return cmdWatch(cfg)
	case "seance":
		return cmdSeance(cfg, args[1:])
	case "projects":
		// Alias for backward compatibility
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
		if len(args) < 2 || hasHelpFlag(args[1:]) {
			return cmdProjectHelp()
		}
		// "wt project list" is same as "wt project" with no subcommand
		if args[1] == "list" {
			return cmdProjects(cfg)
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
		if hasHelpFlag(args[1:]) {
			return cmdHubHelp()
		}
		return cmdHub(cfg, args[1:])
	default:
		// Assume it's a session name or bead ID to switch to
		return cmdSwitch(cfg, args[0])
	}
}

// parseGlobalFlags extracts global flags like --json from args
func parseGlobalFlags(args []string) []string {
	var filtered []string
	for _, arg := range args {
		if arg == "--json" {
			outputJSON = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}
