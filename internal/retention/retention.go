// Package retention owns scheduled deletion of expired data (rooms,
// messages, logs) according to the service's retention policy.
//
// Messages are kept in daily partitions of the messages table. A run creates
// the partitions the days ahead need, and drops each partition whose every
// message is older than the retention period. The messages of a reported
// conversation are moved aside before their partition is dropped, and kept.
// The reasoning is in
// docs/adr/0026-drop-daily-message-partitions-and-move-reported-messages-aside.md.
//
// Boundary: each module exposes its own purge operation; retention
// orchestrates them and never deletes other modules' rows itself. The
// partitions belong to the chat module and are reached through Messages, and
// which conversations were reported is asked of the report module through
// Reports.
package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"
)

const (
	// DefaultMessageDays is how many days a message is kept. The service
	// promises its users that conversations are deleted after 90 days.
	DefaultMessageDays = 90

	// DefaultAheadDays is how many days from today a run makes sure there are
	// partitions for. A message whose time no partition holds cannot be
	// recorded, so this is also how long the runs can keep failing before
	// messages stop being recorded.
	DefaultAheadDays = 14

	day = 24 * time.Hour
)

// Partition is one partition of the messages table, holding the messages
// created from From up to, but not including, To.
type Partition struct {
	// Name is empty for a partition a dry run would create.
	Name string
	// From and To are the zero time for a partition that is not bounded by a
	// range. Such a partition is never dropped.
	From  time.Time
	To    time.Time
	Bytes int64
}

// holds reports whether at falls in the range of p.
func (p Partition) holds(at time.Time) bool {
	return !at.Before(p.From) && at.Before(p.To)
}

// bounded reports whether p has a range.
func (p Partition) bounded() bool {
	return !p.From.IsZero() && !p.To.IsZero()
}

// Messages is what retention asks of the module that owns the messages table.
type Messages interface {
	// Partitions returns every partition of the messages table.
	Partitions(ctx context.Context) ([]Partition, error)

	// CreatePartition adds a partition for the range from from up to to.
	CreatePartition(ctx context.Context, from, to time.Time) (Partition, error)

	// Conversations returns the id of every conversation that has a message
	// in the partition.
	Conversations(ctx context.Context, partition string) ([]string, error)

	// CountMessages returns how many messages of the given conversations the
	// partition holds.
	CountMessages(ctx context.Context, partition string, conversationIDs []string) (int64, error)

	// DropPartition keeps the messages of the conversations in keep and drops
	// the partition, returning how many messages it kept.
	DropPartition(ctx context.Context, partition string, keep []string) (int64, error)
}

// Reports is what retention asks of the module that takes reports.
type Reports interface {
	// Reported returns the conversations among conversationIDs that were
	// reported.
	Reported(ctx context.Context, conversationIDs []string) ([]string, error)
}

// Options tunes a Service. A field left at zero takes its default.
type Options struct {
	MessageDays int
	AheadDays   int
}

// Service runs the retention policy.
type Service struct {
	messages    Messages
	reports     Reports
	messageDays int
	aheadDays   int
}

// NewService builds a Service that manages the partitions of messages and
// keeps the conversations reports says were reported.
func NewService(messages Messages, reports Reports, opts Options) *Service {
	if opts.MessageDays <= 0 {
		opts.MessageDays = DefaultMessageDays
	}
	if opts.AheadDays <= 0 {
		opts.AheadDays = DefaultAheadDays
	}
	return &Service{
		messages:    messages,
		reports:     reports,
		messageDays: opts.MessageDays,
		aheadDays:   opts.AheadDays,
	}
}

// Dropped is a partition a run dropped, or a dry run would drop.
type Dropped struct {
	Partition Partition
	// Retained is how many messages of reported conversations were moved out
	// of the partition before it was dropped.
	Retained int64
}

// Result is what one run did, or what a dry run would do.
type Result struct {
	DryRun  bool
	Created []Partition
	Dropped []Dropped
	// Partitions is how many partitions are left.
	Partitions int
	// Bytes is the size of the partitions left.
	Bytes int64
	// CoveredUntil is where the partitions that run on without a gap from
	// the time of the run end. A message created after it cannot be recorded.
	CoveredUntil time.Time
}

