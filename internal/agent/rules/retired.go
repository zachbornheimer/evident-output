// retired.go is the one table of Evident Output API names that were
// removed, or that the API never had. The API contract check, the docs
// stale-API test, and the MCP rule catalog all read it, so a retired name
// can never come back as live surface or as guidance.

package rules

import (
	"regexp"
	"slices"
)

// RetiredRelease names the release that removed a Symbol.
type RetiredRelease string

// Releases that removed API.
const (
	RetiredRelease1_0 RetiredRelease = "1.0"
	RetiredRelease1_1 RetiredRelease = "1.1"
	RetiredRelease1_2 RetiredRelease = "1.2"
)

// RetiredSymbol is one retired API name.
type RetiredSymbol struct {
	// Contract is the spelling the API contract matches against the live
	// exported surface ("TaskHandle.Record(" — the trailing "(" keeps
	// "TaskHandle.Write(" from matching Writer).
	Contract string
	// RemovedIn is the release that removed it. Names the API never had
	// (speculative spellings from drafts or other libraries) count as
	// removed in 1.0, the first frozen release.
	RemovedIn RetiredRelease
	// Replacement is what to write instead.
	Replacement string
	// Taught matches text that teaches the name as current API: docs, rule
	// GoodCode/Remediation, detector suggestions. Nil means the name only
	// matters to the contract check.
	Taught *regexp.Regexp
}

