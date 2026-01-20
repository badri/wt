package namepool

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/badri/wt/internal/config"
)

type Pool struct {
	names []string
	theme string
	path  string // optional, for file-based pools
}

// Load loads a namepool from the config file (legacy/fallback method)
func Load(cfg *config.Config) (*Pool, error) {
	path := cfg.NamepoolPath()
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening namepool: %w", err)
	}
	defer file.Close()

	var names []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			names = append(names, name)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading namepool: %w", err)
	}

	return &Pool{names: names, path: path}, nil
}

// LoadForProject loads a themed namepool based on the project name.
// The project name is hashed to consistently select a theme.
func LoadForProject(projectName string) (*Pool, error) {
	theme := ThemeForProject(projectName)
	names, err := GetThemeNames(theme)
	if err != nil {
		return nil, err
	}
	return &Pool{names: names, theme: theme}, nil
}

// LoadTheme loads a specific theme by name
func LoadTheme(themeName string) (*Pool, error) {
	names, err := GetThemeNames(themeName)
	if err != nil {
		return nil, err
	}
	return &Pool{names: names, theme: themeName}, nil
}

func (p *Pool) Allocate(usedNames []string) (string, error) {
	used := make(map[string]bool)
	for _, n := range usedNames {
		used[n] = true
	}

	for _, name := range p.names {
		if !used[name] {
			return name, nil
		}
	}

	return "", fmt.Errorf("namepool exhausted: all %d names are in use", len(p.names))
}

func (p *Pool) Names() []string {
	return p.names
}

func (p *Pool) Theme() string {
	return p.theme
}

// NewPool creates a Pool with the given names (useful for testing)
func NewPool(names []string) *Pool {
	return &Pool{names: names}
}
