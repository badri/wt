package session

import "time"

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
}

func (s *Session) UpdateActivity() {
	s.LastActivity = Now()
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
