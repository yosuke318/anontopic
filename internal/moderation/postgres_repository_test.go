package moderation

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
	if err := pool.QueryRow(ctx, "SELECT to_regclass('ng_words') IS NOT NULL").Scan(&ready); err != nil {
		t.Skipf("cannot read the schema of %s: %v", url, err)
	}
	if !ready {
		t.Skip("the database holds no schema, run `make migrate` to test against it")
	}

	return &PostgresRepository{pool: pool}
}

// testWord registers a word of its own, so that tests running against the
// same database do not read each other's rows.
func testWord(t *testing.T, repo *PostgresRepository, active bool) string {
	t.Helper()

	word := fmt.Sprintf("test-%d", rand.Int64())
	_, err := repo.pool.Exec(t.Context(),
		"INSERT INTO ng_words (word, category, is_active) VALUES ($1, 'sexual', $2)", word, active)
	if err != nil {
		t.Fatalf("insert ng word: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if _, err := repo.pool.Exec(ctx, "DELETE FROM ng_words WHERE word = $1", word); err != nil {
			t.Errorf("delete ng word: %v", err)
		}
	})

	return word
}

func TestActiveWordsReadsTheWordsInForce(t *testing.T) {
	repo := postgresTestRepository(t)

	inForce := testWord(t, repo, true)
	switchedOff := testWord(t, repo, false)

	words, err := repo.ActiveWords(t.Context())
	if err != nil {
		t.Fatalf("ActiveWords: %v", err)
	}

	if !slices.Contains(words, inForce) {
		t.Fatalf("ActiveWords left out %q, want it read", inForce)
	}
	if slices.Contains(words, switchedOff) {
		t.Fatalf("ActiveWords read %q, want a word that was switched off left out", switchedOff)
	}
}
