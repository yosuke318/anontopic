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
var testWords = []Word{
	{Text: "会いたい", Category: CategoryMeetup},
	{Text: "エッチ", Category: CategorySexual},
	{Text: "セックス", Category: CategorySexual},
	{Text: "LINE交換", Category: CategoryContact},
}

// stubRepository hands out the words it holds, and counts how often they were
// read so that a test can wait for a reload.
type stubRepository struct {
	mu    sync.Mutex
	words []Word
	err   error
	reads int
}

func newStubRepository(words ...Word) *stubRepository {
	return &stubRepository{words: words}
}

func (r *stubRepository) ActiveWords(context.Context) ([]Word, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.reads++
	if r.err != nil {
		return nil, r.err
	}

	return r.words, nil
}

// setWords is the dictionary a later read answers with.
func (r *stubRepository) setWords(words ...Word) {
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
func moderate(t *testing.T, svc *Service, body string) Verdict {
	t.Helper()

	verdict, err := svc.Moderate(t.Context(), body)
	if err != nil {
		t.Fatalf("Moderate(%q): %v", body, err)
	}

	return verdict
}

// meetup is a word of the meetup category, for the tests that read a
// dictionary of one word.
func meetup(text string) Word {
	return Word{Text: text, Category: CategoryMeetup}
}

func TestAMessageIsRefusedWhileNoDictionaryIsLoaded(t *testing.T) {
	svc := NewService(newStubRepository(testWords...), Options{})

	verdict, err := svc.Moderate(t.Context(), "こんばんは")
	if !errors.Is(err, ErrNoDictionary) {
		t.Fatalf("error = %v, want %v", err, ErrNoDictionary)
	}
	if verdict.Decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", verdict.Decision, DecisionBlock)
	}
}

func TestAWordOfTheDictionaryIsBlockedHoweverItIsWritten(t *testing.T) {
	svc := loadedService(t)

	tests := []struct {
		name string
		body string
		want Category
	}{
		{"as it is written in the dictionary", "今度会いたいです", CategoryMeetup},
		{"in hiragana", "せっくす", CategorySexual},
		{"in full width letters", "ＬＩＮＥ交換しよう", CategoryContact},
		{"in half width katakana", "ｴｯﾁな話", CategorySexual},
		{"with spaces between its characters", "会 い た い", CategoryMeetup},
		{"with symbols between its characters", "会・い・た・い", CategoryMeetup},
		{"with a character written over", "エ○チな話", CategorySexual},
		{"with an inner character written over", "セッ*スの話", CategorySexual},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := Verdict{Decision: DecisionBlock, Category: tc.want}
			if got := moderate(t, svc, tc.body); got != want {
				t.Fatalf("verdict = %+v, want %+v", got, want)
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
			want := Verdict{Decision: DecisionAllow}
			if got := moderate(t, svc, tc.body); got != want {
				t.Fatalf("verdict = %+v, want %+v", got, want)
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
			want := Verdict{Decision: DecisionBlock, Category: CategoryContact}
			if got := moderate(t, svc, tc.body); got != want {
				t.Fatalf("verdict = %+v, want %+v", got, want)
			}
		})
	}
}

func TestAContactDetailIsBlockedAsOneEvenWhenTheDictionaryHoldsAWordOfIt(t *testing.T) {
	svc := loadedService(t)

	want := Verdict{Decision: DecisionBlock, Category: CategoryContact}
	if got := moderate(t, svc, "会いたいから 090-1234-5678 に電話して"); got != want {
		t.Fatalf("verdict = %+v, want %+v", got, want)
	}
}

func TestLoadingAgainJudgesByTheWordsThatAreStored(t *testing.T) {
	repo := newStubRepository(meetup("会いたい"))
	svc := NewService(repo, Options{})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := moderate(t, svc, "会いたいな"); got.Decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", got.Decision, DecisionBlock)
	}

	repo.setWords(Word{Text: "儲か", Category: CategorySolicitation})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := moderate(t, svc, "会いたいな"); got.Decision != DecisionAllow {
		t.Fatalf("decision = %v, want %v", got.Decision, DecisionAllow)
	}
	if got := moderate(t, svc, "これは儲かるよ"); got.Decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", got.Decision, DecisionBlock)
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
	repo := newStubRepository(meetup("会いたい"))
	svc := NewService(repo, Options{})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	repo.err = errors.New("the database is unreachable")
	if err := svc.Load(t.Context()); err == nil {
		t.Fatal("Load returned no error, want the one the repository reported")
	}

	if got := moderate(t, svc, "会いたいな"); got.Decision != DecisionBlock {
		t.Fatalf("decision = %v, want %v", got.Decision, DecisionBlock)
	}
}

// benchmarkWords is a dictionary the size the service is expected to hold
// once the words have been tuned for a while.
func benchmarkWords(n int) []Word {
	words := make([]Word, 0, n)
	for i := range n {
		text := string(rune('ぁ'+i%80)) + "えっちな語" + string(rune('ァ'+i%80))
		words = append(words, Word{Text: text, Category: CategorySexual})
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
