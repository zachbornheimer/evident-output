package engine

import (
	"io"
	"strings"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// Progressive emission implements the spirit of §1 (live becomes durable) and
// §17.5 (render immediately for terminal outcomes). Resolved items are not
// held until Finish — they stream as evidence the moment they settle.

type flusher interface {
	Flush() error
}

// emitLineProgressiveLocked streams a newly appended Line() to the human stream.
func (o *Output) emitLineProgressiveLocked() {
	if o.linesEmitted >= len(o.lines) {
		return
	}
	var b strings.Builder
	for _, line := range o.lines[o.linesEmitted:] {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	o.linesEmitted = len(o.lines)
	o.writeDurableTextLocked(b.String())
}

// writeDurableTextLocked emits durable human text immediately and flushes.
// Interactive: above the live region on the terminal driver.
// Plain/non-interactive: primary (+ AlsoWrite) writers.
func (o *Output) writeDurableTextLocked(text string) {
	if text == "" {
		return
	}
	o.durableRowsEmitted++
	if o.cfg.projection.suppressesHuman() {
		return
	}
	live := o.liveLocked()
	// A live region (including the armed, entity-less title line painted by
	// arm()) may still be on screen even after the surface stops reporting
	// itself interactive — terminal.ANSI disables IsInteractive() permanently
	// on a short write, but the fd keeps accepting writes and the stale frame
	// stays visible. Clear it on the surface's own bookkeeping (o.live.liveActive),
	// not on a fresh interactive re-check, so durable text is never appended
	// straight onto whatever line the live region last drew.
	if live != nil && o.live != nil && o.live.liveActive {
		live.ClearLive()
		o.live.liveActive = false
		o.live.lastLiveText = ""
	}
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		if o.live == nil {
			o.live = &liveEngine{surface: live}
		}
		live.WriteDurable(text)
		// Redraw immediately (still holding o.mu) so the live region never sits
		// blank or stale between this durable write and whatever caller or
		// background tick next touches the terminal — the clear/write/redraw
		// cycle is one atomic sequence under one lock (evo-rec.md #12).
		if o.live.visible && o.hasLiveActivityLocked() {
			o.renderLiveLocked(true)
		}
		return
	}
	// Plain / non-interactive: stream like fmt — write now, flush now.
	writers := make([]io.Writer, 0, 1+len(o.cfg.extraWriters))
	if o.cfg.primary != nil {
		writers = append(writers, o.cfg.primary)
	}
	writers = append(writers, o.cfg.extraWriters...)
	for _, w := range writers {
		if w == nil {
			continue
		}
		_, _ = io.WriteString(w, text)
		if f, ok := w.(flusher); ok {
			_ = f.Flush()
		}
	}
}

// The root name column is progressive emission's sibling-column-alignment
// width (fixture-repo-retire-dryrun.md): every root (non-collection) task
// the caller has declared so far, whether or not it has resolved yet. A
// caller that declares its whole known set of sibling tasks before
// resolving any of them (e.g. branches/worktrees/remote-tracking declared
// upfront, then worked through one at a time) gets the same aligned column
// a batch render would produce — declaring one, fully resolving it, then
// declaring the next narrows the column to whatever was known at each
// commit, which is the honest limit of "render immediately" (§17.5)
// progressive streaming: a name declared after this row already committed
// cannot retroactively widen it.
//
// rootColumn keeps that width current as root Tasks are declared (see
// appendTaskLocked), so a commit reads it without rescanning every Task.
type rootColumn struct{ count, width int }

func (c *rootColumn) add(name string) {
	c.count++
	c.width = max(c.width, utf8.RuneCountInString(name))
}

// nameWidth is the column width, or 0 when fewer than two root Tasks exist
// and there is nothing to align.
func (c rootColumn) nameWidth() int {
	if c.count < 2 {
		return 0
	}
	return c.width
}

// maxChangeSubjectWidth/maxPlanSubjectWidth are residualCompositionLocked's
// batch-time equivalent of rootColumn for the Changes/Plan ledger
// (fixture-repo-retire-dryrun.md's aligned "[planned] <name>  <verb> ..."
// column) — Changes/Plans render only at Finish (never progressively, see
// residualCompositionLocked's doc comment), so the full set is always known
// here, unlike task rows above.
// commitResolvedTaskLocked commits a resolved standalone Task's row to
// durable scrollback the instant it resolves — interactive or not — and
// drops it from the live ticker (liveSnapshotLocked already filters
// coreEmitted tasks). Collection children stay with the collection renderer
// (H.20/H.21 own their ledger via signalLiveLocked instead).
//
// Chronology contract (progressive.go's residualPlainLocked doc comment):
// progressive Task rows and Print lines interleave by resolution/call time —
// a Task that resolves before a later Println/Confirm prompt must render
// above it in scrollback. Committing at resolution time, not at Finish
// (former H.17 behavior), is what makes that true in interactive mode too;
// release-gate round 5 finding 3 is exactly a Confirm prompt or Println
// rendering above a task that had already resolved. This was previously
// Confirm-gate-only (flushGateNowLocked) because a Confirm's answer was the
// one case that couldn't wait for Finish; every standalone Task now gets the
// same immediate commit for the same reason — a later evidence call must
// never race above already-resolved work.
func (o *Output) commitResolvedTaskLocked(id string) {
	st := o.taskByRef[id]
	if st == nil || st.coreEmitted || !core.IsTerminalTask(st.state) {
		return
	}
	if o.heldBackAsNoOpLocked(st.snapshot()) {
		return
	}
	var b strings.Builder
	nameWidth := o.rootColumn.nameWidth()
	render.WriteTaskAligned(&b, st.snapshot(), nameWidth, o.humanStyle())
	st.coreEmitted = true
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() || o.cfg.plain {
		return
	}
	if o.hasLiveActivityLocked() {
		o.live.visible = true
		o.renderLiveLocked(true)
	} else if o.live != nil && o.live.liveActive {
		live.ClearLive()
		o.live.liveActive = false
		o.live.visible = false
		o.stopSpinnerAnimatorLocked()
	}
}

// hasNamedEffectRecord reports whether records holds at least one no-qty
// (evo.File/evo.Exec named) row — the "named record enumerates" half of
// "Quantity records tally; named records enumerate": Effect rows always
// carry a quantity (HasQty true) and stay Finish-only, tallied and bounded
// there exactly as before.
func hasNamedEffectRecord(records []core.EffectRecord) bool {
	for _, r := range records {
		if !r.HasQty {
			return true
		}
	}
	return false
}

// commitNamedEffectsLocked streams owner's Plan/Changes ledger section the
// instant its owning standalone task resolves (task.go's finish), provided
// the section holds at least one named (File/Exec) record — evo-rec.md's
// "a --dry user loses 'what would run' per item" fix: a caller working
// through several tasks in sequence sees each task's planned/changed items
// the moment that task's own work finishes, instead of every task's rows
// piling up at the very end of the whole run's Finish. A pure-quantity
// section (Effect) is untouched — it always
// waits for Finish, exactly as before (see hasNamedEffectRecord).
//
// This calls the same render.WriteEffects Finish already uses (merge,
// bounded-rows cap, "+N more" overflow) so a task that records many named
// items still collapses identical (verb, object) pairs and bounds distinct
// ones — the model in o.plans/o.changes is the only place records
// accumulate; only the presentation instant moves earlier. Marking the
// section namedRowsEmitted is what makes residualCompositionLocked's Finish
// loop skip it — the raw item list must never render twice.
func (o *Output) commitNamedEffectsLocked(owner string) {
	for _, tense := range []ledgerTense{tensePlanned, tenseChanged} {
		s, ok := o.ledger.byOwner[ledgerSectionKey{owner: owner, tense: tense}]
		if !ok || s.namedRowsEmitted || !hasNamedEffectRecord(s.records) {
			continue
		}
		var b strings.Builder
		render.WriteEffects(&b, o.effectSectionLocked(s, maxSubjectWidth(*o.sectionsLocked(tense))), o.humanStyle())
		o.writeDurableTextLocked(b.String())
		s.namedRowsEmitted = true
	}
}

// effectSectionLocked is s laid out for render.WriteEffects — the one
// shape both the streamed and the Finish ledger render.
func (o *Output) effectSectionLocked(s *ledgerSection, nameWidth int) render.EffectSection {
	width := o.cfg.width
	if width <= 0 {
		width = defaultWidth
	}
	return render.EffectSection{
		Kind: s.tense.String(), Subject: s.subject, Records: s.records,
		IntendedVerb: s.intendedVerb, NameWidth: nameWidth, Width: width,
	}
}

// humanStyle is how this Output paints human rows.
func (o *Output) humanStyle() render.Style {
	return render.Style{Color: !o.cfg.noColor, Verbose: o.cfg.verbosity >= VerbosityVerbose, Profile: o.cfg.glyphs}
}

// taskProgressiveTrigger names which evidence call is streaming a Running
// task's plain-mode line, so emitTaskRunningProgressiveLocked can rate-limit
// each kind independently (a phase change always streams; a progress tick
// streams once per milestone; a counted Task's item streams only on a
// milestone's line).
type taskProgressiveTrigger int

const (
	triggerPhase taskProgressiveTrigger = iota
	triggerProgress
	// triggerItem is Doing on a Task that reports a count: the current item
	// of Progress(i, total).Doing(item).
	triggerItem
)

// plainProgressMilestones is how many roughly-even steps a determinate
// total is divided into for plain-mode progress streaming (beginner-8): a
// durable line per increment would flood CI logs for a large total, so
// increments are thinned to this many milestones instead of streaming only
// once ("progress established") and then going silent until Done.
const plainProgressMilestones = 10

// shouldEmitPlainProgressLocked reports whether st's current progress value
// crosses a new milestone since the last plain-mode line streamed for it.
// The first tick and the final tick (completed == total) always stream —
// beginner-8's "always a final n/n" — everything between is thinned.
func shouldEmitPlainProgressLocked(st *taskState) bool {
	completed := st.progress.Completed
	total := st.progress.Total
	if !st.plainStream.progressStarted {
		return true
	}
	if completed == st.plainStream.progressEmitted {
		return false
	}
	if total <= 0 || completed >= total {
		return true
	}
	step := max(total/plainProgressMilestones, 1)
	return completed/step != st.plainStream.progressEmitted/step
}

// progressiveRowName qualifies a streamed plain row with the subject it
// belongs to. The durable transcript indents a collection's children under
// their header, but a streamed milestone arrives on its own, far from any
// header — three sibling subjects each narrating a child called `classify`
// produce three `◐ classify  24/111` lines that name nothing. The live
// region answers the same question the same way; see
// promotesLoneChildOntoHeader.
func progressiveRowName(st *taskState) string {
	if st.collection == nil || st.collection.name == st.name {
		return st.name
	}
	return st.collection.name + "  " + st.name
}

// emitTaskRunningProgressiveLocked streams a Running task's current
// phase/progress as a durable line in plain/non-interactive mode
// (evo-rec.md Problem 10: "Phase as static text once, then terminal rows").
// Interactive mode owns this task's presentation via the live region
// instead. A phase change streams every time its text changes; a progress
// update streams at each milestone (shouldEmitPlainProgressLocked) instead
// of flooding CI logs with every tick or going silent after the first.
//
// A child declared inside a collection streams too. Skipping it left a piped
// run with only its header line for the whole ~70s of work — the dialect's
// "never silent between the first line and Done" is not conditional on where
// the Running task happens to sit in the tree, and a collection whose
// children are explicitly named has no aggregate row streaming in its place.
func (o *Output) emitTaskRunningProgressiveLocked(st *taskState, trigger taskProgressiveTrigger) {
	if st == nil || st.state != Running {
		return
	}
	live := o.liveLocked()
	interactive := live != nil && live.IsInteractive() && !o.cfg.plain
	if interactive {
		return
	}
	itemOnly := false
	switch trigger {
	case triggerPhase:
		if st.phase == st.plainStream.phase {
			return
		}
		st.plainStream.phase = st.phase
	case triggerProgress:
		if !shouldEmitPlainProgressLocked(st) {
			return
		}
		// A milestone still owed from before this one crossed means no
		// Doing claimed it in time — flush it now, as a bare line, before
		// this milestone takes its place. Never let a later milestone
		// silently swallow an earlier one that a Doing might still be
		// about to name for a count that no longer exists.
		o.flushOwedMilestoneLocked(st)
		st.plainStream.progressStarted = true
		st.plainStream.progressEmitted = st.progress.Completed
		if st.plainStream.namesItems {
			// Doing has named an item for this task before: the next item
			// carries this milestone's count, so defer to it rather than
			// naming the item still in progress from the milestone before.
			st.plainStream.itemOwed = true
			st.plainStream.pendingCompleted = st.progress.Completed
			st.plainStream.pendingTotal = st.progress.Total
			return
		}
		// No Doing has named an item yet for this task, so the count alone
		// streams now — a task that never calls Doing at all (pure
		// Progress) must not wait on one. itemOwed stays owed (a Doing
		// immediately after this still gets to name the item in progress),
		// but bareStreamed remembers the count already streamed, so that
		// Doing shows the item alone rather than repeating it (the E-119
		// review's duplicate-first-milestone bug).
		st.plainStream.itemOwed = true
		st.plainStream.bareStreamed = true
	case triggerItem:
		wasFirstNamedItem := !st.plainStream.namesItems
		st.plainStream.namesItems = true
		if !st.plainStream.itemOwed {
			return
		}
		st.plainStream.itemOwed = false
		st.plainStream.phase = st.phase
		if wasFirstNamedItem && st.plainStream.bareStreamed {
			// This exact count already streamed bare an instant ago (the
			// triggerProgress branch above, on the task's very first
			// milestone, before it was known whether Doing would follow).
			// Naming the item is still worth a line, but repeating the
			// count would be the duplicate the E-119 review flagged.
			itemOnly = true
		}
		st.plainStream.bareStreamed = false
	}
	row := st.snapshot()
	row.Name = progressiveRowName(st)
	if itemOnly {
		row.Progress = Progress{}
	}
	var b strings.Builder
	render.WriteTask(&b, row, o.humanStyle())
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
	// This line already proved the task is alive; the §40 heartbeat only
	// needs to cover the silence that follows real narration, not compete
	// with it (see deferPlainHeartbeatLocked).
	o.deferPlainHeartbeatLocked(st, o.cfg.clock.Now())
}

// flushOwedMilestoneLocked streams a still-owed plain-mode milestone line
// that no Doing claimed — a later milestone superseded it, or the task
// resolved before its item arrived (evo-rec.md/E-119 review: "final n/n
// dropped when Doing comes before Progress"). It renders the pinned
// pendingCompleted/pendingTotal, not st.progress's current value, since by
// the time this runs st.progress has typically already moved on to a later
// count. A milestone that DID get claimed by a Doing (itemOwed already
// false) is a no-op here — a claimed count must never stream twice.
func (o *Output) flushOwedMilestoneLocked(st *taskState) {
	if !st.plainStream.itemOwed {
		return
	}
	st.plainStream.itemOwed = false
	if st.plainStream.bareStreamed {
		// Already streamed as a bare line the instant its milestone
		// crossed (triggerProgress, before namesItems was known) — nothing
		// left owed to flush, or it would duplicate that count.
		st.plainStream.bareStreamed = false
		return
	}
	row := st.snapshot()
	row.Name = progressiveRowName(st)
	// This milestone belongs to the run's still-Running phase even when
	// resolution is what finally flushed it (task_commit.go's
	// commitSettledLocked calls this before the terminal row streams) — the
	// line is the count in progress at that moment, never the outcome. Clear
	// Summary/Problems too: by resolution time they may already be set for
	// the terminal row that is about to follow, and taskRow.head() prefers
	// a Summary headline over in-flight progress unconditionally.
	row.State = Running
	row.Summary = ""
	row.Problems = nil
	row.Progress.Completed = st.plainStream.pendingCompleted
	row.Progress.Total = st.plainStream.pendingTotal
	var b strings.Builder
	render.WriteTask(&b, row, o.humanStyle())
	if b.Len() == 0 {
		return
	}
	o.writeDurableTextLocked(b.String())
	o.deferPlainHeartbeatLocked(st, o.cfg.clock.Now())
}

// residualHasTaskRows reports whether the run declared any standalone task or
// collection at all — regardless of whether its row already streamed
// progressively or is still pending render in this call — so
// residualCompositionLocked's blank-line separator sees the run's whole
// shape, not just the slice this one call is about to write.
func residualHasTaskRows(o *Output, snap Snapshot) bool {
	return len(o.tasks) > 0 || len(snap.Collections) > 0
}

// residualHasEffectSections mirrors residualHasTaskRows for the run's
// [changed]/[planned] ledger — true only when a section remains that Finish
// still owns rendering for. A named/enumerate section that already streamed
// at its task's resolution (commitNamedEffectsLocked) does not count: it
// renders nothing further here, so it must not reserve the blank-line
// separator either.
func residualHasEffectSections(o *Output) bool {
	for _, c := range o.changes {
		if !c.namedRowsEmitted {
			return true
		}
	}
	for _, p := range o.plans {
		if !p.namedRowsEmitted {
			return true
		}
	}
	return false
}

// residualCompositionLocked is the ONE ordered sequence every human-stream
// destination reaching Finish's tail renders: unemitted lines, unresolved
// standalone tasks + collections (only for the destination that owns them —
// see includeEntities), effects (Changes/Plan — never streamed
// progressively, so every destination always renders them), the Conclusion
// band, then an optional debug-pane failure tail. Both residualPlainLocked
// (the plain/primary-mirror destination) and residualInteractiveFinalLocked
// (the live terminal's WriteFinal body) call this one function and differ
// only in includeEntities and their own write-target formatting (the
// terminal trims its trailing newline; the mirror does not) — a section
// added here reaches both destinations mechanically, closing release-gate
// round 6 finding 1 (the third parity gap between the two paths: misuse
// lines, then the debug tail, and now writeEffects — residualInteractiveFinalLocked
// used to hand-duplicate this sequence and silently drop the effects
// section, so a TTY dry-run's Plan/Changes ledger never reached the screen).
//
// includeEntities is true for the one destination that owns rendering
// unresolved tasks/collections at this Finish: the interactive live
// terminal always does (H.17 compact line); the plain/primary mirror does
// only when there is no live interactive terminal to own them instead —
// dual-stream skips them here to avoid a second render of the same rows on
// two destinations.
func (o *Output) residualCompositionLocked(snap Snapshot, linesFrom int, includeEntities bool) string {
	style := o.humanStyle()
	var b strings.Builder
	for i := linesFrom; i < len(snap.Lines); i++ {
		render.WriteDebugOrLine(&b, snap.Lines[i], style.Color)
	}
	if includeEntities {
		o.writeResidualEntitiesLocked(&b, snap, style)
	}
	// A blank line separates the task block from the [changed]/[planned]
	// ledger (fixture-repo-retire-dryrun.md: line 12→14) — checked against
	// the run's whole task/collection set, not just what this call happened
	// to render, since a task's row may already have streamed progressively
	// (commitResolvedTaskLocked) before Finish ever reaches this ledger.
	if residualHasTaskRows(o, snap) && residualHasEffectSections(o) {
		b.WriteByte('\n')
	}
	o.writeResidualLedgerLocked(&b, style)
	if snap.Conclusion != nil && !render.ShouldSuppressStandaloneConclusion(snap) {
		render.WriteConclusion(&b, render.StandaloneConclusion(snap), style)
	}
	o.writeDebugTailLocked(&b, snap, style.Color)
	return b.String()
}

// writeResidualEntitiesLocked writes every root Task row not already
// streamed, then every collection, as human output shows them
// (render.HumanProjection).
func (o *Output) writeResidualEntitiesLocked(b *strings.Builder, snap Snapshot, style render.Style) {
	nameWidth := o.rootColumn.nameWidth()
	human := render.HumanProjection(snap, style.Verbose)
	shown := make(map[string]bool, len(human.Tasks))
	for _, t := range human.Tasks {
		shown[t.ID] = true
	}
	for _, t := range o.tasks {
		if t.collection != nil || t.coreEmitted {
			continue
		}
		if shown[t.id] {
			render.WriteTaskAligned(b, t.snapshot(), nameWidth, style)
		}
		t.coreEmitted = true
	}
	for _, col := range human.Collections {
		render.WriteCollection(b, col, style)
	}
}

// writeResidualLedgerLocked writes every [changed] then [planned] section
// that did not already stream at its Task's resolution.
func (o *Output) writeResidualLedgerLocked(b *strings.Builder, style render.Style) {
	for _, sections := range []*[]*ledgerSection{&o.changes, &o.plans} {
		nameWidth := maxSubjectWidth(*sections)
		for _, s := range *sections {
			if !s.namedRowsEmitted {
				render.WriteEffects(b, o.effectSectionLocked(s, nameWidth), style)
			}
		}
	}
}

// writeDebugTailLocked writes the pane-mode diagnostic tail under the final
// result (§21.3.2). The default preserveOnBad path only fires when
// debugPaneActive is true, which only happens for a live rolling pane.
func (o *Output) writeDebugTailLocked(b *strings.Builder, snap Snapshot, color bool) {
	if snap.Conclusion == nil || !o.shouldPreserveDebugTailLocked(*snap.Conclusion) {
		return
	}
	rows := o.cfg.debugPane.height
	if rows <= 0 {
		rows = defaultDebugPaneHeight
	}
	writeDebugTail(b, o.debugRecords, rows, color)
}

// residualPlainLocked builds the Finish tail for the plain/primary-mirror
// human stream: only what has not already been progressive-emitted.
// renderPlain(snap, ...) still renders the full snapshot for a caller that
// wants the complete plain projection (C8: the former FinalPlain cache is
// gone — reconstruct via renderPlain(out.Snapshot(), ...)).
//
// Interactive mode: tasks/collections are owned by WriteFinal (H.17 compact
// line); this destination's own copy must not reprint them onto primary
// (same stream as Terminal) — includeEntities is false whenever a live
// interactive terminal owns them instead. See residualCompositionLocked for
// the shared ordered sequence both destinations render.
//
// Plain order contract (P2): progressive Item/Task rows and Printf lines
// interleave by completion/call time. Residual only appends entities that
// never streamed (still-pending-until-Finish, collections, effects,
// conclusion).
func (o *Output) residualPlainLocked(snap Snapshot) string {
	linesFrom := o.linesEmitted
	interactive := o.liveLocked() != nil && o.liveLocked().IsInteractive() && !o.cfg.plain
	text := o.residualCompositionLocked(snap, linesFrom, !interactive)
	o.linesEmitted = len(snap.Lines)
	return text
}

// residualInteractiveFinalLocked is the WriteFinal body for interactive
// mode: it always owns rendering unresolved tasks/collections (includeEntities
// true), then the same effects/Conclusion/debug-tail sections
// residualCompositionLocked shares with residualPlainLocked — one model, one
// conclusion, both surfaces. Never re-dumps already-streamed durable
// evidence (that was the double-print bug) — a never-ran standalone task
// (o.tasks, not snap.Tasks: coreEmitted lives on the internal state) already
// streamed via flushGateNowLocked/emitTaskRunningProgressiveLocked and is
// skipped by residualCompositionLocked's own coreEmitted check.
//
// linesFrom is the index into snap.Lines this call owns — captured by Finish
// before residualPlainLocked drains the same shared o.linesEmitted counter
// for its own copy, so the live terminal and any primary/AlsoWrite mirror
// each independently render the unemitted tail exactly once (release-gate
// round 5 finding 1). The trailing-newline trim is this destination's own
// write-target formatting (the terminal driver owns its own line discipline;
// the plain/primary mirror does not trim).
func (o *Output) residualInteractiveFinalLocked(snap Snapshot, linesFrom int) string {
	text := o.residualCompositionLocked(snap, linesFrom, true)
	return strings.TrimRight(text, "\n")
}

// plainStreamMark is plain/non-interactive progressive streaming's
// bookkeeping for a still-Running standalone task (P10: CI logs must not
// stay silent until Finish; beginner-8: a durable line per progress
// increment, thinned to milestones for large totals).
type plainStreamMark struct {
	// phase is the last Phase text already streamed, so a repeated/no-op
	// Phase call does not re-emit.
	phase string
	// progressStarted is false until the first Progress/Bytes tick.
	progressStarted bool
	// progressEmitted is the last completed value actually streamed, so a
	// later tick knows whether it crossed a milestone boundary.
	progressEmitted int64
	// namesItems is true once Doing has named a current item of the count,
	// so a milestone waits for the next item instead of repeating a stale
	// one.
	namesItems bool
	// itemOwed is true from a milestone until the next item streams: the
	// one item line that milestone allows.
	itemOwed bool
	// bareStreamed is true when the currently owed milestone already
	// streamed its count as a bare line (the task's first-ever milestone,
	// streamed before namesItems was known) — the Doing that claims it
	// shows the item alone instead of repeating that count.
	bareStreamed bool
	// pendingCompleted/pendingTotal pin the exact count an owed milestone
	// is for, captured the instant the milestone crosses — st.progress has
	// usually moved on again by the time flushOwedMilestoneLocked actually
	// streams it (a later Progress call, or task resolution), so the
	// streamed line must not read the current count off st.progress.
	pendingCompleted, pendingTotal int64
}
