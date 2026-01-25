package bead

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type BeadInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Project string `json:"project"`
}

// ReadyBead represents a bead returned by bd ready
type ReadyBead struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
}

func Show(beadID string) (*BeadInfo, error) {
	return ShowInDir(beadID, "")
}

// ShowInDir returns bead info from a specific beads directory
func ShowInDir(beadID, beadsDir string) (*BeadInfo, error) {
	// Determine project directory
	projectDir := ""
	if beadsDir != "" {
		projectDir = strings.TrimSuffix(beadsDir, "/.beads")
		projectDir = strings.TrimSuffix(projectDir, ".beads")
	}

	// Run bd show to get bead info
	cmd := exec.Command("bd", "show", beadID, "--json")
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	output, err := cmd.Output()
	if err != nil {
		// bd show might not support --json, try parsing text output
		return showFromTextInDir(beadID, projectDir)
	}

	var info BeadInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return showFromTextInDir(beadID, projectDir)
	}

	return &info, nil
}

func showFromText(beadID string) (*BeadInfo, error) {
	return showFromTextInDir(beadID, "")
}

func showFromTextInDir(beadID, projectDir string) (*BeadInfo, error) {
	cmd := exec.Command("bd", "show", beadID)
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead not found: %s", beadID)
	}

	// Parse the output - extract project from bead prefix
	project := extractProject(beadID)

	// Extract title from output (first line usually has the title)
	lines := strings.Split(string(output), "\n")
	title := ""
	for _, line := range lines {
		if strings.Contains(line, "·") {
			// Line format: "○ wt-bqf · Phase 1: Core commands..."
			parts := strings.Split(line, "·")
			if len(parts) >= 2 {
				title = strings.TrimSpace(parts[1])
				// Remove status suffix like "[● P0 · OPEN]"
				if idx := strings.Index(title, "["); idx > 0 {
					title = strings.TrimSpace(title[:idx])
				}
				break
			}
		}
	}

	return &BeadInfo{
		ID:      beadID,
		Title:   title,
		Project: project,
	}, nil
}

func Close(beadID string) error {
	cmd := exec.Command("bd", "close", beadID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("closing bead: %s: %w", string(output), err)
	}
	return nil
}

func UpdateStatus(beadID, status string) error {
	cmd := exec.Command("bd", "update", beadID, "--status", status)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating bead status: %s: %w", string(output), err)
	}
	return nil
}

func extractProject(beadID string) string {
	// Bead IDs are formatted as "project-xyz" where xyz is a random suffix
	// Split on the last hyphen to get the project name
	parts := strings.Split(beadID, "-")
	if len(parts) >= 2 {
		// Return everything except the last part (the random suffix)
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return beadID
}

// ExtractProject exports the project extraction for use by other packages
func ExtractProject(beadID string) string {
	return extractProject(beadID)
}

// Ready returns all beads that are ready to work on (no blockers)
func Ready() ([]ReadyBead, error) {
	cmd := exec.Command("bd", "ready", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting ready beads: %w", err)
	}

	var beads []ReadyBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing ready beads: %w", err)
	}

	return beads, nil
}

// ReadyInDir returns ready beads from a specific beads directory
// beadsDir should be the path to the .beads directory (e.g., /path/to/project/.beads)
func ReadyInDir(beadsDir string) ([]ReadyBead, error) {
	// bd expects to run from the project directory containing .beads/
	// Extract project dir from beadsDir (remove .beads suffix)
	projectDir := strings.TrimSuffix(beadsDir, "/.beads")
	projectDir = strings.TrimSuffix(projectDir, ".beads")

	cmd := exec.Command("bd", "ready", "--json")
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting ready beads from %s: %w", beadsDir, err)
	}

	var beads []ReadyBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing ready beads: %w", err)
	}

	return beads, nil
}

// CreateInDir creates a bead in a specific beads directory
func CreateInDir(beadsDir, title string, opts *CreateOptions) (string, error) {
	// bd expects to run from the project directory containing .beads/
	projectDir := strings.TrimSuffix(beadsDir, "/.beads")
	projectDir = strings.TrimSuffix(projectDir, ".beads")

	args := []string{"create", title}

	if opts != nil {
		if opts.Description != "" {
			args = append(args, "-d", opts.Description)
		}
		if opts.Priority >= 0 {
			args = append(args, "-p", fmt.Sprintf("%d", opts.Priority))
		}
		if opts.Type != "" {
			args = append(args, "-t", opts.Type)
		}
	}

	cmd := exec.Command("bd", args...)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating bead: %s: %w", string(output), err)
	}

	// Parse the created bead ID from output
	// Output format: "✓ Created issue: wt-xyz"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Created issue:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[len(parts)-1]), nil
			}
		}
	}

	return "", fmt.Errorf("could not parse created bead ID from: %s", string(output))
}

