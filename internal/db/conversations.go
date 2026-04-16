package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// conversationColumns is the canonical column list for SELECT queries on conversations.
const conversationColumns = `conversation_id, name, is_group, participants, last_message_ts, unread_count, source_platform, notification_mode, folder, pinned_at, muted_until`

const (
	NotificationModeAll      = "all"
	NotificationModeMentions = "mentions"
	NotificationModeMuted    = "muted"
)

// Folder constants correspond to the libgm ListConversationsRequest_Folder enum
// used during backfill (INBOX, ARCHIVE, SPAM_BLOCKED).
const (
	FolderInbox   = "inbox"
	FolderArchive = "archive"
	FolderSpam    = "spam"
)

func parseFolder(folder string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(folder)) {
	case FolderInbox:
		return FolderInbox, nil
	case FolderArchive:
		return FolderArchive, nil
	case FolderSpam, "spam_blocked":
		return FolderSpam, nil
	default:
		return "", fmt.Errorf("invalid folder %q", folder)
	}
}

func explicitNotificationMode(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return "", false
	case NotificationModeAll:
		return NotificationModeAll, true
	case "mention", NotificationModeMentions:
		return NotificationModeMentions, true
	case "mute", "muted", "none", "off":
		return NotificationModeMuted, true
	default:
		return NotificationModeAll, true
	}
}

func normalizeStoredNotificationMode(mode string) string {
	normalized, ok := explicitNotificationMode(mode)
	if !ok {
		return NotificationModeAll
	}
	return normalized
}

func parseNotificationMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case NotificationModeAll:
		return NotificationModeAll, nil
	case NotificationModeMentions:
		return NotificationModeMentions, nil
	case NotificationModeMuted:
		return NotificationModeMuted, nil
	default:
		return "", fmt.Errorf("invalid notification mode %q", mode)
	}
}

func (s *Store) UpsertConversation(c *Conversation) error {
	if c.SourcePlatform == "" {
		c.SourcePlatform = "sms"
	}
	notificationMode, hasNotificationMode := explicitNotificationMode(c.NotificationMode)
	folder := strings.TrimSpace(strings.ToLower(c.Folder))
	// Normalise SPAM_BLOCKED → spam; empty stays empty so conflict preserves.
	if folder == "spam_blocked" {
		folder = FolderSpam
	}
	_, err := s.db.Exec(`
		INSERT INTO conversations (conversation_id, name, is_group, participants, last_message_ts, unread_count, source_platform, notification_mode, folder)
		VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), 'all'), COALESCE(NULLIF(?, ''), 'inbox'))
		ON CONFLICT(conversation_id) DO UPDATE SET
			name=excluded.name,
			is_group=excluded.is_group,
			participants=excluded.participants,
			last_message_ts=excluded.last_message_ts,
			unread_count=excluded.unread_count,
			source_platform=excluded.source_platform,
			notification_mode=CASE WHEN ? != '' THEN ? ELSE conversations.notification_mode END,
			folder=CASE WHEN ? != '' THEN ? ELSE conversations.folder END
	`,
		c.ConversationID, c.Name, c.IsGroup, c.Participants, c.LastMessageTS, c.UnreadCount, c.SourcePlatform,
		notificationMode, folder,
		maybeNotificationModeArg(hasNotificationMode, notificationMode), maybeNotificationModeArg(hasNotificationMode, notificationMode),
		folder, folder,
	)
	return err
}

func (s *Store) GetConversation(id string) (*Conversation, error) {
	c := &Conversation{}
	err := s.db.QueryRow(`
		SELECT `+conversationColumns+`
		FROM conversations WHERE conversation_id = ?
	`, id).Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform, &c.NotificationMode, &c.Folder, &c.PinnedAt, &c.MutedUntil)
	if err != nil {
		return nil, err
	}
	c.NotificationMode = normalizeStoredNotificationMode(c.NotificationMode)
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

