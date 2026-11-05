// Package activity writes to the log, at a fixed interval, how many users the
// service holds at once: those waiting for a room and those connected to a
// conversation. Next to that number it writes the largest one reached since
// the service last held nobody. The alarms in infra/modules/monitoring read
// that largest number out of the log and mail the operator when it reaches 1,
// 10 or 100, so that each of them is mailed once until the service is empty
// again. The reasoning is in
// docs/adr/0046-mail-the-operator-from-the-peak-number-of-users-written-to-the-log.md.
//
// Boundary: the users are counted by the modules that hold them, through
// Counter. This module keeps only the largest number in Redis.
package activity

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	// DefaultInterval is how often the number of users is counted and
	// written. It is short enough that a user who stays for less than a
	// minute is still counted in the minute the alarms look at.
	DefaultInterval = 10 * time.Second

	// countTimeout bounds one count, so that a slow store does not hold up
	// the counts that follow it.
	countTimeout = 5 * time.Second

	// logMessage is the message of the line the alarms read. The metric
	// filter in infra/modules/monitoring matches on it, so it must not
	// change without the filter.
	logMessage = "activity"
)

// Counter counts the users the service holds now: those waiting for a room
// and those connected to a conversation, together.
type Counter interface {
	Count(ctx context.Context) (int, error)
}

// Store keeps the largest number of users reached since the service last held
// nobody. Every server records into the same Store, so the number is the same
// whichever server wrote it.
type Store interface {
	// RecordPeak takes the number of users held now and returns the largest
	// number since the service last held nobody, this one included. Zero
	// starts it over.
	RecordPeak(ctx context.Context, users int) (int, error)
}

// Service counts the users at an interval and writes what it counted.
type Service struct {
	counter  Counter
	store    Store
	logger   *slog.Logger
	interval time.Duration
}

// Options configures a Service. The zero value of each field selects the
// default described on the field.
type Options struct {
	// Interval defaults to DefaultInterval.
	Interval time.Duration
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// NewService builds a Service.
func NewService(counter Counter, store Store, opts Options) *Service {
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &Service{counter: counter, store: store, logger: opts.Logger, interval: opts.Interval}
}

// Run counts and writes at every interval until ctx is done.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			countCtx, cancel := context.WithTimeout(ctx, countTimeout)
			err := s.Record(countCtx)
			cancel()

			if err != nil && ctx.Err() == nil {
				s.logger.Error("count the users", slog.Any("error", err))
			}
		}
	}
}

// Record counts the users once and writes the line the alarms read. Nothing
// is written when either the count or the store fails, so that a failure is
// not read as an empty service, which would start the largest number over.
func (s *Service) Record(ctx context.Context) error {
	users, err := s.counter.Count(ctx)
	if err != nil {
		return fmt.Errorf("count: %w", err)
	}

	peak, err := s.store.RecordPeak(ctx, users)
	if err != nil {
		return fmt.Errorf("record the peak: %w", err)
	}

	s.logger.Info(logMessage, slog.Int("users", users), slog.Int("peak", peak))
	return nil
}
