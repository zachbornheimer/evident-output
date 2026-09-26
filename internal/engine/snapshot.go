package engine

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/engine/ledger"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// Snapshot returns an immutable copy of current state.
func (o *Output) Snapshot() Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snapshotLocked()
}

// projectMessageLinesLocked copies lines into the public Snapshot.Lines
// projection, dropping any index commitResolvedTaskLocked marked as a held
// Task row rather than a real Println/Printf message. Those entries stay in
// o.lines itself (unfiltered) so the internal Finish-time index math in
// residualCompositionLocked, which is keyed off o.lines, is unaffected —
// only the externally observable message log excludes them, so a caller
// doing render.RenderPlain(out.Snapshot()) never sees a Task's row twice:
// once as this projection's text, once as its own Task/Collection entity.
func projectMessageLinesLocked(lines []string, deferredTaskRows map[int]struct{}) []string {
	if len(deferredTaskRows) == 0 {
		return append([]string(nil), lines...)
	}
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if _, isTaskRow := deferredTaskRows[i]; isTaskRow {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (o *Output) snapshotLocked() Snapshot {
	s := Snapshot{
		Version:  o.version,
		OutputID: o.outputID,
		Subject:  o.cfg.subject,
		// Lines is the message-log projection: a standalone Task's row
		// held in o.lines only to preserve call-order (commitResolvedTaskLocked)
		// is excluded here so it never renders twice — once from this
		// projection's Finish-time tail, once as its own Task entity.
		Lines:         projectMessageLinesLocked(o.lines, o.deferredTaskRowLines),
		Actions:       cloneActions(o.collectActionsLocked()),
		Timestamp:     o.cfg.clock.Now(),
		DryRun:        o.cfg.dryRun,
		Preview:       o.cfg.preview,
		DryRunSubject: o.cfg.dryRunHeaderText,
		Warnings:      core.CloneProblems(o.runWarnings),
		Facts:         core.CloneFacts(o.runFacts),
	}
	for _, col := range o.collections {
		s.Collections = append(s.Collections, col.snapshot())
	}
	// Root tasks not in a collection
	for _, t := range o.tasks {
		if t.collection == nil {
			s.Tasks = append(s.Tasks, t.snapshot())
		}
	}
	for _, ch := range o.book.Sections(ledger.Changed) {
		s.Changes = append(s.Changes, ch.ChangesSnapshot())
	}
	for _, p := range o.book.Sections(ledger.Planned) {
		s.Plans = append(s.Plans, p.PlanSnapshot())
	}
	for _, m := range o.messages {
		s.Messages = append(s.Messages, MessageSnapshot{
			ID:         m.id,
			Text:       m.text,
			Visibility: m.visibility,
		})
	}
	if o.conclusion != nil {
		c := *o.conclusion
		s.Conclusion = &c
	}
	return s
}

func (t *taskState) snapshot() TaskSnapshot {
	s := t.view()
	s.Problems = core.CloneProblems(s.Problems)
	s.Warnings = core.CloneProblems(s.Warnings)
	s.Facts = core.CloneFacts(s.Facts)
	s.Verification = core.CloneVerificationDetails(s.Verification)
	s.Actions = cloneActions(s.Actions)
	s.Skipped = cloneTaxonomy(s.Skipped)
	s.Kept = cloneTaxonomy(s.Kept)
	return s
}

// view is t's snapshot sharing t's slices: read it under o.mu and never
// let it escape, since the next mutation of t can change what it shows.
// It costs no allocation, so a live frame can classify every Task by its
// view and snapshot only the ones it shows.
func (t *taskState) view() TaskSnapshot {
	colID := ""
	if t.collection != nil {
		colID = t.collection.id
	}
	base := TaskSnapshot{
		ID:           t.id,
		Key:          t.key,
		Name:         t.name,
		State:        t.state.Current(),
		Phase:        t.phase,
		ActivityAt:   t.activityAt,
		Progress:     t.progress,
		Summary:      t.summary,
		Problems:     t.problems,
		Warnings:     t.warnings,
		Facts:        t.facts,
		Verification: t.verification,
		Actions:      t.actions,
		Skipped:      t.skipped,
		Kept:         t.kept,
		Collection:   colID,
		Declaration:  t.declaration,
		Resolution:   t.resolution,
		Evidence:     t.verifyEvidence,
	}
	return core.NewTaskSnapshot(base, t.liveFirstSeenAt, t.synthetic)
}

func cloneTaxonomy(in []TaxonomyRecord) []TaxonomyRecord {
	if len(in) == 0 {
		return nil
	}
	out := make([]TaxonomyRecord, len(in))
	copy(out, in)
	for i := range out {
		if len(out[i].Causes) > 0 {
			out[i].Causes = append([]string(nil), out[i].Causes...)
		}
	}
	return out
}

// derivedState folds both this container's own tasks and its nested
// children (P3's recursive nesting) into one verdict: a nested Sequence or
// Group contributes exactly like one more task would, so a failure
// three levels deep still surfaces at the root header.
func (g *tasksState) derivedState() EntityState {
	if len(g.tasks) == 0 && len(g.children) == 0 {
		return Empty
	}
	// A NotStarted child normally borrows its group's verdict from the
	// sibling that failed first, so it contributes nothing of its own. When
	// every child is NotStarted there is no such sibling — whatever stopped
	// the run was another subject entirely — and folding to Done rendered a
	// check over a subject that never ran.
	if len(g.children) == 0 && g.allTasksNotStarted() {
		return NotStarted
	}
	var v verdictFold
	for _, t := range g.tasks {
		v.add(t.state.Current())
	}
	for _, child := range g.children {
		switch s := child.derivedState(); s {
		case Empty:
		case NotStarted:
			v.unresolved = true
		default:
			v.add(s)
		}
	}
	return v.state()
}

func (g *tasksState) allTasksNotStarted() bool {
	for _, t := range g.tasks {
		if t.state.Current() != NotStarted {
			return false
		}
	}
	return len(g.tasks) > 0
}

// verdictFold accumulates member states into one container verdict. A
// Blocked member counts like a Failed one does for the Tasks After the
// container (see stateOutcome): the container finished and did not
// succeed, so its header never reads Incomplete for it.
type verdictFold struct {
	running, failed, blocked, cancelled, unresolved bool
}

func (v *verdictFold) add(s EntityState) {
	switch s {
	case Running:
		v.running = true
	case Failed:
		v.failed = true
	case Blocked:
		v.blocked = true
	case Cancelled:
		v.cancelled = true
	case Done, Skipped, NotStarted:
	default:
		v.unresolved = true
	}
}

func (v verdictFold) state() EntityState {
	switch {
	case v.running:
		return Running
	case v.failed:
		return Failed
	case v.blocked:
		return Blocked
	case v.cancelled:
		return Cancelled
	case v.unresolved:
		return Incomplete
	default:
		return Done
	}
}

// displaySummary is g's Summary as its row shows it, given its derived
// state st: only when all children done/skipped successfully.
func (g *tasksState) displaySummary(st EntityState) string {
	if st == Done && g.summary != "" && !g.hasWarnedOrFailedDescendant() {
		return g.summary
	}
	return ""
}

// hasWarnedOrFailedDescendant reports whether g or any nested child
// container carries a Failed/Cancelled task or a task with a
// warning-severity Problem (E2.5 finding 1): the group's own success
// summary must not paper over a warning or failure living several
// containers deep — the same suppression this method already gave
// Failed/Cancelled, restored and extended to Warnings.
func (g *tasksState) hasWarnedOrFailedDescendant() bool {
	for _, t := range g.tasks {
		if t.state.Current() == Failed || t.state.Current() == Cancelled || len(t.warnings) > 0 {
			return true
		}
	}
	for _, child := range g.children {
		if child.hasWarnedOrFailedDescendant() {
			return true
		}
	}
	return false
}

func (o *Output) collectActionsLocked() []Action {
	seen := map[string]struct{}{}
	var out []Action
	add := func(list []Action) {
		for _, a := range list {
			k := actionKey(a)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, a)
		}
	}
	add(o.actions)
	for _, t := range o.tasks {
		add(t.actions)
		// ZYS-848: a remedy attached via evo.Next(...) to an individual
		// Problem/warning (task.Problem(msg, evo.Next(...)),
		// task.Fail(msg, evo.Next(...))) must reach the run's own Next
		// steps the same way a task-level Next(...) call already does —
		// otherwise a remedy on one of several accumulated Problems is
		// invisible everywhere: writeProblem never renders p.Actions
		// inline (it is evidence, not a decision), and without this loop
		// it was silently dropped from the Conclusion's Next list too.
		for _, p := range t.problems {
			add(p.Actions)
		}
		for _, w := range t.warnings {
			add(w.Actions)
		}
	}
	return out
}

func (g *tasksState) snapshot() TasksSnapshot {
	ts := g.header()
	for _, t := range g.tasks {
		ts.Tasks = append(ts.Tasks, t.snapshot())
	}
	for _, child := range g.children {
		ts.Collections = append(ts.Collections, child.snapshot())
	}
	return ts
}

// header is g's snapshot without its children.
func (g *tasksState) header() TasksSnapshot {
	state := g.derivedState()
	return TasksSnapshot{
		ID:          g.id,
		Key:         g.key,
		Name:        g.name,
		State:       state,
		Summary:     g.displaySummary(state),
		Declaration: g.declaration,
		Sequential:  g.sequential,
	}
}

// liveSnapshot is g as a live frame of rows rows can show it: the
// children it could select, snapshotted, and a tally of the rest (see
// render.LiveChildren).
func (g *tasksState) liveSnapshot(rows int, now time.Time) TasksSnapshot {
	ts := g.header()
	children := render.NewLiveChildren(g.name, rows)
	for _, t := range g.tasks {
		t.stampLiveFirstSeen(now)
		if view := t.view(); children.Admit(&view) {
			children.Keep(t.snapshot())
		}
	}
	ts = liveCollections(g.children, rows, now).Into(ts)
	return children.Collection(ts)
}

// liveCollections projects cols for a live frame of rows rows: the ones
// the frame can reach through liveSnapshot, the rest tallied from views
// (see render.LiveCollections).
func liveCollections(cols []*tasksState, rows int, now time.Time) *render.LiveCollections {
	projected := render.NewLiveCollections(rows)
	for _, col := range cols {
		if projected.Admit(col.census.rank()) {
			projected.Keep(col.liveSnapshot(rows, now))
			continue
		}
		col.stampLiveFirstSeen(now)
		projected.Omit(col.census.counts(), col.liveOwnRow())
	}
	return projected
}

// view is g's header and its Tasks' views, recursively, without the cost
// of a snapshot.
func (g *tasksState) view() TasksSnapshot {
	ts := g.header()
	for _, t := range g.tasks {
		ts.Tasks = append(ts.Tasks, t.view())
	}
	for _, child := range g.children {
		ts.Collections = append(ts.Collections, child.view())
	}
	return ts
}

// stampLiveFirstSeen stamps every Task at or below g, as a frame that
// counted them does (see taskState.stampLiveFirstSeen).
func (g *tasksState) stampLiveFirstSeen(now time.Time) {
	if g.census.unstamped == 0 {
		return
	}
	for _, t := range g.tasks {
		t.stampLiveFirstSeen(now)
	}
	for _, child := range g.children {
		child.stampLiveFirstSeen(now)
	}
}
