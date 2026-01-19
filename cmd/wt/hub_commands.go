package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/events"
	"github.com/badri/wt/internal/handoff"
	"github.com/badri/wt/internal/hub"
	"github.com/badri/wt/internal/monitor"
	"github.com/badri/wt/internal/session"
)

// cmdHub creates or attaches to the dedicated hub session
func cmdHub(cfg *config.Config, args []string) error {
	opts := parseHubFlags(args)
	return hub.Run(cfg, opts)
}

func parseHubFlags(args []string) *hub.Options {
	opts := &hub.Options{}
	for _, arg := range args {
		switch arg {
		case "-d", "--detach":
			opts.Detach = true
		case "-s", "--status":
			opts.Status = true
		case "-k", "--kill":
			opts.Kill = true
		case "-f", "--force":
			opts.Force = true
		}
	}
	return opts
}

// cmdWatch displays a live dashboard of all sessions
func cmdWatch(cfg *config.Config) error {
	const refreshInterval = 2 * time.Second
	const idleThreshold = 5 // minutes

	// Track previous states for notifications
	prevStates := make(map[string]string)
	prevPRStates := make(map[string]string)
	prevSessions := make(map[string]bool)

	for {
		// Clear screen
		fmt.Print("\033[H\033[2J")

		state, err := session.LoadState(cfg)
		if err != nil {
			return err
		}

		// Check for ended sessions (were in prev, not in current)
		currentSessions := make(map[string]bool)
		for name := range state.Sessions {
			currentSessions[name] = true
		}
		for name := range prevSessions {
			if !currentSessions[name] {
				monitor.Notify("wt: Session Ended", fmt.Sprintf("Session '%s' has completed", name))
			}
		}

		now := time.Now().Format("15:04:05")
		fmt.Printf("┌─ wt watch ─────────────────────────────────────────────── %s ─┐\n", now)
		fmt.Println("│                                                                       │")

		if len(state.Sessions) == 0 {
			fmt.Println("│  No active sessions.                                                 │")
			fmt.Println("│                                                                       │")
			fmt.Println("│  Start one with: wt new <bead>                                       │")
		} else {
			fmt.Printf("│  %-3s %-10s %-18s %-8s %-6s %-10s %-6s │\n",
				"", "Name", "Bead", "Status", "Idle", "PR", "Project")
			fmt.Printf("│  %-3s %-10s %-18s %-8s %-6s %-10s %-6s │\n",
				"", "────", "────", "──────", "────", "──", "───────")

			for name, sess := range state.Sessions {
				// Use session status if set, otherwise detect from tmux
				status := sess.Status
				if status == "" {
					status = monitor.DetectStatus(name, idleThreshold)
				}
				idleMin := monitor.GetIdleMinutes(name)

				// Get PR status
				prStatus, _ := monitor.GetPRStatus(sess.Worktree, sess.Branch)

				// Format idle time
				idleStr := "-"
				if idleMin >= 0 {
					if idleMin < 60 {
						idleStr = fmt.Sprintf("%dm", idleMin)
					} else {
						idleStr = fmt.Sprintf("%dh%dm", idleMin/60, idleMin%60)
					}
				}

				// Format PR/message status - prefer status message if set
				prStr := "-"
				if sess.StatusMessage != "" {
					prStr = sess.StatusMessage
				} else if prStatus != "none" && prStatus != "" {
					prStr = prStatus
				}

				statusIcon := getStatusIcon(status)
				prIcon := monitor.PRStatusIcon(prStatus)
				if sess.StatusMessage != "" {
					prIcon = "" // Don't show PR icon when we have a message
				}

				fmt.Printf("│  %s %-10s %-18s %-8s %-6s %s %-8s %-6s │\n",
					statusIcon,
					truncate(name, 10),
					truncate(sess.Bead, 18),
					status,
					idleStr,
					prIcon,
					truncate(prStr, 8),
					truncate(sess.Project, 6))

				// Send notification on status change
				prevStatus, exists := prevStates[name]
				if exists && prevStatus != status {
					if status == "ready" {
						msg := fmt.Sprintf("Session '%s' is ready for review", name)
						if sess.StatusMessage != "" {
							msg = fmt.Sprintf("Session '%s': %s", name, sess.StatusMessage)
						}
						monitor.Notify("wt: Ready for Review", msg)
					} else if status == "idle" {
						monitor.Notify("wt: Session Idle", fmt.Sprintf("Session '%s' is now idle", name))
					} else if status == "error" {
						monitor.Notify("wt: Session Error", fmt.Sprintf("Session '%s' has an error", name))
					} else if status == "blocked" {
						msg := fmt.Sprintf("Session '%s' is blocked", name)
						if sess.StatusMessage != "" {
							msg = fmt.Sprintf("Session '%s': %s", name, sess.StatusMessage)
						}
						monitor.Notify("wt: Session Blocked", msg)
					}
				}
				prevStates[name] = status

				// Send notification on PR merge
				prevPR := prevPRStates[name]
				if prevPR != "" && prevPR != "merged" && prStatus == "merged" {
					monitor.Notify("wt: PR Merged", fmt.Sprintf("PR for '%s' has been merged", name))
				}
				prevPRStates[name] = prStatus
			}
		}

		// Update previous sessions for next iteration
		prevSessions = currentSessions

		fmt.Println("│                                                                       │")
		fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
		fmt.Println("\nPress Ctrl+C to exit")

		time.Sleep(refreshInterval)
	}
}

