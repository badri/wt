package main

import (
	"testing"

	"github.com/badri/wt/internal/project"
)

func TestParseNewFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantBeadID  string
		wantRepo    string
		wantName    string
		wantNoSwitch    bool
		wantForceSwitch bool
		wantNoTestEnv   bool
	}{
		{
			name:       "bead only",
			args:       []string{"test-bead"},
			wantBeadID: "test-bead",
		},
		{
			name:       "with repo",
			args:       []string{"test-bead", "--repo", "/path/to/repo"},
			wantBeadID: "test-bead",
			wantRepo:   "/path/to/repo",
		},
		{
			name:       "with name",
			args:       []string{"test-bead", "--name", "custom-name"},
			wantBeadID: "test-bead",
			wantName:   "custom-name",
		},
		{
			name:         "with no-switch",
			args:         []string{"test-bead", "--no-switch"},
			wantBeadID:   "test-bead",
			wantNoSwitch: true,
		},
		{
			name:            "with switch",
			args:            []string{"test-bead", "--switch"},
			wantBeadID:      "test-bead",
			wantForceSwitch: true,
		},
		{
			name:          "with no-test-env",
			args:          []string{"test-bead", "--no-test-env"},
			wantBeadID:    "test-bead",
			wantNoTestEnv: true,
		},
		{
			name:            "all flags",
			args:            []string{"test-bead", "--repo", "/repo", "--name", "myname", "--switch", "--no-test-env"},
			wantBeadID:      "test-bead",
			wantRepo:        "/repo",
			wantName:        "myname",
			wantForceSwitch: true,
			wantNoTestEnv:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadID, flags := parseNewFlags(tt.args)

			if beadID != tt.wantBeadID {
				t.Errorf("beadID = %q, want %q", beadID, tt.wantBeadID)
			}
			if flags.repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", flags.repo, tt.wantRepo)
			}
			if flags.name != tt.wantName {
				t.Errorf("name = %q, want %q", flags.name, tt.wantName)
			}
			if flags.noSwitch != tt.wantNoSwitch {
				t.Errorf("noSwitch = %v, want %v", flags.noSwitch, tt.wantNoSwitch)
			}
			if flags.forceSwitch != tt.wantForceSwitch {
				t.Errorf("forceSwitch = %v, want %v", flags.forceSwitch, tt.wantForceSwitch)
			}
			if flags.noTestEnv != tt.wantNoTestEnv {
				t.Errorf("noTestEnv = %v, want %v", flags.noTestEnv, tt.wantNoTestEnv)
			}
		})
	}
}

func TestBuildInitialPrompt(t *testing.T) {
	tests := []struct {
		name            string
		beadID          string
		title           string
		proj            *project.Project
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:   "no project - defaults to pr-review",
			beadID: "test-123",
			title:  "Fix the bug",
			proj:   nil,
			wantContains: []string{
				"Work on bead test-123: Fix the bug.",
				"Workflow:",
				"Implement the task",
				"Commit your changes",
				"Create a PR",
				"Do NOT run `wt done`",
			},
			wantNotContains: []string{
				"Run tests",
			},
		},
		{
			name:   "project with pr-review mode",
			beadID: "test-456",
			title:  "Add feature",
			proj: &project.Project{
				MergeMode: "pr-review",
			},
			wantContains: []string{
				"Work on bead test-456: Add feature.",
				"Create a PR",
				"ready for review",
				"Do NOT run `wt done`",
			},
			wantNotContains: []string{
				"Run tests",
			},
		},
		{
			name:   "project with direct mode",
			beadID: "test-789",
			title:  "Quick fix",
			proj: &project.Project{
				MergeMode: "direct",
			},
			wantContains: []string{
				"Push your changes",
				"work is complete",
				"Do NOT run `wt done`",
			},
		},
		{
			name:   "project with pr-auto mode",
			beadID: "test-abc",
			title:  "Auto merge feature",
			proj: &project.Project{
				MergeMode: "pr-auto",
			},
			wantContains: []string{
				"Create a PR",
				"cleanup after merge",
				"Do NOT run `wt done`",
			},
		},
		{
			name:   "project with test env",
			beadID: "test-def",
			title:  "Tested feature",
			proj: &project.Project{
				MergeMode: "pr-review",
				TestEnv: &project.TestEnv{
					Setup: "npm test",
				},
			},
			wantContains: []string{
				"Run tests and fix any failures",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildInitialPrompt(tt.beadID, tt.title, tt.proj)

			for _, want := range tt.wantContains {
				if !contains(result, want) {
					t.Errorf("result should contain %q, got %q", want, result)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if contains(result, notWant) {
					t.Errorf("result should NOT contain %q, got %q", notWant, result)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
