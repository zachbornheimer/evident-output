package graph

// Suspend runs fn, typically an exclusive-terminal window such as an
// interactive prompt or an external program, and starts no new work until it
// returns: a sibling's callback must not begin writing into a window the
// reader is answering. Work already running is not paused. The scheduler
// resumes, and starts what became eligible, once fn returns, however it ends.
// Suspensions nest: work resumes when the last one ends.
//
// A suspended run is not stalled: the window will end, so a wait parked
// meanwhile is neither sealed nor released with ErrWaitDeadlock. The caller
// holds no lock Started takes.
func (g *Graph) Suspend(fn func() error) error {
	g.lock()
	g.exec.suspended++
	g.unlock()
	defer g.resume()
	return fn()
}

// resume ends one Suspend and starts what the pause held back.
func (g *Graph) resume() {
	g.lock()
	g.exec.suspended--
	g.unlock()
	g.Kick()
}

// Suspended reports whether a Suspend window is open.
func (g *Graph) Suspended() bool {
	g.lockRead()
	defer g.unlockRead()
	return g.exec.suspended > 0
}
