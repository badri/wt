package auto

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/badri/wt/internal/agentmail"
	"github.com/badri/wt/internal/config"
)

const (
	orchestratorAgent = "orchestrator"
	agentMailProgram  = "wt-auto"

	// Message subject prefixes for structured communication
	SubjectTaskAssign = "TASK"
	SubjectDone       = "DONE"
	SubjectStuck      = "STUCK"
	SubjectProgress   = "PROGRESS"
)

// AgentMailOrchestrator wraps the agentmail client with orchestration logic.
type AgentMailOrchestrator struct {
	client  *agentmail.Client
	enabled bool
}

// NewAgentMailOrchestrator creates an orchestrator if Agent Mail is available.
// Returns a disabled orchestrator (no-ops) if the server is unreachable.
func NewAgentMailOrchestrator(projectKey string) *AgentMailOrchestrator {
	client := agentmail.NewClient("", projectKey)
	amo := &AgentMailOrchestrator{
		client:  client,
		enabled: client.IsAvailable(),
	}
	return amo
}

// IsEnabled reports whether Agent Mail is active.
func (o *AgentMailOrchestrator) IsEnabled() bool {
	return o.enabled
}

// RegisterOrchestrator registers the orchestrator agent identity.
func (o *AgentMailOrchestrator) RegisterOrchestrator() error {
	if !o.enabled {
		return nil
	}
	return o.client.RegisterAgent(orchestratorAgent, agentMailProgram, "")
}

// RegisterWorker registers a worker agent identity.
func (o *AgentMailOrchestrator) RegisterWorker(workerName string) error {
	if !o.enabled {
		return nil
	}
	return o.client.RegisterAgent(workerName, "claude-code", "claude-opus-4-5-20251101")
}

// SendTask sends a task assignment to a worker.
func (o *AgentMailOrchestrator) SendTask(workerName, beadID, prompt string) (string, error) {
	if !o.enabled {
		return "", nil
	}
	subject := fmt.Sprintf("%s: %s", SubjectTaskAssign, beadID)
	return o.client.SendMessage(orchestratorAgent, []string{workerName}, subject, prompt, true)
}

// PollForCompletion checks the orchestrator inbox for DONE or STUCK messages.
// Returns (beadID, status, message, error). Status is "done", "stuck", or "" if no messages.
func (o *AgentMailOrchestrator) PollForCompletion() (beadID, status, body string, err error) {
	if !o.enabled {
		return "", "", "", nil
	}

	messages, err := o.client.FetchInbox(orchestratorAgent, 10)
	if err != nil {
		return "", "", "", fmt.Errorf("fetch inbox: %w", err)
	}

	for _, msg := range messages {
		if msg.Acknowledged {
			continue
		}

		var msgStatus string
		var msgBeadID string

		if strings.HasPrefix(msg.Subject, SubjectDone+": ") {
			msgStatus = "done"
			msgBeadID = strings.TrimPrefix(msg.Subject, SubjectDone+": ")
		} else if strings.HasPrefix(msg.Subject, SubjectStuck+": ") {
			msgStatus = "stuck"
			msgBeadID = strings.TrimPrefix(msg.Subject, SubjectStuck+": ")
		} else {
			continue
		}

		// Acknowledge the message
		if ackErr := o.client.AcknowledgeMessage(orchestratorAgent, msg.ID); ackErr != nil {
			fmt.Printf("Warning: could not ack message %s: %v\n", msg.ID, ackErr)
		}

		return msgBeadID, msgStatus, msg.Body, nil
	}

	return "", "", "", nil
}

// ReserveWorkerFiles creates advisory file locks for a worker's edit surface.
func (o *AgentMailOrchestrator) ReserveWorkerFiles(workerName string, paths []string) error {
	if !o.enabled {
		return nil
	}
	return o.client.ReserveFiles(workerName, paths, 3600, true)
}

// ReleaseWorkerFiles releases a worker's file locks.
func (o *AgentMailOrchestrator) ReleaseWorkerFiles(workerName string) error {
	if !o.enabled {
		return nil
	}
	return o.client.ReleaseFiles(workerName)
}

// NotifyEpicComplete sends an epic completion notification.
func (o *AgentMailOrchestrator) NotifyEpicComplete(epicID string, completedBeads int) error {
	if !o.enabled {
		return nil
	}
	subject := fmt.Sprintf("EPIC_DONE: %s", epicID)
	body := fmt.Sprintf("Epic %s completed with %d beads.", epicID, completedBeads)
	_, err := o.client.SendMessage(orchestratorAgent, []string{orchestratorAgent}, subject, body, false)
	return err
}

