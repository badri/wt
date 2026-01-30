package auto

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/project"
)

// Config holds auto-mode configuration from project config
type Config struct {
	Command        string `json:"command"`
	TimeoutMinutes int    `json:"timeout_minutes"`
	PromptTemplate string `json:"prompt_template"`
}

// DefaultConfig returns default auto configuration
func DefaultConfig() *Config {
	return &Config{
		Command:        "claude --dangerously-skip-permissions",
		TimeoutMinutes: 30,
		PromptTemplate: "Work on bead {BEAD_ID}: {TITLE}\n\n{DESCRIPTION}",
	}
}

// Options holds CLI options for wt auto
type Options struct {
	Project        string
	MergeMode      string
	DryRun         bool
	Check          bool
	Stop           bool
	Force          bool
	Timeout        int    // minutes, 0 means use project default
	Limit          int    // max beads to process, 0 means no limit
	Epic           string // required: epic ID to process
	PauseOnFailure bool   // stop and preserve worktree if bead fails
	SkipAudit      bool   // bypass implicit audit
	Resume         bool   // resume after failure
	Abort          bool   // abort and clean up after failure
}

// Runner manages the auto execution loop
type Runner struct {
	cfg        *config.Config
	projMgr    *project.Manager
	opts       *Options
	logger     *Logger
	lockFile   string
	stopFile   string
	stopSignal chan struct{}
}

// NewRunner creates a new auto runner
func NewRunner(cfg *config.Config, opts *Options) *Runner {
	r := &Runner{
		cfg:        cfg,
		projMgr:    project.NewManager(cfg),
		opts:       opts,
		stopSignal: make(chan struct{}, 1),
	}
	// Set per-project paths if project is known, otherwise use legacy global paths
	r.setProjectPaths(opts.Project)
	return r
}

// setProjectPaths sets lock/stop file paths based on project name.
// If projectName is empty, uses legacy global paths for backwards compatibility.
func (r *Runner) setProjectPaths(projectName string) {
	if projectName != "" {
		r.lockFile = filepath.Join(r.cfg.ConfigDir(), fmt.Sprintf("auto-%s.lock", projectName))
		r.stopFile = filepath.Join(r.cfg.ConfigDir(), fmt.Sprintf("stop-auto-%s", projectName))
	} else {
		r.lockFile = filepath.Join(r.cfg.ConfigDir(), "auto.lock")
		r.stopFile = filepath.Join(r.cfg.ConfigDir(), "stop-auto")
	}
}

// resolveProjectForEpic finds which project owns the given epic and returns its name.
func (r *Runner) resolveProjectForEpic(epicID string) (string, error) {
	projects, err := r.projMgr.List()
	if err != nil {
		return "", fmt.Errorf("listing projects: %w", err)
	}

	for _, proj := range projects {
		cmd := exec.Command("bd", "show", epicID, "--json")
		cmd.Dir = proj.RepoPath()
		output, err := cmd.Output()
		if err != nil {
			continue
		}

		var infos []struct {
			ID        string `json:"id"`
			IssueType string `json:"issue_type"`
		}
		if err := json.Unmarshal(output, &infos); err != nil || len(infos) == 0 {
			continue
		}
		if infos[0].IssueType == "epic" {
			return proj.Name, nil
		}
	}

	return "", fmt.Errorf("epic %s not found in any registered project", epicID)
}

// findAllAutoLocks returns all per-project lock files in the config directory.
func (r *Runner) findAllAutoLocks() ([]string, error) {
	pattern := filepath.Join(r.cfg.ConfigDir(), "auto-*.lock")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	// Also check legacy global lock
	legacyLock := filepath.Join(r.cfg.ConfigDir(), "auto.lock")
	if _, err := os.Stat(legacyLock); err == nil {
		matches = append(matches, legacyLock)
	}
	return matches, nil
}

// projectNameFromLockFile extracts project name from a lock file path.
// Returns empty string for the legacy global lock.
func projectNameFromLockFile(lockPath string) string {
	base := filepath.Base(lockPath)
	if base == "auto.lock" {
		return ""
	}
	// auto-{project}.lock
	name := strings.TrimPrefix(base, "auto-")
	name = strings.TrimSuffix(name, ".lock")
	return name
}

// Run executes the auto loop
func (r *Runner) Run() error {
	// Handle --stop flag (works without --epic)
	if r.opts.Stop {
		return r.signalStop()
	}

	// Handle --check flag (works without --epic)
	if r.opts.Check {
		return r.checkStatus()
	}

	// Require --epic or --project for all other operations
	if r.opts.Epic == "" && r.opts.Project == "" {
		return fmt.Errorf("--epic <id> or --project <name> is required\n\nUsage:\n  wt auto --epic <epic-id>       Process all beads in an epic (single worktree)\n  wt auto --project <name>       Process ready beads serially (separate worktrees)\n  wt auto --check                Check status of a running auto session\n\nExample:\n  wt auto --epic wt-doc-epic\n  wt auto --project myapp")
	}

	// Project-only mode: serial queue processing
	if r.opts.Epic == "" && r.opts.Project != "" {
		return r.runProjectMode()
	}

	// Resolve project from epic if not already set
	if r.opts.Project == "" {
		projName, err := r.resolveProjectForEpic(r.opts.Epic)
		if err != nil {
			return err
		}
		r.opts.Project = projName
		r.setProjectPaths(projName)
	}

	// Handle --abort flag (needs project resolved for state file)
	if r.opts.Abort {
		return r.abortRun()
	}

	// Initialize logger
	logsDir := filepath.Join(r.cfg.ConfigDir(), "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("creating logs directory: %w", err)
	}

	logger, err := NewLogger(logsDir)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer logger.Close()
	r.logger = logger

	// Handle --resume flag (needs project resolved for state file)
	if r.opts.Resume {
		return r.resumeRun()
	}

	// Acquire lock
	if err := r.acquireLock(); err != nil {
		return err
	}
	defer r.releaseLock()

	// Clean up any existing stop file
	os.Remove(r.stopFile)

	// Setup signal handling
	r.setupSignalHandler()

	// Process the epic
	if err := r.processEpic(); err != nil {
		r.logger.Log("Error processing epic %s: %v", r.opts.Epic, err)
		return err
	}

	r.logger.Log("Auto run complete")
	return nil
}

// runProjectMode processes ready beads for a project serially (separate worktrees per bead).
func (r *Runner) runProjectMode() error {
	r.setProjectPaths(r.opts.Project)

	// Handle --abort (no state to abort in project mode)
	if r.opts.Abort || r.opts.Resume {
		return fmt.Errorf("--abort and --resume are only supported with --epic mode")
	}

	// Initialize logger
	logsDir := filepath.Join(r.cfg.ConfigDir(), "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("creating logs directory: %w", err)
	}
	logger, err := NewLogger(logsDir)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer logger.Close()
	r.logger = logger

	// Acquire lock
	if err := r.acquireLock(); err != nil {
		return err
	}
	defer r.releaseLock()

	os.Remove(r.stopFile)
	r.setupSignalHandler()

	// Resolve project
	proj, err := r.projMgr.Get(r.opts.Project)
	if err != nil {
		return fmt.Errorf("project '%s' not found", r.opts.Project)
	}

	r.logger.Log("Starting project mode for %s", r.opts.Project)
	fmt.Printf("Processing ready beads for project: %s\n", r.opts.Project)

	if err := r.processProject(proj); err != nil {
		r.logger.Log("Error processing project %s: %v", r.opts.Project, err)
		return err
	}

	r.logger.Log("Project mode complete for %s", r.opts.Project)
	return nil
}

// processProject processes all ready beads in a project
func (r *Runner) processProject(proj *project.Project) error {
	beadsDir := proj.BeadsDir()

	// Check if beads directory exists
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return fmt.Errorf("no .beads directory in project %s", proj.Name)
	}

	// Get ready beads
	readyBeads, err := bead.ReadyInDir(beadsDir)
	if err != nil {
		return fmt.Errorf("getting ready beads: %w", err)
	}

	if len(readyBeads) == 0 {
		fmt.Printf("No ready beads in project %s.\n", proj.Name)
		return nil
	}

	// Apply limit if specified
	if r.opts.Limit > 0 && len(readyBeads) > r.opts.Limit {
		fmt.Printf("Found %d ready bead(s) in project %s, limiting to %d.\n", len(readyBeads), proj.Name, r.opts.Limit)
		readyBeads = readyBeads[:r.opts.Limit]
	} else {
		fmt.Printf("Found %d ready bead(s) in project %s.\n", len(readyBeads), proj.Name)
	}

	// Handle --check flag
	if r.opts.Check {
		return r.checkBeads(readyBeads)
	}

	// Process each bead
	for _, b := range readyBeads {
		if r.shouldStop() {
			r.logger.Log("Stop signal received, stopping bead processing")
			break
		}

		if err := r.processBead(proj, &b); err != nil {
			r.logger.Log("Error processing bead %s: %v", b.ID, err)
			fmt.Printf("Error processing bead %s: %v\n", b.ID, err)
			// Continue with next bead
		}
	}

	return nil
}

