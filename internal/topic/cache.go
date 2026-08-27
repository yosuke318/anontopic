package topic

import (
	"slices"
	"sync"
	"time"
)

// cache holds the active topic list for a fixed lifetime. Every server keeps
// its own copy, so a write is visible on the other servers once their copy
// expires.
type cache struct {
	ttl time.Duration
	now func() time.Time

	mu        sync.RWMutex
	topics    []Topic
	expiresAt time.Time
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl, now: time.Now}
}

// get returns a copy of the cached list while it is fresh, so that a caller
// cannot change what the next one reads.
func (c *cache) get() ([]Topic, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// A zero expiry means nothing has been cached, which an empty catalogue
	// must not be mistaken for.
	if c.expiresAt.IsZero() || !c.now().Before(c.expiresAt) {
		return nil, false
	}
	return slices.Clone(c.topics), true
}

// set makes topics the answer for the next ttl.
func (c *cache) set(topics []Topic) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.topics = slices.Clone(topics)
	c.expiresAt = c.now().Add(c.ttl)
}

// invalidate drops the cached list, so the next read goes to PostgreSQL.
func (c *cache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.topics = nil
	c.expiresAt = time.Time{}
}
