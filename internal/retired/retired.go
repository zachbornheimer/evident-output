// Package retired is the one table of Evident Output API names that were
// removed, or that the API never had. The API contract check, the docs
// stale-API test, and the MCP rule catalog all read it, so a retired name
// can never come back as live surface or as guidance.
package retired

import (
	"regexp"
	"slices"
)

// Release names the release that removed a Symbol.
type Release string

// Releases that removed API.
const (
	Release1_0 Release = "1.0"
	Release1_1 Release = "1.1"
)

// CaptureRename is one capture-meaning Evidence* name removed in 1.1
// (E-121, ZYS-1180 freeze) and its Capture-vocabulary replacement.
// rules_capture.go (the MCP migration rules) and review_capture_rename.go
// (the structural detector) both derive from this table instead of
// keeping their own copies, so the three cannot drift apart.
type CaptureRename struct {
	// RuleID is the MCP migration rule that teaches this rename.
	RuleID string
	// From is the removed capture-meaning Evidence* spelling.
	From string
	// To is the 1.1 Capture-vocabulary replacement.
	To string
	// ProblemField marks the one rename that is a struct field on
	// evo.Problem (EvidenceTail -> CaptureTail) rather than a package-level
	// selector. The fix analyzer checks this flag instead of hard-coding
	// the rename's RuleID, so a future Problem-field rename added to this
	// table gets the receiver-scoped check for free and a package-level
	// rename can never accidentally collide with Problem.Evidence, the
	// live satisfaction-proof field that shares API-110's spelling.
	ProblemField bool
}

// CaptureRenames is the one table of capture-meaning renames.
var CaptureRenames = []CaptureRename{
	{RuleID: "API-110", From: "Evidence", To: "Capture"},
	{RuleID: "API-111", From: "EvidenceOption", To: "CaptureOption"},
	{RuleID: "API-112", From: "EvidenceStream", To: "CaptureStream"},
	{RuleID: "API-113", From: "EvidenceStreamCombined", To: "CaptureStreamCombined"},
	{RuleID: "API-114", From: "EvidenceStreamStdout", To: "CaptureStreamStdout"},
	{RuleID: "API-115", From: "EvidenceStreamStderr", To: "CaptureStreamStderr"},
	{RuleID: "API-116", From: "MaxEvidenceBytes", To: "MaxCaptureBytes"},
	{RuleID: "API-117", From: "EvidenceTail", To: "CaptureTail", ProblemField: true},
}

// captureRenameSymbols converts CaptureRenames into retired Symbol entries
// for the API contract check and the docs stale-API scan.
func captureRenameSymbols() []Symbol {
	out := make([]Symbol, len(CaptureRenames))
	for i, r := range CaptureRenames {
		// "Evidence" alone is scoped to evo.Evidence: the bare word is
		// ordinary English prose ("the evidence is...") everywhere else in
		// the docs corpus, unlike the EvidenceStream*/MaxEvidenceBytes
		// compounds, which are unambiguous.
		taught := `\b` + r.From + `\b`
		if r.From == "Evidence" {
			taught = `\bevo\.Evidence\b`
		}
		out[i] = Symbol{
			Contract:    r.From,
			RemovedIn:   Release1_1,
			Replacement: r.To,
			Taught:      regexp.MustCompile(taught),
		}
	}
	return out
}

// Symbol is one retired API name.
type Symbol struct {
	// Contract is the spelling the API contract matches against the live
	// exported surface ("TaskHandle.Record(" — the trailing "(" keeps
	// "TaskHandle.Write(" from matching Writer).
	Contract string
	// RemovedIn is the release that removed it. Names the API never had
	// (speculative spellings from drafts or other libraries) count as
	// removed in 1.0, the first frozen release.
	RemovedIn Release
	// Replacement is what to write instead.
	Replacement string
	// Taught matches text that teaches the name as current API: docs, rule
	// GoodCode/Remediation, detector suggestions. Nil means the name only
	// matters to the contract check.
	Taught *regexp.Regexp
}

