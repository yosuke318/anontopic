package matching

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

// fakeGeo counts nothing itself: it answers the stats of each day from
// stats, and records the days it was asked for and what was saved.
type fakeGeo struct {
	stats   map[string]map[Region]GeoStat
	failing map[string]bool
	asked   []string
	saved   map[string]map[Region]GeoStat
}

func (f *fakeGeo) ReportsBetween(_ context.Context, from, _ time.Time) ([]Report, error) {
	if f.failing[statDate(from)] {
		return nil, errors.New("refused")
	}
	return nil, nil
}

func (f *fakeGeo) CountByRegion(_ context.Context, from, to time.Time, _ []Report) (map[Region]GeoStat, error) {
	if to.Sub(from) != 24*time.Hour || from.In(statZone).Hour() != 0 {
		return nil, fmt.Errorf("asked for %v to %v, want one day from midnight in Japan", from, to)
	}
	date := statDate(from)
	f.asked = append(f.asked, date)
	return f.stats[date], nil
}

func (f *fakeGeo) SaveGeoStats(_ context.Context, date string, stats map[Region]GeoStat) error {
	f.saved[date] = stats
	return nil
}

func TestSummarizeGeoCountsEachPastDayOnce(t *testing.T) {
	tokyo := Region{Country: "JP", Prefecture: "JP-13"}
	f := &fakeGeo{
		stats: map[string]map[Region]GeoStat{
			"2026-10-25": {tokyo: {Connections: 2}},
			"2026-10-31": {tokyo: {Connections: 4, Reports: 1}},
		},
		failing: map[string]bool{"2026-10-30": true},
		saved:   map[string]map[Region]GeoStat{},
	}
	// 2026-11-01 09:00 in Japan.
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	summary, err := SummarizeGeo(t.Context(), f, f, now, DefaultGeoDays, false)
	if err == nil {
		t.Fatal("err = nil, want the failure of 2026-10-30")
	}
	wantAsked := []string{"2026-10-25", "2026-10-26", "2026-10-27", "2026-10-28", "2026-10-29", "2026-10-31"}
	if !slices.Equal(f.asked, wantAsked) {
		t.Fatalf("asked = %v, want %v", f.asked, wantAsked)
	}
	if want := []string{"2026-10-25", "2026-10-31"}; !slices.Equal(summary.Days, want) {
		t.Fatalf("days = %v, want %v", summary.Days, want)
	}
	if got := f.saved["2026-10-31"][tokyo]; got != (GeoStat{Connections: 4, Reports: 1}) {
		t.Fatalf("saved = %+v", got)
	}
}

