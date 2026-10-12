package matching

import (
	"context"
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
	if err := pool.QueryRow(ctx, "SELECT to_regclass('conversation_participants') IS NOT NULL").Scan(&ready); err != nil {
		t.Skipf("cannot read the schema of %s: %v", url, err)
	}
	if !ready {
		t.Skip("the database holds no schema, run `make migrate` to test against it")
	}

	return NewPostgresRepository(pool)
}

// testIdentifiedConversation forms a conversation of two participants who
// joined at joinedAt, each with both identifiers recorded.
func testIdentifiedConversation(t *testing.T, repo *PostgresRepository, joinedAt time.Time) string {
	t.Helper()

	ctx := t.Context()

	var topicID int
	if err := repo.pool.QueryRow(ctx,
		"INSERT INTO topics (name, is_active) VALUES ($1, false) RETURNING id",
		fmt.Sprintf("test-%d", rand.Int64())).Scan(&topicID); err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	conv, err := repo.CreateConversation(ctx, topicID, []Participant{
		{Token: fmt.Sprintf("a-%d", rand.Int64()), Identity: Identity{IPHash: "ip-a", Device: "device-a"}},
		{Token: fmt.Sprintf("b-%d", rand.Int64()), Identity: Identity{IPHash: "ip-b", Device: "device-b"}},
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if _, err := repo.pool.Exec(ctx,
		"UPDATE conversation_participants SET joined_at = $2 WHERE conversation_id = $1",
		conv.ID, joinedAt); err != nil {
		t.Fatalf("set joined_at: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = repo.pool.Exec(ctx, "DELETE FROM conversation_participants WHERE conversation_id = $1", conv.ID)
		_, _ = repo.pool.Exec(ctx, "DELETE FROM conversations WHERE id = $1", conv.ID)
		_, _ = repo.pool.Exec(ctx, "DELETE FROM topics WHERE id = $1", topicID)
	})

	return conv.ID
}

func TestEraseIdentifiersClearsOnlyTheNamedConversationsOlderThanTheCutoff(t *testing.T) {
	repo := postgresTestRepository(t)
	ctx := t.Context()

	// Every conversation of this test sits far in the past, apart from the
	// rows other tests and the seed data write.
	cutoff := time.Date(2000, 7, 1, 0, 0, 0, 0, time.UTC)
	erased := testIdentifiedConversation(t, repo, cutoff.Add(-48*time.Hour))
	kept := testIdentifiedConversation(t, repo, cutoff.Add(-24*time.Hour))
	young := testIdentifiedConversation(t, repo, cutoff.Add(time.Hour))

	ids, err := repo.IdentifiedBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("IdentifiedBefore: %v", err)
	}
	if !slices.Contains(ids, erased) || !slices.Contains(ids, kept) || slices.Contains(ids, young) {
		t.Fatalf("IdentifiedBefore = %v, want %s and %s without %s", ids, erased, kept, young)
	}

	// young is named too, and stays because its participants joined after
	// the cutoff.
	n, err := repo.EraseIdentifiers(ctx, cutoff, []string{erased, young})
	if err != nil {
		t.Fatalf("EraseIdentifiers: %v", err)
	}
	if n != 2 {
		t.Fatalf("EraseIdentifiers changed %d participants, want the 2 of %s", n, erased)
	}

	for id, wantIdentified := range map[string]bool{erased: false, kept: true, young: true} {
		var identified int
		if err := repo.pool.QueryRow(ctx,
			"SELECT count(*) FROM conversation_participants "+
				"WHERE conversation_id = $1 AND ip_hash IS NOT NULL AND device_fingerprint IS NOT NULL",
			id).Scan(&identified); err != nil {
			t.Fatalf("count identified participants: %v", err)
		}
		if got := identified == 2; got != wantIdentified {
			t.Fatalf("conversation %s has %d identified participants, want identified = %v", id, identified, wantIdentified)
		}
	}

	ids, err = repo.IdentifiedBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("IdentifiedBefore: %v", err)
	}
	if slices.Contains(ids, erased) {
		t.Fatalf("IdentifiedBefore still lists %s after its identifiers were erased", erased)
	}
}
