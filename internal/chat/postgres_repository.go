package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository reads conversations and stores the messages sent in
// them.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository builds a Repository on top of pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Conversation reads one conversation and the participants it was formed for.
//
// The participants come back in the order their rows were recorded, which is
// the order the room speaks of them in. One row stands for one participant,
// so a participant who connects again is still a single entry; the reasoning
// is in docs/adr/0003-one-row-per-conversation-participant.md.
func (r *PostgresRepository) Conversation(ctx context.Context, id string) (Conversation, error) {
	// A path value that cannot be an id would reach the query as an error
	// rather than as a missing row.
	var parsed pgtype.UUID
	if err := parsed.Scan(id); err != nil {
		return Conversation{}, ErrConversationNotFound
	}

	conv := Conversation{ID: id}
	var endedAt *time.Time

	row := r.pool.QueryRow(ctx,
		"SELECT topic_id, room_type, started_at, ended_at FROM conversations WHERE id = $1", id)
	if err := row.Scan(&conv.TopicID, &conv.RoomType, &conv.StartedAt, &endedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Conversation{}, ErrConversationNotFound
		}
		return Conversation{}, fmt.Errorf("select conversation: %w", err)
	}

	conv.StartedAt = conv.StartedAt.UTC()
	if endedAt != nil {
		conv.EndedAt = endedAt.UTC()
	}

	rows, err := r.pool.Query(ctx,
		"SELECT session_token FROM conversation_participants "+
			"WHERE conversation_id = $1 ORDER BY joined_at, id", id)
	if err != nil {
		return Conversation{}, fmt.Errorf("select conversation participants: %w", err)
	}

	conv.Participants, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return Conversation{}, fmt.Errorf("read conversation participants: %w", err)
	}

	return conv, nil
}

// AddMessages records messages in one round trip. Each row carries the time
// its message was taken, which is what decides the partition it goes to.
func (r *PostgresRepository) AddMessages(ctx context.Context, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, msg := range messages {
		batch.Queue(
			"INSERT INTO messages (conversation_id, sender_token, body, moderation_flag, created_at) "+
				"VALUES ($1, $2, $3, $4, $5)",
			msg.ConversationID, msg.SenderToken, msg.Body, msg.Flag, msg.CreatedAt)
	}

	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("insert messages: %w", err)
	}

	return nil
}

// End records that a conversation finished. Only the first call writes, so
// that the time and the reason are the ones the conversation actually ended
// with.
func (r *PostgresRepository) End(ctx context.Context, conversationID, reason string, at time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		"UPDATE conversations SET ended_at = $2, end_reason = $3 "+
			"WHERE id = $1 AND ended_at IS NULL",
		conversationID, at, reason)
	if err != nil {
		return false, fmt.Errorf("update conversation: %w", err)
	}

	return tag.RowsAffected() == 1, nil
}

// Flag marks the conversation and the messages the other participants sent in
// one transaction, so that a reported conversation is never kept without the
// messages it was reported for being told apart.
func (r *PostgresRepository) Flag(ctx context.Context, conversationID, reporterToken string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"UPDATE conversations SET is_flagged = true WHERE id = $1", conversationID); err != nil {
			return fmt.Errorf("update conversation: %w", err)
		}

		if _, err := tx.Exec(ctx,
			"UPDATE messages SET moderation_flag = $3 "+
				"WHERE conversation_id = $1 AND sender_token <> $2 AND moderation_flag = $4",
			conversationID, reporterToken, moderationFlagReported, moderationFlagClean); err != nil {
			return fmt.Errorf("update messages: %w", err)
		}

		return nil
	})
}

// Transcript reads one conversation, the participants it was formed for and
// every message recorded in it.
func (r *PostgresRepository) Transcript(ctx context.Context, id string) (Transcript, error) {
	conv, err := r.Conversation(ctx, id)
	if err != nil {
		return Transcript{}, err
	}

	tr := Transcript{Conversation: conv}

	var endReason *string
	if err := r.pool.QueryRow(ctx,
		"SELECT end_reason, is_flagged FROM conversations WHERE id = $1", id,
	).Scan(&endReason, &tr.Flagged); err != nil {
		return Transcript{}, fmt.Errorf("select conversation state: %w", err)
	}
	if endReason != nil {
		tr.EndReason = *endReason
	}

	rows, err := r.pool.Query(ctx,
		"SELECT sender_token, body, moderation_flag, created_at FROM messages "+
			"WHERE conversation_id = $1 ORDER BY created_at, id", id)
	if err != nil {
		return Transcript{}, fmt.Errorf("select messages: %w", err)
	}

	tr.Messages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
		msg := Message{ConversationID: id}
		if err := row.Scan(&msg.SenderToken, &msg.Body, &msg.Flag, &msg.CreatedAt); err != nil {
			return Message{}, err
		}
		msg.CreatedAt = msg.CreatedAt.UTC()
		return msg, nil
	})
	if err != nil {
		return Transcript{}, fmt.Errorf("read messages: %w", err)
	}

	return tr, nil
}
