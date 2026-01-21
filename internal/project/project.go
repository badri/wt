package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/badri/wt/internal/config"
)

// Project represents a registered project configuration.
type Project struct {
	Name          string   `json:"name"`
	Repo          string   `json:"repo"`
	DefaultBranch string   `json:"default_branch,omitempty"`
	BeadsPrefix   string   `json:"beads_prefix,omitempty"`
	MergeMode     string   `json:"merge_mode,omitempty"`
	RequireCI     bool     `json:"require_ci,omitempty"`
	AutoMerge     bool     `json:"auto_merge_on_green,omitempty"`
	AutoRebase    string   `json:"auto_rebase,omitempty"` // "true" (default), "false", or "prompt"
	TestEnv       *TestEnv `json:"test_env,omitempty"`
	Hooks         *Hooks   `json:"hooks,omitempty"`
}

// AutoRebaseMode returns the effective auto-rebase mode for the project.
// Returns "true" (default), "false", or "prompt".
func (p *Project) AutoRebaseMode() string {
	if p.AutoRebase == "" {
		return "true" // Default: auto-rebase enabled
	}
	return p.AutoRebase
}

// TestEnv contains test environment configuration.
type TestEnv struct {
	Setup       string `json:"setup,omitempty"`
	Teardown    string `json:"teardown,omitempty"`
	PortEnv     string `json:"port_env,omitempty"`
	HealthCheck string `json:"health_check,omitempty"`
}

// Hooks contains lifecycle hook commands.
type Hooks struct {
	OnCreate []string `json:"on_create,omitempty"`
	OnClose  []string `json:"on_close,omitempty"`
}

// Manager handles project registration and lookup.
type Manager struct {
	projectsDir string
}

// NewManager creates a new project manager.
func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		projectsDir: filepath.Join(cfg.ConfigDir(), "projects"),
	}
}

// EnsureProjectsDir creates the projects directory if it doesn't exist.
func (m *Manager) EnsureProjectsDir() error {
	return os.MkdirAll(m.projectsDir, 0755)
}

// List returns all registered projects.
func (m *Manager) List() ([]*Project, error) {
	if err := m.EnsureProjectsDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(m.projectsDir)
	if err != nil {
		return nil, err
	}

	var projects []*Project
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".json")
		proj, err := m.Get(name)
		if err != nil {
			continue // Skip invalid configs
		}
		projects = append(projects, proj)
	}

	return projects, nil
}

// Get retrieves a project by name.
func (m *Manager) Get(name string) (*Project, error) {
	path := m.projectPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("project '%s' not found", name)
		}
		return nil, err
	}

	var proj Project
	if err := json.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("invalid project config: %w", err)
	}

	return &proj, nil
}

// Add registers a new project.
func (m *Manager) Add(name, repoPath string) (*Project, error) {
	if err := m.EnsureProjectsDir(); err != nil {
		return nil, err
	}

	// Check if project already exists
	if _, err := m.Get(name); err == nil {
		return nil, fmt.Errorf("project '%s' already exists", name)
	}

	// Expand and validate repo path
	expandedPath := expandPath(repoPath)
	if _, err := os.Stat(expandedPath); err != nil {
		return nil, fmt.Errorf("repo path does not exist: %s", expandedPath)
	}

	// Check it's a git repo
	gitDir := filepath.Join(expandedPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return nil, fmt.Errorf("not a git repository: %s", expandedPath)
	}

	proj := &Project{
		Name:          name,
		Repo:          repoPath, // Store original path (may include ~)
		DefaultBranch: "main",
		BeadsPrefix:   name,
		MergeMode:     "pr-review",
	}

	if err := m.Save(proj); err != nil {
		return nil, err
	}

	return proj, nil
}

// Save writes a project config to disk.
func (m *Manager) Save(proj *Project) error {
	if err := m.EnsureProjectsDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(proj, "", "  ")
	if err != nil {
		return err
	}

	path := m.projectPath(proj.Name)
	return os.WriteFile(path, data, 0644)
}

// Delete removes a project registration.
func (m *Manager) Delete(name string) error {
	path := m.projectPath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("project '%s' not found", name)
	}
	return os.Remove(path)
}

// FindByBeadPrefix finds a project by bead prefix.
func (m *Manager) FindByBeadPrefix(beadID string) (*Project, error) {
	projects, err := m.List()
	if err != nil {
		return nil, err
	}

	// Extract prefix from bead ID (e.g., "supabyoi-xyz" -> "supabyoi")
	prefix := extractPrefix(beadID)

	for _, proj := range projects {
		if proj.BeadsPrefix == prefix || proj.Name == prefix {
			return proj, nil
		}
	}

	return nil, fmt.Errorf("no project found for bead prefix '%s'", prefix)
}

// ConfigPath returns the path to a project's config file.
func (m *Manager) ConfigPath(name string) string {
	return m.projectPath(name)
}

// RepoPath returns the expanded repo path for a project.
func (p *Project) RepoPath() string {
	return expandPath(p.Repo)
}

// BeadsDir returns the beads directory for a project.
func (p *Project) BeadsDir() string {
	return filepath.Join(p.RepoPath(), ".beads")
}

func (m *Manager) projectPath(name string) string {
	return filepath.Join(m.projectsDir, name+".json")
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func extractPrefix(beadID string) string {
	// Bead IDs are "prefix-suffix" where suffix is random
	parts := strings.Split(beadID, "-")
	if len(parts) >= 2 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return beadID
}
