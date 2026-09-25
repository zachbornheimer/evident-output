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
}

// warnTaught matches the removed Warn taught as a call on an evo receiver
// (task.Warn, evo.Warn, Output.Warn) or as a member of an outcome-verb list
// ("Fail/Warn/Block"). A bare logger.Warn is slog, not evo.
var warnTaught = regexp.MustCompile(`\b(?:evo|Output|TaskHandle|[Tt]ask\w*|out)\.Warn\b` +
	"|\\bWarn`?/`?(?:Block|Fail|Problem|Cancel|Skipped|Fact|Summary)\\b" +
	"|\\b(?:Block|Fail|Problem|Cancel|Skipped|Fact|Summary)`?/`?Warn\\b")

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
