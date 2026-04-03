package db

import "database/sql"

type LegacyRepairReport struct {
	DeletedWhatsAppReactionPlaceholders int
	RemainingWhatsAppMediaPlaceholders  int
}

const legacyWhatsAppReactionPlaceholderWhere = `
	source_platform = 'whatsapp'
	AND body = '[Reaction]'
	AND IFNULL(media_id, '') = ''
	AND IFNULL(mime_type, '') = ''
	AND IFNULL(reactions, '') = ''
	AND IFNULL(reply_to_id, '') = ''
	AND IFNULL(source_id, '') != ''
`

func (s *Store) RepairLegacyArtifacts() (LegacyRepairReport, error) {
	report := LegacyRepairReport{}

	tx, err := s.db.Begin()
	if err != nil {
		return report, err
	}
	defer tx.Rollback()

	affectedConversationIDs, err := selectStringColumnTx(tx, `
		SELECT DISTINCT conversation_id
		FROM messages
		WHERE `+legacyWhatsAppReactionPlaceholderWhere)
	if err != nil {
		return report, err
	}

	result, err := tx.Exec(`
		DELETE FROM messages
		WHERE ` + legacyWhatsAppReactionPlaceholderWhere)
	if err != nil {
		return report, err
	}
	if deleted, err := result.RowsAffected(); err == nil {
		report.DeletedWhatsAppReactionPlaceholders = int(deleted)
	}

	for _, conversationID := range affectedConversationIDs {
		if _, err := tx.Exec(`
			UPDATE conversations
			SET last_message_ts = COALESCE((
				SELECT MAX(timestamp_ms)
				FROM messages
				WHERE conversation_id = ?
			), 0)
			WHERE conversation_id = ?
		`, conversationID, conversationID); err != nil {
			return report, err
		}
	}

	if err := tx.Commit(); err != nil {
		return report, err
	}

	if err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM messages
		WHERE source_platform = 'whatsapp'
			AND body IN ('[Photo]', '[Video]', '[Audio]', '[Voice note]')
			AND IFNULL(media_id, '') = ''
	`).Scan(&report.RemainingWhatsAppMediaPlaceholders); err != nil {
		return report, err
	}

	return report, nil
}

func selectStringColumnTx(tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}
