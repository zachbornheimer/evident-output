// Package review — deprecated spellings and hand-assembled failure text.
package review

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// printJoinPattern matches a Print/Println/Printf call fed a joined list —
// the hand-assembled failure summary evo-rec.md's Conclusion already owns.
// failfCaptureTextPattern is EV-001: task.Failf("...%s...", capture.Text())
// (or Blockf) folds the retained evidence ring straight into the summary the
// row already shows — Failf/Blockf's own auto-attach then renders the exact
// same text a second time as evidence underneath it (user-13-problems.md
// Problem 7). Matches any receiver's .Text()/.Tail() call appearing as a
// Failf/Blockf argument, not just a variable literally named "capture" —
// the misuse is the method call shape, not the identifier.
var failfCaptureTextPattern = regexp.MustCompile(`\.(?:Failf|Blockf)\([^)]*\.(?:Text|Tail)\(\)[^)]*\)`)

// detectFailfEmbeddedEvidenceText flags EV-001's anti-pattern.
func detectFailfEmbeddedEvidenceText(filename, src string) []Finding {
	var findings []Finding
	for _, m := range failfCaptureTextPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "EV-001",
			Message:    "Failf/Blockf argument calls .Text()/.Tail() on the retained evidence ring — that text is already auto-attached as a separate evidence line, so embedding it in the summary too duplicates it",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `pass context via the trailing ": %w" wrap instead — e.g. task.Failf("install dependencies: %w", err) — and let Failf/Blockf auto-attach the retained tail`,
		})
	}
	return findings
}

var printJoinPattern = regexp.MustCompile(`\.(Print|Println|Printf)\(\s*strings\.Join\(`)

// detectHandAssembledFailureSummary flags a Print* call whose argument joins
// a collected list — that summary duplicates Conclusion and can drift from
// the glyphs/exit code the ledger already shows.
func detectHandAssembledFailureSummary(filename, src string) []Finding {
	var findings []Finding
	for _, m := range printJoinPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "CON-002",
			Message:    "printing a joined list duplicates the Conclusion summary; resolve each item on its own Item/Task instead",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: "replace with one Item/Task per entry, and Next(evo.Label(...)) for follow-up guidance",
		})
	}
	return findings
}

// causeOptionPattern matches the shape `receiver.Fail("summary", evo.Cause(err))`
// (or Block) so a derived suggestion can name the exact Failf/Blockf call the
// site should become, not just a generic pointer at the rule.
var causeOptionPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(\s*"([^"]*)"\s*,\s*evo\.Cause\(([^()]*)\)\s*\)`)

// bareCausePattern catches every other evo.Cause( shape (backtick summary,
// extra options, wrong receiver text) so at least the deprecation itself is
// still flagged even when a derived Failf/Blockf rewrite isn't cheap.
var bareCausePattern = regexp.MustCompile(`evo\.Cause\(`)

// captureCallPattern matches a Capture accessor call so its receiver can
// drive a derived .Evidence(...) suggestion — Capture and Evidence share the
// same parameter list, so this is a pure spelling substitution.
var captureCallPattern = regexp.MustCompile(`(\w+)\.Capture\(`)

// itemCallPattern matches any receiver's .Item(...) declaration call — the
// shipped-v0.2.x fact-check constructor, now removed (Item folded into Task:
// one entity, one constructor).
var itemCallPattern = regexp.MustCompile(`(\w+)\.Item\(`)

// planCallPattern / changesCallPattern match the retired v0.2 Plan/Changes
// surfaces. Suggestion is evo.Effect / evo.File / Task.Fact, not a new Plan/Changes API.
var planCallPattern = regexp.MustCompile(`(\w+)\.Plan\(`)

var changesCallPattern = regexp.MustCompile(`(\w+)\.Changes\(`)

// becauseCallPattern matches the retired .Because(text) annotation chain —
// its text is now the resolving verb's own argument (e.g. Done(text)).
var becauseCallPattern = regexp.MustCompile(`\.Because\(`)

// okCallPattern matches the retired .OK() resolution verb — Task's spelling
// for the same outcome is Done().
var okCallPattern = regexp.MustCompile(`(\w+)\.OK\(\)`)

