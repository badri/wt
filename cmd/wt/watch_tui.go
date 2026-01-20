package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/monitor"
	"github.com/badri/wt/internal/session"
	"github.com/badri/wt/internal/tmux"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	statusWorkingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	statusIdleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("226"))

	statusReadyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("46"))

	statusBlockedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	statusErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Bold(true)
)

// Key bindings
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Enter  key.Binding
	Quit   key.Binding
	Refresh key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "switch"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
}

// Session item for display
type sessionItem struct {
	name    string
	bead    string
	project string
	status  string
	message string
	idle    int
}

// Model
type watchModel struct {
	cfg           *config.Config
	sessions      []sessionItem
	cursor        int
	width         int
	height        int
	lastRefresh   time.Time
	switchTo      string // Session to switch to on quit
	quitting      bool
}

// Messages
type tickMsg time.Time
type sessionsMsg []sessionItem

// Commands
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func loadSessionsCmd(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		state, err := session.LoadState(cfg)
		if err != nil {
			return sessionsMsg{}
		}

		var items []sessionItem
		for name, sess := range state.Sessions {
			status := sess.Status
			if status == "" {
				status = monitor.DetectStatus(name, 5)
			}
			idle := monitor.GetIdleMinutes(name)

			items = append(items, sessionItem{
				name:    name,
				bead:    sess.Bead,
				project: sess.Project,
				status:  status,
				message: sess.StatusMessage,
				idle:    idle,
			})
		}

		// Sort by name
		sort.Slice(items, func(i, j int) bool {
			return items[i].name < items[j].name
		})

		return sessionsMsg(items)
	}
}

// Initialize model
func newWatchModel(cfg *config.Config) watchModel {
	return watchModel{
		cfg:         cfg,
		sessions:    []sessionItem{},
		cursor:      0,
		lastRefresh: time.Now(),
	}
}

func (m watchModel) Init() tea.Cmd {
	return tea.Batch(loadSessionsCmd(m.cfg), tickCmd())
}

func (m watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, keys.Down):
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}

		case key.Matches(msg, keys.Enter):
			if len(m.sessions) > 0 && m.cursor < len(m.sessions) {
				m.switchTo = m.sessions[m.cursor].name
				m.quitting = true
				return m, tea.Quit
			}

		case key.Matches(msg, keys.Refresh):
			return m, loadSessionsCmd(m.cfg)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		m.lastRefresh = time.Time(msg)
		return m, tea.Batch(loadSessionsCmd(m.cfg), tickCmd())

	case sessionsMsg:
		m.sessions = msg
		// Adjust cursor if needed
		if m.cursor >= len(m.sessions) && len(m.sessions) > 0 {
			m.cursor = len(m.sessions) - 1
		}
	}

	return m, nil
}

func (m watchModel) View() string {
	if m.quitting {
		return ""
	}

	var s string

	// Title
	s += titleStyle.Render("wt watch") + " "
	s += helpStyle.Render(m.lastRefresh.Format("15:04:05")) + "\n\n"

	if len(m.sessions) == 0 {
		s += normalStyle.Render("No active sessions.\n")
		s += helpStyle.Render("\nStart one with: wt new <bead>")
	} else {
		// Header
		s += headerStyle.Render(fmt.Sprintf("  %-12s %-10s %-8s %s", "Session", "Bead", "Status", "Info")) + "\n"

		// Sessions
		for i, sess := range m.sessions {
			// Status icon and style
			var statusStr string
			switch sess.status {
			case "working":
				statusStr = statusWorkingStyle.Render("working")
			case "idle":
				statusStr = statusIdleStyle.Render("idle")
			case "ready":
				statusStr = statusReadyStyle.Render("ready")
			case "blocked":
				statusStr = statusBlockedStyle.Render("blocked")
			case "error":
				statusStr = statusErrorStyle.Render("error")
			default:
				statusStr = normalStyle.Render(sess.status)
			}

			// Info column (message or idle time)
			info := ""
			if sess.message != "" {
				info = truncateStr(sess.message, 20)
			} else if sess.idle > 0 {
				if sess.idle >= 60 {
					info = fmt.Sprintf("%dh%dm", sess.idle/60, sess.idle%60)
				} else {
					info = fmt.Sprintf("%dm", sess.idle)
				}
			}

			// Format line
			line := fmt.Sprintf("%-12s %-10s %-8s %s",
				truncateStr(sess.name, 12),
				truncateStr(sess.bead, 10),
				sess.status,
				info)

			// Apply selection style
			if i == m.cursor {
				s += selectedStyle.Render("> "+line) + "\n"
			} else {
				// Re-render with status color
				line = fmt.Sprintf("  %-12s %-10s %s %s",
					truncateStr(sess.name, 12),
					truncateStr(sess.bead, 10),
					statusStr,
					info)
				s += line + "\n"
			}
		}
	}

	// Help
	s += "\n" + helpStyle.Render("↑/↓ navigate • enter switch • r refresh • q quit")

	return s
}

// Helper to truncate strings
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-2] + ".."
}

// Run the watch TUI
func runWatchTUI(cfg *config.Config) (string, error) {
	m := newWatchModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	// Return the session to switch to (if any)
	if fm, ok := finalModel.(watchModel); ok {
		return fm.switchTo, nil
	}

	return "", nil
}

// cmdWatchTUI runs the new watch TUI
func cmdWatchTUI(cfg *config.Config) error {
	switchTo, err := runWatchTUI(cfg)
	if err != nil {
		return err
	}

	// If user selected a session, switch to it
	if switchTo != "" {
		return tmux.Attach(switchTo)
	}

	return nil
}
