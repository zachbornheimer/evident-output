package evo

import (
	"fmt"
	"runtime"

	"github.com/zachbornheimer/evident-output/internal/core"
)

type mutationSpec struct {
	verb     string
	object   string
	quantity int64
	hasQty   bool
}

type predecessor struct {
	taskID  string
	groupID string
}

func (t *TaskHandle) Define(fn func() error) {
	t.submitWork(fn, nil)
}

func (t *TaskHandle) After(preds ...any) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return t
	}
	if st.submitted {
		t.out.recordMisuse(ErrInvalidConfig)
		return t
	}
	for _, p := range preds {
		if p == nil {
			continue
		}
		switch x := p.(type) {
		case *TaskHandle:
			if x == nil || x.id == "" {
				continue
			}
			st.preds = append(st.preds, predecessor{taskID: x.id})
		case *GroupHandle:
			if x == nil || x.id == "" {
				continue
			}
			st.preds = append(st.preds, predecessor{groupID: x.id})
		case *SequenceHandle:
			if x == nil || x.tasks == nil {
				continue
			}
			st.preds = append(st.preds, predecessor{groupID: x.tasks.id})
		}
	}
	return t
}

func (t *TaskHandle) submitWork(fn func() error, mut *mutationSpec) {
	if t == nil || t.out == nil {
		return
	}
	o := t.out
	o.mu.Lock()
	st := o.taskByRef[t.id]
	if st == nil {
		o.mu.Unlock()
		return
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		o.mu.Unlock()
		return
	}
	if core.IsTerminalTask(st.state) {
		o.recordMisuseFor(st.name, ErrAlreadyResolved)
		o.mu.Unlock()
		return
	}
	if st.submitted {
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	st.submitted = true
	st.workFn = fn
	st.mutation = mut
	o.schedWG.Add(1)
	o.mu.Unlock()
	o.kick()
}

func (o *Output) kick() {
	for {
		st, fn, mut := o.takeEligible()
		if st == nil {
			return
		}
		go o.runWork(st, fn, mut)
	}
}

func (o *Output) takeEligible() (st *taskState, fn func() error, mut *mutationSpec) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.schedCancelled {
		// After an interrupt the queue is abandoned, not drained: nothing
		// new starts, so the run stops at the ^C instead of running to
		// completion behind one cancelled row.
		return nil, nil, nil
	}
	max := o.concurrencyCeilingLocked()
	if o.schedInflight >= max {
		return nil, nil, nil
	}
	for _, cand := range o.tasks {
		if !cand.submitted || cand.runningWork || core.IsTerminalTask(cand.state) {
			continue
		}
		if !o.eligibleLocked(cand) {
			continue
		}
		if o.schedInflight >= max {
			return nil, nil, nil
		}
		cand.runningWork = true
		o.schedInflight++
		if o.schedInflight > o.schedMaxObserved {
			o.schedMaxObserved = o.schedInflight
		}
		o.schedStartOrder = append(o.schedStartOrder, cand.name)
		if cand.state == Pending {
			o.promoteRunningLocked(cand)
		}
		o.bumpLocked()
		o.signalLiveLocked(true)
		return cand, cand.workFn, cand.mutation
	}
	return nil, nil, nil
}

func (o *Output) concurrencyCeilingLocked() int {
	if o.cfg.maxConcurrency > 0 {
		return o.cfg.maxConcurrency
	}
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	return n
}

func (o *Output) runWork(st *taskState, fn func() error, mut *mutationSpec) {
	defer func() {
		if r := recover(); r != nil {
			st.handle.failScheduled(fmt.Sprintf("panic: %v", r))
		}
		o.mu.Lock()
		o.schedInflight--
		o.mu.Unlock()
		o.schedWG.Done()
		o.kick()
	}()

	o.executeWork(st, fn, mut)
}

// executeWork runs one task's callback and resolves the task from what it
// returned — the scheduler's sole resolution point (see TaskHandle.finish).
// Shared by the pooled worker (runWork) and by a waiter that donates its own
// goroutine to work it would otherwise block on (TaskHandle.Wait).
func (o *Output) executeWork(st *taskState, fn func() error, mut *mutationSpec) {
	o.mu.Lock()
	subject := ""
	if st != nil {
		subject = st.name
	}
	dryRun := o.cfg.dryRun
	o.mu.Unlock()

	if mut != nil && dryRun {
		o.recordResolvedMutation(subject, true, mut.verb, mut.quantity, mut.hasQty, mut.object)
		o.recordWorkOutcome(st, nil)
		o.resolveObserved(st, nil)
		return
	}
	var err error
	if fn != nil {
		err = fn()
	}
	o.recordWorkOutcome(st, err)
	// The effect commits on the callback's success alone, before any
	// resolution: a task that mutated and then failed still owes the reader
	// its "! already mutated: ..." line.
	if err == nil && mut != nil {
		o.recordResolvedMutation(subject, false, mut.verb, mut.quantity, mut.hasQty, mut.object)
	}
	// A callback that resolved its own task (Failf/Fail/Block inside fn, or
	// an interrupt that cancelled the row) already stated one outcome. The
	// scheduler neither restates it nor calls it misuse (P13) — returning
	// the same error it already reported is the documented Failf shape.
	if o.taskIsTerminal(st) {
		if err != nil {
			o.failSequenceFollowers(st)
		}
		return
	}
	o.resolveObserved(st, err)
	if err != nil {
		o.failSequenceFollowers(st)
	}
}

