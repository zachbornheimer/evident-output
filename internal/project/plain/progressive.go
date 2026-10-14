package plain

import (
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
// but a streamed milestone arrives on its own: three subjects each
// narrating a child called "classify" would print three "◐ classify" lines
// that name nothing.
func (r *row) streamName() string {
	if !r.InGroup || r.Group == r.Name {
		return r.Name
	}
	return r.Group + "  " + r.Name
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
// resolves, interactive or not, and drops it from the live region.
// Progressive rows and Print lines interleave by resolution and call time:
// a Task that resolved before a later Println or Confirm prompt renders
// above it (release-gate round 5 finding 3). While a window is open a
// sibling's row is held until it closes; the row of the Task that owns the
// window is not, since the window exists to show it. A Task that settles
// again in a different state is written again, so scrollback tells both.
func (s *Stream) Commit(id record.TaskID) {
	r := s.rows[id]
	if r == nil || !record.IsTerminalTask(r.Rec.State()) || (r.emitted && r.emittedAs == r.Rec.State()) {
		return
	}
	snap := s.cfg.Source.Snapshot(id)
	if s.HeldBack(snap) {
		return
	}
	var b strings.Builder
	project.WriteTaskAligned(&b, snap, s.column.nameWidth(), s.cfg.Style)
	r.emitted, r.emittedAs = true, r.Rec.State()
	if b.Len() == 0 {
		return
	}
	if r.ownsWindow {
		s.WriteOwner(b.String())
	} else {
		s.Write(b.String())
	}
	s.cfg.Region.Refresh()
}

// CommitEffects streams owner's planned and changed ledger sections the
// instant its Task resolves, when a section holds a named (File or Exec)
// row: a dry-run user sees each Task's items as its work finishes, not all
// of them at Finish. A quantity-only section (Effect) still waits for
// Finish. The section is marked streamed so Finish never renders it twice.
func (s *Stream) CommitEffects(owner string) {
	for _, tense := range []record.LedgerTense{record.TensePlanned, record.TenseChanged} {
		sec, ok := s.cfg.Run.Section(owner, tense)
		if !ok || s.streamed[sec.ID] || !hasNamedRecord(sec.Records) {
			continue
		}
		var b strings.Builder
		project.WriteEffects(&b, project.EffectSection{
			Kind: sec.Tense.String(), Subject: sec.Subject, Records: sec.Records,
			IntendedVerb: sec.IntendedVerb, NameWidth: s.cfg.Run.MaxSubjectWidth(tense), Width: s.width(),
		}, s.cfg.Style)
		s.Write(b.String())
		s.streamed[sec.ID] = true
	}
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
