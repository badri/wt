package testenv

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/badri/wt/internal/project"
)

// DefaultPortBase is the default starting port offset
const DefaultPortBase = 1000

// DefaultPortStep is the default increment between port offsets
const DefaultPortStep = 100

// AllocatePortOffset finds the next available port offset given used offsets.
// Returns 0 if test env is not enabled.
func AllocatePortOffset(proj *project.Project, usedOffsets []int) int {
	if proj == nil || proj.TestEnv == nil {
		return 0
	}

	base := DefaultPortBase
	step := DefaultPortStep

	// Find next available offset
	offset := base
	for contains(usedOffsets, offset) {
		offset += step
	}

	return offset
}

// RunSetup executes the setup command if configured.
func RunSetup(proj *project.Project, workdir string, portOffset int) error {
	if proj == nil || proj.TestEnv == nil || proj.TestEnv.Setup == "" {
		return nil
	}

	return runHook(proj.TestEnv.Setup, workdir, portOffset, proj.TestEnv.PortEnv)
}

// RunTeardown executes the teardown command if configured.
func RunTeardown(proj *project.Project, workdir string, portOffset int) error {
	if proj == nil || proj.TestEnv == nil || proj.TestEnv.Teardown == "" {
		return nil
	}

	return runHook(proj.TestEnv.Teardown, workdir, portOffset, proj.TestEnv.PortEnv)
}

// WaitForHealthy runs the health check command until it succeeds or times out.
func WaitForHealthy(proj *project.Project, workdir string, portOffset int, timeout time.Duration) error {
	if proj == nil || proj.TestEnv == nil || proj.TestEnv.HealthCheck == "" {
		return nil
	}

	if timeout == 0 {
		timeout = 30 * time.Second
	}

	deadline := time.Now().Add(timeout)
	interval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		err := runHook(proj.TestEnv.HealthCheck, workdir, portOffset, proj.TestEnv.PortEnv)
		if err == nil {
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("health check timed out after %v", timeout)
}

// runHook executes a shell command with PORT_OFFSET (or custom env var) set.
func runHook(command, workdir string, portOffset int, portEnv string) error {
	if portEnv == "" {
		portEnv = "PORT_OFFSET"
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", portEnv, portOffset))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RunOnCreateHooks executes project hooks configured for session creation.
func RunOnCreateHooks(proj *project.Project, workdir string, portOffset int, portEnv string) error {
	if proj == nil || proj.Hooks == nil {
		return nil
	}

	if portEnv == "" {
		portEnv = "PORT_OFFSET"
	}

	for _, hook := range proj.Hooks.OnCreate {
		cmd := exec.Command("sh", "-c", hook)
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", portEnv, portOffset))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("on_create hook failed: %w", err)
		}
	}

	return nil
}

// RunOnCloseHooks executes project hooks configured for session close.
func RunOnCloseHooks(proj *project.Project, workdir string, portOffset int, portEnv string) error {
	if proj == nil || proj.Hooks == nil {
		return nil
	}

	if portEnv == "" {
		portEnv = "PORT_OFFSET"
	}

	for _, hook := range proj.Hooks.OnClose {
		cmd := exec.Command("sh", "-c", hook)
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", portEnv, portOffset))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("on_close hook failed: %w", err)
		}
	}

	return nil
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}
