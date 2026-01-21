package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/namepool"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/testenv"
	"github.com/badri/wt/internal/tmux"
	"github.com/badri/wt/internal/worktree"
)

type taskFlags struct {
	project   string
	condition string
	name      string
	noSwitch  bool
	noTestEnv bool
}

func cmdTaskHelp() error {
	help := `wt task - Create a lightweight task session (non-bead)

USAGE:
    wt task <description> [options]

DESCRIPTION:
    Creates a git worktree and tmux session for transient work that
    doesn't require a bead. Use for quick investigations, MCP queries,
    PR conflict resolution, or other temporary tasks.

    Tasks have explicit completion conditions that determine what
    'wt done' checks before completing.

ARGUMENTS:
    <description>           Short description of the task

OPTIONS:
    --project <name>        Project to create task in (default: auto-detect)
    --condition <cond>      Completion condition (default: none)
                            Options: none, pr-merged, pushed, tests-pass, user-confirm
    --name <name>           Custom session name (default: generated)
    --no-switch             Don't switch to the new session after creation
    --no-test-env           Skip test environment setup
    -h, --help              Show this help

COMPLETION CONDITIONS:
    none          - No checks, task completes immediately with 'wt done'
    pr-merged     - Task waits for PR to be merged before cleanup
    pushed        - Task checks that changes are pushed to remote
    tests-pass    - Task runs test suite before completing
    user-confirm  - Task prompts user to confirm completion

EXAMPLES:
    wt task "Investigate slow query"
    wt task "Fix PR conflicts for wt-123" --condition user-confirm
    wt task "Run integration tests" --condition tests-pass --project myapp
    wt task "Quick refactor" --condition pushed

NOTES:
    - Tasks are lighter than beads - no issue tracking overhead
    - Use beads for strategic work that spans sessions
    - Use tasks for transient work that doesn't need tracking
`
	fmt.Print(help)
	return nil
}

func parseTaskFlags(args []string) (description string, flags taskFlags) {
	var descParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if i+1 < len(args) {
				flags.project = args[i+1]
				i++
			}
		case "--condition":
			if i+1 < len(args) {
				flags.condition = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				flags.name = args[i+1]
				i++
			}
		case "--no-switch":
			flags.noSwitch = true
		case "--no-test-env":
			flags.noTestEnv = true
		default:
			// Collect non-flag arguments as description
			if !strings.HasPrefix(args[i], "-") {
				descParts = append(descParts, args[i])
			}
		}
	}
	description = strings.Join(descParts, " ")
	return
}

