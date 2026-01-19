package namepool

import (
	"fmt"
	"hash/fnv"
	"sort"
)

// Theme names
const (
	ThemeKungFuPanda = "kungfu-panda"
	ThemeToyStory    = "toy-story"
	ThemeGhibli      = "ghibli"
	ThemeStarWars    = "star-wars"
	ThemeDune        = "dune"
	ThemeMatrix      = "matrix"
)

// themeNames contains all available themes and their names
var themeNames = map[string][]string{
	ThemeKungFuPanda: {
		"po", "tigress", "viper", "crane", "mantis", "monkey", "shifu", "oogway", "tailung", "shen",
		"kai", "ping", "li", "mei", "bao", "wolf", "gorilla", "rhino", "croc", "fenghuang",
	},
	ThemeToyStory: {
		"woody", "buzz", "jessie", "rex", "hamm", "slinky", "bullseye", "lotso", "ken", "barbie",
		"forky", "duke", "ducky", "bunny", "gabby", "bopeep", "wheezy", "lenny", "rocky", "sarge",
	},
	ThemeGhibli: {
		"totoro", "chihiro", "haku", "calcifer", "howl", "sophie", "kiki", "jiji", "ponyo", "ashitaka",
		"san", "moro", "nausicaa", "sheeta", "pazu", "arrietty", "sosuke", "markl", "lin", "kamaji",
	},
	ThemeStarWars: {
		"luke", "leia", "han", "chewie", "vader", "yoda", "ahsoka", "mando", "grogu", "boba",
		"rex", "padme", "anakin", "obiwan", "mace", "kylo", "rey", "finn", "poe", "lando",
	},
	ThemeDune: {
		"paul", "leto", "jessica", "duncan", "gurney", "stilgar", "chani", "alia", "feyd", "rabban",
		"irulan", "kynes", "yueh", "thufir", "hawat", "idaho", "mohiam", "jamis", "mapes", "harah",
	},
	ThemeMatrix: {
		"neo", "trinity", "morpheus", "oracle", "cypher", "tank", "niobe", "seraph", "link", "zee",
		"dozer", "apoc", "switch", "ghost", "rama", "lock", "ajax", "jax", "mifune", "soren",
	},
}

// allThemes is a sorted list of theme names for consistent hashing
var allThemes []string

func init() {
	allThemes = make([]string, 0, len(themeNames))
	for theme := range themeNames {
		allThemes = append(allThemes, theme)
	}
	sort.Strings(allThemes)
}

// ListThemes returns all available theme names
func ListThemes() []string {
	return allThemes
}

// GetThemeNames returns the names for a given theme
func GetThemeNames(theme string) ([]string, error) {
	names, ok := themeNames[theme]
	if !ok {
		return nil, fmt.Errorf("unknown theme: %s", theme)
	}
	// Return a copy to prevent modification
	result := make([]string, len(names))
	copy(result, names)
	return result, nil
}

// ThemeForProject returns a consistent theme for a project name.
// The same project name will always get the same theme.
func ThemeForProject(projectName string) string {
	h := fnv.New32a()
	h.Write([]byte(projectName))
	hash := h.Sum32()
	return allThemes[hash%uint32(len(allThemes))]
}

// IsThemedName checks if a name belongs to any theme
func IsThemedName(name string) bool {
	for _, names := range themeNames {
		for _, n := range names {
			if n == name {
				return true
			}
		}
	}
	return false
}
