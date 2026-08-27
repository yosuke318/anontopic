package matching

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository stores formed rooms in the conversations and
// conversation_participants tables.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository builds a Repository on top of pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// CreateConversation writes the conversation and its participants in one
// transaction, so that no conversation is ever seen without the participants
// it was formed for.
//
// One row of conversation_participants stands for one participant, and
// (conversation_id, session_token) is unique, which lets the insert ignore a
// token that appears twice; the reasoning is in
// docs/adr/0003-one-row-per-conversation-participant.md.
func (r *PostgresRepository) CreateConversation(ctx context.Context, topicID int, participants []string) (Conversation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Conversation{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	conv := Conversation{TopicID: topicID, RoomType: len(participants)}
	row := tx.QueryRow(ctx,
		"INSERT INTO conversations (topic_id, room_type) VALUES ($1, $2) RETURNING id, started_at",
		topicID, conv.RoomType)
	if err := row.Scan(&conv.ID, &conv.StartedAt); err != nil {
		return Conversation{}, fmt.Errorf("insert conversation: %w", err)
	}

	if err := insertParticipants(ctx, tx, conv.ID, participants); err != nil {
		return Conversation{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Conversation{}, fmt.Errorf("commit conversation: %w", err)
	}

	conv.StartedAt = conv.StartedAt.UTC()
	return conv, nil
}

// insertParticipants records every participant of one conversation.
func insertParticipants(ctx context.Context, tx pgx.Tx, conversationID string, participants []string) error {
	batch := &pgx.Batch{}
	for _, token := range participants {
		batch.Queue(
			"INSERT INTO conversation_participants (conversation_id, session_token) "+
				"VALUES ($1, $2) ON CONFLICT DO NOTHING",
			conversationID, token)
	}

	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("insert conversation participants: %w", err)
	}
	return nil
}