// processBead processes a single bead
func (r *Runner) processBead(proj *project.Project, b *bead.ReadyBead) error {
	r.logger.LogBeadStart(b.ID, b.Title)
	startTime := time.Now()

	fmt.Printf("\n=== Processing bead: %s ===\n", b.ID)
	fmt.Printf("Title: %s\n", b.Title)

	// Get auto config for project
	autoCfg := r.getAutoConfig(proj)
	timeout := time.Duration(autoCfg.TimeoutMinutes) * time.Minute
	if r.opts.Timeout > 0 {
		timeout = time.Duration(r.opts.Timeout) * time.Minute
	}

	// Dry run mode
	if r.opts.DryRun {
		fmt.Printf("[DRY RUN] Would run: wt new %s --no-switch\n", b.ID)
		fmt.Printf("[DRY RUN] Command: %s\n", autoCfg.Command)
		fmt.Printf("[DRY RUN] Timeout: %v\n", timeout)
		r.logger.LogBeadEnd(b.ID, "dry-run", time.Since(startTime))
		return nil
	}

	// Create session with wt new
	sessionName, err := r.createSession(b.ID)
	if err != nil {
		r.logger.LogBeadEnd(b.ID, "failed-create", time.Since(startTime))
		return fmt.Errorf("creating session: %w", err)
	}

	fmt.Printf("Created session: %s\n", sessionName)

	// Build prompt from template
	prompt := r.buildPrompt(autoCfg.PromptTemplate, b, sessionName, proj)

	// Run claude in the session
	outcome, err := r.runClaudeInSession(sessionName, autoCfg.Command, prompt, timeout)
	if err != nil {
		r.logger.LogBeadEnd(b.ID, outcome, time.Since(startTime))
		return fmt.Errorf("running claude: %w", err)
	}

	r.logger.LogBeadEnd(b.ID, outcome, time.Since(startTime))

	// Handle merge if configured
	mergeMode := r.opts.MergeMode
	if mergeMode == "" {
		mergeMode = proj.MergeMode
	}
	if mergeMode == "" {
		mergeMode = r.cfg.DefaultMergeMode
	}

	if mergeMode != "none" && outcome == "success" {
		fmt.Printf("Merge mode: %s (not yet implemented)\n", mergeMode)
	}

	return nil
}

// createSession creates a new wt session for the bead
func (r *Runner) createSession(beadID string) (string, error) {
	cmd := exec.Command("wt", "new", beadID, "--no-switch")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", string(output), err)
	}

	// Parse session name from output
	// Expected: "Session 'toast' ready."
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Session") && strings.Contains(line, "ready") {
			// Extract session name between quotes
			start := strings.Index(line, "'")
			if start >= 0 {
				end := strings.Index(line[start+1:], "'")
				if end >= 0 {
					return line[start+1 : start+1+end], nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not parse session name from: %s", string(output))
}

// runClaudeInSession runs claude in a tmux session and waits for completion
func (r *Runner) runClaudeInSession(sessionName, command, prompt string, timeout time.Duration) (string, error) {
	// Write prompt to a temp file to avoid send-keys issues with long prompts.
	// Using send-keys with long/complex prompts can cause the text to appear
	// twice in the input buffer (once executed, once echoed without Enter).
	promptFile, err := os.CreateTemp("", "wt-auto-prompt-*.txt")
	if err != nil {
		return "failed-create-prompt", fmt.Errorf("creating temp prompt file: %w", err)
	}
	promptPath := promptFile.Name()

	if _, err := promptFile.WriteString(prompt); err != nil {
		promptFile.Close()
		os.Remove(promptPath)
		return "failed-write-prompt", fmt.Errorf("writing prompt to temp file: %w", err)
	}
	promptFile.Close()

	// Build the shell command to start Claude with the prompt file.
	// Prefix with space to avoid shell history.
	fullCmd := fmt.Sprintf(" %s -p \"$(cat %q)\" && rm -f %q", command, promptPath, promptPath)

	// Use paste-buffer for reliable command delivery (same pattern as NudgeSession).
	// This is more reliable than send-keys for long command strings.
	cmdFile, err := os.CreateTemp("", "wt-auto-cmd-*.txt")
	if err != nil {
		os.Remove(promptPath)
		return "failed-create-cmd", fmt.Errorf("creating temp command file: %w", err)
	}
	cmdPath := cmdFile.Name()
	defer os.Remove(cmdPath)

	if _, err := cmdFile.WriteString(fullCmd); err != nil {
		cmdFile.Close()
		os.Remove(promptPath)
		return "failed-write-cmd", fmt.Errorf("writing command to temp file: %w", err)
	}
	cmdFile.Close()

	// Load command into tmux buffer
	loadCmd := exec.Command("tmux", "load-buffer", cmdPath)
	if err := loadCmd.Run(); err != nil {
		os.Remove(promptPath)
		return "failed-load-buffer", fmt.Errorf("loading buffer: %w", err)
	}

	// Paste buffer to the target pane
	pasteCmd := exec.Command("tmux", "paste-buffer", "-t", sessionName)
	if err := pasteCmd.Run(); err != nil {
		os.Remove(promptPath)
		return "failed-paste", fmt.Errorf("pasting buffer to %s: %w", sessionName, err)
	}

	// Wait for paste to complete, then send Enter
	time.Sleep(500 * time.Millisecond)
	enterCmd := exec.Command("tmux", "send-keys", "-t", sessionName, "Enter")
	if err := enterCmd.Run(); err != nil {
		os.Remove(promptPath)
		return "failed-enter", fmt.Errorf("sending Enter to %s: %w", sessionName, err)
	}

	fmt.Printf("Started claude in session %s (timeout: %v)\n", sessionName, timeout)

	// Wait for session to complete or timeout
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeoutCh := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			// Check if session is still alive and active
			if !r.isSessionActive(sessionName) {
				fmt.Printf("Session %s completed\n", sessionName)
				// Brief delay to let shell stabilize before next prompt
				time.Sleep(2 * time.Second)
				return "success", nil
			}
		case <-timeoutCh:
			fmt.Printf("Session %s timed out after %v\n", sessionName, timeout)
			return "timeout", nil
		case <-r.stopSignal:
			fmt.Printf("Stop signal received, leaving session %s running\n", sessionName)
			return "stopped", nil
		}
	}
}

// isSessionActive checks if a tmux session has active processes
func (r *Runner) isSessionActive(sessionName string) bool {
	// Check if tmux session exists
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check if there's an active process (claude)
	// Get the pane PID and check if it has children
	cmd = exec.Command("tmux", "display-message", "-t", sessionName, "-p", "#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return true // Assume active if we can't check
	}

	pid := strings.TrimSpace(string(output))
	if pid == "" {
		return false
	}

	// Check if the shell has child processes (claude running)
	cmd = exec.Command("pgrep", "-P", pid)
	if err := cmd.Run(); err != nil {
		// No child processes, claude has finished
		return false
	}

	return true
}

// waitForShellPrompt waits until the tmux session has no child processes (shell is idle at prompt).
func (r *Runner) waitForShellPrompt(sessionName string, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		if !r.isSessionActive(sessionName) {
			return
		}
		time.Sleep(1 * time.Second)
	}
	r.logger.Log("Warning: timed out waiting for shell prompt in %s", sessionName)
}

// buildPrompt builds the prompt from template
func (r *Runner) buildPrompt(template string, b *bead.ReadyBead, sessionName string, proj *project.Project) string {
	prompt := template
	prompt = strings.ReplaceAll(prompt, "{BEAD_ID}", b.ID)
	prompt = strings.ReplaceAll(prompt, "{TITLE}", b.Title)
	prompt = strings.ReplaceAll(prompt, "{DESCRIPTION}", b.Description)
	prompt = strings.ReplaceAll(prompt, "{SESSION}", sessionName)
	prompt = strings.ReplaceAll(prompt, "{PROJECT}", proj.Name)

	// Get worktree path
	worktreePath := r.cfg.WorktreePath(sessionName)
	prompt = strings.ReplaceAll(prompt, "{WORKTREE}", worktreePath)

	return prompt
}

// getAutoConfig gets auto configuration for a project
func (r *Runner) getAutoConfig(_ *project.Project) *Config {
	// For now, return defaults
	// TODO: Load from project config when auto section is added
	return DefaultConfig()
}

// checkBeads validates beads are well-groomed
func (r *Runner) checkBeads(beads []bead.ReadyBead) error {
	fmt.Println("\n=== Bead Check ===")
	allGood := true

	for _, b := range beads {
		issues := []string{}

		if b.Title == "" {
			issues = append(issues, "missing title")
		}
		if b.Description == "" {
			issues = append(issues, "missing description")
		}

		if len(issues) > 0 {
			fmt.Printf("  %s: %s\n", b.ID, strings.Join(issues, ", "))
			allGood = false
		} else {
			fmt.Printf("  %s: OK\n", b.ID)
		}
	}

	if !allGood {
		return fmt.Errorf("some beads need grooming")
	}

	fmt.Println("\nAll beads are well-groomed!")
	return nil
}

