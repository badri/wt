package msg

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenCreatesDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "msg.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("DB file not created: %v", err)
	}
}

func TestSendAndRecv(t *testing.T) {
	s := tempStore(t)

	id, err := s.Send(&Message{
		Subject:  SubjectTask,
		From:     "orchestrator",
		To:       "worker-1",
		Body:     "do the thing",
		ThreadID: "epic-1",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	msgs, err := s.Recv("worker-1")
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	m := msgs[0]
	if m.Subject != SubjectTask {
		t.Errorf("subject = %q, want %q", m.Subject, SubjectTask)
	}
	if m.From != "orchestrator" {
		t.Errorf("from = %q, want %q", m.From, "orchestrator")
	}
	if m.Body != "do the thing" {
		t.Errorf("body = %q, want %q", m.Body, "do the thing")
	}
	if m.AckedAt != nil {
		t.Error("expected nil AckedAt before ack")
	}
}

func TestRecvOnlyShowsUnacked(t *testing.T) {
	s := tempStore(t)

	id, _ := s.Send(&Message{Subject: SubjectTask, From: "a", To: "b", Body: "1"})
	s.Send(&Message{Subject: SubjectDone, From: "a", To: "b", Body: "2"})

	// Ack first message
	if err := s.Ack(id); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	msgs, _ := s.Recv("b")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 unacked message, got %d", len(msgs))
	}
	if msgs[0].Body != "2" {
		t.Errorf("expected body '2', got %q", msgs[0].Body)
	}
}

func TestRecvFiltersRecipient(t *testing.T) {
	s := tempStore(t)

	s.Send(&Message{Subject: SubjectTask, From: "a", To: "worker-1", Body: "for w1"})
	s.Send(&Message{Subject: SubjectTask, From: "a", To: "worker-2", Body: "for w2"})

	msgs, _ := s.Recv("worker-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for worker-1, got %d", len(msgs))
	}
	if msgs[0].Body != "for w1" {
		t.Errorf("wrong message body: %q", msgs[0].Body)
	}
}

func TestAckAlreadyAcked(t *testing.T) {
	s := tempStore(t)

	id, _ := s.Send(&Message{Subject: SubjectTask, From: "a", To: "b"})
	s.Ack(id)

	err := s.Ack(id)
	if err == nil {
		t.Fatal("expected error acking already-acked message")
	}
}

func TestAckNonexistent(t *testing.T) {
	s := tempStore(t)
	err := s.Ack(9999)
	if err == nil {
		t.Fatal("expected error acking nonexistent message")
	}
}

func TestListByThread(t *testing.T) {
	s := tempStore(t)

	s.Send(&Message{Subject: SubjectTask, From: "a", To: "b", ThreadID: "t1", Body: "1"})
	s.Send(&Message{Subject: SubjectDone, From: "b", To: "a", ThreadID: "t1", Body: "2"})
	s.Send(&Message{Subject: SubjectTask, From: "a", To: "c", ThreadID: "t2", Body: "3"})

	msgs, _ := s.List("t1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in thread t1, got %d", len(msgs))
	}

	all, _ := s.List("")
	if len(all) != 3 {
		t.Fatalf("expected 3 total messages, got %d", len(all))
	}
}

func TestUnacked(t *testing.T) {
	s := tempStore(t)

	id1, _ := s.Send(&Message{Subject: SubjectTask, From: "a", To: "b"})
	s.Send(&Message{Subject: SubjectDone, From: "b", To: "a"})
	s.Ack(id1)

	msgs, _ := s.Unacked()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 unacked message, got %d", len(msgs))
	}
	if msgs[0].Subject != SubjectDone {
		t.Errorf("expected DONE, got %q", msgs[0].Subject)
	}
}

func TestAckedMessageHasTimestamp(t *testing.T) {
	s := tempStore(t)

	id, _ := s.Send(&Message{Subject: SubjectTask, From: "a", To: "b"})
	s.Ack(id)

	msgs, _ := s.List("")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].AckedAt == nil {
		t.Error("expected AckedAt to be set after ack")
	}
}
