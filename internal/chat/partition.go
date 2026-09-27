package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dropLockTimeout bounds how long dropping a partition waits for the lock on
// messages. Dropping a partition locks the whole table, and every insert that
// arrives while the drop waits queues behind it.
const dropLockTimeout = 5 * time.Second

// ErrNotPartition is returned for a table name that is not a partition of
// messages.
var ErrNotPartition = errors.New("chat: not a partition of messages")

// MessagePartition is one partition of the messages table. It holds the
// messages created from From up to, but not including, To.
type MessagePartition struct {
	Name string
	// From and To are the zero time for a partition that is not bounded by a
	// range, such as a default partition.
	From time.Time
	To   time.Time
	// Bytes is the size of the partition together with its indexes.
	Bytes int64
}

// Partitions creates and drops the partitions of the messages table. Which
// partitions to create and which to drop is decided by the caller.
//
// Before a partition is dropped, the messages of the conversations the caller
// names are moved to retained_messages, where Transcript still reads them.
// The reasoning is in
// docs/adr/0026-drop-daily-message-partitions-and-move-reported-messages-aside.md.
type Partitions struct {
	pool *pgxpool.Pool
}

// NewPartitions builds Partitions on top of pool.
func NewPartitions(pool *pgxpool.Pool) *Partitions {
	return &Partitions{pool: pool}
}

// partitionBounds reads the name, the range and the size of every partition
// of messages. The range is read out of the partition bound expression, which
// PostgreSQL prints as FOR VALUES FROM ('...') TO ('...') for a range
// partition and without quoted values for any other.
const partitionBounds = `
SELECT
    c.relname,
    substring(pg_get_expr(c.relpartbound, c.oid) FROM $re$FROM \('([^']+)'\)$re$)::timestamptz,
    substring(pg_get_expr(c.relpartbound, c.oid) FROM $re$TO \('([^']+)'\)$re$)::timestamptz,
    pg_total_relation_size(c.oid)
FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
WHERE i.inhparent = 'messages'::regclass
ORDER BY 2 NULLS LAST, 1`

// List returns every partition of messages, oldest range first.
func (p *Partitions) List(ctx context.Context) ([]MessagePartition, error) {
	rows, err := p.pool.Query(ctx, partitionBounds)
	if err != nil {
		return nil, fmt.Errorf("select message partitions: %w", err)
	}

	partitions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MessagePartition, error) {
		var (
			part     MessagePartition
			from, to *time.Time
		)
		if err := row.Scan(&part.Name, &from, &to, &part.Bytes); err != nil {
			return MessagePartition{}, err
		}
		if from != nil && to != nil {
			part.From, part.To = from.UTC(), to.UTC()
		}
		return part, nil
	})
	if err != nil {
		return nil, fmt.Errorf("read message partitions: %w", err)
	}
	return partitions, nil
}

// Create adds a partition of messages for the range from from up to to.
func (p *Partitions) Create(ctx context.Context, from, to time.Time) (MessagePartition, error) {
	from, to = from.UTC(), to.UTC()
	name := partitionName(from)

	// DDL takes no bind parameters. The bounds are formatted here from
	// time.Time values and the name is quoted as an identifier.
	_, err := p.pool.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE %s PARTITION OF messages FOR VALUES FROM ('%s') TO ('%s')",
		pgx.Identifier{name}.Sanitize(), from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano)))
	if err != nil {
		return MessagePartition{}, fmt.Errorf("create message partition %s: %w", name, err)
	}

	return MessagePartition{Name: name, From: from, To: to}, nil
}

// partitionName names the partition that starts at from after the UTC day it
// starts on. A partition that starts in the middle of a day, which only
// happens when it fills a gap next to a partition of another size, also
// carries the time it starts at.
func partitionName(from time.Time) string {
	if from.Equal(from.Truncate(24 * time.Hour)) {
		return "messages_" + from.Format("20060102")
	}
	return "messages_" + from.Format("20060102_1504")
}

// Conversations returns the id of every conversation that has a message in
// the partition name names.
func (p *Partitions) Conversations(ctx context.Context, name string) ([]string, error) {
	table, err := partitionTable(ctx, p.pool, name)
	if err != nil {
		return nil, err
	}

	rows, err := p.pool.Query(ctx, "SELECT DISTINCT conversation_id::text FROM "+table)
	if err != nil {
		return nil, fmt.Errorf("select conversations of %s: %w", name, err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read conversations of %s: %w", name, err)
	}
	return ids, nil
}

// CountMessages returns how many messages of the given conversations the
// partition name names holds, which is how many Drop would move.
func (p *Partitions) CountMessages(ctx context.Context, name string, conversationIDs []string) (int64, error) {
	table, err := partitionTable(ctx, p.pool, name)
	if err != nil {
		return 0, err
	}

	var n int64
	if err := p.pool.QueryRow(ctx,
		"SELECT count(*) FROM "+table+" WHERE conversation_id = ANY($1::uuid[])", conversationIDs,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count messages of %s: %w", name, err)
	}
	return n, nil
}

// Drop moves the messages of the conversations in keep out of the partition
// name names into retained_messages, then drops the partition, and returns
// how many messages it moved. Both happen in one transaction, so a partition
// is never gone without the messages it had to keep.
func (p *Partitions) Drop(ctx context.Context, name string, keep []string) (int64, error) {
	var moved int64

	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			"SELECT set_config('lock_timeout', $1, true)",
			fmt.Sprintf("%dms", dropLockTimeout.Milliseconds())); err != nil {
			return fmt.Errorf("set lock timeout: %w", err)
		}

		table, err := partitionTable(ctx, tx, name)
		if err != nil {
			return err
		}

		// A message moved by a run that failed after this statement is
		// already there, so a later run moves nothing twice.
		tag, err := tx.Exec(ctx,
			"INSERT INTO retained_messages (id, conversation_id, sender_token, body, moderation_flag, created_at) "+
				"SELECT id, conversation_id, sender_token, body, moderation_flag, created_at FROM "+table+
				" WHERE conversation_id = ANY($1::uuid[]) ON CONFLICT (id) DO NOTHING", keep)
		if err != nil {
			return fmt.Errorf("move messages of %s: %w", name, err)
		}
		moved = tag.RowsAffected()

		if _, err := tx.Exec(ctx, "DROP TABLE "+table); err != nil {
			return fmt.Errorf("drop %s: %w", name, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return moved, nil
}

// querier is what partitionTable needs of a pool or a transaction.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// partitionTable returns name quoted for use in SQL, after checking that it
// is a partition of messages. The check keeps a wrong name from reaching a
// statement that drops the table it names.
func partitionTable(ctx context.Context, q querier, name string) (string, error) {
	var table string
	err := q.QueryRow(ctx,
		"SELECT c.oid::regclass::text FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid "+
			"WHERE i.inhparent = 'messages'::regclass AND c.relname = $1", name,
	).Scan(&table)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", ErrNotPartition, name)
	}
	if err != nil {
		return "", fmt.Errorf("look up partition %s: %w", name, err)
	}
	return table, nil
}
