package graph

import (
	"sync"
	"sync/atomic"
)

// A worker is a goroutine that runs claimed Task callbacks, one after another.
// Reading a goroutine's id costs a traceback under the runtime's process-wide
// print lock, so a goroutine reads its own once and then serves every claim it
// may.
//
// A worker may serve another claim only if it has started no goroutine, ever.
// A goroutine a callback started can outlive the callback, and a Computed read
// from it must not find the next claim's work running on its creator. The
// goroutine count settles it (see census): a worker whose claim saw any
// goroutine start but the pool's own ends after that claim, and so does one
// that started a worker itself.
type worker struct {
	// next carries the claim an idle worker is handed, or nil to end it.
	next chan *claim
	id   GoroutineID
	// opened is the census taken before the claim's callback ran.
	opened census
	// kept is the claim this worker runs next, taken from the scheduling
	// pass it ran as its claim ended. spawned is whether that pass started
	// a goroutine, which ends the worker.
	kept    *claim
	spawned bool
	// reusable is whether the worker may run another claim.
	reusable bool
}

// workerPool is the workers waiting for a claim. It has its own lock: a
// declaration hands over a claim without queuing on the graph's. A worker
// waits only while the run does: the drain ends it, as does Close.
type workerPool struct {
	mu        sync.Mutex
	idle      []*worker
	dismissed bool
}

// hand gives c to the idle worker that waited least long and reports whether
// there was one.
func (p *workerPool) hand(c *claim) bool {
	p.mu.Lock()
	n := len(p.idle)
	if n == 0 {
		p.mu.Unlock()
		return false
	}
	w := p.idle[n-1]
	p.idle[n-1] = nil
	p.idle = p.idle[:n-1]
	p.mu.Unlock()
	w.next <- c
	return true
}

// wait idles w until a claim is handed to it and returns that claim; nil
// means w must end, as it does once the pool is dismissed.
func (p *workerPool) wait(w *worker) *claim {
	p.mu.Lock()
	if p.dismissed {
		p.mu.Unlock()
		return nil
	}
	p.idle = append(p.idle, w)
	p.mu.Unlock()
	return <-w.next
}

// dismiss ends every idle worker and every worker that finishes afterwards: the
// run has no claim left to hand out.
func (p *workerPool) dismiss() {
	p.mu.Lock()
	p.dismissed = true
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()
	for _, w := range idle {
		w.next <- nil
	}
}

// place sends c to a worker: w itself when it is ending a claim and has none
// kept, else one that is idle, else a new one. A new worker is a goroutine w
// started, so it ends w. A nil w is a caller that is not a worker.
func (g *Graph) place(c *claim, w *worker) {
	switch {
	case w != nil && w.kept == nil && !w.spawned:
		w.kept = c
	case g.workers.hand(c):
	default:
		workerSpawns.start(func() { g.serve(c) })
		if w != nil {
			w.spawned = true
		}
	}
}

// rehome hands the claim w kept to another worker when w turns out not to be
// reusable.
func (g *Graph) rehome(w *worker) {
	if c := w.kept; c != nil && !w.reusable {
		w.kept = nil
		g.place(c, nil)
	}
}

// serve is a worker's goroutine: it runs first, then every claim handed to
// it, for as long as it may.
func (g *Graph) serve(first *claim) {
	w := &worker{next: make(chan *claim, 1), id: CurrentGoroutine()}
	for c := first; c != nil; c = g.nextClaim(w) {
		w.opened = workerSpawns.count()
		g.runPooled(c, w)
	}
}

// nextClaim is the claim w runs after the one it finished: the one its own
// pass kept, else the next one handed to it, else nil when w must end.
func (g *Graph) nextClaim(w *worker) *claim {
	if !w.reusable {
		return nil
	}
	if c := w.kept; c != nil {
		w.kept = nil
		return c
	}
	return g.workers.wait(w)
}

// workerSpawns is the ledger of every goroutine this package starts for a
// claim.
var workerSpawns spawnLedger

// spawnLedger counts the goroutines started through it, so they can be told
// from every other goroutine the process starts.
type spawnLedger struct {
	// active is the spawns begun and not finished, done the ones finished.
	active atomic.Int64
	done   atomic.Uint64
}

// start runs f on a new goroutine.
func (l *spawnLedger) start(f func()) {
	l.active.Add(1)
	go f()
	l.done.Add(1)
	l.active.Add(-1)
}

// census is how many goroutines the process had started and how many of them
// went through the ledger. consistent is whether the two counts describe the
// same instant: false while a spawn was under way, or where the toolchain
// keeps no count.
type census struct {
	created, ours uint64
	consistent    bool
}

// count reads the census. A ledger spawn that begins or ends while it reads
// makes it inconsistent, and a spawn that is neither under way before nor
// during the read is in both counts or in neither.
func (l *spawnLedger) count() census {
	before, ours := l.active.Load(), l.done.Load()
	created, counted := goroutinesCreated()
	steady := before == 0 && l.active.Load() == 0 && l.done.Load() == ours
	return census{created: created, ours: ours, consistent: counted && steady}
}

// quiet reports whether no goroutine but the ledger's started since the
// census since was taken. A census that is not consistent proves nothing, so
// it answers false.
func (l *spawnLedger) quiet(since census) bool {
	now := l.count()
	return since.consistent && now.consistent && now.created-since.created == now.ours-since.ours
}