// getProjects returns projects to process
func (r *Runner) getProjects() ([]*project.Project, error) {
	if r.opts.Project != "" {
		proj, err := r.projMgr.Get(r.opts.Project)
		if err != nil {
			return nil, fmt.Errorf("project '%s' not found", r.opts.Project)
		}
		return []*project.Project{proj}, nil
	}

	return r.projMgr.List()
}

// acquireLock attempts to acquire the lock file
func (r *Runner) acquireLock() error {
	// Check if lock exists
	if data, err := os.ReadFile(r.lockFile); err == nil {
		var lock LockInfo
		if err := json.Unmarshal(data, &lock); err == nil {
			// Check if process is still running
			if r.isProcessRunning(lock.PID) {
				if r.opts.Force {
					fmt.Printf("Warning: Forcing lock override (previous PID: %d)\n", lock.PID)
				} else {
					return fmt.Errorf("another wt auto is running (PID: %d, started: %s). Use --force to override", lock.PID, lock.StartTime)
				}
			}
		}
	}

	// Write lock file
	lock := LockInfo{
		PID:       os.Getpid(),
		StartTime: time.Now().Format(time.RFC3339),
		Project:   r.opts.Project,
		Epic:      r.opts.Epic,
	}

	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.lockFile, data, 0644)
}

// releaseLock removes the lock file
func (r *Runner) releaseLock() {
	os.Remove(r.lockFile)
}

// isProcessRunning checks if a process is running
func (r *Runner) isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// signalStop signals a running wt auto to stop
func (r *Runner) signalStop() error {
	// If a specific project is set, stop only that project
	if r.opts.Project != "" {
		if _, err := os.Stat(r.lockFile); os.IsNotExist(err) {
			return fmt.Errorf("no wt auto is running for project %s", r.opts.Project)
		}
		if err := os.WriteFile(r.stopFile, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
			return fmt.Errorf("creating stop signal: %w", err)
		}
		fmt.Printf("Stop signal sent for project %s. wt auto will stop after the current bead.\n", r.opts.Project)
		return nil
	}

	// No project specified: stop all running autos
	locks, err := r.findAllAutoLocks()
	if err != nil {
		return fmt.Errorf("finding lock files: %w", err)
	}
	if len(locks) == 0 {
		return fmt.Errorf("no wt auto is currently running")
	}

	for _, lockPath := range locks {
		projName := projectNameFromLockFile(lockPath)
		var stopPath string
		if projName != "" {
			stopPath = filepath.Join(r.cfg.ConfigDir(), fmt.Sprintf("stop-auto-%s", projName))
		} else {
			stopPath = filepath.Join(r.cfg.ConfigDir(), "stop-auto")
		}
		if err := os.WriteFile(stopPath, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
			fmt.Printf("Warning: failed to send stop signal for %s: %v\n", lockPath, err)
			continue
		}
		if projName != "" {
			fmt.Printf("Stop signal sent for project %s.\n", projName)
		} else {
			fmt.Println("Stop signal sent (legacy global auto).")
		}
	}
	fmt.Println("All running autos will stop after their current bead.")
	return nil
}

// shouldStop checks if we should stop processing
func (r *Runner) shouldStop() bool {
	select {
	case <-r.stopSignal:
		return true
	default:
	}

	// Check for stop file
	if _, err := os.Stat(r.stopFile); err == nil {
		os.Remove(r.stopFile)
		return true
	}

	return false
}

// setupSignalHandler sets up OS signal handling
func (r *Runner) setupSignalHandler() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, will stop after current bead...")
		r.stopSignal <- struct{}{}
	}()
}

// LockInfo represents the lock file content
type LockInfo struct {
	PID       int    `json:"pid"`
	StartTime string `json:"start_time"`
	Project   string `json:"project,omitempty"`
	Epic      string `json:"epic,omitempty"`
}

// BeadCommitInfo tracks commit information for a completed bead
type BeadCommitInfo struct {
	BeadID     string `json:"bead_id"`
	CommitHash string `json:"commit_hash"`
	Summary    string `json:"summary"`
	Title      string `json:"title"`
}

// EpicState tracks the state of an epic batch run
type EpicState struct {
	EpicID         string            `json:"epic_id"`
	EpicTitle      string            `json:"epic_title,omitempty"` // For batch-aware prompts
	Worktree       string            `json:"worktree"`
	SessionName    string            `json:"session_name"`
	Beads          []string          `json:"beads"`
	BeadTitles     map[string]string `json:"bead_titles,omitempty"` // bead ID -> title
	CompletedBeads []string          `json:"completed_beads"`
	BeadCommits    []BeadCommitInfo  `json:"bead_commits,omitempty"` // Track commit info for each completed bead
	FailedBeads    map[string]string `json:"failed_beads,omitempty"` // bead ID -> failure reason
	CurrentBead    string            `json:"current_bead,omitempty"`
	FailedBead     string            `json:"failed_bead,omitempty"`    // deprecated: use FailedBeads
	FailureReason  string            `json:"failure_reason,omitempty"` // deprecated: use FailedBeads
	Status         string            `json:"status"`                   // running, paused, failed, completed
	StartTime      string            `json:"start_time"`
	ProjectDir     string            `json:"project_dir"`
	MergeMode      string            `json:"merge_mode"`
}

// EpicAuditResult holds the result of auditing an epic
type EpicAuditResult struct {
	EpicID           string            `json:"epic_id"`
	Ready            bool              `json:"ready"`
	Beads            []string          `json:"beads"`
	BeadTitles       map[string]string `json:"bead_titles"`
	Issues           []string          `json:"issues"`
	FileConflicts    []string          `json:"file_conflicts,omitempty"`
	ExternalBlockers []string          `json:"external_blockers,omitempty"`
	ProjectDir       string            `json:"project_dir"`
}

