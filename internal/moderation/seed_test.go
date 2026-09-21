package moderation

import (
	"regexp"
	"testing"

	"github.com/yosuke318/anontopic/db"
)

// seedService is a service holding the NG words a fresh environment is
// seeded with, read from the seed file itself.
func seedService(t *testing.T) *Service {
	t.Helper()

	sql, err := db.Seeds.ReadFile("seeds/base/0002_ng_words.sql")
	if err != nil {
		t.Fatalf("read the seed: %v", err)
	}

	var words []Word
	for _, row := range regexp.MustCompile(`\('([^']+)', '(\w+)'\)`).FindAllStringSubmatch(string(sql), -1) {
		words = append(words, Word{Text: row[1], Category: Category(row[2])})
	}
	if len(words) == 0 {
		t.Fatal("read no words from the seed")
	}

	svc := NewService(newStubRepository(words...), Options{})
	if err := svc.Load(t.Context()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	return svc
}

func TestTheSeededWordsBlockAskingToMeetOrToBeContacted(t *testing.T) {
	svc := seedService(t)

	tests := []struct {
		body string
		want Category
	}{
		{"今度会いませんか", CategoryMeetup},
		{"リアルで会おうよ", CategoryMeetup},
		{"最寄り駅どこ？", CategoryMeetup},
		{"どこ住み？", CategoryMeetup},
		{"家どこ？", CategoryMeetup},
		{"LINEやってる？", CategoryContact},
		{"ラインおしえて", CategoryContact},
		{"ID交換しよ", CategoryContact},
		{"I D 交 換 し よ", CategoryContact},
		{"番号教えてよ", CategoryContact},
		{"続きはDMで話そう", CategoryContact},
		{"discordある？", CategoryContact},
	}
	for _, tc := range tests {
		t.Run(tc.body, func(t *testing.T) {
			want := Verdict{Decision: DecisionBlock, Category: tc.want}
			if got := moderate(t, svc, tc.body); got != want {
				t.Fatalf("Moderate(%q) = %+v, want %+v", tc.body, got, want)
			}
		})
	}
}

// The messages below are of the kind each topic is about, and carry numbers,
// the names of places and the words around meeting and contacting that come
// up without anything being asked for.
func TestTheSeededWordsLeaveAnOrdinaryMessageAlone(t *testing.T) {
	svc := seedService(t)

	for _, body := range []string{
		"今日めっちゃ暑くない？",
		"最近ハマってるお菓子ある？",
		"カメラ始めたんだけど、何から撮ればいいかな",
		"上司との付き合い方に悩んでる",
		"新しいアルバムの3曲目がいい",
		"三丁目の夕日って映画",
		"IDカード忘れて会社に入れなかった",
		"Go 1.25 がリリースされたね",
		"駅で財布を拾って交番に届けた",
		"電車が10分遅れた",
		"最寄り駅から家まで遠い",
		"猫が会いに来てくれた",
		"友達と宅飲みした",
		"子どもを迎えに行く時間だ",
		"LINEの既読がつかない",
		"お会計は1,280円でした",
		"2026年9月19日の日記",
		"テストで90点取った",
	} {
		t.Run(body, func(t *testing.T) {
			want := Verdict{Decision: DecisionAllow}
			if got := moderate(t, svc, body); got != want {
				t.Fatalf("Moderate(%q) = %+v, want %+v", body, got, want)
			}
		})
	}
}
