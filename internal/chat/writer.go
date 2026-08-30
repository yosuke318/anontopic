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
	once    sync.Once
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

// add buffers one message. A buffer that is full makes the sender wait for
// room rather than dropping what it holds, because a conversation is kept for
// the reports and disclosure requests made about it.
func (w *messageWriter) add(ctx context.Context, msg Message) error {
	// A writer that is stopping refuses every message, buffer or no buffer.
	select {
	case <-w.closing:
		return errWriterClosed
	default:
	}

	select {
	case w.queue <- msg:
		return nil
	case <-w.closing:
		return errWriterClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// close stops the writer once what it holds is written, and gives up on a
// write that outlasts ctx.
func (w *messageWriter) close(ctx context.Context) error {
	w.once.Do(func() { close(w.closing) })

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

// drain takes everything the buffer holds, so that nothing waits behind a
// writer that is stopping.
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
