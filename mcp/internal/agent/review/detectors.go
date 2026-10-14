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
	// name is the detector function's name; fixtures live under
	// testdata/dialect_2_0/<name>/.
	name string
	// migration marks a detector whose whole job is to map a removed or
	// superseded spelling onto the 2.0 surface. Its fixture is dirty input
	// (legacy*.go), which cannot compile against 2.0 by design; every other
	// detector's fixture (bad*.go) must compile against it.
	migration bool
	needsEvo  bool
	run       func(fileInput) []Finding
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

// astSourceRule adapts a detector that walks the parsed file and cuts its
// suggestions from the file's source text.
func astSourceRule(fn func(filename, src string, file *ast.File, fset *token.FileSet) []Finding) func(fileInput) []Finding {
	return func(in fileInput) []Finding { return fn(in.filename, in.src, in.file, in.fset) }
}

// fileDetectors is every file-level rule, in the order findings are
// reported. Adding a rule adds one entry here next to its rationale.
var fileDetectors = []detector{
	// STREAM-003/EVO-LIVE-001, API-018/EVO-EXIT-001: one pass over every
	// selector call.
	{name: "detectSelectorCallRules", needsEvo: true, run: detectSelectorCallRules},
	// API-032, migration: what a 1.x program still calls (module path, Task and
	// Output methods, mutation and record verbs, Option funcs, removed exports)
	// mapped straight onto the 2.0 surface.
	{name: "detectRemovedTwoZero", migration: true, needsEvo: true, run: detectRemovedTwoZero},
	// LAYOUT-001/LAYOUT-002: cobra command naming and folder ownership.
	// These fire without an evo import — the fixtures are cobra command files.
	{name: "detectDualCommandNaming", run: astRule(detectDualCommandNaming)},
	{name: "detectCommandInWrongFolder", run: astRule(detectCommandInWrongFolder)},
	// SIG-001: a signal handler that ends the process with os.Exit skips the
	// run's context, so the ledger and the exit code can disagree.
	{name: "detectSignalNotifyWithoutCancel", needsEvo: true, run: astRule(detectSignalNotifyWithoutCancel)},
	// SIG-002: signal.Notify/NotifyContext wired for SIGINT/SIGTERM/
	// os.Interrupt in a file that calls evo.Main, which already owns that
	// lifecycle (it cancels the run's context on both signals).
	{name: "detectDuplicateSignalWiringAroundMain", needsEvo: true, run: astRule(detectDuplicateSignalWiringAroundMain)},
	// EVO-EXIT-002: evo.Main returns the exit code; a bare evo.Main(cfg, run)
	// statement discards it and exits 0 after a failed run (E-114).
	{name: "detectDiscardedMainCode", needsEvo: true, run: astRule(detectDiscardedMainCode)},
	// TERM-015: a child wired to the inherited terminal (Stdout/Stderr =
	// os.Stdout/os.Stderr) paints over the live frame; capture it with
	// task.Writer(), or run one that must own the terminal inside r.Suspend.
	{name: "detectTTYPassthroughWithoutSuspend", needsEvo: true, run: astRule(detectTTYPassthroughWithoutSuspend)},
	// CONFIRM-001: a hand-rolled stdin prompt reintroduces the exact bugs
	// evo.Confirm already closes (spinner tearing, CI hangs, declined
	// answers reported as errors instead of Blocked).
	{name: "detectHandRolledConfirm", needsEvo: true, run: textRule(detectHandRolledConfirm)},
	// FP-001/FP-002: heavy I/O ahead of evo.Main/evo.Execute (FP-001) or ahead
	// of a run body's first Task/Group/Sequence (FP-002) leaves the terminal
	// blank for the user.
	{name: "detectFirstPaintGaps", needsEvo: true, run: astRule(detectFirstPaintGaps)},
	// LOOP-001: a work loop (range with I/O) before any Task/Group/Sequence
	// is the purge/prune silent-pre-output class: real work must live inside
	// the task definition (Define, or one Task per item).
	{name: "detectSilentPreTaskLoops", needsEvo: true, run: astRule(detectSilentPreTaskLoops)},
	// CALL-001: make/new inline inside evo.Main/evo.Execute arguments; a
	// named local extracted before the call is the clean form.
	{name: "detectInlineConstructAtEvoCall", needsEvo: true, run: astRule(detectInlineConstructAtEvoCall)},
	// FP-003: a task's only Doing call precedes a subprocess run with no
	// further Doing/Progress/Writer — the spinner keeps spinning over a
	// silent child with no way to tell slow from hung.
	{name: "detectStaleDoingBeforeSubprocess", needsEvo: true, run: textRule(detectStaleDoingBeforeSubprocess)},
	// TAX-001: a hand-assembled "%d skipped/kept/retained" string bypasses
	// the reason-partitioned taxonomy evo derives from Skipped/Kept.
	{name: "detectHandAssembledTaxonomyCount", needsEvo: true, run: textRule(detectHandAssembledTaxonomyCount)},
	// PROG-001 (Doing form): a Doing string smuggling "%d/%d" is progress
	// hidden in narration text instead of a real Progress call.
	{name: "detectProgressInDoingString", needsEvo: true, run: textRule(detectProgressInDoingString)},
	// BOUND-001: an unbounded slice joined straight into Because/Detail/Doing
	// reproduces the terminal flood evo-rec.md's bounded-rows fix already
	// closed for Plan/Changes.
	{name: "detectUnboundedSliceIntoNarration", needsEvo: true, run: textRule(detectUnboundedSliceIntoNarration)},
	// API-030: Task declared inside a goroutine or g.Go closure; declaration
	// order is topology and must not follow goroutine scheduling.
	{name: "detectTaskDeclaredInsideFanOut", needsEvo: true, run: textRule(detectTaskDeclaredInsideFanOut)},
	// API-031: a hand-rolled io.Writer whose Write calls TaskHandle.Doing
	// reimplements Task.Writer (evo-rec.md "#6").
	{name: "detectHandRolledWriter", needsEvo: true, run: textRule(detectHandRolledWriter)},
	// CONFIRM-002: a destructive-sounding Confirm question missing Destructive().
	{name: "detectConfirmMissingDestructive", needsEvo: true, run: textRule(detectConfirmMissingDestructive)},
	// CON-002: a joined failure list printed directly duplicates Conclusion.
	{name: "detectHandAssembledFailureSummary", needsEvo: true, run: textRule(detectHandAssembledFailureSummary)},
	// FP-004: a Doing string with no domain object is an illegible placeholder.
	{name: "detectPlaceholderDoing", needsEvo: true, run: textRule(detectPlaceholderDoing)},
	// API-033: an entity's own name reused verbatim as its skip/verb argument.
	{name: "detectNameEqualsVerbArgument", needsEvo: true, run: textRule(detectNameEqualsVerbArgument)},
	// API-035: io.Discard wired as a sink in a function that itself Fails/
	// Blocks is an evidence-free security-gate shape — the verdict has
	// nothing to show for itself.
	{name: "detectDiscardSinkInFailingBlock", needsEvo: true, run: textRule(detectDiscardSinkInFailingBlock)},
	// API-038: fmt.Sprintf(...) passed to a printf-variadic evo method
	// (Doing) should flatten into that method's own format + args.
	// Failf/Blockf were removed in 1.1.
	{name: "detectSprintfIntoVariadicVerb", needsEvo: true, run: textRule(detectSprintfIntoVariadicVerb)},
	// API-037: a method whose whole body is one call on a Task/Item handle —
	// pure ceremony over the handle's own verb.
	{name: "detectWrapperMethod", needsEvo: true, run: textRule(detectWrapperMethod)},
	// DOM-018: err.Error() as the summary alongside evo.Cause(err) surfaces
	// the same error twice; evo.Cause no longer affects the returned error.
	{name: "detectErrTwice", needsEvo: true, run: textRule(detectErrTwice)},
	// TAX-002: evo.Reason built from a computed expression opens one
	// taxonomy bucket per distinct rendered value instead of one per
	// classification.
	{name: "detectDynamicReason", needsEvo: true, run: textRule(detectDynamicReason)},
	// EVO-UI-001: routine Fact hand-printed as a "label: value" line.
	{name: "detectFactPrintedAsUIText", needsEvo: true, run: astRule(detectFactPrintedAsUIText)},
	// EVO-UI-002: passing verification hand-printed on the success path.
	{name: "detectPassingVerificationPrinted", needsEvo: true, run: astRule(detectPassingVerificationPrinted)},
	// EVO-UI-003: collection/progress/status text hand-built instead of
	// derived from Task/Group/Sequence state.
	{name: "detectHandBuiltProgressText", needsEvo: true, run: astRule(detectHandBuiltProgressText)},
	// EVO-WIRE-003: JSON/JSONL stdout mixed with human presentation.
	{name: "detectJSONStdoutMixedWithHumanText", needsEvo: true, run: textRule(detectJSONStdoutMixedWithHumanText)},
	// TXT-020: an entity name too long, or narrating a transition (into/->)
	// instead of naming a noun — that detail belongs in Doing/Donef.
	{name: "detectLongEntityName", needsEvo: true, run: textRule(detectLongEntityName)},
	// DOM-019: a variable bound to a Task is assigned another Task before
	// the first was Defined — the earlier Task never runs.
	{name: "detectShadowedHandle", needsEvo: true, run: astRule(detectShadowedHandle)},
	// DOM-021: a Task declared in a function that never Defines,
	// resolves, or hands it on — its row stays unresolved.
	{name: "detectUnresolvedTask", needsEvo: true, run: astRule(detectUnresolvedTask)},
	// TXT-021: a Fail/Warn/Block summary hand-assembles a " — cause:"/
	// " — action:" fragment instead of using Detail/Next.
	{name: "detectCrammedSummary", needsEvo: true, run: textRule(detectCrammedSummary)},
	// API-040: evo.Fail/Refuse/Exclude called as a statement; the ending it
	// builds is discarded instead of returned.
	{name: "detectDiscardedEnding", needsEvo: true, run: astRule(detectDiscardedEnding)},
	// API-041: goroutine/fan-out closure reports on a Task that is never
	// given Define.
	{name: "detectGoroutineResolvesPredeclaredTask", needsEvo: true, run: textRule(detectGoroutineResolvesPredeclaredTask)},
	// API-042: evo.Effect with a nil or no-op callback.
	{name: "detectNoOpEffectCallback", needsEvo: true, run: astRule(detectNoOpEffectCallback)},
	// API-043: plural EffectSpec.Object literal.
	{name: "detectPluralEffectObject", needsEvo: true, run: astRule(detectPluralEffectObject)},
	// API-044: channel-wait wrapper around Define.
	{name: "detectChannelWaitWrapperAroundDefine", needsEvo: true, run: textRule(detectChannelWaitWrapperAroundDefine)},
	// API-048: a Task re-declared by the same string literal to obtain a
	// later dependency reference (duplicate sibling, not a get-or-create) —
	// recommend a typed variable instead.
	{name: "detectRedeclaredTaskLiteral", needsEvo: true, run: astRule(detectRedeclaredTaskLiteral)},
	// TAX-003: an Exclude reason that restates its own verb.
	{name: "detectRestatedExclusionReason", needsEvo: true, run: astRule(detectRestatedExclusionReason)},
	// API-045: Task(name) where name is a bare subject label or a generic
	// container/phase word, not one independently meaningful action.
	{name: "detectSubjectOnlyOrContainerTaskName", needsEvo: true, run: astRule(detectSubjectOnlyOrContainerTaskName)},
	// API-050: a generic phase/category-named Task (fix/check/classify/
	// resolve/finalize) sequences 2+ independently erroring steps in its own
	// Define callback — structural evidence it owns child-looking work.
	{name: "detectPhaseTaskOwningChildWork", needsEvo: true, run: astRule(detectPhaseTaskOwningChildWork)},
	// API-047: a Group and a Sequence under one parent share a literal name;
	// the second declaration is refused.
	{name: "detectDuplicateContainerSiblingName", needsEvo: true, run: astRule(detectDuplicateContainerSiblingName)},
	// API-049: Define callback discards its scheduler-provided context.
	{name: "detectDefineDiscardsSchedulerContext", needsEvo: true, run: astRule(detectDefineDiscardsSchedulerContext)},
	{name: "detectPerFindingFakeTask", needsEvo: true, run: astRule(detectPerFindingFakeTask)},
	{name: "detectFlattenedDiagnosticsLoop", needsEvo: true, run: textRule(detectFlattenedDiagnosticsLoop)},
	// API-057: a filesystem mutator call hidden inside an evo.Effect
	// callback — Effect is the opaque-mutation escape hatch, not a second
	// file-write API (ZYS-851 Decisions).
	{name: "detectFileWriteInEffectCallback", needsEvo: true, run: astRule(detectFileWriteInEffectCallback)},
	// API-058: a patch applied straight to the real workspace through
	// os/exec (`patch`, `git apply`, `git am`) instead of deriving desired
	// file states with evo.Patch, which commits them through evo.File
	// (ZYS-934).
	{name: "detectDirectWorkspacePatchApply", needsEvo: true, run: astRule(detectDirectWorkspacePatchApply)},
	// API-063: a Verify callback that returns a constant observes nothing.
	{name: "detectConstantVerify", needsEvo: true, run: astRule(detectConstantVerify)},
	// EVO-VERIFY-001: Verify callback performs a raw mutation; Verify must
	// be read-only.
	{name: "detectMutatingVerify", needsEvo: true, run: astRule(detectMutatingVerify)},
	// API-046: evo.Exclude("...") whose reason names an already-satisfied
	// condition (the Satisfied outcome, via Verify or evo.File/evo.Exec).
	{name: "detectExcludedForAlreadySatisfied", needsEvo: true, run: astRule(detectExcludedForAlreadySatisfied)},
	// API-060: Summary text that is actually mutation/dry-run/already-
	// satisfied narration rather than the caller's own result metadata.
	{name: "detectSummaryStampNarration", needsEvo: true, run: astRule(detectSummaryStampNarration)},
	// EVO-DRYRUN-001: Define callback raw-calls a side effect Evo's runtime
	// cannot intercept, breaking the dry-run guarantee.
	{name: "detectRawMutationInDefine", needsEvo: true, run: astSourceRule(detectRawMutationInDefine)},
	// EVO-DAG-001: a goroutine exists only to make Evo Tasks parallel.
	{name: "detectGoroutineWrappingDefine", needsEvo: true, run: textRule(detectGoroutineWrappingDefine)},
	// EVO-DAG-002: a chained .After(...) reproduces evo.Sequence.
	{name: "detectAfterChainDuplicatesSequence", needsEvo: true, run: astRule(detectAfterChainDuplicatesSequence)},
	// API-054: raw os/exec.Cmd wired to an Evo Task's Writer() reimplements
	// Exec's own capture/liveness/cancellation with hand-rolled
	// bytes.Buffer/io.MultiWriter plumbing or output-string cancellation
	// matching instead of inspecting the ExecResult evo.Exec now returns
	// (ZYS-850).
	{name: "detectManualSubprocessCaptureAroundTask", needsEvo: true, run: textRule(detectManualSubprocessCaptureAroundTask)},
	// API-053: a second evo.File/evo.Patch/claim-taking evo.Effect call
	// made with a context an enclosing evo.Effect already holds a Path or
	// Resource claim on (ZYS-840), directly or one call away through a
	// same-file helper.
	{name: "detectNestedResourceAcquisition", needsEvo: true, run: astRule(detectNestedResourceAcquisition)},
	// API-055: caller-managed sync.Mutex/RWMutex Lock/Unlock wrapped around
	// an evo.File call that File's automatic resource claim (ZYS-840)
	// already serializes.
	{name: "detectManualLockAroundEvoFile", needsEvo: true, run: astRule(detectManualLockAroundEvoFile)},
	// EVO-DAG-003: a visible producer/consumer relationship has no
	// first-run scheduler ordering.
	{name: "detectMissingProducerConsumerOrdering", needsEvo: true, run: astRule(detectMissingProducerConsumerOrdering)},
	// API-056: a .After(...) edge whose comment and both Tasks' own
	// resource declarations show the only reason is shared-resource
	// exclusion, not a semantic dependency, which File/FSResource/
	// LogicalResource claim coordination (ZYS-840) already provides.
	{name: "detectAfterOnlyForResourceContention", needsEvo: true, run: detectAfterOnlyForResourceContention},
	// API-039: Group that only ever has one child in source.
	{name: "detectSingletonGroup", needsEvo: true, run: astRule(detectSingletonGroup)},
	// EVO-FILE-001: manual write/chmod file reconciliation, and expensive
	// work already run before a trailing evo.File/evo.Exec return.
	{name: "detectManualFileReconciliation", needsEvo: true, run: astRule(detectManualFileReconciliation)},
	{name: "detectExpensiveWorkBeforeFileOp", needsEvo: true, run: astRule(detectExpensiveWorkBeforeFileOp)},
	// EVO-EXEC-001: raw exec guarded by a hand-rolled freshness check.
	{name: "detectRawExecWithManualFreshness", needsEvo: true, run: astRule(detectRawExecWithManualFreshness)},
	// EVO-PROVENANCE-001: a literal path visibly read or passed as a
	// literal Exec Argv entry that the call's own Inputs omit.
	{name: "detectOmittedInputPath", needsEvo: true, run: astRule(detectOmittedInputPath)},
	// DOM-011: a declined Confirm returned as an application error instead
	// of evo.Refuse.
	{name: "detectDeclinedConfirmAsError", needsEvo: true, run: astRule(detectDeclinedConfirmAsError)},
	// API-064: Computed.Get() read by work nothing orders after its producer.
	{name: "detectUnorderedComputedGet", needsEvo: true, run: astRule(detectUnorderedComputedGet)},
	// API-065: After(x) between steps of one Sequence, which already orders them.
	{name: "detectRedundantAfterInSequence", needsEvo: true, run: astRule(detectRedundantAfterInSequence)},
	// API-066: a Task result handed to another Task through an outer variable.
	{name: "detectOuterVariableHandoff", needsEvo: true, run: astRule(detectOuterVariableHandoff)},
	// API-068: child Tasks/containers declared inside a Task's Define.
	{name: "detectChildrenDeclaredInTaskDefine", needsEvo: true, run: astRule(detectChildrenDeclaredInTaskDefine)},
	// API-069: I/O, goroutines or context use in a Build callback.
	{name: "detectImpureTopologyBuilder", needsEvo: true, run: astRule(detectImpureTopologyBuilder)},
	// API-070: one Task looping over items that each deserve an outcome.
	{name: "detectCategoryTaskLoopingOverItems", needsEvo: true, run: astRule(detectCategoryTaskLoopingOverItems)},
	// API-071: a Task whose only job is to print.
	{name: "detectPresentationOnlyTask", needsEvo: true, run: astRule(detectPresentationOnlyTask)},
	// EVO-UI-004: a hand-picked ANSI color or [OK]-style status printed
	// directly. Fires without an evo import: the caller is bypassing evo.
	{name: "detectCallerChosenGlyphColorOrStatus", run: astRule(detectCallerChosenGlyphColorOrStatus)},
	// EVO-WIRE-002: a json wire struct beside an undocumented SchemaVersion.
	{name: "detectUndocumentedWireSchemaVersion", run: astRule(detectUndocumentedWireSchemaVersion)},
}
