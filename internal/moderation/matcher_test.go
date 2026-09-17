package moderation

import "testing"

func TestAMatcherFindsAPatternWhereverItIs(t *testing.T) {
	m := newMatcher([]string{"アイタイ", "エッチ", "line交換"})

	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"at the start", "アイタイデス", true},
		{"in the middle", "コンドアイタイデスネ", true},
		{"at the end", "コンドアイタイ", true},
		{"the whole string", "エッチ", true},
		{"a pattern in the latin script", "コンドline交換シヨウ", true},
		{"a string holding none of them", "コンニチハ", false},
		{"a prefix of a pattern", "アイタ", false},
		{"the characters of a pattern out of order", "タイアイ", false},
		{"an empty string", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.matches(tc.s); got != tc.want {
				t.Fatalf("matches(%q) = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// A pattern is found even where reading it started inside another one, which
// is what the state a failure carries on from is for.
func TestAMatcherFindsAPatternItStartedReadingInsideAnother(t *testing.T) {
	m := newMatcher([]string{"アイタイ", "イタコ"})

	if !m.matches("アイタコ") {
		t.Fatal(`matches("アイタコ") = false, want the second pattern to be found in it`)
	}
}

func TestAMatcherOfNoPatternsMatchesNothing(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
	}{
		{"no patterns at all", nil},
		{"an empty pattern", []string{""}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newMatcher(tc.patterns)
			if m.matches("なんでもいい") {
				t.Fatal("matches = true, want false")
			}
			if m.matches("") {
				t.Fatal(`matches("") = true, want false`)
			}
		})
	}
}
