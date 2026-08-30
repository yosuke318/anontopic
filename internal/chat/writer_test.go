package chat

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTheWriterRecordsABatchAsSoonAsItIsFull(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)

	// The interval outlasts the test, so only a full batch can be written.
	w := newMessageWriter(repo, 2, time.Hour)
	t.Cleanup(func() { closeWriter(t, w) })

	for _, body := range []string{"はじめまして", "こんばんは"} {
		if err := w.add(t.Context(), testMessage(repo, body)); err != nil {
			t.Fatalf("add %q: %v", body, err)
		}
	}

	recorded := awaitRecorded(t, repo, 2)
	if recorded[0].body != "はじめまして" || recorded[1].body != "こんばんは" {
		t.Fatalf("recorded %+v, want the messages in the order they were sent", recorded)
	}
}

func TestTheWriterRecordsWhatIsBufferedOnceTheIntervalIsOver(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)

	// A batch the room will not fill on its own is written anyway.
	w := newMessageWriter(repo, 100, time.Millisecond)
	t.Cleanup(func() { closeWriter(t, w) })

	if err := w.add(t.Context(), testMessage(repo, "はじめまして")); err != nil {
		t.Fatalf("add: %v", err)
	}

	awaitRecorded(t, repo, 1)
}

func TestClosingTheWriterRecordsWhatItStillHolds(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)

	w := newMessageWriter(repo, 100, time.Hour)
	for _, body := range []string{"はじめまして", "こんばんは", "おやすみ"} {
		if err := w.add(t.Context(), testMessage(repo, body)); err != nil {
			t.Fatalf("add %q: %v", body, err)
		}
	}

	closeWriter(t, w)

	if recorded := repo.recorded(); len(recorded) != 3 {
		t.Fatalf("recorded %+v, want the three messages the writer was holding", recorded)
	}

	// Nothing is taken afterwards, so that no message is delivered to a room
	// that will not be recorded.
	if err := w.add(t.Context(), testMessage(repo, "まだいる")); !errors.Is(err, errWriterClosed) {
		t.Fatalf("add after close = %v, want %v", err, errWriterClosed)
	}
}

func TestTheWriterKeepsRecordingAfterABatchItCouldNotWrite(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	failing := &failingRepository{fakeRepository: repo, fail: true}

	w := newMessageWriter(failing, 1, time.Hour)
	t.Cleanup(func() { closeWriter(t, w) })

	if err := w.add(t.Context(), testMessage(repo, "書けないほう")); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := w.add(t.Context(), testMessage(repo, "書けるほう")); err != nil {
		t.Fatalf("add: %v", err)
	}

	recorded := awaitRecorded(t, repo, 1)
	if len(recorded) != 1 || recorded[0].body != "書けるほう" {
		t.Fatalf("recorded %+v, want the batch that followed the one that failed", recorded)
	}
}

func TestAFullBufferMakesTheSenderWaitRatherThanLoseAMessage(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	held := &heldRepository{fakeRepository: repo, released: make(chan struct{})}

	w := newMessageWriter(held, 1, time.Hour)

	// The writer holds one batch in the write it cannot finish and the rest in
	// its buffer. More messages than that have nowhere to go, so the sender
	// waits for room until its own context is over.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var taken int
	var err error
	for range 2 * (1 + writeQueueBatches) {
		if err = w.add(ctx, testMessage(repo, "つまっている")); err != nil {
			break
		}
		taken++
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("add into a full buffer = %v, want the sender to wait for room", err)
	}

	close(held.released)
	closeWriter(t, w)

	if recorded := repo.recorded(); len(recorded) != taken {
		t.Fatalf("recorded %d messages, want the %d the writer took", len(recorded), taken)
	}
}

// testMessage is one message of the conversation repo holds.
func testMessage(repo *fakeRepository, body string) Message {
	return Message{
		ConversationID: repo.conv.ID,
		SenderToken:    tokenAlice,
		Body:           body,
		CreatedAt:      time.Now().UTC(),
	}
}

// closeWriter stops a writer, giving up rather than holding the test open.
func closeWriter(t *testing.T, w *messageWriter) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := w.close(ctx); err != nil {
		t.Errorf("close the writer: %v", err)
	}
}

// failingRepository refuses the first batch it is given and records the ones
// after it.
type failingRepository struct {
	*fakeRepository

	mu   sync.Mutex
	fail bool
}

func (r *failingRepository) AddMessages(ctx context.Context, messages []Message) error {
	r.mu.Lock()
	fail := r.fail
	r.fail = false
	r.mu.Unlock()

	if fail {
		return errors.New("the database is unavailable")
	}
	return r.fakeRepository.AddMessages(ctx, messages)
}

// heldRepository records nothing until it is released, which is what a
// database too slow to keep up looks like to the writer.
type heldRepository struct {
	*fakeRepository

	released chan struct{}
}

func (r *heldRepository) AddMessages(ctx context.Context, messages []Message) error {
	select {
	case <-r.released:
		return r.fakeRepository.AddMessages(ctx, messages)
	case <-ctx.Done():
		return ctx.Err()
	}
}
