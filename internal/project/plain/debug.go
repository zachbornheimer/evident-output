package plain

import (
	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Debug presents a debug record the run just recorded (§4.6, §21.3).
//
// History (the default) writes it as durable scrollback above the live
// region, or into the plain stream. The pane shows it in the live region's
// rolling viewport instead; it reaches scrollback only through the failure
// tail Finish may preserve.
//
// A diagnostic stream distinct from the primary one takes the record alone,
// unless both reach the one terminal a live region owns: raw bytes there
// would bypass the region's clear, write and repaint and corrupt the
// spinner row (the realistic default, Config{Stdout, Stderr} on a shell),
// so the record takes the single-stream path instead.
func (s *Stream) Debug(rec record.DebugRecord) {
	history := project.FormatHistoryLine(rec, s.cfg.Style.Color)
	sinks := s.cfg.Sinks
	interactive := s.cfg.Region.Interactive()
	pane := s.cfg.Pane != nil
	separate := sinks.Separate()
	sameTerminal := separate && interactive && sinks.SharesTerminal()
	defer s.SkipLines()

	if separate && !sameTerminal {
		sinks.Diagnostic(project.FormatHistoryLine(rec, false) + "\n")
		// The record went to the diagnostic stream alone. Pane mode still
		// counts as presenting it, or a failing Finish could never
		// preserve the tail on the live terminal (§21.3.2).
		s.paneActive = s.paneActive || (interactive && pane)
		return
	}
	switch {
	case interactive && pane:
		s.paneActive = true
		s.cfg.Region.Signal(true)
	case interactive:
		s.cfg.Region.Durable(history)
	default:
		s.Write(history + "\n")
	}
}
