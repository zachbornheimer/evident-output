package fix_test

// Fixture source/golden pairs for TestAnalyzersAgainstGoldenFixtures, one
// per 1.1-migration analyzer. See analyzer_fixtures_test.go for why these
// are Go string constants rather than testdata/src analysistest fixtures.

const warnFixtureSrc = `package evowarn

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// logger has its own Warn method, unrelated to evo. WarnAnalyzer resolves
// purely through go/types back to the evo package (recvNamedType), so this
// same-named, same-shaped method on an unrelated type must never be
// flagged.
type logger struct{}

func (l *logger) Warn(msg string) {}

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	t.Warn("stale cache") // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `

	// chained call: the receiver of Warn is itself a call result
	// (Doing(...) returns *TaskHandle), not a plain identifier.
	t.Doing("scan").Warn("chained") // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `

	// call inside a Define callback
	t.Define(func(ctx context.Context) error {
		t.Warn("inside define") // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
		return nil
	})

	// multi-line argument
	t.Warn( // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
		"multi\n" +
			"line summary",
	)

	// method value: f := t.Warn, evaluated once at creation
	f := t.Warn // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
	f("via value")

	// method expression: (*evo.TaskHandle).Warn
	e := (*evo.TaskHandle).Warn // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
	e(t, "via expression")

	// a spread trailing ProblemOption slice is not a mechanical rewrite
	opts := []evo.ProblemOption{}
	t.Warn("spread", opts...) // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: rewrite to Problem\(summary, append\(opts, evo\.Severity\(evo\.SeverityWarning\)\)\.\.\.\) by hand — the spread trailing argument isn't a mechanical rewrite` + "`" + `

	// package-level evo.Warn (removed in 1.1): no Task to attach a Problem to
	evo.Warn("run scoped") // want ` + "`" + `evo\.Warn was removed in 1.1: declare a Task and call its Problem\(summary, evo\.Severity\(evo\.SeverityWarning\)\) instead — attaching a run-scoped warning to a Task is not mechanical` + "`" + `

	// a different evo type's Warn: named but unfixable
	g := out.Group("workers")
	g.Warn("group scoped") // want ` + "`" + `\(\*evo\.GroupHandle\)\.Warn was removed in 1.1: GroupHandle has no Task to attach a Problem to — declare a Task and call its Problem\(summary, evo\.Severity\(evo\.SeverityWarning\)\) instead` + "`" + `

	l := &logger{}
	l.Warn("not evo, never flagged")

	return out.Finish()
}
`

const warnFixtureGolden = `package evowarn

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// logger has its own Warn method, unrelated to evo. WarnAnalyzer resolves
// purely through go/types back to the evo package (recvNamedType), so this
// same-named, same-shaped method on an unrelated type must never be
// flagged.
type logger struct{}

func (l *logger) Warn(msg string) {}

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	t.Problem("stale cache", evo.Severity(evo.SeverityWarning)) // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `

	// chained call: the receiver of Warn is itself a call result
	// (Doing(...) returns *TaskHandle), not a plain identifier.
	t.Doing("scan").Problem("chained", evo.Severity(evo.SeverityWarning)) // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `

	// call inside a Define callback
	t.Define(func(ctx context.Context) error {
		t.Problem("inside define", evo.Severity(evo.SeverityWarning)) // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
		return nil
	})

	// multi-line argument
	t.Problem( // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
		"multi\n"+
			"line summary", evo.Severity(evo.SeverityWarning),
	)

	// method value: f := t.Warn, evaluated once at creation
	f := func() func(summary string) {
		recv := t
		return func(summary string) {
			recv.Problem(summary, evo.Severity(evo.SeverityWarning))
		}
	}() // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
	f("via value")

	// method expression: (*evo.TaskHandle).Warn
	e := func(recv *evo.TaskHandle, summary string) {
		recv.Problem(summary, evo.Severity(evo.SeverityWarning))
	} // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: Problem wins over Warn, warning is a Problem severity` + "`" + `
	e(t, "via expression")

	// a spread trailing ProblemOption slice is not a mechanical rewrite
	opts := []evo.ProblemOption{}
	t.Warn("spread", opts...) // want ` + "`" + `\(\*evo\.TaskHandle\)\.Warn was removed in 1.1: rewrite to Problem\(summary, append\(opts, evo\.Severity\(evo\.SeverityWarning\)\)\.\.\.\) by hand — the spread trailing argument isn't a mechanical rewrite` + "`" + `

	// package-level evo.Warn (removed in 1.1): no Task to attach a Problem to
	evo.Warn("run scoped") // want ` + "`" + `evo\.Warn was removed in 1.1: declare a Task and call its Problem\(summary, evo\.Severity\(evo\.SeverityWarning\)\) instead — attaching a run-scoped warning to a Task is not mechanical` + "`" + `

	// a different evo type's Warn: named but unfixable
	g := out.Group("workers")
	g.Warn("group scoped") // want ` + "`" + `\(\*evo\.GroupHandle\)\.Warn was removed in 1.1: GroupHandle has no Task to attach a Problem to — declare a Task and call its Problem\(summary, evo\.Severity\(evo\.SeverityWarning\)\) instead` + "`" + `

	l := &logger{}
	l.Warn("not evo, never flagged")

	return out.Finish()
}
`

const stepFixtureSrc = `package evostep

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// counter has its own Step method, unrelated to evo, and must never be
// flagged.
type counter struct{}

