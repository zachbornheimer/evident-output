package plain

import (
	"strings"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Residual is what Finish still owes the human streams, from snap: plain
// for the primary stream and its mirrors, final for a live terminal's last
// word. Both are one composition (compose), so a section added there
// reaches both: the two used to drift, dropping misuse lines, the debug
// tail and then the ledger from the terminal (release-gate round 6
// finding 1). Neither re-dumps what already streamed.
//
// The live terminal owns the rows of unresolved Tasks and collections; the
// primary stream renders them only when no live terminal does, so a run
// with both never prints them twice.
func (s *Stream) Residual(snap record.Snapshot) (plain, final string) {
	linesFrom := s.linesEmitted
	interactive := s.cfg.Region.Interactive()
	plain = s.compose(snap, linesFrom, !interactive)
	s.linesEmitted = len(snap.Lines)
	if interactive {
		final = strings.TrimRight(s.compose(snap, linesFrom, true), "\n")
	}
	return plain, final
}

// compose is the one ordered sequence every human destination renders at
// Finish: the lines not yet streamed, the root rows and collections (when
// entities), the ledger sections that did not stream, the conclusion band,
// then the debug pane's failure tail.
func (s *Stream) compose(snap record.Snapshot, linesFrom int, entities bool) string {
	style := s.cfg.Style
	var b strings.Builder
	for i := linesFrom; i < len(snap.Lines); i++ {
		WriteDebugOrLine(&b, snap.Lines[i], style.Color)
	}
	if entities {
		s.writeEntities(&b, snap)
	}
	// A blank line separates the Task block from the ledger
	// (fixture-repo-retire-dryrun.md), judged on the run's whole shape: a
	// row may have streamed long before Finish reached the ledger.
	if (s.declared > 0 || len(snap.Collections) > 0) && s.hasUnstreamedSection() {
		b.WriteByte('\n')
	}
	for _, tense := range []record.LedgerTense{record.TenseChanged, record.TensePlanned} {
		for _, sec := range s.foldedSections(tense) {
			project.WriteEffects(&b, sec, style)
		}
	}
	if snap.Conclusion != nil && !project.ShouldSuppressStandaloneConclusion(snap) {
		WriteConclusion(&b, project.StandaloneConclusion(snap), style)
	}
	if snap.Conclusion != nil && s.preserveTail(snap.Conclusion.Outcome) {
		rows := s.cfg.Pane.Rows()
		project.WriteDebugTail(&b, s.cfg.Run.DebugRecordTail(rows), rows, style.Color)
	}
	return b.String()
}

// writeEntities writes every root row not already streamed, then every
// collection, as human output shows them (project.HumanProjection).
func (s *Stream) writeEntities(b *strings.Builder, snap record.Snapshot) {
	human := project.HumanProjection(snap, s.cfg.Style.Verbose)
	shown := make(map[string]bool, len(human.Tasks))
	for _, t := range human.Tasks {
		shown[t.ID] = true
	}
	for _, r := range s.roots {
		if r.emitted {
			continue
		}
		if shown[string(r.ID)] {
			project.WriteTaskAligned(b, s.cfg.Source.Snapshot(r.ID), s.column.nameWidth(), s.cfg.Style)
		}
		r.emitted = true
	}
	for _, col := range human.Collections {
		if !s.blockStreamed(col.ID) {
			WriteCollection(b, col, s.cfg.Style)
		}
	}
}

// hasUnstreamedSection reports a ledger section Finish still renders.
func (s *Stream) hasUnstreamedSection() bool {
	for _, tense := range []record.LedgerTense{record.TenseChanged, record.TensePlanned} {
		for _, sec := range s.cfg.Run.Sections(tense) {
			if !s.streamed[sec.ID] {
				return true
			}
		}
	}
	return false
}

// foldedSections is tense's ledger laid out for project.WriteEffects, with
// sibling same-effect sections folded into one row.
func (s *Stream) foldedSections(tense record.LedgerTense) []project.EffectSection {
	sections := s.cfg.Run.Sections(tense)
	sources := make([]project.SectionSource, len(sections))
	for i, sec := range sections {
		sources[i] = project.SectionSource{
			Subject: sec.Subject, Records: sec.Records, IntendedVerb: sec.IntendedVerb,
			Containers: sec.Containers, Streamed: s.streamed[sec.ID],
		}
	}
	return project.FoldEffectSections(tense.String(), s.width(), sources)
}

// preserveTail reports whether Finish appends the debug pane's
// diagnostic tail: always when asked for, else only on a run that did not
// succeed and whose records the live pane, not history, presented
// (§21.3.2). History already streamed every line; it needs no second dump.
func (s *Stream) preserveTail(outcome record.Outcome) bool {
	pane := s.cfg.Pane
	if pane == nil {
		return false
	}
	if pane.PreserveAlways {
		return s.cfg.Run.DebugRecordCount() > 0
	}
	if !s.paneActive || !pane.PreserveOnBad {
		return false
	}
	switch outcome {
	case record.OutcomeFailed, record.OutcomeRefused, record.OutcomeCancelled:
		return true
	default:
		return false
	}
}
