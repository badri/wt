package auto

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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
	return &Runner{
		cfg:        cfg,
		projMgr:    project.NewManager(cfg),
		opts:       opts,
		lockFile:   filepath.Join(cfg.ConfigDir(), "auto.lock"),
		stopFile:   filepath.Join(cfg.ConfigDir(), "stop-auto"),
		stopSignal: make(chan struct{}, 1),
	}
}

// Run executes the auto loop
func (r *Runner) Run() error {
	// Handle --stop flag
	if r.opts.Stop {
		return r.signalStop()
	}

	// Handle --abort flag
	if r.opts.Abort {
		return r.abortRun()
	}

	// Require --epic flag (no longer allows processing all ready beads)
	if r.opts.Epic == "" && !r.opts.Check {
		return fmt.Errorf("--epic <id> is required\n\nwt auto now requires an epic to process. This prevents accidental batch processing.\n\nUsage:\n  wt auto --epic <epic-id>    Process all beads in an epic\n  wt auto --check             Check status of a running auto session\n\nExample:\n  bd create \"Documentation batch\" -t epic\n  bd dep add wt-tcf wt-doc-epic   # wt-tcf blocks wt-doc-epic (dep of epic)\n  bd dep add wt-1a3 wt-doc-epic\n  wt auto --epic wt-doc-epic")
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

	// Handle --check flag (status check)
	if r.opts.Check {
		return r.checkStatus()
	}

	// Handle --resume flag
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

	// Send command to tmux session using the prompt file.
	// Use cat with command substitution to read the prompt, then delete the temp file.
	// Prefix with space to avoid shell history.
	fullCmd := fmt.Sprintf(" %s -p \"$(cat %q)\" && rm -f %q", command, promptPath, promptPath)
	tmuxCmd := exec.Command("tmux", "send-keys", "-t", sessionName, fullCmd, "Enter")
	if err := tmuxCmd.Run(); err != nil {
		os.Remove(promptPath) // Clean up on error
		return "failed-send", fmt.Errorf("sending command to tmux: %w", err)
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
	// Check if auto is running
	if _, err := os.Stat(r.lockFile); os.IsNotExist(err) {
		return fmt.Errorf("no wt auto is currently running")
	}

	// Create stop file
	if err := os.WriteFile(r.stopFile, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
		return fmt.Errorf("creating stop signal: %w", err)
	}

	fmt.Println("Stop signal sent. wt auto will stop after the current bead.")
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

// processEpic processes all beads in an epic sequentially in a single worktree
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
		return nil
	}

	// Get project for this epic
	proj, err := r.getProjectForPath(projectDir)
	if err != nil {
		return fmt.Errorf("finding project: %w", err)
	}

	// Create single worktree for the epic
	sessionName, worktreePath, err := r.createEpicWorktree(epicID, proj)
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	fmt.Printf("\nCreated worktree: %s\n", worktreePath)
	fmt.Printf("Session: %s\n", sessionName)

	// Fetch epic title for batch-aware prompts
	epicTitle := r.getEpicTitle(epicID, projectDir)

	// Save epic state for resume/abort
	state := &EpicState{
		EpicID:      epicID,
		EpicTitle:   epicTitle,
		Worktree:    worktreePath,
		SessionName: sessionName,
		Beads:       make([]string, len(beads)),
		BeadTitles:  make(map[string]string),
		FailedBeads: make(map[string]string),
		BeadCommits: []BeadCommitInfo{},
		Status:      "running",
		StartTime:   time.Now().Format(time.RFC3339),
		ProjectDir:  projectDir,
		MergeMode:   r.opts.MergeMode,
	}
	for i, b := range beads {
		state.Beads[i] = b.ID
		state.BeadTitles[b.ID] = b.Title
	}
	if err := r.saveEpicState(state); err != nil {
		r.logger.Log("Warning: could not save epic state: %v", err)
	}

	// Get auto config
	autoCfg := r.getAutoConfig(proj)
	timeout := time.Duration(autoCfg.TimeoutMinutes) * time.Minute
	if r.opts.Timeout > 0 {
		timeout = time.Duration(r.opts.Timeout) * time.Minute
	}

	// Process beads sequentially in the shared worktree with fresh sessions
	fmt.Printf("\nProcessing %d bead(s) sequentially (fresh session per bead)...\n", len(beads))
	for i, b := range beads {
		if r.shouldStop() {
			r.logger.Log("Stop signal received, pausing epic")
			state.Status = "paused"
			state.CurrentBead = b.ID
			r.saveEpicState(state)
			fmt.Printf("\nPaused at bead %d/%d. Use 'wt auto --resume --epic %s' to continue.\n", i+1, len(beads), epicID)
			return nil
		}

		fmt.Printf("\n=== Bead %d/%d: %s ===\n", i+1, len(beads), b.ID)
		fmt.Printf("Title: %s\n", b.Title)

		state.CurrentBead = b.ID
		r.saveEpicState(state)

		// Mark bead as in_progress so it doesn't show in `wt ready`
		if err := bead.UpdateStatusInDir(b.ID, "in_progress", projectDir); err != nil {
			r.logger.Log("Warning: could not mark bead %s as in_progress: %v", b.ID, err)
		}

		// Build batch-aware prompt for this bead (includes previous bead summaries)
		prompt := r.buildEpicBeadPrompt(&b, sessionName, proj, i+1, len(beads), state)

		// Run claude for this bead
		outcome, err := r.runClaudeInSession(sessionName, autoCfg.Command, prompt, timeout)
		if err != nil || (outcome != "success" && outcome != "dry-run") {
			r.logger.Log("Bead %s failed: %s", b.ID, outcome)

			if r.opts.PauseOnFailure {
				state.Status = "failed"
				state.FailedBead = b.ID
				state.FailureReason = outcome
				r.saveEpicState(state)
				fmt.Printf("\n✗ Bead %s failed (%s). Worktree preserved.\n", b.ID, outcome)
				fmt.Printf("Options:\n")
				fmt.Printf("  - Fix manually in %s and run 'wt auto --resume --epic %s'\n", worktreePath, epicID)
				fmt.Printf("  - Run 'wt auto --abort --epic %s' to clean up\n", epicID)
				return fmt.Errorf("bead %s failed: %s", b.ID, outcome)
			}

			// Track failed bead
			state.FailedBeads[b.ID] = outcome
			r.saveEpicState(state)
			fmt.Printf("Warning: bead %s failed (%s), continuing...\n", b.ID, outcome)
			continue
		}

		// Capture commit info for this bead (for next bead's context)
		commitHash, commitMsg, err := r.getLatestCommitInfo(worktreePath)
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

		// Kill Claude session to start fresh for next bead (prevents context rot)
		// Only if there are more beads to process
		if i < len(beads)-1 {
			fmt.Printf("  Ending Claude session for fresh context on next bead...\n")
			r.killClaudeSession(sessionName)
			// Brief delay to let shell stabilize before starting new Claude
			time.Sleep(2 * time.Second)
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

	fmt.Printf("\n=== All %d bead(s) processed ===\n", len(beads))
	fmt.Printf("  Completed: %d\n", len(state.CompletedBeads))
	if len(state.FailedBeads) > 0 {
		fmt.Printf("  Failed: %d\n", len(state.FailedBeads))
		for beadID, reason := range state.FailedBeads {
			fmt.Printf("    - %s: %s\n", beadID, reason)
		}
	}

	// Determine merge mode
	mergeMode := r.opts.MergeMode
	if mergeMode == "" {
		mergeMode = proj.MergeMode
	}
	if mergeMode == "" {
		mergeMode = r.cfg.DefaultMergeMode
	}

	if mergeMode != "none" && allSucceeded {
		fmt.Printf("Merge mode: %s\n", mergeMode)
		// TODO: Implement merge/PR creation
		fmt.Println("(Merge/PR creation not yet implemented)")
	}

	// Only close epic if all beads succeeded
	if allSucceeded {
		if err := r.closeEpic(epicID, projectDir); err != nil {
			r.logger.Log("Warning: could not close epic: %v", err)
			fmt.Printf("Warning: could not auto-close epic: %v\n", err)
		} else {
			fmt.Printf("✓ Epic %s closed\n", epicID)
		}

		// Remove batch mode marker so wt done can clean up if run manually later
		batchMarkerPath := filepath.Join(worktreePath, ".wt-batch-mode")
		os.Remove(batchMarkerPath)

		// Clean up state file
		r.removeEpicState()
	} else {
		fmt.Printf("\n✗ Epic %s NOT closed due to failed beads\n", epicID)
		fmt.Printf("  Fix failures and run 'wt auto --resume --epic %s' to retry\n", epicID)
		fmt.Printf("  Or run 'wt auto --abort --epic %s' to clean up\n", epicID)
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

		// Get dependencies that block this epic (beads where epic depends on them)
		cmd = exec.Command("bd", "dep", "list", epicID, "--json", "--direction", "blocked-by")
		cmd.Dir = projectDir
		output, err = cmd.Output()
		if err != nil {
			// Try alternative: list all beads that this epic depends on
			cmd = exec.Command("bd", "show", epicID, "--json")
			cmd.Dir = projectDir
			output, err = cmd.Output()
			if err != nil {
				return nil, projectDir, fmt.Errorf("getting epic dependencies: %w", err)
			}
		}

		// Get ready beads from this project
		readyBeads, err := bead.ReadyInDir(beadsDir)
		if err != nil {
			return nil, projectDir, fmt.Errorf("getting ready beads: %w", err)
		}

		// Parse dependencies
		var deps []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		json.Unmarshal(output, &deps)

		// Filter ready beads to only those that are dependencies of the epic
		var epicBeads []bead.ReadyBead
		depSet := make(map[string]bool)
		for _, d := range deps {
			depSet[d.ID] = true
		}

		for _, b := range readyBeads {
			if depSet[b.ID] {
				epicBeads = append(epicBeads, b)
			}
		}

		// If no deps found via dep command, assume all ready beads with matching prefix
		if len(deps) == 0 && len(epicBeads) == 0 {
			// Fall back to pattern matching: beads with same prefix
			prefix := strings.Split(epicID, "-")[0]
			for _, b := range readyBeads {
				if strings.HasPrefix(b.ID, prefix+"-") && b.ID != epicID {
					epicBeads = append(epicBeads, b)
				}
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

	// Create worktree using wt new equivalent logic
	cmd := exec.Command("wt", "new", epicID, "--no-switch", "--name", sessionName)
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
	// Check for epic state file
	state, err := r.loadEpicState()
	if err != nil {
		// Check lock file
		if _, err := os.Stat(r.lockFile); os.IsNotExist(err) {
			fmt.Println("No wt auto session is running.")
			return nil
		}

		// Read lock info
		data, err := os.ReadFile(r.lockFile)
		if err != nil {
			return fmt.Errorf("reading lock file: %w", err)
		}

		var lock LockInfo
		if err := json.Unmarshal(data, &lock); err != nil {
			return fmt.Errorf("parsing lock file: %w", err)
		}

		fmt.Println("wt auto Status")
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

	// Show epic state
	fmt.Println("wt auto Epic Status")
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
		// Backwards compatibility with old state format
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
		// Check FailedBeads map first
		if reason, failed := state.FailedBeads[beadID]; failed {
			status = fmt.Sprintf("✗ failed (%s)", reason)
		} else if beadID == state.FailedBead {
			// Backwards compatibility
			status = "✗ failed"
		} else if beadID == state.CurrentBead && state.Status == "running" {
			status = "→ running"
		}
		fmt.Printf("  %d. %s [%s]\n", i+1, beadID, status)
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

		// Kill Claude session to start fresh for next bead (prevents context rot)
		// Only if there are more beads to process
		if i < len(remainingBeads)-1 {
			fmt.Printf("  Ending Claude session for fresh context on next bead...\n")
			r.killClaudeSession(state.SessionName)
			// Brief delay to let shell stabilize before starting new Claude
			time.Sleep(2 * time.Second)
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
	// Send Ctrl+C to gracefully stop claude, then wait a moment
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, "C-c")
	if err := cmd.Run(); err != nil {
		r.logger.Log("Warning: could not send Ctrl+C to session: %v", err)
	}

	// Wait for Claude to exit
	time.Sleep(2 * time.Second)

	// If still running, send another Ctrl+C
	if r.isSessionActive(sessionName) {
		cmd = exec.Command("tmux", "send-keys", "-t", sessionName, "C-c")
		cmd.Run()
		time.Sleep(1 * time.Second)
	}

	return nil
}