func (c *counter) Step(completed, total int, name string) {}

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	t.Step(1, 3, "scanning") // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `

	t.Doing("prep").Step(2, 3, "chained") // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `

	t.Define(func(ctx context.Context) error {
		t.Step(3, 3, "inside define") // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `
		return nil
	})

	t.Step( // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `
		1,
		3,
		"multi\n"+"line",
	)

	f := t.Step // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `
	f(1, 3, "via value")

	e := (*evo.TaskHandle).Step // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `
	e(t, 1, 3, "via expression")

	c := &counter{}
	c.Step(1, 1, "not evo, never flagged")

	return out.Finish()
}
`

const stepFixtureGolden = `package evostep

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// counter has its own Step method, unrelated to evo, and must never be
// flagged.
type counter struct{}

func (c *counter) Step(completed, total int, name string) {}

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	t.Progress(1, 3).Doing("scanning") // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `

	t.Doing("prep").Progress(2, 3).Doing("chained") // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `

	t.Define(func(ctx context.Context) error {
		t.Progress(3, 3).Doing("inside define") // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `
		return nil
	})

	t.Progress(1, 3).Doing("multi\n" + "line")

	f := func() func(completed, total int, name string) *evo.TaskHandle {
		recv := t
		return func(completed, total int, name string) *evo.TaskHandle {
			return recv.Progress(completed, total).Doing(name)
		}
	}() // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `
	f(1, 3, "via value")

	e := func(recv *evo.TaskHandle, completed, total int, name string) *evo.TaskHandle {
		return recv.Progress(completed, total).Doing(name)
	} // want ` + "`" + `evo\.TaskHandle\.Step was removed in 1.1: Progress wins over Step; current-item text is orthogonal \(task\.Progress\(i, total\)\.Doing\(name\)\)` + "`" + `
	e(t, 1, 3, "via expression")

	c := &counter{}
	c.Step(1, 1, "not evo, never flagged")

	return out.Finish()
}
`

const keptFixtureSrc = `package evokept

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// tally has its own Kept method, unrelated to evo, and must never be
// flagged.
type tally struct{}

func (t *tally) Kept(reason string) {}

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	t.Kept(evo.Reason("dirty")) // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `

	t.Doing("prep").Kept(evo.Reason("dirty")) // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `

	t.Define(func(ctx context.Context) error {
		t.Kept(evo.Reason("dirty")) // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `
		return nil
	})

	t.Kept( // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `
		evo.Reason(
			"multi\n" + "line",
		),
	)

	f := t.Kept // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `
	f(evo.Reason("via value"))

	e := (*evo.TaskHandle).Kept // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `
	e(t, evo.Reason("via expression"))

	tl := &tally{}
	tl.Kept("not evo, never flagged")

	return out.Finish()
}
`

const keptFixtureGolden = `package evokept

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// tally has its own Kept method, unrelated to evo, and must never be
// flagged.
type tally struct{}

