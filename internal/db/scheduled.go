package db

import (
	"database/sql"
	"fmt"
	"time"
)

// ScheduledStatus values stored in scheduled_messages.status.
const (
	ScheduledStatusPending   = "pending"
	ScheduledStatusSent      = "sent"
	ScheduledStatusFailed    = "failed"
	ScheduledStatusCancelled = "cancelled"
)

// ScheduledMessage is a deferred message queued for future send.
type ScheduledMessage struct {
	ID             int64  `json:"id"`
	ConversationID string `json:"conversation_id"`
	Body           string `json:"body"`
	ReplyToID      string `json:"reply_to_id,omitempty"`
	ScheduledAt    int64  `json:"scheduled_at"` // unix milliseconds
	Status         string `json:"status"`
	SentMessageID  string `json:"sent_message_id,omitempty"`
	Error          string `json:"error,omitempty"`
	CreatedAt      int64  `json:"created_at"` // unix milliseconds
}

// migrateScheduled is invoked from migrate(); idempotent.
func (s *Store) migrateScheduled() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS scheduled_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			reply_to_id TEXT NOT NULL DEFAULT '',
			scheduled_at INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			sent_message_id TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_scheduled_status_at ON scheduled_messages(status, scheduled_at);
	`)
	return err
}

// CreateScheduledMessage inserts a new pending scheduled message and returns
// the populated row (with ID, Status, CreatedAt filled in).
func (s *Store) CreateScheduledMessage(item *ScheduledMessage) (*ScheduledMessage, error) {
	if item == nil {
		return nil, fmt.Errorf("scheduled message is nil")
	}
	if item.ConversationID == "" {
		return nil, fmt.Errorf("conversation_id is required")
	}
	if item.ScheduledAt <= 0 {
		return nil, fmt.Errorf("scheduled_at must be a future unix-ms timestamp")
	}
	if item.Status == "" {
		item.Status = ScheduledStatusPending
	}
	if item.CreatedAt == 0 {
		item.CreatedAt = time.Now().UnixMilli()
	}
	res, err := s.db.Exec(`
		INSERT INTO scheduled_messages (conversation_id, body, reply_to_id, scheduled_at, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.ConversationID, item.Body, item.ReplyToID, item.ScheduledAt, item.Status, item.CreatedAt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	item.ID = id
	return item, nil
}

// ListOutboxMessages returns pending + failed rows ordered by scheduled_at ASC.
// Sent/cancelled rows are excluded — the caller only cares about items still
// visible in the outbox UI.
func (s *Store) ListOutboxMessages() ([]*ScheduledMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, conversation_id, body, reply_to_id, scheduled_at, status, sent_message_id, error, created_at
		FROM scheduled_messages
		WHERE status IN (?, ?)
		ORDER BY scheduled_at ASC
	`, ScheduledStatusPending, ScheduledStatusFailed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledRows(rows)
}

// ListDueScheduledMessages returns pending items whose scheduled_at <= now (unix ms).
func (s *Store) ListDueScheduledMessages(nowMS int64) ([]*ScheduledMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, conversation_id, body, reply_to_id, scheduled_at, status, sent_message_id, error, created_at
		FROM scheduled_messages
		WHERE status = ? AND scheduled_at <= ?
		ORDER BY scheduled_at ASC
	`, ScheduledStatusPending, nowMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledRows(rows)
}

// GetScheduledMessage fetches a single row by ID, or nil if it doesn't exist.
func (s *Store) GetScheduledMessage(id int64) (*ScheduledMessage, error) {
	row := s.db.QueryRow(`
		SELECT id, conversation_id, body, reply_to_id, scheduled_at, status, sent_message_id, error, created_at
		FROM scheduled_messages WHERE id = ?
	`, id)
	it := &ScheduledMessage{}
	err := row.Scan(&it.ID, &it.ConversationID, &it.Body, &it.ReplyToID, &it.ScheduledAt, &it.Status, &it.SentMessageID, &it.Error, &it.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return it, nil
}

// MarkScheduledSent transitions a row to sent with the resulting message ID.
func (s *Store) MarkScheduledSent(id int64, messageID string) error {
	_, err := s.db.Exec(`
		UPDATE scheduled_messages SET status = ?, sent_message_id = ?, error = '' WHERE id = ?
	`, ScheduledStatusSent, messageID, id)
	return err
}

// MarkScheduledFailed transitions a row to failed with an error message.
func (s *Store) MarkScheduledFailed(id int64, errMsg string) error {
	_, err := s.db.Exec(`
		UPDATE scheduled_messages SET status = ?, error = ? WHERE id = ?
	`, ScheduledStatusFailed, errMsg, id)
	return err
}

// CancelScheduledMessage flips a pending row to cancelled. Returns whether a row
// was cancelled (false if the row didn't exist or wasn't pending anymore).
func (s *Store) CancelScheduledMessage(id int64) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE scheduled_messages SET status = ? WHERE id = ? AND status = ?
	`, ScheduledStatusCancelled, id, ScheduledStatusPending)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func scanScheduledRows(rows *sql.Rows) ([]*ScheduledMessage, error) {
	var items []*ScheduledMessage
	for rows.Next() {
		it := &ScheduledMessage{}
		if err := rows.Scan(&it.ID, &it.ConversationID, &it.Body, &it.ReplyToID, &it.ScheduledAt, &it.Status, &it.SentMessageID, &it.Error, &it.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
