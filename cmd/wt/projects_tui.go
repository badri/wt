package main

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/table"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
)

// printProjectsList prints a styled table of projects
func printProjectsList(cfg *config.Config) error {
	mgr := project.NewManager(cfg)
	projects, err := mgr.List()
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		printEmptyMessage("No projects registered.", "Register a project: wt project add <name> <path>")
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

	// Define columns
	columns := []table.Column{
		{Title: "Name", Width: 16},
		{Title: "Repo", Width: 28},
		{Title: "Merge Mode", Width: 12},
		{Title: "Sessions", Width: 10},
	}

	// Build rows
	var rows []table.Row
	for _, proj := range projects {
		count := sessionCount[proj.Name]
		countStr := "-"
		if count > 0 {
			countStr = fmt.Sprintf("%d", count)
		}

		repoStr := "-"
		if proj.Repo != "" {
			repoStr = truncate(proj.Repo, 28)
		}

		modeStr := proj.MergeMode
		if modeStr == "" {
			modeStr = "pr-review"
		}

		rows = append(rows, table.Row{
			proj.Name,
			repoStr,
			modeStr,
			countStr,
		})
	}

	printTable("Projects", columns, rows)
	return nil
}
