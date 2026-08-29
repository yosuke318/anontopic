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

// AddMessage records one message of a conversation.
func (r *PostgresRepository) AddMessage(ctx context.Context, conversationID, senderToken, body string, flag int) (Message, error) {
	msg := Message{SenderToken: senderToken, Body: body}

	row := r.pool.QueryRow(ctx,
		"INSERT INTO messages (conversation_id, sender_token, body, moderation_flag) "+
			"VALUES ($1, $2, $3, $4) RETURNING id, created_at",
		conversationID, senderToken, body, flag)
	if err := row.Scan(&msg.ID, &msg.CreatedAt); err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
	}

	msg.CreatedAt = msg.CreatedAt.UTC()
	return msg, nil
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
