package chat

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

const (
	// writeQueueBatches is how many batches the buffer holds. It takes more
	// than one, so that senders keep filling the buffer while a batch is
	// being written.
	writeQueueBatches = 4

	// writeTimeout bounds one write of a batch. It outlasts a slow write, and
	// gives up on one that would hold back everything buffered behind it.
	writeTimeout = 5 * time.Second
)

// errWriterClosed is returned for a message handed to a writer that is
// stopping.
var errWriterClosed = errors.New("chat: the message writer is closed")

// messageWriter records the messages of the rooms one server holds, off the
// path a message takes to its room. Messages are buffered and written in
// batches; the reasoning is in
// docs/adr/0012-buffer-message-writes-into-batched-inserts.md.
type messageWriter struct {
	repo     Repository
	batch    int
	interval time.Duration

	queue   chan Message
	closing chan struct{}
	stopped chan struct{}

	// mu is held for reading by every sender and exclusively by close, so
	// that closing cannot fall between a sender finding the writer open and
	// buffering its message.
	mu     sync.RWMutex
	closed bool
}

// newMessageWriter starts a writer that records up to batch messages at a
// time, and waits at most interval for a batch to fill.
func newMessageWriter(repo Repository, batch int, interval time.Duration) *messageWriter {
	w := &messageWriter{
		repo:     repo,
		batch:    batch,
		interval: interval,
		queue:    make(chan Message, batch*writeQueueBatches),
		closing:  make(chan struct{}),
		stopped:  make(chan struct{}),
	}

	go w.run()

	return w
}

// add buffers one message, and refuses every message once the writer is
// stopping: a message the writer takes is one it will write. A buffer that is
// full makes the sender wait for room rather than dropping what it holds,
// because a conversation is kept for the reports and disclosure requests made
// about it.
func (w *messageWriter) add(ctx context.Context, msg Message) error {
	// Holding the lock while waiting for room keeps close from stopping the
	// writer under a sender. Senders share it, and the run loop is still
	// draining the buffer, so the wait ends whatever else is going on.
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		return errWriterClosed
	}

	select {
	case w.queue <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// close stops the writer once what it holds is written, and stops waiting for
// it when ctx is over. The batch being written is left to its own timeout,
// because giving up on it in the middle would lose the messages it holds.
func (w *messageWriter) close(ctx context.Context) error {
	// Taking the lock exclusively waits for the senders on their way in, so
	// that the buffer takes nothing more once the writer is told to stop and
	// the run loop can empty it for good.
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.closing)
	}
	w.mu.Unlock()

	select {
	case <-w.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run writes what is buffered until the writer is closed: a batch as soon as
// it is full, and whatever is buffered once the interval is over.
func (w *messageWriter) run() {
	defer close(w.stopped)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	pending := make([]Message, 0, w.batch)
	for {
		select {
		case msg := <-w.queue:
			pending = append(pending, msg)
			if len(pending) >= w.batch {
				pending = w.write(pending)
			}
		case <-ticker.C:
			pending = w.write(pending)
		case <-w.closing:
			w.write(w.drain(pending))
			return
		}
	}
}

// drain takes everything the buffer holds. Nothing is buffered after the
// writer is told to stop, so emptying it once empties it for good.
func (w *messageWriter) drain(pending []Message) []Message {
	for {
		select {
		case msg := <-w.queue:
			pending = append(pending, msg)
			if len(pending) >= w.batch {
				pending = w.write(pending)
			}
		default:
			return pending
		}
	}
}

// write records one batch and answers with the empty batch to fill again.
// Messages are written after they were delivered, so a batch that fails is
// lost and the room is never told.
func (w *messageWriter) write(pending []Message) []Message {
	if len(pending) == 0 {
		return pending
	}

	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()

	if err := w.repo.AddMessages(ctx, pending); err != nil {
		slog.Error("record messages",
			slog.Int("messages", len(pending)), slog.Any("error", err))
	}

	return pending[:0]
}
