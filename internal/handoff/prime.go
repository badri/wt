package handoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/hub"
	"github.com/badri/wt/internal/session"
)

const (
	// SessionIDFile is the name of the file that stores the Claude session ID
	SessionIDFile = "session_id"
)

// PrimeOptions configures prime behavior
type PrimeOptions struct {
	Quiet     bool // Suppress non-essential output
	NoBdPrime bool // Skip running bd prime
	HookMode  bool // Read session_id from Claude's SessionStart hook JSON on stdin
}

// PrimeResult contains the outcome of a prime operation
type PrimeResult struct {
	IsPostHandoff     bool
	PrevSession       string
	HandoffTime       time.Time
	HandoffContent    string
	BdPrimeOutput     string
	IsPostCompaction  bool        // Session resumed after compaction
	CheckpointContent *Checkpoint // Recovered checkpoint data
}

// Prime injects context on session startup
func Prime(cfg *config.Config, opts *PrimeOptions) (*PrimeResult, error) {
	result := &PrimeResult{}

	// 1. Check for handoff marker (explicit handoff via wt handoff)
	exists, prevSession, handoffTime, err := CheckMarker(cfg)
	if err != nil {
		return nil, fmt.Errorf("checking marker: %w", err)
	}

	if exists {
		result.IsPostHandoff = true
		result.PrevSession = prevSession
		result.HandoffTime = handoffTime

		// Clear the marker
		if err := ClearMarker(cfg); err != nil {
			fmt.Printf("Warning: could not clear handoff marker: %v\n", err)
		}
	}

	// 2. Check for checkpoint (compaction recovery)
	checkpoint, err := LoadCheckpoint()
	if err != nil {
		// Non-fatal
		if !opts.Quiet {
			fmt.Printf("Warning: could not load checkpoint: %v\n", err)
		}
	}
	if checkpoint != nil {
		result.IsPostCompaction = true
		result.CheckpointContent = checkpoint
	}

	// 3. Get handoff content - from hub bead or legacy file
	inHub := os.Getenv("WT_HUB") == "1"
	var content string
	if inHub {
		// Try hub handoff bead first
		content, err = hub.ReadHandoffBead(cfg)
		if err != nil {
			if !opts.Quiet {
				fmt.Printf("Note: hub bead not available (%v), using file\n", err)
			}
			// Fall back to file
			content, err = GetHandoffContent(cfg)
		}
	} else {
		content, err = GetHandoffContent(cfg)
	}
	if err != nil && !opts.Quiet {
		fmt.Printf("Warning: could not get handoff content: %v\n", err)
	}
	result.HandoffContent = content

	// 4. Run bd prime if not disabled
	if !opts.NoBdPrime {
		bdOutput, err := runBdPrime()
		if err != nil {
			// Non-fatal - bd might not be available
			if !opts.Quiet {
				fmt.Printf("Note: bd prime not available: %v\n", err)
			}
		} else {
			result.BdPrimeOutput = bdOutput
		}
	}

	return result, nil
}

// OutputPrimeResult outputs the prime result in a formatted way
func OutputPrimeResult(result *PrimeResult) {
	// Post-compaction recovery (highest priority)
	if result.IsPostCompaction && result.CheckpointContent != nil {
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║  🔄 CONTEXT RECOVERED - Resuming after compaction            ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println(FormatCheckpointForRecovery(result.CheckpointContent))
		return // Don't show other content when recovering from compaction
	}

	// Post-handoff warning
	if result.IsPostHandoff {
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║  ✅ HANDOFF COMPLETE - You are the NEW session               ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		fmt.Println()
		if result.PrevSession != "" {
			fmt.Printf("Your predecessor (%s) handed off to you.\n", result.PrevSession)
		}
		if !result.HandoffTime.IsZero() {
			fmt.Printf("Handoff time: %s\n", result.HandoffTime.Format(time.RFC3339))
		}
		fmt.Println()
		fmt.Println("⚠️  DO NOT run /handoff - that was your predecessor's action.")
		fmt.Println("    The /handoff you see in context is NOT a request for you.")
		fmt.Println()
	}

	// Handoff content
	if result.HandoffContent != "" {
		fmt.Println("## 🤝 Handoff from Previous Session")
		fmt.Println()
		fmt.Println(result.HandoffContent)
		// Note: Content is cleared by the caller after display if needed
	}

	// bd prime output
	if result.BdPrimeOutput != "" {
		fmt.Println(result.BdPrimeOutput)
	}

	// Startup suggestions if not post-handoff and not post-compaction
	if !result.IsPostHandoff && !result.IsPostCompaction && result.HandoffContent == "" {
		fmt.Println("## 🚀 Session Started")
		fmt.Println()
		fmt.Println("Quick actions:")
		fmt.Println("  wt ready     - See available work")
		fmt.Println("  wt list      - See active sessions")
		fmt.Println("  bd ready     - See ready beads")
		fmt.Println()
	}
}

