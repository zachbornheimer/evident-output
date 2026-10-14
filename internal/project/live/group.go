package live

import (
	"fmt"
	"slices"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// liveGroupHeader is a live Group's header row. While any child is
// unresolved it spins and carries "N/M complete" plus the one monotonic
// elapsed clock (P5), anchored to the earliest live-first-seen time among
// its (recursive) children — when this header itself first painted. Once
// every child has settled (spec §18's worked example: "✓ launch agent",
// no count) the count is redundant with the glyph and disappears.
func liveGroupHeader(col record.TasksSnapshot, done, total int, st liveStyle) project.DisplayUnit {
	spin, color, now, profile := st.spin, st.Color, st.now, st.Profile
	glyph, state := project.TaskGlyph(col.State, profile), col.State
	if col.State == record.Failed {
		glyph = project.GlyphFailedState.Render(profile)
	}
	unresolved := anyChildRunning(col) || anyChildPendingActive(col)
	if unresolved {
		glyph, state = spin, record.Running
	}
	unit := project.DisplayUnit{Glyph: project.StyleGlyph(glyph, project.StateColor(state), color), Name: col.Name}
	if unresolved {
		unit.Elapsed = heartbeatSuffix(now, earliestLiveFirstSeen(col))
		unit.Detail = fmt.Sprintf("%d/%d complete", done, total) + unit.Elapsed
	}
	return unit
}

func anyChildRunning(col record.TasksSnapshot) bool {
	if project.OwnCounts(col).Running || project.CollectionTallyOf(col).Tasks.Running {
		return true
	}
	return slices.ContainsFunc(col.Collections, anyChildRunning)
}

// anyChildPendingActive reports whether the collection has any unresolved
// child — record.Running, or record.Pending regardless of Phase. A record.Pending child that
// never called Phase is the ordinary case (Phase/record.Progress are the only
// record.Running promoters), so requiring Phase here used to leave an all-record.Pending
// or record.Pending-tailed collection's header frozen on derivedState's static
// record.Pending glyph (evo-rec.md record.Problem 9). Recurses into nested
// containers (P3) so a still-pending grandchild keeps the root header honest.
func anyChildPendingActive(col record.TasksSnapshot) bool {
	if counts := project.OwnCounts(col); counts.Running || counts.Pending {
		return true
	}
	if left := project.CollectionTallyOf(col).Tasks; left.Running || left.Pending {
		return true
	}
	return slices.ContainsFunc(col.Collections, anyChildPendingActive)
}

// earliestLiveFirstSeen returns the earliest LiveFirstSeenAt among a
// container's tasks and (recursively) its nested containers — the moment
// this header itself was first actually painted, and so the anchor its own
// elapsed-time suffix measures from (P5).
func earliestLiveFirstSeen(col record.TasksSnapshot) time.Time {
	earliest := earlierSeen(project.OwnCounts(col).EarliestSeen, project.CollectionTallyOf(col).Tasks.EarliestSeen)
	for _, child := range col.Collections {
		earliest = earlierSeen(earliest, earliestLiveFirstSeen(child))
	}
	return earliest
}

// earlierSeen is the earlier of two live-first-seen stamps, where zero
// means never seen.
func earlierSeen(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// attentionRankCount is how many liveRank classes can fill a frame's rows
// once a collection has more children than fit: failed, warned, running,
// pending.
const attentionRankCount = 4

// liveRank orders a child for the rows of a frame too small for all of
// them: failed, warning, active (running), pending, successful, other.
// Warning is a Done-task annotation (P2), not a lifecycle state, so it
// ranks ahead of state on len(t.Warnings) rather than t.State.
func liveRank(t record.TaskSnapshot) int {
	switch {
	case t.State == record.Failed:
		return 0
	case len(t.Warnings) > 0:
		return 1
	case t.State == record.Running:
		return 2
	case t.State == record.Pending:
		return 3
	case t.State.Succeeded(), t.State == record.Excluded:
		return 4
	default:
		return 5
	}
}

// selectLiveChildren picks at most max of a collection's total children
// to show, from tasks, which holds all of them or the ones a LiveChildren
// projection kept.
func selectLiveChildren(tasks []record.TaskSnapshot, total, max int) (selected []record.TaskSnapshot, omitted int) {
	if total <= max {
		return tasks, 0
	}
	// Stable: collect by rank preserving declaration order within class.
	// Only the attention ranks can be selected (below), so routine rows —
	// nearly every row of a large finished Group — are never copied.
	var buckets [attentionRankCount][]record.TaskSnapshot
	for _, t := range tasks {
		if r := liveRank(t); r < attentionRankCount {
			buckets[r] = append(buckets[r], t)
		}
	}
	// Once there are more children than fit (the len(tasks) <= max early
	// return above did not apply), only the attention ranks (0-3: failed,
	// warning, running, pending) ever fill the budget — never rank 4/5
	// (routine Done/Skipped/other). A large aggregated Group answers "what
	// needs my attention" plus the header's own N/total count; padding the
	// remaining rows with routine "✓ package-NNN" landmarks merely because
	// there happens to be vertical room left repeats what the header already
	// said, one row at a time, for a Group large enough that its own
	// aggregation was already necessary (spec §25: "aggregation is
	// renderer-owned and automatic").
	for r := 0; r < attentionRankCount && len(selected) < max; r++ {
		for _, t := range buckets[r] {
			if len(selected) >= max {
				break
			}
			selected = append(selected, t)
		}
	}
	return selected, total - len(selected)
}
