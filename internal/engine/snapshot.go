package engine

import "github.com/zachbornheimer/evident-output/internal/core"

// Snapshot returns an immutable copy of current state.
func (o *Output) Snapshot() Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snapshotLocked()
}

func (o *Output) snapshotLocked() Snapshot {
	s := Snapshot{
		Version:       o.version,
		OutputID:      o.outputID,
		Subject:       o.cfg.subject,
		Lines:         append([]string(nil), o.lines...),
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
	for _, ch := range o.changes {
		s.Changes = append(s.Changes, ch.changesSnapshot())
	}
	for _, p := range o.plans {
		s.Plans = append(s.Plans, p.planSnapshot())
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
	colID := ""
	if t.collection != nil {
		colID = t.collection.id
	}
	base := TaskSnapshot{
		ID:           t.id,
		Key:          t.key,
		Name:         t.name,
		State:        t.state,
		Phase:        t.phase,
		ActivityAt:   t.activityAt,
		Progress:     t.progress,
		Summary:      t.summary,
		Problems:     core.CloneProblems(t.problems),
		Warnings:     core.CloneProblems(t.warnings),
		Facts:        core.CloneFacts(t.facts),
		Verification: core.CloneVerificationDetails(t.verification),
		Actions:      cloneActions(t.actions),
		Skipped:      cloneTaxonomy(t.skipped),
		Kept:         cloneTaxonomy(t.kept),
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
		v.add(t.state)
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
		if t.state != NotStarted {
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

func (g *tasksState) displaySummary() string {
	// Success summary only when all children done/skipped successfully.
	st := g.derivedState()
	if st == Done && g.summary != "" && !g.hasWarnedOrFailedDescendant() {
		return g.summary
	}
	return ""
}

// hasWarnedOrFailedDescendant reports whether g or any nested child
// container carries a Failed/Cancelled task or a task with a Warn annotation
// (E2.5 finding 1): the group's own success summary must not paper over a
// warning or failure living several containers deep — the same suppression
// this method already gave Failed/Cancelled, restored and extended to
// Warnings.
func (g *tasksState) hasWarnedOrFailedDescendant() bool {
	for _, t := range g.tasks {
		if t.state == Failed || t.state == Cancelled || len(t.warnings) > 0 {
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
	ts := TasksSnapshot{
		ID:          g.id,
		Key:         g.key,
		Name:        g.name,
		State:       g.derivedState(),
		Summary:     g.displaySummary(),
		Declaration: g.declaration,
		Sequential:  g.sequential,
	}
	for _, t := range g.tasks {
		ts.Tasks = append(ts.Tasks, t.snapshot())
	}
	for _, child := range g.children {
		ts.Collections = append(ts.Collections, child.snapshot())
	}
	return ts
}