// runBdPrime runs bd prime and returns its output
func runBdPrime() (string, error) {
	cmd := exec.Command("bd", "prime")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// GenerateSummary creates a summary of current state for handoff or context
func GenerateSummary(cfg *config.Config) (string, error) {
	var sb strings.Builder

	sb.WriteString("## Current State Summary\n\n")

	// Active sessions
	state, err := session.LoadState(cfg)
	if err == nil {
		if len(state.Sessions) > 0 {
			sb.WriteString("### Active Sessions\n")
			for name, sess := range state.Sessions {
				sb.WriteString(fmt.Sprintf("- **%s**: bead=%s, project=%s\n", name, sess.Bead, sess.Project))
			}
			sb.WriteString("\n")
		} else {
			sb.WriteString("### Active Sessions\nNo active sessions.\n\n")
		}
	}

	// Ready beads (top 5)
	readyBeads, err := bead.Ready()
	if err == nil {
		if len(readyBeads) > 0 {
			sb.WriteString("### Ready Beads (Top 5)\n")
			count := len(readyBeads)
			if count > 5 {
				count = 5
			}
			for i := 0; i < count; i++ {
				b := readyBeads[i]
				sb.WriteString(fmt.Sprintf("- %s: %s (P%d)\n", b.ID, b.Title, b.Priority))
			}
			if len(readyBeads) > 5 {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(readyBeads)-5))
			}
			sb.WriteString("\n")
		}
	}

	// In-progress work
	inProgress, err := bead.List("in_progress")
	if err == nil && len(inProgress) > 0 {
		sb.WriteString("### In Progress\n")
		for _, b := range inProgress {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", b.ID, b.Title))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// QuickPrime is a lightweight version of Prime that only outputs essential info
func QuickPrime(cfg *config.Config) error {
	// Check for handoff marker
	exists, prevSession, handoffTime, err := CheckMarker(cfg)
	if err != nil {
		return err
	}

	if exists {
		fmt.Println("╔══════════════════════════════════════════════════════════════╗")
		fmt.Println("║  ✅ POST-HANDOFF SESSION                                      ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════╝")
		if prevSession != "" {
			fmt.Printf("Previous: %s at %s\n", prevSession, handoffTime.Format("15:04:05"))
		}
		fmt.Println("⚠️  DO NOT run handoff again - check your tasks instead.")
		fmt.Println()

		// Clear marker
		ClearMarker(cfg)
	}

	// Get handoff content - from hub bead or legacy file
	inHub := os.Getenv("WT_HUB") == "1"
	var content string
	if inHub {
		content, _ = hub.ReadHandoffBead(cfg)
	} else {
		content, _ = GetHandoffContent(cfg)
	}
	if content != "" {
		fmt.Println("## Handoff Notes")
		fmt.Println(content)
		// Clear after displaying
		if inHub {
			hub.ClearHandoffBead(cfg)
		} else {
			ClearHandoffContent(cfg)
		}
	}

	return nil
}

// ClaudeHookData represents the JSON structure from Claude's SessionStart hook
type ClaudeHookData struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
}

// PrimeHook handles the --hook mode for wt prime.
// It reads Claude's SessionStart hook JSON from stdin, extracts the session_id,
// and persists it to .wt/session_id in the current directory.
func PrimeHook() error {
	// Read JSON from stdin
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}

	// Parse JSON
	var hookData ClaudeHookData
	if err := json.Unmarshal(data, &hookData); err != nil {
		return fmt.Errorf("parsing hook JSON: %w", err)
	}

	// Validate session_id
	if hookData.SessionID == "" {
		return fmt.Errorf("no session_id in hook data")
	}

	// Persist session_id to .wt/session_id in current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}

	runtimeDir := filepath.Join(cwd, RuntimeDir)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("creating runtime dir: %w", err)
	}

	sessionIDPath := filepath.Join(runtimeDir, SessionIDFile)
	if err := os.WriteFile(sessionIDPath, []byte(hookData.SessionID), 0644); err != nil {
		return fmt.Errorf("writing session_id: %w", err)
	}

	return nil
}

// ReadSessionID reads the Claude session ID from .wt/session_id in the given directory.
// Returns empty string if the file doesn't exist.
func ReadSessionID(dir string) string {
	sessionIDPath := filepath.Join(dir, RuntimeDir, SessionIDFile)
	data, err := os.ReadFile(sessionIDPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
