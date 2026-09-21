package moderation

import "testing"

func TestFoldingReadsTheWaysOfWritingAWordAsOne(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"katakana as it is", "エッチ", "エッチ"},
		{"hiragana as katakana", "えっち", "エッチ"},
		{"half width katakana", "ｴｯﾁ", "エッチ"},
		{"a voiced half width katakana", "ﾗｲﾝ", "ライン"},
		{"full width letters", "ＬＩＮＥ", "line"},
		{"upper case letters", "LINE", "line"},
		{"full width digits", "０９０", "090"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fold(tc.s); got != tc.want {
				t.Fatalf("fold(%q) = %q, want %q", tc.s, got, tc.want)
			}
		})
	}
}

func TestStrippingLeavesTheLettersAndDigitsOfAMessage(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"symbols between the characters of a word", "会・い・た・い", "会いたい"},
		{"spaces between them", "会 い た い", "会いたい"},
		{"a message that carries none", "会いたい", "会いたい"},
		{"punctuation around a word", "、会いたい。", "会いたい"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripped(tc.s); got != tc.want {
				t.Fatalf("stripped(%q) = %q, want %q", tc.s, got, tc.want)
			}
		})
	}
}

func TestMaskingStandsForWhatWasWrittenOverBetweenTwoCharacters(t *testing.T) {
	mask := string(maskRune)

	tests := []struct {
		name string
		s    string
		want string
	}{
		{"a character written over", "エ○チ", "エ" + mask + "チ"},
		{"a run of them", "エ○○チ", "エ" + mask + "チ"},
		{"what a message opens with", "○エチ", "エチ"},
		{"what a message closes with", "エチ○", "エチ"},
		{"a message that carries none", "エッチ", "エッチ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := masked(tc.s); got != tc.want {
				t.Fatalf("masked(%q) = %q, want %q", tc.s, got, tc.want)
			}
		})
	}
}

func TestTheVariantsOfAWordWriteOverOneInnerCharacter(t *testing.T) {
	mask := string(maskRune)

	tests := []struct {
		name string
		word string
		want []string
	}{
		{"a word of three characters", "エッチ", []string{"エ" + mask + "チ"}},
		{
			"a word of four characters",
			"セックス",
			[]string{"セ" + mask + "クス", "セッ" + mask + "ス"},
		},
		{"a word too short to have an inner character", "裸", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskedVariants(tc.word)
			if len(got) != len(tc.want) {
				t.Fatalf("maskedVariants(%q) = %q, want %q", tc.word, got, tc.want)
			}
			for i, want := range tc.want {
				if got[i] != want {
					t.Fatalf("maskedVariants(%q) = %q, want %q", tc.word, got, tc.want)
				}
			}
		})
	}
}
