// Package core owns evident-output's data model: the immutable, fields-only
// value types a finished or in-flight run presents (Snapshot, Problem,
// Conclusion, and peers) plus the pure functions that derive one from
// another (InferConclusion, SanitizeProblem, ...). Never imports the root
// package or internal/render or internal/evidence — every rendering/capture
// machinery package imports core, never the reverse.
//
// The root package (github.com/zachbornheimer/evident-output) re-declares
// every type here as a public alias (type X = core.X) so the published API
// surface is unchanged; root's own doc comment on each alias is the
// pkg.go.dev-visible one (see EVIDENT_OUTPUT_ARCHITECTURE_SPEC_v0.5.md §38).
package core

import "time"

// Snapshot is an immutable complete presentation state at a version.
type Snapshot struct {
	Version     uint64
	OutputID    string
	Subject     string
	Tasks       []TaskSnapshot
	Collections []TasksSnapshot
	Changes     []ChangesSnapshot
	Plans       []PlanSnapshot
	Messages    []MessageSnapshot
	// Lines is a derived compatibility projection of projected message texts
	// (and legacy debug history lines). Prefer Messages for structured consumers.
	Lines      []string
	Actions    []Action
	Conclusion *Conclusion
	Timestamp  time.Time
	// DryRun is the run's planned tense — mutation verbs are Plan-only,
	// never Changes — set by Config.DryRun and Config.Preview alike. The
	// plain/final projection uses this to open with an unmissable header
	// line; no caller decides whether to announce it.
	DryRun bool
	// Preview mirrors Config.Preview: the planned tense above is a preview
	// before a confirm gate, so the header is the subject alone rather than
	// "[dry-run] <subject>". A preview that announced itself as a dry run
	// would promise nothing will happen and then ask permission to apply.
	Preview bool
	// DryRunSubject is Config.Subject's text, carried separately from the
	// Title-derived Subject field above, so the dry-run marker can merge it
	// onto its own single line ("[dry-run] <subject>",
	// fixture-repo-retire-dryrun.md) instead of printing subject as its own
	// durable line the way a non-dry-run run does. Empty when the caller set
	// no Config.Subject, in which case the marker falls back to its plain
	// announcement text.
	DryRunSubject string
	// Warnings holds evo.Problem's run-scoped warning-severity annotations
	// (P8 symmetry with a task's own
	// Problem(summary, evo.Severity(evo.SeverityWarning))) — a warning about
	// the run itself, not about any one task. Feeds Conclusion.Warned/
	// "· warned" exactly like a task warning, never a headline state of its
	// own (evo-rec.md "warnings annotate lifecycle; they do not replace it").
	Warnings []Problem
	// Facts holds evo.Fact's run-scoped annotations (P8) — discovered
	// information about the run itself (evo.Fact("language", "go")), fire-
	// and-forget durable dim lines, rendered once in call order alongside
	// Lines/Messages.
	Facts []Fact

	// rootTally mirrors TasksSnapshot's tally for the standalone root
	// Tasks of a live projection. See WithRootTally.
	rootTally *ChildTally
	// rootCollectionTally mirrors TasksSnapshot's collectionTally for the
	// root collections of a live projection. See WithRootCollectionTally.
	rootCollectionTally *CollectionTally
}

// TaskSnapshot is an immutable task view.
type TaskSnapshot struct {
	ID string
	// Key is the §3.1 stable machine identity: the default kind+parent-key+
	// normalized-name derivation, or an explicit evo.ID/TaskHandle.Key
	// override. Always populated once the task is declared.
	Key   string
	Name  string
	State EntityState
	Phase string
	// ActivityAt is the domain-clock time of the most recent Phase, Progress,
	// or mutation-verb-callback-starting call. Zero when the task has never
	// had any of those. The live renderer's elapsed-time suffix (P5,
	// elapsedAfter) does not read this field — it anchors to
	// liveFirstSeenAt alone, so a fresh Phase/Progress call never restarts
	// the render clock.
	ActivityAt time.Time
	// liveFirstSeenAt is presentation-internal bookkeeping (live-region
	// rendering's activitySince) — the one elapsed-time anchor every row's
	// heartbeat suffix reads (P5), including a row with no ActivityAt at
	// all. Never part of the public snapshot contract. Set via
	// NewTaskSnapshot, read via LiveFirstSeenAt.
	liveFirstSeenAt time.Time
	Progress        Progress
	Summary         string
	Problems        []Problem
	// Warnings holds the task's accumulated warning-severity Problem
	// annotations (Problem(summary, evo.Severity(evo.SeverityWarning)))
	// (P2): warnings
	// annotate the task's lifecycle, they never become a lifecycle state of
	// their own. Rendering inlines a single short warning on the
	// task's own row; multiple or long warnings render as nested lines.
	Warnings []Problem
	// Facts holds TaskHandle.Fact's accumulated annotations (P8): discovered
	// information about the task, at info severity — never a lifecycle state,
	// never work of its own. Renders as a dim "name  value" line, inline when
	// it is the task's only annotation, nested otherwise (same DisplayUnit
	// annotations-slot placement rule Warnings uses at warning severity).
	Facts []Fact
	// Verification holds every VerificationDetail an evo.File/evo.Exec
	// operation this Task ran recorded (spec §2/§8.2/§36) — one entry per
	// managed attribute, satisfied or not. Rendering shows the full list
	// once the Task itself failed (so a reader sees which attribute is
	// isolated from the ones that already held); a succeeded Task's own
	// satisfied entries surface only under Verbose (spec §49).
	Verification []VerificationDetail
	Actions      []Action
	// Skipped is the disposition taxonomy accumulated by
	// TaskHandle.Skipped — the source the "- skipped N (...)" render line
	// derives its count and reason partition from.
	Skipped     []TaxonomyRecord
	Collection  string
	Declaration int
	// Resolution names why this Task settled successfully (§29/§30): empty
	// until it does. See Resolution's own doc for the three reasons.
	Resolution Resolution
	// Evidence preserves both Verify observation phases this Task recorded,
	// if any (§30) — the zero value when Verify was never called.
	Evidence TaskEvidence
	// synthetic marks a task the library invented to carry an output-level
	// outcome (Output.Failf/Cancel) rather than one the caller declared —
	// presentation-internal bookkeeping (coalescing), never part of the
	// public snapshot contract. Set via NewTaskSnapshot, read via Synthetic.
	synthetic bool
}