// resolveObserved commits the task's outcome from what the callback actually
// returned, ratifying or rejecting the caller's proposal (see
// TaskHandle.finish). A ratified proposal supplies the row's summary — the
// caller's own words, now backed by an observation.
func (o *Output) resolveObserved(st *taskState, err error) {
	proposal := o.takeProposal(st)
	if err != nil {
		if proposal != nil {
			o.rejectProposal(st, proposal)
		}
		st.handle.failScheduled(err.Error())
		return
	}
	if proposal != nil {
		st.handle.resolveScheduled(proposal.state, proposal.summary, proposal.problems)
		return
	}
	st.handle.doneScheduled()
}

func (o *Output) takeProposal(st *taskState) *proposedOutcome {
	if st == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	proposal := st.proposed
	st.proposed = nil
	return proposal
}

// rejectProposal records the misuse a contradicted success claim earns: the
// caller said the work was done, the work says otherwise, and the reader is
// told which task and which claim was dropped.
func (o *Output) rejectProposal(st *taskState, proposal *proposedOutcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.recordAlreadyResolvedLocked(st.name, proposal.summary)
}

// recordWorkOutcome stores the callback's error on the task so a waiter
// (TaskHandle.Wait) can return the same value the callback returned.
func (o *Output) recordWorkOutcome(st *taskState, err error) {
	if st == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	st.workErr = err
}

func (o *Output) taskIsTerminal(st *taskState) bool {
	if st == nil {
		return true
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return core.IsTerminalTask(st.state)
}

// runForWaiter executes the task a caller is about to block on, on the
// caller's own goroutine, when the scheduler has not started it yet. It
// consumes no scheduler slot: the waiting goroutine either already holds one
// (a callback nested inside another callback) or holds none at all, so the
// number of callbacks actually executing never rises above the ceiling.
// A no-op when the task is not claimable — already running, already
// terminal, never defined, not yet eligible, or the run is cancelling.
func (o *Output) runForWaiter(taskID string) {
	st, fn, mut, claimed := o.claimForWaiter(taskID)
	if !claimed {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			st.handle.failScheduled(fmt.Sprintf("panic: %v", r))
		}
		o.schedWG.Done()
		o.kick()
	}()
	o.executeWork(st, fn, mut)
}

// claimForWaiter marks one submitted, eligible, not-yet-started task as
// running for a waiting goroutine — takeEligible's body without the
// concurrency ceiling and without the in-flight accounting (see
// runForWaiter).
func (o *Output) claimForWaiter(taskID string) (st *taskState, fn func() error, mut *mutationSpec, claimed bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	cand := o.taskByRef[taskID]
	if o.schedCancelled || cand == nil || !cand.submitted || cand.runningWork || core.IsTerminalTask(cand.state) {
		return nil, nil, nil, false
	}
	if !o.eligibleLocked(cand) {
		return nil, nil, nil, false
	}
	cand.runningWork = true
	o.schedStartOrder = append(o.schedStartOrder, cand.name)
	if cand.state == Pending {
		o.promoteRunningLocked(cand)
	}
	o.bumpLocked()
	o.signalLiveLocked(true)
	return cand, cand.workFn, cand.mutation, true
}

func (o *Output) drainScheduler() {
	o.mu.Lock()
	o.schedDraining = true
	o.cascadeIneligibleLocked()
	o.mu.Unlock()
	o.kick()
	o.schedWG.Wait()
}

func (o *Output) eligibleLocked(st *taskState) bool {
	if !o.predsSatisfiedLocked(st) {
		return false
	}
	if st.collection != nil && st.collection.sequential {
		if prev := previousSibling(st); prev != nil && !predecessorSucceeded(prev.state) {
			return false
		}
	}
	return true
}

func (o *Output) predsSatisfiedLocked(st *taskState) bool {
	for _, p := range st.preds {
		if p.taskID != "" {
			pred := o.taskByRef[p.taskID]
			if pred == nil || !predecessorSucceeded(pred.state) {
				return false
			}
			continue
		}
		if p.groupID != "" {
			col := o.tasksByRef[p.groupID]
			if col == nil || !collectionSucceeded(col) {
				return false
			}
		}
	}
	return true
}

func (o *Output) cascadeIneligibleLocked() {
	changed := true
	for changed {
		changed = false
		for _, st := range o.tasks {
			if !st.submitted || st.runningWork || core.IsTerminalTask(st.state) {
				continue
			}
			if o.eligibleLocked(st) {
				continue
			}
			if o.canStillBecomeEligibleLocked(st) && !o.predecessorBlockedLocked(st) {
				continue
			}
			o.markNotStartedLocked(st)
			changed = true
		}
	}
}

