package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/badri/wt/internal/config"
	"github.com/charmbracelet/lipgloss"
)

// AuditResult represents the audit findings for a bead
type AuditResult struct {
	BeadID          string   `json:"bead_id"`
	Title           string   `json:"title"`
	Readiness       string   `json:"readiness"` // Ready, Partial, NotReady
	IssueCount      int      `json:"issue_count"`
	ValidRefs       []string `json:"valid_refs,omitempty"`
	InvalidRefs     []string `json:"invalid_refs,omitempty"`
	MissingContext  []string `json:"missing_context,omitempty"`
	Questions       []string `json:"questions,omitempty"`
	HasDescription  bool     `json:"has_description"`
	HasAcceptance   bool     `json:"has_acceptance_criteria"`
	HasDependencies bool     `json:"has_dependencies"`
	Dependencies    []string `json:"dependencies,omitempty"`
}

// BeadFullInfo contains all bead data for auditing
type BeadFullInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
	Owner       string `json:"owner"`
}

// auditFlags holds parsed flags for the audit command
type auditFlags struct {
	interactive bool
	projectDir  string
}

func parseAuditFlags(args []string) (string, auditFlags) {
	flags := auditFlags{}
	var beadID string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-i", "--interactive":
			flags.interactive = true
		case "-p", "--project":
			if i+1 < len(args) {
				flags.projectDir = args[i+1]
				i++
			}
		default:
			if beadID == "" && !strings.HasPrefix(args[i], "-") {
				beadID = args[i]
			}
		}
	}

	return beadID, flags
}

func cmdAuditHelp() error {
	help := `wt audit - Check bead readiness before implementation

USAGE:
    wt audit <bead-id> [options]

DESCRIPTION:
    Analyzes a bead's description and cross-references it with the codebase
    to determine implementation readiness. Reports what's well-defined,
    what's missing, and suggests clarifying questions.

OPTIONS:
    -i, --interactive       Enter interactive mode to resolve issues
    -p, --project <dir>     Project directory (default: current)

READINESS LEVELS:
    Ready     - Bead has sufficient context for implementation
    Partial   - Some issues need resolution before starting
    NotReady  - Significant context missing, not ready for implementation

EXAMPLES:
    wt audit wt-abc               Audit bead wt-abc
    wt audit wt-abc -i            Audit and interactively resolve issues
    wt audit wt-abc --project /path/to/project

OUTPUT:
    Shows:
    ✓ Valid file/code references found in codebase
    ✗ Missing context or invalid references
    ? Suggested clarifying questions
`
	fmt.Print(help)
	return nil
}

func cmdAudit(cfg *config.Config, args []string) error {
	beadID, flags := parseAuditFlags(args)

	if beadID == "" {
		return fmt.Errorf("usage: wt audit <bead-id> [-i|--interactive] [-p|--project <dir>]")
	}

	// Fetch bead information
	info, err := fetchBeadInfo(beadID, flags.projectDir)
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}

	// Perform the audit
	result := auditBead(info, flags.projectDir)

	// Output results
	if outputJSON {
		printJSON(result)
		return nil
	}

	printAuditResult(result, info)

	// Interactive mode
	if flags.interactive && result.Readiness != "Ready" {
		return runInteractiveAudit(info, result, flags.projectDir)
	}

	return nil
}

func fetchBeadInfo(beadID, projectDir string) (*BeadFullInfo, error) {
	cmd := exec.Command("bd", "show", beadID, "--json")
	if projectDir != "" {
		cmd.Dir = projectDir
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead not found: %s", beadID)
	}

	// bd show --json returns an array with one element
	var infos []BeadFullInfo
	if err := json.Unmarshal(output, &infos); err != nil {
		return nil, fmt.Errorf("parsing bead info: %w", err)
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("bead not found: %s", beadID)
	}

	return &infos[0], nil
}

func fetchDependencies(beadID, projectDir string) []string {
	cmd := exec.Command("bd", "dep", "list", beadID, "--json")
	if projectDir != "" {
		cmd.Dir = projectDir
	}

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var deps []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(output, &deps); err != nil {
		return nil
	}

	var result []string
	for _, d := range deps {
		result = append(result, fmt.Sprintf("%s: %s", d.ID, d.Title))
	}
	return result
}

