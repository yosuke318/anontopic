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

func TestRedisStoreCountsTheDemandOfAQueue(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 3)
	ctx := context.Background()
	now := time.Now().UTC()

	first := testToken(t, store, "first")
	second := testToken(t, store, "second")
	third := testToken(t, store, "third")
	gaveUp := testToken(t, store, "gave-up")
	enqueue(t, store, first, q, now.Add(-90*time.Second))
	enqueue(t, store, second, q, now.Add(-30*time.Second))
	enqueue(t, store, third, q, now.Add(-10*time.Second))
	enqueue(t, store, gaveUp, q, now)
	// A repeated request keeps the place the user holds and is not counted.
	enqueue(t, store, first, q, now)

	participants, err := store.FormRoom(ctx, q, now, time.Minute*5, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}
	if len(participants) != 3 {
		t.Fatalf("participants = %v, want three", participants)
	}
	if err := store.Remove(ctx, gaveUp, now); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Leaving when no longer waiting is not counted.
	if err := store.Remove(ctx, gaveUp, now); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	demand, err := store.Demand(ctx, statDate(now))
	if err != nil {
		t.Fatalf("Demand: %v", err)
	}
	want := Demand{Queued: 4, Matched: 3, Left: 1, Wait: 130 * time.Second}
	if got := demand[q]; got != want {
		t.Fatalf("demand = %+v, want %+v", got, want)
	}
}

func TestRedisStoreCountsARoomOfTwoUnderTheQueueOfThree(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 3)
	ctx := context.Background()
	now := time.Now().UTC()

	enqueue(t, store, testToken(t, store, "first"), q, now.Add(-2*time.Minute))
	enqueue(t, store, testToken(t, store, "second"), q, now.Add(-2*time.Minute))

	participants, err := store.FormRoom(ctx, q, now, 5*time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}
	if len(participants) != 2 {
		t.Fatalf("participants = %v, want two", participants)
	}

	demand, err := store.Demand(ctx, statDate(now))
	if err != nil {
		t.Fatalf("Demand: %v", err)
	}
	if got := demand[q]; got.Matched != 2 {
		t.Fatalf("matched under the queue of three = %d, want 2", got.Matched)
	}
	if _, ok := demand[Queue{TopicID: q.TopicID, RoomType: 2}]; ok {
		t.Fatal("the room of two was counted under the queue of two")
	}
}

func TestDemandDateCutsTheDayInJapan(t *testing.T) {
	cases := []struct {
		at   time.Time
		want string
	}{
		{time.Date(2026, 11, 1, 14, 59, 59, 0, time.UTC), "2026-11-01"},
		{time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC), "2026-11-02"},
	}
	for _, c := range cases {
		if got := statDate(c.at); got != c.want {
			t.Errorf("statDate(%v) = %s, want %s", c.at, got, c.want)
		}
	}
}

// fakeDemand holds the demand of each date in memory and records what was
// saved.
type fakeDemand struct {
	counts  map[string]map[Queue]Demand
	failing map[string]bool
	saved   map[string]map[Queue]Demand
}

func (f *fakeDemand) Demand(_ context.Context, date string) (map[Queue]Demand, error) {
	return f.counts[date], nil
}

func (f *fakeDemand) SaveDemand(_ context.Context, date string, demand map[Queue]Demand) error {
	if f.failing[date] {
		return errors.New("refused")
	}
	f.saved[date] = demand
	return nil
}

func TestSummarizeDemandCopiesThePastDays(t *testing.T) {
	q := Queue{TopicID: 1, RoomType: 2}
	f := &fakeDemand{
		counts: map[string]map[Queue]Demand{
			"2026-10-24": {q: {Queued: 1}}, // eight days before, out of the run
			"2026-10-25": {q: {Queued: 2}},
			"2026-10-30": {q: {Queued: 3}},
			"2026-10-31": {q: {Queued: 4}},
			"2026-11-01": {q: {Queued: 5}}, // today, not over yet
		},
		failing: map[string]bool{"2026-10-30": true},
		saved:   map[string]map[Queue]Demand{},
	}
	// 2026-11-01 09:00 in Japan.
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	summary, err := SummarizeDemand(t.Context(), f, f, now, DefaultDemandDays, false)
	if err == nil {
		t.Fatal("err = nil, want the failure of 2026-10-30")
	}
	if want := []string{"2026-10-25", "2026-10-31"}; !slices.Equal(summary.Days, want) {
		t.Fatalf("days = %v, want %v", summary.Days, want)
	}
	if len(f.saved) != 2 || f.saved["2026-10-31"][q].Queued != 4 {
		t.Fatalf("saved = %v, want 2026-10-25 and 2026-10-31", f.saved)
	}
}

func TestSummarizeDemandWritesNothingInADryRun(t *testing.T) {
	q := Queue{TopicID: 1, RoomType: 2}
	f := &fakeDemand{
		counts: map[string]map[Queue]Demand{"2026-10-31": {q: {Queued: 4}}},
		saved:  map[string]map[Queue]Demand{},
	}
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	summary, err := SummarizeDemand(t.Context(), f, f, now, DefaultDemandDays, true)
	if err != nil {
		t.Fatalf("SummarizeDemand: %v", err)
	}
	if summary.Rows != 1 || len(f.saved) != 0 {
		t.Fatalf("rows = %d, saved = %v, want one row and nothing saved", summary.Rows, f.saved)
	}
}

func TestSaveDemandReplacesTheRowsOfADay(t *testing.T) {
	repo := postgresTestRepository(t)
	ctx := t.Context()

	var topicID int
	if err := repo.pool.QueryRow(ctx,
		"INSERT INTO topics (name, is_active) VALUES ($1, false) RETURNING id",
		fmt.Sprintf("test-%d", rand.Int64())).Scan(&topicID); err != nil {
		t.Fatalf("insert topic: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = repo.pool.Exec(ctx, "DELETE FROM topic_daily_stats WHERE topic_id = $1", topicID)
		_, _ = repo.pool.Exec(ctx, "DELETE FROM topics WHERE id = $1", topicID)
	})

	q := Queue{TopicID: topicID, RoomType: 3}
	if err := repo.SaveDemand(ctx, "2026-10-31", map[Queue]Demand{q: {Queued: 1}}); err != nil {
		t.Fatalf("SaveDemand: %v", err)
	}
	want := Demand{Queued: 5, Matched: 3, Left: 1, Wait: 90 * time.Second}
	if err := repo.SaveDemand(ctx, "2026-10-31", map[Queue]Demand{q: want}); err != nil {
		t.Fatalf("SaveDemand again: %v", err)
	}

	var got Demand
	var waitMS int64
	if err := repo.pool.QueryRow(ctx,
		"SELECT queued_count, matched_count, left_count, total_wait_ms FROM topic_daily_stats "+
			"WHERE stat_date = '2026-10-31' AND topic_id = $1 AND room_type = 3", topicID).
		Scan(&got.Queued, &got.Matched, &got.Left, &waitMS); err != nil {
		t.Fatalf("read topic_daily_stats: %v", err)
	}
	got.Wait = time.Duration(waitMS) * time.Millisecond
	if got != want {
		t.Fatalf("row = %+v, want %+v", got, want)
	}
}
