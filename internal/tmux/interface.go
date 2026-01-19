package tmux

// Runner defines the interface for tmux operations.
// This allows mocking in tests.
type Runner interface {
	NewSession(name, workdir, beadsDir, editorCmd string) error
	Attach(name string) error
	Kill(name string) error
	SessionExists(name string) bool
	ListSessions() ([]string, error)
}

// DefaultRunner implements Runner using actual tmux commands.
type DefaultRunner struct{}

func (r *DefaultRunner) NewSession(name, workdir, beadsDir, editorCmd string) error {
	return NewSession(name, workdir, beadsDir, editorCmd)
}

func (r *DefaultRunner) Attach(name string) error {
	return Attach(name)
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
