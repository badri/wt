package main

import (
	"testing"
)

func TestParseAuditFlags(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantBeadID      string
		wantInteractive bool
		wantProjectDir  string
	}{
		{
			name:       "bead only",
			args:       []string{"test-bead"},
			wantBeadID: "test-bead",
		},
		{
			name:            "with interactive short",
			args:            []string{"test-bead", "-i"},
			wantBeadID:      "test-bead",
			wantInteractive: true,
		},
		{
			name:            "with interactive long",
			args:            []string{"test-bead", "--interactive"},
			wantBeadID:      "test-bead",
			wantInteractive: true,
		},
		{
			name:           "with project short",
			args:           []string{"test-bead", "-p", "/path/to/project"},
			wantBeadID:     "test-bead",
			wantProjectDir: "/path/to/project",
		},
		{
			name:           "with project long",
			args:           []string{"test-bead", "--project", "/path/to/project"},
			wantBeadID:     "test-bead",
			wantProjectDir: "/path/to/project",
		},
		{
			name:            "all flags",
			args:            []string{"test-bead", "-i", "-p", "/project"},
			wantBeadID:      "test-bead",
			wantInteractive: true,
			wantProjectDir:  "/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadID, flags := parseAuditFlags(tt.args)

			if beadID != tt.wantBeadID {
				t.Errorf("beadID = %q, want %q", beadID, tt.wantBeadID)
			}
			if flags.interactive != tt.wantInteractive {
				t.Errorf("interactive = %v, want %v", flags.interactive, tt.wantInteractive)
			}
			if flags.projectDir != tt.wantProjectDir {
				t.Errorf("projectDir = %q, want %q", flags.projectDir, tt.wantProjectDir)
			}
		})
	}
}

func TestIdentifyMissingContext(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantIssues  []string
	}{
		{
			name:        "empty description",
			description: "",
			wantIssues:  []string{"No acceptance criteria defined"},
		},
		{
			name:        "vague table reference",
			description: "Update the table to add a new column",
			wantIssues:  []string{"Which specific table/model?", "No acceptance criteria defined"},
		},
		{
			name:        "vague api reference",
			description: "Call the API to get user data",
			wantIssues:  []string{"Which API endpoint?", "No acceptance criteria defined"},
		},
		{
			name:        "todo item",
			description: "Implement feature TODO: decide on approach",
			wantIssues:  []string{"Unresolved TODO items", "No acceptance criteria defined"},
		},
		{
			name:        "detailed with acceptance",
			description: "When the user clicks submit, the form should validate all fields and show errors. This is a longer description with more details about the feature.",
			wantIssues:  nil, // No issues for well-defined description (over 100 chars with acceptance keywords)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := identifyMissingContext(tt.description)

			if len(tt.wantIssues) == 0 {
				if len(issues) != 0 {
					t.Errorf("expected no issues, got %v", issues)
				}
				return
			}

			for _, want := range tt.wantIssues {
				found := false
				for _, got := range issues {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected issue %q not found in %v", want, issues)
				}
			}
		})
	}
}

func TestIsCommonFalsePositive(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{"example.com/foo.go", true},
		{"localhost:3000", true},
		{"https://github.com/user/repo", true},
		{"v1.0.0", true},
		{"ab", true}, // too short
		{"app/main.go", false},
		{"internal/config/config.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := isCommonFalsePositive(tt.ref)
			if got != tt.want {
				t.Errorf("isCommonFalsePositive(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestGenerateQuestions(t *testing.T) {
	tests := []struct {
		name            string
		description     string
		result          *AuditResult
		wantQuestionsIn []string
	}{
		{
			name:        "task without schedule",
			description: "Create a task to process orders",
			result: &AuditResult{
				HasAcceptance: false,
			},
			wantQuestionsIn: []string{
				"What is the execution schedule/frequency?",
				"What are the acceptance criteria for this feature?",
			},
		},
		{
			name:        "data storage without persistence details",
			description: "Store user preferences data",
			result: &AuditResult{
				HasAcceptance: true,
			},
			wantQuestionsIn: []string{
				"How should the data be persisted/cached?",
			},
		},
		{
			name:        "user feature without permissions",
			description: "Allow user to delete their account",
			result: &AuditResult{
				HasAcceptance: true,
			},
			wantQuestionsIn: []string{
				"What permissions/roles should users have for this feature?",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			questions := generateQuestions(tt.description, tt.result)

			for _, want := range tt.wantQuestionsIn {
				found := false
				for _, got := range questions {
					if got == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected question %q not found in %v", want, questions)
				}
			}
		})
	}
}

func TestAuditBead(t *testing.T) {
	tests := []struct {
		name          string
		info          *BeadFullInfo
		wantReadiness string
	}{
		{
			name: "minimal bead - partial (few issues)",
			info: &BeadFullInfo{
				ID:          "test-123",
				Title:       "Do something",
				Description: "short",
			},
			wantReadiness: "Partial", // Brief description + no acceptance = 2 issues
		},
		{
			name: "well defined bead - ready",
			info: &BeadFullInfo{
				ID:    "test-456",
				Title: "Add user authentication",
				Description: `When a user visits the login page, they should be able to enter
their username and password. The system must validate the credentials and
redirect to the dashboard on success. If validation fails, show an error message.
The user should be able to reset their password via email.`,
			},
			wantReadiness: "Ready", // Has acceptance criteria keywords and detailed description
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := auditBead(tt.info, "")

			if result.Readiness != tt.wantReadiness {
				t.Errorf("Readiness = %q, want %q (issues: %d)", result.Readiness, tt.wantReadiness, result.IssueCount)
			}
		})
	}
}