func cmdTask(cfg *config.Config, args []string) error {
	description, flags := parseTaskFlags(args)

	if description == "" {
		return fmt.Errorf("task description required. Usage: wt task <description> [options]")
	}

	// Validate completion condition
	condition := session.ConditionNone
	if flags.condition != "" {
		switch flags.condition {
		case "none":
			condition = session.ConditionNone
		case "pr-merged":
			condition = session.ConditionPRMerged
		case "pushed":
			condition = session.ConditionPushed
		case "tests-pass":
			condition = session.ConditionTestsPass
		case "user-confirm":
			condition = session.ConditionUserConfirm
		default:
			return fmt.Errorf("invalid completion condition: %s\nValid options: none, pr-merged, pushed, tests-pass, user-confirm", flags.condition)
		}
	}

	// Load state
	state, err := session.LoadState(cfg)
	if err != nil {
		return err
	}

	// Determine project and repo
	mgr := project.NewManager(cfg)
	var proj *project.Project
	var repoPath string

	if flags.project != "" {
		proj, err = mgr.Get(flags.project)
		if err != nil {
			return fmt.Errorf("project not found: %s", flags.project)
		}
		repoPath = proj.RepoPath()
	} else {
		// Try to auto-detect from current directory
		repoPath, err = worktree.FindGitRoot()
		if err != nil {
			return fmt.Errorf("not in a git repository. Use --project <name> to specify a project")
		}

		// Try to find matching project
		projects, _ := mgr.List()
		for _, p := range projects {
			if p.RepoPath() == repoPath {
				proj = p
				break
			}
		}
	}

	// Allocate name from themed pool
	var pool *namepool.Pool
	projectName := ""
	if proj != nil {
		projectName = proj.Name
		pool, err = namepool.LoadForProject(projectName)
		if err != nil {
			return err
		}
		fmt.Printf("Using theme: %s\n", pool.Theme())
	} else {
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
		// Prefix with "task-" to distinguish from bead sessions
		if projectName != "" {
			sessionName = projectName + "-task-" + themeName
		} else {
			sessionName = "task-" + themeName
		}
	}

	// Create branch name from sanitized description
	branchName := sanitizeBranchName("task/" + description)

	// Create worktree
	worktreePath := cfg.WorktreePath(sessionName)
	fmt.Printf("Creating git worktree at %s...\n", worktreePath)

	// Get default branch to branch from
	defaultBranch := "main"
	if proj != nil && proj.DefaultBranch != "" {
		defaultBranch = proj.DefaultBranch
	}

	if err := worktree.CreateFromBranch(repoPath, worktreePath, branchName, defaultBranch); err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	// Determine BEADS_DIR (main repo's .beads, even for tasks)
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
		worktree.Remove(worktreePath)
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Run test env setup if configured and not skipped
	if proj != nil && proj.TestEnv != nil && proj.TestEnv.Setup != "" && !flags.noTestEnv {
		fmt.Println("Running test environment setup...")
		if err := testenv.RunSetup(proj, worktreePath, portOffset); err != nil {
			fmt.Printf("Warning: test env setup failed: %v\n", err)
		}

		if proj.TestEnv.HealthCheck != "" {
			fmt.Println("Waiting for test environment to be ready...")
			if err := testenv.WaitForHealthy(proj, worktreePath, portOffset, 30*time.Second); err != nil {
				fmt.Printf("Warning: health check failed: %v\n", err)
			}
		}
	}

	// Run on_create hooks if configured
	if proj != nil && proj.Hooks != nil && len(proj.Hooks.OnCreate) > 0 {
		fmt.Println("Running on_create hooks...")
		if err := testenv.RunOnCreateHooks(proj, worktreePath, portOffset, portEnv); err != nil {
			fmt.Printf("Warning: on_create hook failed: %v\n", err)
		}
	}

	// Save session state
	sess := &session.Session{
		Bead:                "", // No bead for tasks
		Project:             projectName,
		Worktree:            worktreePath,
		Branch:              branchName,
		PortOffset:          portOffset,
		BeadsDir:            beadsDir,
		Status:              "working",
		CreatedAt:           session.Now(),
		Type:                session.SessionTypeTask,
		TaskDescription:     description,
		CompletionCondition: condition,
	}
	sess.UpdateActivity()

	state.Sessions[sessionName] = sess
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Log session start event
	eventLogger := events.NewLogger(cfg)
	eventLogger.LogSessionStart(sessionName, "task:"+description, projectName, worktreePath)

	fmt.Printf("\nTask session '%s' ready.\n", sessionName)
	fmt.Printf("  Task:       %s\n", description)
	fmt.Printf("  Worktree:   %s\n", worktreePath)
	fmt.Printf("  Branch:     %s\n", branchName)
	fmt.Printf("  Condition:  %s\n", condition)

	// Wait for Claude to start
	fmt.Println("Waiting for Claude to start...")
	if err := tmux.WaitForClaude(sessionName, 60*time.Second); err != nil {
		fmt.Printf("Warning: %v (sending prompt anyway)\n", err)
	}

	// Accept bypass permissions warning if present
	if err := tmux.AcceptBypassPermissionsWarning(sessionName); err != nil {
		fmt.Printf("Warning: could not accept bypass warning: %v\n", err)
	}

	time.Sleep(2 * time.Second)

	// Send initial task prompt
	fmt.Println("Sending initial prompt to worker...")
	prompt := buildTaskPrompt(description, condition, sessionName, proj)
	if err := tmux.NudgeSession(sessionName, prompt); err != nil {
		fmt.Printf("Warning: could not send initial prompt: %v\n", err)
	}

	// Determine if we should switch
	shouldSwitch := !flags.noSwitch
	if os.Getenv("WT_HUB") == "1" {
		shouldSwitch = false
		fmt.Println("\n(Running from hub - staying in hub. Use 'wt <name>' to attach)")
	}

	if shouldSwitch {
		fmt.Println("\nSwitching...")
		return tmux.Attach(sessionName)
	}

	return nil
}