func (o *Output) canStillBecomeEligibleLocked(st *taskState) bool {
	for _, p := range st.preds {
		if p.taskID != "" {
			pred := o.taskByRef[p.taskID]
			if pred != nil && !core.IsTerminalTask(pred.state) {
				return true
			}
			continue
		}
		if p.groupID != "" {
			col := o.tasksByRef[p.groupID]
			if col != nil && !collectionResolved(col) {
				return true
			}
		}
	}
	if st.collection != nil && st.collection.sequential {
		if prev := previousSibling(st); prev != nil && !core.IsTerminalTask(prev.state) {
			return true
		}
	}
	return false
}

func (o *Output) predecessorBlockedLocked(st *taskState) bool {
	for _, p := range st.preds {
		if p.taskID != "" {
			pred := o.taskByRef[p.taskID]
			if pred != nil && predecessorFailed(pred.state) {
				return true
			}
			if o.schedDraining && (pred == nil || !pred.submitted && !core.IsTerminalTask(pred.state)) {
				return true
			}
			continue
		}
		if p.groupID != "" {
			col := o.tasksByRef[p.groupID]
			if col != nil && collectionFailed(col) {
				return true
			}
		}
	}
	if st.collection != nil && st.collection.sequential {
		if prev := previousSibling(st); prev != nil && predecessorFailed(prev.state) {
			return true
		}
	}
	return false
}

func (o *Output) markNotStartedLocked(st *taskState) {
	st.state = NotStarted
	st.phase = ""
	st.summary = notStartedSummary
	st.runningWork = true
	if st.submitted {
		st.submitted = false
		o.schedWG.Done()
	}
	st.closeDoneLocked()
	o.appendEventLocked(Event{Type: "task.not_started", EntityID: st.id})
}

func (o *Output) failSequenceFollowers(failed *taskState) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if failed.collection == nil || !failed.collection.sequential {
		return
	}
	seen := false
	for _, sib := range failed.collection.tasks {
		if sib == failed {
			seen = true
			continue
		}
		if !seen || core.IsTerminalTask(sib.state) || sib.runningWork {
			continue
		}
		o.markNotStartedLocked(sib)
	}
}

func previousSibling(st *taskState) *taskState {
	if st.collection == nil {
		return nil
	}
	var prev *taskState
	for _, sib := range st.collection.tasks {
		if sib == st {
			return prev
		}
		prev = sib
	}
	return nil
}

func predecessorSucceeded(s EntityState) bool {
	return s == Done || s == Skipped
}

func predecessorFailed(s EntityState) bool {
	return s == Failed || s == Blocked || s == Cancelled || s == NotStarted
}

func collectionSucceeded(col *tasksState) bool {
	if col == nil {
		return false
	}
	if len(col.tasks) == 0 && len(col.children) == 0 {
		return true
	}
	for _, t := range col.tasks {
		if !predecessorSucceeded(t.state) {
			return false
		}
	}
	for _, child := range col.children {
		if !collectionSucceeded(child) {
			return false
		}
	}
	return true
}

func collectionFailed(col *tasksState) bool {
	if col == nil {
		return false
	}
	for _, t := range col.tasks {
		if predecessorFailed(t.state) {
			return true
		}
	}
	for _, child := range col.children {
		if collectionFailed(child) {
			return true
		}
	}
	return false
}

func collectionResolved(col *tasksState) bool {
	if col == nil {
		return true
	}
	for _, t := range col.tasks {
		if !core.IsTerminalTask(t.state) {
			return false
		}
	}
	for _, child := range col.children {
		if !collectionResolved(child) {
			return false
		}
	}
	return true
}

func (st *taskState) closeDoneLocked() {
	if st.doneCh == nil {
		return
	}
	st.doneOnce.Do(func() { close(st.doneCh) })
}

// Wait blocks until the task is terminal and returns the error its callback
// returned (nil on Done, Skipped, or a dry run that never invoked it).
//
// A task that was already resolved before it was defined never runs its
// callback (the misuse is recorded at the Define call), and Wait returns nil
// immediately rather than blocking on work that will never happen (P15).
//
// A waiter has stopped doing work, so the concurrency ceiling must not be
// the reason the task it waits on cannot start (P16). Wait therefore runs
// that task on its own goroutine when the scheduler has not picked it up:
// nested Define+Wait completes even at MaxConcurrency 1, because the waiting
// callback's slot carries the work it is waiting for instead of idling.
func (t *TaskHandle) Wait() error {
	if t == nil || t.out == nil {
		return nil
	}
	t.out.runForWaiter(t.id)
	t.waitSubmitted()
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	if st == nil {
		return nil
	}
	return st.workErr
}

func (t *TaskHandle) wasSubmitted() bool {
	if t == nil || t.out == nil {
		return false
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskByRef[t.id]
	return st != nil && st.submitted
}

func (t *TaskHandle) waitSubmitted() {
	if t == nil || t.out == nil {
		return
	}
	t.out.mu.Lock()
	st := t.out.taskByRef[t.id]
	if st == nil || (!st.submitted && !st.runningWork && !core.IsTerminalTask(st.state)) {
		t.out.mu.Unlock()
		return
	}
	ch := st.doneCh
	t.out.mu.Unlock()
	if ch != nil {
		<-ch
	}
}
