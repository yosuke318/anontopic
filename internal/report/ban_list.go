package report

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ipHashIdentifier is the identifier_type of a ban that applies to the hashed
// address a session was issued to.
const ipHashIdentifier = "ip_hash"

// PostgresBanList reads the banned_identifiers table.
type PostgresBanList struct {
	pool *pgxpool.Pool
}

// NewPostgresBanList builds a ban list on top of pool.
func NewPostgresBanList(pool *pgxpool.Pool) *PostgresBanList {
	return &PostgresBanList{pool: pool}
}

// IsBanned reports whether a ban applies to ipHash. A ban with no
// banned_until never lifts.
func (b *PostgresBanList) IsBanned(ctx context.Context, ipHash string) (bool, error) {
	var banned bool
	err := b.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM banned_identifiers "+
			"WHERE identifier_type = $1 AND identifier = $2 "+
			"AND (banned_until IS NULL OR banned_until > now()))",
		ipHashIdentifier, ipHash).Scan(&banned)
	if err != nil {
		return false, fmt.Errorf("read ban list: %w", err)
	}
	return banned, nil
}