func auditBead(info *BeadFullInfo, projectDir string) *AuditResult {
	result := &AuditResult{
		BeadID: info.ID,
		Title:  info.Title,
	}

	// Check description presence and quality
	result.HasDescription = len(strings.TrimSpace(info.Description)) > 50

	// Check for acceptance criteria keywords
	descLower := strings.ToLower(info.Description)
	result.HasAcceptance = strings.Contains(descLower, "acceptance") ||
		strings.Contains(descLower, "expected") ||
		strings.Contains(descLower, "should") ||
		strings.Contains(descLower, "must") ||
		strings.Contains(descLower, "requirement")

	// Fetch dependencies
	deps := fetchDependencies(info.ID, projectDir)
	result.Dependencies = deps
	result.HasDependencies = len(deps) > 0

	// Extract and validate file references
	validRefs, invalidRefs := extractAndValidateRefs(info.Description, projectDir)
	result.ValidRefs = validRefs
	result.InvalidRefs = invalidRefs

	// Identify missing context
	result.MissingContext = identifyMissingContext(info.Description)

	// Generate suggested questions
	result.Questions = generateQuestions(info.Description, result)

	// Calculate readiness score
	issues := 0
	if !result.HasDescription {
		issues++
	}
	if !result.HasAcceptance {
		issues++
	}
	issues += len(result.InvalidRefs)
	issues += len(result.MissingContext)

	result.IssueCount = issues

	if issues == 0 {
		result.Readiness = "Ready"
	} else if issues <= 3 {
		result.Readiness = "Partial"
	} else {
		result.Readiness = "NotReady"
	}

	return result
}

func extractAndValidateRefs(description, projectDir string) (valid, invalid []string) {
	// Patterns to match file references
	patterns := []*regexp.Regexp{
		// File paths like app/services/foo.py, src/components/Bar.tsx
		regexp.MustCompile(`(?:^|\s|[(\[])([a-zA-Z0-9_/.-]+\.(go|py|ts|tsx|js|jsx|rb|java|rs|c|cpp|h|hpp|swift|kt))`),
		// Directory paths like internal/config/, src/utils/
		regexp.MustCompile(`(?:^|\s|[(\[])([a-zA-Z0-9_]+(?:/[a-zA-Z0-9_]+)+/)`),
		// Markdown code blocks with file paths
		regexp.MustCompile("```[^`]*?([a-zA-Z0-9_/.-]+\\.(go|py|ts|tsx|js|jsx))"),
	}

	seen := make(map[string]bool)
	var validRefs, invalidRefs []string

	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(description, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			ref := strings.TrimSpace(match[1])
			ref = strings.TrimPrefix(ref, "-")
			ref = strings.TrimSuffix(ref, "/")

			if ref == "" || seen[ref] {
				continue
			}
			seen[ref] = true

			// Skip common false positives
			if isCommonFalsePositive(ref) {
				continue
			}

			// Check if file exists
			searchDir := projectDir
			if searchDir == "" {
				searchDir = "."
			}

			if fileExists(filepath.Join(searchDir, ref)) {
				validRefs = append(validRefs, ref)
			} else {
				// Try to find with glob
				found := findFileWithGlob(searchDir, ref)
				if found != "" {
					validRefs = append(validRefs, found)
				} else {
					invalidRefs = append(invalidRefs, ref)
				}
			}
		}
	}

	// Sort for consistent output
	sort.Strings(validRefs)
	sort.Strings(invalidRefs)

	return validRefs, invalidRefs
}