// ReconcileState checks Agent Mail for any completion messages that were
// missed during a crash/restart. Returns bead IDs that completed while
// the orchestrator was down.
func (o *AgentMailOrchestrator) ReconcileState(state *EpicState) ([]string, error) {
	if !o.enabled {
		return nil, nil
	}

	messages, err := o.client.FetchInbox(orchestratorAgent, 100)
	if err != nil {
		return nil, fmt.Errorf("fetch inbox for reconciliation: %w", err)
	}

	var missedCompletions []string
	for _, msg := range messages {
		if msg.Acknowledged {
			continue
		}
		if !strings.HasPrefix(msg.Subject, SubjectDone+": ") {
			continue
		}
		beadID := strings.TrimPrefix(msg.Subject, SubjectDone+": ")

		// Check if this bead is in our epic and not yet marked complete
		found := false
		for _, b := range state.Beads {
			if b == beadID {
				found = true
				break
			}
		}
		alreadyComplete := false
		for _, b := range state.CompletedBeads {
			if b == beadID {
				alreadyComplete = true
				break
			}
		}

		if found && !alreadyComplete {
			missedCompletions = append(missedCompletions, beadID)
			// Acknowledge it
			o.client.AcknowledgeMessage(orchestratorAgent, msg.ID)
		}
	}

	return missedCompletions, nil
}

// BuildWorkerInstructions returns CLAUDE.md content that tells a worker
// how to communicate via Agent Mail.
func BuildWorkerInstructions(workerName, beadID, orchestratorURL string) string {
	if orchestratorURL == "" {
		orchestratorURL = agentmail.DefaultBaseURL
	}
	return fmt.Sprintf(`## Agent Mail Communication

You are worker agent "%s". Use MCP Agent Mail to communicate with the orchestrator.

### When you finish the task successfully:
Send a message to the orchestrator:
- To: orchestrator
- Subject: DONE: %s
- Body: Brief summary of what you did

### If you get stuck or need help:
Send a message to the orchestrator:
- To: orchestrator
- Subject: STUCK: %s
- Body: Description of what's blocking you

### Signal completion via wt:
Also run: wt signal bead-done "<summary of what you did>"

Agent Mail server: %s
`, workerName, beadID, beadID, orchestratorURL)
}

// SetupAgentMail initializes Agent Mail for an auto run if available.
// Called at the start of processEpic. Returns the orchestrator (may be disabled).
func SetupAgentMail(cfg *config.Config, epicID string) *AgentMailOrchestrator {
	// Use project directory basename as project key
	projectKey := filepath.Base(cfg.ConfigDir())
	if projectKey == "" || projectKey == "." {
		projectKey = "wt"
	}

	amo := NewAgentMailOrchestrator(projectKey)
	if !amo.IsEnabled() {
		fmt.Println("Agent Mail: not available (server not running)")
		return amo
	}

	fmt.Println("Agent Mail: connected")
	if err := amo.RegisterOrchestrator(); err != nil {
		fmt.Printf("Agent Mail: failed to register orchestrator: %v\n", err)
		amo.enabled = false
		return amo
	}

	fmt.Println("Agent Mail: orchestrator registered")
	return amo
}

// WaitForAgentMailCompletion polls Agent Mail for a bead completion message.
// Falls back immediately if Agent Mail is disabled.
// Returns (status, summary, error). Status is "done", "stuck", or "timeout".
func (o *AgentMailOrchestrator) WaitForAgentMailCompletion(beadID string, timeout time.Duration) (string, string, error) {
	if !o.enabled {
		return "", "", fmt.Errorf("agent mail not enabled")
	}

	deadline := time.Now().Add(timeout)
	pollInterval := 10 * time.Second

	for time.Now().Before(deadline) {
		completedBead, status, body, err := o.PollForCompletion()
		if err != nil {
			fmt.Printf("Agent Mail poll error: %v\n", err)
			time.Sleep(pollInterval)
			continue
		}

		if completedBead == beadID {
			return status, body, nil
		}

		time.Sleep(pollInterval)
	}

	return "timeout", "", fmt.Errorf("timed out waiting for bead %s", beadID)
}
