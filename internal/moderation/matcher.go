package moderation

// matcher reports whether a string contains any of a set of patterns, reading
// the string once whatever the patterns are.
//
// It is an Aho-Corasick automaton. The patterns are held as a trie, and every
// state also knows the state standing for the longest pattern prefix that is
// a suffix of what has been read, so a character that does not continue the
// pattern being read is answered without going back over the string. Judging
// a message is on the path it takes to its room, and the dictionary holds as
// many words as the operators put in it.
type matcher struct {
	states []state
}

// state is one state of the automaton.
type state struct {
	// next is the state each character leads to from here.
	next map[rune]int
	// fail is the state to carry on from when no character does.
	fail int
	// pattern is the index of a pattern that ends here, or at a state fail
	// leads to, and -1 where none does.
	pattern int
}

// newMatcher builds the automaton of the given patterns. An empty pattern is
// left out: it would end at the state every string starts in and match
// everything.
func newMatcher(patterns []string) *matcher {
	m := &matcher{states: []state{newState()}}
	for i, pattern := range patterns {
		m.add(i, pattern)
	}
	m.link()

	return m
}

// newState is a state no pattern ends at.
func newState() state {
	return state{next: map[rune]int{}, pattern: -1}
}

// add puts the pattern at index i into the trie. Where two patterns are the
// same, the one added first is the one reported.
func (m *matcher) add(i int, pattern string) {
	at := 0
	for _, r := range pattern {
		next, ok := m.states[at].next[r]
		if !ok {
			m.states = append(m.states, newState())
			next = len(m.states) - 1
			m.states[at].next[r] = next
		}
		at = next
	}

	if at != 0 && m.states[at].pattern < 0 {
		m.states[at].pattern = i
	}
}

// link fills in the state each state carries on from. States are taken in the
// order of their depth, so that the state a failure leads to is complete
// before it is read.
func (m *matcher) link() {
	queue := make([]int, 0, len(m.states))
	for _, next := range m.states[0].next {
		queue = append(queue, next)
	}

	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]

		// A pattern ending where a failure leads ends in what was read here
		// as well, so the answer is carried forward once instead of being
		// followed on every character.
		if m.states[at].pattern < 0 {
			m.states[at].pattern = m.states[m.states[at].fail].pattern
		}

		for r, next := range m.states[at].next {
			m.states[next].fail = m.step(m.states[at].fail, r)
			queue = append(queue, next)
		}
	}
}

// step reads one character from a state, carrying on from where the failures
// lead until a character leads somewhere or the automaton is back at its
// start.
func (m *matcher) step(at int, r rune) int {
	for {
		if next, ok := m.states[at].next[r]; ok {
			return next
		}
		if at == 0 {
			return 0
		}
		at = m.states[at].fail
	}
}

// find reports the index of the first pattern s is found to contain, reading
// it from the start, and whether it contains any.
func (m *matcher) find(s string) (int, bool) {
	at := 0
	for _, r := range s {
		at = m.step(at, r)
		if m.states[at].pattern >= 0 {
			return m.states[at].pattern, true
		}
	}

	return -1, false
}
