// Package hub provides hub-level beads management.
package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/badri/wt/internal/config"
)

const (
	// HubBeadsDir is the subdirectory for hub beads within config
	HubBeadsDir = ".beads"

	// HubBeadPrefix is the prefix for hub-level beads
	HubBeadPrefix = "hub"

	// HandoffBeadTitle is the well-known title for the handoff bead
	HandoffBeadTitle = "Hub Handoff"

	// StatusPinned is the status for pinned beads that persist permanently
	StatusPinned = "pinned"
)

// HubBead represents a hub-level bead
type HubBead struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// InitHubBeads initializes the hub-level beads store
func InitHubBeads(cfg *config.Config) error {
	beadsDir := GetHubBeadsDir(cfg)

	// Check if already initialized
	if _, err := os.Stat(beadsDir); err == nil {
		return nil // Already exists
	}

	// Create the directory
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating hub beads directory: %w", err)
	}

	// Initialize beads with hub- prefix
	cmd := exec.Command("bd", "init", "--prefix", HubBeadPrefix)
	cmd.Dir = cfg.ConfigDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initializing hub beads: %s: %w", string(output), err)
	}

	return nil
}

// GetHubBeadsDir returns the path to the hub beads directory
func GetHubBeadsDir(cfg *config.Config) string {
	return filepath.Join(cfg.ConfigDir(), HubBeadsDir)
}

// EnsureHubBeads ensures hub beads are initialized, returns the beads dir
func EnsureHubBeads(cfg *config.Config) (string, error) {
	beadsDir := GetHubBeadsDir(cfg)

	// Check if initialized
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		if err := InitHubBeads(cfg); err != nil {
			return "", err
		}
	}

	return beadsDir, nil
}

// GetHandoffBead returns the hub handoff bead, creating it if needed
func GetHandoffBead(cfg *config.Config) (*HubBead, error) {
	beadsDir, err := EnsureHubBeads(cfg)
	if err != nil {
		return nil, err
	}

	// Search for existing handoff bead
	bead, err := findBeadByTitle(cfg.ConfigDir(), HandoffBeadTitle)
	if err != nil {
		return nil, err
	}

	if bead != nil {
		return bead, nil
	}

	// Create new handoff bead
	return createHandoffBead(cfg.ConfigDir(), beadsDir)
}

// UpdateHandoffBead updates the handoff bead's description with new content
func UpdateHandoffBead(cfg *config.Config, content string) error {
	bead, err := GetHandoffBead(cfg)
	if err != nil {
		return err
	}

	// Update the description
	cmd := exec.Command("bd", "update", bead.ID, "--description", content)
	cmd.Dir = cfg.ConfigDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating handoff bead: %s: %w", string(output), err)
	}

	return nil
}

// ReadHandoffBead reads the current content of the handoff bead
func ReadHandoffBead(cfg *config.Config) (string, error) {
	bead, err := GetHandoffBead(cfg)
	if err != nil {
		return "", err
	}

	return bead.Description, nil
}

// ClearHandoffBead clears the handoff bead's description
func ClearHandoffBead(cfg *config.Config) error {
	return UpdateHandoffBead(cfg, "")
}

// findBeadByTitle searches for a bead by title in the hub beads
func findBeadByTitle(projectDir, title string) (*HubBead, error) {
	// List all beads and find by title
	cmd := exec.Command("bd", "list", "--json")
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		// No beads yet is not an error
		return nil, nil
	}

	var beads []HubBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing beads: %w", err)
	}

	for _, b := range beads {
		if b.Title == title {
			// Fetch full details
			return showBead(projectDir, b.ID)
		}
	}

	return nil, nil
}

// showBead returns full bead details
func showBead(projectDir, beadID string) (*HubBead, error) {
	cmd := exec.Command("bd", "show", beadID, "--json")
	cmd.Dir = projectDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("showing bead %s: %w", beadID, err)
	}

	var bead HubBead
	if err := json.Unmarshal(output, &bead); err != nil {
		return nil, fmt.Errorf("parsing bead: %w", err)
	}

	return &bead, nil
}

