package matching

import "testing"

func TestDuplicateFindsATokenThatAppearsTwice(t *testing.T) {
	cases := []struct {
		name         string
		participants []string
		want         string
		wantFound    bool
	}{
		{"one room of three", []string{"alice", "bob", "carol"}, "", false},
		{"the same token twice", []string{"alice", "bob", "alice"}, "alice", true},
		{"no participant at all", nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			token, found := duplicate(c.participants)
			if token != c.want || found != c.wantFound {
				t.Fatalf("duplicate(%v) = %q, %v, want %q, %v",
					c.participants, token, found, c.want, c.wantFound)
			}
		})
	}
}