func (s *Store) DeleteConversation(id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM messages WHERE conversation_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM drafts WHERE conversation_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM conversations WHERE conversation_id = ?`, id); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) MarkConversationRead(id string) error {
	_, err := s.db.Exec(`UPDATE conversations SET unread_count = 0 WHERE conversation_id = ?`, id)
	return err
}

func (s *Store) SetConversationNotificationMode(id, mode string) error {
	normalized, err := parseNotificationMode(mode)
	if err != nil {
		return err
	}
	// Mode changes away from 'muted' always clear muted_until; switching to muted
	// without a duration clears muted_until too (callers that want a deadline use
	// SetConversationMute).
	_, err = s.db.Exec(`UPDATE conversations SET notification_mode = ?, muted_until = 0 WHERE conversation_id = ?`, normalized, id)
	return err
}

// SetConversationMute mutes a conversation until a specific unix second. Pass
// untilUnix=0 to mute indefinitely ("until I unmute"). Pass a past or zero
// timestamp together with NotificationModeAll via SetConversationNotificationMode
// to unmute.
func (s *Store) SetConversationMute(id string, untilUnix int64) error {
	if untilUnix < 0 {
		untilUnix = 0
	}
	_, err := s.db.Exec(`UPDATE conversations SET notification_mode = ?, muted_until = ? WHERE conversation_id = ?`, NotificationModeMuted, untilUnix, id)
	return err
}

// IsMuted returns true when the conversation's NotificationMode is muted and the
// mute has not auto-expired. Callers should use this instead of comparing
// NotificationMode directly so time-bounded mutes behave correctly.
func (c *Conversation) IsMuted() bool {
	if c == nil || c.NotificationMode != NotificationModeMuted {
		return false
	}
	if c.MutedUntil == 0 {
		return true // indefinite
	}
	return time.Now().Unix() < c.MutedUntil
}

// SetConversationPinned toggles a conversation's pinned state. When pinning,
// pinned_at is set to the current unix second so newer pins sort above older
// ones; when unpinning, pinned_at is reset to 0.
func (s *Store) SetConversationPinned(id string, pinned bool) error {
	var ts int64
	if pinned {
		ts = time.Now().Unix()
	}
	_, err := s.db.Exec(`UPDATE conversations SET pinned_at = ? WHERE conversation_id = ?`, ts, id)
	return err
}

// SetConversationFolder moves a conversation to `folder` (inbox/archive/spam).
// Returns an error for unknown folder values.
func (s *Store) SetConversationFolder(id, folder string) error {
	normalized, err := parseFolder(folder)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE conversations SET folder = ? WHERE conversation_id = ?`, normalized, id)
	return err
}

// ListConversationsByFolder returns conversations in the given folder, ordered
// by last_message_ts DESC, up to limit. Pass "" for folder to return all.
func (s *Store) ListConversationsByFolder(folder string, limit int) ([]*Conversation, error) {
	if folder == "" {
		return s.ListConversations(limit)
	}
	normalized, err := parseFolder(folder)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
		SELECT `+conversationColumns+`
		FROM conversations
		WHERE folder = ?
		ORDER BY pinned_at DESC, last_message_ts DESC
		LIMIT ?
	`, normalized, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversations(rows)
}

func (s *Store) ListConversations(limit int) ([]*Conversation, error) {
	rows, err := s.db.Query(`
		SELECT `+conversationColumns+`
		FROM conversations
		ORDER BY pinned_at DESC, last_message_ts DESC
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
		ORDER BY pinned_at DESC, last_message_ts DESC
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
		ORDER BY pinned_at DESC, last_message_ts DESC
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
		if err := rows.Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform, &c.NotificationMode, &c.Folder, &c.PinnedAt, &c.MutedUntil); err != nil {
			return nil, err
		}
		c.NotificationMode = normalizeStoredNotificationMode(c.NotificationMode)
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

func getConversationTx(tx *sql.Tx, id string) (*Conversation, error) {
	c := &Conversation{}
	err := tx.QueryRow(`
		SELECT `+conversationColumns+`
		FROM conversations WHERE conversation_id = ?
	`, id).Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform, &c.NotificationMode, &c.Folder, &c.PinnedAt, &c.MutedUntil)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c.NotificationMode = normalizeStoredNotificationMode(c.NotificationMode)
	return c, nil
}

func upsertConversationTx(tx *sql.Tx, c *Conversation) error {
	if c.SourcePlatform == "" {
		c.SourcePlatform = "sms"
	}
	notificationMode, hasNotificationMode := explicitNotificationMode(c.NotificationMode)
	_, err := tx.Exec(`
		INSERT INTO conversations (conversation_id, name, is_group, participants, last_message_ts, unread_count, source_platform, notification_mode)
		VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), 'all'))
		ON CONFLICT(conversation_id) DO UPDATE SET
			name=excluded.name,
			is_group=excluded.is_group,
			participants=excluded.participants,
			last_message_ts=excluded.last_message_ts,
			unread_count=excluded.unread_count,
			source_platform=excluded.source_platform,
			notification_mode=CASE WHEN ? != '' THEN ? ELSE conversations.notification_mode END
	`, c.ConversationID, c.Name, c.IsGroup, c.Participants, c.LastMessageTS, c.UnreadCount, c.SourcePlatform, notificationMode, maybeNotificationModeArg(hasNotificationMode, notificationMode), maybeNotificationModeArg(hasNotificationMode, notificationMode))
	return err
}

func mergeConversationRecords(source, target *Conversation, targetID string) *Conversation {
	if target == nil {
		merged := *source
		merged.ConversationID = targetID
		merged.NotificationMode = normalizeStoredNotificationMode(merged.NotificationMode)
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
	if normalizeStoredNotificationMode(merged.NotificationMode) == NotificationModeAll && normalizeStoredNotificationMode(source.NotificationMode) != NotificationModeAll {
		merged.NotificationMode = normalizeStoredNotificationMode(source.NotificationMode)
	} else {
		merged.NotificationMode = normalizeStoredNotificationMode(merged.NotificationMode)
	}
	return &merged
}

func maybeNotificationModeArg(hasMode bool, mode string) string {
	if !hasMode {
		return ""
	}
	return mode
}
