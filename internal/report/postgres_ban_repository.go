package report

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresBanRepository stores sanctions in the banned_identifiers table.
type PostgresBanRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresBanRepository builds a ban repository on top of pool.
func NewPostgresBanRepository(pool *pgxpool.Pool) *PostgresBanRepository {
	return &PostgresBanRepository{pool: pool}
}

const banColumns = "id, identifier_type, identifier, sanction, source, reason, conversation_id, " +
	"banned_until, created_at, lifted_at"

// inForce is the condition a row bars its identifier under at the time the
// argument it names holds.
func inForce(nowArg string) string {
	return "sanction IN ('" + SanctionSuspension + "', '" + SanctionPermanent + "') " +
		"AND lifted_at IS NULL AND (banned_until IS NULL OR banned_until > " + nowArg + ")"
}

// AddBan records one sanction.
func (r *PostgresBanRepository) AddBan(ctx context.Context, b Ban) (Ban, error) {
	rows, err := r.pool.Query(ctx,
		"INSERT INTO banned_identifiers "+
			"(identifier_type, identifier, sanction, source, reason, conversation_id, banned_until) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING "+banColumns,
		b.IdentifierType, b.Identifier, b.Sanction, b.Source,
		nullIfEmpty(b.Reason), nullIfEmpty(b.ConversationID), nullIfZero(b.BannedUntil))
	if err != nil {
		return Ban{}, fmt.Errorf("insert ban: %w", err)
	}
	return collectOne(rows, scanBan)
}

// BannedUntil reads whether a ban is in force on one identifier and when the
// last one ends.
func (r *PostgresBanRepository) BannedUntil(ctx context.Context, identifierType, identifier string, now time.Time) (bool, time.Time, error) {
	var (
		n         int
		unbounded bool
		until     *time.Time
	)
	err := r.pool.QueryRow(ctx,
		"SELECT count(*), coalesce(bool_or(banned_until IS NULL), false), max(banned_until) "+
			"FROM banned_identifiers "+
			"WHERE identifier_type = $1 AND identifier = $2 AND "+inForce("$3"),
		identifierType, identifier, now).Scan(&n, &unbounded, &until)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("query banned identifiers: %w", err)
	}

	if n == 0 {
		return false, time.Time{}, nil
	}
	if unbounded || until == nil {
		return true, time.Time{}, nil
	}
	return true, until.UTC(), nil
}

// CountSanctions counts the sanctions on one identifier that are not lifted.
func (r *PostgresBanRepository) CountSanctions(ctx context.Context, identifierType, identifier string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		"SELECT count(*) FROM banned_identifiers "+
			"WHERE identifier_type = $1 AND identifier = $2 AND lifted_at IS NULL",
		identifierType, identifier).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count sanctions: %w", err)
	}
	return n, nil
}

// ListBans reads the bans f keeps, newest first.
func (r *PostgresBanRepository) ListBans(ctx context.Context, f BanFilter, now time.Time) ([]Ban, error) {
	var conds []string
	var args []any

	if f.Active {
		args = append(args, now)
		conds = append(conds, inForce("$"+strconv.Itoa(len(args))))
	}
	if f.BeforeID > 0 {
		args = append(args, f.BeforeID)
		conds = append(conds, "id < $"+strconv.Itoa(len(args)))
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, f.Limit)

	rows, err := r.pool.Query(ctx,
		"SELECT "+banColumns+" FROM banned_identifiers"+where+
			" ORDER BY id DESC LIMIT $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("select bans: %w", err)
	}

	bans, err := pgx.CollectRows(rows, scanBan)
	if err != nil {
		return nil, fmt.Errorf("read bans: %w", err)
	}
	return bans, nil
}

// GetBan reads one ban.
func (r *PostgresBanRepository) GetBan(ctx context.Context, id int64) (Ban, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+banColumns+" FROM banned_identifiers WHERE id = $1", id)
	if err != nil {
		return Ban{}, fmt.Errorf("select ban: %w", err)
	}
	return collectOne(rows, scanBan)
}

// LiftBan records that one ban was lifted.
func (r *PostgresBanRepository) LiftBan(ctx context.Context, id int64, at time.Time) (Ban, error) {
	rows, err := r.pool.Query(ctx,
		"UPDATE banned_identifiers SET lifted_at = $2 WHERE id = $1 RETURNING "+banColumns, id, at)
	if err != nil {
		return Ban{}, fmt.Errorf("update ban: %w", err)
	}
	return collectOne(rows, scanBan)
}

func scanBan(row pgx.CollectableRow) (Ban, error) {
	var (
		b                      Ban
		reason, conversationID *string
		bannedUntil, liftedAt  *time.Time
	)
	if err := row.Scan(&b.ID, &b.IdentifierType, &b.Identifier, &b.Sanction, &b.Source,
		&reason, &conversationID, &bannedUntil, &b.CreatedAt, &liftedAt); err != nil {
		return Ban{}, err
	}

	if reason != nil {
		b.Reason = *reason
	}
	if conversationID != nil {
		b.ConversationID = *conversationID
	}
	if bannedUntil != nil {
		b.BannedUntil = bannedUntil.UTC()
	}
	if liftedAt != nil {
		b.LiftedAt = liftedAt.UTC()
	}
	b.CreatedAt = b.CreatedAt.UTC()
	return b, nil
}

// nullIfEmpty writes an empty string as NULL.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullIfZero writes the zero time as NULL.
func nullIfZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
