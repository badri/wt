package testenv

import (
	"testing"

	"github.com/badri/wt/internal/project"
)

func TestAllocatePortOffset_NilProject(t *testing.T) {
	offset := AllocatePortOffset(nil, nil)
	if offset != 0 {
		t.Errorf("expected 0 for nil project, got %d", offset)
	}
}

func TestAllocatePortOffset_NoTestEnv(t *testing.T) {
	proj := &project.Project{Name: "test"}
	offset := AllocatePortOffset(proj, nil)
	if offset != 0 {
		t.Errorf("expected 0 for project without test env, got %d", offset)
	}
}

func TestAllocatePortOffset_FirstAllocation(t *testing.T) {
	proj := &project.Project{
		Name:    "test",
		TestEnv: &project.TestEnv{},
	}
	offset := AllocatePortOffset(proj, nil)
	if offset != DefaultPortBase {
		t.Errorf("expected %d for first allocation, got %d", DefaultPortBase, offset)
	}
}

func TestAllocatePortOffset_SkipsUsed(t *testing.T) {
	proj := &project.Project{
		Name:    "test",
		TestEnv: &project.TestEnv{},
	}

	used := []int{1000, 1100, 1200}
	offset := AllocatePortOffset(proj, used)
	if offset != 1300 {
		t.Errorf("expected 1300 (next available), got %d", offset)
	}
}

func TestAllocatePortOffset_SkipsGaps(t *testing.T) {
	proj := &project.Project{
		Name:    "test",
		TestEnv: &project.TestEnv{},
	}

	// Used: 1000, 1200 (gap at 1100)
	used := []int{1000, 1200}
	offset := AllocatePortOffset(proj, used)
	// Should find 1100 as the first available
	if offset != 1100 {
		t.Errorf("expected 1100 (first gap), got %d", offset)
	}
}

func TestRunSetup_NilProject(t *testing.T) {
	err := RunSetup(nil, "/tmp", 1000)
	if err != nil {
		t.Errorf("expected nil error for nil project, got %v", err)
	}
}

func TestRunSetup_NoTestEnv(t *testing.T) {
	proj := &project.Project{Name: "test"}
	err := RunSetup(proj, "/tmp", 1000)
	if err != nil {
		t.Errorf("expected nil error for project without test env, got %v", err)
	}
}

func TestRunSetup_NoSetupCmd(t *testing.T) {
	proj := &project.Project{
		Name:    "test",
		TestEnv: &project.TestEnv{},
	}
	err := RunSetup(proj, "/tmp", 1000)
	if err != nil {
		t.Errorf("expected nil error for empty setup cmd, got %v", err)
	}
}

func TestRunSetup_Success(t *testing.T) {
	proj := &project.Project{
		Name: "test",
		TestEnv: &project.TestEnv{
			Setup: "true", // Always succeeds
		},
	}
	err := RunSetup(proj, "/tmp", 1000)
	if err != nil {
		t.Errorf("expected nil error for 'true' command, got %v", err)
	}
}

func TestRunSetup_Failure(t *testing.T) {
	proj := &project.Project{
		Name: "test",
		TestEnv: &project.TestEnv{
			Setup: "false", // Always fails
		},
	}
	err := RunSetup(proj, "/tmp", 1000)
	if err == nil {
		t.Error("expected error for 'false' command")
	}
}

func TestRunTeardown_NilProject(t *testing.T) {
	err := RunTeardown(nil, "/tmp", 1000)
	if err != nil {
		t.Errorf("expected nil error for nil project, got %v", err)
	}
}

func TestRunTeardown_Success(t *testing.T) {
	proj := &project.Project{
		Name: "test",
		TestEnv: &project.TestEnv{
			Teardown: "true",
		},
	}
	err := RunTeardown(proj, "/tmp", 1000)
	if err != nil {
		t.Errorf("expected nil error for 'true' command, got %v", err)
	}
}

func TestRunOnCreateHooks_NilProject(t *testing.T) {
	err := RunOnCreateHooks(nil, "/tmp", 1000, "")
	if err != nil {
		t.Errorf("expected nil error for nil project, got %v", err)
	}
}

func TestRunOnCreateHooks_NoHooks(t *testing.T) {
	proj := &project.Project{Name: "test"}
	err := RunOnCreateHooks(proj, "/tmp", 1000, "")
	if err != nil {
		t.Errorf("expected nil error for project without hooks, got %v", err)
	}
}

func TestRunOnCreateHooks_Success(t *testing.T) {
	proj := &project.Project{
		Name: "test",
		Hooks: &project.Hooks{
			OnCreate: []string{"true", "true"},
		},
	}
	err := RunOnCreateHooks(proj, "/tmp", 1000, "")
	if err != nil {
		t.Errorf("expected nil error for 'true' hooks, got %v", err)
	}
}

func TestRunOnCreateHooks_Failure(t *testing.T) {
	proj := &project.Project{
		Name: "test",
		Hooks: &project.Hooks{
			OnCreate: []string{"true", "false"},
		},
	}
	err := RunOnCreateHooks(proj, "/tmp", 1000, "")
	if err == nil {
		t.Error("expected error when hook fails")
	}
}

func TestRunOnCloseHooks_Success(t *testing.T) {
	proj := &project.Project{
		Name: "test",
		Hooks: &project.Hooks{
			OnClose: []string{"true"},
		},
	}
	err := RunOnCloseHooks(proj, "/tmp", 1000, "")
	if err != nil {
		t.Errorf("expected nil error for 'true' hook, got %v", err)
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		slice    []int
		val      int
		expected bool
	}{
		{[]int{1, 2, 3}, 2, true},
		{[]int{1, 2, 3}, 4, false},
		{[]int{}, 1, false},
		{nil, 1, false},
	}

	for _, tc := range tests {
		result := contains(tc.slice, tc.val)
		if result != tc.expected {
			t.Errorf("contains(%v, %d) = %v, expected %v", tc.slice, tc.val, result, tc.expected)
		}
	}
}
