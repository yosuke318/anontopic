package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// The demand for a topic is counted per queue and per day in Redis while
// users wait, and the daily batch copies the counts of the past days into the
// topic_daily_stats table. Nothing is recorded per user. The reasoning is in
// docs/adr/0043-count-the-topic-demand-in-redis-and-keep-only-daily-totals.md.

// The counters a day keeps for each queue. The scripts in redis_store.go
// write them by the same names.
const (
	demandQueued  = "queued"
	demandMatched = "matched"
	demandLeft    = "left"
	demandWaitMS  = "wait_ms"
)

// statZone is the time zone the days of the daily statistics are cut in.
// Japan keeps no daylight saving time, so a fixed offset needs no time zone
// database in the container.
var statZone = time.FixedZone("JST", 9*60*60)

// DefaultDemandDays is how many past days a run of SummarizeDemand copies.
// It has to stay below the days demandTTL keeps a day in Redis.
const DefaultDemandDays = 7

// Demand is what happened in one queue during one day.
type Demand struct {
	// Queued counts the users who entered the queue. A user who enters again
	// is counted again.
	Queued int64
	// Matched counts the users taken out of the queue into a room.
	Matched int64
	// Left counts the users who gave up their wait themselves. The users who
	// are neither matched nor left are the ones whose wait ran out, give or
	// take the users whose wait crosses midnight.
	Left int64
	// Wait is the summed wait of the matched users.
	Wait time.Duration
}

// DemandCounts reads the demand counted for one day.
type DemandCounts interface {
	Demand(ctx context.Context, date string) (map[Queue]Demand, error)
}

// DemandRecords keeps the demand of whole days.
type DemandRecords interface {
	SaveDemand(ctx context.Context, date string, demand map[Queue]Demand) error
}

// DailySummary is what one run of SummarizeDemand or SummarizeGeo did.
type DailySummary struct {
	DryRun bool
	// Days lists the dates that had any demand to copy.
	Days []string
	// Rows counts the rows written, or that would be written in a dry run.
	Rows int
}

// SummarizeDemand copies the demand of each of the days past days before the
// day now falls on, from counts into records. A day is written over as a
// whole, so a run can be repeated, and a day Redis no longer holds is left as
// it was. A dry run reads the counts and writes nothing. A day that fails is
// reported in the error and the other days are still copied.
func SummarizeDemand(ctx context.Context, counts DemandCounts, records DemandRecords, now time.Time, days int, dryRun bool) (DailySummary, error) {
	summary := DailySummary{DryRun: dryRun}
	today := now.In(statZone)

	var errs []error
	for i := days; i >= 1; i-- {
		date := statDate(today.AddDate(0, 0, -i))

		demand, err := counts.Demand(ctx, date)
		if err != nil {
			errs = append(errs, fmt.Errorf("read the demand of %s: %w", date, err))
			continue
		}
		if len(demand) == 0 {
			continue
		}

		if dryRun {
			slog.Info("would summarize the demand", slog.String("date", date), slog.Int("rows", len(demand)))
		} else if err := records.SaveDemand(ctx, date, demand); err != nil {
			errs = append(errs, fmt.Errorf("save the demand of %s: %w", date, err))
			continue
		}

		summary.Days = append(summary.Days, date)
		summary.Rows += len(demand)
	}
	return summary, errors.Join(errs...)
}

// statDate is the day t falls on, as the daily statistics are kept under.
func statDate(t time.Time) string { return t.In(statZone).Format(time.DateOnly) }

// demandFieldPrefix starts the name of every counter q keeps in the hash of
// a day.
func demandFieldPrefix(q Queue) string {
	return strconv.Itoa(q.TopicID) + ":" + strconv.Itoa(q.RoomType) + ":"
}

// Demand reads the counters of every queue for date.
func (s *RedisStore) Demand(ctx context.Context, date string) (map[Queue]Demand, error) {
	fields, err := s.client.HGetAll(ctx, demandKeyPrefix+date).Result()
	if err != nil {
		return nil, fmt.Errorf("read the demand of %s: %w", date, err)
	}

	demand := make(map[Queue]Demand)
	for field, value := range fields {
		q, counter, err := parseDemandField(field)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse the demand counter %s: %w", field, err)
		}

		d := demand[q]
		switch counter {
		case demandQueued:
			d.Queued = n
		case demandMatched:
			d.Matched = n
		case demandLeft:
			d.Left = n
		case demandWaitMS:
			d.Wait = time.Duration(n) * time.Millisecond
		default:
			return nil, fmt.Errorf("unknown demand counter %s", field)
		}
		demand[q] = d
	}
	return demand, nil
}

// parseDemandField reads the queue and the counter a field of the hash of a
// day names.
func parseDemandField(field string) (Queue, string, error) {
	parts := strings.Split(field, ":")
	if len(parts) != 3 {
		return Queue{}, "", fmt.Errorf("malformed demand counter %s", field)
	}

	topicID, err := strconv.Atoi(parts[0])
	if err != nil {
		return Queue{}, "", fmt.Errorf("parse the topic of the demand counter %s: %w", field, err)
	}
	roomType, err := strconv.Atoi(parts[1])
	if err != nil {
		return Queue{}, "", fmt.Errorf("parse the room type of the demand counter %s: %w", field, err)
	}
	return Queue{TopicID: topicID, RoomType: roomType}, parts[2], nil
}

// SaveDemand writes the demand of every queue for date into
// topic_daily_stats, replacing the rows the day already has.
func (r *PostgresRepository) SaveDemand(ctx context.Context, date string, demand map[Queue]Demand) error {
	batch := &pgx.Batch{}
	for q, d := range demand {
		batch.Queue(
			"INSERT INTO topic_daily_stats "+
				"(stat_date, topic_id, room_type, queued_count, matched_count, left_count, total_wait_ms) "+
				"VALUES ($1::date, $2, $3, $4, $5, $6, $7) "+
				"ON CONFLICT (stat_date, topic_id, room_type) DO UPDATE SET "+
				"queued_count = EXCLUDED.queued_count, matched_count = EXCLUDED.matched_count, "+
				"left_count = EXCLUDED.left_count, total_wait_ms = EXCLUDED.total_wait_ms, updated_at = now()",
			date, q.TopicID, q.RoomType, d.Queued, d.Matched, d.Left, d.Wait.Milliseconds())
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("write topic_daily_stats: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit topic_daily_stats: %w", err)
	}
	return nil
}