var symbols = []Symbol{
	{Contract: "MainWith", RemovedIn: Release1_0, Replacement: "evo.Main / Output.Run", Taught: regexp.MustCompile(`\bMainWith\b`)},
	{Contract: "Task.Run", RemovedIn: Release1_0, Replacement: "task.Writer() on cmd.Stdout/Stderr", Taught: regexp.MustCompile(`\b[Tt]ask\.Run\b`)},
	{Contract: "Task.Go", RemovedIn: Release1_0, Replacement: "Task.Define", Taught: regexp.MustCompile(`\bTask\.Go\b`)},
	{Contract: "Task.Each", RemovedIn: Release1_0, Replacement: "one Group.Task(...).Define per item", Taught: regexp.MustCompile(`\b(?:Task|Group|Sequence)\.Each\b|\.Each\(`)},
	{Contract: "DisplayGroup", RemovedIn: Release1_0, Replacement: "Group", Taught: regexp.MustCompile(`\bDisplayGroup\b`)},
	{Contract: "Group.Done", RemovedIn: Release1_0, Replacement: "let the Group settle from its children", Taught: regexp.MustCompile(`\bGroup\.Done\b`)},
	{Contract: "Sequence.Fail", RemovedIn: Release1_0, Replacement: "fail the child Task", Taught: regexp.MustCompile(`\bSequence\.Fail\b`)},
	{Contract: "TaskConfig", RemovedIn: Release1_0, Replacement: "Config", Taught: regexp.MustCompile(`\bTaskConfig\b`)},

	// ZYS-950: the TaskHandle mutation verbs and their quantity option.
	// Opaque mutations use evo.Effect; file state uses evo.File.
	{Contract: "TaskHandle.Add(", RemovedIn: Release1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectAdd, ...}, fn)", Taught: mutationVerb("Add")},
	{Contract: "TaskHandle.Create(", RemovedIn: Release1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectCreate, ...}, fn)", Taught: mutationVerb("Create")},
	{Contract: "TaskHandle.Delete(", RemovedIn: Release1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectDelete, ...}, fn)", Taught: mutationVerb("Delete")},
	{Contract: "TaskHandle.Push(", RemovedIn: Release1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectPush, ...}, fn)", Taught: mutationVerb("Push")},
	{Contract: "TaskHandle.Remove(", RemovedIn: Release1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectRemove, ...}, fn)", Taught: mutationVerb("Remove")},
	{Contract: "TaskHandle.Update(", RemovedIn: Release1_1, Replacement: "evo.Effect(ctx, EffectSpec{Verb: EffectUpdate, ...}, fn)", Taught: mutationVerb("Update")},
	{Contract: "TaskHandle.Write(", RemovedIn: Release1_1, Replacement: "evo.File for file state, evo.Effect otherwise", Taught: mutationVerb("Write")},
	{Contract: "Affected", RemovedIn: Release1_1, Replacement: "EffectSpec.Quantity", Taught: regexp.MustCompile(`\bAffected\(`)},
	{Contract: "MutationOption", RemovedIn: Release1_1, Replacement: "EffectSpec", Taught: regexp.MustCompile(`\bMutationOption\b`)},

	// ZYS-812: the Done success stamp and the record-only ledger verbs.
	// Success resolves through Define (result text is Summary); mutations
	// go through Effect, information through Fact.
	{Contract: "TaskHandle.Done(", RemovedIn: Release1_1, Replacement: "Define (result text via Summary)", Taught: regexp.MustCompile(`\b(?:[Tt]ask\w*|TaskHandle)\.Done\(|\bTask\([^)]*\)\.Done\(|/Done\b`)},
	{Contract: "TaskHandle.Record(", RemovedIn: Release1_1, Replacement: "evo.Effect for a mutation, Fact for information", Taught: regexp.MustCompile(`\b[Tt]ask\w*\.Record\(|\bTaskHandle\.Record\b`)},
	{Contract: "TaskHandle.RecordLabel(", RemovedIn: Release1_1, Replacement: "Fact", Taught: regexp.MustCompile(`\bRecordLabel\(`)},
	{Contract: "TaskHandle.RecordName(", RemovedIn: Release1_1, Replacement: "Fact", Taught: regexp.MustCompile(`\bRecordName\(`)},

	// Owner vocabulary freeze: a warning is a Problem severity, not a
	// second verb. One entry covers TaskHandle.Warn, Output.Warn, and
	// the package-level evo.Warn; slog's Logger.Warn is not evo API.
	{Contract: "Warn", RemovedIn: Release1_1, Replacement: "Problem(summary, evo.Severity(evo.SeverityWarning))", Taught: warnTaught},

	// Task is name-only, so nothing accepted an EntityOption: ID and
	// StartPhase built values no API consumed (PHIL-007).
	{Contract: "ID", RemovedIn: Release1_1, Replacement: "TaskHandle.Key"},
	{Contract: "EntityOption", RemovedIn: Release1_1, Replacement: "TaskHandle.Key for identity, Doing for the first step"},
	{Contract: "StartPhase", RemovedIn: Release1_1, Replacement: "Doing"},

	// Owner vocabulary freeze (2026-09-25): Fail/Block are the one
	// statement-form spelling; Blockf/Failf/Failure were compatibility
	// sugar around a same-line %w-wrapped return, with no mechanical
	// rewrite (see internal/agent/fix/failf.go, API-140).
	{Contract: "TaskHandle.Failf(", RemovedIn: Release1_1, Replacement: "Fail(summary) — fold the wrapped error into the summary string, then return it separately"},
	{Contract: "TaskHandle.Blockf(", RemovedIn: Release1_1, Replacement: "Block(summary) — fold the wrapped error into the summary string, then return it separately"},
	{Contract: "Output.Failf(", RemovedIn: Release1_1, Replacement: "Output.Fail — fold the wrapped error into the summary string, then return it separately"},
	{Contract: "Failure", RemovedIn: Release1_1, Replacement: "a plain error, with Next/NextCommand ProblemOptions for a remedy"},

	// Owner vocabulary freeze (2026-09-25): Progress+Doing win over Step;
	// a kept item is domain information (Fact), not a third resolution;
	// ForSkip/OnTask restricted where a Reason could be used, a
	// constraint Reason never needed to enforce structurally.
	{Contract: "TaskHandle.Step(", RemovedIn: Release1_1, Replacement: "Progress(completed, total).Doing(name)"},
	{Contract: "TaskHandle.Kept(", RemovedIn: Release1_1, Replacement: `Fact("kept", reason.Name())`},
	{Contract: "ForSkip", RemovedIn: Release1_1, Replacement: "none: evo.Reason takes only its name"},
	{Contract: "OnTask", RemovedIn: Release1_1, Replacement: "none: evo.Reason takes only its name"},
	{Contract: "ReasonOption", RemovedIn: Release1_1, Replacement: "none: evo.Reason takes only its name"},

	// Owner vocabulary freeze (2026-09-25): Config already carries every
	// setting as a field; the parallel functional-options surface
	// (Config.Options []Option, and each Option-returning constructor)
	// never reached the public root package — only internal/engine keeps
	// them, for the fixer/reviewer's own rewrite machinery.
	{Contract: "Option", RemovedIn: Release1_1, Replacement: "the matching evo.Config field"},
	// Title/Plain/DryRun as functional-option CALLS (evo.Title("x")) are
	// gone; Config.Title/Plain/DryRun as struct fields are unaffected and
	// unambiguous without a call — the Taught patterns require parens so
	// this table entry never flags the live field spelling.
	{Contract: "Title(", RemovedIn: Release1_1, Replacement: "Config.Title", Taught: regexp.MustCompile(`\bevo\.Title\(`)},
	{Contract: "Plain(", RemovedIn: Release1_1, Replacement: "Config.Plain", Taught: regexp.MustCompile(`\bevo\.Plain\(\)`)},
	{Contract: "DryRun(", RemovedIn: Release1_1, Replacement: "Config.DryRun", Taught: regexp.MustCompile(`\bevo\.DryRun\(\)`)},
}

