package chat

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// partitionHolding returns the name of the partition whose range holds at.
func partitionHolding(t *testing.T, partitions *Partitions, at time.Time) string {
	t.Helper()

	list, err := partitions.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range list {
		if !at.Before(p.From) && at.Before(p.To) {
			return p.Name
		}
	}
	t.Fatalf("no partition holds %v", at)
	return ""
}

// testPartition creates a partition for one day far enough ahead that no
// message of the running service reaches it, and drops it once the test is
// over unless the test dropped it itself.
func testPartition(t *testing.T, partitions *Partitions) MessagePartition {
	t.Helper()

	from := time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, rand.IntN(10000))
	part, err := partitions.Create(t.Context(), from, from.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Cleanup(func() {
		if _, err := partitions.pool.Exec(context.Background(),
			"DROP TABLE IF EXISTS "+pgx.Identifier{part.Name}.Sanitize()); err != nil {
			t.Errorf("clean up the partition: %v", err)
		}
	})

	return part
}

func TestPartitionsCreateAPartitionForTheRangeTheyAreGiven(t *testing.T) {
	repo := postgresTestRepository(t)
	partitions := NewPartitions(repo.pool)
	part := testPartition(t, partitions)

	if part.Name != "messages_"+part.From.Format("20060102") {
		t.Fatalf("created %s, want it named after the day it starts on", part.Name)
	}

	list, err := partitions.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	i := slices.IndexFunc(list, func(p MessagePartition) bool { return p.Name == part.Name })
	if i < 0 {
		t.Fatalf("List = %+v, want it to hold %s", list, part.Name)
	}
	if !list[i].From.Equal(part.From) || !list[i].To.Equal(part.To) {
		t.Fatalf("read the range as %v to %v, want %v to %v", list[i].From, list[i].To, part.From, part.To)
	}
	if list[i].Bytes < 0 {
		t.Fatalf("read the size as %d", list[i].Bytes)
	}

	if !slices.IsSortedFunc(list, func(a, b MessagePartition) int { return a.From.Compare(b.From) }) {
		t.Fatal("the partitions were not listed oldest range first")
	}
}

func TestPartitionsMoveTheMessagesToKeepBeforeDroppingAPartition(t *testing.T) {
	repo := postgresTestRepository(t)
	partitions := NewPartitions(repo.pool)
	ctx := t.Context()

	part := testPartition(t, partitions)
	reported := testConversation(t, repo, "reporter-token", "other-token")
	unreported := testConversation(t, repo, "first-token", "second-token")

	sentAt := part.From.Add(time.Hour)
	if err := repo.AddMessages(ctx, []Message{
		{ConversationID: reported, SenderToken: "reporter-token", Body: "やめてください", Flag: moderationFlagClean, CreatedAt: sentAt},
		{ConversationID: reported, SenderToken: "other-token", Body: "会いませんか", Flag: moderationFlagReported, CreatedAt: sentAt.Add(time.Second)},
		{ConversationID: unreported, SenderToken: "first-token", Body: "こんにちは", Flag: moderationFlagClean, CreatedAt: sentAt},
	}); err != nil {
		t.Fatalf("AddMessages: %v", err)
	}

	ids, err := partitions.Conversations(ctx, part.Name)
	if err != nil {
		t.Fatalf("Conversations: %v", err)
	}
	slices.Sort(ids)
	want := []string{reported, unreported}
	slices.Sort(want)
	if !slices.Equal(ids, want) {
		t.Fatalf("Conversations = %v, want %v", ids, want)
	}

	n, err := partitions.CountMessages(ctx, part.Name, []string{reported})
	if err != nil {
		t.Fatalf("CountMessages: %v", err)
	}
	if n != 2 {
		t.Fatalf("CountMessages = %d, want the 2 messages of the reported conversation", n)
	}

	moved, err := partitions.Drop(ctx, part.Name, []string{reported})
	if err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if moved != 2 {
		t.Fatalf("Drop moved %d messages, want 2", moved)
	}

	list, err := partitions.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if slices.ContainsFunc(list, func(p MessagePartition) bool { return p.Name == part.Name }) {
		t.Fatalf("%s is still there after Drop", part.Name)
	}

	tr, err := repo.Transcript(ctx, reported)
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	if len(tr.Messages) != 2 || tr.Messages[0].Body != "やめてください" || tr.Messages[1].Flag != moderationFlagReported {
		t.Fatalf("Transcript of the reported conversation = %+v, want both messages as they were recorded", tr.Messages)
	}
	if !tr.Messages[0].CreatedAt.Equal(sentAt) {
		t.Fatalf("read the first message as sent at %v, want %v", tr.Messages[0].CreatedAt, sentAt)
	}

	tr, err = repo.Transcript(ctx, unreported)
	if err != nil {
		t.Fatalf("Transcript: %v", err)
	}
	if len(tr.Messages) != 0 {
		t.Fatalf("Transcript of the other conversation = %+v, want its messages gone", tr.Messages)
	}
}

func TestPartitionsRefuseATableThatIsNoPartitionOfMessages(t *testing.T) {
	repo := postgresTestRepository(t)
	partitions := NewPartitions(repo.pool)

	if _, err := partitions.Drop(t.Context(), "conversations", nil); !errors.Is(err, ErrNotPartition) {
		t.Fatalf("Drop(conversations) = %v, want %v", err, ErrNotPartition)
	}
	if _, err := partitions.Conversations(t.Context(), "messages"); !errors.Is(err, ErrNotPartition) {
		t.Fatalf("Conversations(messages) = %v, want %v", err, ErrNotPartition)
	}

	var exists bool
	if err := repo.pool.QueryRow(t.Context(),
		"SELECT to_regclass('conversations') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatalf("look up conversations: %v", err)
	}
	if !exists {
		t.Fatal("conversations was dropped")
	}
}

func TestPartitionNameCarriesTheTimeOfAPartitionThatStartsInsideADay(t *testing.T) {
	tests := map[time.Time]string{
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC): "messages_20261001",
		time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC): "messages_20261001_0900",
	}
	for from, want := range tests {
		if got := partitionName(from); got != want {
			t.Errorf("partitionName(%v) = %q, want %q", from, got, want)
		}
	}
}