// processEpic sets up an epic for batch processing and starts the first bead.
// Subsequent beads are handled by `wt signal bead-done` which triggers transitions.
func (r *Runner) processEpic() error {
	epicID := r.opts.Epic
	r.logger.Log("Processing epic: %s", epicID)
	fmt.Printf("Processing epic: %s\n", epicID)

	// Run implicit audit unless skipped
	if !r.opts.SkipAudit {
		auditResult, err := r.auditEpic(epicID)
		if err != nil {
			return fmt.Errorf("audit failed: %w", err)
		}

		if !auditResult.Ready {
			fmt.Println("\n=== Epic Audit Failed ===")
			for _, issue := range auditResult.Issues {
				fmt.Printf("  ✗ %s\n", issue)
			}
			fmt.Println("\nUse --skip-audit to bypass (not recommended)")
			return fmt.Errorf("epic not ready for batch processing")
		}

		fmt.Printf("\n✓ Audit passed: %d bead(s) ready\n", len(auditResult.Beads))
	}

	// Get beads that block this epic (its dependencies)
	beads, projectDir, err := r.getEpicBeads(epicID)
	if err != nil {
		return err
	}

	if len(beads) == 0 {
		fmt.Println("No beads to process in epic.")
		return nil
	}

	// Dry run mode
	if r.opts.DryRun {
		fmt.Println("\n=== Dry Run ===")
		fmt.Printf("Would process %d bead(s) in epic %s:\n", len(beads), epicID)
		for i, b := range beads {
			fmt.Printf("  %d. %s: %s\n", i+1, b.ID, b.Title)
		}
		fmt.Println("\nWould create single worktree for sequential processing.")
		fmt.Println("Worker signals completion via: wt signal bead-done \"<summary>\"")
		return nil
	}

	// Get project for this epic
	proj, err := r.getProjectForPath(projectDir)
	if err != nil {
		return fmt.Errorf("finding project: %w", err)
	}

	// Create single worktree for the epic (without --shell, so Claude starts)
	sessionName, worktreePath, err := r.createEpicWorktree(epicID, proj)
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	fmt.Printf("\nCreated worktree: %s\n", worktreePath)
	fmt.Printf("Session: %s\n", sessionName)

	// Fetch epic title for batch-aware prompts
	epicTitle := r.getEpicTitle(epicID, projectDir)

	// Save epic state for signal-driven orchestration
	state := &EpicState{
		EpicID:         epicID,
		EpicTitle:      epicTitle,
		Worktree:       worktreePath,
		SessionName:    sessionName,
		Beads:          make([]string, len(beads)),
		BeadTitles:     make(map[string]string),
		FailedBeads:    make(map[string]string),
		BeadCommits:    []BeadCommitInfo{},
		CompletedBeads: []string{},
		Status:         "running",
		StartTime:      time.Now().Format(time.RFC3339),
		ProjectDir:     projectDir,
		MergeMode:      r.opts.MergeMode,
	}
	for i, b := range beads {
		state.Beads[i] = b.ID
		state.BeadTitles[b.ID] = b.Title
	}

	if err := r.saveEpicState(state); err != nil {
		return fmt.Errorf("saving epic state: %w", err)
	}

	fmt.Printf("\n=== Starting Epic: %d bead(s) to process ===\n", len(beads))

	// Get auto config for timeout
	autoCfg := r.getAutoConfig(proj)
	timeout := time.Duration(autoCfg.TimeoutMinutes) * time.Minute
	if r.opts.Timeout > 0 {
		timeout = time.Duration(r.opts.Timeout) * time.Minute
	}

	// Process all beads sequentially, staying alive for the entire epic
	for i, b := range beads {
		beadNum := i + 1
		totalBeads := len(beads)

		if r.shouldStop() {
			state.Status = "paused"
			state.CurrentBead = b.ID
			r.saveEpicState(state)
			fmt.Printf("\nPaused at bead %d/%d. Use 'wt auto --resume' to continue.\n", beadNum, totalBeads)
			return nil
		}

		fmt.Printf("\n=== Bead %d/%d: %s ===\n", beadNum, totalBeads, b.ID)

		// Re-check bead status (may have been closed by a previous bead's commit)
		if info, err := bead.ShowInDir(b.ID, filepath.Join(state.ProjectDir, ".beads")); err == nil && info.Status == "closed" {
			b.Status = "closed"
		}

		// Skip beads that are already closed
		if b.Status == "closed" {
			fmt.Printf("  Bead %s already closed, skipping\n", b.ID)
			if !slices.Contains(state.CompletedBeads, b.ID) {
				state.CompletedBeads = append(state.CompletedBeads, b.ID)
				r.saveEpicState(state)
			}
			continue
		}

		state.CurrentBead = b.ID
		r.saveEpicState(state)

		// Mark bead as in_progress
		if err := bead.UpdateStatusInDir(b.ID, "in_progress", state.ProjectDir); err != nil {
			r.logger.Log("Warning: could not mark bead %s as in_progress: %v", b.ID, err)
		}

		// Build batch-aware prompt
		prompt := r.buildEpicBeadPrompt(&b, state.SessionName, proj, beadNum, totalBeads, state)

		outcome, err := r.runClaudeInSession(state.SessionName, autoCfg.Command, prompt, timeout)
		if err != nil || (outcome != "success" && outcome != "dry-run") {
			if r.opts.PauseOnFailure {
				state.Status = "failed"
				state.FailedBead = b.ID
				state.FailureReason = outcome
				r.saveEpicState(state)
				return fmt.Errorf("bead %s failed: %s", b.ID, outcome)
			}
			state.FailedBeads[b.ID] = outcome
			r.saveEpicState(state)
			fmt.Printf("Warning: bead %s failed (%s), continuing...\n", b.ID, outcome)
			continue
		}

		// Capture commit info
		commitHash, commitMsg, err := r.getLatestCommitInfo(state.Worktree)
		if err != nil {
			r.logger.Log("Warning: could not get commit info: %v", err)
		} else {
			state.BeadCommits = append(state.BeadCommits, BeadCommitInfo{
				BeadID:     b.ID,
				CommitHash: commitHash,
				Summary:    commitMsg,
				Title:      b.Title,
			})
		}

		state.CompletedBeads = append(state.CompletedBeads, b.ID)
		r.saveEpicState(state)
		fmt.Printf("✓ Bead %s completed (commit: %s)\n", b.ID, commitHash)

		// Ensure Claude has exited before starting next bead (prevents pasting into REPL)
		if i < len(beads)-1 {
			fmt.Printf("  Ensuring Claude session is terminated for next bead...\n")
			r.killClaudeSession(state.SessionName)
			// Wait for shell prompt to be ready
			r.waitForShellPrompt(state.SessionName, 10*time.Second)
		}
	}

	// Determine final status
	allSucceeded := len(state.FailedBeads) == 0
	if allSucceeded {
		state.Status = "completed"
	} else {
		state.Status = "partial"
	}
	r.saveEpicState(state)

	fmt.Printf("\n=== All %d bead(s) processed ===\n", len(state.Beads))
	fmt.Printf("  Completed: %d\n", len(state.CompletedBeads))
	if len(state.FailedBeads) > 0 {
		fmt.Printf("  Failed: %d\n", len(state.FailedBeads))
		for beadID, reason := range state.FailedBeads {
			fmt.Printf("    - %s: %s\n", beadID, reason)
		}
	}

	// Only close epic if all beads succeeded
	if allSucceeded {
		if err := r.closeEpic(state.EpicID, state.ProjectDir); err != nil {
			fmt.Printf("Warning: could not auto-close epic: %v\n", err)
		} else {
			fmt.Printf("✓ Epic %s closed\n", state.EpicID)
		}

		batchMarkerPath := filepath.Join(state.Worktree, ".wt-batch-mode")
		os.Remove(batchMarkerPath)
		r.removeEpicState()
	} else {
		fmt.Printf("\n✗ Epic %s NOT closed due to failed beads\n", state.EpicID)
		fmt.Printf("  Fix failures and run 'wt auto --resume --epic %s' to retry\n", state.EpicID)
		fmt.Printf("  Or run 'wt auto --abort --epic %s' to clean up\n", state.EpicID)
	}

	return nil
}

// auditEpic checks if an epic is ready for batch processing
func (r *Runner) auditEpic(epicID string) (*EpicAuditResult, error) {
	fmt.Printf("Auditing epic %s...\n", epicID)

	result := &EpicAuditResult{
		EpicID:     epicID,
		Ready:      true,
		BeadTitles: make(map[string]string),
	}

	// Get beads blocking this epic
	beads, projectDir, err := r.getEpicBeads(epicID)
	if err != nil {
		return nil, err
	}
	result.ProjectDir = projectDir

	if len(beads) == 0 {
		result.Ready = false
		result.Issues = append(result.Issues, "No beads found blocking this epic")
		return result, nil
	}

	for _, b := range beads {
		result.Beads = append(result.Beads, b.ID)
		result.BeadTitles[b.ID] = b.Title
	}

	// Check: beads all from same project (already ensured by getEpicBeads)

	// Check: no external blockers (beads should be ready)
	for _, b := range beads {
		blockers, err := r.getBeadBlockers(b.ID, projectDir)
		if err != nil {
			continue
		}
		for _, blocker := range blockers {
			// Check if blocker is in our bead list (internal) or external
			// Also skip if blocker is the epic itself (parent-child relationship)
			isInternal := blocker == epicID
			for _, ob := range beads {
				if ob.ID == blocker {
					isInternal = true
					break
				}
			}
			if !isInternal {
				result.Ready = false
				result.ExternalBlockers = append(result.ExternalBlockers, fmt.Sprintf("%s blocked by %s", b.ID, blocker))
				result.Issues = append(result.Issues, fmt.Sprintf("Bead %s has external blocker: %s", b.ID, blocker))
			}
		}
	}

	// Check: beads have descriptions (basic readiness)
	for _, b := range beads {
		if b.Description == "" {
			result.Ready = false
			result.Issues = append(result.Issues, fmt.Sprintf("Bead %s has no description", b.ID))
		}
	}

	return result, nil
}

// getEpicBeads returns all beads that block the given epic (dependencies of the epic)
func (r *Runner) getEpicBeads(epicID string) ([]bead.ReadyBead, string, error) {
	// First, find which project contains this epic
	projects, err := r.projMgr.List()
	if err != nil {
		return nil, "", fmt.Errorf("listing projects: %w", err)
	}

	for _, proj := range projects {
		beadsDir := proj.BeadsDir()
		projectDir := proj.RepoPath()

		// Check if this epic exists in this project
		cmd := exec.Command("bd", "show", epicID, "--json")
		cmd.Dir = projectDir
		output, err := cmd.Output()
		if err != nil {
			continue // Epic not in this project
		}

		// Parse to verify it's an epic
		var infos []struct {
			ID        string `json:"id"`
			IssueType string `json:"issue_type"`
		}
		if err := json.Unmarshal(output, &infos); err != nil || len(infos) == 0 {
			continue
		}
		if infos[0].IssueType != "epic" {
			return nil, "", fmt.Errorf("%s is not an epic (type: %s)", epicID, infos[0].IssueType)
		}

		// Get epic's children via dependents from bd show --json
		cmd = exec.Command("bd", "show", epicID, "--json")
		cmd.Dir = projectDir
		showOutput, showErr := cmd.Output()

		var childIDs []string

		if showErr == nil {
			var showResults []struct {
				Dependents []struct {
					ID string `json:"id"`
				} `json:"dependents"`
			}
			if err := json.Unmarshal(showOutput, &showResults); err == nil && len(showResults) > 0 {
				for _, dep := range showResults[0].Dependents {
					childIDs = append(childIDs, dep.ID)
				}
			}
		}

		// Fallback: try bd dep list --direction blocked-by
		if len(childIDs) == 0 {
			cmd = exec.Command("bd", "dep", "list", epicID, "--json", "--direction", "blocked-by")
			cmd.Dir = projectDir
			if depOutput, depErr := cmd.Output(); depErr == nil {
				var deps []struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(depOutput, &deps); err == nil {
					for _, d := range deps {
						childIDs = append(childIDs, d.ID)
					}
				}
			}
		}

		if len(childIDs) == 0 {
			return nil, projectDir, fmt.Errorf("epic %s has no linked dependencies; add children with: bd dep add <child> %s", epicID, epicID)
		}

		// Get ready beads from this project
		readyBeads, err := bead.ReadyInDir(beadsDir)
		if err != nil {
			return nil, projectDir, fmt.Errorf("getting ready beads: %w", err)
		}

		// Filter ready beads to only those that are children of the epic
		childSet := make(map[string]bool)
		for _, id := range childIDs {
			childSet[id] = true
		}

		var epicBeads []bead.ReadyBead
		for _, b := range readyBeads {
			if childSet[b.ID] {
				epicBeads = append(epicBeads, b)
			}
		}

		return epicBeads, projectDir, nil
	}

	return nil, "", fmt.Errorf("epic %s not found in any registered project", epicID)
}

