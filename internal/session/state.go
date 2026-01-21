package session

import (
	"encoding/json"
	"os"

	"github.com/badri/wt/internal/config"
)

type State struct {
	Sessions map[string]*Session `json:"sessions"`
	path     string
}

func LoadState(cfg *config.Config) (*State, error) {
	path := cfg.SessionsPath()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// sessions.json is a flat map, not nested under "sessions"
	var sessions map[string]*Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}

	if sessions == nil {
		sessions = make(map[string]*Session)
	}

	return &State{
		Sessions: sessions,
		path:     path,
	}, nil
}

func (s *State) Save() error {
	data, err := json.MarshalIndent(s.Sessions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// UsedNames returns theme names that have been allocated from namepools.
// This is used by namepool.Allocate() to skip already-used names.
func (s *State) UsedNames() []string {
	names := make([]string, 0, len(s.Sessions))
	for sessionName, sess := range s.Sessions {
		// Use ThemeName if available, fall back to session name for backwards compatibility
		if sess.ThemeName != "" {
			names = append(names, sess.ThemeName)
		} else {
			names = append(names, sessionName)
		}
	}
	return names
}

func (s *State) FindByBead(beadID string) (string, *Session) {
	for name, sess := range s.Sessions {
		if sess.Bead == beadID {
			return name, sess
		}
	}
	return "", nil
}
