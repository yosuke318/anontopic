package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// The daily batch counts, for each country and prefecture, how many users
// joined a conversation and how many were in a conversation that someone
// else in it reported, and keeps the counts in geo_daily_stats. The reasoning
// is in
// docs/adr/0045-locate-the-prefecture-of-a-session-with-geolite-downloaded-by-the-server.md.

// DefaultGeoDays is how many past days a run of SummarizeGeo counts, so that
// a day a failed run missed is filled in by a later one. It has to stay well
// below the 180 days the location of a participant is kept.
const DefaultGeoDays = 7

// Region is a country and a prefecture. Either is empty when it is not known,
// and the prefecture is empty outside Japan.
type Region struct {
	Country    string
	Prefecture string
}

// GeoStat is what one region counted on one day.
type GeoStat struct {
	Connections int64
	Reports     int64
}

// Report is one report a user made about a conversation.
type Report struct {
	ConversationID string
	ReporterToken  string
}

// Reports reads the reports made in a period.
type Reports interface {
	ReportsBetween(ctx context.Context, from, to time.Time) ([]Report, error)
}

// GeoRecords counts the participants of a period by region and keeps the
// counts of whole days.
type GeoRecords interface {
	CountByRegion(ctx context.Context, from, to time.Time, reports []Report) (map[Region]GeoStat, error)
	SaveGeoStats(ctx context.Context, date string, stats map[Region]GeoStat) error
}

// SummarizeGeo counts each of the days past days before the day now falls on
// and writes the counts into records. A day is written over as a whole, so a
// run can be repeated. A dry run counts and writes nothing. A day that fails
// is reported in the error and the other days are still counted.
func SummarizeGeo(ctx context.Context, records GeoRecords, reports Reports, now time.Time, days int, dryRun bool) (DailySummary, error) {
	summary := DailySummary{DryRun: dryRun}
	today := now.In(statZone)
	midnight := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, statZone)

	var errs []error
	for i := days; i >= 1; i-- {
		from, to := midnight.AddDate(0, 0, -i), midnight.AddDate(0, 0, -i+1)
		date := statDate(from)

		reported, err := reports.ReportsBetween(ctx, from, to)
		if err != nil {
			errs = append(errs, fmt.Errorf("read the reports of %s: %w", date, err))
			continue
		}
		stats, err := records.CountByRegion(ctx, from, to, reported)
		if err != nil {
			errs = append(errs, fmt.Errorf("count the participants of %s: %w", date, err))
			continue
		}
		if len(stats) == 0 {
			continue
		}

		if dryRun {
			slog.Info("would summarize the regions", slog.String("date", date), slog.Int("rows", len(stats)))
		} else if err := records.SaveGeoStats(ctx, date, stats); err != nil {
			errs = append(errs, fmt.Errorf("save the regions of %s: %w", date, err))
			continue
		}

		summary.Days = append(summary.Days, date)
		summary.Rows += len(stats)
	}
	return summary, errors.Join(errs...)
}

// CountByRegion counts by region the participants who joined from from up
// to, but not including, to, and the participants of the conversations in
// reports. A conversation reported more than once counts each of its
// participants once, and none of the users who reported it.
func (r *PostgresRepository) CountByRegion(ctx context.Context, from, to time.Time, reports []Report) (map[Region]GeoStat, error) {
	stats := make(map[Region]GeoStat)

	rows, err := r.pool.Query(ctx,
		"SELECT coalesce(country_code, ''), coalesce(prefecture, ''), count(*) "+
			"FROM conversation_participants WHERE joined_at >= $1 AND joined_at < $2 GROUP BY 1, 2",
		from, to)
	if err != nil {
		return nil, fmt.Errorf("count the participants by region: %w", err)
	}
	if err := collectRegions(rows, stats, func(s *GeoStat, n int64) { s.Connections = n }); err != nil {
		return nil, err
	}

	if len(reports) == 0 {
		return stats, nil
	}

	conversations := make([]string, len(reports))
	reporters := make([]string, len(reports))
	for i, rep := range reports {
		conversations[i] = rep.ConversationID
		reporters[i] = rep.ReporterToken
	}
	rows, err = r.pool.Query(ctx,
		"WITH r AS (SELECT * FROM unnest($1::uuid[], $2::text[]) AS r(conversation_id, reporter_token)) "+
			"SELECT coalesce(p.country_code, ''), coalesce(p.prefecture, ''), count(*) "+
			"FROM conversation_participants p "+
			"WHERE p.conversation_id IN (SELECT conversation_id FROM r) "+
			"AND NOT EXISTS (SELECT 1 FROM r "+
			"WHERE r.conversation_id = p.conversation_id AND r.reporter_token = p.session_token) "+
			"GROUP BY 1, 2",
		conversations, reporters)
	if err != nil {
		return nil, fmt.Errorf("count the reported participants by region: %w", err)
	}
	if err := collectRegions(rows, stats, func(s *GeoStat, n int64) { s.Reports = n }); err != nil {
		return nil, err
	}
	return stats, nil
}

// collectRegions reads rows of (country, prefecture, count) into stats with
// set.
func collectRegions(rows pgx.Rows, stats map[Region]GeoStat, set func(*GeoStat, int64)) error {
	defer rows.Close()
	for rows.Next() {
		var region Region
		var n int64
		if err := rows.Scan(&region.Country, &region.Prefecture, &n); err != nil {
			return fmt.Errorf("scan a region: %w", err)
		}
		s := stats[region]
		set(&s, n)
		stats[region] = s
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read the regions: %w", err)
	}
	return nil
}

// SaveGeoStats replaces the rows of date in geo_daily_stats with stats.
func (r *PostgresRepository) SaveGeoStats(ctx context.Context, date string, stats map[Region]GeoStat) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	batch := &pgx.Batch{}
	batch.Queue("DELETE FROM geo_daily_stats WHERE stat_date = $1::date", date)
	for region, s := range stats {
		batch.Queue(
			"INSERT INTO geo_daily_stats (stat_date, country_code, prefecture, connection_count, report_count) "+
				"VALUES ($1::date, $2, $3, $4, $5)",
			date, region.Country, region.Prefecture, s.Connections, s.Reports)
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("write geo_daily_stats: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit geo_daily_stats: %w", err)
	}
	return nil
}
