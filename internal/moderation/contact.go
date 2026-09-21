package moderation

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// addressReplacer reads the words written in place of "@" and "." in a
// folded message as the symbols they stand for. A word comes before a shorter
// word it starts with, so that the longer one is the one replaced.
var addressReplacer = strings.NewReplacer(
	"アットマーク", "@",
	"アット", "@",
	"atmark", "@",
	"(at)", "@", "[at]", "@", "{at}", "@", "<at>", "@",
	"(@)", "@", "[@]", "@",

	"ドットコム", ".com",
	"ドットネット", ".net",
	"ドットジェーピー", ".jp",
	"ドット", ".",
	"(dot)", ".", "[dot]", ".", "{dot}", ".", "<dot>", ".",
	"(.)", ".", "[.]", ".",
)

// spaceAroundSymbol is the space written around the symbols a link, an
// address or a telephone number is joined by.
var spaceAroundSymbol = regexp.MustCompile(`\s*([@.:/\-。・ー‐‑‒–—―−])\s*`)

// contactForm rewrites a folded message into the form a link, an email
// address, a handle or a street address is read from: the words written for
// "@" and "." become the symbols, the space around those symbols goes, a
// Japanese full stop or middle dot between two Latin letters is read as ".",
// and a dash between two digits is read as "-".
func contactForm(folded string) string {
	s := spaceAroundSymbol.ReplaceAllString(addressReplacer.Replace(folded), "$1")

	runes := []rune(s)
	for i := 1; i < len(runes)-1; i++ {
		prev, next := runes[i-1], runes[i+1]
		switch {
		case (runes[i] == '。' || runes[i] == '・') && isLatinOrDigit(prev) && isLatinOrDigit(next):
			runes[i] = '.'
		case isDash(runes[i]) && isDigit(prev) && isDigit(next):
			runes[i] = '-'
		}
	}

	return string(runes)
}

// kanjiDigits are the kanji and the circles written in place of a digit.
var kanjiDigits = map[rune]byte{
	'〇': '0', '零': '0', '○': '0', '◯': '0',
	'一': '1', '壱': '1',
	'二': '2', '弐': '2',
	'三': '3', '参': '3',
	'四': '4',
	'五': '5',
	'六': '6',
	'七': '7',
	'八': '8',
	'九': '9',
}

// digitForm rewrites a folded message into the form a telephone number is
// read from. Digits and the kanji written for them are kept as digits, and
// "+" as it is. Symbols, spaces, the long vowel mark and "ノ" written between
// them are dropped, so that the groups of a number are read as one run. Any
// other character ends a run, and a stretch of them is written as one space.
func digitForm(folded string) string {
	var b strings.Builder
	b.Grow(len(folded))

	ended := false
	for _, r := range folded {
		if d, ok := kanjiDigits[r]; ok {
			r = rune(d)
		}

		switch {
		case isDigit(r), r == '+':
			b.WriteRune(r)
			ended = false
		case r == 'ー', r == 'ノ', !unicode.IsLetter(r) && !unicode.IsDigit(r):
		case !ended:
			b.WriteByte(' ')
			ended = true
		}
	}

	return b.String()
}

// The services an account is named by alongside its handle, as they read in
// a contact form.
const services = `line|ライン|インスタグラム|インスタ|instagram|insta|twitter|ツイッター|` +
	`discord|ディスコード|ディスコ|kakao|カカオトーク|カカオ|telegram|テレグラム|` +
	`tiktok|ティックトック|skype|スカイプ|facebook|フェイスブック|snapchat|wechat`

