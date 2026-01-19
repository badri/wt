package namepool

import (
	"testing"
)

func TestListThemes(t *testing.T) {
	themes := ListThemes()
	if len(themes) != 6 {
		t.Errorf("expected 6 themes, got %d", len(themes))
	}

	expected := map[string]bool{
		"kungfu-panda": true,
		"toy-story":    true,
		"ghibli":       true,
		"star-wars":    true,
		"dune":         true,
		"matrix":       true,
	}

	for _, theme := range themes {
		if !expected[theme] {
			t.Errorf("unexpected theme: %s", theme)
		}
	}
}

func TestGetThemeNames(t *testing.T) {
	// Test valid themes have 20 names each
	for _, theme := range ListThemes() {
		names, err := GetThemeNames(theme)
		if err != nil {
			t.Errorf("failed to get names for theme %s: %v", theme, err)
			continue
		}
		if len(names) != 20 {
			t.Errorf("theme %s has %d names, expected 20", theme, len(names))
		}
	}

	// Test invalid theme
	_, err := GetThemeNames("invalid-theme")
	if err == nil {
		t.Error("expected error for invalid theme")
	}
}

func TestThemeForProject(t *testing.T) {
	// Same project should always get same theme
	theme1 := ThemeForProject("myproject")
	theme2 := ThemeForProject("myproject")
	if theme1 != theme2 {
		t.Errorf("same project got different themes: %s vs %s", theme1, theme2)
	}

	// Different projects should (likely) get different themes
	themes := make(map[string]bool)
	projects := []string{"project-a", "project-b", "project-c", "project-d", "foo", "bar"}
	for _, p := range projects {
		themes[ThemeForProject(p)] = true
	}
	// With 6 projects and 6 themes, we should get at least 2 different themes
	if len(themes) < 2 {
		t.Errorf("expected variety in themes, got only %d unique theme(s)", len(themes))
	}
}

func TestLoadForProject(t *testing.T) {
	pool, err := LoadForProject("wt")
	if err != nil {
		t.Fatalf("failed to load pool for project: %v", err)
	}

	if pool.Theme() == "" {
		t.Error("pool should have a theme set")
	}

	if len(pool.Names()) != 20 {
		t.Errorf("pool should have 20 names, got %d", len(pool.Names()))
	}
}

func TestLoadTheme(t *testing.T) {
	pool, err := LoadTheme(ThemeStarWars)
	if err != nil {
		t.Fatalf("failed to load star-wars theme: %v", err)
	}

	if pool.Theme() != ThemeStarWars {
		t.Errorf("expected theme %s, got %s", ThemeStarWars, pool.Theme())
	}

	// Check some expected names
	names := pool.Names()
	hasLuke := false
	for _, n := range names {
		if n == "luke" {
			hasLuke = true
			break
		}
	}
	if !hasLuke {
		t.Error("star-wars theme should contain 'luke'")
	}
}
