package session

import "time"

// SessionType identifies whether a session is bead-based or a lightweight task
type SessionType string

const (
	SessionTypeBead SessionType = "bead" // Default: bead-driven session
	SessionTypeTask SessionType = "task" // Lightweight task session
)

// CompletionCondition defines how a task session is completed
type CompletionCondition string

const (
	ConditionNone        CompletionCondition = "none"         // Just confirm done
	ConditionPRMerged    CompletionCondition = "pr-merged"    // PR must be merged
	ConditionPushed      CompletionCondition = "pushed"       // Changes pushed to remote
	ConditionTestsPass   CompletionCondition = "tests-pass"   // Tests must pass
	ConditionUserConfirm CompletionCondition = "user-confirm" // User explicitly confirms
)

type Session struct {
	Bead          string `json:"bead"`
	Project       string `json:"project"`
	Worktree      string `json:"worktree"`
	Branch        string `json:"branch"`
	PortOffset    int    `json:"port_offset,omitempty"`
	BeadsDir      string `json:"beads_dir"`
	CreatedAt     string `json:"created_at"`
	LastActivity  string `json:"last_activity"`
	Status        string `json:"status"`                   // working, idle, ready, blocked, error
	StatusMessage string `json:"status_message,omitempty"` // Optional message (e.g., PR URL, error details)

	// Task session fields
	Type                SessionType         `json:"type,omitempty"`                 // "bead" or "task"
	TaskDescription     string              `json:"task_description,omitempty"`     // Description for task sessions
	CompletionCondition CompletionCondition `json:"completion_condition,omitempty"` // How task is considered complete
}

// IsBead returns true if this is a bead-based session
func (s *Session) IsBead() bool {
	return s.Type == "" || s.Type == SessionTypeBead
}

// IsTask returns true if this is a lightweight task session
func (s *Session) IsTask() bool {
	return s.Type == SessionTypeTask
}

func (s *Session) UpdateActivity() {
	s.LastActivity = Now()
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