// sanitizeBranchName converts a description to a valid git branch name
func sanitizeBranchName(s string) string {
	// Replace spaces and special chars with hyphens
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '/' || r == '-' {
			return r
		}
		return '-'
	}, s)

	// Remove consecutive hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}

	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-")

	// Truncate to reasonable length
	if len(s) > 50 {
		s = s[:50]
	}

	return s
}

// cmdDoneTask completes a task session based on its completion condition
func cmdDoneTask(cfg *config.Config, state *session.State, sessionName string, sess *session.Session, cwd string) error {
	fmt.Printf("Completing task session '%s'...\n", sessionName)
	fmt.Printf("  Task:      %s\n", sess.TaskDescription)
	fmt.Printf("  Branch:    %s\n", sess.Branch)
	fmt.Printf("  Condition: %s\n", sess.CompletionCondition)

	// Check completion condition
	switch sess.CompletionCondition {
	case session.ConditionNone:
		// No checks needed
		fmt.Println("\nNo completion condition - proceeding with cleanup.")

	case session.ConditionPushed:
		// Check if changes are pushed
		if err := checkChangesPushed(cwd); err != nil {
			return fmt.Errorf("completion check failed: %w", err)
		}
		fmt.Println("\n Changes verified as pushed.")

	case session.ConditionPRMerged:
		// Check if PR is merged (from status message)
		prURL := extractPRURL(sess.StatusMessage)
		if prURL == "" {
			return fmt.Errorf("no PR URL found in status message. Signal ready with PR URL first")
		}
		if err := checkPRMerged(cwd, prURL); err != nil {
			return fmt.Errorf("completion check failed: %w", err)
		}
		fmt.Printf("\n PR %s verified as merged.\n", prURL)

	case session.ConditionTestsPass:
		// Run tests and check they pass
		if err := runTestsAndCheck(cwd); err != nil {
			return fmt.Errorf("completion check failed: %w", err)
		}
		fmt.Println("\n Tests verified as passing.")

	case session.ConditionUserConfirm:
		// This should have been confirmed before calling done
		// Just proceed
		fmt.Println("\n User confirmation assumed.")
	}

	// Run teardown hooks if configured
	mgr := project.NewManager(cfg)
	if proj, _ := mgr.Get(sess.Project); proj != nil {
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
	eventLogger.LogSessionEnd(sessionName, "task:"+sess.TaskDescription, sess.Project, claudeSession, "task-completed", "")

	// Remove from state
	delete(state.Sessions, sessionName)
	if err := state.Save(); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	fmt.Println("\nTask completed!")
	return nil
}

// checkChangesPushed verifies that changes have been pushed to remote
func checkChangesPushed(cwd string) error {
	cmd := exec.Command("git", "-C", cwd, "status", "-sb")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("checking git status: %w", err)
	}

	// Check if we're behind or ahead
	status := string(output)
	if strings.Contains(status, "[ahead") {
		return fmt.Errorf("local commits not pushed to remote. Push changes first")
	}

	return nil
}

// extractPRURL extracts a PR URL from a status message
func extractPRURL(message string) string {
	// Look for patterns like "PR: https://github.com/..." or just "https://github.com/.../pull/123"
	message = strings.TrimSpace(message)

	// Check for "PR:" prefix
	if strings.HasPrefix(strings.ToLower(message), "pr:") {
		message = strings.TrimSpace(message[3:])
	}

	// Look for GitHub PR URL
	if strings.Contains(message, "github.com") && strings.Contains(message, "/pull/") {
		// Extract URL from message
		parts := strings.Fields(message)
		for _, part := range parts {
			if strings.Contains(part, "github.com") && strings.Contains(part, "/pull/") {
				return part
			}
		}
	}

	return message
}

