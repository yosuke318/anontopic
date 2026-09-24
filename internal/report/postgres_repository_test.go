package report

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
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
	if err := pool.QueryRow(ctx, "SELECT to_regclass('infringement_claims') IS NOT NULL").Scan(&ready); err != nil {
		t.Skipf("cannot read the schema of %s: %v", url, err)
	}
	if !ready {
		t.Skip("the database holds no schema, run `make migrate` to test against it")
	}

	return &PostgresRepository{pool: pool}
}

// openConversation opens a conversation of its own for reports to reference,
// and removes it with its reports afterwards.
func openConversation(t *testing.T, repo *PostgresRepository) string {
	t.Helper()
	ctx := t.Context()

	var topicID int
	if err := repo.pool.QueryRow(ctx,
		"INSERT INTO topics (name, is_active) VALUES ($1, false) RETURNING id",
		fmt.Sprintf("test-%d", rand.Int64())).Scan(&topicID); err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	var id string
	if err := repo.pool.QueryRow(ctx,
		"INSERT INTO conversations (topic_id, room_type) VALUES ($1, 2) RETURNING id", topicID).Scan(&id); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		for _, stmt := range []struct {
			sql string
			arg any
		}{
			{"DELETE FROM reports WHERE conversation_id = $1", id},
			{"DELETE FROM conversations WHERE id = $1", id},
			{"DELETE FROM topics WHERE id = $1", topicID},
		} {
			if _, err := repo.pool.Exec(ctx, stmt.sql, stmt.arg); err != nil {
				t.Errorf("clean up: %v", err)
			}
		}
	})

	return id
}

func TestPostgresRepositoryListsAndMovesTheReportsOfAConversation(t *testing.T) {
	repo := postgresTestRepository(t)
	ctx := t.Context()
	id := openConversation(t, repo)

	for _, token := range []string{"first-token", "second-token", "first-token"} {
		if err := repo.Add(ctx, Report{ConversationID: id, ReporterToken: token, Reason: ReasonDating}); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	reports, err := repo.List(ctx, Filter{ConversationID: id, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("listed %d reports, want one per reporter", len(reports))
	}
	newest := reports[0]
	if newest.ConversationID != id || newest.ReporterToken != "second-token" ||
		newest.Status != StatusOpen || newest.Reason != ReasonDating || newest.CreatedAt.IsZero() {
		t.Fatalf("newest = %+v, want the open report of second-token on %s", newest, id)
	}

	older, err := repo.List(ctx, Filter{ConversationID: id, BeforeID: newest.ID, Limit: 10})
	if err != nil {
		t.Fatalf("List before: %v", err)
	}
	if len(older) != 1 || older[0].ReporterToken != "first-token" {
		t.Fatalf("older = %+v, want the report of first-token", older)
	}

	moved, err := repo.SetStatus(ctx, newest.ID, StatusRejected)
	if err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if moved.Status != StatusRejected {
		t.Fatalf("status = %q, want %q", moved.Status, StatusRejected)
	}

	open, err := repo.List(ctx, Filter{ConversationID: id, Status: StatusOpen, Limit: 10})
	if err != nil {
		t.Fatalf("List open: %v", err)
	}
	if len(open) != 1 || open[0].ID != older[0].ID {
		t.Fatalf("open = %+v, want the report that was not moved", open)
	}

	got, err := repo.Get(ctx, newest.ID)
	if err != nil || got.Status != StatusRejected {
		t.Fatalf("Get = %+v, %v, want the rejected report", got, err)
	}
}

func TestPostgresRepositoryAnswersAReportThatDoesNotExist(t *testing.T) {
	repo := postgresTestRepository(t)

	if _, err := repo.Get(t.Context(), -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want %v", err, ErrNotFound)
	}
	if _, err := repo.SetStatus(t.Context(), -1, StatusOpen); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetStatus = %v, want %v", err, ErrNotFound)
	}
}

func TestPostgresRepositoryStoresAndMovesClaims(t *testing.T) {
	repo := postgresTestRepository(t)
	ctx := t.Context()

	created, err := repo.AddClaim(ctx, Claim{
		Name:    "山田 太郎",
		Email:   "taro@example.com",
		Right:   RightPrivacy,
		Details: "住所が書き込まれていた",
		Status:  StatusOpen,
	})
	if err != nil {
		t.Fatalf("AddClaim: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.pool.Exec(context.Background(),
			"DELETE FROM infringement_claims WHERE id = $1", created.ID); err != nil {
			t.Errorf("clean up the claim: %v", err)
		}
	})

	if created.ID == 0 || created.CreatedAt.IsZero() || created.Right != RightPrivacy {
		t.Fatalf("created = %+v, want an id, a time and the right it was filed for", created)
	}

	moved, err := repo.SetClaimStatus(ctx, created.ID, StatusReviewing)
	if err != nil {
		t.Fatalf("SetClaimStatus: %v", err)
	}
	if moved.Status != StatusReviewing {
		t.Fatalf("status = %q, want %q", moved.Status, StatusReviewing)
	}

	claims, err := repo.ListClaims(ctx, Filter{Status: StatusReviewing, BeforeID: created.ID + 1, Limit: 1})
	if err != nil {
		t.Fatalf("ListClaims: %v", err)
	}
	if len(claims) != 1 || claims[0].ID != created.ID {
		t.Fatalf("claims = %+v, want the claim that was moved", claims)
	}
}
