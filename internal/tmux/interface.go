package tmux

import "time"

// Runner defines the interface for tmux operations.
// This allows mocking in tests.
type Runner interface {
	NewSession(name, workdir, beadsDir, editorCmd string, opts *SessionOptions) error
	Attach(name string) error
	SwitchClient(name string) error
	NudgeSession(session, message string) error
	WaitForClaude(session string, timeout time.Duration) error
	Kill(name string) error
	SessionExists(name string) bool
	ListSessions() ([]string, error)
}

// DefaultRunner implements Runner using actual tmux commands.
type DefaultRunner struct{}

func (r *DefaultRunner) NewSession(name, workdir, beadsDir, editorCmd string, opts *SessionOptions) error {
	return NewSession(name, workdir, beadsDir, editorCmd, opts)
}

func (r *DefaultRunner) Attach(name string) error {
	return Attach(name)
}

func (r *DefaultRunner) SwitchClient(name string) error {
	return SwitchClient(name)
}

func (r *DefaultRunner) NudgeSession(session, message string) error {
	return NudgeSession(session, message)
}

func (r *DefaultRunner) WaitForClaude(session string, timeout time.Duration) error {
	return WaitForClaude(session, timeout)
}

func (r *DefaultRunner) Kill(name string) error {
	return Kill(name)
}

func (r *DefaultRunner) SessionExists(name string) bool {
	return SessionExists(name)
}

func (r *DefaultRunner) ListSessions() ([]string, error) {
	return ListSessions()
}
