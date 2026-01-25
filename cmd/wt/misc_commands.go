package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"

	"github.com/badri/wt/internal/auto"
	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/session"
)

// cmdAuto runs autonomous batch processing of beads
func cmdAuto(cfg *config.Config, args []string) error {
	opts := parseAutoFlags(args)
	runner := auto.NewRunner(cfg, opts)
	return runner.Run()
}

func parseAutoFlags(args []string) *auto.Options {
	opts := &auto.Options{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--project":
			if i+1 < len(args) {
				opts.Project = args[i+1]
				i++
			}
		case "-m", "--merge-mode":
			if i+1 < len(args) {
				opts.MergeMode = args[i+1]
				i++
			}
		case "--timeout":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.Timeout)
				i++
			}
		case "-n", "--limit":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &opts.Limit)
				i++
			}
		case "--epic", "-e":
			if i+1 < len(args) {
				opts.Epic = args[i+1]
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
		case "--pause-on-failure":
			opts.PauseOnFailure = true
		case "--skip-audit":
			opts.SkipAudit = true
		case "--resume":
			opts.Resume = true
		case "--abort":
			opts.Abort = true
		}
	}
	return opts
}

// cmdAutoHelp prints help for the auto command
func cmdAutoHelp() error {
	help := `wt auto - Autonomous batch processing of beads in an epic

USAGE:
    wt auto --epic <id> [options]

DESCRIPTION:
    Processes all ready beads in an epic sequentially in a single worktree.
    Creates one PR/merge at the end instead of one per bead.

    IMPORTANT: --epic is required. This prevents accidental batch processing
    of all ready beads. Group related work into an epic first.

CORE CONCEPT:
    Instead of 1 worktree per bead (N PRs), creates 1 worktree for the epic:
    1. Runs implicit audit (catches issues early)
    2. Creates single worktree from project's default branch
    3. Processes beads sequentially, each building on previous work
    4. Creates 1 PR/merge at the end

OPTIONS:
    -e, --epic <id>         (required) Epic ID to process
    -m, --merge-mode <mode> Merge mode: direct, pr-auto, pr-review
    --timeout <minutes>     Per-bead timeout in minutes (default: 30)
    --dry-run               Preview what would be processed (includes audit)
    --pause-on-failure      Stop and preserve worktree if a bead fails
    --skip-audit            Bypass implicit audit (use with caution)
    --check                 Check status of running/paused auto session
    --resume                Resume a paused or failed epic run
    --abort                 Abort and clean up a paused/failed run
    --stop                  Stop the auto runner gracefully
    --force                 Force start even if another auto is running

WORKFLOW:
    1. Group work into an epic:
       bd create "Documentation batch" -t epic
       bd dep add wt-tcf wt-doc-epic
       bd dep add wt-1a3 wt-doc-epic

    2. Run batch processing:
       wt auto --epic wt-doc-epic

    3. If a bead fails with --pause-on-failure:
       - Fix manually in the preserved worktree
       - wt auto --resume    (continue from where it stopped)
       - wt auto --abort     (clean up and abandon)

IMPLICIT AUDIT:
    Before starting, wt auto checks:
    - All beads have descriptions
    - No external blockers (beads outside the epic)
    - Beads are ready for implementation

    Use --skip-audit to bypass if you've already resolved warnings.

EXAMPLES:
    wt auto --epic wt-doc-batch           Process beads in epic
    wt auto --epic wt-xyz --dry-run       Preview without executing
    wt auto --epic wt-xyz --pause-on-failure  Stop on first failure
    wt auto --check                       Check status of current run
    wt auto --resume                      Resume after failure
    wt auto --abort                       Clean up failed run
`
	fmt.Print(help)
	return nil
}

