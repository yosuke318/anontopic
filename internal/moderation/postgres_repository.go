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
func (r *PostgresRepository) ActiveWords(ctx context.Context) ([]Word, error) {
	rows, err := r.pool.Query(ctx, "SELECT word, category FROM ng_words WHERE is_active ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("select ng words: %w", err)
	}

	words, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Word, error) {
		var w Word
		err := row.Scan(&w.Text, &w.Category)
		return w, err
	})
	if err != nil {
		return nil, fmt.Errorf("read ng words: %w", err)
	}

	return words, nil
}
