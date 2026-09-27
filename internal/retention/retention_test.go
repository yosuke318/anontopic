package retention

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// fakeMessages keeps partitions in memory, with the conversations each one
// holds messages of and how many.
type fakeMessages struct {
	partitions []Partition
	// messages counts the messages of each conversation in each partition.
	messages map[string]map[string]int64
	// retained counts the messages DropPartition kept, per conversation.
	retained map[string]int64

	createErr error
	dropErr   map[string]error
}

func (f *fakeMessages) Partitions(context.Context) ([]Partition, error) {
	return slices.Clone(f.partitions), nil
}

func (f *fakeMessages) CreatePartition(_ context.Context, from, to time.Time) (Partition, error) {
	if f.createErr != nil {
		return Partition{}, f.createErr
	}
	p := Partition{Name: "messages_" + from.Format("20060102"), From: from, To: to}
	f.partitions = append(f.partitions, p)
	return p, nil
}

func (f *fakeMessages) Conversations(_ context.Context, partition string) ([]string, error) {
	var ids []string
	for id := range f.messages[partition] {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

func (f *fakeMessages) CountMessages(_ context.Context, partition string, conversationIDs []string) (int64, error) {
	var n int64
	for _, id := range conversationIDs {
		n += f.messages[partition][id]
	}
	return n, nil
}

func (f *fakeMessages) DropPartition(_ context.Context, partition string, keep []string) (int64, error) {
	if err := f.dropErr[partition]; err != nil {
		return 0, err
	}

	var n int64
	for _, id := range keep {
		if f.retained == nil {
			f.retained = map[string]int64{}
		}
		f.retained[id] += f.messages[partition][id]
		n += f.messages[partition][id]
	}
	delete(f.messages, partition)
	f.partitions = slices.DeleteFunc(f.partitions, func(p Partition) bool { return p.Name == partition })
	return n, nil
}

// fakeReports answers that the conversations it holds were reported.
type fakeReports struct {
	reported []string
	asked    [][]string
}

func (f *fakeReports) Reported(_ context.Context, conversationIDs []string) ([]string, error) {
	f.asked = append(f.asked, conversationIDs)
	var out []string
	for _, id := range conversationIDs {
		if slices.Contains(f.reported, id) {
			out = append(out, id)
		}
	}
	return out, nil
}

var now = time.Date(2026, 9, 27, 18, 30, 0, 0, time.UTC)

func date(month time.Month, d int) time.Time {
	return time.Date(2026, month, d, 0, 0, 0, 0, time.UTC)
}

// daily returns a partition for each day from from up to to.
func daily(from, to time.Time) []Partition {
	var out []Partition
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		out = append(out, Partition{Name: "messages_" + d.Format("20060102"), From: d, To: d.AddDate(0, 0, 1), Bytes: 100})
	}
	return out
}

func TestRunDropsThePartitionsOlderThanTheRetentionPeriodAndKeepsReportedConversations(t *testing.T) {
	// The cutoff is 90 days before now: 2026-06-29 18:30. The partition of
	// 06-28 ends before it; the one of 06-29 still holds messages younger.
	messages := &fakeMessages{
		partitions: daily(date(6, 27), date(10, 11)),
		messages: map[string]map[string]int64{
			"messages_20260627": {"reported": 3, "unreported": 5},
			"messages_20260628": {"unreported": 2},
			"messages_20260629": {"reported": 1},
		},
	}
	reports := &fakeReports{reported: []string{"reported"}}

	res, err := NewService(messages, reports, Options{}).Run(t.Context(), now, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var dropped []string
	for _, d := range res.Dropped {
		dropped = append(dropped, d.Partition.Name)
	}
	if !slices.Equal(dropped, []string{"messages_20260627", "messages_20260628"}) {
		t.Fatalf("dropped %v, want the partitions of 06-27 and 06-28", dropped)
	}
	if res.Dropped[0].Retained != 3 || res.Dropped[1].Retained != 0 {
		t.Fatalf("retained %d and %d messages, want 3 and 0", res.Dropped[0].Retained, res.Dropped[1].Retained)
	}
	if messages.retained["reported"] != 3 || messages.retained["unreported"] != 0 {
		t.Fatalf("kept %v, want only the 3 messages of the reported conversation", messages.retained)
	}

	// Only the conversations of a partition are asked about.
	if !slices.Equal(reports.asked[0], []string{"reported", "unreported"}) {
		t.Fatalf("asked about %v first, want the conversations of 06-27", reports.asked[0])
	}

	if len(res.Created) != 0 {
		t.Fatalf("created %+v, want none while the partitions reach far enough", res.Created)
	}
	if res.Partitions != len(messages.partitions) || res.Bytes != int64(100*len(messages.partitions)) {
		t.Fatalf("counted %d partitions of %d bytes left, want %d of %d",
			res.Partitions, res.Bytes, len(messages.partitions), 100*len(messages.partitions))
	}
	if !res.CoveredUntil.Equal(date(10, 11)) {
		t.Fatalf("covered until %v, want %v", res.CoveredUntil, date(10, 11))
	}
}

func TestRunCreatesTheDailyPartitionsTheDaysAheadLack(t *testing.T) {
	// A monthly partition holds the rest of September, as the migrations
	// leave it.
	messages := &fakeMessages{partitions: []Partition{
		{Name: "messages_202609", From: date(9, 1), To: date(10, 1)},
		{Name: "messages_20261003", From: date(10, 3), To: date(10, 4)},
	}}

	res, err := NewService(messages, &fakeReports{}, Options{AheadDays: 8}).Run(t.Context(), now, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var got []string
	for _, p := range res.Created {
		got = append(got, p.From.Format("01-02")+"/"+p.To.Format("01-02"))
	}
	want := []string{"10-01/10-02", "10-02/10-03", "10-04/10-05"}
	if !slices.Equal(got, want) {
		t.Fatalf("created %v, want %v", got, want)
	}
	if !res.CoveredUntil.Equal(date(10, 5)) {
		t.Fatalf("covered until %v, want %v", res.CoveredUntil, date(10, 5))
	}
}

func TestPlanCreateStopsARangeShortOfAPartitionThatStartsInsideTheDay(t *testing.T) {
	existing := []Partition{
		{Name: "messages_202609", From: date(9, 1), To: date(9, 28).Add(9 * time.Hour)},
		{Name: "messages_20260929_1500", From: date(9, 29).Add(15 * time.Hour), To: date(9, 30)},
	}

	var got []string
	for _, p := range planCreate(existing, now, 3) {
		got = append(got, p.From.Format("01-02 15:04")+"/"+p.To.Format("01-02 15:04"))
	}
	want := []string{
		"09-28 09:00/09-29 00:00",
		"09-29 00:00/09-29 15:00",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("planned %v, want %v", got, want)
	}
}

func TestADryRunChangesNothing(t *testing.T) {
	messages := &fakeMessages{
		partitions: daily(date(6, 27), date(6, 28)),
		messages: map[string]map[string]int64{
			"messages_20260627": {"reported": 3, "unreported": 5},
		},
	}
	before := slices.Clone(messages.partitions)

	res, err := NewService(messages, &fakeReports{reported: []string{"reported"}}, Options{}).Run(t.Context(), now, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !slices.Equal(messages.partitions, before) || messages.retained != nil {
		t.Fatalf("a dry run changed the partitions to %+v and kept %v", messages.partitions, messages.retained)
	}
	if !res.DryRun || len(res.Dropped) != 1 || res.Dropped[0].Retained != 3 {
		t.Fatalf("Result = %+v, want the partition it would drop and the 3 messages it would keep", res)
	}
	if len(res.Created) != DefaultAheadDays {
		t.Fatalf("would create %d partitions, want %d", len(res.Created), DefaultAheadDays)
	}
	if !res.CoveredUntil.Equal(now.Truncate(day).AddDate(0, 0, DefaultAheadDays)) {
		t.Fatalf("would cover until %v", res.CoveredUntil)
	}
}

func TestRunGoesOnPastAPartitionItFailedToDrop(t *testing.T) {
	failure := errors.New("lock timeout")
	messages := &fakeMessages{
		partitions: daily(date(6, 26), date(10, 11)),
		dropErr:    map[string]error{"messages_20260626": failure},
	}

	res, err := NewService(messages, &fakeReports{}, Options{}).Run(t.Context(), now, false)
	if !errors.Is(err, failure) {
		t.Fatalf("Run = %v, want the failure to drop 06-26", err)
	}
	if len(res.Dropped) != 2 || res.Dropped[0].Partition.Name != "messages_20260627" {
		t.Fatalf("dropped %+v, want the partitions after the one that failed", res.Dropped)
	}
}

func TestRunReportsAPartitionItFailedToCreate(t *testing.T) {
	failure := errors.New("permission denied")
	messages := &fakeMessages{createErr: failure}

	res, err := NewService(messages, &fakeReports{}, Options{AheadDays: 2}).Run(t.Context(), now, false)
	if !errors.Is(err, failure) {
		t.Fatalf("Run = %v, want the failure to create", err)
	}
	if len(res.Created) != 0 || !res.CoveredUntil.Equal(now) {
		t.Fatalf("Result = %+v, want nothing created and nothing covered", res)
	}
}

func TestRunNeverDropsAPartitionWithoutARange(t *testing.T) {
	messages := &fakeMessages{partitions: append(
		daily(date(9, 27), date(10, 11)),
		Partition{Name: "messages_default", Bytes: 7},
	)}

	res, err := NewService(messages, &fakeReports{}, Options{}).Run(t.Context(), now, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Dropped) != 0 {
		t.Fatalf("dropped %+v, want nothing", res.Dropped)
	}
	if res.Bytes != 14*100+7 {
		t.Fatalf("counted %d bytes left, want the default partition among them", res.Bytes)
	}
}
