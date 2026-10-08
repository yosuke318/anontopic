// Command retention applies the retention policy to the database once and
// exits. It is meant to be started by a scheduler late at night.
//
// Usage:
//
//	retention            パーティションを作成・削除し、期限を過ぎた参加者の識別子を消す
//	retention -dry-run   作成・削除・消去の予定だけをログに出し、何も変えない
//
// The command exits with status 1 when any step failed, and logs every failure
// with the message "retention failed", which is what an alert should watch.
// The reasoning is in
// docs/adr/0026-drop-daily-message-partitions-and-move-reported-messages-aside.md
// and docs/adr/0037-keep-only-hashed-sender-identifiers-and-erase-them-after-180-days.md.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yosuke318/anontopic/internal/chat"
	"github.com/yosuke318/anontopic/internal/matching"
	"github.com/yosuke318/anontopic/internal/report"
	"github.com/yosuke318/anontopic/internal/retention"
)

const defaultDatabaseURL = "postgres://anontopic:anontopic@localhost:5432/anontopic?sslmode=disable"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	dryRun := flag.Bool("dry-run", false, "log what would be created and dropped, and change nothing")
	flag.Parse()

	if err := run(*dryRun); err != nil {
		slog.Error("retention failed", slog.Bool("dry_run", *dryRun), slog.Any("error", err))
		os.Exit(1)
	}
}

func run(dryRun bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	svc := retention.NewService(
		messagePartitions{chat.NewPartitions(pool)},
		matching.NewPostgresRepository(pool),
		report.NewService(report.NewPostgresRepository(pool), nil, nil),
		retention.Options{
			MessageDays:    envInt("RETENTION_MESSAGE_DAYS", retention.DefaultMessageDays),
			IdentifierDays: envInt("RETENTION_IDENTIFIER_DAYS", retention.DefaultIdentifierDays),
			AheadDays:      envInt("RETENTION_AHEAD_DAYS", retention.DefaultAheadDays),
		},
	)

	res, err := svc.Run(ctx, time.Now(), dryRun)

	var retained int64
	for _, d := range res.Dropped {
		retained += d.Retained
	}
	slog.Info("retention finished",
		slog.Bool("dry_run", res.DryRun),
		slog.Int("created_partitions", len(res.Created)),
		slog.Int("dropped_partitions", len(res.Dropped)),
		slog.Int64("retained_messages", retained),
		slog.Int("partitions", res.Partitions),
		slog.Int64("partition_bytes", res.Bytes),
		slog.Time("covered_until", res.CoveredUntil),
		slog.Int("erased_conversations", res.ErasedConversations),
		slog.Int64("erased_participants", res.ErasedParticipants))

	return err
}

// messagePartitions carries what the retention module asks about the
// partitions of messages to the chat module that owns the table.
type messagePartitions struct {
	partitions *chat.Partitions
}

func (m messagePartitions) Partitions(ctx context.Context) ([]retention.Partition, error) {
	list, err := m.partitions.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]retention.Partition, 0, len(list))
	for _, p := range list {
		out = append(out, retention.Partition(p))
	}
	return out, nil
}

func (m messagePartitions) CreatePartition(ctx context.Context, from, to time.Time) (retention.Partition, error) {
	p, err := m.partitions.Create(ctx, from, to)
	return retention.Partition(p), err
}

func (m messagePartitions) Conversations(ctx context.Context, partition string) ([]string, error) {
	return m.partitions.Conversations(ctx, partition)
}

func (m messagePartitions) CountMessages(ctx context.Context, partition string, conversationIDs []string) (int64, error) {
	return m.partitions.CountMessages(ctx, partition, conversationIDs)
}

func (m messagePartitions) DropPartition(ctx context.Context, partition string, keep []string) (int64, error) {
	return m.partitions.Drop(ctx, partition, keep)
}

func envInt(key string, fallback int) int {
	n, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return n
}