// getBeadBlockers returns IDs of beads that block the given bead
func (r *Runner) getBeadBlockers(beadID, projectDir string) ([]string, error) {
	cmd := exec.Command("bd", "dep", "list", beadID, "--json", "--direction", "blocked-by")
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var deps []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(output, &deps); err != nil {
		return nil, err
	}

	var blockers []string
	for _, d := range deps {
		blockers = append(blockers, d.ID)
	}
	return blockers, nil
}

// createEpicWorktree creates a single worktree for processing an epic
func (r *Runner) createEpicWorktree(epicID string, _ *project.Project) (string, string, error) {
	// Generate a session name from epic ID
	sessionName := strings.ReplaceAll(epicID, "-", "")
	if len(sessionName) > 8 {
		sessionName = sessionName[:8]
	}
	sessionName = "auto-" + sessionName

	// Create worktree with --shell flag so Claude doesn't auto-start.
	// runClaudeInSession will launch claude from the shell prompt with the bead prompt.
	args := []string{"new", epicID, "--no-switch", "--name", sessionName, "--shell"}
	if r.opts.Project != "" {
		args = append(args, "--project", r.opts.Project)
	}
	cmd := exec.Command("wt", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("creating worktree: %s: %w", string(output), err)
	}

	// Parse session name and worktree path from output
	lines := strings.Split(string(output), "\n")
	var worktreePath string
	for _, line := range lines {
		if strings.Contains(line, "Worktree:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				worktreePath = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "Session") && strings.Contains(line, "ready") {
			// Extract actual session name if different
			start := strings.Index(line, "'")
			if start >= 0 {
				end := strings.Index(line[start+1:], "'")
				if end >= 0 {
					sessionName = line[start+1 : start+1+end]
				}
			}
		}
	}

	if worktreePath == "" {
		worktreePath = r.cfg.WorktreePath(sessionName)
	}

	// Create batch mode marker file so wt done knows not to clean up session
	markerPath := filepath.Join(worktreePath, ".wt-batch-mode")
	if err := os.WriteFile(markerPath, []byte(epicID), 0644); err != nil {
		r.logger.Log("Warning: could not create batch mode marker: %v", err)
	}

	return sessionName, worktreePath, nil
}

// buildEpicBeadPrompt builds the prompt for processing a bead within an epic
// This is the batch-aware version that includes epic context and previous bead summaries
func (r *Runner) buildEpicBeadPrompt(b *bead.ReadyBead, sessionName string, _ *project.Project, current, total int, state *EpicState) string {
	var sb strings.Builder

	// Header with epic context
	sb.WriteString(fmt.Sprintf("You are working on bead %d/%d in epic %s: \"%s\"\n\n", current, total, state.EpicID, b.Title))

	// Epic context section
	sb.WriteString("## Epic Context\n")
	if state.EpicTitle != "" {
		sb.WriteString(fmt.Sprintf("Epic: %s\n", state.EpicTitle))
	} else {
		sb.WriteString(fmt.Sprintf("Epic ID: %s\n", state.EpicID))
	}
	sb.WriteString(fmt.Sprintf("Total beads: %d\n", total))
	sb.WriteString(fmt.Sprintf("Current: %d/%d\n\n", current, total))

	// Previous work section (if any beads completed)
	if len(state.BeadCommits) > 0 {
		sb.WriteString("## Previous Work (already committed in this worktree)\n")
		for _, commit := range state.BeadCommits {
			title := commit.Title
			if title == "" {
				title = commit.Summary
			}
			sb.WriteString(fmt.Sprintf("- %s: %s (commit %s)\n", commit.BeadID, title, commit.CommitHash))
		}
		sb.WriteString("\n")
	}

	// Your task section
	sb.WriteString("## Your Task\n")
	sb.WriteString(fmt.Sprintf("Bead: %s\n", b.ID))
	sb.WriteString(fmt.Sprintf("Title: %s\n\n", b.Title))

	if b.Description != "" {
		sb.WriteString(b.Description)
		sb.WriteString("\n\n")
	}

	// Workflow section with bead-done signal
	sb.WriteString("## Workflow\n")
	sb.WriteString("1. Review previous commits if relevant: `git log --oneline -5`\n")
	sb.WriteString("2. Implement this bead (build on existing work)\n")
	sb.WriteString("3. Commit your changes with descriptive message\n")
	sb.WriteString("4. Signal completion: `wt signal bead-done \"<brief summary of what was done>\"`\n\n")

	sb.WriteString("## Important\n")
	sb.WriteString("Do NOT:\n")
	sb.WriteString("- Create a PR (wt auto handles this after all beads)\n")
	sb.WriteString("- Run `wt done` (use `wt signal bead-done` instead)\n")
	sb.WriteString("- Modify files unrelated to this bead\n\n")

	// Commit message format
	sb.WriteString("## Commit Message Format\n")
	sb.WriteString("Include this footer in your commit messages:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<commit message>\n\n")
	sb.WriteString("Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>\n")
	sb.WriteString(fmt.Sprintf("Session: %s\n", sessionName))
	sb.WriteString("```\n")

	return sb.String()
}

// getProjectForPath finds the project containing the given path
func (r *Runner) getProjectForPath(path string) (*project.Project, error) {
	projects, err := r.projMgr.List()
	if err != nil {
		return nil, err
	}

	for _, proj := range projects {
		if proj.RepoPath() == path || strings.HasPrefix(path, proj.RepoPath()) {
			return proj, nil
		}
	}

	return nil, fmt.Errorf("no project found for path: %s", path)
}

// closeEpic closes the epic bead
func (r *Runner) closeEpic(epicID, projectDir string) error {
	cmd := exec.Command("bd", "close", epicID)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("closing epic: %s: %w", string(output), err)
	}
	return nil
}

// getEpicTitle fetches the title of an epic for batch-aware prompts
func (r *Runner) getEpicTitle(epicID, projectDir string) string {
	cmd := exec.Command("bd", "show", epicID, "--json")
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	var infos []struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(output, &infos); err != nil || len(infos) == 0 {
		return ""
	}
	return infos[0].Title
}

// Epic state file management
func (r *Runner) epicStateFile() string {
	if r.opts.Project != "" {
		return filepath.Join(r.cfg.ConfigDir(), fmt.Sprintf("auto-epic-state-%s.json", r.opts.Project))
	}
	return filepath.Join(r.cfg.ConfigDir(), "auto-epic-state.json")
}

func (r *Runner) saveEpicState(state *EpicState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.epicStateFile(), data, 0644)
}

func (r *Runner) loadEpicState() (*EpicState, error) {
	data, err := os.ReadFile(r.epicStateFile())
	if err != nil {
		return nil, err
	}
	var state EpicState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *Runner) removeEpicState() {
	os.Remove(r.epicStateFile())
}

// checkStatus shows the status of a running or paused auto session
func (r *Runner) checkStatus() error {
	// If a specific project is set (or resolved), check only that project
	if r.opts.Project != "" {
		return r.checkStatusForProject(r.opts.Project)
	}

	// No project specified: show status for all running autos
	locks, err := r.findAllAutoLocks()
	if err != nil {
		return fmt.Errorf("finding lock files: %w", err)
	}

	if len(locks) == 0 {
		fmt.Println("No wt auto sessions are running.")
		return nil
	}

	for i, lockPath := range locks {
		if i > 0 {
			fmt.Println()
		}
		projName := projectNameFromLockFile(lockPath)
		if err := r.checkStatusForProject(projName); err != nil {
			fmt.Printf("Error checking status for %s: %v\n", lockPath, err)
		}
	}
	return nil
}

