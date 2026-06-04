package store

import (
	"fmt"
	"time"

	"ai-interviewer/internal/models"
)

// CreateSession inserts a new session row and returns the populated struct.
func (db *DB) CreateSession(id, problemID, model string) (models.Session, error) {
	now := time.Now().UTC()
	_, err := db.conn.Exec(
		`INSERT INTO sessions (id, problem_id, model, started_at) VALUES (?, ?, ?, ?)`,
		id, problemID, model, now,
	)
	if err != nil {
		return models.Session{}, fmt.Errorf("store: create session: %w", err)
	}
	return models.Session{
		ID:        id,
		ProblemID: problemID,
		Model:     model,
		StartedAt: now,
	}, nil
}

// EndSession sets the ended_at timestamp on a session.
func (db *DB) EndSession(id string) error {
	now := time.Now().UTC()
	_, err := db.conn.Exec(`UPDATE sessions SET ended_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("store: end session: %w", err)
	}
	return nil
}

// ListSessions returns a summary of all sessions, newest first.
func (db *DB) ListSessions() ([]models.SessionSummary, error) {
	rows, err := db.conn.Query(`
		SELECT s.id, s.model, s.started_at, COUNT(m.id) AS msg_count
		FROM sessions s
		LEFT JOIN messages m ON m.session_id = s.id
		GROUP BY s.id
		ORDER BY s.started_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer rows.Close()

	var out []models.SessionSummary
	for rows.Next() {
		var s models.SessionSummary
		var startedAt string
		if err := rows.Scan(&s.ID, &s.Model, &startedAt, &s.MessageCount); err != nil {
			return nil, fmt.Errorf("store: scan session row: %w", err)
		}
		s.StartedAt, err = time.Parse(time.RFC3339, startedAt)
		if err != nil {
			// SQLite may store as a slightly different format; try the datetime format.
			s.StartedAt, _ = time.Parse("2006-01-02 15:04:05", startedAt)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// AddMessage persists a single conversation turn.
func (db *DB) AddMessage(msg models.Message) error {
	_, err := db.conn.Exec(
		`INSERT INTO messages (id, session_id, role, content, has_image, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.SessionID, msg.Role, msg.Content, msg.HasImage, msg.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("store: add message: %w", err)
	}
	return nil
}

// GetMessages returns all messages for a session in chronological order.
func (db *DB) GetMessages(sessionID string) ([]models.Message, error) {
	rows, err := db.conn.Query(
		`SELECT id, session_id, role, content, has_image, created_at
		 FROM messages WHERE session_id = ? ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: get messages: %w", err)
	}
	defer rows.Close()

	var out []models.Message
	for rows.Next() {
		var m models.Message
		var createdAt string
		var hasImage int
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &hasImage, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan message row: %w", err)
		}
		m.HasImage = hasImage == 1
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if m.CreatedAt.IsZero() {
			m.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
