// Package topic owns the catalogue of chat topics users can queue for.
// A topic is either active, and offered on the topic selection screen, or
// inactive, and kept only for the conversations that already reference it.
//
// The active list is read on every visit to the selection screen and changes
// rarely, so Service answers it from an in-process cache with a TTL; the
// reasoning is in docs/adr/0006-cache-the-topic-list-in-process.md.
//
// Boundary: other modules refer to a topic by its ID, not by importing
// this package's persistence models.
package topic

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// DefaultCacheTTL is how long the active list is answered from memory
	// before it is read from PostgreSQL again.
	DefaultCacheTTL = 5 * time.Minute

	// maxNameLength matches topics.name (VARCHAR(100)), which PostgreSQL
	// measures in characters.
	maxNameLength = 100
)

var (
	// ErrNotFound is returned when no topic has the requested ID.
	ErrNotFound = errors.New("topic: not found")

	// ErrInvalidName is returned for a name that is empty once trimmed or
	// longer than the column holds.
	ErrInvalidName = errors.New("topic: invalid name")

	// ErrInUse is returned when a topic cannot be deleted because
	// conversations still reference it. Deactivating it takes it off the
	// selection screen while those conversations keep their reference.
	ErrInUse = errors.New("topic: referenced by conversations")
)

// Topic is one choice on the selection screen.
type Topic struct {
	ID        int
	Name      string
	IsActive  bool
	CreatedAt time.Time
}

// Service reads and administers the topic catalogue.
type Service struct {
	repo  Repository
	cache *cache

	// loadMu lets a single caller refill the cache while the others wait for
	// its result, so an expired cache does not turn every concurrent visit to
	// the selection screen into its own query.
	loadMu sync.Mutex
}

// Options configures a Service. The zero value of each field selects the
// default described on the field.
type Options struct {
	// CacheTTL defaults to DefaultCacheTTL.
	CacheTTL time.Duration
}

// NewService builds a Service on top of repo.
func NewService(repo Repository, opts Options) *Service {
	if opts.CacheTTL <= 0 {
		opts.CacheTTL = DefaultCacheTTL
	}

	return &Service{
		repo:  repo,
		cache: newCache(opts.CacheTTL),
	}
}

// ListActive returns the topics offered to users, in catalogue order.
func (s *Service) ListActive(ctx context.Context) ([]Topic, error) {
	if topics, ok := s.cache.get(); ok {
		return topics, nil
	}

	s.loadMu.Lock()
	defer s.loadMu.Unlock()

	// Another caller may have refilled the cache while this one waited.
	if topics, ok := s.cache.get(); ok {
		return topics, nil
	}

	topics, err := s.repo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active topics: %w", err)
	}

	s.cache.set(topics)
	return slices.Clone(topics), nil
}

// IsActive reports whether users can be offered the topic with the given ID.
// It reads the same cached list the selection screen is drawn from.
func (s *Service) IsActive(ctx context.Context, id int) (bool, error) {
	topics, err := s.ListActive(ctx)
	if err != nil {
		return false, err
	}

	return slices.ContainsFunc(topics, func(t Topic) bool { return t.ID == id }), nil
}

// List returns every topic, active or not. It is read straight from
// PostgreSQL so that an administrator sees the effect of their own writes.
func (s *Service) List(ctx context.Context) ([]Topic, error) {
	topics, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	return topics, nil
}

// Create adds an active topic.
func (s *Service) Create(ctx context.Context, name string) (Topic, error) {
	normalized, err := normalizeName(name)
	if err != nil {
		return Topic{}, err
	}

	created, err := s.repo.Create(ctx, normalized)
	if err != nil {
		return Topic{}, fmt.Errorf("create topic: %w", err)
	}

	s.cache.invalidate()
	return created, nil
}

// Update carries the fields an administrator changes. A nil field is left as
// it is.
type Update struct {
	Name     *string
	IsActive *bool
}

// Update applies upd to the topic with the given ID and returns the result.
func (s *Service) Update(ctx context.Context, id int, upd Update) (Topic, error) {
	if upd.Name != nil {
		normalized, err := normalizeName(*upd.Name)
		if err != nil {
			return Topic{}, err
		}
		upd.Name = &normalized
	}

	updated, err := s.repo.Update(ctx, id, upd)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Topic{}, err
		}
		return Topic{}, fmt.Errorf("update topic: %w", err)
	}

	s.cache.invalidate()
	return updated, nil
}

// Delete removes a topic that no conversation references.
func (s *Service) Delete(ctx context.Context, id int) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInUse) {
			return err
		}
		return fmt.Errorf("delete topic: %w", err)
	}

	s.cache.invalidate()
	return nil
}

// normalizeName is the name as it is stored: surrounding whitespace removed,
// and short enough for the column.
func normalizeName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" || utf8.RuneCountInString(normalized) > maxNameLength {
		return "", ErrInvalidName
	}
	return normalized, nil
}
