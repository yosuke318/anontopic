package report

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository stores reports in the reports table.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository builds a repository on top of pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Add records rep. The table holds one row per reporter per conversation, so
// a reporter who reports the same conversation again writes nothing.
func (r *PostgresRepository) Add(ctx context.Context, rep Report) error {
	_, err := r.pool.Exec(ctx,
		"INSERT INTO reports (conversation_id, reporter_token, reason) VALUES ($1, $2, $3) "+
			"ON CONFLICT (conversation_id, reporter_token) DO NOTHING",
		rep.ConversationID, rep.ReporterToken, rep.Reason)
	if err != nil {
		return fmt.Errorf("insert report: %w", err)
	}
	return nil
}
