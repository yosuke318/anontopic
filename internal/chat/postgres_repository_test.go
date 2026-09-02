package chat

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresTestRepository builds a repository on the PostgreSQL of the local
// stack. The test skips when no database answers or the schema is not there,
// so `go test ./...` runs without either.
func postgresTestRepository(t *testing.T) *PostgresRepository {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://anontopic:anontopic@localhost:5432/anontopic?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Skipf("no PostgreSQL at %s: %v", url, err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("no PostgreSQL at %s: %v", url, err)
	}

	var ready bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('conversations') IS NOT NULL").Scan(&ready); err != nil {
		t.Skipf("cannot read the schema of %s: %v", url, err)
	}
	if !ready {
		t.Skip("the database holds no schema, run `make migrate` to test against it")
	}

	return &PostgresRepository{pool: pool}
}

// testConversation opens a conversation of its own, so that tests running
// against the same database do not read each other's rows.
func testConversation(t *testing.T, repo *PostgresRepository, participants ...string) string {
	t.Helper()

	ctx := t.Context()

	var topicID int
	name := fmt.Sprintf("test-%d", rand.Int64())
	if err := repo.pool.QueryRow(ctx,
		"INSERT INTO topics (name, is_active) VALUES ($1, false) RETURNING id", name).Scan(&topicID); err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	var conversationID string
	if err := repo.pool.QueryRow(ctx,
		"INSERT INTO conversations (topic_id, room_type) VALUES ($1, $2) RETURNING id",
		topicID, len(participants)).Scan(&conversationID); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	for _, token := range participants {
		if _, err := repo.pool.Exec(ctx,
			"INSERT INTO conversation_participants (conversation_id, session_token) VALUES ($1, $2)",
			conversationID, token); err != nil {
			t.Fatalf("insert conversation participant: %v", err)
		}
	}

	t.Cleanup(func() {
		ctx := context.Background()
		for _, statement := range []string{
			"DELETE FROM messages WHERE conversation_id = $1",
			"DELETE FROM conversation_participants WHERE conversation_id = $1",
			"DELETE FROM conversations WHERE id = $1",
		} {
			if _, err := repo.pool.Exec(ctx, statement, conversationID); err != nil {
				t.Errorf("clean up the conversation: %v", err)
			}
		}
		if _, err := repo.pool.Exec(ctx, "DELETE FROM topics WHERE id = $1", topicID); err != nil {
			t.Errorf("clean up the topic: %v", err)
		}
	})

	return conversationID
}

func TestPostgresRepositoryReadsAConversationWithItsParticipants(t *testing.T) {
	repo := postgresTestRepository(t)
	id := testConversation(t, repo, "first-token", "second-token")

	conv, err := repo.Conversation(t.Context(), id)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}

	if conv.ID != id || conv.RoomType != 2 {
		t.Fatalf("read %+v, want the conversation %s of two participants", conv, id)
	}
	if !slices.Equal(conv.Participants, []string{"first-token", "second-token"}) {
		t.Fatalf("participants = %v, want them in the order they were recorded", conv.Participants)
	}
	if !conv.EndedAt.IsZero() {
		t.Fatalf("ended at %v, want a conversation in progress", conv.EndedAt)
	}
	if conv.StartedAt.IsZero() {
		t.Fatal("the conversation was read without the time it started")
	}
}

func TestPostgresRepositoryReportsAConversationItCannotRead(t *testing.T) {
	repo := postgresTestRepository(t)

	tests := map[string]string{
		"a conversation that does not exist": "e6a7d1c7-6d4e-4a2f-89b6-2b2a3e6b1f10",
		"a value that is no id":              "1c8f",
	}
	for name, id := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := repo.Conversation(t.Context(), id); !errors.Is(err, ErrConversationNotFound) {
				t.Fatalf("Conversation(%q) = %v, want %v", id, err, ErrConversationNotFound)
			}
		})
	}
}

func TestPostgresRepositoryStoresMessagesInThePartitionOfTheirTime(t *testing.T) {
	repo := postgresTestRepository(t)
	id := testConversation(t, repo, "first-token", "second-token")

	// PostgreSQL keeps a timestamp to the microsecond, so a time the test can
	// compare with what it reads back is rounded to one.
	sentAt := time.Now().UTC().Truncate(time.Microsecond)
	messages := []Message{
		{
			ConversationID: id,
			SenderToken:    "first-token",
			Body:           "こんにちは",
			Flag:           moderationFlagClean,
			CreatedAt:      sentAt,
		},
		{
			ConversationID: id,
			SenderToken:    "second-token",
			Body:           "会いませんか",
			Flag:           moderationFlagNG,
			CreatedAt:      sentAt.Add(time.Millisecond),
		},
	}

	if err := repo.AddMessages(t.Context(), messages); err != nil {
		t.Fatalf("AddMessages: %v", err)
	}

	rows, err := repo.pool.Query(t.Context(),
		"SELECT sender_token, body, moderation_flag, created_at, tableoid::regclass::text "+
			"FROM messages WHERE conversation_id = $1 ORDER BY created_at", id)
	if err != nil {
		t.Fatalf("read the messages back: %v", err)
	}
	defer rows.Close()

	// The month of the message decides its partition, and the partitions of
	// the months around this one are created by the migration.
	partition := "messages_" + sentAt.Format("200601")

	var read int
	for rows.Next() {
		var senderToken, body, table string
		var flag int
		var createdAt time.Time
		if err := rows.Scan(&senderToken, &body, &flag, &createdAt, &table); err != nil {
			t.Fatalf("read a message back: %v", err)
		}

		want := messages[read]
		if senderToken != want.SenderToken || body != want.Body || flag != want.Flag {
			t.Fatalf("read back %q of %q with flag %d, want %q of %q with flag %d",
				body, senderToken, flag, want.Body, want.SenderToken, want.Flag)
		}
		if !createdAt.UTC().Equal(want.CreatedAt) {
			t.Fatalf("recorded at %v, want the %v the message was taken at",
				createdAt.UTC(), want.CreatedAt)
		}
		if table != partition {
			t.Fatalf("the message is in %s, want %s", table, partition)
		}

		read++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read the messages back: %v", err)
	}
	if read != len(messages) {
		t.Fatalf("read back %d messages, want %d", read, len(messages))
	}
}

func TestPostgresRepositoryEndsAConversationOnce(t *testing.T) {
	repo := postgresTestRepository(t)
	id := testConversation(t, repo, "first-token", "second-token")
	endedAt := time.Now().UTC().Truncate(time.Millisecond)

	ended, err := repo.End(t.Context(), id, endReasonUserLeft, endedAt)
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	if !ended {
		t.Fatal("the conversation was not ended by the call that ended it")
	}

	// Another server reaching the same conclusion leaves the time and the
	// reason the conversation ended with.
	ended, err = repo.End(t.Context(), id, "system", endedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("End again: %v", err)
	}
	if ended {
		t.Fatal("a conversation that was already over was ended a second time")
	}

	conv, err := repo.Conversation(t.Context(), id)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if !conv.EndedAt.Equal(endedAt) {
		t.Fatalf("ended at %v, want %v", conv.EndedAt, endedAt)
	}

	var reason string
	if err := repo.pool.QueryRow(t.Context(),
		"SELECT end_reason FROM conversations WHERE id = $1", id).Scan(&reason); err != nil {
		t.Fatalf("read the reason back: %v", err)
	}
	if reason != endReasonUserLeft {
		t.Fatalf("end_reason = %q, want %q", reason, endReasonUserLeft)
	}
}
