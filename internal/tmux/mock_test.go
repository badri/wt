package tmux

import (
	"errors"
	"testing"
)

func TestMockRunner_NewSession(t *testing.T) {
	mock := NewMockRunner()

	err := mock.NewSession("test", "/tmp/workdir", "/tmp/beads", "claude", nil)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	if !mock.SessionExists("test") {
		t.Error("session should exist after creation")
	}

	sess := mock.Sessions["test"]
	if sess.Workdir != "/tmp/workdir" {
		t.Errorf("expected workdir '/tmp/workdir', got %q", sess.Workdir)
	}
	if sess.BeadsDir != "/tmp/beads" {
		t.Errorf("expected beadsDir '/tmp/beads', got %q", sess.BeadsDir)
	}
}

func TestMockRunner_NewSession_WithPortOffset(t *testing.T) {
	mock := NewMockRunner()

	opts := &SessionOptions{PortOffset: 1000, PortEnv: "TEST_PORT"}
	err := mock.NewSession("test", "/tmp/workdir", "/tmp/beads", "claude", opts)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	sess := mock.Sessions["test"]
	if sess.PortOffset != 1000 {
		t.Errorf("expected PortOffset 1000, got %d", sess.PortOffset)
	}
	if sess.PortEnv != "TEST_PORT" {
		t.Errorf("expected PortEnv 'TEST_PORT', got %q", sess.PortEnv)
	}
}

func TestMockRunner_NewSession_DuplicateError(t *testing.T) {
	mock := NewMockRunner()

	mock.NewSession("test", "/tmp", "/tmp", "", nil)
	err := mock.NewSession("test", "/tmp", "/tmp", "", nil)

	if err == nil {
		t.Error("expected error for duplicate session")
	}
}

func TestMockRunner_NewSession_ConfiguredError(t *testing.T) {
	mock := NewMockRunner()
	mock.NewSessionErr = errors.New("forced error")

	err := mock.NewSession("test", "/tmp", "/tmp", "", nil)
	if err == nil {
		t.Error("expected configured error")
	}
}

func TestMockRunner_Attach(t *testing.T) {
	mock := NewMockRunner()
	mock.AddSession("test", "/tmp", "/tmp")

	err := mock.Attach("test")
	if err != nil {
		t.Fatalf("Attach failed: %v", err)
	}

	if !mock.AttachCalled {
		t.Error("AttachCalled should be true")
	}
	if mock.AttachedTo != "test" {
		t.Errorf("expected AttachedTo 'test', got %q", mock.AttachedTo)
	}
}

func TestMockRunner_Attach_NotFound(t *testing.T) {
	mock := NewMockRunner()

	err := mock.Attach("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestMockRunner_SwitchClient(t *testing.T) {
	mock := NewMockRunner()
	mock.AddSession("test", "/tmp", "/tmp")

	err := mock.SwitchClient("test")
	if err != nil {
		t.Fatalf("SwitchClient failed: %v", err)
	}

	if !mock.SwitchClientCalled {
		t.Error("SwitchClientCalled should be true")
	}
	if mock.SwitchClientTo != "test" {
		t.Errorf("expected SwitchClientTo 'test', got %q", mock.SwitchClientTo)
	}
}

func TestMockRunner_SwitchClient_NotFound(t *testing.T) {
	mock := NewMockRunner()

	err := mock.SwitchClient("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestMockRunner_SwitchClient_ConfiguredError(t *testing.T) {
	mock := NewMockRunner()
	mock.AddSession("test", "/tmp", "/tmp")
	mock.SwitchClientErr = errors.New("forced error")

	err := mock.SwitchClient("test")
	if err == nil {
		t.Error("expected configured error")
	}
}

func TestMockRunner_Kill(t *testing.T) {
	mock := NewMockRunner()
	mock.AddSession("test", "/tmp", "/tmp")

	err := mock.Kill("test")
	if err != nil {
		t.Fatalf("Kill failed: %v", err)
	}

	if mock.SessionExists("test") {
		t.Error("session should not exist after kill")
	}
	if !mock.KillCalled {
		t.Error("KillCalled should be true")
	}
	if mock.KilledSession != "test" {
		t.Errorf("expected KilledSession 'test', got %q", mock.KilledSession)
	}
}

func TestMockRunner_ListSessions(t *testing.T) {
	mock := NewMockRunner()
	mock.AddSession("alpha", "/tmp", "/tmp")
	mock.AddSession("beta", "/tmp", "/tmp")

	sessions, err := mock.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestMockRunner_ListSessions_Empty(t *testing.T) {
	mock := NewMockRunner()

	sessions, err := mock.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestMockRunner_SessionExists(t *testing.T) {
	mock := NewMockRunner()

	if mock.SessionExists("test") {
		t.Error("session should not exist initially")
	}

	mock.AddSession("test", "/tmp", "/tmp")

	if !mock.SessionExists("test") {
		t.Error("session should exist after adding")
	}
}

// Verify MockRunner implements Runner interface
var _ Runner = (*MockRunner)(nil)