// checkPRMerged verifies that a PR has been merged
func checkPRMerged(cwd, prURL string) error {
	// Use gh CLI to check PR status
	cmd := exec.Command("gh", "pr", "view", prURL, "--json", "state", "-q", ".state")
	cmd.Dir = cwd
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("checking PR status: %w", err)
	}

	state := strings.TrimSpace(string(output))
	if state != "MERGED" {
		return fmt.Errorf("PR is not merged (current state: %s)", state)
	}

	return nil
}

// runTestsAndCheck runs the test suite and checks if tests pass
func runTestsAndCheck(cwd string) error {
	// Try common test commands
	testCommands := [][]string{
		{"go", "test", "./..."},
		{"npm", "test"},
		{"pytest"},
		{"cargo", "test"},
		{"make", "test"},
	}

	for _, testCmd := range testCommands {
		// Check if the command exists
		if _, err := exec.LookPath(testCmd[0]); err != nil {
			continue
		}

		// For npm, check if package.json exists
		if testCmd[0] == "npm" {
			if _, err := os.Stat(cwd + "/package.json"); os.IsNotExist(err) {
				continue
			}
		}

		// For Go, check if go.mod exists
		if testCmd[0] == "go" {
			if _, err := os.Stat(cwd + "/go.mod"); os.IsNotExist(err) {
				continue
			}
		}

		fmt.Printf("Running: %s\n", strings.Join(testCmd, " "))
		cmd := exec.Command(testCmd[0], testCmd[1:]...)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("tests failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("no test command found. Ensure your project has a test suite configured")
}

// buildTaskPrompt creates the prompt to send to Claude for a task session
func buildTaskPrompt(description string, condition session.CompletionCondition, sessionName string, _ *project.Project) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Task: %s\n\n", description))
	sb.WriteString("This is a lightweight task session (not a bead).\n\n")

	sb.WriteString("Workflow:\n")
	sb.WriteString("1. Complete the task described above\n")
	sb.WriteString("2. Commit your changes with a descriptive message\n")

	switch condition {
	case session.ConditionNone:
		sb.WriteString("\nWhen finished, signal completion:\n")
		sb.WriteString("  wt signal ready \"Task completed\"\n")
		sb.WriteString("\nThe hub will run `wt done` to clean up.")

	case session.ConditionPushed:
		sb.WriteString("3. Push your changes to the remote\n")
		sb.WriteString("\nWhen finished, signal completion:\n")
		sb.WriteString("  wt signal ready \"Changes pushed\"\n")
		sb.WriteString("\nThe hub will verify changes are pushed before cleanup.")

	case session.ConditionPRMerged:
		sb.WriteString("3. Create a PR with `gh pr create`\n")
		sb.WriteString("\nWhen finished, signal completion with PR URL:\n")
		sb.WriteString("  wt signal ready \"PR: <paste PR URL here>\"\n")
		sb.WriteString("\nThe hub will wait for the PR to merge before cleanup.")

	case session.ConditionTestsPass:
		sb.WriteString("3. Run tests and ensure they pass\n")
		sb.WriteString("\nWhen finished, signal completion:\n")
		sb.WriteString("  wt signal ready \"Tests passing\"\n")
		sb.WriteString("\nThe hub will verify tests pass before cleanup.")

	case session.ConditionUserConfirm:
		sb.WriteString("\nWhen finished, signal completion:\n")
		sb.WriteString("  wt signal ready \"Ready for review\"\n")
		sb.WriteString("\nThe hub will ask for user confirmation before cleanup.")
	}

	// Add commit message format with session name
	sb.WriteString("\n\n## Commit Message Format\n")
	sb.WriteString("Include this footer in your commit messages for traceability:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<commit message>\n\n")
	sb.WriteString("Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>\n")
	sb.WriteString(fmt.Sprintf("Session: %s\n", sessionName))
	sb.WriteString("```\n")

	return sb.String()
}
