package moderation

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository reads the NG word dictionary from ng_words.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository builds a Repository on top of pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// ActiveWords reads the words in force, in the order they were registered.
func (r *PostgresRepository) ActiveWords(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, "SELECT word FROM ng_words WHERE is_active ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("select ng words: %w", err)
	}

	words, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read ng words: %w", err)
	}

	return words, nil
}
