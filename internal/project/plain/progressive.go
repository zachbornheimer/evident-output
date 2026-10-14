package plain

import (
	"slices"
	"strings"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Trigger names the evidence call streaming a Running Task's line, so each
// kind thins on its own: a phase change always streams, a progress tick
// once per milestone.
type Trigger int

const (
	TriggerPhase Trigger = iota
	TriggerProgress
)

// progressMilestones is how many roughly even steps a determinate total is
// cut into for plain streaming (beginner-8): a line per increment would
// flood a CI log, one line and then silence until Done would say nothing.
const progressMilestones = 10

// mark is what plain streaming already wrote for a Running Task.
type mark struct {
	// phase is the last phase streamed, so a repeated phase does not
	// stream again.
	phase string
	// progressStarted is false until the first progress tick streamed;
	// progressEmitted is the last completed value streamed.
	progressStarted bool
	progressEmitted int64
}

// crossesMilestone reports whether completed of total crosses a milestone
// since the last streamed tick. The first and the final tick always do
// (beginner-8's "always a final n/n").
func (m mark) crossesMilestone(completed, total int64) bool {
	if !m.progressStarted {
		return true
	}
	if completed == m.progressEmitted {
		return false
	}
	if total <= 0 || completed >= total {
		return true
	}
	step := max(total/progressMilestones, 1)
	return completed/step != m.progressEmitted/step
}

// streamName qualifies a streamed row with the subject it belongs to. The
// durable transcript indents a collection's children under their header,
// but a streamed milestone or heartbeat arrives on its own: three subjects
// each narrating a child called "classify" would print three "◐ classify"
// lines that name nothing. A Task nested deeper names every container
// outermost first ("outer › inner  long"), so same-named Tasks in different
// branches stay apart.
func (r *row) streamName() string {
	if !r.InGroup {
		return r.Name
	}
	path := r.containerNames()
	if r.Group == r.Name {
		path = path[:len(path)-1]
	}
	if len(path) == 0 {
		return r.Name
	}
	return strings.Join(path, record.QualifiedSubjectSeparator) + "  " + r.Name
}

// containerNames is the names of the collections enclosing r, outermost
// first. A row whose block was never declared knows only its own Group.
func (r *row) containerNames() []string {
	if r.block == nil {
		return []string{r.Group}
	}
	var path []string
	for b := r.block; b != nil; b = b.parent {
		path = append(path, b.name)
	}
	slices.Reverse(path)
	return path
}

// Running streams the Task id's current phase or progress as a durable
// line when no live region shows it (evo-rec.md Problem 10): a phase every
// time its text changes, progress at each milestone. A Task inside a
// collection streams too: "never silent between the first line and Done"
// does not depend on where the Running Task sits in the tree.
func (s *Stream) Running(id record.TaskID, trigger Trigger, now time.Time) {
	r := s.rows[id]
	if r == nil || s.cfg.Region.Interactive() {
		return
	}
	truth := r.Rec.Truth()
	if truth.State != record.Running {
		return
	}
	switch trigger {
	case TriggerPhase:
		if truth.Phase == r.mark.phase {
			return
		}
		r.mark.phase = truth.Phase
	case TriggerProgress:
		if !r.mark.crossesMilestone(truth.Progress.Completed, truth.Progress.Total) {
			return
		}
		r.mark.progressStarted = true
		r.mark.progressEmitted = truth.Progress.Completed
	}
	snap := s.cfg.Source.Snapshot(id)
	snap.Name = r.streamName()
	var b strings.Builder
	project.WriteTask(&b, snap, s.cfg.Style)
	if b.Len() == 0 {
		return
	}
	s.Write(b.String())
	// This line proved the Task alive; the heartbeat covers only the
	// silence after it.
	r.milestone(now)
}

// HeldBack reports whether a resolved root Task is a zero-information row
// (project.IsProvenNoOpTask) with no ledger section of its own. Such a row
// does not stream when it resolves: Finish decides whether the run has
// anything else to show, and prints the row only if not.
func (s *Stream) HeldBack(t record.TaskSnapshot) bool {
	if !project.IsProvenNoOpTask(project.TaskAtVerbosity(t, s.cfg.Style.Verbose)) {
		return false
	}
	return !s.cfg.Run.HasLedgerSection(t.ID)
}

// Commit writes a resolved root Task's row to scrollback the instant it
// resolves, interactive or not, and drops it from the live region. A Task
// inside a collection writes with its collection instead (commitMember); a
// collection that settled earlier writes before this row does, and while one
// is still open the row waits for it to close.
// Progressive rows and Print lines interleave by resolution and call time:
// a Task that resolved before a later Println or Confirm prompt renders
// above it (release-gate round 5 finding 3). While a window is open a
// sibling's row is held until it closes; the row of the Task that owns the
// window is not, since the window exists to show it. A Task that settles
// again in a different state is written again, so scrollback tells both.
func (s *Stream) Commit(id record.TaskID) {
	r := s.rows[id]
	if r == nil {
		return
	}
	if r.InGroup {
		s.commitMember(r)
		return
	}
	if !record.IsTerminalTask(r.Rec.State()) || (r.emitted && r.emittedAs == r.Rec.State()) {
		return
	}
	s.CommitSettledBlocks()
	if !r.ownsWindow && s.mustWait(r) {
		if !r.held {
			r.held = true
			s.held = append(s.held, r)
		}
		return
	}
	s.writeRoot(r)
}

// writeRoot writes the resolved root row r now, unless it is a row Finish
// owns (HeldBack) or its state is already in scrollback.
func (s *Stream) writeRoot(r *row) {
	state := r.Rec.State()
	if r.emitted && r.emittedAs == state {
		return
	}
	snap := s.cfg.Source.Snapshot(r.ID)
	if s.HeldBack(snap) {
		return
	}
	var b strings.Builder
	project.WriteTaskAligned(&b, snap, s.column.nameWidth(), s.cfg.Style)
	r.emitted, r.emittedAs = true, state
	if b.Len() == 0 {
		return
	}
	if r.ownsWindow {
		s.WriteOwner(b.String())
	} else {
		s.Write(b.String())
	}
	s.writeEffects(string(r.ID))
	s.cfg.Region.Refresh()
}

// CommitEffects streams owner's planned and changed ledger sections right
// after the owner's row, when a section holds a named (File or Exec) row: a
// dry-run user sees each Task's items as its work finishes, not all of them
// at Finish. A quantity-only section (Effect) still waits for Finish. The
// section is marked streamed so Finish never renders it twice. A root row
// still held back (it ran After an open collection) has not reached
// scrollback, so its sections wait for it: writeRoot and Finish write them
// the moment the row is written.
func (s *Stream) CommitEffects(owner string) {
	if r := s.rows[record.TaskID(owner)]; r != nil && !r.InGroup && !r.emitted {
		return
	}
	s.writeEffects(owner)
}

// writeEffects writes owner's unstreamed named ledger sections now.
func (s *Stream) writeEffects(owner string) {
	if text := s.takeEffects(owner); text != "" {
		s.Write(text)
	}
}

// takeEffects is owner's unstreamed named ledger sections as ledger text,
// marked streamed.
func (s *Stream) takeEffects(owner string) string {
	var b strings.Builder
	for _, tense := range []record.LedgerTense{record.TensePlanned, record.TenseChanged} {
		sec, ok := s.cfg.Run.Section(owner, tense)
		if !ok || s.streamed[sec.ID] || !hasNamedRecord(sec.Records) {
			continue
		}
		project.WriteEffects(&b, project.EffectSection{
			Kind: sec.Tense.String(), Subject: sec.Subject, Records: sec.Records,
			IntendedVerb: sec.IntendedVerb, NameWidth: s.foldedSubjectWidth(tense, sec.ID), Width: s.width(),
		}, s.cfg.Style)
		s.streamed[sec.ID] = true
	}
	return b.String()
}

// foldedSubjectWidth is the subject column the ledger will have once folded,
// counting the section being streamed as the row it is about to be. Finish
// aligns the sections it writes to the same column. The fold asks nothing of
// a container's membership here: the run is still declaring them.
func (s *Stream) foldedSubjectWidth(tense record.LedgerTense, streaming string) int {
	return project.FoldedSubjectWidth(tense.String(), s.ledgerSources(tense, streaming), nil)
}

// hasNamedRecord reports whether records holds a row without a quantity
// (evo.File or evo.Exec): "quantity records tally; named records
// enumerate".
func hasNamedRecord(records []record.EffectRecord) bool {
	for _, r := range records {
		if !r.HasQty {
			return true
		}
	}
	return false
}

// width is the terminal width the ledger lays out for.
func (s *Stream) width() int {
	if w := s.cfg.Scene.Width(); w > 0 {
		return w
	}
	return project.DefaultWidth
}
