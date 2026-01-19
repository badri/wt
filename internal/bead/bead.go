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

func Show(beadID string) (*BeadInfo, error) {
	// Run bd show to get bead info
	cmd := exec.Command("bd", "show", beadID, "--json")
	output, err := cmd.Output()
	if err != nil {
		// bd show might not support --json, try parsing text output
		return showFromText(beadID)
	}

	var info BeadInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return showFromText(beadID)
	}

	return &info, nil
}

func showFromText(beadID string) (*BeadInfo, error) {
	cmd := exec.Command("bd", "show", beadID)
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