// checkStatusForProject shows the status of a running auto for a specific project.
// Pass empty string for legacy global lock.
func (r *Runner) checkStatusForProject(projectName string) error {
	// Temporarily set paths for this project
	var lockFile, stateFile string
	if projectName != "" {
		lockFile = filepath.Join(r.cfg.ConfigDir(), fmt.Sprintf("auto-%s.lock", projectName))
		stateFile = filepath.Join(r.cfg.ConfigDir(), fmt.Sprintf("auto-epic-state-%s.json", projectName))
	} else {
		lockFile = filepath.Join(r.cfg.ConfigDir(), "auto.lock")
		stateFile = filepath.Join(r.cfg.ConfigDir(), "auto-epic-state.json")
	}

	// Try epic state file first
	if data, err := os.ReadFile(stateFile); err == nil {
		var state EpicState
		if err := json.Unmarshal(data, &state); err == nil {
			if projectName != "" {
				fmt.Printf("wt auto Epic Status [%s]\n", projectName)
			} else {
				fmt.Println("wt auto Epic Status")
			}
			fmt.Println("-------------------")
			fmt.Printf("  Epic:       %s\n", state.EpicID)
			fmt.Printf("  Status:     %s\n", state.Status)
			fmt.Printf("  Worktree:   %s\n", state.Worktree)
			fmt.Printf("  Session:    %s\n", state.SessionName)
			fmt.Printf("  Started:    %s\n", state.StartTime)
			fmt.Printf("  Progress:   %d/%d beads completed\n", len(state.CompletedBeads), len(state.Beads))

			if state.CurrentBead != "" {
				fmt.Printf("  Current:    %s\n", state.CurrentBead)
			}
			if len(state.FailedBeads) > 0 {
				fmt.Printf("  Failed:     %d bead(s)\n", len(state.FailedBeads))
			} else if state.FailedBead != "" {
				fmt.Printf("  Failed:     %s (%s)\n", state.FailedBead, state.FailureReason)
			}

			fmt.Println("\nBeads:")
			for i, beadID := range state.Beads {
				status := "pending"
				for _, completed := range state.CompletedBeads {
					if completed == beadID {
						status = "✓ completed"
						break
					}
				}
				if reason, failed := state.FailedBeads[beadID]; failed {
					status = fmt.Sprintf("✗ failed (%s)", reason)
				} else if beadID == state.FailedBead {
					status = "✗ failed"
				} else if beadID == state.CurrentBead && state.Status == "running" {
					status = "→ running"
				}
				fmt.Printf("  %d. %s [%s]\n", i+1, beadID, status)
			}
			return nil
		}
	}

	// Fall back to lock file
	if _, err := os.Stat(lockFile); os.IsNotExist(err) {
		if projectName != "" {
			fmt.Printf("No wt auto session is running for project %s.\n", projectName)
		} else {
			fmt.Println("No wt auto session is running.")
		}
		return nil
	}

	data, err := os.ReadFile(lockFile)
	if err != nil {
		return fmt.Errorf("reading lock file: %w", err)
	}

	var lock LockInfo
	if err := json.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("parsing lock file: %w", err)
	}

	if projectName != "" {
		fmt.Printf("wt auto Status [%s]\n", projectName)
	} else {
		fmt.Println("wt auto Status")
	}
	fmt.Println("--------------")
	fmt.Printf("  PID:       %d\n", lock.PID)
	fmt.Printf("  Started:   %s\n", lock.StartTime)
	if lock.Epic != "" {
		fmt.Printf("  Epic:      %s\n", lock.Epic)
	}
	if lock.Project != "" {
		fmt.Printf("  Project:   %s\n", lock.Project)
	}

	if r.isProcessRunning(lock.PID) {
		fmt.Printf("  Status:    running\n")
	} else {
		fmt.Printf("  Status:    stale (process not running)\n")
	}
	return nil
}

// resumeRun resumes a paused or failed epic run
func (r *Runner) resumeRun() error {
	state, err := r.loadEpicState()
	if err != nil {
		return fmt.Errorf("no epic state found to resume. Start with: wt auto --epic <id>")
	}

	if state.Status == "completed" {
		fmt.Println("Epic already completed. Nothing to resume.")
		r.removeEpicState()
		return nil
	}

	// Verify epic ID matches if provided
	if r.opts.Epic != "" && r.opts.Epic != state.EpicID {
		return fmt.Errorf("epic mismatch: state has %s, you specified %s", state.EpicID, r.opts.Epic)
	}

	fmt.Printf("Resuming epic %s...\n", state.EpicID)
	fmt.Printf("  Status: %s\n", state.Status)
	fmt.Printf("  Progress: %d/%d completed\n", len(state.CompletedBeads), len(state.Beads))

	// Find where to resume
	resumeIndex := 0
	completedSet := make(map[string]bool)
	for _, id := range state.CompletedBeads {
		completedSet[id] = true
	}

	for i, id := range state.Beads {
		if !completedSet[id] {
			resumeIndex = i
			break
		}
	}

	fmt.Printf("  Resuming from bead %d: %s\n", resumeIndex+1, state.Beads[resumeIndex])

	// Get remaining beads
	var remainingBeads []bead.ReadyBead
	for i := resumeIndex; i < len(state.Beads); i++ {
		beadID := state.Beads[i]
		// Fetch bead info
		cmd := exec.Command("bd", "show", beadID, "--json")
		cmd.Dir = state.ProjectDir
		output, _ := cmd.Output()

		var infos []bead.ReadyBead
		json.Unmarshal(output, &infos)
		if len(infos) > 0 {
			remainingBeads = append(remainingBeads, infos[0])
		} else {
			remainingBeads = append(remainingBeads, bead.ReadyBead{ID: beadID})
		}
	}

	// Get project
	proj, err := r.getProjectForPath(state.ProjectDir)
	if err != nil {
		return fmt.Errorf("finding project: %w", err)
	}

	// Update state - clear failures on resume
	state.Status = "running"
	state.FailedBead = ""
	state.FailureReason = ""
	if state.FailedBeads == nil {
		state.FailedBeads = make(map[string]string)
	} else {
		// Clear failed beads on resume - we're retrying them
		state.FailedBeads = make(map[string]string)
	}
	r.saveEpicState(state)

	// Get auto config
	autoCfg := r.getAutoConfig(proj)
	timeout := time.Duration(autoCfg.TimeoutMinutes) * time.Minute
	if r.opts.Timeout > 0 {
		timeout = time.Duration(r.opts.Timeout) * time.Minute
	}

	// Ensure BeadCommits is initialized (may be nil if loaded from old state)
	if state.BeadCommits == nil {
		state.BeadCommits = []BeadCommitInfo{}
	}

	// Process remaining beads with fresh sessions
	for i, b := range remainingBeads {
		beadNum := resumeIndex + i + 1
		totalBeads := len(state.Beads)

		if r.shouldStop() {
			state.Status = "paused"
			state.CurrentBead = b.ID
			r.saveEpicState(state)
			fmt.Printf("\nPaused at bead %d/%d. Use 'wt auto --resume' to continue.\n", beadNum, totalBeads)
			return nil
		}

		fmt.Printf("\n=== Bead %d/%d: %s ===\n", beadNum, totalBeads, b.ID)

		// Re-check bead status (may have been closed by a previous bead's commit)
		if info, err := bead.ShowInDir(b.ID, filepath.Join(state.ProjectDir, ".beads")); err == nil && info.Status == "closed" {
			b.Status = "closed"
		}

		// Skip beads that are already closed (e.g. worker batched multiple closes in one commit)
		if b.Status == "closed" {
			fmt.Printf("  Bead %s already closed, skipping\n", b.ID)
			if !slices.Contains(state.CompletedBeads, b.ID) {
				state.CompletedBeads = append(state.CompletedBeads, b.ID)
				r.saveEpicState(state)
			}
			continue
		}

		state.CurrentBead = b.ID
		r.saveEpicState(state)

		// Mark bead as in_progress so it doesn't show in `wt ready`
		if err := bead.UpdateStatusInDir(b.ID, "in_progress", state.ProjectDir); err != nil {
			r.logger.Log("Warning: could not mark bead %s as in_progress: %v", b.ID, err)
		}

		// Build batch-aware prompt (includes previous bead summaries)
		prompt := r.buildEpicBeadPrompt(&b, state.SessionName, proj, beadNum, totalBeads, state)

		outcome, err := r.runClaudeInSession(state.SessionName, autoCfg.Command, prompt, timeout)
		if err != nil || (outcome != "success" && outcome != "dry-run") {
			if r.opts.PauseOnFailure {
				state.Status = "failed"
				state.FailedBead = b.ID
				state.FailureReason = outcome
				r.saveEpicState(state)
				return fmt.Errorf("bead %s failed: %s", b.ID, outcome)
			}
			// Track failed bead
			state.FailedBeads[b.ID] = outcome
			r.saveEpicState(state)
			fmt.Printf("Warning: bead %s failed (%s), continuing...\n", b.ID, outcome)
			continue
		}

		// Capture commit info for this bead (for next bead's context)
		commitHash, commitMsg, err := r.getLatestCommitInfo(state.Worktree)
		if err != nil {
			r.logger.Log("Warning: could not get commit info: %v", err)
		} else {
			state.BeadCommits = append(state.BeadCommits, BeadCommitInfo{
				BeadID:     b.ID,
				CommitHash: commitHash,
				Summary:    commitMsg,
				Title:      b.Title,
			})
		}

		state.CompletedBeads = append(state.CompletedBeads, b.ID)
		r.saveEpicState(state)
		fmt.Printf("✓ Bead %s completed (commit: %s)\n", b.ID, commitHash)

		// Ensure Claude has exited before starting next bead (prevents pasting into REPL)
		// Only if there are more beads to process
		if i < len(remainingBeads)-1 {
			fmt.Printf("  Ensuring Claude session is terminated for next bead...\n")
			r.killClaudeSession(state.SessionName)
			// Wait for shell prompt to be ready
			r.waitForShellPrompt(state.SessionName, 10*time.Second)
		}
	}

	// Determine final status based on failures
	allSucceeded := len(state.FailedBeads) == 0
	if allSucceeded {
		state.Status = "completed"
	} else {
		state.Status = "partial"
	}
	r.saveEpicState(state)

	fmt.Printf("\n=== All %d bead(s) processed ===\n", len(state.Beads))
	fmt.Printf("  Completed: %d\n", len(state.CompletedBeads))
	if len(state.FailedBeads) > 0 {
		fmt.Printf("  Failed: %d\n", len(state.FailedBeads))
		for beadID, reason := range state.FailedBeads {
			fmt.Printf("    - %s: %s\n", beadID, reason)
		}
	}

	// Only close epic if all beads succeeded
	if allSucceeded {
		if err := r.closeEpic(state.EpicID, state.ProjectDir); err != nil {
			fmt.Printf("Warning: could not auto-close epic: %v\n", err)
		} else {
			fmt.Printf("✓ Epic %s closed\n", state.EpicID)
		}

		// Remove batch mode marker so wt done can clean up if run manually later
		batchMarkerPath := filepath.Join(state.Worktree, ".wt-batch-mode")
		os.Remove(batchMarkerPath)

		r.removeEpicState()
	} else {
		fmt.Printf("\n✗ Epic %s NOT closed due to failed beads\n", state.EpicID)
		fmt.Printf("  Fix failures and run 'wt auto --resume --epic %s' to retry\n", state.EpicID)
		fmt.Printf("  Or run 'wt auto --abort --epic %s' to clean up\n", state.EpicID)
	}

	return nil
}

