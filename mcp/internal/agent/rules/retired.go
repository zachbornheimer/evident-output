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
	RetiredRelease2_0 RetiredRelease = "2.0"
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
	{Contract: "MainWith", RemovedIn: RetiredRelease1_0, Replacement: "evo.Main(cfg, run) or evo.Execute(ctx, cfg, run)", Taught: regexp.MustCompile(`\bMainWith\b`)},
	{Contract: "Task.Run", RemovedIn: RetiredRelease1_0, Replacement: "task.Writer() on cmd.Stdout/Stderr", Taught: regexp.MustCompile(`\b[Tt]ask\.Run\b`)},
	{Contract: "Task.Go", RemovedIn: RetiredRelease1_0, Replacement: "Task.Define", Taught: regexp.MustCompile(`\bTask\.Go\b`)},
	{Contract: "Task.Each", RemovedIn: RetiredRelease1_0, Replacement: "one Group.Task(...).Define per item", Taught: regexp.MustCompile(`\b(?:Task|Group|Sequence)\.Each\b|\.Each\(`)},
	{Contract: "DisplayGroup", RemovedIn: RetiredRelease1_0, Replacement: "Group", Taught: regexp.MustCompile(`\bDisplayGroup\b`)},
	{Contract: "Group.Done", RemovedIn: RetiredRelease1_0, Replacement: "let the Group settle from its children", Taught: regexp.MustCompile(`\bGroup\.Done\b`)},
	{Contract: "Sequence.Fail", RemovedIn: RetiredRelease1_0, Replacement: "fail the child Task", Taught: regexp.MustCompile(`\bSequence\.Fail\b`)},
	{Contract: "TaskConfig", RemovedIn: RetiredRelease1_0, Replacement: "Config", Taught: regexp.MustCompile(`\bTaskConfig\b`)},

	// ZYS-950: the TaskHandle mutation verbs and their quantity option.
	// Opaque mutations use evo.Effect; file state uses evo.File.
	{Contract: "TaskHandle.Add(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, ...}, fn)", Taught: mutationVerb("Add")},
	{Contract: "TaskHandle.Create(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, evo.EffectSpec{Verb: evo.Create, ...}, fn)", Taught: mutationVerb("Create")},
	{Contract: "TaskHandle.Delete(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, ...}, fn)", Taught: mutationVerb("Delete")},
	{Contract: "TaskHandle.Push(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, ...}, fn)", Taught: mutationVerb("Push")},
	{Contract: "TaskHandle.Remove(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, ...}, fn)", Taught: mutationVerb("Remove")},
	{Contract: "TaskHandle.Update(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, ...}, fn)", Taught: mutationVerb("Update")},
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
	{Contract: "ID", RemovedIn: RetiredRelease1_1, Replacement: "the Task name is its identity"},
	{Contract: "EntityOption", RemovedIn: RetiredRelease1_1, Replacement: "the Task name is its identity; Task.Doing for the first step"},
	{Contract: "StartPhase", RemovedIn: RetiredRelease1_1, Replacement: "Task.Doing"},
	{Contract: "TaskHandle.Warn(", RemovedIn: RetiredRelease1_1, Replacement: "Task.Problem(summary, opts...), which never ends the Task", Taught: warnTaught()},
	{Contract: "Output.Warn(", RemovedIn: RetiredRelease1_1, Replacement: "Task.Problem(summary, opts...), which never ends the Task"},
	{Contract: "Warn", RemovedIn: RetiredRelease1_1, Replacement: "Task.Problem(summary, opts...), which never ends the Task"},
	{Contract: "TaskHandle.Blockf(", RemovedIn: RetiredRelease1_1, Replacement: "return evo.Refuse(summary, opts...) from Define", Taught: regexp.MustCompile(`\bBlockf\b`)},
	{Contract: "TaskHandle.Failf(", RemovedIn: RetiredRelease1_1, Replacement: "return evo.Fail(summary, opts...) from Define", Taught: regexp.MustCompile(`\bFailf\b`)},
	{Contract: "Output.Failf(", RemovedIn: RetiredRelease1_1, Replacement: "return evo.Fail(summary, opts...) from Define"},
	{Contract: "Failure.Next(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Next(action) as an option on evo.Fail or evo.Refuse"},
	{Contract: "Failure.NextCommand(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Next(evo.Action{Command: []string{...}}) as an option on evo.Fail or evo.Refuse"},

	// ZYS-1182: a remedy belongs to the Problem it explains, so Next and
	// NextCommand exist only as ProblemOptions. The Task and Output methods
	// are gone.
	{Contract: "TaskHandle.Next(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Next(action) as an option on evo.Fail, evo.Refuse or Task.Problem", Taught: nextMethodTaught("Next")},
	{Contract: "TaskHandle.NextCommand(", RemovedIn: RetiredRelease1_1, Replacement: "evo.Next(evo.Action{Command: []string{...}}) as an option on evo.Fail, evo.Refuse or Task.Problem", Taught: nextMethodTaught("NextCommand")},
	{Contract: "Output.Next(", RemovedIn: RetiredRelease1_1, Replacement: "Task.Problem(summary, evo.Next(action)) on the Task the remedy explains"},
	{Contract: "Output.NextCommand(", RemovedIn: RetiredRelease1_1, Replacement: "Task.Problem(summary, evo.Next(evo.Action{Command: []string{...}})) on the Task the remedy explains"},
	{Contract: "TaskHandle.Step(", RemovedIn: RetiredRelease1_1, Replacement: "Task.Progress(completed, total).Doing(item)", Taught: apiSpelling("Step")},
	{Contract: "TaskHandle.Kept(", RemovedIn: RetiredRelease1_1, Replacement: "Task.Verify, which resolves the Task Satisfied without running it; Task.Fact or Task.Summary for kept counts", Taught: apiSpelling("Kept")},

	// ZYS-1185: capture-meaning Evidence* names. Satisfaction-meaning
	// EvidencePhase / TaskEvidence stay: Taught never uses a bare
	// \bEvidence\b that would match them, EvidenceTail, or "Fail evidence".
	{Contract: "EvidenceOption", RemovedIn: RetiredRelease1_1, Replacement: "Task.Writer()", Taught: regexp.MustCompile(`\bEvidenceOption\b`)},
	{Contract: "EvidenceStream", RemovedIn: RetiredRelease1_1, Replacement: "Task.Writer()", Taught: regexp.MustCompile(`\bEvidenceStream(?:Combined|Stdout|Stderr)?\b`)},
	{Contract: "EvidenceStreamCombined", RemovedIn: RetiredRelease1_1, Replacement: "Task.Writer()"},
	{Contract: "EvidenceStreamStdout", RemovedIn: RetiredRelease1_1, Replacement: "Task.Writer()"},
	{Contract: "EvidenceStreamStderr", RemovedIn: RetiredRelease1_1, Replacement: "Task.Writer()"},
	{Contract: "MaxEvidenceBytes", RemovedIn: RetiredRelease1_1, Replacement: "no knob; the tail is bounded", Taught: regexp.MustCompile(`\bMaxEvidenceBytes\b`)},
	{Contract: "Evidence", RemovedIn: RetiredRelease1_1, Replacement: "Task.Writer()", Taught: captureMeaningEvidenceTaught()},
	// ZYS-1186/1187: 1.1 vocabulary freeze. Contract names match Walk
	// identifiers (single-word uses identifier boundaries).
	{Contract: "AlsoWrite", RemovedIn: RetiredRelease1_1, Replacement: "io.MultiWriter on Config.Facades.Stdout"},
	{Contract: "ConclusionJSON", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "DataProjection", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output and Run.ResultWriter()"},
	{Contract: "DebugAddSource", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug (a slog.Level) or EVO_DEBUG"},
	{Contract: "DebugHistory", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug (a slog.Level) or EVO_DEBUG"},
	{Contract: "DebugLevel", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug (a slog.Level) or EVO_DEBUG"},
	{Contract: "DebugPane", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug (a slog.Level) or EVO_DEBUG"},
	{Contract: "Diagnostics", RemovedIn: RetiredRelease1_1, Replacement: "Config.Facades.Stderr"},
	{Contract: "EncodeEventJSON", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSONL (evo.event lines)"},
	{Contract: "EncodeJSON", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "EncodeJSONL", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSONL (evo.event lines)"},
	{Contract: "ErrReasonSkipOnly", RemovedIn: RetiredRelease1_1, Replacement: "no equivalent; Exclude takes a plain reason"},
	{Contract: "ErrReasonWrongTask", RemovedIn: RetiredRelease1_1, Replacement: "no equivalent; Exclude takes a plain reason"},
	{Contract: "EventJSON", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSONL (evo.event lines)"},
	{Contract: "EventSchemaVersion", RemovedIn: RetiredRelease1_1, Replacement: "the schemas in schema/"},
	{Contract: "ExternalProjection", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output and Run.ResultWriter()"},
	{Contract: "ForSkip", RemovedIn: RetiredRelease1_1, Replacement: "return evo.Exclude(reason, opts...) from Define"},
	{Contract: "Glyphs", RemovedIn: RetiredRelease1_1, Replacement: "no option; Evo picks the presentation from Output and Terminal"},
	{Contract: "JSONAction", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONChanges", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONCollection", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONCommand", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONDocument", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONEffectRecord", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONMessage", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONOutputMeta", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONPlan", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONProblem", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONProgress", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONSchemaVersion", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "JSONTask", RemovedIn: RetiredRelease1_1, Replacement: "Config.Output JSON (one evo.run document)"},
	{Contract: "KeepLastLines", RemovedIn: RetiredRelease1_1, Replacement: "no knob; the tail is bounded", Taught: regexp.MustCompile(`\bKeepLastLines\b`)},
	{Contract: "MaxEntities", RemovedIn: RetiredRelease1_1, Replacement: "no knob; the bounds are fixed"},
	{Contract: "MaxEvents", RemovedIn: RetiredRelease1_1, Replacement: "no knob; the bounds are fixed"},
	{Contract: "MaxFrameRate", RemovedIn: RetiredRelease1_1, Replacement: "no knob; the bounds are fixed"},
	{Contract: "MirrorToDebug", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug (a slog.Level) or EVO_DEBUG"},
	{Contract: "MirrorToDiagnostics", RemovedIn: RetiredRelease1_1, Replacement: "Config.Debug (a slog.Level) or EVO_DEBUG"},
	{Contract: "NoColor", RemovedIn: RetiredRelease1_1, Replacement: "NO_COLOR and FORCE_COLOR"},
	{Contract: "OnTask", RemovedIn: RetiredRelease1_1, Replacement: "return evo.Exclude(reason, opts...) from Define"},
	{Contract: "Option", RemovedIn: RetiredRelease1_1, Replacement: "Config fields"},
	{Contract: "ReasonOption", RemovedIn: RetiredRelease1_1, Replacement: "return evo.Exclude(reason, opts...) from Define"},
	{Contract: "Redact", RemovedIn: RetiredRelease1_1, Replacement: "Config.Facades.Redactor"},
	{Contract: "ResultStream", RemovedIn: RetiredRelease1_1, Replacement: "Run.ResultWriter()"},
	{Contract: "Runner", RemovedIn: RetiredRelease1_1, Replacement: "Config.Facades.Process"},
	{Contract: "Strict", RemovedIn: RetiredRelease1_1, Replacement: "no option; misuse is always recorded in Result with a remedy"},
	{Contract: "Title", RemovedIn: RetiredRelease1_1, Replacement: "Config.Name"},
	{Contract: "To", RemovedIn: RetiredRelease1_1, Replacement: "Config.Facades.Stdout"},
	{Contract: "VisibilityDelay", RemovedIn: RetiredRelease1_1, Replacement: "no option; Evo picks the presentation from Output and Terminal"},
	{Contract: "Width", RemovedIn: RetiredRelease1_1, Replacement: "no option; Evo picks the presentation from Output and Terminal"},

	// ZYS-1382 was not 1.2: File/Patch/Exec stay the 1.1 operations
	// (contract §31/§37). These nouns leaked on integrate/evo-1.2-candidate
	// and are not evo.
	{Contract: "Tree", RemovedIn: RetiredRelease1_2, Replacement: "evo.File / evo.Effect", Taught: regexp.MustCompile(`\bevo\.Tree\b`)},
	{Contract: "Find", RemovedIn: RetiredRelease1_2, Replacement: "caller-owned discovery, then evo.File", Taught: regexp.MustCompile(`\bevo\.Find\b`)},
	{Contract: "Download", RemovedIn: RetiredRelease1_2, Replacement: "caller fetch, then evo.File", Taught: regexp.MustCompile(`\bevo\.Download\b`)},
	{Contract: "Extract", RemovedIn: RetiredRelease1_2, Replacement: "caller unpack, then evo.File", Taught: regexp.MustCompile(`\bevo\.Extract\b`)},
	{Contract: "Clone", RemovedIn: RetiredRelease1_2, Replacement: "caller copy, then evo.File", Taught: regexp.MustCompile(`\bevo\.Clone\b`)},
	{Contract: "Checksum", RemovedIn: RetiredRelease1_2, Replacement: "not evo", Taught: regexp.MustCompile(`\bevo\.Checksum\b|\.Checksum\(`)},

	// 2.0 (CONTRACT.md): the surface is exactly the contract tables. These
	// 1.x names are gone.
	{Contract: "Init", RemovedIn: RetiredRelease2_0, Replacement: "evo.Main(cfg, run) or evo.Execute(ctx, cfg, run)", Taught: regexp.MustCompile(`\bInit\b`)},
	{Contract: "Output.Run", RemovedIn: RetiredRelease2_0, Replacement: "evo.Execute(ctx, cfg, run)", Taught: regexp.MustCompile(`\b(?:out|output|Output)\.Run\b`)},
	{Contract: "Isolated", RemovedIn: RetiredRelease2_0, Replacement: "evo.Execute(ctx, cfg, run) with its own Config", Taught: regexp.MustCompile(`\bIsolated\b`)},
	{Contract: "Output.Finish", RemovedIn: RetiredRelease2_0, Replacement: "return from run; Main and Execute finish the run", Taught: regexp.MustCompile(`\b(?:out|output|Output)\.Finish\b|\bFinish \+ Close\b|` + "`Finish`")},
	{Contract: "Output.Close", RemovedIn: RetiredRelease2_0, Replacement: "return from run; Main and Execute close the run", Taught: regexp.MustCompile(`\b(?:out|output|Output)\.Close\b|\bFinish \+ Close\b|` + "`Close`")},
	{Contract: "Println", RemovedIn: RetiredRelease2_0, Replacement: "fmt.Fprintln(r.Text(), ...)", Taught: regexp.MustCompile(`\b(?:evo|out|Output)\.Print(?:f|ln)?\b|(?:^|[^.\w|])Print(?:f|ln)?\(`)},
	{Contract: "Block", RemovedIn: RetiredRelease2_0, Replacement: "return evo.Refuse(summary, opts...) from Define", Taught: regexp.MustCompile(`(?:^|[^.\w|])Block\(|\.Block\(|\bTask\.Block\b|\bBlock,\s+or\s+Skipped\b`)},
	{Contract: "Wait", RemovedIn: RetiredRelease2_0, Replacement: "return from run; Evo owns the waiting", Taught: regexp.MustCompile(`\b(?:[Tt]ask\w*|[Gg]roup|[Ss]equence|seq)\.Wait\b|\)\.Wait\(\)`)},
	{Contract: "Key", RemovedIn: RetiredRelease2_0, Replacement: "the Task name is its identity", Taught: regexp.MustCompile(`\b(?:[Tt]ask\w*)\.Key\b`)},
	{Contract: "TaskHandle", RemovedIn: RetiredRelease2_0, Replacement: "Task", Taught: regexp.MustCompile(`\b(?:Task|Group|Sequence)Handle\b`)},
	{Contract: "Files", RemovedIn: RetiredRelease2_0, Replacement: "evo.File per file"},
	{Contract: "Fingerprint", RemovedIn: RetiredRelease2_0, Replacement: "FileSpec/ExecSpec Inputs and Values"},
	{Contract: "FSPath", RemovedIn: RetiredRelease2_0, Replacement: "EffectSpec.Path"},
	{Contract: "Value", RemovedIn: RetiredRelease2_0, Replacement: "FileSpec/ExecSpec Values"},
	{Contract: "App", RemovedIn: RetiredRelease2_0, Replacement: "State.AppID"},
	{Contract: "Location", RemovedIn: RetiredRelease2_0, Replacement: "evo.On(\"path:line:col\")"},
	{Contract: "Severity", RemovedIn: RetiredRelease2_0, Replacement: "Problem is always non-fatal; there is no severity"},
	{Contract: "SeverityWarning", RemovedIn: RetiredRelease2_0, Replacement: "Task.Problem"},
	{Contract: "ColorNever", RemovedIn: RetiredRelease2_0, Replacement: "NO_COLOR"},
	{Contract: "FormatData", RemovedIn: RetiredRelease2_0, Replacement: "Run.ResultWriter()"},
	{Contract: "EffectAdd", RemovedIn: RetiredRelease2_0, Replacement: "evo.Create", Taught: regexp.MustCompile(`\bEffectAdd\b`)},
	{Contract: "EffectCreate", RemovedIn: RetiredRelease2_0, Replacement: "evo.Create", Taught: regexp.MustCompile(`\bEffectCreate\b`)},
	{Contract: "EffectDelete", RemovedIn: RetiredRelease2_0, Replacement: "evo.Delete", Taught: regexp.MustCompile(`\bEffectDelete\b`)},
	{Contract: "EffectPush", RemovedIn: RetiredRelease2_0, Replacement: "evo.Update", Taught: regexp.MustCompile(`\bEffectPush\b`)},
	{Contract: "EffectRemove", RemovedIn: RetiredRelease2_0, Replacement: "evo.Delete", Taught: regexp.MustCompile(`\bEffectRemove\b`)},
	{Contract: "EffectUpdate", RemovedIn: RetiredRelease2_0, Replacement: "evo.Update", Taught: regexp.MustCompile(`\bEffectUpdate\b`)},
	{Contract: "EffectInstall", RemovedIn: RetiredRelease2_0, Replacement: "evo.Create", Taught: regexp.MustCompile(`\bEffectInstall\b`)},
	{Contract: "EffectUninstall", RemovedIn: RetiredRelease2_0, Replacement: "evo.Delete", Taught: regexp.MustCompile(`\bEffectUninstall\b`)},
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

// nextMethodTaught matches the removed Task/Output remedy method taught as
// current API: a call on a task or output receiver (task.Next(...),
// out.NextCommand(...)) or a method signature. It does not match the live
// evo.Next / evo.NextCommand ProblemOptions, nor unrelated iterators.
func nextMethodTaught(name string) *regexp.Regexp {
	return regexp.MustCompile(`\b(?:[Tt]ask\w*|item|it|out|output|Output|TaskHandle)\.` + name + `\(` +
		`|func \((?:t|o) \*(?:TaskHandle|Output)\) ` + name + `\(`)
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
		"|" + mutationVerbList(verb))
}

// mutationVerbList matches verb as a member of a removed-verb list
// ("Delete/Create", "Write/Remove").
func mutationVerbList(verb string) string {
	const (
		start = "(?:^|[\\s(`/])"
		sep   = "`?/`?"
	)
	return start + verb + sep + removedMutationVerbs + `\b` +
		"|" + start + removedMutationVerbs + sep + verb + `\b`
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
