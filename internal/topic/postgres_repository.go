package topic

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// foreignKeyViolation is the SQLSTATE PostgreSQL reports when a row is still
// referenced from another table.
const foreignKeyViolation = "23503"

// topicColumns is the projection every query in this file scans.
const topicColumns = "id, name, is_active, created_at"

// PostgresRepository stores the catalogue in the topics table.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository builds a Repository on top of pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// ListActive returns the topics whose is_active is true.
func (r *PostgresRepository) ListActive(ctx context.Context) ([]Topic, error) {
	return r.list(ctx, "SELECT "+topicColumns+" FROM topics WHERE is_active ORDER BY id")
}

// List returns every topic.
func (r *PostgresRepository) List(ctx context.Context) ([]Topic, error) {
	return r.list(ctx, "SELECT "+topicColumns+" FROM topics ORDER BY id")
}

// Create inserts a topic, which is active by the column default.
func (r *PostgresRepository) Create(ctx context.Context, name string) (Topic, error) {
	row := r.pool.QueryRow(ctx,
		"INSERT INTO topics (name) VALUES ($1) RETURNING "+topicColumns, name)

	created, err := scanTopic(row)
	if err != nil {
		return Topic{}, fmt.Errorf("insert topic: %w", err)
	}
	return created, nil
}

// Update writes the fields upd sets, leaving the others at their stored value.
func (r *PostgresRepository) Update(ctx context.Context, id int, upd Update) (Topic, error) {
	row := r.pool.QueryRow(ctx,
		"UPDATE topics SET name = COALESCE($2, name), is_active = COALESCE($3, is_active) "+
			"WHERE id = $1 RETURNING "+topicColumns, id, upd.Name, upd.IsActive)

	updated, err := scanTopic(row)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Topic{}, ErrNotFound
	case err != nil:
		return Topic{}, fmt.Errorf("update topic: %w", err)
	}
	return updated, nil
}

// Delete removes a topic. PostgreSQL refuses the delete while conversations
// reference the row, and that refusal is reported as ErrInUse.
func (r *PostgresRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM topics WHERE id = $1", id)

	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation:
		return ErrInUse
	case err != nil:
		return fmt.Errorf("delete topic: %w", err)
	case tag.RowsAffected() == 0:
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) list(ctx context.Context, sql string) ([]Topic, error) {
	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query topics: %w", err)
	}
	defer rows.Close()

	topics := make([]Topic, 0)
	for rows.Next() {
		t, err := scanTopic(rows)
		if err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		topics = append(topics, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read topics: %w", err)
	}
	return topics, nil
}

// scanTopic reads one row of topicColumns.
func scanTopic(row pgx.Row) (Topic, error) {
	var t Topic
	if err := row.Scan(&t.ID, &t.Name, &t.IsActive, &t.CreatedAt); err != nil {
		return Topic{}, err
	}
	return t, nil
}
