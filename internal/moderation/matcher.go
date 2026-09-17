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
	// ends reports whether a pattern ends here, or at a state fail leads to.
	ends bool
}

// newMatcher builds the automaton of the given patterns. An empty pattern is
// left out: it would end at the state every string starts in and match
// everything.
func newMatcher(patterns []string) *matcher {
	m := &matcher{states: []state{{next: map[rune]int{}}}}
	for _, pattern := range patterns {
		m.add(pattern)
	}
	m.link()

	return m
}

// add puts one pattern into the trie.
func (m *matcher) add(pattern string) {
	at := 0
	for _, r := range pattern {
		next, ok := m.states[at].next[r]
		if !ok {
			m.states = append(m.states, state{next: map[rune]int{}})
			next = len(m.states) - 1
			m.states[at].next[r] = next
		}
		at = next
	}

	if at != 0 {
		m.states[at].ends = true
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
		m.states[at].ends = m.states[at].ends || m.states[m.states[at].fail].ends

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

// matches reports whether s contains any of the patterns.
func (m *matcher) matches(s string) bool {
	at := 0
	for _, r := range s {
		at = m.step(at, r)
		if m.states[at].ends {
			return true
		}
	}

	return false
}