var retiredSymbols = []RetiredSymbol{
	{Contract: "MainWith", RemovedIn: RetiredRelease1_0, Replacement: "evo.Main / Output.Run", Taught: regexp.MustCompile(`\bMainWith\b`)},
	{Contract: "Task.Run", RemovedIn: RetiredRelease1_0, Replacement: "task.Writer() on cmd.Stdout/Stderr", Taught: regexp.MustCompile(`\b[Tt]ask\.Run\b`)},
	{Contract: "Task.Go", RemovedIn: RetiredRelease1_0, Replacement: "Task.Define", Taught: regexp.MustCompile(`\bTask\.Go\b`)},
	{Contract: "Task.Each", RemovedIn: RetiredRelease1_0, Replacement: "one Group.Task(...).Define per item", Taught: regexp.MustCompile(`\b(?:Task|Group|Sequence)\.Each\b|\.Each\(`)},
	{Contract: "DisplayGroup", RemovedIn: RetiredRelease1_0, Replacement: "Group", Taught: regexp.MustCompile(`\bDisplayGroup\b`)},
	{Contract: "Group.Done", RemovedIn: RetiredRelease1_0, Replacement: "let the Group settle from its children", Taught: regexp.MustCompile(`\bGroup\.Done\b`)},
	{Contract: "Sequence.Fail", RemovedIn: RetiredRelease1_0, Replacement: "fail the child Task", Taught: regexp.MustCompile(`\bSequence\.Fail\b`)},
	{Contract: "TaskConfig", RemovedIn: RetiredRelease1_0, Replacement: "Config", Taught: regexp.MustCompile(`\bTaskConfig\b`)},

	// ZYS-950: the TaskHandle mutation verbs and their quantity option.
	// Opaque mutations use evo.Effect; file state uses evo.File.
	{Contract: "TaskHandle.Add(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectAdd, ...}, fn)", Taught: mutationVerb("Add")},
	{Contract: "TaskHandle.Create(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectCreate, ...}, fn)", Taught: mutationVerb("Create")},
	{Contract: "TaskHandle.Delete(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectDelete, ...}, fn)", Taught: mutationVerb("Delete")},
	{Contract: "TaskHandle.Push(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectPush, ...}, fn)", Taught: mutationVerb("Push")},
	{Contract: "TaskHandle.Remove(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectRemove, ...}, fn)", Taught: mutationVerb("Remove")},
	{Contract: "TaskHandle.Update(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectUpdate, ...}, fn)", Taught: mutationVerb("Update")},
	{Contract: "TaskHandle.Write(", RemovedIn: RetiredRelease1_1, Replacement: "evo.File for file state, evo.Effect otherwise", Taught: mutationVerb("Write")},
	{Contract: "Affected", RemovedIn: RetiredRelease1_1, Replacement: "EffectSpec.Quantity", Taught: regexp.MustCompile(`\bAffected\(`)},
	{Contract: "MutationOption", RemovedIn: RetiredRelease1_1, Replacement: "EffectSpec", Taught: regexp.MustCompile(`\bMutationOption\b`)},

	// ZYS-812: the Done success stamp and the record-only ledger verbs.
	// Success resolves through Define (result text is Summary); mutations
	// go through Effect, information through Fact.
	{Contract: "TaskHandle.Done(", RemovedIn: RetiredRelease1_1, Replacement: "Define (result text via Summary)", Taught: regexp.MustCompile(`\b(?:[Tt]ask\w*|TaskHandle)\.Done\(|\bTask\([^)]*\)\.Done\(|/Done\b`)},
	{Contract: "TaskHandle.Record(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect for a mutation, Fact for information", Taught: regexp.MustCompile(`\b[Tt]ask\w*\.Record\(|\bTaskHandle\.Record\b`)},
	{Contract: "TaskHandle.RecordLabel(", RemovedIn: RetiredRelease1_1, Replacement: "Fact", Taught: regexp.MustCompile(`\bRecordLabel\(`)},
	{Contract: "TaskHandle.RecordName(", RemovedIn: RetiredRelease1_1, Replacement: "Fact", Taught: regexp.MustCompile(`\bRecordName\(`)},

	// Task is name-only, so nothing accepted an EntityOption: ID and
	// StartPhase built values no API consumed (PHIL-007).
	{Contract: "ID", RemovedIn: RetiredRelease1_1, Replacement: "TaskHandle.Key"},
	{Contract: "EntityOption", RemovedIn: RetiredRelease1_1, Replacement: "TaskHandle.Key for identity, Doing for the first step"},
	{Contract: "StartPhase", RemovedIn: RetiredRelease1_1, Replacement: "Doing"},
	{Contract: "TaskHandle.Warn(", RemovedIn: RetiredRelease1_1, Replacement: "Problem(summary, Severity(SeverityWarning))", Taught: warnTaught()},
	{Contract: "Output.Warn(", RemovedIn: RetiredRelease1_1, Replacement: "Problem(summary, Severity(SeverityWarning))"},
	{Contract: "Warn", RemovedIn: RetiredRelease1_1, Replacement: "Problem(summary, Severity(SeverityWarning))"},
	{Contract: "TaskHandle.Blockf(", RemovedIn: RetiredRelease1_1, Replacement: "Block(summary, opts...)", Taught: regexp.MustCompile(`\bBlockf\b`)},
	{Contract: "TaskHandle.Failf(", RemovedIn: RetiredRelease1_1, Replacement: "Fail(summary, opts...)", Taught: regexp.MustCompile(`\bFailf\b`)},
	{Contract: "Output.Failf(", RemovedIn: RetiredRelease1_1, Replacement: "Fail(summary, opts...)"},
	{Contract: "Failure.Next(", RemovedIn: RetiredRelease1_1, Replacement: "TaskHandle.Next after Fail or Block"},
	{Contract: "Failure.NextCommand(", RemovedIn: RetiredRelease1_1, Replacement: "TaskHandle.NextCommand after Fail or Block"},
	{Contract: "TaskHandle.Step(", RemovedIn: RetiredRelease1_1, Replacement: "Progress(completed, total).Doing(item)", Taught: apiSpelling("Step")},
	{Contract: "TaskHandle.Kept(", RemovedIn: RetiredRelease1_1, Replacement: "Skipped(Reason(...)) for a policy exclusion; Fact/Summary for kept counts", Taught: apiSpelling("Kept")},

	// ZYS-1185: capture-meaning Evidence* names. Satisfaction-meaning
	// EvidencePhase / TaskEvidence stay: Taught never uses a bare
	// \bEvidence\b that would match them, EvidenceTail, or "Fail evidence".
	{Contract: "EvidenceOption", RemovedIn: RetiredRelease1_1, Replacement: "CaptureOption", Taught: regexp.MustCompile(`\bEvidenceOption\b`)},
	{Contract: "EvidenceStream", RemovedIn: RetiredRelease1_1, Replacement: "CaptureStream", Taught: regexp.MustCompile(`\bEvidenceStream(?:Combined|Stdout|Stderr)?\b`)},
	{Contract: "EvidenceStreamCombined", RemovedIn: RetiredRelease1_1, Replacement: "CaptureStreamCombined"},
	{Contract: "EvidenceStreamStdout", RemovedIn: RetiredRelease1_1, Replacement: "CaptureStreamStdout"},
	{Contract: "EvidenceStreamStderr", RemovedIn: RetiredRelease1_1, Replacement: "CaptureStreamStderr"},
	{Contract: "MaxEvidenceBytes", RemovedIn: RetiredRelease1_1, Replacement: "MaxCaptureBytes", Taught: regexp.MustCompile(`\bMaxEvidenceBytes\b`)},
	{Contract: "Evidence", RemovedIn: RetiredRelease1_1, Replacement: "Capture", Taught: captureMeaningEvidenceTaught()},
	// ZYS-1186/1187: 1.1 vocabulary freeze. Contract names match Walk
	// identifiers (single-word uses identifier boundaries).
	{Contract: "AlsoWrite", RemovedIn: RetiredRelease1_1, Replacement: "io.MultiWriter on Config.Stdout"},
	{Contract: "Clock", RemovedIn: RetiredRelease1_1, Replacement: "Config.Clock"},
	{Contract: "ConclusionJSON", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "DataProjection", RemovedIn: RetiredRelease1_1, Replacement: "Config.Format = FormatData"},
	{Contract: "DebugAddSource", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug.AddSource"},
	{Contract: "DebugHistory", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug.View = DebugPresentationHistory"},
	{Contract: "DebugLevel", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug.Level"},
	{Contract: "DebugPane", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug.View = DebugPresentationPane"},
	{Contract: "Diagnostics", RemovedIn: RetiredRelease1_1, Replacement: "Config.Stderr"},
	{Contract: "DryRun", RemovedIn: RetiredRelease1_1, Replacement: "Config.DryRun"},
	{Contract: "EncodeEventJSON", RemovedIn: RetiredRelease1_1, Replacement: "FormatJSONL / WriteJSON"},
	{Contract: "EncodeJSON", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "EncodeJSONL", RemovedIn: RetiredRelease1_1, Replacement: "FormatJSONL"},
	{Contract: "ErrReasonSkipOnly", RemovedIn: RetiredRelease1_1, Replacement: "Reason without ForSkip"},
	{Contract: "ErrReasonWrongTask", RemovedIn: RetiredRelease1_1, Replacement: "Reason without OnTask"},
	{Contract: "EventJSON", RemovedIn: RetiredRelease1_1, Replacement: "FormatJSONL"},
	{Contract: "EventSchemaVersion", RemovedIn: RetiredRelease1_1, Replacement: "wire EventSchemaVersion"},
	{Contract: "ExternalProjection", RemovedIn: RetiredRelease1_1, Replacement: "Config.Format = FormatExternal"},
	{Contract: "ForSkip", RemovedIn: RetiredRelease1_1, Replacement: "Reason name plus Skipped"},
	{Contract: "Glyphs", RemovedIn: RetiredRelease1_1, Replacement: "Config.Glyphs"},
	{Contract: "JSONAction", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONChanges", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONCollection", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONCommand", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONDocument", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONEffectRecord", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONMessage", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONOutputMeta", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONPlan", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONProblem", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONProgress", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONSchemaVersion", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "JSONTask", RemovedIn: RetiredRelease1_1, Replacement: "WriteJSON / FormatJSON"},
	{Contract: "KeepLastLines", RemovedIn: RetiredRelease1_1, Replacement: "MaxCaptureBytes", Taught: regexp.MustCompile(`\bKeepLastLines\b`)},
	{Contract: "MaxEntities", RemovedIn: RetiredRelease1_1, Replacement: "Config.MaxEntities"},
	{Contract: "MaxEvents", RemovedIn: RetiredRelease1_1, Replacement: "Config.MaxEvents"},
	{Contract: "MaxFrameRate", RemovedIn: RetiredRelease1_1, Replacement: "Config.MaxFrameRate"},
	{Contract: "MirrorToDebug", RemovedIn: RetiredRelease1_1, Replacement: "Capture without mirror options"},
	{Contract: "MirrorToDiagnostics", RemovedIn: RetiredRelease1_1, Replacement: "Capture without mirror options"},
	{Contract: "NoColor", RemovedIn: RetiredRelease1_1, Replacement: "Config.Color = ColorNever"},
	{Contract: "OnTask", RemovedIn: RetiredRelease1_1, Replacement: "Reason name plus Skipped"},
	{Contract: "Option", RemovedIn: RetiredRelease1_1, Replacement: "Config fields"},
	{Contract: "Plain", RemovedIn: RetiredRelease1_1, Replacement: "Config.Plain"},
	{Contract: "ReasonOption", RemovedIn: RetiredRelease1_1, Replacement: "Reason(name)"},
	{Contract: "Redact", RemovedIn: RetiredRelease1_1, Replacement: "Config.Redactor"},
	{Contract: "ResultStream", RemovedIn: RetiredRelease1_1, Replacement: "Config.Result"},
	{Contract: "Runner", RemovedIn: RetiredRelease1_1, Replacement: "Config.ProcessRunner"},
	{Contract: "Stdin", RemovedIn: RetiredRelease1_1, Replacement: "Config.Stdin"},
	{Contract: "Strict", RemovedIn: RetiredRelease1_1, Replacement: "Config.Strict"},
	{Contract: "Terminal", RemovedIn: RetiredRelease1_1, Replacement: "Config.Terminal"},
	{Contract: "Title", RemovedIn: RetiredRelease1_1, Replacement: "Config.Title"},
	{Contract: "To", RemovedIn: RetiredRelease1_1, Replacement: "Config.Stdout"},
	{Contract: "VisibilityDelay", RemovedIn: RetiredRelease1_1, Replacement: "Config.VisibilityDelay"},
	{Contract: "Width", RemovedIn: RetiredRelease1_1, Replacement: "Config.Width"},

	// ZYS-1382: File, Tree, and Exec are plain structs; Basis is Task
	// freshness; Patch applies directly. Contract-only until the MCP
	// rules teach the new shapes.
	{Contract: "FileSpec", RemovedIn: RetiredRelease1_2, Replacement: "evo.File{Path, Content, Mode}.Write(ctx)"},
	{Contract: "ExecSpec", RemovedIn: RetiredRelease1_2, Replacement: "evo.Exec{Path, Args, Dir, Env, Outputs}.Run(ctx)"},
	{Contract: "FSPath", RemovedIn: RetiredRelease1_2, Replacement: "task.Basis(evo.File{Path: p}) or evo.Tree{Path: p}"},
	{Contract: "FileSet", RemovedIn: RetiredRelease1_2, Replacement: "evo.Patch(ctx, diff) error"},
	{Contract: "Files", RemovedIn: RetiredRelease1_2, Replacement: "evo.Patch(ctx, diff) error"},
	{Contract: "ErrStaleBasis", RemovedIn: RetiredRelease1_2, Replacement: "ErrPatchStale"},
	{Contract: "ErrFileSpecMissingPath", RemovedIn: RetiredRelease1_2, Replacement: "ErrPathMissing"},
	{Contract: "ErrFileUnmanagedContentsMissing", RemovedIn: RetiredRelease1_2, Replacement: "ErrContentMissing"},
	{Contract: "ErrExecSpecMissingExecutable", RemovedIn: RetiredRelease1_2, Replacement: "ErrExecPathMissing"},
	{Contract: "ErrPatchDeleteUnsupported", RemovedIn: RetiredRelease1_2, Replacement: "Patch applies deletes"},
	{Contract: "ErrPatchRenameUnsupported", RemovedIn: RetiredRelease1_2, Replacement: "Patch applies renames"},
}

// apiSpelling matches a retired method taught as current API: a known evo
// receiver (task.Warn), a call (.Warn(), `Warn`). It does not match the
// English word in unrelated prose.
func apiSpelling(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?:task|evo|Output|TaskHandle|Task)\.` + name + `\b` +
		`|\.` + name + `\(` +
		"|`" + name + "`")
}

// warnTaught is apiSpelling("Warn") plus listings that teach Warn as a live
// outcome without a receiver: a method signature, a heading, a table cell,
// or a Done/Warn/… resolver list. It does not match English "warn" or
// slog.Logger.Warn.
func warnTaught() *regexp.Regexp {
	return regexp.MustCompile(`(?:task|evo|Output|TaskHandle|Task|Item|item)\.Warn\b` +
		"|`Warn`" +
		`|func \(.*\) Warn\(` +
		`|## Warn\b` +
		`|\*\*Warn\*\*` +
		`|Done/Warn`)
}

// captureMeaningEvidenceTaught matches Evidence as the capture sink or
// public type. It does not match satisfaction-meaning EvidencePhase /
// TaskEvidence, Problem.EvidenceTail, or the English phrase "for Fail
// evidence".
func captureMeaningEvidenceTaught() *regexp.Regexp {
	return regexp.MustCompile(`(?:task|TaskHandle|Task)\.Evidence\(` +
		`|type Evidence struct` +
		`|## Evidence\b` +
		`|Evidence ownership` +
		`|Evidence belongs` +
		"|`Evidence`," +
		`|\[\]Evidence\b`)
}

// mutationVerb matches a removed TaskHandle mutation verb taught as prose
// (Task.Delete), as a call (task.Delete("worktree", fn)), as the
// quantity-first signature (Delete(object, fn)), or as a member of a verb
// list ("`Add`/`Delete`/…", "Delete/Create/Update").
func mutationVerb(verb string) *regexp.Regexp {
	return regexp.MustCompile(`\bTask\.` + verb + `\b` +
		`|\.` + verb + `\("[^"]*",\s*(?:func|fn|nil|[a-z]\w*\))` +
		`|\b` + verb + `\(object, fn` +
		"|(?:^|[\\s(`/])" + verb + "`?/`?" + removedMutationVerbs + `\b`)
}

// removedMutationVerbs matches any removed TaskHandle mutation or record
// verb, so a verb list is recognized by its neighbor ("Delete/Create")
// while an unrelated pair ("Write/WriteString") is not.
const removedMutationVerbs = `(?:Add|Create|Delete|Push|Record|Remove|Update|Write)`

// RetiredSymbols returns every retired name, in table order.
func RetiredSymbols() []RetiredSymbol { return slices.Clone(retiredSymbols) }

// RetiredContractNames returns every Symbol's Contract spelling.
func RetiredContractNames() []string {
	names := make([]string, len(retiredSymbols))
	for i, s := range retiredSymbols {
		names[i] = s.Contract
	}
	return names
}

// TaughtRetired returns every retired Symbol text teaches, with the matched
// spelling.
func TaughtRetired(text string) []RetiredHit {
	var hits []RetiredHit
	for _, s := range retiredSymbols {
		if s.Taught == nil {
			continue
		}
		if m := s.Taught.FindString(text); m != "" {
			hits = append(hits, RetiredHit{Symbol: s, Match: m})
		}
	}
	return hits
}

// RetiredHit is one retired Symbol found in text.
type RetiredHit struct {
	Symbol RetiredSymbol
	Match  string
}
