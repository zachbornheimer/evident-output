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
// Reuse depends on Go 1.27: the census reads the /sched/goroutines-created
// runtime metric, which no earlier toolchain has. Without it (the module still
// builds on Go 1.25) no worker is ever reusable, so every claim runs on a
// goroutine of its own, as it did before pooling; WorkerReuseAvailable says
// which case a build is in.
//
// A worker may serve another claim only if it has started no goroutine, ever.
// The census is process-wide, so a goroutine any other code starts while a
// claim runs also ends the worker: reuse is an optimisation that may be
// declined, never a guarantee, and declining is always safe.
// A goroutine a callback started can outlive the callback, and a Computed read
// from it must not find the next claim's work running on its creator. The
// goroutine count settles it (see census): a worker whose claim saw any
// goroutine start but the pool's own ends after that claim, and so does one
// that started a worker itself.
//
// A callback that exits through runtime.Goexit is not reusable: its goroutine
// is already unwinding, so finishClaimed runs on a goroutine that ends when it
// returns, and reusable stays false. Only a callback that returned or
// panicked (and was recovered) leaves a worker that can run another claim.
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
//
// spawns is the ledger every worker the pool starts goes through, and the one
// a worker's census is read from. A graph uses the process-wide workerSpawns.
type workerPool struct {
	mu        sync.Mutex
	idle      []*worker
	dismissed bool
	spawns    *spawnLedger
	// serving is the workers alive, by the goroutine each runs on, so the
	// worker a goroutine is can be told when it starts another.
	serving map[GoroutineID]*worker
}

// enroll records w as serving on its goroutine.
func (p *workerPool) enroll(w *worker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.serving == nil {
		p.serving = make(map[GoroutineID]*worker)
	}
	p.serving[w.id] = w
}

// retire forgets w, whose goroutine is ending.
func (p *workerPool) retire(w *worker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.serving, w.id)
}

// markStarter ends the reuse of the worker the calling goroutine is, if it is
// one: the worker is about to start a goroutine, and a goroutine started
// during a callback is that callback's, whatever the worker runs next. The
// caller is the worker itself, so the mark needs no lock. A pool with no
// worker alive costs no goroutine id.
func (p *workerPool) markStarter() {
	p.mu.Lock()
	alive := len(p.serving) > 0
	p.mu.Unlock()
	if !alive {
		return
	}
	id := CurrentGoroutine()
	p.mu.Lock()
	w := p.serving[id]
	p.mu.Unlock()
	if w != nil {
		w.spawned = true
	}
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
// kept, else one that is idle, else a new one. A new worker is a goroutine its
// starter started, so it ends the starter: w, or, for a nil w, the worker the
// calling goroutine is when a callback's own scheduling pass starts it.
func (g *Graph) place(c *claim, w *worker) {
	switch {
	case w != nil && w.kept == nil && !w.spawned:
		w.kept = c
	case g.workers.hand(c):
	default:
		if w == nil {
			g.workers.markStarter()
		}
		g.workers.spawns.start(func() { g.serve(c) })
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
	g.workers.enroll(w)
	defer g.workers.retire(w)
	for c := first; c != nil; c = g.nextClaim(w) {
		w.opened = g.workers.spawns.count()
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
// claim. It is process-wide because the census it is compared with is.
var workerSpawns spawnLedger

// spawnLedger counts the goroutines started through it, so they can be told
// from every other goroutine the process starts.
type spawnLedger struct {
	// active is the spawns begun and not finished, done the ones finished.
	active atomic.Int64
	done   atomic.Uint64
	// created reads how many goroutines the process has started; nil reads
	// the runtime's count. A ledger whose created counts only its own spawns
	// sees no goroutine but its own, however busy the process is.
	created func() (uint64, bool)
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
	created, counted := l.processCreated()
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

// processCreated is how many goroutines the process has started, as the ledger
// counts them.
func (l *spawnLedger) processCreated() (uint64, bool) {
	if l.created != nil {
		return l.created()
	}
	return goroutinesCreated()
}

// WorkerReuseAvailable reports whether this toolchain lets a worker serve more
// than one claim: it needs the /sched/goroutines-created runtime metric, which
// Go 1.27 added. Where it is missing every claim runs on its own goroutine.
func WorkerReuseAvailable() bool {
	_, counted := goroutinesCreated()
	return counted
}
