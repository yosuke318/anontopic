package moderation

import "slices"

// minMaskedRunes is the shortest word a masked variant is made of. A word of
// two characters has none between its first and its last to write over, and
// the variants of a shorter word would match a symbol alone.
const minMaskedRunes = 3

// dictionary is the set of words in force, ready to judge a message against.
type dictionary struct {
	matcher *matcher
	// categories holds the category of the word each pattern of matcher was
	// made from, at the index of the pattern.
	categories []Category
}

// newDictionary folds the words and builds what a message is read against:
// every word, and the variants of it that have one of its inner characters
// written over.
func newDictionary(words []Word) *dictionary {
	patterns := make([]string, 0, len(words))
	categories := make([]Category, 0, len(words))
	for _, word := range words {
		folded := stripped(fold(word.Text))
		if folded == "" {
			continue
		}

		variants := append([]string{folded}, maskedVariants(folded)...)
		for range variants {
			categories = append(categories, word.Category)
		}
		patterns = append(patterns, variants...)
	}

	return &dictionary{matcher: newMatcher(patterns), categories: categories}
}

// find reports the category of a word of the dictionary a folded message
// carries, and whether it carries any.
//
// The message is read twice. Without its symbols, a word is found whatever
// was written between its characters. With a mask in place of them, a word is
// found when one of its characters was written over.
func (d *dictionary) find(folded string) (Category, bool) {
	if i, ok := d.matcher.find(stripped(folded)); ok {
		return d.categories[i], true
	}
	if i, ok := d.matcher.find(masked(folded)); ok {
		return d.categories[i], true
	}

	return "", false
}

// maskedVariants is the word with each of its inner characters written over
// in turn.
func maskedVariants(word string) []string {
	runes := []rune(word)
	if len(runes) < minMaskedRunes {
		return nil
	}

	variants := make([]string, 0, len(runes)-2)
	for i := 1; i < len(runes)-1; i++ {
		variant := slices.Clone(runes)
		variant[i] = maskRune
		variants = append(variants, string(variant))
	}

	return variants
}
