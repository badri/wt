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
	Project   string
	MergeMode string
	DryRun    bool
	Check     bool
	Stop      bool
	Force     bool
	Timeout   int // minutes, 0 means use project default
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

	// Clean up any existing stop file
	os.Remove(r.stopFile)

	// Setup signal handling
	r.setupSignalHandler()

	// Get projects to process
	projects, err := r.getProjects()
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		fmt.Println("No projects to process.")
		return nil
	}

	r.logger.Log("Starting auto run for %d project(s)", len(projects))

	// Process each project
	for _, proj := range projects {
		if r.shouldStop() {
			r.logger.Log("Stop signal received, exiting")
			fmt.Println("Stop signal received, exiting after current project.")
			break
		}

		if err := r.processProject(proj); err != nil {
			r.logger.Log("Error processing project %s: %v", proj.Name, err)
			fmt.Printf("Error processing project %s: %v\n", proj.Name, err)
		}
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

	fmt.Printf("Found %d ready bead(s) in project %s.\n", len(readyBeads), proj.Name)

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
	// Expected: "Created session 'toast' for bead wt-xyz"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Created session") {
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
	// Send command to tmux session
	fullCmd := fmt.Sprintf("%s -p %q", command, prompt)
	tmuxCmd := exec.Command("tmux", "send-keys", "-t", sessionName, fullCmd, "Enter")
	if err := tmuxCmd.Run(); err != nil {
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
func (r *Runner) getAutoConfig(proj *project.Project) *Config {
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
}
