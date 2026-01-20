package main

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/lipgloss"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
)

// Styles for projects list
var (
	projTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	projHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Bold(true)

	projNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Bold(true)

	projRepoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	projMergeModeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	projSessionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("226"))

	projDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// printProjectsList prints a styled list of projects
func printProjectsList(cfg *config.Config) error {
	mgr := project.NewManager(cfg)
	projects, err := mgr.List()
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		fmt.Println(projDimStyle.Render("No projects registered."))
		fmt.Println(projDimStyle.Render("\nRegister a project: wt project add <name> <path>"))
		return nil
	}

	// Count active sessions per project
	state, _ := session.LoadState(cfg)
	sessionCount := make(map[string]int)
	for _, sess := range state.Sessions {
		sessionCount[sess.Project]++
	}

	// Sort projects by name
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})

	// Title
	fmt.Println(projTitleStyle.Render("Projects"))
	fmt.Println()

	// Header
	fmt.Printf("  %s  %s  %s  %s\n",
		projHeaderStyle.Render(fmt.Sprintf("%-16s", "Name")),
		projHeaderStyle.Render(fmt.Sprintf("%-24s", "Repo")),
		projHeaderStyle.Render(fmt.Sprintf("%-12s", "Merge Mode")),
		projHeaderStyle.Render("Sessions"))

	// Projects
	for _, proj := range projects {
		count := sessionCount[proj.Name]
		countStr := projDimStyle.Render("-")
		if count > 0 {
			countStr = projSessionStyle.Render(fmt.Sprintf("%d", count))
		}

		repoStr := projDimStyle.Render("-")
		if proj.Repo != "" {
			repoStr = projRepoStyle.Render(truncate(proj.Repo, 24))
		}

		modeStr := projMergeModeStyle.Render(proj.MergeMode)
		if proj.MergeMode == "" {
			modeStr = projDimStyle.Render("pr-review")
		}

		fmt.Printf("  %s  %s  %s  %s\n",
			projNameStyle.Render(fmt.Sprintf("%-16s", truncate(proj.Name, 16))),
			fmt.Sprintf("%-24s", repoStr),
			fmt.Sprintf("%-12s", modeStr),
			countStr)
	}

	fmt.Println()
	return nil
}
