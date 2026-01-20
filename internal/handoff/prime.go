package handoff

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/badri/wt/internal/bead"
	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/session"
)

// PrimeOptions configures prime behavior
type PrimeOptions struct {
	Quiet     bool // Suppress non-essential output
	NoBdPrime bool // Skip running bd prime
}

// PrimeResult contains the outcome of a prime operation
type PrimeResult struct {
	IsPostHandoff  bool
	PrevSession    string
	HandoffTime    time.Time
	HandoffContent string
	BdPrimeOutput  string
}

// Prime injects context on session startup
func Prime(cfg *config.Config, opts *PrimeOptions) (*PrimeResult, error) {
	result := &PrimeResult{}

	// 1. Check for handoff marker
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

	// 2. Get handoff content from file
	content, err := GetHandoffContent(cfg)
	if err != nil {
		fmt.Printf("Warning: could not get handoff content: %v\n", err)
	}
	result.HandoffContent = content

	// 3. Run bd prime if not disabled
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
	}

	// bd prime output
	if result.BdPrimeOutput != "" {
		fmt.Println(result.BdPrimeOutput)
	}

	// Startup suggestions if not post-handoff
	if !result.IsPostHandoff && result.HandoffContent == "" {
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

	// Get handoff content
	content, _ := GetHandoffContent(cfg)
	if content != "" {
		fmt.Println("## Handoff Notes")
		fmt.Println(content)
		// Archive after displaying
		ClearHandoffContent(cfg)
	}

	return nil
}
