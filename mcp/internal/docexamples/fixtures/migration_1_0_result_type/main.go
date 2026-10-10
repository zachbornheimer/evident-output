// Package main compiles docs/migration/1.0.md's "After (1.0)" fence
// documenting the shape of Result (which docexamples/fixtures/migration_1_0
// itself mirrors, not evo.Result directly, so this fixture never needs to
// track the real internal/core.Result's field order — only that the
// documented shape still parses and type-checks). See
// TestDocFencesMatchFixtures. Never run.
package main

// Conclusion stands in for evo.Conclusion, whose real definition lives in
// internal/core/conclusion.go; only the one field this fence reads is
// reproduced here.
type Conclusion struct {
	ExitCode int
}

// docexamples:snippet start
type Result struct {
	Conclusion Conclusion
	Err        error // the application error run returned, if any
}

func (r Result) ExitCode() int { return r.Conclusion.ExitCode }

// docexamples:snippet end

func main() { _ = Result{}.ExitCode() }
