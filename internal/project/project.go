package project

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/badri/wt/internal/config"
)

// Project represents a registered project configuration.
type Project struct {
	Name          string   `json:"name"`
	Repo          string   `json:"repo"`                     // Local path to the repository (may include ~)
	RepoURL       string   `json:"repo_url,omitempty"`       // Canonical git remote URL for repo identity
	DefaultBranch string   `json:"default_branch,omitempty"` // Branch to create worktrees from and merge back to
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

// AddOptions contains optional parameters for project registration.
type AddOptions struct {
	Branch    string // Target branch (defaults to "main")
	MergeMode string // Merge mode: "pr-review" or "direct"
}

// Add registers a new project.
func (m *Manager) Add(name, repoPath string, opts *AddOptions) (*Project, error) {
	if err := m.EnsureProjectsDir(); err != nil {
		return nil, err
	}

	// Check if project already exists
	if _, err := m.Get(name); err == nil {
		return nil, fmt.Errorf("project '%s' already exists", name)
	}

	// Expand and validate repo path
	expandedPath := ExpandPath(repoPath)
	if _, err := os.Stat(expandedPath); err != nil {
		return nil, fmt.Errorf("repo path does not exist: %s", expandedPath)
	}

	// Check it's a git repo
	gitDir := filepath.Join(expandedPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return nil, fmt.Errorf("not a git repository: %s", expandedPath)
	}

	// Determine branch
	branch := "main"
	if opts != nil && opts.Branch != "" {
		branch = opts.Branch
	}

	// Get git remote URL for canonical repo identity
	repoURL := getGitRemoteURL(expandedPath)

	// Auto-discover beads prefix from .beads/config.json if available
	beadsPrefix := discoverBeadsPrefix(expandedPath)
	if beadsPrefix == "" {
		beadsPrefix = name
	}

	// Check for existing registrations of this repo
	if repoURL != "" {
		existingProjects, _ := m.FindByRepoURL(repoURL)

		// If repo is already registered, inherit the beads prefix from existing registration
		if len(existingProjects) > 0 {
			beadsPrefix = existingProjects[0].BeadsPrefix
		}

		// Validate: check for conflicts with existing projects
		if err := m.validateRepoRegistration(repoURL, branch, beadsPrefix); err != nil {
			return nil, err
		}
	}

	// Determine merge mode
	mergeMode := "pr-review"
	if opts != nil && opts.MergeMode != "" {
		mergeMode = opts.MergeMode
	}

	proj := &Project{
		Name:          name,
		Repo:          repoPath, // Store original path (may include ~)
		RepoURL:       repoURL,
		DefaultBranch: branch,
		BeadsPrefix:   beadsPrefix,
		MergeMode:     mergeMode,
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
	return ExpandPath(p.Repo)
}

// BeadsDir returns the beads directory for a project.
func (p *Project) BeadsDir() string {
	return filepath.Join(p.RepoPath(), ".beads")
}

func (m *Manager) projectPath(name string) string {
	return filepath.Join(m.projectsDir, name+".json")
}

// ExpandPath expands ~ to the user's home directory.
func ExpandPath(path string) string {
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

// getGitRemoteURL gets the origin remote URL from a git repository.
func getGitRemoteURL(repoPath string) string {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// discoverBeadsPrefix reads the beads prefix from .beads/config.json if it exists.
func discoverBeadsPrefix(repoPath string) string {
	configPath := filepath.Join(repoPath, ".beads", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	var config struct {
		Prefix string `json:"prefix"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ""
	}

	return config.Prefix
}

// validateRepoRegistration checks for conflicts when registering a repo.
// It ensures:
// - Same repo with same branch cannot be registered twice (use existing project)
// - Same repo with different branch must have matching beads_prefix
func (m *Manager) validateRepoRegistration(repoURL, branch, beadsPrefix string) error {
	projects, err := m.List()
	if err != nil {
		return nil // Can't validate, proceed anyway
	}

	for _, existing := range projects {
		if existing.RepoURL != repoURL {
			continue
		}

		// Same repo found
		if existing.DefaultBranch == branch {
			return fmt.Errorf("repo already registered as project '%s' with branch '%s'. Use that project or choose a different branch", existing.Name, branch)
		}

		// Same repo, different branch - ensure beads prefix matches
		if existing.BeadsPrefix != beadsPrefix {
			return fmt.Errorf("repo already registered with beads prefix '%s' (project '%s'). All registrations of the same repo must share the same beads prefix", existing.BeadsPrefix, existing.Name)
		}
	}

	return nil
}

// FindByRepoURL finds all projects registered for a given repo URL.
func (m *Manager) FindByRepoURL(repoURL string) ([]*Project, error) {
	if repoURL == "" {
		return nil, nil
	}

	projects, err := m.List()
	if err != nil {
		return nil, err
	}

	var matches []*Project
	for _, proj := range projects {
		if proj.RepoURL == repoURL {
			matches = append(matches, proj)
		}
	}

	return matches, nil
}
