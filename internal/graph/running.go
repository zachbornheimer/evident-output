package graph

import "sync"

// starter is the work that started a goroutine: the goroutine running it, and
// the Task or builder gate it is running for. The zero starter is none.
type starter struct {
	goroutine GoroutineID
	task      *Task
}

// startingGoroutine is the nearest goroutine running a task callback or
// builder that started creator's child, through any number of goroutines in
// between, and the work it runs that did; the zero starter when none did.
// Walking starts from creator, and stops the world only when creator
// is one nothing has asked about yet (see lineage), so every goroutine one
// creator starts shares a single dump, and the running callbacks' goroutines
// cost a bounded number of dumps in all (see refreshesPerCallback); with no
// goroutine running anything it reads nothing.
func (g *Graph) startingGoroutine(creator GoroutineID) starter {
	if creator == 0 {
		return starter{}
	}
	direct, working := g.frames.on(creator), g.frames.anyRunning()
	if direct != nil {
		return starter{goroutine: creator, task: direct}
	}
	if !working {
		return starter{}
	}
	for _, id := range processLineage.chain(creator, &g.frames.refreshes) {
		if task := g.frames.on(id); task != nil {
			return starter{goroutine: id, task: task}
		}
	}
	return starter{}
}

// workFrame is a claim's work on the goroutine running it: a task callback,
// or a container builder for a gate. The zero workFrame was never entered.
type workFrame struct {
	goroutine GoroutineID
	task      *Task
}

// frames is which work each goroutine runs right now: the Task (or builder
// gate) a Computed read there speaks for, and that a goroutine it started
// is traced back to. Go has no goroutine-local storage and a read takes no
// context, so the scheduler records it per goroutine. It has its own lock,
// beneath the graph's: a worker entering or leaving work touches nothing
// else, and must not queue behind scheduling.
type frames struct {
	mu sync.Mutex
	// stacks is, per goroutine, the work it runs, innermost last.
	stacks map[GoroutineID][]*Task
	// loaded is how many frames are open across every goroutine.
	loaded int
	// refreshes is how many more dumps asking about unknown creators the run
	// may take, earned by the work running (see refreshesPerCallback).
	refreshes refreshBudget
}

// enter makes the calling goroutine t's. Work beginning also earns the run
// the dumps asking about unknown creators may cost (see
// refreshesPerCallback). id is the calling goroutine's, read once by the
// worker that runs the callback (see worker).
func (f *frames) enter(t *Task, id GoroutineID) workFrame {
	frame := workFrame{goroutine: id, task: t}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stacks == nil {
		f.stacks = make(map[GoroutineID][]*Task)
	}
	f.stacks[frame.goroutine] = append(f.stacks[frame.goroutine], t)
	f.loaded++
	f.refreshes.settle(f.loaded, refreshesPerCallback)
	return frame
}

// leave ends frame, giving back the share of dumps its work held. A frame
// never entered leaves nothing.
func (f *frames) leave(frame workFrame) {
	if frame.task == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id := frame.goroutine
	if stack := f.stacks[id]; len(stack) <= 1 {
		delete(f.stacks, id)
	} else {
		f.stacks[id] = stack[:len(stack)-1]
	}
	f.loaded--
	f.refreshes.settle(f.loaded, 0)
}

// on is the Task or builder gate running on goroutine id, or nil. A pooled
// worker has started no goroutine before its callback began (see worker), so
// every goroutine it starts is that callback's.
func (f *frames) on(id GoroutineID) *Task {
	f.mu.Lock()
	defer f.mu.Unlock()
	if stack := f.stacks[id]; len(stack) > 0 {
		return stack[len(stack)-1]
	}
	return nil
}

// anyRunning reports whether any goroutine runs a callback or builder.
func (f *frames) anyRunning() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loaded > 0
}