func isCommonFalsePositive(ref string) bool {
	// Skip version numbers, URLs, and other common false positives
	falsePositives := []string{
		"example.com", "localhost", "github.com", "0.0.0.0",
		"v1.0", "v2.0", "1.0.0", "2.0.0",
	}
	for _, fp := range falsePositives {
		if strings.Contains(ref, fp) {
			return true
		}
	}

	// Skip if it starts with http/https
	if strings.HasPrefix(ref, "http") {
		return true
	}

	// Skip very short references
	if len(ref) < 3 {
		return true
	}

	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findFileWithGlob(searchDir, pattern string) string {
	// Try to find file with basename
	basename := filepath.Base(pattern)
	if basename == pattern {
		// It's just a filename, search for it
		matches, err := filepath.Glob(filepath.Join(searchDir, "**", basename))
		if err == nil && len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

func identifyMissingContext(description string) []string {
	var missing []string
	descLower := strings.ToLower(description)

	// Check for vague references
	vaguePatterns := []struct {
		pattern string
		issue   string
	}{
		{"the table", "Which specific table/model?"},
		{"the database", "Which database or data store?"},
		{"the api", "Which API endpoint?"},
		{"the service", "Which service specifically?"},
		{"the config", "Which configuration file/setting?"},
		{"somewhere", "Location not specified"},
		{"somehow", "Implementation approach unclear"},
		{"etc", "Requirements may be incomplete"},
		{"tbd", "To be determined items exist"},
		{"todo", "Unresolved TODO items"},
	}

	for _, vp := range vaguePatterns {
		if strings.Contains(descLower, vp.pattern) {
			missing = append(missing, vp.issue)
		}
	}

	// Check for missing common requirements
	if len(description) < 100 && !strings.Contains(descLower, "acceptance") {
		missing = append(missing, "No acceptance criteria defined")
	}

	// Check for error handling mention
	if strings.Contains(descLower, "error") || strings.Contains(descLower, "fail") {
		if !strings.Contains(descLower, "if") && !strings.Contains(descLower, "when") {
			missing = append(missing, "Error handling behavior not specified")
		}
	}

	return missing
}

func generateQuestions(description string, result *AuditResult) []string {
	var questions []string
	descLower := strings.ToLower(description)

	// Add questions based on missing context
	for _, mc := range result.MissingContext {
		switch {
		case strings.Contains(mc, "table"):
			questions = append(questions, "What is the name of the table/model to use?")
		case strings.Contains(mc, "API"):
			questions = append(questions, "Which API endpoint should this integrate with?")
		case strings.Contains(mc, "service"):
			questions = append(questions, "Which service should handle this functionality?")
		}
	}

	// Add questions based on common patterns
	if strings.Contains(descLower, "task") || strings.Contains(descLower, "job") || strings.Contains(descLower, "schedule") {
		if !strings.Contains(descLower, "daily") && !strings.Contains(descLower, "hourly") &&
			!strings.Contains(descLower, "minute") && !strings.Contains(descLower, "cron") {
			questions = append(questions, "What is the execution schedule/frequency?")
		}
	}

	if strings.Contains(descLower, "user") {
		if !strings.Contains(descLower, "permission") && !strings.Contains(descLower, "auth") {
			questions = append(questions, "What permissions/roles should users have for this feature?")
		}
	}

	if strings.Contains(descLower, "data") || strings.Contains(descLower, "store") {
		if !strings.Contains(descLower, "persist") && !strings.Contains(descLower, "cache") {
			questions = append(questions, "How should the data be persisted/cached?")
		}
	}

	if !result.HasAcceptance {
		questions = append(questions, "What are the acceptance criteria for this feature?")
	}

	// Deduplicate questions
	seen := make(map[string]bool)
	var unique []string
	for _, q := range questions {
		if !seen[q] {
			seen[q] = true
			unique = append(unique, q)
		}
	}

	return unique
}

func printAuditResult(result *AuditResult, info *BeadFullInfo) {
	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	readyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("46"))
	partialStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
	notReadyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	crossStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	questionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))

	// Header
	fmt.Println()
	fmt.Printf("%s %s \"%s\"\n",
		headerStyle.Render("Audit:"),
		result.BeadID,
		truncate(result.Title, 50))
	fmt.Println()

	// Readiness status
	var readinessDisplay string
	switch result.Readiness {
	case "Ready":
		readinessDisplay = readyStyle.Render("Ready")
	case "Partial":
		readinessDisplay = fmt.Sprintf("%s (%d issues)",
			partialStyle.Render("Partial"),
			result.IssueCount)
	case "NotReady":
		readinessDisplay = fmt.Sprintf("%s (%d issues)",
			notReadyStyle.Render("Not Ready"),
			result.IssueCount)
	}
	fmt.Printf("Readiness: %s\n\n", readinessDisplay)

	// Valid references
	if len(result.ValidRefs) > 0 {
		fmt.Printf("%s References existing code:\n", checkStyle.Render("✓"))
		for _, ref := range result.ValidRefs {
			fmt.Printf("  %s %s\n", dimStyle.Render("-"), ref)
		}
		fmt.Println()
	}

	// Invalid references
	if len(result.InvalidRefs) > 0 {
		fmt.Printf("%s References not found:\n", crossStyle.Render("✗"))
		for _, ref := range result.InvalidRefs {
			fmt.Printf("  %s %s\n", dimStyle.Render("-"), ref)
		}
		fmt.Println()
	}

	// Missing context
	if len(result.MissingContext) > 0 {
		fmt.Printf("%s Missing context:\n", crossStyle.Render("✗"))
		for _, mc := range result.MissingContext {
			fmt.Printf("  %s %s\n", dimStyle.Render("-"), mc)
		}
		fmt.Println()
	}

	// Check description quality
	if !result.HasDescription {
		fmt.Printf("%s Description too brief (less than 50 characters)\n", crossStyle.Render("✗"))
	}
	if !result.HasAcceptance {
		fmt.Printf("%s No acceptance criteria detected\n", crossStyle.Render("✗"))
	}

	// Dependencies
	if len(result.Dependencies) > 0 {
		fmt.Printf("\n%s Dependencies:\n", dimStyle.Render("→"))
		for _, dep := range result.Dependencies {
			fmt.Printf("  %s %s\n", dimStyle.Render("-"), dep)
		}
	}

	// Suggested questions
	if len(result.Questions) > 0 {
		fmt.Printf("\n%s Suggested questions:\n", questionStyle.Render("?"))
		for i, q := range result.Questions {
			fmt.Printf("  %d. %s\n", i+1, q)
		}
	}

	fmt.Println()
}