// warnTaught matches the removed Warn taught as a call on an evo receiver
// (task.Warn, evo.Warn, Output.Warn) or as a member of an outcome-verb list
// ("Fail/Warn/Block"). A bare logger.Warn is slog, not evo.
var warnTaught = regexp.MustCompile(`\b(?:evo|Output|TaskHandle|[Tt]ask\w*|out)\.Warn\b` +
	"|\\bWarn`?/`?(?:Block|Fail|Problem|Cancel|Skipped|Fact|Summary)\\b" +
	"|\\b(?:Block|Fail|Problem|Cancel|Skipped|Fact|Summary)`?/`?Warn\\b")

// init appends the E-121 (ZYS-1180 freeze) capture-meaning Evidence* renames
// generated from CaptureRenames, the one table shared with the MCP rules
// and the structural detector.
func init() {
	symbols = append(symbols, captureRenameSymbols()...)
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

// Symbols returns every retired name, in table order.
func Symbols() []Symbol { return slices.Clone(symbols) }

// ContractNames returns every Symbol's Contract spelling.
func ContractNames() []string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Contract
	}
	return names
}

// TaughtIn returns every retired Symbol text teaches, with the matched
// spelling.
func TaughtIn(text string) []Hit {
	var hits []Hit
	for _, s := range symbols {
		if s.Taught == nil {
			continue
		}
		if m := s.Taught.FindString(text); m != "" {
			hits = append(hits, Hit{Symbol: s, Match: m})
		}
	}
	return hits
}

// Hit is one retired Symbol found in text.
type Hit struct {
	Symbol Symbol
	Match  string
}