// NewTaskSnapshot returns base with its presentation-internal bookkeeping
// fields set — the only way to populate them from outside this package
// (they are deliberately unexported: never part of the public snapshot
// contract). Called once, by the root package's taskState.snapshot().
func NewTaskSnapshot(base TaskSnapshot, liveFirstSeenAt time.Time, synthetic bool) TaskSnapshot {
	base.liveFirstSeenAt = liveFirstSeenAt
	base.synthetic = synthetic
	return base
}

// LiveFirstSeenAt returns the domain-clock time this task was first actually
// painted in the live region (zero if never painted, or not yet computed).
func (t TaskSnapshot) LiveFirstSeenAt() time.Time { return t.liveFirstSeenAt }

// Synthetic reports whether the library invented this task to carry an
// output-level outcome (Output.Failf/Cancel) rather than the caller having
// declared it.
func (t TaskSnapshot) Synthetic() bool { return t.synthetic }

// TaxonomyRecord is one accumulated (reason, name) disposition entry —
// recorded by TaskHandle.Skipped, never assembled by hand.
type TaxonomyRecord struct {
	Reason string
	Name   string
	// Causes holds the sanitized text of any errs passed to Skipped for
	// this record — evidence for why the disposition happened, rendered as
	// one bounded └─ line under the count row (first cause + "(+N more)"),
	// full list under Verbose.
	Causes []string
}

// TasksSnapshot is an immutable collection view (evo.Group or
// evo.Sequence).
type TasksSnapshot struct {
	ID string
	// Key is the §3.1 stable machine identity for this Group/Sequence: the
	// default kind+parent-key+normalized-name derivation, or an explicit
	// override. Always populated — Group/Sequence keys are recorded even
	// though the container itself carries no persisted operation state,
	// because Task parent identity and manifest graph edges depend on them.
	Key     string
	Name    string
	State   EntityState
	Summary string
	Tasks   []TaskSnapshot
	// Collections holds nested child containers declared via
	// Group.Group, Group.Sequence, Sequence.Group, or Sequence.Sequence
	// (P3's recursive nesting) — a rendering walk that stops at Tasks alone
	// misses any container nested this way.
	Collections []TasksSnapshot
	// Sequential reports whether this container is an evo.Sequence (ordered
	// dependency, "one Running child" heart contract, failure cascades to
	// NotStarted) rather than an evo.Group (independent children,
	// scheduler may overlap, concurrent Running children expected).
	Sequential  bool
	Declaration int

	// tally, when set, marks Tasks as a partial list: a live projection
	// kept only the children a frame can show, and tally counts every
	// child it was built from. Unexported presentation bookkeeping; see
	// WithChildTally.
	tally *ChildTally
	// collectionTally, when set, marks Collections as a partial list: a
	// live projection left out the child collections a frame cannot
	// reach, and collectionTally sums them. See WithCollectionTally.
	collectionTally *CollectionTally
}

// ChangesSnapshot is an immutable changes section.
type ChangesSnapshot struct {
	ID      string
	Subject string
	Records []EffectRecord
	// IntendedVerb is the first mutation verb recorded for this section, even
	// when every record ended up with zero quantity and none survived into
	// Records. Empty when no verb was ever recorded (evo-rec.md "empty effect
	// section grammar"). Never caller-assembled.
	IntendedVerb string

	// owner is the ID of the Task the section belongs to (see
	// NewChangesSnapshot). Unexported: identity for presentation policy,
	// never part of the public snapshot.
	owner string
}

// NewChangesSnapshot is c owned by the Task ownerID.
func NewChangesSnapshot(c ChangesSnapshot, ownerID string) ChangesSnapshot {
	c.owner = ownerID
	return c
}

// ChangesOwner is the ID of the Task c belongs to, or "" when c was built
// without one.
func ChangesOwner(c ChangesSnapshot) string { return c.owner }

// PlanSnapshot is an immutable plan section.
type PlanSnapshot struct {
	ID      string
	Subject string
	Records []EffectRecord
	// IntendedVerb mirrors ChangesSnapshot.IntendedVerb for plan sections.
	IntendedVerb string

	// owner mirrors ChangesSnapshot's.
	owner string
}

// NewPlanSnapshot is p owned by the Task ownerID.
func NewPlanSnapshot(p PlanSnapshot, ownerID string) PlanSnapshot {
	p.owner = ownerID
	return p
}

// PlanOwner is the ID of the Task p belongs to, or "" when p was built
// without one.
func PlanOwner(p PlanSnapshot) string { return p.owner }

// EffectRecord is one semantic change or plan row.
type EffectRecord struct {
	Verb     string
	Quantity int64
	HasQty   bool
	Object   string
}
