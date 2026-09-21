// Package moderation owns automated content checks applied to messages
// and the resulting enforcement actions.
//
// A message is judged before the room it was sent to sees it, against the NG
// word dictionary and the shapes an external contact detail is written in.
// The dictionary is held in memory and read again at an interval, so that
// judging a message does not wait on the database and the words the operators
// change still reach a running server; the reasoning is in
// docs/adr/0004-ng-word-dictionary-in-database.md.
//
// A message is folded into one form before it is read, so that kana, width,
// case, symbols written between the characters of a word and a character
// written over do not hide a word the dictionary holds; the reasoning is in
// docs/adr/0017-fold-a-message-into-one-form-before-matching-it.md.
//
// A contact detail is read from forms of its own, where the words written in
// place of "@" and "." and the kanji written in place of digits are read as
// what they stand for; the reasoning is in
// docs/adr/0019-read-a-contact-detail-from-forms-of-its-own.md.
//
// Boundary: it consumes message payloads passed in by the caller and does
// not read another module's storage on its own.
package moderation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
)

const (
	// DefaultReloadInterval is how often the dictionary is read again. It is
	// how long a word the operators add or switch off takes to apply.
	DefaultReloadInterval = 5 * time.Minute

	// loadTimeout bounds one read of the dictionary, so that a database that
	// stopped answering does not hold the reload loop.
	loadTimeout = 5 * time.Second
)

// ErrNoDictionary is returned by Moderate while no dictionary has been read.
// A message the service cannot judge is refused rather than delivered
// unjudged.
var ErrNoDictionary = errors.New("moderation: no dictionary has been loaded")

// Decision is what the filter says about the body of one message.
type Decision int

const (
	// DecisionAllow found nothing to act on.
	DecisionAllow Decision = iota
	// DecisionBlock keeps the message from the room it was sent to.
	DecisionBlock
)

// Category is the kind of prohibited use a message was blocked for. Its
// values are those of ng_words.category.
type Category string

const (
	// CategoryMeetup is meeting in person, or arranging to.
	CategoryMeetup Category = "meetup"
	// CategoryDating is seeking a partner or a date.
	CategoryDating Category = "dating"
	// CategorySexual is sexual content.
	CategorySexual Category = "sexual"
	// CategoryContact is an external contact detail, or asking for one.
	CategoryContact Category = "contact"
	// CategorySolicitation is soliciting and spam.
	CategorySolicitation Category = "solicitation"
)

// Verdict is what the filter decided about one message, and why.
type Verdict struct {
	Decision Decision
	// Category is what the message was blocked for. It is empty when the
	// message was allowed, and when it was refused because it could not be
	// judged.
	Category Category
}

// Service judges messages against the dictionary it holds.
type Service struct {
	repo     Repository
	interval time.Duration

	// dict is replaced whole by every load, so that a message is judged
	// against one dictionary without a lock on the path it takes to its room.
	dict atomic.Pointer[dictionary]
}

// Options configures a Service. The zero value of each field selects the
// default described on the field.
type Options struct {
	// ReloadInterval defaults to DefaultReloadInterval.
	ReloadInterval time.Duration
}

// NewService builds a Service holding no dictionary. Load fills it, and Run
// keeps it in step with the words that are stored.
func NewService(repo Repository, opts Options) *Service {
	if opts.ReloadInterval <= 0 {
		opts.ReloadInterval = DefaultReloadInterval
	}

	return &Service{repo: repo, interval: opts.ReloadInterval}
}

// Load reads the dictionary and makes it the one messages are judged against.
func (s *Service) Load(ctx context.Context) error {
	words, err := s.repo.ActiveWords(ctx)
	if err != nil {
		return fmt.Errorf("load ng words: %w", err)
	}

	s.dict.Store(newDictionary(words))
	slog.Info("ng word dictionary loaded", slog.Int("words", len(words)))

	return nil
}

// Run reads the dictionary again at the reload interval until ctx is over. A
// read that fails leaves the words the service already holds in force, so
// that a database it cannot reach does not stop it judging messages.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			loadCtx, cancel := context.WithTimeout(ctx, loadTimeout)
			err := s.Load(loadCtx)
			cancel()

			if err != nil && ctx.Err() == nil {
				slog.Error("reload ng word dictionary", slog.Any("error", err))
			}
		}
	}
}

// Moderate judges the body of one message. It reports ErrNoDictionary while
// there is nothing to judge against.
//
// A contact detail is looked for whatever the dictionary holds, because the
// strings it is written as are not words anyone could list.
func (s *Service) Moderate(_ context.Context, body string) (Verdict, error) {
	dict := s.dict.Load()
	if dict == nil {
		return Verdict{Decision: DecisionBlock}, ErrNoDictionary
	}

	folded := fold(body)
	if carriesContact(folded) {
		return Verdict{Decision: DecisionBlock, Category: CategoryContact}, nil
	}
	if category, ok := dict.find(folded); ok {
		return Verdict{Decision: DecisionBlock, Category: category}, nil
	}

	return Verdict{Decision: DecisionAllow}, nil
}