func runInteractiveAudit(info *BeadFullInfo, result *AuditResult, projectDir string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter interactive mode to resolve? [y/N] ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		return nil
	}

	fmt.Println("\nInteractive mode: answer questions to improve bead description")
	fmt.Println("Press Enter to skip a question, type 'done' to finish")
	fmt.Println()

	var updates []string

	for i, question := range result.Questions {
		fmt.Printf("[%d/%d] %s\n", i+1, len(result.Questions), question)
		fmt.Print("> ")

		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(answer)

		if answer == "done" {
			break
		}

		if answer != "" {
			updates = append(updates, fmt.Sprintf("- %s: %s", question, answer))
		}
	}

	if len(updates) == 0 {
		fmt.Println("\nNo updates to apply.")
		return nil
	}

	// Build new description
	newContext := "\n\n## Additional Context\n" + strings.Join(updates, "\n")
	newDescription := info.Description + newContext

	fmt.Println("\nProposed update to description:")
	fmt.Println("---")
	fmt.Println(newContext)
	fmt.Println("---")

	fmt.Print("\nApply this update? [y/N] ")
	response, _ = reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response != "y" && response != "yes" {
		fmt.Println("Update cancelled.")
		return nil
	}

	// Update the bead description
	cmd := exec.Command("bd", "update", info.ID, "--description", newDescription)
	if projectDir != "" {
		cmd.Dir = projectDir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("updating bead: %s: %w", string(output), err)
	}

	fmt.Println("✓ Bead description updated.")

	// Re-audit
	fmt.Println("\nRe-auditing...")
	updatedInfo, err := fetchBeadInfo(info.ID, projectDir)
	if err != nil {
		return err
	}

	newResult := auditBead(updatedInfo, projectDir)
	printAuditResult(newResult, updatedInfo)

	if newResult.Readiness != "Ready" && len(newResult.Questions) > 0 {
		return runInteractiveAudit(updatedInfo, newResult, projectDir)
	}

	return nil
}
