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
	path  string
}

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

// NewPool creates a Pool with the given names (useful for testing)
func NewPool(names []string) *Pool {
	return &Pool{names: names}
}