// handlePatterns are the shapes a link, an email address or the handle of an
// account is written in, read against a contact form. Each of them holds at
// least three Latin letters, digits or the symbols a handle is made of in a
// row.
var handlePatterns = []*regexp.Regexp{
	// A link, whether it names its scheme, one missing its first letters, or
	// is written for the reader to type in.
	regexp.MustCompile(`[a-z]*://[a-z0-9]`),
	regexp.MustCompile(`www\.[a-z0-9\-]+\.[a-z]{2,}`),
	regexp.MustCompile(`[a-z0-9][a-z0-9\-]*\.(?:com|net|org|jp|io|me|co|tv|gg|xyz|link|site|shop|info|biz|ly|gd|ee|cc|gl|app|dev|page|tokyo)(?:[^a-z]|$)`),
	regexp.MustCompile(`youtu\.be|amzn\.to`),

	// An email address, with its domain written out or named by the provider.
	regexp.MustCompile(`[a-z0-9._%+\-]+@[a-z0-9\-]+(?:\.[a-z0-9\-]+)*\.[a-z]{2,}`),
	regexp.MustCompile(`[a-z0-9._%+\-]+@(?:gmail|yahoo|icloud|outlook|hotmail|docomo|ezweb|softbank)(?:[^a-z]|$)`),

	// The handle of an account somewhere else, written after "@", as a
	// Discord tag, or after the name of its service or "ID".
	regexp.MustCompile(`@[a-z0-9_.]{3,}`),
	regexp.MustCompile(`[a-z0-9_.]{2,32}#[0-9]{4}(?:[^0-9]|$)`),
	regexp.MustCompile(`(?:^|[^a-z])(?:` + services + `)(?:(?:ノid|id|ノアイディー|アイディー)\s*[:=ハ→]?|\s*[:=ハ→])\s*@?[a-z0-9_.\-]{3,}`),
	regexp.MustCompile(`(?:^|[^a-z])(?:id|アイディー)\s*[:=ハ→]\s*@?[a-z0-9_.\-]{3,}`),
	regexp.MustCompile(`(?:^|[^a-z0-9])x\s*[:=]\s*@?[a-z_][a-z0-9_.\-]{2,}`),
}

// placePatterns are the shapes a street address, a postal code or a place
// given as coordinates is written in, read against a contact form. Each of
// them holds a digit.
var placePatterns = []*regexp.Regexp{
	regexp.MustCompile(`[0-9]+丁目`),
	regexp.MustCompile(`[0-9]+番地`),
	regexp.MustCompile(`[0-9]+番[0-9]+号`),
	regexp.MustCompile(`[都道府県市区町村郡][^0-9]{0,8}[0-9]{1,4}-[0-9]{1,4}-[0-9]{1,4}`),
	regexp.MustCompile(`〒[0-9]{3}-?[0-9]{4}`),
	regexp.MustCompile(`郵便番号[^0-9]{0,4}[0-9]{3}-?[0-9]{4}`),
	regexp.MustCompile(`-?[0-9]{1,3}\.[0-9]{4,},\s*-?[0-9]{1,3}\.[0-9]{4,}`),
}

// phonePattern is a Japanese telephone number of ten or eleven digits, read
// against a digit form. Counting the digits and asking for the digit after
// the leading 0 keeps dates, times, amounts and the placeholder "〇〇" out of
// it.
var phonePattern = regexp.MustCompile(`(?:^|[^0-9+])(?:0[1-9][0-9]{8,9}|\+81[1-9][0-9]{8,9})(?:[^0-9]|$)`)

// carriesContact reports whether a folded message carries an external
// contact detail.
func carriesContact(folded string) bool {
	if phonePattern.MatchString(digitForm(folded)) {
		return true
	}

	// Most messages hold no Latin letters or digits at all, and none of the
	// patterns needing them is read against those.
	form := contactForm(folded)
	matches := func(re *regexp.Regexp) bool { return re.MatchString(form) }

	if hasHandleRun(form) && slices.ContainsFunc(handlePatterns, matches) {
		return true
	}
	return strings.ContainsFunc(form, isDigit) && slices.ContainsFunc(placePatterns, matches)
}

// hasHandleRun reports whether a contact form holds three Latin letters,
// digits or the symbols a handle is made of in a row.
func hasHandleRun(form string) bool {
	run := 0
	for _, r := range form {
		if !isLatinOrDigit(r) && r != '_' && r != '.' && r != '-' {
			run = 0
			continue
		}
		if run++; run >= 3 {
			return true
		}
	}

	return false
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isLatinOrDigit(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'z')
}

// isDash reports whether r is one of the dashes a digit group or a block
// number is separated by. The long vowel mark is among them, because it is
// what a Japanese keyboard types for "-".
func isDash(r rune) bool {
	return strings.ContainsRune("ー‐‑‒–—―−", r)
}
