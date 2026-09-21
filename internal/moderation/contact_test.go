package moderation

import "testing"

// contactOnlyService is a service holding no words, so that what it blocks
// is what the contact patterns found.
func contactOnlyService(t *testing.T) *Service {
	t.Helper()

	svc := NewService(newStubRepository(), Options{})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	return svc
}

func TestAContactDetailIsBlockedHoweverItIsWritten(t *testing.T) {
	svc := contactOnlyService(t)

	tests := []struct {
		name string
		body string
	}{
		// Links.
		{"a link missing the first letter of its scheme", "ttps://example.com"},
		{"a link with spaces in its scheme", "https : // example.com"},
		{"a shortened link", "bit.ly/3abcDEF 見て"},
		{"a shortened link with its dot written as a word", "bitドットly/3abc"},
		{"a LINE invitation", "lin.ee/abcdef から追加して"},
		{"a Discord invitation", "discord.gg/abcdef"},
		{"a host in full width letters", "ｅｘａｍｐｌｅ．ｃｏｍ"},
		{"a host with its dot written as a word", "exampleドットコム"},
		{"a host with its dot in brackets", "example(.)com"},
		{"a host with spaces around its dot", "example . com"},
		{"a host with a Japanese full stop", "example。com"},

		// Telephone numbers.
		{"a number in full width digits", "０９０１２３４５６７８"},
		{"a number with spaces between its digits", "0 9 0 1 2 3 4 5 6 7 8"},
		{"a number with dots between its groups", "090.1234.5678"},
		{"a number with long vowel marks between its groups", "090ー1234ー5678"},
		{"a number over several lines", "090\n1234\n5678"},
		{"a number in kanji", "〇九〇一二三四五六七八"},
		{"a number in kanji and digits", "〇九〇-1234-五六七八"},
		{"a number with circles for its zeros", "○9○-1234-5678"},
		{"a number in circled digits", "⓪⑨⓪①②③④⑤⑥⑦⑧"},
		{"a number with の between its groups", "090の1234の5678"},
		{"a landline number", "03-1234-5678"},
		{"a number with the country code", "+81 90 1234 5678"},

		// Email addresses.
		{"an address with a full width at sign", "yosuke318＠example.com"},
		{"an address with at written in katakana", "yosuke318アットマークexample.com"},
		{"an address with at written in hiragana", "yosuke318あっとgmail.com"},
		{"an address with every symbol written as a word", "yosuke318 アット gmail ドット com"},
		{"an address with its symbols in brackets", "yosuke318(at)gmail(dot)com"},
		{"an address naming its provider alone", "yosuke318@gmailに送って"},

		// Handles.
		{"a LINE ID", "LINEのIDは yosuke318 です"},
		{"a LINE ID after a colon", "ライン: yosuke_318"},
		{"an Instagram handle", "インスタはyosuke.318"},
		{"a handle after ID", "ID: yosuke318"},
		{"a handle on X", "X: yosuke_318"},
		{"a handle after a full width at sign", "＠yosuke_318"},
		{"a Discord tag", "yosuke#1234 で追加して"},

		// Addresses and places.
		{"a block of a town", "渋谷区道玄坂2丁目に住んでる"},
		{"a lot number", "5番地のアパート"},
		{"a block and a house number", "3番12号"},
		{"a block number after a ward", "渋谷区道玄坂1-2-3"},
		{"a block number with long vowel marks", "世田谷区太子堂1ー2ー3"},
		{"a postal code", "〒150-0043"},
		{"a postal code after its name", "郵便番号は150-0043"},
		{"coordinates", "35.6581, 139.7017 にいる"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := Verdict{Decision: DecisionBlock, Category: CategoryContact}
			if got := moderate(t, svc, tc.body); got != want {
				t.Fatalf("Moderate(%q) = %+v, want %+v", tc.body, got, want)
			}
		})
	}
}

// The messages below are of the kind the topics are about. Numbers, dots,
// the names of services and places come up in them without anything being
// exchanged, and none of them is to be blocked for it.
func TestAMessageThatOnlyLooksLikeAContactDetailIsAllowed(t *testing.T) {
	svc := contactOnlyService(t)

	tests := []struct {
		name string
		body string
	}{
		{"a date and a time", "2026/09/19 09:30 に起きた"},
		{"a range of times", "09:00-18:00 で働いてる"},
		{"an amount of money", "10,000,000円あったら何する？"},
		{"a score", "3-1で勝った"},
		{"a score in a ward tournament", "地区大会で3-1だった"},
		{"a version number", "Go 1.25 と Node.js 22 を使ってる"},
		{"a decimal", "身長は1.5メートルくらい"},
		{"a count in kanji", "一二三四五と数える"},
		{"placeholders", "〇〇さんが〇〇〇〇って言ってた"},
		{"a list of numbers", "1 2 3 4 5 6 7 8 9 10"},
		{"a service by name", "LINEの新しいスタンプ可愛い"},
		{"a service and a sentence", "インスタは見る専です"},
		{"a place by name", "東京都に住んでる人いる？"},
		{"a street by name", "三丁目の夕日って映画"},
		{"a URL scheme in a sentence", "HTTPSって何の略？"},
		{"a word after a colon", "ポイント: 早起き"},
		{"an at home feeling", "アットホームな職場"},
		{"pixel art", "ドット絵を描いてる"},
		{"an ID card", "IDカードを忘れた"},
		{"a mention of an email service", "Gmailの容量がいっぱい"},
		{"an ellipsis", "そうなんだ...com なんとか"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			want := Verdict{Decision: DecisionAllow}
			if got := moderate(t, svc, tc.body); got != want {
				t.Fatalf("Moderate(%q) = %+v, want %+v", tc.body, got, want)
			}
		})
	}
}

func TestTheContactFormReadsTheSymbolsWrittenAsWords(t *testing.T) {
	tests := []struct {
		name   string
		folded string
		want   string
	}{
		{"at and dot as words", "yosuke アットマーク gmail ドット com", "yosuke@gmail.com"},
		{"at and dot in brackets", "yosuke(at)gmail[dot]com", "yosuke@gmail.com"},
		{"a Japanese full stop between Latin letters", "example。com", "example.com"},
		{"a Japanese full stop after a sentence", "ソウ。ソレデ", "ソウ。ソレデ"},
		{"a dash between digits", "1ー2ー3", "1-2-3"},
		{"a long vowel mark in a word", "ラーメン", "ラーメン"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := contactForm(tc.folded); got != tc.want {
				t.Fatalf("contactForm(%q) = %q, want %q", tc.folded, got, tc.want)
			}
		})
	}
}

func TestTheDigitFormJoinsTheGroupsOfANumber(t *testing.T) {
	tests := []struct {
		name   string
		folded string
		want   string
	}{
		{"groups separated by symbols", "090-1234.5678", "09012345678"},
		{"kanji digits", "〇九〇", "090"},
		{"digits separated by a letter", "12ジ30フン", "12 30 "},
		{"the country code", "+81 90", "+8190"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := digitForm(tc.folded); got != tc.want {
				t.Fatalf("digitForm(%q) = %q, want %q", tc.folded, got, tc.want)
			}
		})
	}
}
