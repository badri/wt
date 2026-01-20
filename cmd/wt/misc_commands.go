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
	help := `wt - Worktree session manager for AI-assisted development

USAGE:
    wt [command] [options]

SESSION COMMANDS:
    wt                      List all active sessions
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

HUB COMMANDS:
    wt hub                  Start or attach to hub session
                            Options: -d/--detach, -s/--status, -k/--kill
    wt watch                Live dashboard of all sessions
    wt auto                 Autonomous batch processing
                            Options: --project, --merge-mode, --timeout, --dry-run, --check, --stop

HISTORY COMMANDS:
    wt seance               List past sessions for resumption
    wt seance <name>        Resume conversation with past session
    wt seance <name> -p 'q' One-shot query to past session
    wt events               Show event history
                            Options: --since <duration>, -f/--follow, -n <count>

HANDOFF COMMANDS:
    wt handoff              Hand off to fresh Claude instance
                            Options: -m <message>, -c/--collect, --dry-run
    wt prime                Inject context on session startup
                            Options: -q/--quiet, --no-bd-prime

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
