// Package review — deprecated spellings: superseded call shapes with a
// mechanical rewrite (API-032).
package review

import (
	"regexp"
	"strings"
)

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
			return `replace OK().Because("text") with Summary("text").Define(...) (or fold into Warn/Block/Fail's summary)`
		},
	},
	{
		// Capture and Evidence share one parameter list, so this is a pure
		// spelling substitution.
		pattern:         regexp.MustCompile(`(\w+)\.Capture\(`),
		evoReceiverOnly: true,
		message:         "Capture was renamed to Evidence — \"Stdout\" would lie as a name since it also takes stderr",
		suggest:         func(recv string) string { return "replace " + recv + ".Capture(...) with " + recv + ".Evidence(...)" },
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
// the retiredSpellings table (Item, Plan, Changes, OK, Because, Capture),
// evo.Cause (a %w-wrapped error returned from Define since Fail/Block are
// statement-form), and the rec-surface spellings (Config.Options, Option
// funcs, the mutation verbs removed in 1.1, Skip, ID, StartPhase).
func detectDeprecatedSpellings(in fileInput) []Finding {
	var findings []Finding
	if dialectAtLeast(in.desiredVersion, dialectFold) {
		findings = append(findings, newInMainFindings(in.filename, in.src)...)
		for _, r := range retiredSpellings {
			findings = append(findings, r.findings(in.filename, in.src)...)
		}
		findings = append(findings, causeFindings(in)...)
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

