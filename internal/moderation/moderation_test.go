package moderation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// testWords is the dictionary the tests judge messages against. It is small
// enough to read here, and holds a word of each shape the folding has to deal
// with: kanji and kana, katakana alone, and one written in the Latin script.
var testWords = []string{"会いたい", "エッチ", "セックス", "LINE交換"}

// stubRepository hands out the words it holds, and counts how often they were
// read so that a test can wait for a reload.
type stubRepository struct {
	mu    sync.Mutex
	words []string
	err   error
	reads int
}

func newStubRepository(words ...string) *stubRepository {
	return &stubRepository{words: words}
}

func (r *stubRepository) ActiveWords(context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reads++
	if r.err != nil {
		return nil, r.err
	}

	return r.words, nil
}

// setWords is the dictionary a later read answers with.
func (r *stubRepository) setWords(words ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.words = words
}

func (r *stubRepository) readCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.reads
}

// loadedService is a service holding testWords.
func loadedService(t *testing.T) *Service {
	t.Helper()

	svc := NewService(newStubRepository(testWords...), Options{})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	return svc
}

// moderate judges one body and fails the test when it could not be judged.
func moderate(t *testing.T, svc *Service, body string) Decision {
	t.Helper()

	decision, err := svc.Moderate(t.Context(), body)
	if err != nil {
		t.Fatalf("Moderate(%q): %v", body, err)
	}

	return decision
}

func TestAMessageIsRefusedWhileNoDictionaryIsLoaded(t *testing.T) {
	svc := NewService(newStubRepository(testWords...), Options{})

	decision, err := svc.Moderate(t.Context(), "こんばんは")
	if !errors.Is(err, ErrNoDictionary) {
		t.Fatalf("error = %v, want %v", err, ErrNoDictionary)
	}
	if decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", decision, DecisionBlock)
	}
}

func TestAWordOfTheDictionaryIsBlockedHoweverItIsWritten(t *testing.T) {
	svc := loadedService(t)

	tests := []struct {
		name string
		body string
	}{
		{"as it is written in the dictionary", "今度会いたいです"},
		{"in hiragana", "せっくす"},
		{"in full width letters", "ＬＩＮＥ交換しよう"},
		{"in half width katakana", "ｴｯﾁな話"},
		{"with spaces between its characters", "会 い た い"},
		{"with symbols between its characters", "会・い・た・い"},
		{"with a character written over", "エ○チな話"},
		{"with an inner character written over", "セッ*スの話"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if decision := moderate(t, svc, tc.body); decision != DecisionBlock {
				t.Fatalf("decision = %v, want %v", decision, DecisionBlock)
			}
		})
	}
}

func TestAMessageHoldingNoneOfThemIsAllowed(t *testing.T) {
	svc := loadedService(t)

	tests := []struct {
		name string
		body string
	}{
		{"an ordinary message", "こんばんは、今日は寒いですね"},
		{"a date", "2026年8月20日に見た映画の話"},
		{"a time", "だいたい12時30分くらいまで起きてる"},
		{"a number in a sentence", "身長は1.5メートルくらい"},
		{"a word the dictionary does not hold", "会話が続かない"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if decision := moderate(t, svc, tc.body); decision != DecisionAllow {
				t.Fatalf("decision = %v, want %v", decision, DecisionAllow)
			}
		})
	}
}

func TestAContactDetailIsBlockedWithoutBeingInTheDictionary(t *testing.T) {
	svc := loadedService(t)

	tests := []struct {
		name string
		body string
	}{
		{"a link", "続きはこっち https://example.com/room"},
		{"a host written for the reader to type in", "www.example.jp を見て"},
		{"a bare host", "example.com にいるよ"},
		{"an email address", "yosuke.318@example.com まで"},
		{"a telephone number", "09012345678 にかけて"},
		{"a telephone number in groups", "090-1234-5678 でもいいよ"},
		{"a handle somewhere else", "@yosuke_318 をフォローして"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if decision := moderate(t, svc, tc.body); decision != DecisionBlock {
				t.Fatalf("decision = %v, want %v", decision, DecisionBlock)
			}
		})
	}
}

func TestLoadingAgainJudgesByTheWordsThatAreStored(t *testing.T) {
	repo := newStubRepository("会いたい")
	svc := NewService(repo, Options{})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if decision := moderate(t, svc, "会いたいな"); decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", decision, DecisionBlock)
	}

	repo.setWords("儲か")
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if decision := moderate(t, svc, "会いたいな"); decision != DecisionAllow {
		t.Fatalf("decision = %v, want %v", decision, DecisionAllow)
	}
	if decision := moderate(t, svc, "これは儲かるよ"); decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", decision, DecisionBlock)
	}
}

func TestARunReadsTheDictionaryUntilItsContextIsOver(t *testing.T) {
	repo := newStubRepository(testWords...)
	svc := NewService(repo, Options{ReloadInterval: time.Millisecond})

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		svc.Run(ctx)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for repo.readCount() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("read the dictionary %d times, want it read again", repo.readCount())
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return once its context was over")
	}
}

func TestAReadThatFailsLeavesTheWordsInForce(t *testing.T) {
	repo := newStubRepository("会いたい")
	svc := NewService(repo, Options{})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	repo.err = errors.New("the database is unreachable")
	if err := svc.Load(t.Context()); err == nil {
		t.Fatal("Load returned no error, want the one the repository reported")
	}

	if decision := moderate(t, svc, "会いたいな"); decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", decision, DecisionBlock)
	}
}

// benchmarkWords is a dictionary the size the service is expected to hold
// once the words have been tuned for a while.
func benchmarkWords(n int) []string {
	words := make([]string, 0, n)
	for i := range n {
		words = append(words, string(rune('ぁ'+i%80))+"えっちな語"+string(rune('ァ'+i%80)))
	}

	return append(words, testWords...)
}

// BenchmarkModerate judges a message of the length people actually send, so
// that what one message costs a room can be read off it.
func BenchmarkModerate(b *testing.B) {
	svc := NewService(newStubRepository(benchmarkWords(500)...), Options{})
	if err := svc.Load(b.Context()); err != nil {
		b.Fatalf("Load: %v", err)
	}

	body := strings.Repeat("今日はいい天気ですね、", 10)
	b.ResetTimer()

	for b.Loop() {
		if _, err := svc.Moderate(b.Context(), body); err != nil {
			b.Fatalf("Moderate: %v", err)
		}
	}
}
