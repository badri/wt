package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	WorktreeRoot     string `json:"worktree_root"`
	EditorCmd        string `json:"editor_cmd"`
	DefaultMergeMode string `json:"default_merge_mode"`

	// Internal paths
	configDir string
}

func Load() (*Config, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, err
	}
	return LoadFromDir(configDir)
}

func LoadFromDir(configDir string) (*Config, error) {
	cfg := &Config{
		WorktreeRoot:     "~/worktrees",
		EditorCmd:        "claude --dangerously-skip-permissions",
		DefaultMergeMode: "pr-review",
		configDir:        configDir,
	}

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	// Load config.json if it exists
	configPath := filepath.Join(configDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	// Ensure namepool exists
	if err := cfg.ensureNamepool(); err != nil {
		return nil, err
	}

	// Ensure sessions.json exists
	if err := cfg.ensureSessionsFile(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) ConfigDir() string {
	return c.configDir
}

func (c *Config) NamepoolPath() string {
	return filepath.Join(c.configDir, "namepool.txt")
}

func (c *Config) SessionsPath() string {
	return filepath.Join(c.configDir, "sessions.json")
}

func (c *Config) WorktreePath(sessionName string) string {
	root := expandPath(c.WorktreeRoot)
	return filepath.Join(root, sessionName)
}

func (c *Config) ConfigPath() string {
	return filepath.Join(c.configDir, "config.json")
}

// Save writes the config to disk
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.ConfigPath(), data, 0644)
}

// ConfigExists returns true if config.json exists
func (c *Config) ConfigExists() bool {
	_, err := os.Stat(c.ConfigPath())
	return err == nil
}

func (c *Config) ensureNamepool() error {
	path := c.NamepoolPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		defaultNames := `toast
shadow
obsidian
quartz
jasper
onyx
opal
topaz
marble
granite
amber
crystal
flint
slate
copper
bronze
silver
cobalt
iron
steel
`
		return os.WriteFile(path, []byte(defaultNames), 0644)
	}
	return nil
}

func (c *Config) ensureSessionsFile() error {
	path := c.SessionsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.WriteFile(path, []byte("{}"), 0644)
	}
	return nil
}

func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "wt"), nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
