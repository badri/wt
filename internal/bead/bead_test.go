package bead

import (
	"strings"
	"testing"
)

func TestExtractProject(t *testing.T) {
	tests := []struct {
		beadID   string
		expected string
	}{
		{"wt-abc", "wt"},
		{"beads-xyz", "beads"},
		{"my-project-123", "my-project"},
		{"simple", "simple"},
		{"a-b-c-d", "a-b-c"},
	}

	for _, tc := range tests {
		result := ExtractProject(tc.beadID)
		if result != tc.expected {
			t.Errorf("ExtractProject(%s) = %s, want %s", tc.beadID, result, tc.expected)
		}
	}
}

func TestSearchFallback(t *testing.T) {
	// searchFallback filters by title, but it depends on List which requires bd
	// Just test that the function doesn't panic with empty input
	results, err := searchFallback("nonexistent-query-that-wont-match")
	if err != nil {
		// This is expected if bd is not available
		t.Skipf("skipping searchFallback test: bd not available: %v", err)
	}

	// Results should be empty or contain only matching beads
	for _, b := range results {
		if !strings.Contains(strings.ToLower(b.Title), "nonexistent-query-that-wont-match") {
			t.Errorf("searchFallback returned non-matching bead: %s", b.Title)
		}
	}
}

func TestBeadInfoStruct(t *testing.T) {
	info := &BeadInfo{
		ID:      "test-123",
		Title:   "Test Bead",
		Status:  "open",
		Project: "test",
	}

	if info.ID != "test-123" {
		t.Errorf("unexpected ID: %s", info.ID)
	}
	if info.Title != "Test Bead" {
		t.Errorf("unexpected Title: %s", info.Title)
	}
	if info.Status != "open" {
		t.Errorf("unexpected Status: %s", info.Status)
	}
	if info.Project != "test" {
		t.Errorf("unexpected Project: %s", info.Project)
	}
}

func TestReadyBeadStruct(t *testing.T) {
	bead := &ReadyBead{
		ID:          "test-456",
		Title:       "Test Ready Bead",
		Description: "A test description",
		Status:      "open",
		Priority:    1,
		IssueType:   "task",
	}

	if bead.ID != "test-456" {
		t.Errorf("unexpected ID: %s", bead.ID)
	}
	if bead.Title != "Test Ready Bead" {
		t.Errorf("unexpected Title: %s", bead.Title)
	}
	if bead.Description != "A test description" {
		t.Errorf("unexpected Description: %s", bead.Description)
	}
	if bead.Status != "open" {
		t.Errorf("unexpected Status: %s", bead.Status)
	}
	if bead.Priority != 1 {
		t.Errorf("unexpected Priority: %d", bead.Priority)
	}
	if bead.IssueType != "task" {
		t.Errorf("unexpected IssueType: %s", bead.IssueType)
	}
}

func TestCreateOptionsStruct(t *testing.T) {
	opts := &CreateOptions{
		Description: "Test description",
		Priority:    2,
		Type:        "feature",
	}

	if opts.Description != "Test description" {
		t.Errorf("unexpected Description: %s", opts.Description)
	}
	if opts.Priority != 2 {
		t.Errorf("unexpected Priority: %d", opts.Priority)
	}
	if opts.Type != "feature" {
		t.Errorf("unexpected Type: %s", opts.Type)
	}
}

func TestBeadInfoFullStruct(t *testing.T) {
	info := &BeadInfoFull{
		ID:          "test-789",
		Title:       "Full Info Test",
		Status:      "in_progress",
		Project:     "test-project",
		Description: "Full description here",
		Priority:    0,
		IssueType:   "bug",
	}

	if info.ID != "test-789" {
		t.Errorf("unexpected ID: %s", info.ID)
	}
	if info.Title != "Full Info Test" {
		t.Errorf("unexpected Title: %s", info.Title)
	}
	if info.Status != "in_progress" {
		t.Errorf("unexpected Status: %s", info.Status)
	}
	if info.Project != "test-project" {
		t.Errorf("unexpected Project: %s", info.Project)
	}
	if info.Description != "Full description here" {
		t.Errorf("unexpected Description: %s", info.Description)
	}
	if info.Priority != 0 {
		t.Errorf("unexpected Priority: %d", info.Priority)
	}
	if info.IssueType != "bug" {
		t.Errorf("unexpected IssueType: %s", info.IssueType)
	}
}