func (t *tally) Kept(reason string) {}

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	t.Skipped(evo.Reason("dirty")) // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `

	t.Doing("prep").Skipped(evo.Reason("dirty")) // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `

	t.Define(func(ctx context.Context) error {
		t.Skipped(evo.Reason("dirty")) // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `
		return nil
	})

	t.Skipped(evo.Reason(
		"multi\n" + "line",
	))

	f := func() func(reason evo.TaxonomyReason) {
		recv := t
		return func(reason evo.TaxonomyReason) {
			recv.Skipped(reason)
		}
	}() // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `
	f(evo.Reason("via value"))

	e := func(recv *evo.TaskHandle, reason evo.TaxonomyReason) {
		recv.Skipped(reason)
	} // want ` + "`" + `evo\.TaskHandle\.Kept is not canonical vocabulary: a Task intentionally not executed is Skipped\(reason\)` + "`" + `
	e(t, evo.Reason("via expression"))

	tl := &tally{}
	tl.Kept("not evo, never flagged")

	return out.Finish()
}
`

const captureFixtureSrc = `package evocapture

import (
	evo "github.com/zachbornheimer/evident-output"
)

// recorder has its own EvidenceTail field, unrelated to evo.Problem, and
// must never be flagged.
type recorder struct{ EvidenceTail string }

func run() error {
	var opt evo.EvidenceOption // want ` + "`" + `evo\.EvidenceOption was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
	_ = opt

	var maxBytes = evo.MaxEvidenceBytes // want ` + "`" + `evo\.MaxEvidenceBytes was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
	_ = maxBytes

	p := evo.Problem{}
	if p.EvidenceTail != "" { // want ` + "`" + `p\.EvidenceTail was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
		_ = p.EvidenceTail // want ` + "`" + `p\.EvidenceTail was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
	}

	r := &recorder{}
	_ = r.EvidenceTail

	return nil
}
`

const captureFixtureGolden = `package evocapture

import (
	evo "github.com/zachbornheimer/evident-output"
)

// recorder has its own EvidenceTail field, unrelated to evo.Problem, and
// must never be flagged.
type recorder struct{ EvidenceTail string }

func run() error {
	var opt evo.CaptureOption // want ` + "`" + `evo\.EvidenceOption was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
	_ = opt

	var maxBytes = evo.MaxCaptureBytes // want ` + "`" + `evo\.MaxEvidenceBytes was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
	_ = maxBytes

	p := evo.Problem{}
	if p.CaptureTail != "" { // want ` + "`" + `p\.EvidenceTail was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
		_ = p.CaptureTail // want ` + "`" + `p\.EvidenceTail was removed in 1.1: Evidence means satisfaction proof, retained process output is Capture` + "`" + `
	}

	r := &recorder{}
	_ = r.EvidenceTail

	return nil
}
`

const reasonOptionFixtureSrc = `package evoreasonoption

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// scope has its own ForSkip method, unrelated to evo, and must never be
// flagged.
type scope struct{}

func (s *scope) ForSkip() string { return "" }

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	r1 := evo.Reason("dirty", evo.ForSkip()) // want ` + "`" + `evo\.ForSkip was removed in 1.1: a Reason has no usage constraints` + "`" + `
	t.Skipped(r1)

	t.Define(func(ctx context.Context) error {
		r2 := evo.Reason("busy", evo.OnTask("check")) // want ` + "`" + `evo\.OnTask was removed in 1.1: a Reason has no usage constraints` + "`" + `
		t.Skipped(r2)
		return nil
	})

	r3 := evo.Reason(
		"multi\nline",
		evo.ForSkip(), // want ` + "`" + `evo\.ForSkip was removed in 1.1: a Reason has no usage constraints` + "`" + `
	)
	t.Skipped(r3)

	// a bare reference is not a direct evo.Reason(...) argument, so it is
	// reported with no fix.
	var opt evo.ReasonOption  // want ` + "`" + `evo\.ReasonOption was removed in 1.1: a Reason has no usage constraints — not rewritten: not a direct evo\.Reason\(\.\.\.\) argument` + "`" + `
	opt = evo.OnTask("check") // want ` + "`" + `evo\.OnTask was removed in 1.1: a Reason has no usage constraints — not rewritten: not a direct evo\.Reason\(\.\.\.\) argument` + "`" + `
	_ = opt

	s := &scope{}
	_ = s.ForSkip()

	return out.Finish()
}
`

const reasonOptionFixtureGolden = `package evoreasonoption

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

// scope has its own ForSkip method, unrelated to evo, and must never be
// flagged.
type scope struct{}

func (s *scope) ForSkip() string { return "" }

