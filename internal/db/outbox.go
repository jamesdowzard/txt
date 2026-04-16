package db

import (
	"database/sql"
	"fmt"
	"time"
)

// OutboxStatus values stored in outbox.status.
const (
	OutboxStatusPending = "pending"
	OutboxStatusSent    = "sent"
	OutboxStatusFailed  = "failed"
)

// OutboxItem is a deferred message awaiting dispatch.
type OutboxItem struct {
	ID             int64  `json:"id"`
	ConversationID string `json:"conversation_id"`
	Body           string `json:"body"`
	SendAt         int64  `json:"send_at"` // unix seconds
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
	Attempts       int    `json:"attempts"`
	CreatedAt      int64  `json:"created_at"`
	SentMessageID  string `json:"sent_message_id,omitempty"`
}

// migrateOutbox is invoked from migrate(); idempotent.
func (s *Store) migrateOutbox() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			send_at INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			error TEXT NOT NULL DEFAULT '',
			attempts INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			sent_message_id TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_outbox_due ON outbox(status, send_at);
	`)
	return err
}

func (s *Store) CreateOutboxItem(item *OutboxItem) (int64, error) {
	if item.ConversationID == "" {
		return 0, fmt.Errorf("conversation_id is required")
	}
	if item.SendAt <= 0 {
		return 0, fmt.Errorf("send_at must be a future unix timestamp")
	}
	if item.Status == "" {
		item.Status = OutboxStatusPending
	}
	now := time.Now().Unix()
	if item.CreatedAt == 0 {
		item.CreatedAt = now
	}
	res, err := s.db.Exec(`
		INSERT INTO outbox (conversation_id, body, send_at, status, attempts, created_at)
		VALUES (?, ?, ?, ?, 0, ?)
	`, item.ConversationID, item.Body, item.SendAt, item.Status, item.CreatedAt)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	item.ID = id
	return id, nil
}

func (s *Store) ListOutboxItems(status string, limit int) ([]*OutboxItem, error) {
	if limit <= 0 {
		limit = 200
	}
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = s.db.Query(`
			SELECT id, conversation_id, body, send_at, status, error, attempts, created_at, sent_message_id
			FROM outbox ORDER BY send_at ASC LIMIT ?
		`, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT id, conversation_id, body, send_at, status, error, attempts, created_at, sent_message_id
			FROM outbox WHERE status = ? ORDER BY send_at ASC LIMIT ?
		`, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*OutboxItem
	for rows.Next() {
		it := &OutboxItem{}
		if err := rows.Scan(&it.ID, &it.ConversationID, &it.Body, &it.SendAt, &it.Status, &it.Error, &it.Attempts, &it.CreatedAt, &it.SentMessageID); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// ListDueOutboxItems returns pending items whose send_at <= now.
func (s *Store) ListDueOutboxItems() ([]*OutboxItem, error) {
	now := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT id, conversation_id, body, send_at, status, error, attempts, created_at, sent_message_id
		FROM outbox WHERE status = 'pending' AND send_at <= ?
		ORDER BY send_at ASC
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*OutboxItem
	for rows.Next() {
		it := &OutboxItem{}
		if err := rows.Scan(&it.ID, &it.ConversationID, &it.Body, &it.SendAt, &it.Status, &it.Error, &it.Attempts, &it.CreatedAt, &it.SentMessageID); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Store) MarkOutboxSent(id int64, messageID string) error {
	_, err := s.db.Exec(`UPDATE outbox SET status = ?, sent_message_id = ?, error = '' WHERE id = ?`, OutboxStatusSent, messageID, id)
	return err
}

func (s *Store) MarkOutboxFailed(id int64, errMsg string) error {
	_, err := s.db.Exec(`UPDATE outbox SET status = ?, error = ?, attempts = attempts + 1 WHERE id = ?`, OutboxStatusFailed, errMsg, id)
	return err
}

func (s *Store) IncrementOutboxAttempts(id int64, errMsg string) error {
	_, err := s.db.Exec(`UPDATE outbox SET attempts = attempts + 1, error = ? WHERE id = ?`, errMsg, id)
	return err
}

func (s *Store) DeleteOutboxItem(id int64) error {
	_, err := s.db.Exec(`DELETE FROM outbox WHERE id = ?`, id)
	return err
}
