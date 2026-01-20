package tmux

import "fmt"

// MockRunner is a mock implementation of Runner for testing.
type MockRunner struct {
	Sessions          map[string]MockSession
	NewSessionErr     error
	AttachErr         error
	SwitchClientErr   error
	KillErr           error
	ListErr           error
	AttachCalled      bool
	AttachedTo        string
	SwitchClientCalled bool
	SwitchClientTo    string
	KillCalled        bool
	KilledSession     string
}

// MockSession represents a mock tmux session.
type MockSession struct {
	Name       string
	Workdir    string
	BeadsDir   string
	EditorCmd  string
	PortOffset int
	PortEnv    string
}

// NewMockRunner creates a new MockRunner with an empty session map.
func NewMockRunner() *MockRunner {
	return &MockRunner{
		Sessions: make(map[string]MockSession),
	}
}

func (m *MockRunner) NewSession(name, workdir, beadsDir, editorCmd string, opts *SessionOptions) error {
	if m.NewSessionErr != nil {
		return m.NewSessionErr
	}

	if _, exists := m.Sessions[name]; exists {
		return fmt.Errorf("tmux session '%s' already exists", name)
	}

	sess := MockSession{
		Name:      name,
		Workdir:   workdir,
		BeadsDir:  beadsDir,
		EditorCmd: editorCmd,
	}
	if opts != nil {
		sess.PortOffset = opts.PortOffset
		sess.PortEnv = opts.PortEnv
	}
	m.Sessions[name] = sess
	return nil
}

func (m *MockRunner) Attach(name string) error {
	m.AttachCalled = true
	m.AttachedTo = name

	if m.AttachErr != nil {
		return m.AttachErr
	}

	if _, exists := m.Sessions[name]; !exists {
		return fmt.Errorf("session '%s' not found", name)
	}
	return nil
}

func (m *MockRunner) SwitchClient(name string) error {
	m.SwitchClientCalled = true
	m.SwitchClientTo = name

	if m.SwitchClientErr != nil {
		return m.SwitchClientErr
	}

	if _, exists := m.Sessions[name]; !exists {
		return fmt.Errorf("session '%s' not found", name)
	}
	return nil
}

func (m *MockRunner) Kill(name string) error {
	m.KillCalled = true
	m.KilledSession = name

	if m.KillErr != nil {
		return m.KillErr
	}

	if _, exists := m.Sessions[name]; !exists {
		return fmt.Errorf("session '%s' not found", name)
	}
	delete(m.Sessions, name)
	return nil
}

func (m *MockRunner) SessionExists(name string) bool {
	_, exists := m.Sessions[name]
	return exists
}

func (m *MockRunner) ListSessions() ([]string, error) {
	if m.ListErr != nil {
		return nil, m.ListErr
	}

	names := make([]string, 0, len(m.Sessions))
	for name := range m.Sessions {
		names = append(names, name)
	}
	return names, nil
}

// AddSession adds a session to the mock (for test setup).
func (m *MockRunner) AddSession(name, workdir, beadsDir string) {
	m.Sessions[name] = MockSession{
		Name:     name,
		Workdir:  workdir,
		BeadsDir: beadsDir,
	}
}
