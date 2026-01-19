package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/badri/wt/internal/config"
)

// EventType represents the type of event
type EventType string

const (
	EventSessionStart EventType = "session_start"
	EventSessionEnd   EventType = "session_end"
	EventSessionKill  EventType = "session_kill"
	EventHubHandoff   EventType = "hub_handoff"
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

// LogHubHandoff logs a hub handoff event (hub session can be resumed via seance)
func (l *Logger) LogHubHandoff(claudeSession, message string) error {
	return l.Log(&Event{
		Type:          EventHubHandoff,
		Session:       "hub",
		Bead:          "",
		Project:       "",
		ClaudeSession: claudeSession,
		MergeMode:     message, // Reuse field for handoff message
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

// RecentSessions returns recent session_end and hub_handoff events (sessions that can be resumed)
func (l *Logger) RecentSessions(n int) ([]Event, error) {
	events, err := l.Recent(1000) // Read more to filter
	if err != nil {
		return nil, err
	}

	var sessions []Event
	for i := len(events) - 1; i >= 0 && len(sessions) < n; i-- {
		e := events[i]
		// Include session_end with Claude session, or hub_handoff events
		if (e.Type == EventSessionEnd || e.Type == EventHubHandoff) && e.ClaudeSession != "" {
			sessions = append(sessions, e)
		}
	}

	return sessions, nil
}

// FindSession finds a session by name, bead ID, or project in recent events.
// Matching priority: exact session name > bead ID > project prefix
func (l *Logger) FindSession(query string) (*Event, error) {
	events, err := l.Recent(1000)
	if err != nil {
		return nil, err
	}

	// Filter to session_end or hub_handoff events with Claude session (can be resumed)
	var sessions []Event
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if (e.Type == EventSessionEnd || e.Type == EventHubHandoff) && e.ClaudeSession != "" {
			sessions = append(sessions, e)
		}
	}

	// 1. Exact session name match (including "hub")
	for _, e := range sessions {
		if e.Session == query {
			return &e, nil
		}
	}

	// 2. Bead ID match (exact or prefix)
	for _, e := range sessions {
		if e.Bead != "" && (e.Bead == query || strings.HasPrefix(e.Bead, query)) {
			return &e, nil
		}
	}

	// 3. Project match
	for _, e := range sessions {
		if e.Project != "" && e.Project == query {
			return &e, nil
		}
	}

	return nil, fmt.Errorf("no session found matching '%s' (tried: session name, bead ID, project)", query)
}

// Since returns events from the last duration
func (l *Logger) Since(d time.Duration) ([]Event, error) {
	cutoff := time.Now().Add(-d)
	return l.SinceTime(cutoff)
}

// SinceTime returns events since the given time
func (l *Logger) SinceTime(cutoff time.Time) ([]Event, error) {
	allEvents, err := l.All()
	if err != nil {
		return nil, err
	}

	var filtered []Event
	for _, e := range allEvents {
		eventTime, err := time.Parse(time.RFC3339, e.Time)
		if err != nil {
			continue
		}
		if eventTime.After(cutoff) {
			filtered = append(filtered, e)
		}
	}

	return filtered, nil
}

// All returns all events
func (l *Logger) All() ([]Event, error) {
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
			continue
		}
		allEvents = append(allEvents, event)
	}

	return allEvents, nil
}

// NewSinceLastRead returns events since the last read marker and optionally clears
func (l *Logger) NewSinceLastRead(clear bool) ([]Event, error) {
	markerFile := l.eventsFile + ".lastread"

	// Get last read time
	var lastRead time.Time
	if data, err := os.ReadFile(markerFile); err == nil {
		lastRead, _ = time.Parse(time.RFC3339, string(data))
	}

	// Get events since last read
	var events []Event
	var err error
	if lastRead.IsZero() {
		// No marker, return recent events (last 10)
		events, err = l.Recent(10)
	} else {
		events, err = l.SinceTime(lastRead)
	}
	if err != nil {
		return nil, err
	}

	// Update marker if clearing
	if clear {
		now := time.Now().Format(time.RFC3339)
		if err := os.WriteFile(markerFile, []byte(now), 0644); err != nil {
			return events, fmt.Errorf("updating read marker: %w", err)
		}
	}

	return events, nil
}

// MarkAsRead updates the last read marker to now
func (l *Logger) MarkAsRead() error {
	markerFile := l.eventsFile + ".lastread"
	now := time.Now().Format(time.RFC3339)
	return os.WriteFile(markerFile, []byte(now), 0644)
}

// Tail watches for new events and sends them to the channel
func (l *Logger) Tail(ctx context.Context, events chan<- Event) error {
	// Get current file size
	var lastSize int64
	if info, err := os.Stat(l.eventsFile); err == nil {
		lastSize = info.Size()
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			info, err := os.Stat(l.eventsFile)
			if err != nil {
				continue
			}

			if info.Size() > lastSize {
				// Read new content
				f, err := os.Open(l.eventsFile)
				if err != nil {
					continue
				}

				f.Seek(lastSize, 0)
				newData := make([]byte, info.Size()-lastSize)
				f.Read(newData)
				f.Close()

				for _, line := range splitLines(newData) {
					if len(line) == 0 {
						continue
					}
					var event Event
					if err := json.Unmarshal(line, &event); err != nil {
						continue
					}
					select {
					case events <- event:
					case <-ctx.Done():
						return nil
					}
				}

				lastSize = info.Size()
			}
		}
	}
}

// EventsFile returns the path to the events file
func (l *Logger) EventsFile() string {
	return l.eventsFile
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