// createHandoffBead creates the handoff bead with pinned status
func createHandoffBead(projectDir, _ string) (*HubBead, error) {
	// Create the bead
	args := []string{
		"create",
		"--title", HandoffBeadTitle,
		"--type", "task",
		"--priority", "2",
		"--description", "",
	}

	cmd := exec.Command("bd", args...)
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("creating handoff bead: %s: %w", string(output), err)
	}

	// Parse the created bead ID
	beadID := parseCreatedBeadID(string(output))
	if beadID == "" {
		return nil, fmt.Errorf("could not parse created bead ID from: %s", string(output))
	}

	// Update to pinned status
	updateCmd := exec.Command("bd", "update", beadID, "--status", StatusPinned)
	updateCmd.Dir = projectDir
	if output, err := updateCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pinning handoff bead: %s: %w", string(output), err)
	}

	// Return the created bead
	return showBead(projectDir, beadID)
}

// parseCreatedBeadID extracts the bead ID from bd create output
func parseCreatedBeadID(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Created issue:") || strings.Contains(line, "Created") {
			// Try to find the bead ID (hub-xxx format)
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.HasPrefix(part, HubBeadPrefix+"-") {
					return strings.TrimSuffix(part, ".")
				}
			}
			// Fallback: look after colon
			if idx := strings.LastIndex(line, ":"); idx != -1 {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""
}

// FormatHandoffContent formats handoff content with metadata
func FormatHandoffContent(message string, autoCollected string) string {
	var sb strings.Builder

	sb.WriteString("## Handoff Context\n")
	sb.WriteString(fmt.Sprintf("Time: %s\n\n", time.Now().Format(time.RFC3339)))

	if message != "" {
		sb.WriteString("### Notes\n")
		sb.WriteString(message)
		sb.WriteString("\n\n")
	}

	if autoCollected != "" {
		sb.WriteString(autoCollected)
	}

	return sb.String()
}

// CollectHubState gathers hub state for handoff context
func CollectHubState(cfg *config.Config) string {
	var parts []string

	// Get active worker sessions
	sessionsOutput, err := exec.Command("wt", "list", "--json").Output()
	if err == nil {
		var sessions map[string]interface{}
		if json.Unmarshal(sessionsOutput, &sessions) == nil && len(sessions) > 0 {
			var lines []string
			for name, info := range sessions {
				if infoMap, ok := info.(map[string]interface{}); ok {
					bead := infoMap["bead"]
					project := infoMap["project"]
					lines = append(lines, fmt.Sprintf("- %s: %v (%v)", name, bead, project))
				}
			}
			if len(lines) > 0 {
				parts = append(parts, "### Active Sessions\n"+strings.Join(lines, "\n"))
			}
		}
	}

	// Get ready beads across projects
	readyOutput, err := exec.Command("wt", "ready", "--json").Output()
	if err == nil {
		var ready []map[string]interface{}
		if json.Unmarshal(readyOutput, &ready) == nil && len(ready) > 0 {
			var lines []string
			for _, b := range ready {
				id := b["id"]
				title := b["title"]
				priority := b["priority"]
				lines = append(lines, fmt.Sprintf("- %v: %v (P%v)", id, title, priority))
			}
			if len(lines) > 0 {
				if len(lines) > 10 {
					lines = append(lines[:10], "... (more)")
				}
				parts = append(parts, "### Ready Beads\n"+strings.Join(lines, "\n"))
			}
		}
	}

	// Get in-progress beads
	inProgressOutput, err := exec.Command("bd", "list", "--status=in_progress", "--json").Output()
	if err == nil {
		var inProgress []map[string]interface{}
		if json.Unmarshal(inProgressOutput, &inProgress) == nil && len(inProgress) > 0 {
			var lines []string
			for _, b := range inProgress {
				id := b["id"]
				title := b["title"]
				lines = append(lines, fmt.Sprintf("- %v: %v", id, title))
			}
			if len(lines) > 0 {
				if len(lines) > 5 {
					lines = append(lines[:5], "... (more)")
				}
				parts = append(parts, "### In Progress\n"+strings.Join(lines, "\n"))
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}