// cmdEventsHelp shows help for the events command
func cmdEventsHelp() error {
	help := `wt events - Show event history

USAGE:
    wt events [options]

DESCRIPTION:
    Shows the history of wt events (session starts, completions, etc).

OPTIONS:
    --since <duration>  Show events since duration (e.g., 1h, 24h, 7d)
    -f, --follow        Tail mode - watch for new events
    -n <count>          Number of events to show (default: 20)
    -h, --help          Show this help

EXAMPLES:
    wt events               Show last 20 events
    wt events --since 24h   Show events from the last 24 hours
    wt events -f            Watch for new events
    wt events -n 50         Show last 50 events
`
	fmt.Print(help)
	return nil
}

// cmdConfigHelp shows help for the config command
func cmdConfigHelp() error {
	help := `wt config - Manage wt configuration

USAGE:
    wt config [command] [options]

COMMANDS:
    (none), show        Show current configuration
    init                Create config file with defaults
    set <key> <value>   Set a configuration value
    edit                Open config in editor

CONFIG KEYS:
    worktree_root       Directory where worktrees are created
    editor_cmd          Editor command for config editing
    default_merge_mode  Default merge mode: direct, pr-auto, pr-review

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt config                           Show current config
    wt config init                      Create config file
    wt config set worktree_root ~/wt    Set worktree directory
    wt config edit                      Open config in editor
`
	fmt.Print(help)
	return nil
}

// cmdPickHelp shows help for the pick command
func cmdPickHelp() error {
	help := `wt pick - Interactive session picker

USAGE:
    wt pick

DESCRIPTION:
    Opens an interactive picker to select and switch to a session.
    Uses fzf if available, otherwise falls back to a numbered list.

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt pick             Open session picker
`
	fmt.Print(help)
	return nil
}

// cmdKeysHelp shows help for the keys command
func cmdKeysHelp() error {
	help := `wt keys - Output tmux keybinding suggestions

USAGE:
    wt keys

DESCRIPTION:
    Outputs suggested tmux keybindings for wt commands.
    Add these to your ~/.tmux.conf file.

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt keys                     Show keybinding suggestions
    wt keys >> ~/.tmux.conf     Append to tmux config
`
	fmt.Print(help)
	return nil
}

// cmdCompletionHelp shows help for the completion command
func cmdCompletionHelp() error {
	help := `wt completion - Generate shell completions

USAGE:
    wt completion <shell>

DESCRIPTION:
    Generates shell completion scripts for bash, zsh, or fish.

ARGUMENTS:
    <shell>             Shell type: bash, zsh, fish

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt completion bash          Generate bash completions
    wt completion zsh           Generate zsh completions
    wt completion fish          Generate fish completions

    # Add to shell config:
    eval "$(wt completion bash)"     # bash
    eval "$(wt completion zsh)"      # zsh
    wt completion fish > ~/.config/fish/completions/wt.fish
`
	fmt.Print(help)
	return nil
}

// cmdDoctorHelp shows help for the doctor command
func cmdDoctorHelp() error {
	help := `wt doctor - Check system requirements

USAGE:
    wt doctor

DESCRIPTION:
    Checks that all required tools are installed and configured correctly.
    Validates git, tmux, bd (beads), and other dependencies.

OPTIONS:
    -h, --help          Show this help

EXAMPLES:
    wt doctor           Run system check
`
	fmt.Print(help)
	return nil
}

