package main

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
)

// Styles for projects TUI (reuse colors from watch_tui)
var (
	projTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	projSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("57"))

	projNormalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	projHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	projCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	projCardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("170"))

	projCardLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	projCardValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	projMergeModeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	projSessionCountStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("226"))
)

// Key bindings for projects TUI
type projKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Quit  key.Binding
}

var projKeys = projKeyMap{
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
		key.WithHelp("enter", "view config"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c", "esc"),
		key.WithHelp("q/esc", "quit"),
	),
}

// Project item for display
type projectItem struct {
	name         string
	repo         string
	mergeMode    string
	sessionCount int
}

// Model
type projectsModel struct {
	cfg      *config.Config
	projects []projectItem
	cursor   int
	width    int
	height   int
	quitting bool
}

// Messages
type projectsLoadedMsg []projectItem

// Commands
func loadProjectsCmd(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		mgr := project.NewManager(cfg)
		projects, err := mgr.List()
		if err != nil {
			return projectsLoadedMsg{}
		}

		// Count active sessions per project
		state, _ := session.LoadState(cfg)
		sessionCount := make(map[string]int)
		for _, sess := range state.Sessions {
			sessionCount[sess.Project]++
		}

		var items []projectItem
		for _, proj := range projects {
			items = append(items, projectItem{
				name:         proj.Name,
				repo:         proj.Repo,
				mergeMode:    proj.MergeMode,
				sessionCount: sessionCount[proj.Name],
			})
		}

		// Sort by name
		sort.Slice(items, func(i, j int) bool {
			return items[i].name < items[j].name
		})

		return projectsLoadedMsg(items)
	}
}

// Initialize model
func newProjectsModel(cfg *config.Config) projectsModel {
	return projectsModel{
		cfg:      cfg,
		projects: []projectItem{},
		cursor:   0,
	}
}

func (m projectsModel) Init() tea.Cmd {
	return loadProjectsCmd(m.cfg)
}

func (m projectsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, projKeys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, projKeys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, projKeys.Down):
			if m.cursor < len(m.projects)-1 {
				m.cursor++
			}

		case key.Matches(msg, projKeys.Enter):
			// Could open config or do something with selected project
			// For now, just quit and print selected project name
			if len(m.projects) > 0 && m.cursor < len(m.projects) {
				m.quitting = true
				return m, tea.Quit
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case projectsLoadedMsg:
		m.projects = msg
		if m.cursor >= len(m.projects) && len(m.projects) > 0 {
			m.cursor = len(m.projects) - 1
		}
	}

	return m, nil
}

func (m projectsModel) View() string {
	if m.quitting {
		return ""
	}

	var s string

	// Title
	s += projTitleStyle.Render("wt projects") + "\n\n"

	if len(m.projects) == 0 {
		s += projNormalStyle.Render("No projects registered.\n")
		s += projHelpStyle.Render("\nRegister a project: wt project add <name> <path>")
	} else {
		// Projects list
		for i, proj := range m.projects {
			// Session count indicator
			countStr := "-"
			if proj.sessionCount > 0 {
				countStr = fmt.Sprintf("%d", proj.sessionCount)
			}

			// Format line
			line := fmt.Sprintf("%-16s %-12s %s",
				truncateStr(proj.name, 16),
				proj.mergeMode,
				countStr)

			// Apply selection style
			if i == m.cursor {
				s += projSelectedStyle.Render("> "+line) + "\n"
			} else {
				s += "  " + projNormalStyle.Render(line) + "\n"
			}
		}

		// Detail card for selected project
		if m.cursor < len(m.projects) {
			proj := m.projects[m.cursor]
			s += "\n"

			// Build card content
			var cardContent string
			cardContent += projCardTitleStyle.Render(proj.name) + "\n"
			if proj.repo != "" {
				cardContent += projCardLabelStyle.Render("Repo:       ") + projCardValueStyle.Render(proj.repo) + "\n"
			}
			cardContent += projCardLabelStyle.Render("Merge Mode: ") + projMergeModeStyle.Render(proj.mergeMode) + "\n"

			sessionStr := "none"
			if proj.sessionCount > 0 {
				sessionStr = fmt.Sprintf("%d active", proj.sessionCount)
			}
			cardContent += projCardLabelStyle.Render("Sessions:   ") + projSessionCountStyle.Render(sessionStr) + "\n"

			s += projCardStyle.Render(cardContent)
		}
	}

	// Help
	s += "\n\n"
	s += projHelpStyle.Render("↑/↓  navigate") + "\n"
	s += projHelpStyle.Render("q/esc  quit")

	return s
}

// Run the projects TUI
func runProjectsTUI(cfg *config.Config) error {
	m := newProjectsModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err := p.Run()
	return err
}