// abortRun aborts a paused or failed epic run and cleans up
func (r *Runner) abortRun() error {
	state, err := r.loadEpicState()
	if err != nil {
		return fmt.Errorf("no epic state found to abort")
	}

	// Verify epic ID if provided
	if r.opts.Epic != "" && r.opts.Epic != state.EpicID {
		return fmt.Errorf("epic mismatch: state has %s, you specified %s", state.EpicID, r.opts.Epic)
	}

	fmt.Printf("Aborting epic %s...\n", state.EpicID)
	fmt.Printf("  Status: %s\n", state.Status)
	fmt.Printf("  Completed: %d/%d beads\n", len(state.CompletedBeads), len(state.Beads))

	// Kill tmux session if exists
	if state.SessionName != "" {
		fmt.Printf("  Killing session: %s\n", state.SessionName)
		cmd := exec.Command("tmux", "kill-session", "-t", state.SessionName)
		cmd.Run() // Ignore errors
	}

	// Remove worktree
	if state.Worktree != "" {
		fmt.Printf("  Removing worktree: %s\n", state.Worktree)
		cmd := exec.Command("git", "worktree", "remove", state.Worktree, "--force")
		cmd.Run() // Ignore errors
	}

	// Clean up state
	r.removeEpicState()
	r.releaseLock()

	fmt.Println("\n✓ Epic run aborted and cleaned up.")
	fmt.Printf("Note: %d bead(s) were completed before abort.\n", len(state.CompletedBeads))

	return nil
}

// getLatestCommitInfo retrieves the latest commit hash and message from a worktree
func (r *Runner) getLatestCommitInfo(worktreePath string) (hash, message string, err error) {
	// Get the latest commit hash
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = worktreePath
	hashOutput, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("getting commit hash: %w", err)
	}
	hash = strings.TrimSpace(string(hashOutput))

	// Get the commit message (first line)
	cmd = exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = worktreePath
	msgOutput, err := cmd.Output()
	if err != nil {
		return hash, "", fmt.Errorf("getting commit message: %w", err)
	}
	message = strings.TrimSpace(string(msgOutput))

	return hash, message, nil
}

// killClaudeSession terminates the Claude process in a tmux session without killing the session
func (r *Runner) killClaudeSession(sessionName string) error {
	return KillClaudeInSession(sessionName)
}

// KillClaudeInSession terminates the Claude process in a tmux session without killing the session.
// Exported for use by wt signal bead-done.
func KillClaudeInSession(sessionName string) error {
	// Send Ctrl+C to gracefully stop claude, then wait a moment
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, "C-c")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not send Ctrl+C to session: %v\n", err)
	}

	// Wait for Claude to exit
	time.Sleep(2 * time.Second)

	// If still running, send another Ctrl+C
	cmd = exec.Command("tmux", "display-message", "-t", sessionName, "-p", "#{pane_pid}")
	output, err := cmd.Output()
	if err == nil {
		pid := strings.TrimSpace(string(output))
		if pid != "" {
			// Check if the shell has child processes (claude running)
			cmd = exec.Command("pgrep", "-P", pid)
			if cmd.Run() == nil {
				// Still has children, send another Ctrl+C
				cmd = exec.Command("tmux", "send-keys", "-t", sessionName, "C-c")
				cmd.Run()
				time.Sleep(1 * time.Second)
			}
		}
	}

	return nil
}

// --- Exported functions for wt signal bead-done ---

// EpicStateFile returns the path to the epic state file
func EpicStateFile(cfg *config.Config) string {
	return filepath.Join(cfg.ConfigDir(), "auto-epic-state.json")
}

