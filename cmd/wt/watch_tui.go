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

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	cardTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	cardLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	cardValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
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
	cfg         *config.Config
	sessions    []sessionItem
	cursor      int
	width       int
	height      int
	lastRefresh time.Time
	quitting    bool
}

// Messages
type tickMsg time.Time
type sessionsMsg []sessionItem
type switchedMsg struct{ err error }

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

func switchSessionCmd(sessionName string) tea.Cmd {
	return func() tea.Msg {
		// Use tmux switch-client to switch to the selected session
		// This keeps the watch TUI running in its pane
		err := tmux.SwitchClient(sessionName)
		return switchedMsg{err: err}
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
				// Switch to the selected session without quitting
				// The watch continues running in its pane
				sessionName := m.sessions[m.cursor].name
				return m, switchSessionCmd(sessionName)
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

	case switchedMsg:
		// Session switch completed, continue running
		// (ignore any error - user will see it when they can't switch)
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
		// Sessions list
		for i, sess := range m.sessions {
			// Status style
			var statusStr string
			switch sess.status {
			case "working":
				statusStr = statusWorkingStyle.Render("●")
			case "idle":
				statusStr = statusIdleStyle.Render("●")
			case "ready":
				statusStr = statusReadyStyle.Render("●")
			case "blocked":
				statusStr = statusBlockedStyle.Render("●")
			case "error":
				statusStr = statusErrorStyle.Render("●")
			default:
				statusStr = normalStyle.Render("●")
			}

			// Format line - compact for narrow panes
			line := fmt.Sprintf("%s %-14s %s",
				statusStr,
				truncateStr(sess.name, 14),
				truncateStr(sess.bead, 12))

			// Apply selection style
			if i == m.cursor {
				s += selectedStyle.Render("> "+truncateStr(sess.name, 14)+" "+truncateStr(sess.bead, 12)) + "\n"
			} else {
				s += "  " + line + "\n"
			}
		}

		// Detail card for selected session
		if m.cursor < len(m.sessions) {
			sess := m.sessions[m.cursor]
			s += "\n"

			// Build card content
			var cardContent string
			cardContent += cardTitleStyle.Render(sess.name) + "\n"
			cardContent += cardLabelStyle.Render("Bead:    ") + cardValueStyle.Render(sess.bead) + "\n"
			cardContent += cardLabelStyle.Render("Project: ") + cardValueStyle.Render(sess.project) + "\n"
			cardContent += cardLabelStyle.Render("Status:  ") + m.renderStatus(sess.status) + "\n"
			if sess.message != "" {
				cardContent += cardLabelStyle.Render("Message: ") + cardValueStyle.Render(sess.message) + "\n"
			}
			if sess.idle > 0 {
				idleStr := fmt.Sprintf("%dm", sess.idle)
				if sess.idle >= 60 {
					idleStr = fmt.Sprintf("%dh %dm", sess.idle/60, sess.idle%60)
				}
				cardContent += cardLabelStyle.Render("Idle:    ") + cardValueStyle.Render(idleStr) + "\n"
			}

			s += cardStyle.Render(cardContent)
		}
	}

	// Help - vertical
	s += "\n\n"
	s += helpStyle.Render("↑/↓  navigate") + "\n"
	s += helpStyle.Render("enter  switch to session") + "\n"
	s += helpStyle.Render("r  refresh") + "\n"
	s += helpStyle.Render("q  quit")

	return s
}

// renderStatus returns a styled status string
func (m watchModel) renderStatus(status string) string {
	switch status {
	case "working":
		return statusWorkingStyle.Render(status)
	case "idle":
		return statusIdleStyle.Render(status)
	case "ready":
		return statusReadyStyle.Render(status)
	case "blocked":
		return statusBlockedStyle.Render(status)
	case "error":
		return statusErrorStyle.Render(status)
	default:
		return normalStyle.Render(status)
	}
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
func runWatchTUI(cfg *config.Config) error {
	m := newWatchModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err := p.Run()
	return err
}

// cmdWatchTUI runs the new watch TUI
func cmdWatchTUI(cfg *config.Config) error {
	return runWatchTUI(cfg)
}
