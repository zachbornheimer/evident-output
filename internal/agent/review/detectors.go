// Package review — the file-level detector registry GoSourceAt runs.
package review

import (
	"go/ast"
	"go/token"
)

// fileInput is everything one file-level detector may read.
type fileInput struct {
	filename       string
	src            string
	desiredVersion string
	file           *ast.File
	fset           *token.FileSet
	hasEvo         bool
}

// detector is one file-level rule check. needsEvo keeps it off files that
// do not import evo. The release a rule needs is not stated here: it is
// the rule's catalog MinDialect, applied to every finding (see
// admitDialect), so a detector can never drift from its rule.
type detector struct {
	needsEvo bool
	run      func(fileInput) []Finding
}

// admits reports whether d runs on in at all.
func (d detector) admits(in fileInput) bool { return !d.needsEvo || in.hasEvo }

// textRule adapts a detector that reads the file's source text.
func textRule(fn func(filename, src string) []Finding) func(fileInput) []Finding {
	return func(in fileInput) []Finding { return fn(in.filename, in.src) }
}

// astRule adapts a detector that walks the parsed file.
func astRule(fn func(filename string, file *ast.File, fset *token.FileSet) []Finding) func(fileInput) []Finding {
	return func(in fileInput) []Finding { return fn(in.filename, in.file, in.fset) }
}

