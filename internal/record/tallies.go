package record

// Tallies are the two counts a live projection attaches to a snapshot it
// trimmed to what a frame can show: Children counts the child Tasks left
// out, Collections the child collections left out. record carries them and
// never reads them; the projection that built them owns their type.
type Tallies struct {
	Children    any
	Collections any
}

// Tallies is the tally pair a projection attached to c, zero when c is
// complete.
func (c TasksSnapshot) Tallies() Tallies { return c.tallies }

// WithTallies is c carrying t.
func (c TasksSnapshot) WithTallies(t Tallies) TasksSnapshot {
	c.tallies = t
	return c
}

// RootTallies is the tally pair a projection attached to the standalone root
// Tasks and root collections of s, zero when s is complete.
func (s Snapshot) RootTallies() Tallies { return s.rootTallies }

// WithRootTallies is s carrying t for its root Tasks and root collections.
func (s Snapshot) WithRootTallies(t Tallies) Snapshot {
	s.rootTallies = t
	return s
}