func run() error {
	out := evo.Init(evo.Config{Title: "demo"})
	t := out.Task("check")

	r1 := evo.Reason("dirty") // want ` + "`" + `evo\.ForSkip was removed in 1.1: a Reason has no usage constraints` + "`" + `
	t.Skipped(r1)

	t.Define(func(ctx context.Context) error {
		r2 := evo.Reason("busy") // want ` + "`" + `evo\.OnTask was removed in 1.1: a Reason has no usage constraints` + "`" + `
		t.Skipped(r2)
		return nil
	})

	r3 := evo.Reason(
		"multi\nline", // want ` + "`" + `evo\.ForSkip was removed in 1.1: a Reason has no usage constraints` + "`" + `
	)
	t.Skipped(r3)

	// a bare reference is not a direct evo.Reason(...) argument, so it is
	// reported with no fix.
	var opt evo.ReasonOption  // want ` + "`" + `evo\.ReasonOption was removed in 1.1: a Reason has no usage constraints — not rewritten: not a direct evo\.Reason\(\.\.\.\) argument` + "`" + `
	opt = evo.OnTask("check") // want ` + "`" + `evo\.OnTask was removed in 1.1: a Reason has no usage constraints — not rewritten: not a direct evo\.Reason\(\.\.\.\) argument` + "`" + `
	_ = opt

	s := &scope{}
	_ = s.ForSkip()

	return out.Finish()
}
`

const goldenOptionsFixtureSrc = `package evooptions

import (
	"bytes"

	evo "github.com/zachbornheimer/evident-output"
)

// builder has its own Init method, unrelated to evo, and must never be
// flagged.
type builder struct{}

func (b *builder) Init(opts ...string) *builder { return b }

func run() error {
	// a Config{...} literal call is already 1.1 shape: never flagged.
	skip := evo.Init(evo.Config{Title: "already 1.1"})
	_ = skip

	out := evo.Init(evo.Title("demo"), evo.DryRun(), evo.Width(80)) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly` + "`" + `
	_ = out

	multi := evo.Init( // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly` + "`" + `
		evo.Title("multi-line"),
		evo.Strict(),
	)
	_ = multi

	var buf bytes.Buffer
	unmapped := evo.Init(evo.Title("demo"), evo.AlsoWrite(&buf)) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly — not rewritten: AlsoWrite has no single Config field; keep it via Config.Options or migrate by hand` + "`" + `
	_ = unmapped

	dup := evo.Init(evo.Title("a"), evo.Title("b")) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly — not rewritten: more than one option writes the Title Config field, which would produce a duplicate-field struct literal that does not compile; merge them by hand` + "`" + `
	_ = dup

	debug := evo.Init(evo.DebugAddSource(), evo.DebugLevel(2)) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly` + "`" + `
	_ = debug

	b := &builder{}
	b.Init("not evo, never flagged")

	return nil
}
`

const goldenOptionsFixtureGolden = `package evooptions

import (
	"bytes"

	evo "github.com/zachbornheimer/evident-output"
)

// builder has its own Init method, unrelated to evo, and must never be
// flagged.
type builder struct{}

func (b *builder) Init(opts ...string) *builder { return b }

func run() error {
	// a Config{...} literal call is already 1.1 shape: never flagged.
	skip := evo.Init(evo.Config{Title: "already 1.1"})
	_ = skip

	out := evo.Init(evo.Config{Title: "demo", DryRun: true, Width: 80}) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly` + "`" + `
	_ = out

	multi := evo.Init(evo.Config{Title: "multi-line", Strict: true})
	_ = multi

	var buf bytes.Buffer
	unmapped := evo.Init(evo.Title("demo"), evo.AlsoWrite(&buf)) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly — not rewritten: AlsoWrite has no single Config field; keep it via Config.Options or migrate by hand` + "`" + `
	_ = unmapped

	dup := evo.Init(evo.Title("a"), evo.Title("b")) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly — not rewritten: more than one option writes the Title Config field, which would produce a duplicate-field struct literal that does not compile; merge them by hand` + "`" + `
	_ = dup

	debug := evo.Init(evo.Config{Debug: evo.DebugConfig{AddSource: true, Level: 2}}) // want ` + "`" + `evo\.Init built from functional Options is legacy: 1.1 constructs evo\.Config\{\.\.\.\} directly` + "`" + `
	_ = debug

	b := &builder{}
	b.Init("not evo, never flagged")

	return nil
}
`