// LoadEpicState loads the current epic state from disk
func LoadEpicState(cfg *config.Config) (*EpicState, error) {
	data, err := os.ReadFile(EpicStateFile(cfg))
	if err != nil {
		return nil, err
	}
	var state EpicState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveEpicState saves the epic state to disk
func SaveEpicState(cfg *config.Config, state *EpicState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(EpicStateFile(cfg), data, 0644)
}

// RemoveEpicState removes the epic state file
func RemoveEpicState(cfg *config.Config) {
	os.Remove(EpicStateFile(cfg))
}

// IsInAutoMode checks if the given worktree is part of an active auto mode epic
func IsInAutoMode(cfg *config.Config, worktreePath string) (*EpicState, bool) {
	state, err := LoadEpicState(cfg)
	if err != nil {
		return nil, false
	}
	// Check if this worktree matches the epic's worktree
	if state.Worktree == worktreePath && state.Status == "running" {
		return state, true
	}
	return nil, false
}

// HandleBeadDone handles the bead-done signal in auto mode.
// This is the main orchestration function called by wt signal bead-done.
func HandleBeadDone(cfg *config.Config, state *EpicState, summary string) error {
	fmt.Println("\n=== Auto Mode: Bead Complete ===")

	// 1. Post-bead housekeeping
	if err := postBeadHousekeeping(cfg, state, summary); err != nil {
		return fmt.Errorf("post-bead housekeeping: %w", err)
	}

	// 2. Check if there are more beads
	nextBeadIndex := len(state.CompletedBeads)
	if nextBeadIndex >= len(state.Beads) {
		// All beads done - finalize epic
		return finalizeEpic(cfg, state)
	}

	// 3. Pre-bead housekeeping for next bead
	nextBeadID := state.Beads[nextBeadIndex]
	if err := preBeadHousekeeping(cfg, state, nextBeadID); err != nil {
		return fmt.Errorf("pre-bead housekeeping: %w", err)
	}

	// 4. Kill Claude and start next bead
	if err := transitionToNextBead(cfg, state, nextBeadID, nextBeadIndex); err != nil {
		return fmt.Errorf("transitioning to next bead: %w", err)
	}

	return nil
}

// postBeadHousekeeping handles tasks after a bead completes
func postBeadHousekeeping(cfg *config.Config, state *EpicState, summary string) error {
	currentBead := state.CurrentBead
	fmt.Printf("Post-bead housekeeping for %s...\n", currentBead)

	// Capture commit info
	commitHash, commitMsg, err := getLatestCommit(state.Worktree)
	if err != nil {
		fmt.Printf("Warning: could not get commit info: %v\n", err)
	} else {
		state.BeadCommits = append(state.BeadCommits, BeadCommitInfo{
			BeadID:     currentBead,
			CommitHash: commitHash,
			Summary:    commitMsg,
			Title:      state.BeadTitles[currentBead],
		})
		fmt.Printf("  Commit: %s\n", commitHash)
	}

	// Mark bead as complete in state
	state.CompletedBeads = append(state.CompletedBeads, currentBead)
	fmt.Printf("  Progress: %d/%d beads completed\n", len(state.CompletedBeads), len(state.Beads))

	// Close the bead
	fmt.Printf("  Closing bead %s...\n", currentBead)
	cmd := exec.Command("bd", "close", currentBead, "--reason", summary)
	cmd.Dir = state.ProjectDir
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("  Warning: could not close bead: %s\n", string(output))
	}

	// Sync beads
	fmt.Println("  Syncing beads...")
	cmd = exec.Command("bd", "sync")
	cmd.Dir = state.ProjectDir
	cmd.Run() // Ignore errors

	// Save state
	if err := SaveEpicState(cfg, state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

// preBeadHousekeeping handles tasks before starting a new bead
func preBeadHousekeeping(cfg *config.Config, state *EpicState, beadID string) error {
	fmt.Printf("Pre-bead housekeeping for %s...\n", beadID)

	// Mark bead as in_progress
	cmd := exec.Command("bd", "update", beadID, "--status", "in_progress")
	cmd.Dir = state.ProjectDir
	if err := cmd.Run(); err != nil {
		fmt.Printf("  Warning: could not mark bead as in_progress: %v\n", err)
	}

	// Update current bead in state
	state.CurrentBead = beadID
	if err := SaveEpicState(cfg, state); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

// transitionToNextBead kills the current Claude session and starts the next bead
func transitionToNextBead(cfg *config.Config, state *EpicState, beadID string, beadIndex int) error {
	fmt.Printf("\n=== Starting bead %d/%d: %s ===\n", beadIndex+1, len(state.Beads), beadID)
	fmt.Printf("Title: %s\n", state.BeadTitles[beadID])

	// Kill current Claude session
	fmt.Println("Ending current Claude session...")
	if err := KillClaudeInSession(state.SessionName); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}

	// Wait for shell to stabilize
	time.Sleep(2 * time.Second)

	// Build prompt for next bead
	prompt := BuildEpicBeadPrompt(beadID, state, beadIndex+1)

	// Send prompt via NudgeSession
	fmt.Println("Sending prompt to Claude...")
	if err := nudgeSession(state.SessionName, prompt); err != nil {
		return fmt.Errorf("sending prompt: %w", err)
	}

	fmt.Printf("\n✓ Bead %s started. Worker should now process it.\n", beadID)
	return nil
}

// finalizeEpic completes the epic after all beads are done
func finalizeEpic(cfg *config.Config, state *EpicState) error {
	fmt.Println("\n=== All Beads Complete! ===")
	fmt.Printf("Completed %d beads in epic %s\n", len(state.CompletedBeads), state.EpicID)

	// Close the epic
	fmt.Printf("Closing epic %s...\n", state.EpicID)
	cmd := exec.Command("bd", "close", state.EpicID)
	cmd.Dir = state.ProjectDir
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Warning: could not close epic: %s\n", string(output))
	} else {
		fmt.Printf("✓ Epic %s closed\n", state.EpicID)
	}

	// Sync beads
	cmd = exec.Command("bd", "sync")
	cmd.Dir = state.ProjectDir
	cmd.Run()

	// Update state
	state.Status = "completed"
	state.CurrentBead = ""
	if err := SaveEpicState(cfg, state); err != nil {
		fmt.Printf("Warning: could not save final state: %v\n", err)
	}

	// Remove batch mode marker
	batchMarkerPath := filepath.Join(state.Worktree, ".wt-batch-mode")
	os.Remove(batchMarkerPath)

	fmt.Println("\n=== Epic Processing Complete ===")
	fmt.Printf("Session '%s' remains active for final review.\n", state.SessionName)
	fmt.Println("Run 'wt done' when ready to clean up, or create a PR manually.")

	return nil
}

// BuildEpicBeadPrompt builds the prompt for a bead in an epic.
// Exported for use by wt signal.
func BuildEpicBeadPrompt(beadID string, state *EpicState, beadNum int) string {
	var sb strings.Builder

	total := len(state.Beads)
	title := state.BeadTitles[beadID]

	// Header with epic context
	sb.WriteString(fmt.Sprintf("You are working on bead %d/%d in epic %s: \"%s\"\n\n", beadNum, total, state.EpicID, title))

	// Epic context section
	sb.WriteString("## Epic Context\n")
	if state.EpicTitle != "" {
		sb.WriteString(fmt.Sprintf("Epic: %s\n", state.EpicTitle))
	} else {
		sb.WriteString(fmt.Sprintf("Epic ID: %s\n", state.EpicID))
	}
	sb.WriteString(fmt.Sprintf("Total beads: %d\n", total))
	sb.WriteString(fmt.Sprintf("Current: %d/%d\n\n", beadNum, total))

	// Previous work section (if any beads completed)
	if len(state.BeadCommits) > 0 {
		sb.WriteString("## Previous Work (already committed in this worktree)\n")
		for _, commit := range state.BeadCommits {
			t := commit.Title
			if t == "" {
				t = commit.Summary
			}
			sb.WriteString(fmt.Sprintf("- %s: %s (commit %s)\n", commit.BeadID, t, commit.CommitHash))
		}
		sb.WriteString("\n")
	}

	// Get bead description
	description := getBeadDescription(beadID, state.ProjectDir)

	// Your task section
	sb.WriteString("## Your Task\n")
	sb.WriteString(fmt.Sprintf("Bead: %s\n", beadID))
	sb.WriteString(fmt.Sprintf("Title: %s\n\n", title))

	if description != "" {
		sb.WriteString(description)
		sb.WriteString("\n\n")
	}

	// Workflow section with bead-done signal
	sb.WriteString("## Workflow\n")
	sb.WriteString("1. Review previous commits if relevant: `git log --oneline -5`\n")
	sb.WriteString("2. Implement this bead (build on existing work)\n")
	sb.WriteString("3. Commit your changes with descriptive message\n")
	sb.WriteString("4. Signal completion: `wt signal bead-done \"<brief summary of what was done>\"`\n\n")

	sb.WriteString("## Important\n")
	sb.WriteString("Do NOT:\n")
	sb.WriteString("- Create a PR (wt auto handles this after all beads)\n")
	sb.WriteString("- Run `wt done` (use `wt signal bead-done` instead)\n")
	sb.WriteString("- Modify files unrelated to this bead\n\n")

	// Commit message format
	sb.WriteString("## Commit Message Format\n")
	sb.WriteString("Include this footer in your commit messages:\n\n")
	sb.WriteString("```\n")
	sb.WriteString("<commit message>\n\n")
	sb.WriteString("Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>\n")
	sb.WriteString(fmt.Sprintf("Session: %s\n", state.SessionName))
	sb.WriteString("```\n")

	return sb.String()
}

// getBeadDescription fetches the description for a bead
func getBeadDescription(beadID, projectDir string) string {
	cmd := exec.Command("bd", "show", beadID, "--json")
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	var infos []struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(output, &infos); err != nil || len(infos) == 0 {
		return ""
	}
	return infos[0].Description
}

// getLatestCommit retrieves the latest commit hash and message from a worktree
func getLatestCommit(worktreePath string) (hash, message string, err error) {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = worktreePath
	hashOutput, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("getting commit hash: %w", err)
	}
	hash = strings.TrimSpace(string(hashOutput))

	cmd = exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = worktreePath
	msgOutput, err := cmd.Output()
	if err != nil {
		return hash, "", fmt.Errorf("getting commit message: %w", err)
	}
	message = strings.TrimSpace(string(msgOutput))

	return hash, message, nil
}

// nudgeSession sends a message to a tmux session using the reliable paste-buffer pattern
func nudgeSession(sessionName, message string) error {
	// Write message to temp file
	tmpFile, err := os.CreateTemp("", "wt-auto-nudge-*.txt")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(message); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmpFile.Close()

	// Load into tmux buffer
	loadCmd := exec.Command("tmux", "load-buffer", tmpPath)
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("loading buffer: %w", err)
	}

	// Paste buffer to the target pane
	pasteCmd := exec.Command("tmux", "paste-buffer", "-t", sessionName)
	if err := pasteCmd.Run(); err != nil {
		return fmt.Errorf("pasting buffer to %s: %w", sessionName, err)
	}

	// Wait then send Enter
	time.Sleep(500 * time.Millisecond)
	enterCmd := exec.Command("tmux", "send-keys", "-t", sessionName, "Enter")
	if err := enterCmd.Run(); err != nil {
		return fmt.Errorf("sending Enter to %s: %w", sessionName, err)
	}

	return nil
}
