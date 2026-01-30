package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/badri/wt/internal/tmux"
)

// NudgeEntry is a single nudge log record.
type NudgeEntry struct {
	Session string    `json:"session"`
	Type    string    `json:"type"` // "interrupted" or "idle"
	Time    time.Time `json:"time"`
}

// Nudger manages auto-nudge state including cooldown tracking and logging.
type Nudger struct {
	mu        sync.Mutex
	lastNudge map[string]time.Time // session -> last nudge time
	logPath   string
	cooldown  time.Duration
}

// NewNudger creates a Nudger that logs to configDir/nudge.log.
func NewNudger(configDir string) *Nudger {
	return &Nudger{
		lastNudge: make(map[string]time.Time),
		logPath:   filepath.Join(configDir, "nudge.log"),
		cooldown:  2 * time.Minute,
	}
}

// TryNudge nudges a stuck session if the cooldown has elapsed.
// Returns true if a nudge was sent.
func (n *Nudger) TryNudge(sessionName string, state StuckState) bool {
	if state.Type == "none" {
		return false
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if last, ok := n.lastNudge[sessionName]; ok {
		if time.Since(last) < n.cooldown {
			return false
		}
	}

	var err error
	switch state.Type {
	case "interrupted":
		err = nudgeInterrupted(sessionName)
	case "idle":
		err = nudgeIdle(sessionName)
	}

	if err != nil {
		return false
	}

	n.lastNudge[sessionName] = time.Now()
	n.log(NudgeEntry{Session: sessionName, Type: state.Type, Time: time.Now()})
	return true
}

// LastNudgeTime returns when a session was last nudged (zero if never).
func (n *Nudger) LastNudgeTime(sessionName string) time.Time {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastNudge[sessionName]
}

func nudgeInterrupted(sessionName string) error {
	// Send Enter to resume after an interruption
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, "Enter")
	return cmd.Run()
}

func nudgeIdle(sessionName string) error {
	return tmux.NudgeSession(sessionName, "please continue working on the current task")
}

func (n *Nudger) log(entry NudgeEntry) {
	f, err := os.OpenFile(n.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s\n", data)
}
