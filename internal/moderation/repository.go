package moderation

import "context"

// Word is one entry of the NG word dictionary.
type Word struct {
	Text     string
	Category Category
}

// Repository stores the NG word dictionary.
type Repository interface {
	// ActiveWords returns the words currently in force. A word that was
	// switched off is left out, so that a word can be stopped without being
	// lost.
	ActiveWords(ctx context.Context) ([]Word, error)
}