// fileDetectors is every file-level rule, in the order findings are
// reported. Adding a rule adds one entry here next to its rationale.
var fileDetectors = []detector{
	// API-006, API-026, STREAM-003/EVO-LIVE-001, API-028, API-029,
	// API-018/EVO-EXIT-001, PROG-001: one pass over every selector call.
	// API-006 fires without an evo import, so this entry gates per rule.
	{run: detectSelectorCallRules},
	// LAYOUT-001/LAYOUT-002: cobra command naming and folder ownership.
	// These fire without an evo import — the fixtures are cobra command files.
	{run: astRule(detectDualCommandNaming)},
	{run: astRule(detectCommandInWrongFolder)},
	// SIG-001: a hand-rolled signal.Notify in a file that never calls Cancel
	// reintroduces the exact bug evo.Main already closes — the visual ledger
	// and the exit code can disagree because the signal path never reconciles
	// through Conclusion (evo-rec.md "Interrupts").
	{needsEvo: true, run: textRule(detectSignalNotifyWithoutCancel)},
	// SIG-002: signal.Notify/NotifyContext wired for SIGINT/SIGTERM/
	// os.Interrupt in a file that also calls evo.Main/evo.Run — those
	// entrypoints own that exact lifecycle (RunFunc's context.Context is
	// cancelled on SIGINT/SIGTERM internally), so a
	// second interrupt layer built solely to duplicate it can let the
	// ledger and the process's actual exit path diverge (Decisions
	// 2026-09-23, ZYS-939).
	{needsEvo: true, run: astRule(detectDuplicateSignalWiringAroundMain)},
	// EVO-EXIT-002: evo.Main returns the exit code; a bare evo.Main(run)
	// statement discards it and exits 0 after a failed run (E-114).
	{needsEvo: true, run: astRule(detectDiscardedMainCode)},
	// TERM-015: a child that owns the terminal (tty passthrough) must run
	// inside out.Suspend, or its own UI glues onto the parent's live
	// spinner — no in-process fix helps once two processes share one tty
	// (evo-rec.md "#7b").
	{needsEvo: true, run: textRule(detectTTYPassthroughWithoutSuspend)},
	// CONFIRM-001: a hand-rolled stdin prompt reintroduces the exact bugs
	// evo.Confirm already closes (spinner tearing, CI hangs, declined
	// answers reported as errors instead of Blocked).
	{needsEvo: true, run: textRule(detectHandRolledConfirm)},
	// FP-001/FP-002: heavy I/O ahead of evo.Init/New (FP-001) or between
	// init and the first declared entity (FP-002) reopens the blank-terminal
	// window evo-rec.md "First paint" exists to close.
	{needsEvo: true, run: textRule(detectFirstPaintGaps)},
	// LOOP-001: a work loop (for/range with I/O) before any Task/Group/Sequence
	// is the purge/prune silent-pre-output class — real work must live inside
	// the task definition (Define, or one Task per item), not before entity creation.
	{needsEvo: true, run: textRule(detectSilentPreTaskLoops)},
	// CALL-001: make/new inline inside evo.Init/Task/Group arguments; a
	// named local extracted before the call is the clean form.
	{needsEvo: true, run: astRule(detectInlineConstructAtEvoCall)},
	// FP-003: a task's only Doing call precedes a subprocess run with no
	// further Doing/Progress/Writer — the spinner keeps spinning over a
	// silent child with no way to tell slow from hung.
	{needsEvo: true, run: textRule(detectStaleDoingBeforeSubprocess)},
	// TAX-001: a hand-assembled "%d skipped/kept/retained" string bypasses
	// the reason-partitioned taxonomy evo derives from Skipped/Kept.
	{needsEvo: true, run: textRule(detectHandAssembledTaxonomyCount)},
	// PROG-001 (Doing form): a Doing string smuggling "%d/%d" is progress
	// hidden in narration text instead of a real Progress call.
	{needsEvo: true, run: textRule(detectProgressInDoingString)},
	// BOUND-001: an unbounded slice joined straight into Because/Detail/Doing
	// reproduces the terminal flood evo-rec.md's bounded-rows fix already
	// closed for Plan/Changes.
	{needsEvo: true, run: textRule(detectUnboundedSliceIntoNarration)},
	// API-030: Task/Group.Task declared inside a goroutine or g.Go closure
	// races task creation with rendering (evo-rec.md "predeclare Tasks").
	{needsEvo: true, run: textRule(detectTaskDeclaredInsideFanOut)},
	// API-031: a hand-rolled io.Writer whose Write calls TaskHandle.Doing
	// reimplements Task.Writer (evo-rec.md "#6").
	{needsEvo: true, run: textRule(detectHandRolledWriter)},
	// CONFIRM-002: a destructive-sounding Confirm question missing Destructive().
	{needsEvo: true, run: textRule(detectConfirmMissingDestructive)},
	// CON-002: a joined failure list printed directly duplicates Conclusion.
	{needsEvo: true, run: textRule(detectHandAssembledFailureSummary)},
	// EV-001: Fail/Block embedding the retained evidence ring's own .Text()/
	// .Tail() in the summary duplicates what auto-attach already renders.
	{needsEvo: true, run: textRule(detectFailEmbeddedEvidenceText)},
	// FP-004: a Doing string with no domain object is an illegible placeholder.
	{needsEvo: true, run: textRule(detectPlaceholderDoing)},
	// API-032: every superseded spelling (evo.New in main, Cause, Capture,
	// rec-surface Options/To/Plain, the mutation verbs removed in 1.1, Skip/MainWith (removed in 1.0)) gets a derived fix, not a lecture.
	{needsEvo: true, run: detectDeprecatedSpellings},
	// API-033: an entity's own name reused verbatim as its skip/verb argument.
	{needsEvo: true, run: textRule(detectNameEqualsVerbArgument)},
	// API-034: a statement-form Fail/Block immediately followed by return nil
	// discards the error the caller needed to propagate.
	{needsEvo: true, run: textRule(detectFailBlockThenReturnNil)},
	// API-035: io.Discard wired as a sink in a function that itself Fails/
	// Blocks is an evidence-free security-gate shape — the verdict has
	// nothing to show for itself.
	{needsEvo: true, run: textRule(detectDiscardSinkInFailingBlock)},
	// API-036 was removed in 1.1: it offered the Failf/Blockf rewrite for a
	// Fail/Block(fmt.Sprintf(...)) statement, and that family has no f-form
	// any more. The fmt.Sprintf(...) shape API-034 already catches (a
	// statement-form Fail/Block followed by return nil) covers what remains
	// of it.
	// API-038: fmt.Sprintf(...) passed to a printf-variadic evo method
	// (Doing) should flatten into that method's own format + args.
	{needsEvo: true, run: textRule(detectSprintfIntoVariadicVerb)},
	// API-037: a method whose whole body is one call on a Task/Item handle —
	// pure ceremony over the handle's own verb.
	{needsEvo: true, run: textRule(detectWrapperMethod)},
	// DOM-018: err.Error() as the summary alongside evo.Cause(err) surfaces
	// the same error twice; evo.Cause no longer affects the returned error.
	{needsEvo: true, run: textRule(detectErrTwice)},
	// TAX-002: evo.Reason built from a computed expression opens one
	// taxonomy bucket per distinct rendered value instead of one per
	// classification.
	{needsEvo: true, run: textRule(detectDynamicReason)},
	// EVO-UI-001: routine Fact hand-printed as a "label: value" line.
	{needsEvo: true, run: astRule(detectFactPrintedAsUIText)},
	// EVO-UI-002: passing verification hand-printed on the success path.
	{needsEvo: true, run: astRule(detectPassingVerificationPrinted)},
	// EVO-UI-003: collection/progress/status text hand-built instead of
	// derived from Task/Group/Sequence state.
	{needsEvo: true, run: astRule(detectHandBuiltProgressText)},
	// EVO-WIRE-001: internal Snapshot marshaled directly instead of through
	// the sanctioned JSON encoder.
	{needsEvo: true, run: astRule(detectMarshalOfInternalSnapshot)},
	// EVO-WIRE-003: JSON/JSONL stdout mixed with human presentation.
	{needsEvo: true, run: textRule(detectJSONStdoutMixedWithHumanText)},
	// TXT-020: an entity name too long, or narrating a transition (into/->)
	// instead of naming a noun — that detail belongs in Doing/Donef.
	{needsEvo: true, run: textRule(detectLongEntityName)},
	// DOM-019: a Task/Item handle variable reassigned from a new declaration
	// before the previous one was resolved — the earlier row is orphaned
	// Running forever (a double row under one variable name).
	{needsEvo: true, run: textRule(detectShadowedHandle)},
	// DOM-021: a Task declared in a function that never Defines,
	// resolves, or hands it on — its row stays unresolved.
	{needsEvo: true, run: astRule(detectUnresolvedTask)},
	// TXT-021: a Fail/Warn/Block summary hand-assembles a " — cause:"/
	// " — action:" fragment instead of using Detail/Next.
	{needsEvo: true, run: textRule(detectCrammedSummary)},
	// FP-005: Task created and Done with no Doing/Progress/Writer window.
	{needsEvo: true, run: astRule(detectInstantDone)},
	// FP-006: Doing(...) immediately followed by Done(...) with no
	// Define submitting work between them (theater).
	{needsEvo: true, run: astRule(detectDoingDoneTheater)},
	// API-040: Failf/Fail inside a Define callback whose result
	// reaches that same callback — double-resolves the task.
	{needsEvo: true, run: astRule(detectFailInResolvedCallback)},
	// API-041: goroutine/fan-out closure resolves a predeclared Task with
	// no Define inside it.
	{needsEvo: true, run: textRule(detectGoroutineResolvesPredeclaredTask)},
	// API-042: evo.Effect with a nil or no-op callback.
	{needsEvo: true, run: astRule(detectNoOpEffectCallback)},
	// API-043: plural EffectSpec.Object literal.
	{needsEvo: true, run: astRule(detectPluralEffectObject)},
	// API-044: channel-wait wrapper around Define.
	{needsEvo: true, run: textRule(detectChannelWaitWrapperAroundDefine)},
	// API-048: a Group/Sequence Task re-declared by the same string literal
	// to obtain a later dependency reference (duplicate sibling, not a
	// get-or-create) — recommend a typed variable instead.
	{needsEvo: true, run: astRule(detectRedeclaredTaskLiteral)},
	// TAX-003: inline evo.Reason("...") literal, or a reason that restates
	// its own verb.
	{needsEvo: true, run: astRule(detectInlineReasonLiteral)},
	// API-045: Task(name) where name is a bare subject label or a generic
	// container/phase word, not one independently meaningful action.
	{needsEvo: true, run: astRule(detectSubjectOnlyOrContainerTaskName)},
	// API-050: a generic phase/category-named Task (fix/check/classify/
	// resolve/finalize) sequences 2+ independently erroring steps in its own
	// Define callback — structural evidence it owns child-looking work.
	{needsEvo: true, run: astRule(detectPhaseTaskOwningChildWork)},
	// API-047: Task/Group/Sequence declaration reuses a sibling literal name
	// already used by a different entity kind under the same parent.
	{needsEvo: true, run: astRule(detectCrossKindDuplicateSiblingName)},
	// API-049: Define callback discards its scheduler-provided context.
	{needsEvo: true, run: astRule(detectDefineDiscardsSchedulerContext)},
	{needsEvo: true, run: astRule(detectPerFindingFakeTask)},
	{needsEvo: true, run: textRule(detectFlattenedDiagnosticsLoop)},
	// API-057: a filesystem mutator call hidden inside an evo.Effect
	// callback — Effect is the opaque-mutation escape hatch, not a second
	// file-write API (ZYS-851 Decisions).
	{needsEvo: true, run: astRule(detectFileWriteInEffectCallback)},
	// API-058: a patch applied straight to the real workspace through
	// os/exec (`patch`, `git apply`, `git am`) instead of deriving desired
	// file states with evo.Patch and committing them through
	// evo.Files/evo.File (ZYS-934).
	{needsEvo: true, run: astRule(detectDirectWorkspacePatchApply)},
	// API-059: a Patch-derived FileSet is never passed to evo.Files, and
	// the same function commits a freshly built FileSpec through evo.File
	// instead, discarding the source Basis/stale-write guard the FileSet
	// carried (ZYS-935).
	{needsEvo: true, run: astRule(detectPatchFileSetDiscardedBeforeCommit)},
	// API-061: a call site still uses the record-only mutation verbs
	// Record/RecordLabel/RecordName, which have no record-only
	// replacement (ZYS-974) — steer it to Effect (mutation), Fact
	// (information), or File/Patch (file writes).
	{needsEvo: true, run: astRule(detectDeprecatedRecordCall)},
	// API-062: a second Kept/Skipped on one Task — the item is the Task, so
	// the per-item shape is group.Task(item).Kept(reason) (contract §25
	// renderer aggregation folds those children into one tally).
	{needsEvo: true, run: astRule(detectRepeatedDisposition)},
	// API-063: a Verify callback that returns a constant observes nothing.
	{needsEvo: true, run: astRule(detectConstantVerify)},
	// EVO-EVIDENCE-001: legacy named Evidence callback performs a raw mutation.
	{needsEvo: true, run: astRule(detectMutatingLegacyEvidence)},
	// EVO-VERIFY-001: Verify callback performs a raw mutation; Verify must
	// be read-only.
	{needsEvo: true, run: astRule(detectMutatingVerify)},
	// API-046: Skipped(evo.Reason("...")) whose reason names an
	// already-satisfied condition instead of true inapplicability
	// (ResolutionAlreadySatisfied, via Verify or evo.File/evo.Exec).
	{needsEvo: true, run: astRule(detectSkippedForAlreadySatisfied)},
	// API-060: Summary text that is actually mutation/dry-run/already-
	// satisfied narration rather than the caller's own result metadata.
	{needsEvo: true, run: astRule(detectSummaryStampNarration)},
	// EVO-DRYRUN-001: Define callback raw-calls a side effect Evo's runtime
	// cannot intercept, breaking the dry-run guarantee.
	{needsEvo: true, run: astRule(detectRawMutationInDefine)},
	// EVO-DAG-001: a goroutine exists only to make Evo Tasks parallel.
	{needsEvo: true, run: textRule(detectGoroutineWrappingDefine)},
	// EVO-DAG-002: a chained .After(...) reproduces evo.Sequence.
	{needsEvo: true, run: astRule(detectAfterChainDuplicatesSequence)},
	// API-054: raw os/exec.Cmd wired to an Evo Task's Writer() reimplements
	// Exec's own capture/liveness/cancellation with hand-rolled
	// bytes.Buffer/io.MultiWriter plumbing or output-string cancellation
	// matching instead of inspecting the ExecResult evo.Exec now returns
	// (ZYS-850).
	{needsEvo: true, run: textRule(detectManualSubprocessCaptureAroundTask)},
	// API-052: caller-owned Wait loop over stored Task handles, filtering
	// ErrNotStarted/snapshotting/hand-counting failures instead of using
	// GroupHandle.Wait()/SequenceHandle.Wait() (ZYS-849).
	{needsEvo: true, run: textRule(detectCallerWaitLoopOverContainerChildren)},
	// API-053: a second evo.File/Resource-claiming evo.Effect call made
	// with a context an enclosing evo.Effect already holds a Resource on
	// (ZYS-840), directly or one call away through a same-file helper.
	{needsEvo: true, run: astRule(detectNestedResourceAcquisition)},
	// API-055: caller-managed sync.Mutex/RWMutex Lock/Unlock wrapped around
	// an evo.File call that File's automatic resource claim (ZYS-840)
	// already serializes.
	{needsEvo: true, run: astRule(detectManualLockAroundEvoFile)},
	// EVO-DAG-003: a visible producer/consumer relationship has no
	// first-run scheduler ordering.
	{needsEvo: true, run: astRule(detectMissingProducerConsumerOrdering)},
	// API-056: a .After(...) edge whose comment and both Tasks' own
	// resource declarations show the only reason is shared-resource
	// exclusion, not a semantic dependency, which File/FSResource/
	// LogicalResource claim coordination (ZYS-840) already provides.
	{needsEvo: true, run: detectAfterOnlyForResourceContention},
	// API-027: Done/Fail/Progress on Group/Sequence (name-match).
	{needsEvo: true, run: astRule(detectCollectionLeafMisuse)},
	// API-039: Group that only ever has one child in source.
	{needsEvo: true, run: astRule(detectSingletonGroup)},
	// EVO-FILE-001: manual write/chmod file reconciliation, and expensive
	// work already run before a trailing evo.File/evo.Exec return.
	{needsEvo: true, run: astRule(detectManualFileReconciliation)},
	{needsEvo: true, run: astRule(detectExpensiveWorkBeforeFileOp)},
	// EVO-EXEC-001: raw exec guarded by a hand-rolled freshness check.
	{needsEvo: true, run: astRule(detectRawExecWithManualFreshness)},
	// EVO-PROVENANCE-001: a literal path visibly read or passed as a
	// literal Exec Arg that the call's own Basis omits.
	{needsEvo: true, run: astRule(detectOmittedBasisPath)},
	// DOM-014: Detail(err) where Detail expects user-visible text.
	{needsEvo: true, run: textRule(detectDetailOfError)},
	// MCP-014 / DOM-011: expected blocked item treated as application error.
	{needsEvo: true, run: textRule(detectBlockedAsError)},
}
