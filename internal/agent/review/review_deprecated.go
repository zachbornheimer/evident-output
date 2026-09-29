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
// failfCaptureTextPattern is EV-001: a Fail/Block argument that calls
// .Text()/.Tail() on the retained capture ring folds that text into the
// summary the row already shows. Matches any receiver's .Text()/.Tail()
// call appearing as an argument — the misuse is the method call shape, not
// the identifier. The removed-in-1.1 printf verbs remain dirty input.
var failfCaptureTextPattern = regexp.MustCompile(`\.(?:Fail(?:f)?|Block(?:f)?)\([^)]*\.(?:Text|Tail)\(\)[^)]*\)`)

// detectFailfEmbeddedEvidenceText flags EV-001's anti-pattern.
func detectFailfEmbeddedEvidenceText(filename, src string) []Finding {
	var findings []Finding
	for _, m := range failfCaptureTextPattern.FindAllStringIndex(src, -1) {
		findings = append(findings, Finding{
			RuleID:     "EV-001",
			Message:    "Fail/Block argument calls .Text()/.Tail() on the retained capture ring — that text is already auto-attached as a separate evidence line, so embedding it in the summary too duplicates it",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `keep the summary short and let Writer/Capture auto-attach the retained tail; inside Define return fmt.Errorf("install dependencies: %w", err)`,
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
// (or Block) so a derived suggestion can name the Fail/Block+Detail call the
// site should become. Cause was removed in 1.1.
var causeOptionPattern = regexp.MustCompile(`(\w+)\.(Fail|Block)\(\s*"([^"]*)"\s*,\s*evo\.Cause\(([^()]*)\)\s*\)`)

// bareCausePattern catches every other evo.Cause( shape (backtick summary,
// extra options, wrong receiver text) so at least the deprecation itself is
// still flagged even when a derived Fail/Block rewrite isn't cheap.
var bareCausePattern = regexp.MustCompile(`evo\.Cause\(`)

// retiredSpelling is one removed API-032 call spelling and its
// replacement: a match of pattern, optionally only on an evo surface
// receiver, yields message and the suggestion suggest derives from the
// matched receiver.
type retiredSpelling struct {
	pattern *regexp.Regexp
	// evoReceiverOnly restricts matches to receivers isEvoSurfaceRecv
	// accepts, for names common outside evo (Plan, Changes, Capture).
	evoReceiverOnly bool
	message         string
	suggest         func(recv string) string
}

// retiredSpellings are the removed spellings with a mechanical rewrite. A
// pattern's first capture group, when it has one, is the receiver.
var retiredSpellings = []retiredSpelling{
	{
		// Item folded into Task: one entity, one constructor.
		pattern: regexp.MustCompile(`(\w+)\.Item\(`),
		message: "Item folded into Task — Item was removed",
		suggest: func(recv string) string { return "replace " + recv + ".Item(...) with " + recv + ".Task(...)" },
	},
	{
		pattern:         regexp.MustCompile(`(\w+)\.Plan\(`),
		evoReceiverOnly: true,
		message:         "Plan was removed in v0.4 — use evo.Effect, evo.File, or Task.Fact",
		suggest: func(recv string) string {
			return "replace " + recv + ".Plan(...) with evo.Effect (opaque mutations), evo.File (file state), or Task.Fact (information), not a new Plan API"
		},
	},
	{
		pattern:         regexp.MustCompile(`(\w+)\.Changes\(`),
		evoReceiverOnly: true,
		message:         "Changes was removed in v0.4 — use evo.Effect, evo.File, or Task.Fact",
		suggest: func(recv string) string {
			return "replace " + recv + ".Changes(...) with evo.Effect (opaque mutations), evo.File (file state), or Task.Fact (information), not a new Changes API"
		},
	},
	{
		// OK was retired with Item: a Task resolves by running its Define
		// callback.
		pattern: regexp.MustCompile(`(\w+)\.OK\(\)`),
		message: "OK was retired with Item — a Task resolves by running its Define callback",
		suggest: func(recv string) string {
			return "replace " + recv + ".OK() with " + recv + ".Define(func(ctx context.Context) error { ... })"
		},
	},
	{
		// Because's text is now the resolving verb's own argument.
		pattern: regexp.MustCompile(`\.Because\(`),
		message: "Because was retired with Item — its text is now the resolving verb's own argument",
		suggest: func(string) string {
			return `replace OK().Because("text") with Summary("text").Define(...) (or fold into Block/Fail's summary)`
		},
	},
}

// findings reports every match of r in src.
func (r retiredSpelling) findings(filename, src string) []Finding {
	var out []Finding
	for _, m := range r.pattern.FindAllStringSubmatchIndex(src, -1) {
		recv := ""
		if len(m) >= 4 && m[2] >= 0 {
			recv = src[m[2]:m[3]]
		}
		if r.evoReceiverOnly && !isEvoSurfaceRecv(recv) {
			continue
		}
		out = append(out, Finding{
			RuleID:     "API-032",
			Message:    r.message,
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: r.suggest(recv),
		})
	}
	return out
}

// detectDeprecatedSpellings is API-032: it catches every superseded spelling
// with a fix, not a lecture — evo.New (evo.Init is the sole constructor),
// the retiredSpellings table (Item, Plan, Changes, OK, Because),
// evo.Cause (Fail/Block with Detail; Cause was removed in 1.1), and the
// rec-surface spellings (Config.Options, Option funcs, the mutation verbs
// removed in 1.1, Skip, ID, StartPhase, Failf/Blockf/Warn/Step/Kept/Evidence).
func detectDeprecatedSpellings(in fileInput) []Finding {
	var findings []Finding
	if dialectAtLeast(in.desiredVersion, dialectFold) {
		findings = append(findings, newInMainFindings(in.filename, in.src)...)
		for _, r := range retiredSpellings {
			findings = append(findings, r.findings(in.filename, in.src)...)
		}
		findings = append(findings, causeFindings(in.filename, in.src)...)
	}
	if dialectAtLeast(in.desiredVersion, dialectRec) {
		findings = append(findings, detectSupersededRecSurface(in)...)
	}
	return findings
}

// newInMainFindings flags evo.New in main: evo.Init is the sole
// constructor, and evo.MainWith was removed in 1.0 (ordinary main uses
// evo.Main, Isolated instances use Output.Run).
func newInMainFindings(filename, src string) []Finding {
	body, offset := firstFuncBody(src, "main")
	if offset < 0 {
		return nil
	}
	idx := strings.Index(body, "evo.New(")
	if idx < 0 {
		return nil
	}
	return []Finding{{
		RuleID:     "API-032",
		Message:    "evo.New was removed with the item/task fold; evo.Init is the sole constructor",
		File:       filename,
		Line:       lineAt(src, offset+idx),
		Suggestion: "replace evo.New(cfg) with evo.Init(cfg) (Isolated: true for a hosted instance; os.Exit(evo.Main(run)) in ordinary main, out.Run(ctx, run).ExitCode() when holding *Output)",
	}}
}

// causeFindings flags evo.Cause: a Fail/Block(summary, evo.Cause(err))
// site gets Fail/Block with Detail, and every other evo.Cause( is still
// flagged, once, with the generic one. Cause was removed in 1.1.
func causeFindings(filename, src string) []Finding {
	var findings []Finding
	// derived marks every evo.Cause( the derived pass already covered.
	var derived []int
	for _, m := range causeOptionPattern.FindAllStringSubmatchIndex(src, -1) {
		recv, verb, summary, cause := src[m[2]:m[3]], src[m[4]:m[5]], src[m[6]:m[7]], src[m[8]:m[9]]
		if idx := strings.Index(src[m[0]:m[1]], "evo.Cause("); idx >= 0 {
			derived = append(derived, m[0]+idx)
		}
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Message:    "evo.Cause was removed in 1.1; Fail/Block are statements — attach the error text with Detail",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: fmt.Sprintf(`%s.%s(%q, evo.Detail(%s.Error()))`, recv, verb, summary, cause),
		})
	}
	for _, m := range bareCausePattern.FindAllStringIndex(src, -1) {
		if slices.Contains(derived, m[0]) {
			continue
		}
		findings = append(findings, Finding{
			RuleID:     "API-032",
			Message:    "evo.Cause was removed in 1.1; Fail/Block are statements — attach the error text with Detail, or return fmt.Errorf inside Define",
			File:       filename,
			Line:       lineAt(src, m[0]),
			Suggestion: `replace evo.Cause(err) with evo.Detail(err.Error()) on Fail/Block, or return fmt.Errorf("...: %w", err) inside Define`,
		})
	}
	return findings
}