func TestCountByRegionCountsTheParticipantsAndTheReportedOnes(t *testing.T) {
	repo := postgresTestRepository(t)
	ctx := t.Context()

	// The day sits far in the past, apart from the rows other tests and the
	// seed data write.
	from := time.Date(2001, 3, 1, 0, 0, 0, 0, statZone)
	to := from.AddDate(0, 0, 1)

	var topicID int
	if err := repo.pool.QueryRow(ctx,
		"INSERT INTO topics (name, is_active) VALUES ($1, false) RETURNING id",
		fmt.Sprintf("test-%d", rand.Int64())).Scan(&topicID); err != nil {
		t.Fatalf("insert topic: %v", err)
	}

	tokyo := Location{Country: "JP", Prefecture: "JP-13", ASN: 2516}
	osaka := Location{Country: "JP", Prefecture: "JP-27", ASN: 17676}
	reporter := fmt.Sprintf("reporter-%d", rand.Int64())
	second := fmt.Sprintf("b-%d", rand.Int64())

	conv, err := repo.CreateConversation(ctx, topicID, []Participant{
		{Token: reporter, Location: tokyo},
		{Token: second, Location: osaka},
		{Token: fmt.Sprintf("c-%d", rand.Int64())},
	})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = repo.pool.Exec(ctx, "DELETE FROM conversation_participants WHERE conversation_id = $1", conv.ID)
		_, _ = repo.pool.Exec(ctx, "DELETE FROM conversations WHERE id = $1", conv.ID)
		_, _ = repo.pool.Exec(ctx, "DELETE FROM topics WHERE id = $1", topicID)
		_, _ = repo.pool.Exec(ctx, "DELETE FROM geo_daily_stats WHERE stat_date = '2001-03-01'")
	})
	if _, err := repo.pool.Exec(ctx,
		"UPDATE conversation_participants SET joined_at = $2 WHERE conversation_id = $1",
		conv.ID, from.Add(time.Hour)); err != nil {
		t.Fatalf("set joined_at: %v", err)
	}

	stats, err := repo.CountByRegion(ctx, from, to, []Report{{ConversationID: conv.ID, ReporterToken: reporter}})
	if err != nil {
		t.Fatalf("CountByRegion: %v", err)
	}
	// The reporter is not counted as reported.
	want := map[Region]GeoStat{
		{Country: "JP", Prefecture: "JP-13"}: {Connections: 1},
		{Country: "JP", Prefecture: "JP-27"}: {Connections: 1, Reports: 1},
		{}:                                   {Connections: 1, Reports: 1},
	}
	if len(stats) != len(want) {
		t.Fatalf("stats = %+v, want %+v", stats, want)
	}
	for region, s := range want {
		if stats[region] != s {
			t.Fatalf("stats[%+v] = %+v, want %+v", region, stats[region], s)
		}
	}

	// Reported by two of its participants, the conversation counts each of
	// the others once, and neither of the reporters.
	stats, err = repo.CountByRegion(ctx, from, to, []Report{
		{ConversationID: conv.ID, ReporterToken: reporter},
		{ConversationID: conv.ID, ReporterToken: second},
	})
	if err != nil {
		t.Fatalf("CountByRegion: %v", err)
	}
	for region, wantReports := range map[Region]int64{
		{Country: "JP", Prefecture: "JP-13"}: 0,
		{Country: "JP", Prefecture: "JP-27"}: 0,
		{}:                                   1,
	} {
		if got := stats[region].Reports; got != wantReports {
			t.Fatalf("reports of %+v = %d, want %d", region, got, wantReports)
		}
	}

	if err := repo.SaveGeoStats(ctx, "2001-03-01", stats); err != nil {
		t.Fatalf("SaveGeoStats: %v", err)
	}
	// Saving the day again replaces it.
	if err := repo.SaveGeoStats(ctx, "2001-03-01", map[Region]GeoStat{{}: {Connections: 9}}); err != nil {
		t.Fatalf("SaveGeoStats again: %v", err)
	}
	var rows, connections int
	if err := repo.pool.QueryRow(ctx,
		"SELECT count(*), coalesce(sum(connection_count), 0) FROM geo_daily_stats WHERE stat_date = '2001-03-01'").
		Scan(&rows, &connections); err != nil {
		t.Fatalf("read geo_daily_stats: %v", err)
	}
	if rows != 1 || connections != 9 {
		t.Fatalf("rows = %d, connections = %d, want the second save only", rows, connections)
	}
}

func TestEraseIdentifiersClearsTheLocationToo(t *testing.T) {
	repo := postgresTestRepository(t)
	ctx := t.Context()

	cutoff := time.Date(2000, 7, 1, 0, 0, 0, 0, time.UTC)
	conv := testIdentifiedConversation(t, repo, cutoff.Add(-48*time.Hour))

	// A participant with only a location left is still found and cleared.
	if _, err := repo.pool.Exec(ctx,
		"UPDATE conversation_participants SET ip_hash = NULL, device_fingerprint = NULL, "+
			"country_code = 'JP', prefecture = 'JP-13', asn = 2516 WHERE conversation_id = $1", conv); err != nil {
		t.Fatalf("set location: %v", err)
	}

	ids, err := repo.IdentifiedBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("IdentifiedBefore: %v", err)
	}
	if !slices.Contains(ids, conv) {
		t.Fatalf("IdentifiedBefore = %v, want %s", ids, conv)
	}
	if n, err := repo.EraseIdentifiers(ctx, cutoff, []string{conv}); err != nil || n != 2 {
		t.Fatalf("EraseIdentifiers = %d, %v, want 2 participants", n, err)
	}

	var located int
	if err := repo.pool.QueryRow(ctx,
		"SELECT count(*) FROM conversation_participants WHERE conversation_id = $1 "+
			"AND (country_code IS NOT NULL OR prefecture IS NOT NULL OR asn IS NOT NULL)",
		conv).Scan(&located); err != nil {
		t.Fatalf("count located participants: %v", err)
	}
	if located != 0 {
		t.Fatalf("%d participants keep their location, want none", located)
	}
}
