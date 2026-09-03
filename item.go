package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

// Item is a handle for one named final-report condition.
type ItemHandle struct {
	out *Output
	id  string
}

// Start marks the item running so it becomes visible in the live region
// (indeterminate) while the application evaluates it. Optional: OK/Block/…
// may resolve pending items directly without Start (no transient frame when
// resolution is instant — §7.4 rule 5).
func (i *ItemHandle) Start() *ItemHandle {
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
func (i *ItemHandle) OK() *ItemHandle {
	i.resolve(OK, nil)
	return i
}

// Warn marks the item with a simple warning problem.
func (i *ItemHandle) Warn(summary string, options ...ProblemOption) *ItemHandle {
	p := applyProblemOptions(sanitize.Text(summary), options)
	i.resolve(Warning, []Problem{p})
	return i
}

// WarnedBy marks the item warning with structured problems.
func (i *ItemHandle) WarnedBy(problems ...Problem) *ItemHandle {
	i.resolveBy(Warning, problems)
	return i
}

// Block marks the item blocked. This is a statement, not a fluent chain —
// Block returns nothing, so a bare `item.Block("summary")` is errcheck-clean.
// A nil *ItemHandle is safe and resolves nothing. Use Blockf to build and
// return a %w-wrapped error in one line.
func (i *ItemHandle) Block(summary string, options ...ProblemOption) {
	p := applyProblemOptions(sanitize.Text(summary), options)
	if i != nil {
		i.resolve(Blocked, []Problem{p})
	}
}

// Blockf marks the item blocked with a formatted summary and returns the
// built error so a call site can `return` it directly:
// `return item.Blockf("policy manifest missing: %w", err)`. fmt.Errorf
// semantics: %w wraps its argument so errors.Is/As still reach it. See
// splitWrappedMessage for how a trailing ": %w"/", %w" splits the formatted
// text into the rendered summary and evidence line.
func (i *ItemHandle) Blockf(format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	summary, evidence := splitWrappedMessage(format, err)
	p := sanitizeProblem(Problem{Summary: summary, Detail: evidence})
	if i != nil {
		i.resolve(Blocked, []Problem{p})
	}
	return err
}

// BlockedBy marks the item blocked with structured problems.
func (i *ItemHandle) BlockedBy(problems ...Problem) *ItemHandle {
	i.resolveBy(Blocked, problems)
	return i
}

// Fail marks the item failed. This is a statement, not a fluent chain — Fail
// returns nothing, so a bare `item.Fail("summary")` is errcheck-clean. A nil
// *ItemHandle is safe and resolves nothing. Use Failf to build and return a
// %w-wrapped error in one line.
func (i *ItemHandle) Fail(summary string, options ...ProblemOption) {
	p := applyProblemOptions(sanitize.Text(summary), options)
	if i != nil {
		i.resolve(Failed, []Problem{p})
	}
}

// Failf marks the item failed with a formatted summary and returns the built
// error exactly like Blockf — see Blockf for the fmt.Errorf %w and
// summary/evidence split contract.
func (i *ItemHandle) Failf(format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	summary, evidence := splitWrappedMessage(format, err)
	p := sanitizeProblem(Problem{Summary: summary, Detail: evidence})
	if i != nil {
		i.resolve(Failed, []Problem{p})
	}
	return err
}

// FailedBy marks the item failed with structured problems.
func (i *ItemHandle) FailedBy(problems ...Problem) *ItemHandle {
	i.resolveBy(Failed, problems)
	return i
}

// Unknown marks the item undetermined.
func (i *ItemHandle) Unknown(summary string, options ...ProblemOption) *ItemHandle {
	p := applyProblemOptions(sanitize.Text(summary), options)
	i.resolve(Unknown, []Problem{p})
	return i
}

// Skip marks the item skipped.
func (i *ItemHandle) Skip(reason string) *ItemHandle {
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
func (i *ItemHandle) Because(text string) *ItemHandle {
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
func (i *ItemHandle) Next(actions ...Action) *ItemHandle {
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
func (i *ItemHandle) NextCommand(executable string, args ...string) *ItemHandle {
	return i.Next(Command(executable, args...))
}

// Snapshot returns the item's current snapshot.
func (i *ItemHandle) Snapshot() ItemSnapshot {
	i.out.mu.Lock()
	defer i.out.mu.Unlock()
	st := i.out.itemByRef[i.id]
	if st == nil {
		return ItemSnapshot{ID: i.id, State: Pending}
	}
	return st.snapshot()
}

func (i *ItemHandle) resolve(state EntityState, problems []Problem) {
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
		// storeProblems: shared CSI-safe path (identical to Task).
		st.problems = storeProblems(problems)
	}
	i.out.bumpLocked()
	i.out.appendEventLocked(Event{Type: "item." + string(state), EntityID: i.id})
	// §17.5: terminal outcomes render immediately — durable evidence, not Finish buffer.
	i.out.emitItemProgressiveLocked(st)
}

func (i *ItemHandle) resolveBy(state EntityState, problems []Problem) {
	if len(problems) == 0 {
		i.out.mu.Lock()
		defer i.out.mu.Unlock()
		i.out.recordMisuse(ErrNoProblems)
		return
	}
	i.resolve(state, problems)
}
