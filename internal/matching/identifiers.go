package matching

import (
	"context"
	"fmt"
	"time"
)

// IdentifiedBefore returns the id of every conversation with a participant
// who joined before cutoff and still has an ip_hash or a device_fingerprint
// recorded.
func (r *PostgresRepository) IdentifiedBefore(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		"SELECT DISTINCT conversation_id::text FROM conversation_participants "+
			"WHERE joined_at < $1 AND (ip_hash IS NOT NULL OR device_fingerprint IS NOT NULL) "+
			"ORDER BY 1", cutoff)
	if err != nil {
		return nil, fmt.Errorf("list identified conversations: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan identified conversation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list identified conversations: %w", err)
	}
	return ids, nil
}

// EraseIdentifiers sets ip_hash and device_fingerprint to NULL for the
// participants of the given conversations who joined before cutoff, and
// returns how many participants it changed. A participant whose identifiers
// are gone is no longer matched to a ban.
func (r *PostgresRepository) EraseIdentifiers(ctx context.Context, cutoff time.Time, conversationIDs []string) (int64, error) {
	if len(conversationIDs) == 0 {
		return 0, nil
	}

	tag, err := r.pool.Exec(ctx,
		"UPDATE conversation_participants SET ip_hash = NULL, device_fingerprint = NULL "+
			"WHERE conversation_id = ANY($1::uuid[]) AND joined_at < $2 "+
			"AND (ip_hash IS NOT NULL OR device_fingerprint IS NOT NULL)",
		conversationIDs, cutoff)
	if err != nil {
		return 0, fmt.Errorf("erase participant identifiers: %w", err)
	}
	return tag.RowsAffected(), nil
}
