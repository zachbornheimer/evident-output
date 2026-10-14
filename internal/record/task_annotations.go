package record

import "slices"

// Problems is a copy of the Task's blocking Problems, nil when it has none.
func (t *Task) Problems() []Problem {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneProblems(t.problems)
}

// ProblemCount is how many blocking Problems the Task holds.
func (t *Task) ProblemCount() int {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return len(t.problems)
}

// AppendProblems records blocking Problems, sanitized and copied.
func (t *Task) AppendProblems(problems ...Problem) {
	defer t.changed()
	t.lockForWrite()
	defer t.run.mu.Unlock()
	t.problems = append(t.problems, StoreProblems(problems)...)
}

// ResolveProblems folds extra into the Task's Problems as it settles into
// state. A Failed or Blocked row's Problems that carry no detail of their
// own gain evidenceTail, the capture tail the Task already gathered, so the
// evidence a caller collected needs no opt-in.
func (t *Task) ResolveProblems(state State, extra []Problem, evidenceTail string) {
	defer t.changed()
	t.lockForWrite()
	defer t.run.mu.Unlock()
	if len(extra) == 0 && len(t.problems) == 0 {
		return
	}
	all := slices.Concat(t.problems, extra)
	if takesEvidenceTail(state) {
		for i := range all {
			if lacksDetail(all[i]) {
				all[i].Detail = evidenceTail
			}
		}
	}
	t.problems = StoreProblems(all)
}

// NeedsEvidenceTail reports whether settling into state with extra would give
// some Problem the capture tail, so the caller reads the capture only then.
func (t *Task) NeedsEvidenceTail(state State, extra []Problem) bool {
	if !takesEvidenceTail(state) {
		return false
	}
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return slices.ContainsFunc(t.problems, lacksDetail) || slices.ContainsFunc(extra, lacksDetail)
}

func takesEvidenceTail(state State) bool { return state == Failed || state == Refused }

func lacksDetail(p Problem) bool { return p.Detail == "" && p.EvidenceTail == "" }

// Warnings is a copy of the Task's warning-severity Problems.
func (t *Task) Warnings() []Problem {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneProblems(t.warnings)
}

// WarningCount is how many warnings the Task holds.
func (t *Task) WarningCount() int {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return len(t.warnings)
}

// AppendWarning records a warning and returns how many the Task now holds.
// Warnings annotate the Task's lifecycle; they never replace it.
func (t *Task) AppendWarning(p Problem) int {
	defer t.changed()
	t.lockForWrite()
	defer t.run.mu.Unlock()
	t.warnings = append(t.warnings, p)
	return len(t.warnings)
}

// Facts is a copy of the Task's discovered name/value annotations.
func (t *Task) Facts() []Fact {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return CloneFacts(t.facts)
}

// AppendFact records a discovered name/value annotation.
func (t *Task) AppendFact(f Fact) {
	defer t.changed()
	t.lockForWrite()
	defer t.run.mu.Unlock()
	t.facts = append(t.facts, f)
}

// Actions is a copy of the next steps the Task attached.
func (t *Task) Actions() []Action {
	t.run.mu.Lock()
	defer t.run.mu.Unlock()
	return slices.Clone(t.actions)
}

// AppendAction records a next step for the Task.
func (t *Task) AppendAction(a Action) {
	defer t.changed()
	t.lockForWrite()
	defer t.run.mu.Unlock()
	t.actions = append(t.actions, a)
}