// Run applies the retention policy as of now. A dry run reads what it would
// do and changes nothing.
//
// Partitions are created before any is dropped, since a missing partition
// stops messages from being recorded. A step that fails does not stop the
// ones after it; every failure is returned together once the run is over.
func (s *Service) Run(ctx context.Context, now time.Time, dryRun bool) (Result, error) {
	now = now.UTC()
	res := Result{DryRun: dryRun}

	partitions, err := s.messages.Partitions(ctx)
	if err != nil {
		return res, fmt.Errorf("list message partitions: %w", err)
	}

	var errs []error

	for _, plan := range planCreate(partitions, now, s.aheadDays) {
		created := plan
		if !dryRun {
			created, err = s.messages.CreatePartition(ctx, plan.From, plan.To)
			if err != nil {
				slog.Error("create message partition",
					slog.Time("from", plan.From), slog.Time("to", plan.To), slog.Any("error", err))
				errs = append(errs, err)
				continue
			}
		}

		slog.Info("create message partition",
			slog.Bool("dry_run", dryRun), slog.String("partition", created.Name),
			slog.Time("from", created.From), slog.Time("to", created.To))
		res.Created = append(res.Created, created)
		partitions = append(partitions, created)
	}

	cutoff := now.Add(-time.Duration(s.messageDays) * day)
	for _, p := range partitions {
		if !p.bounded() || p.To.After(cutoff) {
			res.Partitions++
			res.Bytes += p.Bytes
			continue
		}

		retained, err := s.drop(ctx, p, dryRun)
		if err != nil {
			slog.Error("drop message partition", slog.String("partition", p.Name), slog.Any("error", err))
			errs = append(errs, fmt.Errorf("drop %s: %w", p.Name, err))
			res.Partitions++
			res.Bytes += p.Bytes
			continue
		}

		slog.Info("drop message partition",
			slog.Bool("dry_run", dryRun), slog.String("partition", p.Name),
			slog.Time("from", p.From), slog.Time("to", p.To),
			slog.Int64("bytes", p.Bytes), slog.Int64("retained_messages", retained))
		res.Dropped = append(res.Dropped, Dropped{Partition: p, Retained: retained})
	}

	remaining := slices.DeleteFunc(slices.Clone(partitions), func(p Partition) bool {
		return slices.ContainsFunc(res.Dropped, func(d Dropped) bool { return d.Partition.Name == p.Name })
	})
	res.CoveredUntil = coveredUntil(remaining, now)

	return res, errors.Join(errs...)
}

// drop keeps the messages of the reported conversations in p and drops it,
// or in a dry run counts the messages it would keep.
func (s *Service) drop(ctx context.Context, p Partition, dryRun bool) (int64, error) {
	ids, err := s.messages.Conversations(ctx, p.Name)
	if err != nil {
		return 0, fmt.Errorf("list conversations: %w", err)
	}

	keep, err := s.reports.Reported(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("ask which conversations were reported: %w", err)
	}

	if dryRun {
		return s.messages.CountMessages(ctx, p.Name, keep)
	}
	return s.messages.DropPartition(ctx, p.Name, keep)
}

// planCreate returns the ranges the partitions lack between the start of the
// UTC day now falls on and aheadDays later. Each range is one UTC day, cut
// short where it would overlap a partition that is already there.
func planCreate(partitions []Partition, now time.Time, aheadDays int) []Partition {
	bounded := slices.DeleteFunc(slices.Clone(partitions), func(p Partition) bool { return !p.bounded() })
	slices.SortFunc(bounded, func(a, b Partition) int { return a.From.Compare(b.From) })

	start := now.Truncate(day)
	horizon := start.Add(time.Duration(aheadDays) * day)

	var plan []Partition
	for at := start; at.Before(horizon); {
		if i := slices.IndexFunc(bounded, func(p Partition) bool { return p.holds(at) }); i >= 0 {
			at = bounded[i].To
			continue
		}

		end := at.Truncate(day).Add(day)
		if i := slices.IndexFunc(bounded, func(p Partition) bool { return p.From.After(at) }); i >= 0 && bounded[i].From.Before(end) {
			end = bounded[i].From
		}

		plan = append(plan, Partition{From: at, To: end})
		at = end
	}
	return plan
}

// coveredUntil returns where the partitions that run on without a gap from
// now end, or now itself when no partition holds it.
func coveredUntil(partitions []Partition, now time.Time) time.Time {
	at := now
	for {
		i := slices.IndexFunc(partitions, func(p Partition) bool { return p.bounded() && p.holds(at) })
		if i < 0 {
			return at
		}
		at = partitions[i].To
	}
}