// cmdEvents shows wt events
func cmdEvents(cfg *config.Config, args []string) error {
	logger := events.NewLogger(cfg)

	// Parse flags
	var since time.Duration
	var tail bool
	var count int = 20

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--since":
			if i+1 < len(args) {
				d, err := time.ParseDuration(args[i+1])
				if err == nil {
					since = d
				}
				i++
			}
		case "-f", "--follow":
			tail = true
		case "-n", "--count":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &count)
				i++
			}
		}
	}

	// Tail mode
	if tail {
		fmt.Println("Watching events... (Ctrl+C to exit)")
		ctx := make(chan struct{})
		eventCh := make(chan events.Event, 10)

		go func() {
			logger.Tail(nil, eventCh)
		}()

		for {
			select {
			case e := <-eventCh:
				printEvent(&e)
			case <-ctx:
				return nil
			}
		}
	}

	// Get events
	var evts []events.Event
	var err error

	if since > 0 {
		evts, err = logger.Since(since)
	} else {
		evts, err = logger.Recent(count)
	}
	if err != nil {
		return err
	}

	if len(evts) == 0 {
		printEmptyMessage("No events found.", "")
		return nil
	}

	// Define columns
	columns := []table.Column{
		{Title: "Time", Width: 19},
		{Title: "", Width: 1},
		{Title: "Type", Width: 14},
		{Title: "Project", Width: 12},
		{Title: "Bead", Width: 18},
		{Title: "Session", Width: 12},
	}

	// Build rows
	var rows []table.Row
	for _, e := range evts {
		t, _ := time.Parse(time.RFC3339, e.Time)
		timeStr := t.Format("2006-01-02 15:04:05")
		icon := getEventIcon(e.Type)

		rows = append(rows, table.Row{
			timeStr,
			icon,
			string(e.Type),
			truncate(e.Project, 12),
			truncate(e.Bead, 18),
			truncate(e.Session, 12),
		})
	}

	printTable("Recent Events", columns, rows)

	return nil
}

func getEventIcon(eventType events.EventType) string {
	switch eventType {
	case events.EventSessionStart:
		return ">"
	case events.EventSessionEnd:
		return "#"
	case events.EventSessionKill:
		return "x"
	case events.EventHubHandoff:
		return "~"
	case events.EventPRCreated:
		return "^"
	case events.EventPRMerged:
		return "+"
	default:
		return "*"
	}
}

func printEvent(e *events.Event) {
	t, _ := time.Parse(time.RFC3339, e.Time)
	timeStr := t.Format("2006-01-02 15:04:05")
	icon := getEventIcon(e.Type)

	fmt.Printf("%s %s %-14s %-12s %-18s %s\n",
		timeStr, icon, e.Type, e.Project, e.Bead, e.Session)
}

// cmdConfig manages wt configuration
func cmdConfig(cfg *config.Config, args []string) error {
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
	case "edit", "editor":
		return configEditor(cfg)
	default:
		return fmt.Errorf("unknown config command: %s\nUsage: wt config [show|init|set|edit]", args[0])
	}
}

func showConfig(cfg *config.Config) error {
	fmt.Println("wt Configuration")
	fmt.Println("---------------------------------------------")
	fmt.Printf("  Config dir:       %s\n", cfg.ConfigDir())
	fmt.Printf("  Config file:      %s\n", cfg.ConfigPath())
	fmt.Printf("  Worktree root:    %s\n", cfg.WorktreeRoot)
	fmt.Printf("  Editor command:   %s\n", cfg.EditorCmd)
	fmt.Printf("  Default merge:    %s\n", cfg.DefaultMergeMode)
	fmt.Printf("  Sessions file:    %s\n", cfg.SessionsPath())
	fmt.Printf("  Namepool file:    %s\n", cfg.NamepoolPath())

	// Show if config exists
	if cfg.ConfigExists() {
		fmt.Println("\n  Config file exists: yes")
	} else {
		fmt.Println("\n  Config file exists: no (using defaults)")
		fmt.Println("  Run 'wt config init' to create config file")
	}

	return nil
}

func initConfig(cfg *config.Config) error {
	if cfg.ConfigExists() {
		return fmt.Errorf("config file already exists at %s\nUse 'wt config edit' to modify", cfg.ConfigPath())
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("creating config: %w", err)
	}

	fmt.Printf("Created config file: %s\n", cfg.ConfigPath())
	fmt.Println("\nEdit with: wt config edit")
	return nil
}

