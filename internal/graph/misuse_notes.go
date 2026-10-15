package graph

// misuseNote is misuse the scheduler found while it held the graph lock. The
// MisuseSink is told only once that lock is free: a strict sink panics, and a
// panic under the lock would strand it, with every parked waiter behind it.
type misuseNote struct {
	subject string
	err     error
}

// noteMisuseLocked keeps misuse for tellMisuse to deliver once the lock is
// released.
func (g *Graph) noteMisuseLocked(subject string, err error) {
	g.exec.misuseNotes = append(g.exec.misuseNotes, misuseNote{subject: subject, err: err})
}

// takeMisuseNotesLocked hands over the misuse noted so far, to be told after
// the lock is released.
func (g *Graph) takeMisuseNotesLocked() []misuseNote {
	notes := g.exec.misuseNotes
	g.exec.misuseNotes = nil
	return notes
}

// tellMisuse reports notes to the MisuseSink. The caller holds no graph lock.
// A strict sink panics after recording, and the panic unwinds into the
// caller: the Define, Wait or verb whose scheduling pass found the misuse.
func (g *Graph) tellMisuse(notes []misuseNote) {
	for _, n := range notes {
		g.misuse.RecordMisuseFor(n.subject, n.err)
	}
}

// tellMisuseWithoutCaller is tellMisuse for a pooled worker finishing: no
// caller's code is on its stack to take a strict panic, and the sink has
// already recorded the misuse before it panics, so Finish still reports it.
// The panic stops here instead of ending the process.
func (g *Graph) tellMisuseWithoutCaller(notes []misuseNote) {
	for _, n := range notes {
		g.tellContained(n)
	}
}

func (g *Graph) tellContained(n misuseNote) {
	defer func() { _ = recover() }()
	g.misuse.RecordMisuseFor(n.subject, n.err)
}
