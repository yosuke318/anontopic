package moderation

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// maskRune stands for a character of a message that was written over to slip
// a word past the dictionary. It is a private use rune, so nothing a message
// can carry is read as one.
const maskRune = '\ue000'

// fold rewrites a message into the one form the ways of writing the same word
// share: NFKC folds full width characters and half width katakana, letters
// are lower cased, and hiragana is read as the katakana of the same sound.
func fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range strings.ToLower(norm.NFKC.String(s)) {
		b.WriteRune(katakana(r))
	}

	return b.String()
}

// katakana reads one hiragana as the katakana of the same sound, and leaves
// every other rune as it is.
func katakana(r rune) rune {
	const toKatakana = 'ァ' - 'ぁ'

	if r >= 'ぁ' && r <= 'ゖ' {
		return r + toKatakana
	}
	return r
}

// stripped keeps the letters and digits of a folded message, so that spaces
// and symbols written between the characters of a word do not hide it.
func stripped(folded string) string {
	return compact(folded, false)
}

// masked keeps the letters and digits of a folded message and puts one
// maskRune in place of every run of what stripped drops, so that a word one
// of whose characters was written over still reads as that word with a
// masked character.
func masked(folded string) string {
	return compact(folded, true)
}

// compact drops from a folded message everything that is neither a letter nor
// a digit, marking where it dropped a run when asked to.
func compact(folded string, mark bool) string {
	var b strings.Builder
	b.Grow(len(folded))

	dropped := false
	for _, r := range folded {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			dropped = true
			continue
		}

		// A run is marked only between two kept runes: what a message opens
		// or closes with stands in place of nothing.
		if dropped && mark && b.Len() > 0 {
			b.WriteRune(maskRune)
		}
		dropped = false
		b.WriteRune(r)
	}

	return b.String()
}
