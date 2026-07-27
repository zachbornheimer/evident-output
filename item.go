package evo

import "github.com/zachbornheimer/evident-output/internal/sanitize"

// Item is a handle for one named final-report condition.
type Item struct {
	out *Output
	id  string
}

// Start marks the item running so it becomes visible in the live region
// (indeterminate) while the application evaluates it. Optional: OK/Block/…
// may resolve pending items directly without Start (no transient frame when
// resolution is instant — §7.4 rule 5).
func (i *Item) Start() *Item {
	i.out.mu.Lock()
	defer i.out.mu.Unlock()
	st := i.out.itemByRef[i.id]
	if st == nil {
		return i
	}
	if err := i.out.ensureOpen(); err != nil {
		i.out.recordMisuse(err)
		return i
	}
	if isTerminalItem(st.state) {
		i.out.recordMisuse(ErrAlreadyResolved)
		return i
	}
	if st.state == Running {
		return i
	}
	st.state = Running
	i.out.bumpLocked()
	i.out.appendEventLocked(Event{Type: "item.started", EntityID: i.id})
	i.out.signalLiveLocked(true)
	return i
}

// OK marks the item satisfactory.
func (i *Item) OK() *Item {
	i.resolve(OK, nil)
	return i
}

// Warn marks the item with a simple warning problem.
func (i *Item) Warn(summary string, options ...ProblemOption) *Item {
	p := applyProblemOptions(sanitize.Text(summary), options)
	i.resolve(Warning, []Problem{p})
	return i
}

// WarnedBy marks the item warning with structured problems.
func (i *Item) WarnedBy(problems ...Problem) *Item {
	i.resolveBy(Warning, problems)
	return i
}

// Block marks the item blocked with a simple problem.
func (i *Item) Block(summary string, options ...ProblemOption) *Item {
	p := applyProblemOptions(sanitize.Text(summary), options)
	i.resolve(Blocked, []Problem{p})
	return i
}

// BlockedBy marks the item blocked with structured problems.
func (i *Item) BlockedBy(problems ...Problem) *Item {
	i.resolveBy(Blocked, problems)
	return i
}

// Fail marks the item failed with a simple problem.
func (i *Item) Fail(summary string, options ...ProblemOption) *Item {
	p := applyProblemOptions(sanitize.Text(summary), options)
	i.resolve(Failed, []Problem{p})
	return i
}

// FailedBy marks the item failed with structured problems.
func (i *Item) FailedBy(problems ...Problem) *Item {
	i.resolveBy(Failed, problems)
	return i
}

// Unknown marks the item undetermined.
func (i *Item) Unknown(summary string, options ...ProblemOption) *Item {
	p := applyProblemOptions(sanitize.Text(summary), options)
	i.resolve(Unknown, []Problem{p})
	return i
}

// Skip marks the item skipped.
func (i *Item) Skip(reason string) *Item {
	i.out.mu.Lock()
	defer i.out.mu.Unlock()
	st := i.out.itemByRef[i.id]
	if st == nil {
		return i
	}
	if err := i.out.ensureOpen(); err != nil {
		i.out.recordMisuse(err)
		return i
	}
	if isTerminalItem(st.state) {
		i.out.recordMisuse(ErrAlreadyResolved)
		return i
	}
	st.state = Skipped
	st.because = sanitize.Text(reason)
	i.out.bumpLocked()
	i.out.appendEventLocked(Event{Type: "item.skipped", EntityID: i.id})
	i.out.emitItemProgressiveLocked(st)
	return i
}

// Because annotates an explanation after resolution.
func (i *Item) Because(text string) *Item {
	i.out.mu.Lock()
	defer i.out.mu.Unlock()
	st := i.out.itemByRef[i.id]
	if st == nil {
		return i
	}
	if i.out.finishing || i.out.finished || i.out.closed {
		i.out.recordMisuse(ErrClosed)
		return i
	}
	st.because = sanitize.Text(text)
	i.out.bumpLocked()
	i.out.appendEventLocked(Event{Type: "item.annotated", EntityID: i.id})
	// Stream the explanation as soon as it is known (fluent Block…Because chain).
	i.out.emitItemProgressiveLocked(st)
	return i
}

// Next attaches actions.
func (i *Item) Next(actions ...Action) *Item {
	i.out.mu.Lock()
	defer i.out.mu.Unlock()
	st := i.out.itemByRef[i.id]
	if st == nil {
		return i
	}
	if i.out.finishing || i.out.finished || i.out.closed {
		i.out.recordMisuse(ErrClosed)
		return i
	}
	st.actions = append(st.actions, cloneActions(actions)...)
	i.out.bumpLocked()
	i.out.emitItemProgressiveLocked(st)
	return i
}

// NextCommand attaches a command action.
func (i *Item) NextCommand(executable string, args ...string) *Item {
	return i.Next(Command(executable, args...))
}

// Snapshot returns the item's current snapshot.
func (i *Item) Snapshot() ItemSnapshot {
	i.out.mu.Lock()
	defer i.out.mu.Unlock()
	st := i.out.itemByRef[i.id]
	if st == nil {
		return ItemSnapshot{ID: i.id, State: Pending}
	}
	return st.snapshot()
}

func (i *Item) resolve(state EntityState, problems []Problem) {
	i.out.mu.Lock()
	defer i.out.mu.Unlock()
	st := i.out.itemByRef[i.id]
	if st == nil {
		return
	}
	if err := i.out.ensureOpen(); err != nil {
		i.out.recordMisuse(err)
		return
	}
	if isTerminalItem(st.state) {
		i.out.recordMisuse(ErrAlreadyResolved)
		return
	}
	st.state = state
	if len(problems) > 0 {
		st.problems = cloneProblems(problems)
		for j := range st.problems {
			st.problems[j].Summary = sanitize.Text(st.problems[j].Summary)
			st.problems[j].Detail = sanitize.Text(st.problems[j].Detail)
			st.problems[j].Subject = sanitize.Text(st.problems[j].Subject)
		}
	}
	i.out.bumpLocked()
	i.out.appendEventLocked(Event{Type: "item." + string(state), EntityID: i.id})
	// §17.5: terminal outcomes render immediately — durable evidence, not Finish buffer.
	i.out.emitItemProgressiveLocked(st)
}

func (i *Item) resolveBy(state EntityState, problems []Problem) {
	if len(problems) == 0 {
		i.out.mu.Lock()
		defer i.out.mu.Unlock()
		i.out.recordMisuse(ErrNoProblems)
		return
	}
	i.resolve(state, problems)
}
