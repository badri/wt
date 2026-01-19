package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func NewSession(name, workdir, beadsDir, editorCmd string) error {
	// Check if session already exists
	if SessionExists(name) {
		return fmt.Errorf("tmux session '%s' already exists", name)
	}

	// Create tmux session in detached mode
	// Set BEADS_DIR environment variable and start the editor
	cmd := exec.Command("tmux", "new-session",
		"-d",           // detached
		"-s", name,     // session name
		"-c", workdir,  // working directory
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("BEADS_DIR=%s", beadsDir))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set BEADS_DIR as an environment variable in the session
	setEnvCmd := exec.Command("tmux", "set-environment", "-t", name, "BEADS_DIR", beadsDir)
	if err := setEnvCmd.Run(); err != nil {
		// Non-fatal, but log it
		fmt.Printf("Warning: could not set BEADS_DIR in tmux session: %v\n", err)
	}

	// Send the editor command to start
	if editorCmd != "" {
		sendCmd := exec.Command("tmux", "send-keys", "-t", name, editorCmd, "Enter")
		if err := sendCmd.Run(); err != nil {
			return fmt.Errorf("starting editor: %w", err)
		}
	}

	return nil
}

func Attach(name string) error {
	// Check if we're inside tmux
	if os.Getenv("TMUX") != "" {
		// Switch to the session
		cmd := exec.Command("tmux", "switch-client", "-t", name)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Attach to the session
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Kill(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	return cmd.Run()
}

func SessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

func ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// No sessions is not an error
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var sessions []string
	for _, line := range lines {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}
