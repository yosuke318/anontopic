package report

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository stores reports in the reports table and claims in the
// infringement_claims table.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository builds a repository on top of pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const reportColumns = "id, conversation_id, reporter_token, reason, status, created_at"

const claimColumns = "id, name, email, infringed_right, details, status, created_at"

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

// List reads the reports f keeps, newest first.
func (r *PostgresRepository) List(ctx context.Context, f Filter) ([]Report, error) {
	where, args := filterClause(f, true)
	args = append(args, f.Limit)

	rows, err := r.pool.Query(ctx,
		"SELECT "+reportColumns+" FROM reports"+where+
			" ORDER BY id DESC LIMIT $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("select reports: %w", err)
	}

	reports, err := pgx.CollectRows(rows, scanReport)
	if err != nil {
		return nil, fmt.Errorf("read reports: %w", err)
	}
	return reports, nil
}

// Get reads one report.
func (r *PostgresRepository) Get(ctx context.Context, id int64) (Report, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+reportColumns+" FROM reports WHERE id = $1", id)
	if err != nil {
		return Report{}, fmt.Errorf("select report: %w", err)
	}
	return collectOne(rows, scanReport)
}

// SetStatus moves one report to status.
func (r *PostgresRepository) SetStatus(ctx context.Context, id int64, status string) (Report, error) {
	rows, err := r.pool.Query(ctx,
		"UPDATE reports SET status = $2 WHERE id = $1 RETURNING "+reportColumns, id, status)
	if err != nil {
		return Report{}, fmt.Errorf("update report: %w", err)
	}
	return collectOne(rows, scanReport)
}

// AddClaim records one claim.
func (r *PostgresRepository) AddClaim(ctx context.Context, c Claim) (Claim, error) {
	rows, err := r.pool.Query(ctx,
		"INSERT INTO infringement_claims (name, email, infringed_right, details, status) "+
			"VALUES ($1, $2, $3, $4, $5) RETURNING "+claimColumns,
		c.Name, c.Email, c.Right, c.Details, c.Status)
	if err != nil {
		return Claim{}, fmt.Errorf("insert claim: %w", err)
	}
	return collectOne(rows, scanClaim)
}

// ListClaims reads the claims f keeps, newest first.
func (r *PostgresRepository) ListClaims(ctx context.Context, f Filter) ([]Claim, error) {
	where, args := filterClause(f, false)
	args = append(args, f.Limit)

	rows, err := r.pool.Query(ctx,
		"SELECT "+claimColumns+" FROM infringement_claims"+where+
			" ORDER BY id DESC LIMIT $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("select claims: %w", err)
	}

	claims, err := pgx.CollectRows(rows, scanClaim)
	if err != nil {
		return nil, fmt.Errorf("read claims: %w", err)
	}
	return claims, nil
}

// SetClaimStatus moves one claim to status.
func (r *PostgresRepository) SetClaimStatus(ctx context.Context, id int64, status string) (Claim, error) {
	rows, err := r.pool.Query(ctx,
		"UPDATE infringement_claims SET status = $2 WHERE id = $1 RETURNING "+claimColumns, id, status)
	if err != nil {
		return Claim{}, fmt.Errorf("update claim: %w", err)
	}
	return collectOne(rows, scanClaim)
}

// filterClause builds the WHERE clause of f and the arguments it refers to.
// Only the values go through arguments; the clause itself is built from fixed
// fragments.
func filterClause(f Filter, byConversation bool) (string, []any) {
	var conds []string
	var args []any

	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, cond+" $"+strconv.Itoa(len(args)))
	}

	if f.Status != "" {
		add("status =", f.Status)
	}
	if byConversation && f.ConversationID != "" {
		add("conversation_id =", f.ConversationID)
	}
	if f.BeforeID > 0 {
		add("id <", f.BeforeID)
	}

	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func scanReport(row pgx.CollectableRow) (Report, error) {
	var rep Report
	var reason *string
	if err := row.Scan(&rep.ID, &rep.ConversationID, &rep.ReporterToken, &reason, &rep.Status, &rep.CreatedAt); err != nil {
		return Report{}, err
	}
	if reason != nil {
		rep.Reason = *reason
	}
	rep.CreatedAt = rep.CreatedAt.UTC()
	return rep, nil
}

func scanClaim(row pgx.CollectableRow) (Claim, error) {
	var c Claim
	if err := row.Scan(&c.ID, &c.Name, &c.Email, &c.Right, &c.Details, &c.Status, &c.CreatedAt); err != nil {
		return Claim{}, err
	}
	c.CreatedAt = c.CreatedAt.UTC()
	return c, nil
}

// collectOne reads the single row a statement returns, and reports ErrNotFound
// when it returned none.
func collectOne[T any](rows pgx.Rows, scan pgx.RowToFunc[T]) (T, error) {
	v, err := pgx.CollectExactlyOneRow(rows, scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, fmt.Errorf("read row: %w", err)
	}
	return v, nil
}
