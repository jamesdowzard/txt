package db

import "database/sql"

// conversationColumns is the canonical column list for SELECT queries on conversations.
const conversationColumns = `conversation_id, name, is_group, participants, last_message_ts, unread_count, source_platform`

func (s *Store) UpsertConversation(c *Conversation) error {
	if c.SourcePlatform == "" {
		c.SourcePlatform = "sms"
	}
	_, err := s.db.Exec(`
		INSERT INTO conversations (conversation_id, name, is_group, participants, last_message_ts, unread_count, source_platform)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET
			name=excluded.name,
			is_group=excluded.is_group,
			participants=excluded.participants,
			last_message_ts=excluded.last_message_ts,
			unread_count=excluded.unread_count,
			source_platform=excluded.source_platform
	`, c.ConversationID, c.Name, c.IsGroup, c.Participants, c.LastMessageTS, c.UnreadCount, c.SourcePlatform)
	return err
}

func (s *Store) GetConversation(id string) (*Conversation, error) {
	c := &Conversation{}
	err := s.db.QueryRow(`
		SELECT `+conversationColumns+`
		FROM conversations WHERE conversation_id = ?
	`, id).Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Store) UpdateConversationTimestamp(id string, ts int64) error {
	_, err := s.db.Exec(`UPDATE conversations SET last_message_ts = ? WHERE conversation_id = ?`, ts, id)
	return err
}

func (s *Store) BumpConversationTimestamp(id string, ts int64) error {
	_, err := s.db.Exec(`
		UPDATE conversations
		SET last_message_ts = CASE
			WHEN last_message_ts < ? THEN ?
			ELSE last_message_ts
		END
		WHERE conversation_id = ?
	`, ts, ts, id)
	return err
}

func (s *Store) MergeConversationIDs(sourceID, targetID string) error {
	if sourceID == "" || targetID == "" || sourceID == targetID {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	source, err := getConversationTx(tx, sourceID)
	if err != nil {
		return err
	}
	if source == nil {
		return tx.Commit()
	}
	target, err := getConversationTx(tx, targetID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE messages SET conversation_id = ? WHERE conversation_id = ?`, targetID, sourceID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE drafts SET conversation_id = ? WHERE conversation_id = ?`, targetID, sourceID); err != nil {
		return err
	}

	merged := mergeConversationRecords(source, target, targetID)
	if err := upsertConversationTx(tx, merged); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM conversations WHERE conversation_id = ?`, sourceID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) MarkConversationRead(id string) error {
	_, err := s.db.Exec(`UPDATE conversations SET unread_count = 0 WHERE conversation_id = ?`, id)
	return err
}

func (s *Store) ListConversations(limit int) ([]*Conversation, error) {
	rows, err := s.db.Query(`
		SELECT `+conversationColumns+`
		FROM conversations
		ORDER BY last_message_ts DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversations(rows)
}

// ConversationCount returns the total number of conversations, optionally filtered by source platform.
func (s *Store) ConversationCount(sourcePlatform string) (int, error) {
	var count int
	var err error
	if sourcePlatform != "" {
		err = s.db.QueryRow(`SELECT COUNT(*) FROM conversations WHERE source_platform = ?`, sourcePlatform).Scan(&count)
	} else {
		err = s.db.QueryRow(`SELECT COUNT(*) FROM conversations`).Scan(&count)
	}
	return count, err
}

// ListConversationsByPlatform lists conversations filtered by source platform.
func (s *Store) ListConversationsByPlatform(platform string, limit int) ([]*Conversation, error) {
	rows, err := s.db.Query(`
		SELECT `+conversationColumns+`
		FROM conversations
		WHERE source_platform = ?
		ORDER BY last_message_ts DESC
		LIMIT ?
	`, platform, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversations(rows)
}

func (s *Store) SearchConversationsByMetadata(query string, limit int) ([]*Conversation, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT `+conversationColumns+`
		FROM conversations
		WHERE name LIKE ?
			OR participants LIKE ?
			OR conversation_id IN (
				SELECT DISTINCT conversation_id
				FROM messages
				WHERE sender_name LIKE ? OR sender_number LIKE ?
			)
			OR conversation_id IN (
				SELECT DISTINCT m.conversation_id
				FROM messages m
				JOIN contacts c ON c.number = m.sender_number
				WHERE c.name LIKE ? OR c.number LIKE ?
			)
		ORDER BY last_message_ts DESC
		LIMIT ?
	`, "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversations(rows)
}

func scanConversations(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]*Conversation, error) {
	var convs []*Conversation
	for rows.Next() {
		c := &Conversation{}
		if err := rows.Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform); err != nil {
			return nil, err
		}
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

func getConversationTx(tx *sql.Tx, id string) (*Conversation, error) {
	c := &Conversation{}
	err := tx.QueryRow(`
		SELECT `+conversationColumns+`
		FROM conversations WHERE conversation_id = ?
	`, id).Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func upsertConversationTx(tx *sql.Tx, c *Conversation) error {
	if c.SourcePlatform == "" {
		c.SourcePlatform = "sms"
	}
	_, err := tx.Exec(`
		INSERT INTO conversations (conversation_id, name, is_group, participants, last_message_ts, unread_count, source_platform)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET
			name=excluded.name,
			is_group=excluded.is_group,
			participants=excluded.participants,
			last_message_ts=excluded.last_message_ts,
			unread_count=excluded.unread_count,
			source_platform=excluded.source_platform
	`, c.ConversationID, c.Name, c.IsGroup, c.Participants, c.LastMessageTS, c.UnreadCount, c.SourcePlatform)
	return err
}

func mergeConversationRecords(source, target *Conversation, targetID string) *Conversation {
	if target == nil {
		merged := *source
		merged.ConversationID = targetID
		return &merged
	}

	merged := *target
	if merged.Name == "" {
		merged.Name = source.Name
	}
	merged.IsGroup = merged.IsGroup || source.IsGroup
	if merged.Participants == "" || merged.Participants == "[]" {
		merged.Participants = source.Participants
	}
	if source.LastMessageTS > merged.LastMessageTS {
		merged.LastMessageTS = source.LastMessageTS
	}
	if source.UnreadCount > merged.UnreadCount {
		merged.UnreadCount = source.UnreadCount
	}
	if merged.SourcePlatform == "" {
		merged.SourcePlatform = source.SourcePlatform
	}
	return &merged
}
