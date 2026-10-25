package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// stats counts what happened to the virtual users and keeps the durations
// the summary reports percentiles of.
type stats struct {
	live atomic.Int64
	peak atomic.Int64

	mu        sync.Mutex
	counts    map[string]int
	durations map[string][]time.Duration
	started   time.Time
}

func newStats() *stats {
	return &stats{
		counts:    map[string]int{},
		durations: map[string][]time.Duration{},
		started:   time.Now(),
	}
}

func (s *stats) inc(name string) {
	s.mu.Lock()
	s.counts[name]++
	s.mu.Unlock()
}

// fail counts a user who stopped at step. A refused request is counted by its
// status code, so that the hard cap (503) and the per-address cap (429) read
// apart from errors.
func (s *stats) fail(step string, err error) {
	name := step + "_failed"
	var se *statusError
	if errors.As(err, &se) {
		name = fmt.Sprintf("%s_%d", se.step, se.status)
	}
	s.inc(name)
}

func (s *stats) observe(name string, d time.Duration) {
	s.mu.Lock()
	s.durations[name] = append(s.durations[name], d)
	s.mu.Unlock()
}

func (s *stats) connected(delta int64) {
	n := s.live.Add(delta)
	for {
		p := s.peak.Load()
		if n <= p || s.peak.CompareAndSwap(p, n) {
			return
		}
	}
}

func (s *stats) progress() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("elapsed=%s connected=%d peak=%d matched=%d sent=%d received=%d",
		time.Since(s.started).Round(time.Second), s.live.Load(), s.peak.Load(),
		s.counts["matched"], s.counts["sent"], s.counts["event_message"])
}

func (s *stats) summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "\n=== summary (%s) ===\n", time.Since(s.started).Round(time.Second))
	fmt.Fprintf(&b, "peak connections: %d\n\n", s.peak.Load())

	b.WriteString("counts:\n")
	for _, name := range slices.Sorted(maps.Keys(s.counts)) {
		fmt.Fprintf(&b, "  %-28s %d\n", name, s.counts[name])
	}

	b.WriteString("\nlatency:\n")
	for _, name := range slices.Sorted(maps.Keys(s.durations)) {
		d := slices.Clone(s.durations[name])
		slices.Sort(d)
		fmt.Fprintf(&b, "  %-12s n=%d p50=%s p90=%s p99=%s max=%s\n", name, len(d),
			percentile(d, 0.50), percentile(d, 0.90), percentile(d, 0.99), d[len(d)-1])
	}
	return b.String()
}

// percentile reads the q-th quantile from durations sorted in ascending order.
func percentile(sorted []time.Duration, q float64) time.Duration {
	i := int(q * float64(len(sorted)-1))
	return sorted[i].Round(time.Millisecond)
}

// sentRegistry remembers when each message was sent and by whom, so that the
// user who reads it can tell how long it took to arrive. A message carries
// only a key written in Latin letters: a run of digits in its body would be
// read as a telephone number and blocked.
type sentRegistry struct {
	mu   sync.Mutex
	next uint64
	sent map[string]sentMessage
}

type sentMessage struct {
	at     time.Time
	sender *user
}

func newSentRegistry() *sentRegistry {
	return &sentRegistry{sent: map[string]sentMessage{}}
}

const bodyPrefix = "負荷試験 "

// record makes the body of the next message of sender and notes the time it
// is sent. The body is padded with kana to about length characters.
func (r *sentRegistry) record(sender *user, length int) string {
	r.mu.Lock()
	r.next++
	key := letters(r.next)
	r.sent[key] = sentMessage{at: time.Now(), sender: sender}
	r.mu.Unlock()

	body := bodyPrefix + key + " "
	if pad := length - len([]rune(body)); pad > 0 {
		body += strings.Repeat("あ", pad)
	}
	return body
}

// lookup finds the message body was made for. own is true when reader sent it.
func (r *sentRegistry) lookup(body string, reader *user) (at time.Time, own, ok bool) {
	rest, found := strings.CutPrefix(body, bodyPrefix)
	if !found {
		return time.Time{}, false, false
	}
	key, _, _ := strings.Cut(rest, " ")

	r.mu.Lock()
	m, ok := r.sent[key]
	r.mu.Unlock()
	return m.at, m.sender == reader, ok
}

// letters writes n in base 26 with the letters a to z.
func letters(n uint64) string {
	var b []byte
	for n > 0 {
		n--
		b = append(b, byte('a'+n%26))
		n /= 26
	}
	slices.Reverse(b)
	return string(b)
}