// CreateOptions holds options for creating a bead
type CreateOptions struct {
	Description string
	Priority    int
	Type        string
}

// ListInDir returns all beads from a specific beads directory
func ListInDir(beadsDir string, status string) ([]ReadyBead, error) {
	// bd expects to run from the project directory containing .beads/
	projectDir := strings.TrimSuffix(beadsDir, "/.beads")
	projectDir = strings.TrimSuffix(projectDir, ".beads")

	args := []string{"list", "--json"}
	if status != "" {
		args = append(args, "--status", status)
	}

	cmd := exec.Command("bd", args...)
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing beads from %s: %w", beadsDir, err)
	}

	var beads []ReadyBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing beads list: %w", err)
	}

	return beads, nil
}

// List returns beads with optional status filter
func List(status string) ([]ReadyBead, error) {
	args := []string{"list", "--json"}
	if status != "" {
		args = append(args, "--status", status)
	}

	cmd := exec.Command("bd", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing beads: %w", err)
	}

	var beads []ReadyBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing beads list: %w", err)
	}

	return beads, nil
}

// Search searches for beads by title
func Search(query string) ([]ReadyBead, error) {
	cmd := exec.Command("bd", "search", query, "--json")
	output, err := cmd.Output()
	if err != nil {
		// Search might not support --json, fall back to list and filter
		return searchFallback(query)
	}

	var beads []ReadyBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return searchFallback(query)
	}

	return beads, nil
}

// searchFallback lists all beads and filters by title
func searchFallback(query string) ([]ReadyBead, error) {
	beads, err := List("")
	if err != nil {
		return nil, err
	}

	var results []ReadyBead
	queryLower := strings.ToLower(query)
	for _, b := range beads {
		if strings.Contains(strings.ToLower(b.Title), queryLower) {
			results = append(results, b)
		}
	}

	return results, nil
}

// Create creates a new bead and returns its ID
func Create(title string, opts *CreateOptions) (string, error) {
	args := []string{"create", "--title", title}

	if opts != nil {
		if opts.Description != "" {
			args = append(args, "--description", opts.Description)
		}
		if opts.Priority >= 0 {
			args = append(args, "--priority", fmt.Sprintf("%d", opts.Priority))
		}
		if opts.Type != "" {
			args = append(args, "--type", opts.Type)
		}
	}

	cmd := exec.Command("bd", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating bead: %s: %w", string(output), err)
	}

	// Parse the created bead ID from output
	// Output format: "✓ Created issue: wt-xyz"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Created issue:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[len(parts)-1]), nil
			}
		}
	}

	return "", fmt.Errorf("could not parse created bead ID from: %s", string(output))
}

// UpdateDescription updates a bead's description
func UpdateDescription(beadID, description string) error {
	cmd := exec.Command("bd", "update", beadID, "--description", description)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating bead description: %s: %w", string(output), err)
	}
	return nil
}

// ShowFull returns full bead info including description
type BeadInfoFull struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Project     string `json:"project"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
}

// ShowFull returns full bead information including description
func ShowFull(beadID string) (*BeadInfoFull, error) {
	return ShowFullInDir(beadID, "")
}

// ShowFullInDir returns full bead info from a specific beads directory
func ShowFullInDir(beadID, beadsDir string) (*BeadInfoFull, error) {
	// Determine project directory
	projectDir := ""
	if beadsDir != "" {
		projectDir = strings.TrimSuffix(beadsDir, "/.beads")
		projectDir = strings.TrimSuffix(projectDir, ".beads")
	}

	// Try JSON first
	cmd := exec.Command("bd", "show", beadID, "--json")
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	output, err := cmd.Output()
	if err == nil {
		var info BeadInfoFull
		if err := json.Unmarshal(output, &info); err == nil {
			return &info, nil
		}
	}

	// Fallback to text parsing (bd show might not support --json)
	cmd = exec.Command("bd", "show", beadID)
	if projectDir != "" {
		cmd.Dir = projectDir
	}
	output, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead not found: %s", beadID)
	}

	// Parse text output
	info := &BeadInfoFull{
		ID:      beadID,
		Project: extractProject(beadID),
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Title line format: "○ wt-bqf · Phase 1: Core commands   [● P0 · OPEN]"
		if strings.Contains(line, "·") && strings.Contains(line, beadID) {
			parts := strings.Split(line, "·")
			if len(parts) >= 2 {
				title := strings.TrimSpace(parts[1])
				// Remove status suffix like "[● P0 · OPEN]"
				if idx := strings.Index(title, "["); idx > 0 {
					title = strings.TrimSpace(title[:idx])
				}
				info.Title = title
			}
		}
		// Description section
		if strings.HasPrefix(strings.TrimSpace(line), "DESCRIPTION") {
			// Next lines are description - simplified parsing
			continue
		}
	}

	return info, nil
}
