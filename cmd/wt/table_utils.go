package main

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// Table styles
var (
	tableTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("170"))

	tableDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// renderTable creates and renders a styled table
func renderTable(title string, columns []table.Column, rows []table.Row) string {
	if len(rows) == 0 {
		return ""
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithHeight(len(rows)+1),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("229"))
	s.Selected = lipgloss.NewStyle() // No selection highlighting for static display
	s.Cell = s.Cell.Foreground(lipgloss.Color("252"))
	t.SetStyles(s)

	var output string
	if title != "" {
		output = tableTitleStyle.Render(title) + "\n\n"
	}
	output += t.View()

	return output
}

// printTable is a convenience function that prints a table directly
func printTable(title string, columns []table.Column, rows []table.Row) {
	fmt.Println(renderTable(title, columns, rows))
}

// printEmptyMessage prints a styled empty state message
func printEmptyMessage(message, hint string) {
	fmt.Println(tableDimStyle.Render(message))
	if hint != "" {
		fmt.Println(tableDimStyle.Render("\n" + hint))
	}
}
