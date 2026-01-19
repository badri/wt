package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/badri/wt/internal/config"
)

// EventType represents the type of event
type EventType string

const (
	EventSessionStart EventType = "session_start"
	EventSessionEnd   EventType = "session_end"
	EventSessionKill  EventType = "session_kill"
	EventPRCreated    EventType = "pr_created"
	EventPRMerged     EventType = "pr_merged"
)

// Event represents a logged event
type Event struct {
	Time           string    `json:"time"`
	Type           EventType `json:"type"`
	Session        string    `json:"session"`
	Bead           string    `json:"bead"`
	Project        string    `json:"project"`
	ClaudeSession  string    `json:"claude_session,omitempty"`
	PRURL          string    `json:"pr_url,omitempty"`
	MergeMode      string    `json:"merge_mode,omitempty"`
	WorktreePath   string    `json:"worktree,omitempty"`
}

// Logger handles event logging
type Logger struct {
	eventsFile string
}

// NewLogger creates a new event logger
func NewLogger(cfg *config.Config) *Logger {
	return &Logger{
		eventsFile: filepath.Join(cfg.ConfigDir(), "events.jsonl"),
	}
}

// Log writes an event to the events file
func (l *Logger) Log(event *Event) error {
	if event.Time == "" {
		event.Time = time.Now().Format(time.RFC3339)
	}

	f, err := os.OpenFile(l.eventsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening events file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing event: %w", err)
	}

	return nil
}

// LogSessionStart logs a session start event
func (l *Logger) LogSessionStart(session, bead, project, worktree string) error {
	return l.Log(&Event{
		Type:         EventSessionStart,
		Session:      session,
		Bead:         bead,
		Project:      project,
		WorktreePath: worktree,
	})
}

// LogSessionEnd logs a session end event
func (l *Logger) LogSessionEnd(session, bead, project, claudeSession, mergeMode, prURL string) error {
	return l.Log(&Event{
		Type:          EventSessionEnd,
		Session:       session,
		Bead:          bead,
		Project:       project,
		ClaudeSession: claudeSession,
		MergeMode:     mergeMode,
		PRURL:         prURL,
	})
}

// LogSessionKill logs a session kill event
func (l *Logger) LogSessionKill(session, bead, project string) error {
	return l.Log(&Event{
		Type:    EventSessionKill,
		Session: session,
		Bead:    bead,
		Project: project,
	})
}

// Recent returns the most recent N events
func (l *Logger) Recent(n int) ([]Event, error) {
	data, err := os.ReadFile(l.eventsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var allEvents []Event
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue // Skip invalid lines
		}
		allEvents = append(allEvents, event)
	}

	// Return last N events
	if len(allEvents) <= n {
		return allEvents, nil
	}
	return allEvents[len(allEvents)-n:], nil
}

// RecentSessions returns recent session_end events (sessions that can be resumed)
func (l *Logger) RecentSessions(n int) ([]Event, error) {
	events, err := l.Recent(1000) // Read more to filter
	if err != nil {
		return nil, err
	}

	var sessions []Event
	for i := len(events) - 1; i >= 0 && len(sessions) < n; i-- {
		if events[i].Type == EventSessionEnd && events[i].ClaudeSession != "" {
			sessions = append(sessions, events[i])
		}
	}

	return sessions, nil
}

// FindSession finds a session by name in recent events
func (l *Logger) FindSession(name string) (*Event, error) {
	events, err := l.Recent(1000)
	if err != nil {
		return nil, err
	}

	// Search from most recent
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Session == name && events[i].Type == EventSessionEnd {
			return &events[i], nil
		}
	}

	return nil, fmt.Errorf("session '%s' not found in history", name)
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
