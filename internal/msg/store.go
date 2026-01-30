package msg

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Subject represents the type of message.
type Subject string

const (
	SubjectTask     Subject = "TASK"
	SubjectDone     Subject = "DONE"
	SubjectStuck    Subject = "STUCK"
	SubjectProgress Subject = "PROGRESS"
)

// Message represents a single message in the store.
type Message struct {
	ID        int64
	Subject   Subject
	From      string
	To        string
	Body      string
	ThreadID  string
	CreatedAt time.Time
	AckedAt   *time.Time
}

// Store is a SQLite-backed message store.
type Store struct {
	db *sql.DB
}

// Open opens or creates a message store at the given path.
func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subject TEXT NOT NULL,
		from_id TEXT NOT NULL,
		to_id TEXT NOT NULL,
		body TEXT,
		thread_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		acked_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_messages_to_unacked ON messages(to_id) WHERE acked_at IS NULL;
	CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages(thread_id);
	`
	_, err := db.Exec(schema)
	return err
}

// Send inserts a new message and returns its ID.
func (s *Store) Send(m *Message) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO messages (subject, from_id, to_id, body, thread_id) VALUES (?, ?, ?, ?, ?)`,
		string(m.Subject), m.From, m.To, m.Body, m.ThreadID,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting message: %w", err)
	}
	return res.LastInsertId()
}

// Recv returns all unacknowledged messages for the given recipient.
func (s *Store) Recv(to string) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, subject, from_id, to_id, body, thread_id, created_at, acked_at
		 FROM messages WHERE to_id = ? AND acked_at IS NULL ORDER BY id`,
		to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Ack acknowledges a message by ID.
func (s *Store) Ack(id int64) error {
	res, err := s.db.Exec(
		`UPDATE messages SET acked_at = CURRENT_TIMESTAMP WHERE id = ? AND acked_at IS NULL`,
		id,
	)
	if err != nil {
		return fmt.Errorf("acking message: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("message %d not found or already acked", id)
	}
	return nil
}

// List returns all messages in a thread, or all messages if threadID is empty.
func (s *Store) List(threadID string) ([]Message, error) {
	var rows *sql.Rows
	var err error
	if threadID != "" {
		rows, err = s.db.Query(
			`SELECT id, subject, from_id, to_id, body, thread_id, created_at, acked_at
			 FROM messages WHERE thread_id = ? ORDER BY id`,
			threadID,
		)
	} else {
		rows, err = s.db.Query(
			`SELECT id, subject, from_id, to_id, body, thread_id, created_at, acked_at
			 FROM messages ORDER BY id`,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Unacked returns all unacknowledged messages across all recipients.
// Useful for crash recovery — the orchestrator can find messages that were never processed.
func (s *Store) Unacked() ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, subject, from_id, to_id, body, thread_id, created_at, acked_at
		 FROM messages WHERE acked_at IS NULL ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying unacked messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Close closes the store.
func (s *Store) Close() error {
	return s.db.Close()
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var msgs []Message
	for rows.Next() {
		var m Message
		var subject string
		var acked sql.NullTime
		if err := rows.Scan(&m.ID, &subject, &m.From, &m.To, &m.Body, &m.ThreadID, &m.CreatedAt, &acked); err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		m.Subject = Subject(subject)
		if acked.Valid {
			m.AckedAt = &acked.Time
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