func setConfig(cfg *config.Config, key, value string) error {
	switch key {
	case "worktree_root":
		cfg.WorktreeRoot = value
	case "editor_cmd":
		cfg.EditorCmd = value
	case "default_merge_mode":
		if value != "direct" && value != "pr-auto" && value != "pr-review" {
			return fmt.Errorf("invalid merge mode: %s\nValid: direct, pr-auto, pr-review", value)
		}
		cfg.DefaultMergeMode = value
	default:
		return fmt.Errorf("unknown config key: %s\nValid keys: worktree_root, editor_cmd, default_merge_mode", key)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}

func configEditor(cfg *config.Config) error {
	// Ensure config exists
	if !cfg.ConfigExists() {
		if err := cfg.Save(); err != nil {
			return err
		}
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	cmd := exec.Command(editor, cfg.ConfigPath())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// pickerEntry represents an entry in the session picker
type pickerEntry struct {
	name    string
	bead    string
	title   string
	project string
	status  string
}

// cmdPick launches an interactive session picker using fzf
func cmdPick(cfg *config.Config) error {
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	if len(state.Sessions) == 0 {
		fmt.Println("No active sessions to pick from.")
		return nil
	}

	// Build entries
	var entries []pickerEntry
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

		entries = append(entries, pickerEntry{
			name:    name,
			bead:    sess.Bead,
			title:   title,
			project: sess.Project,
			status:  status,
		})
	}

	// Check for fzf
	if hasFzf() {
		return pickWithFzf(entries)
	}

	return pickWithPrompt(entries)
}

func hasFzf() bool {
	_, err := exec.LookPath("fzf")
	return err == nil
}

func pickWithFzf(entries []pickerEntry) error {
	// Build input for fzf
	var lines []string
	for _, e := range entries {
		titleStr := e.title
		if len(titleStr) > 30 {
			titleStr = titleStr[:28] + ".."
		}
		line := fmt.Sprintf("%-12s %-10s %-30s %s", e.name, e.status, titleStr, e.project)
		lines = append(lines, line)
	}
	input := strings.Join(lines, "\n")

	// Run fzf
	cmd := exec.Command("fzf", "--header=Name         Status     Title                          Project",
		"--prompt=Select session: ",
		"--height=40%",
		"--reverse")
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		// User cancelled or fzf error
		return nil
	}

	// Parse selection (first field is name)
	selection := strings.TrimSpace(string(output))
	if selection == "" {
		return nil
	}

	fields := strings.Fields(selection)
	if len(fields) == 0 {
		return nil
	}

	sessionName := fields[0]

	// Switch to session using tmux
	fmt.Printf("Switching to session: %s\n", sessionName)
	switchCmd := exec.Command("tmux", "switch-client", "-t", sessionName)
	return switchCmd.Run()
}

func pickWithPrompt(entries []pickerEntry) error {
	fmt.Println("Active Sessions:")
	fmt.Println("--------------------------------------------------------------------------------")
	for i, e := range entries {
		titleStr := e.title
		if len(titleStr) > 30 {
			titleStr = titleStr[:28] + ".."
		}
		fmt.Printf("  [%d] %-12s %-10s %-30s %s\n", i+1, e.name, e.status, titleStr, e.project)
	}
	fmt.Println()

	fmt.Print("Select session (number or name): ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return err
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Try number first
	var idx int
	if _, err := fmt.Sscanf(input, "%d", &idx); err == nil {
		if idx > 0 && idx <= len(entries) {
			sessionName := entries[idx-1].name
			fmt.Printf("Switching to session: %s\n", sessionName)
			cmd := exec.Command("tmux", "switch-client", "-t", sessionName)
			return cmd.Run()
		}
	}

	// Try name match
	for _, e := range entries {
		if e.name == input || strings.HasPrefix(e.name, input) {
			fmt.Printf("Switching to session: %s\n", e.name)
			cmd := exec.Command("tmux", "switch-client", "-t", e.name)
			return cmd.Run()
		}
	}

	return fmt.Errorf("no session matching '%s'", input)
}

// cmdKeys outputs tmux keybinding configuration
func cmdKeys() error {
	keybindings := `# wt tmux keybindings
# Add these to your ~/.tmux.conf

# Session management
bind-key W display-popup -E -w 80% -h 60% "wt pick"
bind-key N command-prompt -p "bead:" "run-shell 'wt new %%'"
bind-key K command-prompt -p "session:" "run-shell 'wt kill %%'"

# Quick actions
bind-key S run-shell "wt status"
bind-key L run-shell "wt list"
bind-key R run-shell "wt ready"

# Watch mode in a popup
bind-key M display-popup -E -w 90% -h 80% "wt watch"

# Hub control
bind-key H run-shell "wt hub"
bind-key D run-shell "wt hub --detach"

# Signal shortcuts (from within a session)
bind-key F1 run-shell "wt signal ready"
bind-key F2 run-shell "wt signal blocked"
bind-key F3 run-shell "wt signal error"

# Reload this config
# bind-key r source-file ~/.tmux.conf \; display "Reloaded!"
`
	fmt.Print(keybindings)
	return nil
}

// cmdVersion prints version information
func cmdVersion() error {
	fmt.Printf("wt %s\n", version)
	fmt.Printf("  commit: %s\n", commit)
	fmt.Printf("  built:  %s\n", date)
	fmt.Printf("  go:     %s\n", runtime.Version())
	fmt.Printf("  os:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return nil
}

// cmdHelp prints categorized help information
func cmdHelp() error {
	help := `wt - Claude Code orchestrator

USAGE:
    wt [command] [options]

SESSION COMMANDS:
    wt list                 List all active sessions
    wt new <bead>           Create new session for a bead
                            Options: --repo <path>, --name <name>, --no-switch, --no-test-env
    wt <name>               Switch to session by name or bead ID
    wt kill <name>          Terminate session (keeps bead open)
                            Options: --keep-worktree
    wt close <name>         Complete session and close bead
    wt done                 Complete current session with merge
                            Options: --merge-mode <mode>
    wt abandon              Abandon current session without merge
    wt status               Show current session status
    wt signal <status>      Update session status (ready, blocked, error, working, idle)
    wt pick                 Interactive session picker (uses fzf if available)

PROJECT COMMANDS:
    wt projects             List registered projects
    wt project add <n> <p>  Register a project
    wt project config <n>   Edit project configuration
    wt project remove <n>   Unregister a project
    wt ready [project]      Show beads ready to work on
    wt beads <project>      List beads for a project
                            Options: --status <status>
    wt create <proj> <title> Create a new bead in project
                            Options: --description, --priority, --type
    wt audit <bead>         Audit bead readiness for implementation
                            Options: -i/--interactive, -p/--project

HUB COMMANDS:
    wt hub                  Start or attach to hub session
                            Options: -d/--detach, -s/--status, -k/--kill
    wt watch                Live dashboard of all sessions
    wt auto                 Autonomous batch processing
                            Options: --project, --merge-mode, --timeout, --dry-run, --check, --stop

HISTORY COMMANDS:
    wt seance               List past sessions for resumption
    wt seance <name>        Resume in new tmux pane (safe from hub)
    wt seance <name> --spawn  Spawn new tmux session for seance
    wt seance <name> -p 'q' One-shot query to past session
    wt events               Show event history
                            Options: --since <duration>, -f/--follow, -n <count>

HANDOFF COMMANDS:
    wt handoff              Hand off to fresh Claude instance
                            Options: -m <message>, -c/--collect, --dry-run
    wt prime                Inject context on session startup
                            Options: -q/--quiet, --no-bd-prime, --hook
                            --hook: Read session_id from Claude SessionStart hook JSON on stdin

CONFIGURATION:
    wt config               Show current configuration
    wt config init          Create config file with defaults
    wt config set <k> <v>   Set a config value
    wt config edit          Open config in editor
    wt keys                 Output tmux keybinding suggestions
    wt doctor               Check system requirements

OTHER:
    wt completion <shell>   Generate shell completion (bash, zsh, fish)
    wt version              Show version information
    wt help                 Show this help

EXAMPLES:
    wt new wt-123                     Start working on bead wt-123
    wt signal ready "PR created"      Signal that work is ready
    wt hub                            Start orchestrating workers
    wt seance toast -p "What did you change?"   Ask past session

For more information: https://github.com/badri/wt
`
	fmt.Print(help)
	return nil
}

// cmdCompletion generates shell completion scripts
func cmdCompletion(shell string) error {
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
		return nil
	case "zsh":
		fmt.Print(zshCompletion)
		return nil
	case "fish":
		fmt.Print(fishCompletion)
		return nil
	default:
		return fmt.Errorf("unsupported shell: %s\nSupported: bash, zsh, fish", shell)
	}
}

const bashCompletion = `# wt bash completion
# Add to ~/.bashrc: eval "$(wt completion bash)"

_wt_completions() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    commands="list new kill close done status abandon watch seance projects ready create beads project auto events doctor config pick keys completion version help hub handoff prime signal"

    case "${prev}" in
        wt)
            COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
            return 0
            ;;
        new)
            local beads=$(wt ready 2>/dev/null | awk '/^[|]/ {print $2}' | grep -v '^$' | grep -v '^Bead')
            COMPREPLY=( $(compgen -W "${beads}" -- "${cur}") )
            return 0
            ;;
        kill|close)
            local sessions=$(wt list 2>/dev/null | awk '/^[|]/ {print $2}' | grep -v '^$' | grep -v '^Name')
            COMPREPLY=( $(compgen -W "${sessions}" -- "${cur}") )
            return 0
            ;;
        project)
            COMPREPLY=( $(compgen -W "add config remove" -- "${cur}") )
            return 0
            ;;
        config)
            COMPREPLY=( $(compgen -W "show init set edit" -- "${cur}") )
            return 0
            ;;
        signal)
            COMPREPLY=( $(compgen -W "ready blocked error working idle" -- "${cur}") )
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
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
    local -a commands

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
        'config:Configuration management'
        'pick:Interactive session picker'
        'keys:Output tmux keybindings'
        'completion:Generate shell completions'
        'version:Show version information'
        'help:Show help'
        'hub:Hub session management'
        'handoff:Hand off to fresh Claude'
        'prime:Inject context on startup'
        'signal:Update session status'
    )

    _arguments -C \
        '1: :->command' \
        '*: :->args'

    case $state in
        command)
            _describe 'command' commands
            ;;
        args)
            case $words[2] in
                project)
                    _describe 'subcommand' '(add config remove)'
                    ;;
                config)
                    _describe 'subcommand' '(show init set edit)'
                    ;;
                signal)
                    _describe 'status' '(ready blocked error working idle)'
                    ;;
                completion)
                    _describe 'shell' '(bash zsh fish)'
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
complete -c wt -n __fish_use_subcommand -a config -d 'Configuration management'
complete -c wt -n __fish_use_subcommand -a pick -d 'Interactive session picker'
complete -c wt -n __fish_use_subcommand -a keys -d 'Output tmux keybindings'
complete -c wt -n __fish_use_subcommand -a completion -d 'Generate shell completions'
complete -c wt -n __fish_use_subcommand -a version -d 'Show version information'
complete -c wt -n __fish_use_subcommand -a help -d 'Show help'
complete -c wt -n __fish_use_subcommand -a hub -d 'Hub session management'
complete -c wt -n __fish_use_subcommand -a handoff -d 'Hand off to fresh Claude'
complete -c wt -n __fish_use_subcommand -a prime -d 'Inject context on startup'
complete -c wt -n __fish_use_subcommand -a signal -d 'Update session status'

# Completions for 'project' subcommand
complete -c wt -n '__fish_seen_subcommand_from project' -a 'add config remove' -d 'Project subcommand'

# Completions for 'config' subcommand
complete -c wt -n '__fish_seen_subcommand_from config' -a 'show init set edit' -d 'Config subcommand'

# Completions for 'signal' subcommand
complete -c wt -n '__fish_seen_subcommand_from signal' -a 'ready blocked error working idle' -d 'Status'

# Completions for 'completion' - shell types
complete -c wt -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish' -d 'Shell'
`
