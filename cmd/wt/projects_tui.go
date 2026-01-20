package main

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/table"

	"github.com/badri/wt/internal/config"
	"github.com/badri/wt/internal/project"
	"github.com/badri/wt/internal/session"
)

// ProjectJSON is the JSON output format for a project
type ProjectJSON struct {
	Name          string `json:"name"`
	Repo          string `json:"repo"`
	DefaultBranch string `json:"default_branch,omitempty"`
	BeadsPrefix   string `json:"beads_prefix,omitempty"`
	MergeMode     string `json:"merge_mode"`
	RequireCI     bool   `json:"require_ci,omitempty"`
	AutoMerge     bool   `json:"auto_merge_on_green,omitempty"`
	SessionCount  int    `json:"session_count"`
}

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

	// JSON output
	if outputJSON {
		var result []ProjectJSON
		for _, proj := range projects {
			modeStr := proj.MergeMode
			if modeStr == "" {
				modeStr = "pr-review"
			}
			result = append(result, ProjectJSON{
				Name:          proj.Name,
				Repo:          proj.Repo,
				DefaultBranch: proj.DefaultBranch,
				BeadsPrefix:   proj.BeadsPrefix,
				MergeMode:     modeStr,
				RequireCI:     proj.RequireCI,
				AutoMerge:     proj.AutoMerge,
				SessionCount:  sessionCount[proj.Name],
			})
		}
		printJSON(result)
		return nil
	}

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
