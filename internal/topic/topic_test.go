package topic

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// testClock lets a test move the cache past its TTL without waiting.
type testClock struct {
	current time.Time
}

func (c *testClock) Now() time.Time { return c.current }

func (c *testClock) Advance(d time.Duration) { c.current = c.current.Add(d) }

// fakeRepository is a Repository backed by a slice, counting how often the
// active list is read so that a test can see the cache work.
type fakeRepository struct {
	mu          sync.Mutex
	topics      []Topic
	nextID      int
	activeReads int
	deleteErr   error
}

func newFakeRepository(names ...string) *fakeRepository {
	repo := &fakeRepository{nextID: 1}
	for _, name := range names {
		repo.topics = append(repo.topics, Topic{
			ID:        repo.nextID,
			Name:      name,
			IsActive:  true,
			CreatedAt: time.Unix(0, 0).UTC(),
		})
		repo.nextID++
	}
	return repo
}

func (r *fakeRepository) ListActive(_ context.Context) ([]Topic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.activeReads++

	active := make([]Topic, 0, len(r.topics))
	for _, t := range r.topics {
		if t.IsActive {
			active = append(active, t)
		}
	}
	return active, nil
}

func (r *fakeRepository) List(_ context.Context) ([]Topic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]Topic(nil), r.topics...), nil
}

func (r *fakeRepository) Create(_ context.Context, name string) (Topic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	created := Topic{ID: r.nextID, Name: name, IsActive: true, CreatedAt: time.Unix(0, 0).UTC()}
	r.nextID++
	r.topics = append(r.topics, created)
	return created, nil
}

func (r *fakeRepository) Update(_ context.Context, id int, upd Update) (Topic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, t := range r.topics {
		if t.ID != id {
			continue
		}
		if upd.Name != nil {
			t.Name = *upd.Name
		}
		if upd.IsActive != nil {
			t.IsActive = *upd.IsActive
		}
		r.topics[i] = t
		return t, nil
	}
	return Topic{}, ErrNotFound
}

func (r *fakeRepository) Delete(_ context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.deleteErr != nil {
		return r.deleteErr
	}
	for i, t := range r.topics {
		if t.ID == id {
			r.topics = append(r.topics[:i], r.topics[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// newTestService builds a service whose cache runs on a clock the test moves.
func newTestService(t *testing.T, repo *fakeRepository) (*Service, *testClock) {
	t.Helper()

	svc := NewService(repo, Options{CacheTTL: time.Minute})
	clock := &testClock{current: time.Unix(0, 0).UTC()}
	svc.cache.now = clock.Now

	return svc, clock
}

func TestListActiveLeavesOutInactiveTopics(t *testing.T) {
	repo := newFakeRepository("雑談", "趣味")
	repo.topics[1].IsActive = false
	svc, _ := newTestService(t, repo)

	topics, err := svc.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	if len(topics) != 1 || topics[0].Name != "雑談" {
		t.Fatalf("ListActive = %v, want only 雑談", topics)
	}
}

func TestListActiveIsAnsweredFromTheCacheUntilItExpires(t *testing.T) {
	repo := newFakeRepository("雑談")
	svc, clock := newTestService(t, repo)
	ctx := context.Background()

	for range 3 {
		if _, err := svc.ListActive(ctx); err != nil {
			t.Fatalf("ListActive: %v", err)
		}
	}
	if repo.activeReads != 1 {
		t.Fatalf("repository reads = %d, want 1", repo.activeReads)
	}

	clock.Advance(time.Minute)
	if _, err := svc.ListActive(ctx); err != nil {
		t.Fatalf("ListActive after the TTL: %v", err)
	}
	if repo.activeReads != 2 {
		t.Fatalf("repository reads = %d, want 2", repo.activeReads)
	}
}

func TestConcurrentListActiveReadsTheRepositoryOnce(t *testing.T) {
	repo := newFakeRepository("雑談")
	svc, _ := newTestService(t, repo)
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.ListActive(ctx); err != nil {
				t.Errorf("ListActive: %v", err)
			}
		}()
	}
	wg.Wait()

	if repo.activeReads != 1 {
		t.Fatalf("repository reads = %d, want 1", repo.activeReads)
	}
}

func TestListActiveHandsOutACopyOfTheCachedList(t *testing.T) {
	repo := newFakeRepository("雑談")
	svc, _ := newTestService(t, repo)
	ctx := context.Background()

	topics, err := svc.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	topics[0].Name = "書き換え"

	again, err := svc.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if again[0].Name != "雑談" {
		t.Fatalf("cached name = %q, want 雑談", again[0].Name)
	}
}

func TestCreateShowsUpInTheNextListActive(t *testing.T) {
	repo := newFakeRepository("雑談")
	svc, _ := newTestService(t, repo)
	ctx := context.Background()

	if _, err := svc.ListActive(ctx); err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if _, err := svc.Create(ctx, "  ゲーム  "); err != nil {
		t.Fatalf("Create: %v", err)
	}

	topics, err := svc.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(topics) != 2 || topics[1].Name != "ゲーム" {
		t.Fatalf("ListActive = %v, want 雑談 and the trimmed ゲーム", topics)
	}
}

func TestCreateRejectsANameTheColumnCannotHold(t *testing.T) {
	svc, _ := newTestService(t, newFakeRepository())
	ctx := context.Background()

	names := map[string]string{
		"empty":      "",
		"whitespace": "   ",
		"too long":   strings.Repeat("あ", maxNameLength+1),
	}
	for what, name := range names {
		if _, err := svc.Create(ctx, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Create with a %s name: error = %v, want ErrInvalidName", what, err)
		}
	}
}

func TestUpdateDeactivatesATopicWithoutTouchingItsName(t *testing.T) {
	repo := newFakeRepository("雑談")
	svc, _ := newTestService(t, repo)
	ctx := context.Background()

	if _, err := svc.ListActive(ctx); err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	inactive := false
	updated, err := svc.Update(ctx, 1, Update{IsActive: &inactive})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "雑談" || updated.IsActive {
		t.Fatalf("Update = %+v, want 雑談 deactivated", updated)
	}

	topics, err := svc.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(topics) != 0 {
		t.Fatalf("ListActive = %v, want no topics", topics)
	}
}

func TestUpdateReportsAnUnknownTopic(t *testing.T) {
	svc, _ := newTestService(t, newFakeRepository())

	name := "ゲーム"
	if _, err := svc.Update(context.Background(), 404, Update{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update of an unknown topic: error = %v, want ErrNotFound", err)
	}
}

func TestDeleteReportsATopicConversationsReference(t *testing.T) {
	repo := newFakeRepository("雑談")
	repo.deleteErr = ErrInUse
	svc, _ := newTestService(t, repo)

	if err := svc.Delete(context.Background(), 1); !errors.Is(err, ErrInUse) {
		t.Fatalf("Delete: error = %v, want ErrInUse", err)
	}
}
