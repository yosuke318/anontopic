package moderation

import (
	"regexp"
	"slices"
)

// The shapes an external contact detail is written in. They are blocked
// whatever the dictionary holds, because taking the conversation off the
// service is a prohibited use in its own right and the strings themselves are
// not words anyone could list.
//
// Each is read against a folded message, where a full width character has
// already been read as the half width one it stands for.
var patterns = []*regexp.Regexp{
	// A link, whether it names its scheme or is written for the reader to
	// type in.
	regexp.MustCompile(`https?://\S`),
	regexp.MustCompile(`www\.[a-z0-9\-]+\.[a-z]{2,}`),
	regexp.MustCompile(`[a-z0-9][a-z0-9\-]*\.(?:com|net|org|jp|io|me|co|tv|gg|xyz|link|site|shop|info|biz)(?:[^a-z]|$)`),

	// An email address.
	regexp.MustCompile(`[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`),

	// A telephone number of ten or eleven digits, written in one run or in
	// the groups it is usually separated into. Counting the digits keeps the
	// numbers a conversation is about out of it.
	regexp.MustCompile(`(?:^|[^0-9])(?:0[0-9]{9,10}|0[0-9]{1,4}[- ][0-9]{1,4}[- ][0-9]{4}|\+81[- ]?[0-9]{1,4}[- ]?[0-9]{1,4}[- ]?[0-9]{4})(?:[^0-9]|$)`),

	// The handle of an account somewhere else.
	regexp.MustCompile(`@[a-z0-9_.]{3,}`),
}

// matchesPattern reports whether a folded message carries a contact detail.
func matchesPattern(folded string) bool {
	return slices.ContainsFunc(patterns, func(re *regexp.Regexp) bool {
		return re.MatchString(folded)
	})
}
