package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// conversationColumns is the canonical column list for SELECT queries on conversations.
const conversationColumns = `conversation_id, name, is_group, participants, last_message_ts, unread_count, source_platform, notification_mode, folder, pinned_at, muted_until, nickname, snoozed_until, archived_at, is_vip`

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
	`, id).Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform, &c.NotificationMode, &c.Folder, &c.PinnedAt, &c.MutedUntil, &c.Nickname, &c.SnoozedUntil, &c.ArchivedAt, &c.IsVIP)
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
	// Best-effort: outbox exists only on DBs migrated through outbox.go. Swallow
	// a 'no such table' so this call remains a no-op on older DBs.
	if _, err := tx.Exec(`DELETE FROM outbox WHERE conversation_id = ?`, id); err != nil && !strings.Contains(err.Error(), "no such table") {
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

// MarkConversationUnread sets unread_count to at least 1 so the sidebar badge
// reappears. No upstream sync — this is local presentation only, since
// Google Messages itself has no "mark unread" concept.
func (s *Store) MarkConversationUnread(id string) error {
	_, err := s.db.Exec(`UPDATE conversations SET unread_count = MAX(unread_count, 1) WHERE conversation_id = ?`, id)
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

// SetConversationArchived sets the local archived_at timestamp. When archived,
// archived_at is the current unix second and folder moves to 'archive'; when
// un-archived both archived_at and folder reset (inbox). Note: the libgm sync
// for Google Messages archive state is handled by the caller in api.go — this
// method updates the local DB only.
func (s *Store) SetConversationArchived(id string, archived bool) error {
	var ts int64
	folder := FolderInbox
	if archived {
		ts = time.Now().Unix()
		folder = FolderArchive
	}
	_, err := s.db.Exec(`UPDATE conversations SET archived_at = ?, folder = ? WHERE conversation_id = ?`, ts, folder, id)
	return err
}

// MuteConversationFor mutes a conversation for durationMS milliseconds from
// now. Pass durationMS=0 to mute indefinitely ("until I unmute").
func (s *Store) MuteConversationFor(id string, durationMS int64) error {
	var until int64
	if durationMS > 0 {
		until = time.Now().Unix() + (durationMS / 1000)
	}
	return s.SetConversationMute(id, until)
}

// UnmuteConversation clears mute and resets the notification mode to 'all'.
func (s *Store) UnmuteConversation(id string) error {
	return s.SetConversationNotificationMode(id, NotificationModeAll)
}

// SetConversationSnooze sets snoozed_until to the given unix second, hiding
// the conversation from list queries until the time passes. Pass 0 to
// unsnooze immediately.
func (s *Store) SetConversationSnooze(id string, until int64) error {
	if until < 0 {
		until = 0
	}
	_, err := s.db.Exec(`UPDATE conversations SET snoozed_until = ? WHERE conversation_id = ?`, until, id)
	return err
}

// SetConversationNickname sets a local display name override. Empty nickname
// clears the override and callers fall back to conversations.name.
// The nickname is local-only — never synced upstream to libgm/Google Messages.
func (s *Store) SetConversationNickname(id, nickname string) error {
	trimmed := strings.TrimSpace(nickname)
	_, err := s.db.Exec(`UPDATE conversations SET nickname = ? WHERE conversation_id = ?`, trimmed, id)
	return err
}

// SetVIP marks or unmarks a conversation as a VIP (starred). VIP conversations
// are shown in a dedicated section above the folder filters in the sidebar.
func (s *Store) SetVIP(conversationID string, vip bool) error {
	v := 0
	if vip {
		v = 1
	}
	_, err := s.db.Exec(`UPDATE conversations SET is_vip = ? WHERE conversation_id = ?`, v, conversationID)
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
// by last_message_ts DESC, up to limit. Pass "" for folder to return all
// non-archived conversations.
func (s *Store) ListConversationsByFolder(folder string, limit int) ([]*Conversation, error) {
	if folder == "" {
		return s.ListConversations(limit)
	}
	normalized, err := parseFolder(folder)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT `+conversationColumns+`
		FROM conversations
		WHERE folder = ?
		  AND (snoozed_until = 0 OR snoozed_until <= ?)
		  AND EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = conversations.conversation_id)
		ORDER BY pinned_at DESC, last_message_ts DESC
		LIMIT ?
	`, normalized, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConversations(rows)
}

// ListConversations returns non-archived conversations. Use
// ListConversationsIncludingArchived (or ListConversationsByFolder with
// folder="archive") to include archived rows.
func (s *Store) ListConversations(limit int) ([]*Conversation, error) {
	return s.listConversations(limit, false)
}

// ListConversationsIncludingArchived returns all conversations regardless of
// archived_at state. Used by the sidebar's "show archived" toggle.
func (s *Store) ListConversationsIncludingArchived(limit int) ([]*Conversation, error) {
	return s.listConversations(limit, true)
}

func (s *Store) listConversations(limit int, includeArchived bool) ([]*Conversation, error) {
	now := time.Now().Unix()
	archivedClause := "AND archived_at = 0 "
	if includeArchived {
		archivedClause = ""
	}
	rows, err := s.db.Query(`
		SELECT `+conversationColumns+`
		FROM conversations
		WHERE (snoozed_until = 0 OR snoozed_until <= ?)
		  `+archivedClause+`
		  AND EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = conversations.conversation_id)
		ORDER BY pinned_at DESC, last_message_ts DESC
		LIMIT ?
	`, now, limit)
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
	now := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT `+conversationColumns+`
		FROM conversations
		WHERE source_platform = ?
		  AND (snoozed_until = 0 OR snoozed_until <= ?)
		  AND EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = conversations.conversation_id)
		ORDER BY pinned_at DESC, last_message_ts DESC
		LIMIT ?
	`, platform, now, limit)
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
		WHERE (name LIKE ?
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
			))
		  AND EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = conversations.conversation_id)
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
		if err := rows.Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform, &c.NotificationMode, &c.Folder, &c.PinnedAt, &c.MutedUntil, &c.Nickname, &c.SnoozedUntil, &c.ArchivedAt, &c.IsVIP); err != nil {
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
	`, id).Scan(&c.ConversationID, &c.Name, &c.IsGroup, &c.Participants, &c.LastMessageTS, &c.UnreadCount, &c.SourcePlatform, &c.NotificationMode, &c.Folder, &c.PinnedAt, &c.MutedUntil, &c.Nickname, &c.SnoozedUntil, &c.ArchivedAt, &c.IsVIP)
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
