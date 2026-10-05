package render

// CompactLayoutMaxWidth switches changes/plans to compact rows, and bounds
// how many display cells a Done task's one inline warning/fact/taxonomy
// annotation may occupy before it moves to its own nested line — plain and
// live share this one width policy.
const CompactLayoutMaxWidth = 40

// TaskAnnotationIndent nests a standalone task's annotations — taxonomy
// tallies, verification details, warnings, facts — under its row
// (spec §26/§27: "✓ branches  50 checked" / "  ! kept 13 (...)").
const TaskAnnotationIndent = "  "

// GroupChildIndent nests a Group header's children: its child rows and
// the tallies its folded items leave behind, in one column.
const GroupChildIndent = "   "
