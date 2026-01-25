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

	// No args → show help
	if len(args) == 0 {
		return cmdHelp()
	}
	if args[0] == "list" {
		if hasHelpFlag(args[1:]) {
			return cmdListHelp()
		}
		return cmdList(cfg, args[1:])
	}

	switch args[0] {
	case "new":
		if len(args) < 2 || hasHelpFlag(args[1:]) {
			return cmdNewHelp()
		}
		return cmdNew(cfg, args[1:])
	case "kill":
		if hasHelpFlag(args[1:]) {
			return cmdKillHelp()
		}
		if len(args) < 2 {
			return cmdKillHelp()
		}
		return cmdKill(cfg, args[1], parseKillFlags(args[2:]))
	case "close":
		if hasHelpFlag(args[1:]) {
			return cmdCloseHelp()
		}
		if len(args) < 2 {
			return cmdCloseHelp()
		}
		return cmdClose(cfg, args[1])
	case "done":
		if hasHelpFlag(args[1:]) {
			return cmdDoneHelp()
		}
		return cmdDone(cfg, parseDoneFlags(args[1:]))
	case "status":
		if hasHelpFlag(args[1:]) {
			return cmdStatusHelp()
		}
		return cmdStatus(cfg)
	case "signal":
		if hasHelpFlag(args[1:]) {
			return cmdSignalHelp()
		}
		if len(args) < 2 {
			return cmdSignalHelp()
		}
		return cmdSignal(cfg, args[1:])
	case "abandon":
		if hasHelpFlag(args[1:]) {
			return cmdAbandonHelp()
		}
		return cmdAbandon(cfg)
	case "watch":
		if hasHelpFlag(args[1:]) {
			return cmdWatchHelp()
		}
		return cmdWatch(cfg)
	case "seance":
		if hasHelpFlag(args[1:]) {
			return cmdSeanceHelp()
		}
		return cmdSeance(cfg, args[1:])
	case "projects":
		if hasHelpFlag(args[1:]) {
			return cmdProjectsHelp()
		}
		return cmdProjects(cfg)
	case "ready":
		if hasHelpFlag(args[1:]) {
			return cmdReadyHelp()
		}
		var projectFilter string
		if len(args) > 1 {
			projectFilter = args[1]
		}
		return cmdReady(cfg, projectFilter)
	case "create":
		if hasHelpFlag(args[1:]) {
			return cmdCreateHelp()
		}
		if len(args) < 3 {
			return cmdCreateHelp()
		}
		return cmdCreate(cfg, args[1], args[2:])
	case "beads":
		if hasHelpFlag(args[1:]) {
			return cmdBeadsHelp()
		}
		if len(args) < 2 {
			return cmdBeadsHelp()
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
		if hasHelpFlag(args[1:]) {
			return cmdAutoHelp()
		}
		return cmdAuto(cfg, args[1:])
	case "events":
		if hasHelpFlag(args[1:]) {
			return cmdEventsHelp()
		}
		return cmdEvents(cfg, args[1:])
	case "doctor":
		if hasHelpFlag(args[1:]) {
			return cmdDoctorHelp()
		}
		return doctor.Run(cfg)
	case "config":
		if hasHelpFlag(args[1:]) {
			return cmdConfigHelp()
		}
		return cmdConfig(cfg, args[1:])
	case "pick":
		if hasHelpFlag(args[1:]) {
			return cmdPickHelp()
		}
		return cmdPick(cfg)
	case "keys":
		if hasHelpFlag(args[1:]) {
			return cmdKeysHelp()
		}
		return cmdKeys()
	case "completion":
		if hasHelpFlag(args[1:]) {
			return cmdCompletionHelp()
		}
		if len(args) < 2 {
			return cmdCompletionHelp()
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
	case "checkpoint":
		return cmdCheckpoint(cfg, args[1:])
	case "hub":
		if hasHelpFlag(args[1:]) {
			return cmdHubHelp()
		}
		return cmdHub(cfg, args[1:])
	case "task":
		if len(args) < 2 || hasHelpFlag(args[1:]) {
			return cmdTaskHelp()
		}
		return cmdTask(cfg, args[1:])
	case "bead":
		if len(args) < 2 || hasHelpFlag(args[1:]) {
			return cmdBeadHelp()
		}
		return cmdBead(cfg, args[1:])
	case "audit":
		if len(args) < 2 || hasHelpFlag(args[1:]) {
			return cmdAuditHelp()
		}
		return cmdAudit(cfg, args[1:])
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