// detectDeprecatedSpellings is API-032: it catches every superseded spelling
// with a fix, not a lecture — evo.New (evo.Init is the sole constructor;
// evo.MainWith was removed in 1.0 — ordinary main uses evo.Main, Isolated
// instances use Output.Run), Item/.OK/.Because (Item folded into Task:
// Item(name).OK().Because(text) is now Task(name).Summary(text).Define(...)), evo.Cause
// (Failf/Blockf's trailing %w since Fail/Block are statement-form), Capture
// (renamed to Evidence), and the rec-surface spellings (Config.Options,
// Option funcs, the mutation verbs removed in 1.1, Skip, ID, StartPhase).
func detectDeprecatedSpellings(in fileInput) []Finding {
	filename, src, desiredVersion := in.filename, in.src, in.desiredVersion
	var findings []Finding
	if dialectAtLeast(desiredVersion, dialectFold) {

		if body, offset := firstFuncBody(src, "main"); offset >= 0 {
			if idx := strings.Index(body, "evo.New("); idx >= 0 {
				findings = append(findings, Finding{
					RuleID:     "API-032",
					Message:    "evo.New was removed with the item/task fold; evo.Init is the sole constructor",
					File:       filename,
					Line:       lineAt(src, offset+idx),
					Suggestion: "replace evo.New(cfg) with evo.Init(cfg) (Isolated: true for a hosted instance; os.Exit(evo.Main(run)) in ordinary main, out.Run(ctx, run).ExitCode() when holding *Output)",
				})
			}
		}

		for _, m := range itemCallPattern.FindAllStringSubmatchIndex(src, -1) {
			recv := src[m[2]:m[3]]
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "Item folded into Task — Item was removed",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: "replace " + recv + ".Item(...) with " + recv + ".Task(...)",
			})
		}

		for _, m := range planCallPattern.FindAllStringSubmatchIndex(src, -1) {
			recv := src[m[2]:m[3]]
			if !isEvoSurfaceRecv(recv) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "Plan was removed in v0.4 — use evo.Effect, evo.File, or Task.Fact",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: "replace " + recv + ".Plan(...) with evo.Effect (opaque mutations), evo.File (file state), or Task.Fact (information), not a new Plan API",
			})
		}

		for _, m := range changesCallPattern.FindAllStringSubmatchIndex(src, -1) {
			recv := src[m[2]:m[3]]
			if !isEvoSurfaceRecv(recv) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "Changes was removed in v0.4 — use evo.Effect, evo.File, or Task.Fact",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: "replace " + recv + ".Changes(...) with evo.Effect (opaque mutations), evo.File (file state), or Task.Fact (information), not a new Changes API",
			})
		}

		for _, m := range okCallPattern.FindAllStringSubmatchIndex(src, -1) {
			recv := src[m[2]:m[3]]
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "OK was retired with Item — a Task resolves by running its Define callback",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: "replace " + recv + ".OK() with " + recv + ".Define(func(ctx context.Context) error { ... })",
			})
		}

		for _, m := range becauseCallPattern.FindAllStringIndex(src, -1) {
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "Because was retired with Item — its text is now the resolving verb's own argument",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: `replace OK().Because("text") with Summary("text").Define(...) (or fold into Warn/Block/Fail's summary)`,
			})
		}

		// derivedCauseSpans marks the byte range of every evo.Cause( occurrence
		// already covered by a derived Failf/Blockf suggestion below, so the
		// generic fallback pass doesn't double-report the same call site.
		derivedCauseSpans := make([]int, 0)
		for _, m := range causeOptionPattern.FindAllStringSubmatchIndex(src, -1) {
			recv, verb, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]], src[m[8]:m[9]]
			if idx := strings.Index(src[m[0]:m[1]], "evo.Cause("); idx >= 0 {
				derivedCauseSpans = append(derivedCauseSpans, m[0]+idx)
			}
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "evo.Cause no longer affects the returned error since Fail/Block are statement-form; use " + verb + "f's trailing %w",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: fmt.Sprintf(`%s.%sf(%q, %s)`, recv, verb, summary+": %w", cause),
			})
		}
		for _, m := range bareCausePattern.FindAllStringIndex(src, -1) {
			if slices.Contains(derivedCauseSpans, m[0]) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "evo.Cause no longer affects the returned error since Fail/Block are statement-form; use Failf/Blockf's trailing %w",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: `replace evo.Cause(err) with a %w-wrapped Failf/Blockf, e.g. task.Failf("...: %w", err)`,
			})
		}

		for _, m := range captureCallPattern.FindAllStringSubmatchIndex(src, -1) {
			recv := src[m[2]:m[3]]
			if !isEvoSurfaceRecv(recv) {
				continue
			}
			findings = append(findings, Finding{
				RuleID:     "API-032",
				Message:    "Capture was renamed to Evidence — \"Stdout\" would lie as a name since it also takes stderr",
				File:       filename,
				Line:       lineAt(src, m[0]),
				Suggestion: "replace " + recv + ".Capture(...) with " + recv + ".Evidence(...)",
			})
		}

	}
	if dialectAtLeast(desiredVersion, dialectRec) {
		findings = append(findings, detectSupersededRecSurface(in)...)
	}
	return findings
}
