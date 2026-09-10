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
			st.handle.Fail(fmt.Sprintf("panic: %v", r))
		}
		o.mu.Lock()
		o.schedInflight--
		o.mu.Unlock()
		o.schedWG.Done()
		o.kick()
	}()

	dryRun := false
	subject := ""
	o.mu.Lock()
	if st != nil {
		subject = st.name
	}
	dryRun = o.cfg.dryRun
	o.mu.Unlock()

	if mut != nil && dryRun {
		o.recordResolvedMutation(subject, true, mut.verb, mut.quantity, mut.hasQty, mut.object)
		st.handle.Done()
		return
	}
	var err error
	if fn != nil {
		err = fn()
	}
	if err != nil {
		st.handle.Fail(err.Error())
		o.failSequenceFollowers(st)
		return
	}
	if mut != nil {
		o.recordResolvedMutation(subject, false, mut.verb, mut.quantity, mut.hasQty, mut.object)
	}
	if st.handle.Snapshot().State == Running || st.handle.Snapshot().State == Pending {
		st.handle.Done()
	}
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
