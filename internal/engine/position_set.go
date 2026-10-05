package engine

import "math/bits"

// positionSet is a set of non-negative positions that yields its members
// in ascending order without visiting the gaps: a tree of 64-bit words
// where each bit of a level marks a non-empty word of the level below.
// Adding, removing, and finding the next member cost the tree's height,
// so the first k of a set of 16000 cost k steps, not 16000.
type positionSet struct {
	levels [][]uint64
	size   int
}

// len is how many positions the set holds.
func (s *positionSet) len() int { return s.size }

// has reports whether i is a member.
func (s *positionSet) has(i int) bool {
	w := i >> 6
	return len(s.levels) > 0 && w < len(s.levels[0]) && s.levels[0][w]&(1<<(i&63)) != 0
}

// add makes i a member.
func (s *positionSet) add(i int) {
	if s.has(i) {
		return
	}
	s.size++
	s.reach(i)
	for l := range s.levels {
		w := i >> 6
		wasEmpty := s.levels[l][w] == 0
		s.levels[l][w] |= 1 << (i & 63)
		if !wasEmpty {
			return
		}
		i = w
	}
}

// reach grows the tree until it can hold i: every level has the word i
// falls in, and the top level is one word, so add has a root to stop at.
func (s *positionSet) reach(i int) {
	for l := 0; ; l++ {
		if l == len(s.levels) {
			s.levels = append(s.levels, nil)
			s.seed(l)
		}
		for len(s.levels[l]) <= i>>6 {
			s.levels[l] = append(s.levels[l], 0)
		}
		if l == len(s.levels)-1 && len(s.levels[l]) == 1 {
			return
		}
		i >>= 6
	}
}

// seed marks in a new level l the non-empty words of level l-1.
func (s *positionSet) seed(l int) {
	if l == 0 {
		return
	}
	for w, word := range s.levels[l-1] {
		if word == 0 {
			continue
		}
		for len(s.levels[l]) <= w>>6 {
			s.levels[l] = append(s.levels[l], 0)
		}
		s.levels[l][w>>6] |= 1 << (w & 63)
	}
}

// remove makes i a non-member.
func (s *positionSet) remove(i int) {
	if !s.has(i) {
		return
	}
	s.size--
	for l := 0; l < len(s.levels); l++ {
		w := i >> 6
		s.levels[l][w] &^= 1 << (i & 63)
		if s.levels[l][w] != 0 {
			return
		}
		i = w
	}
}

// next is the smallest member at or above from, or -1.
func (s *positionSet) next(from int) int { return s.nextAt(0, from) }

func (s *positionSet) nextAt(level, from int) int {
	if level >= len(s.levels) {
		return -1
	}
	w := from >> 6
	if w >= len(s.levels[level]) {
		return -1
	}
	if m := s.levels[level][w] & (^uint64(0) << (from & 63)); m != 0 {
		return w<<6 + bits.TrailingZeros64(m)
	}
	nw := s.nextAt(level+1, w+1)
	if nw < 0 {
		return -1
	}
	return nw<<6 + bits.TrailingZeros64(s.levels[level][nw])
}

// appendFirst appends the k smallest members, ascending, to dst.
func (s *positionSet) appendFirst(dst []int, k int) []int {
	for p, n := s.next(0), 0; p >= 0 && n < k; p, n = s.next(p+1), n+1 {
		dst = append(dst, p)
	}
	return dst
}

// each calls fn on every member, ascending.
func (s *positionSet) each(fn func(int)) {
	for p := s.next(0); p >= 0; p = s.next(p + 1) {
		fn(p)
	}
}