// cmdSeance allows talking to past sessions
func cmdSeance(cfg *config.Config, args []string) error {
	eventLogger := events.NewLogger(cfg)

	// No args - list recent sessions
	if len(args) == 0 {
		return cmdSeanceList(eventLogger)
	}

	// Parse flags
	sessionName := args[0]
	var prompt string
	for i := 1; i < len(args); i++ {
		if args[i] == "-p" && i+1 < len(args) {
			prompt = args[i+1]
			break
		}
	}

	// Find the session
	event, err := eventLogger.FindSession(sessionName)
	if err != nil {
		return err
	}

	if event.ClaudeSession == "" {
		return fmt.Errorf("session '%s' has no Claude session ID recorded", sessionName)
	}

	if prompt != "" {
		// One-shot query
		return cmdSeanceQuery(event, prompt)
	}

	// Resume session
	return cmdSeanceResume(event)
}

func cmdSeanceList(logger *events.Logger) error {
	sessions, err := logger.RecentSessions(10)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println("No past sessions found.")
		fmt.Println("\nSessions are recorded when they end via 'wt done', 'wt close', or 'wt handoff'.")
		return nil
	}

	fmt.Println("┌─ Past Sessions (seance) ─────────────────────────────────────────────┐")
	fmt.Println("│                                                                       │")
	fmt.Printf("│  %-10s %-18s %-12s %-24s │\n", "Session", "Bead", "Project", "Time")
	fmt.Printf("│  %-10s %-18s %-12s %-24s │\n", "───────", "────", "───────", "────")

	for _, sess := range sessions {
		t, _ := time.Parse(time.RFC3339, sess.Time)
		timeStr := t.Format("2006-01-02 15:04")

		// Determine icon based on session type
		icon := "  "
		if sess.ClaudeSession != "" {
			if sess.Type == events.EventHubHandoff {
				icon = "🏠"
			} else {
				icon = "💬"
			}
		}

		// For hub sessions, show special values
		beadDisplay := sess.Bead
		projectDisplay := sess.Project
		if sess.Type == events.EventHubHandoff {
			beadDisplay = "(hub)"
			projectDisplay = "(orchestrator)"
		}

		fmt.Printf("│  %s %-8s %-18s %-12s %-24s │\n",
			icon,
			truncate(sess.Session, 8),
			truncate(beadDisplay, 18),
			truncate(projectDisplay, 12),
			timeStr)
	}

	fmt.Println("│                                                                       │")
	fmt.Println("└───────────────────────────────────────────────────────────────────────┘")
	fmt.Println("\n💬 = Worker session   🏠 = Hub session")
	fmt.Println("\nCommands:")
	fmt.Println("  wt seance <name>           Resume conversation")
	fmt.Println("  wt seance <name> -p 'msg'  One-shot query")
	fmt.Println("  wt seance hub              Resume last hub session")

	return nil
}

func cmdSeanceResume(event *events.Event) error {
	if event.Type == events.EventHubHandoff {
		fmt.Printf("Resuming hub session...\n")
	} else {
		fmt.Printf("Resuming Claude session for '%s' (bead: %s)...\n", event.Session, event.Bead)
	}

	// Run claude with --resume flag
	cmd := exec.Command("claude", "--resume", event.ClaudeSession)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func cmdSeanceQuery(event *events.Event, prompt string) error {
	fmt.Printf("Querying Claude session for '%s'...\n", event.Session)

	// Run claude with --resume and --print for one-shot
	cmd := exec.Command("claude", "--resume", event.ClaudeSession, "--print", prompt)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// cmdHandoff performs a session handoff to a fresh Claude instance
func cmdHandoff(cfg *config.Config, args []string) error {
	opts := parseHandoffFlags(args)

	// Check if we're in tmux
	if !handoff.IsInTmux() {
		return fmt.Errorf("handoff requires running inside a tmux session")
	}

	fmt.Println("Performing handoff to fresh Claude instance...")

	result, err := handoff.Run(cfg, opts)
	if err != nil {
		return err
	}

	if opts.DryRun {
		return nil
	}

	if result.BeadUpdated {
		fmt.Println("  ✓ Handoff bead updated")
	}
	if result.MarkerWritten {
		fmt.Println("  ✓ Handoff marker written")
	}

	fmt.Println("\nRespawning Claude...")
	return nil
}

func parseHandoffFlags(args []string) *handoff.Options {
	opts := &handoff.Options{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-m", "--message":
			if i+1 < len(args) {
				opts.Message = args[i+1]
				i++
			}
		case "-c", "--collect":
			opts.AutoCollect = true
		case "--dry-run":
			opts.DryRun = true
		}
	}
	return opts
}

// cmdPrime injects context on session startup
func cmdPrime(cfg *config.Config, args []string) error {
	opts := parsePrimeFlags(args)

	result, err := handoff.Prime(cfg, opts)
	if err != nil {
		return err
	}

	handoff.OutputPrimeResult(result)
	return nil
}

func parsePrimeFlags(args []string) *handoff.PrimeOptions {
	opts := &handoff.PrimeOptions{}
	for _, arg := range args {
		switch arg {
		case "-q", "--quiet":
			opts.Quiet = true
		case "--no-bd-prime":
			opts.NoBdPrime = true
		}
	}
	return opts
}
