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
			if _, got := m.find(tc.s); got != tc.want {
				t.Fatalf("find(%q) found = %v, want %v", tc.s, got, tc.want)
			}
		})
	}
}

// A pattern is found even where reading it started inside another one, which
// is what the state a failure carries on from is for.
func TestAMatcherFindsAPatternItStartedReadingInsideAnother(t *testing.T) {
	m := newMatcher([]string{"アイタイ", "イタコ"})

	i, ok := m.find("アイタコ")
	if !ok || i != 1 {
		t.Fatalf(`find("アイタコ") = %d, %v, want the second pattern found in it`, i, ok)
	}
}

func TestAMatcherReportsWhichPatternItFound(t *testing.T) {
	m := newMatcher([]string{"アイタイ", "エッチ", "アイタイ"})

	tests := []struct {
		name string
		s    string
		want int
	}{
		{"the first pattern", "コンドアイタイ", 0},
		{"the second pattern", "エッチナハナシ", 1},
		{"the one read first of two", "エッチデアイタイ", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := m.find(tc.s); !ok || got != tc.want {
				t.Fatalf("find(%q) = %d, %v, want %d", tc.s, got, ok, tc.want)
			}
		})
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
			if _, ok := m.find("なんでもいい"); ok {
				t.Fatal("find found a pattern, want none")
			}
			if _, ok := m.find(""); ok {
				t.Fatal(`find("") found a pattern, want none`)
			}
		})
	}
}
