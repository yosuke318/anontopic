package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
)

// stubCounter answers the counts it holds, one per call.
type stubCounter struct {
	counts []int
	err    error
}

func (c *stubCounter) Count(context.Context) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n := c.counts[0]
	c.counts = c.counts[1:]
	return n, nil
}

// memoryStore keeps the largest number in memory, by the rules RedisStore
// follows.
type memoryStore struct {
	peak int
	err  error
}

func (s *memoryStore) RecordPeak(_ context.Context, users int) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	if users == 0 {
		s.peak = 0
	} else if users > s.peak {
		s.peak = users
	}
	return s.peak, nil
}

// line is the part of a written line the alarms read.
type line struct {
	Msg   string `json:"msg"`
	Users int    `json:"users"`
	Peak  int    `json:"peak"`
}

func newTestService(counter Counter, store Store) (*Service, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	return NewService(counter, store, Options{Logger: logger}), &buf
}

func readLines(t *testing.T, buf *bytes.Buffer) []line {
	t.Helper()

	var lines []line
	dec := json.NewDecoder(buf)
	for dec.More() {
		var l line
		if err := dec.Decode(&l); err != nil {
			t.Fatalf("decode a written line: %v", err)
		}
		lines = append(lines, l)
	}
	return lines
}

func TestRecordWritesTheUsersAndTheLargestNumberSinceTheServiceWasEmpty(t *testing.T) {
	counter := &stubCounter{counts: []int{1, 12, 4, 0, 3}}
	svc, buf := newTestService(counter, &memoryStore{})

	for range 5 {
		if err := svc.Record(t.Context()); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got := readLines(t, buf)
	want := []line{
		{Msg: "activity", Users: 1, Peak: 1},
		{Msg: "activity", Users: 12, Peak: 12},
		// Fewer users keep the largest number, so the alarm for 10 is not
		// mailed again while the service stays busy.
		{Msg: "activity", Users: 4, Peak: 12},
		{Msg: "activity", Users: 0, Peak: 0},
		{Msg: "activity", Users: 3, Peak: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("lines = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestRecordWritesNothingWhenTheCountFails(t *testing.T) {
	store := &memoryStore{peak: 12}
	svc, buf := newTestService(&stubCounter{err: errors.New("redis is down")}, store)

	if err := svc.Record(t.Context()); err == nil {
		t.Fatal("Record succeeded with a count that failed")
	}

	// An empty line would read as an empty service and start the largest
	// number over.
	if buf.Len() != 0 {
		t.Fatalf("wrote %q, want nothing", buf.String())
	}
	if store.peak != 12 {
		t.Fatalf("peak = %d, want the 12 it held", store.peak)
	}
}

func TestRecordWritesNothingWhenTheStoreFails(t *testing.T) {
	svc, buf := newTestService(&stubCounter{counts: []int{3}}, &memoryStore{err: errors.New("redis is down")})

	if err := svc.Record(t.Context()); err == nil {
		t.Fatal("Record succeeded with a store that failed")
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %q, want nothing", buf.String())
	}
}
