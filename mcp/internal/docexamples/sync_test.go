package docexamples_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/mcp/internal/docexamples"
)

// docFixtures pins every ```go fence in the library's public docs to the
// fixture package that proves it still compiles against the shipped API. A
// fence with no row here fails loudly (a new snippet shipped unchecked); a
// row whose fence no longer exists fails loudly too (edit the fixture list
// alongside the doc). The fixtures themselves are ordinary Go source under
// fixtures/, built by `go build ./...` like any other package — this test
// only proves each fixture's marked region(s) are still the same content
// (modulo whitespace gofmt is free to change, see NormalizeIndent) as the
// fence they claim to cover, so a doc edit that forgets its fixture — or a
// fixture edit that drifts from its doc — fails here instead of shipping
// aspirational syntax.
//
// Scope: every doc the MCP `sections` corpus serves
// (mcp/internal/agent/sections/sections.go: reference.md, development.md,
// mcp.md, adoption-ladder.md) plus every doc README.md's own "Learn more"
// list links to as a live usage example — the flagship quickstart, the
// primary upgrade doc (migration/1.0.md), the teaching ladder, and the
// exit-code guide.
// Those are the docs a reader copy-pastes from, so every fence in them is
// pinned here, TextOnly (see DocFixture's doc comment) where the fence
// documents removed or elided API on purpose.
var docFixtures = []docexamples.DocFixture{
	{Doc: "README.md", FenceIndex: 0, Fixture: "fixtures/readme_quickstart"},

	{Doc: "mcp/docs/reference.md", FenceIndex: 0, Fixture: "fixtures/reference_1"},
	{Doc: "mcp/docs/reference.md", FenceIndex: 1, Fixture: "fixtures/reference_2"},
	{Doc: "mcp/docs/reference.md", FenceIndex: 2, Fixture: "fixtures/reference_3"},

	{Doc: "mcp/docs/development.md", FenceIndex: 0, Fixture: "fixtures/development_1"},
	{Doc: "mcp/docs/development.md", FenceIndex: 1, Fixture: "fixtures/development_2"},
	{Doc: "mcp/docs/development.md", FenceIndex: 2, Fixture: "fixtures/development_3"},

	{Doc: "mcp/docs/teaching-ladder.md", FenceIndex: 0, Fixture: "fixtures/teaching_ladder_1"},
	{Doc: "mcp/docs/teaching-ladder.md", FenceIndex: 1, Fixture: "fixtures/teaching_ladder_2"},
	{Doc: "mcp/docs/teaching-ladder.md", FenceIndex: 2, Fixture: "fixtures/teaching_ladder_3"},
	{Doc: "mcp/docs/teaching-ladder.md", FenceIndex: 3, Fixture: "fixtures/teaching_ladder_4"},
	{Doc: "mcp/docs/teaching-ladder.md", FenceIndex: 4, Fixture: "fixtures/teaching_ladder_5"},
	{Doc: "mcp/docs/teaching-ladder.md", FenceIndex: 5, Fixture: "fixtures/teaching_ladder_6"},

	{Doc: "mcp/docs/exit-code-fidelity.md", FenceIndex: 0, Fixture: "fixtures/exit_code_fidelity_1"},

	// docs/migration/1.0.md is README.md's primary upgrade doc ("every
	// breaking change with before/after code") and the doc most likely to
	// drift given how many signatures changed across 1.0/1.1. Several of
	// its fences are deliberately "Before (0.5)" snippets that must never
	// compile again (evo.MainWith, Group.Each, a run() with no ctx) or
	// elide a real initializer for brevity (the Projection const list) —
	// those are TextOnly: pinned as exact text via a `//go:build ignore`
	// fixture file, never fed to `go build`/`go vet`. See DocFixture's
	// TextOnly doc comment and each such fixture's own header comment.
	{Doc: "docs/migration/1.0.md", FenceIndex: 0, Fixture: "fixtures/migration_1_0_before_run_no_ctx", TextOnly: true},
	{Doc: "docs/migration/1.0.md", FenceIndex: 1, Fixture: "fixtures/migration_1_0_run_ctx"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 2, Fixture: "fixtures/migration_1_0_result_type"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 3, Fixture: "fixtures/migration_1_0_before_after_exitcode", TextOnly: true},
	{Doc: "docs/migration/1.0.md", FenceIndex: 4, Fixture: "fixtures/migration_1_0_before_mainwith", TextOnly: true},
	{Doc: "docs/migration/1.0.md", FenceIndex: 5, Fixture: "fixtures/migration_1_0_after_isolated_run"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 6, Fixture: "fixtures/migration_1_0_before_each", TextOnly: true},
	{Doc: "docs/migration/1.0.md", FenceIndex: 7, Fixture: "fixtures/migration_1_0_after_group_define"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 8, Fixture: "fixtures/migration_1_0_verify"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 9, Fixture: "fixtures/migration_1_0_task_key"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 10, Fixture: "fixtures/migration_1_0_duplicate_sibling"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 11, Fixture: "fixtures/migration_1_0_file_spec"},
	{Doc: "docs/migration/1.0.md", FenceIndex: 12, Fixture: "fixtures/migration_1_0_projection_consts", TextOnly: true},
}

// hasBuildIgnoreTag reports whether fixture file src carries the exact
// `//go:build ignore` constraint as its first non-blank line — the same
// tag Go's own build system recognizes. It does not attempt general build
// constraint parsing (no `//go:build linux && !cgo`-style expressions):
// TextOnly fixtures in this package use exactly one shape, and a fixture
// author reaching for anything more elaborate should reconsider whether
// TextOnly is the right tool.
func hasBuildIgnoreTag(src []byte) bool {
	for line := range strings.SplitSeq(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		return trimmed == "//go:build ignore"
	}
	return false
}

// TestHistoricalFixturesMatchTextOnly guards the other half of the
// TextOnly contract (see DocFixture's doc comment): every fixture file
// with a `//go:build ignore` tag must belong to a TextOnly DocFixture, and
// every TextOnly DocFixture's Fixture directory must contain at least one
// such file. Without this, a TextOnly fixture could lose its ignore tag
// (silently starting to fail `go build ./...`/`go vet ./...` on pre-1.0
// API) or a non-TextOnly fixture could gain one (silently dropping out of
// compile coverage) and TestDocFencesMatchFixtures would never notice —
// it only compares text, never build tags.
func TestHistoricalFixturesMatchTextOnly(t *testing.T) {
	root := repoRoot(t)

	for _, df := range docFixtures {
		t.Run(df.Doc+"#"+df.Fixture, func(t *testing.T) {
			fixtureDir := filepath.Join(root, "mcp", "internal", "docexamples", df.Fixture)
			entries, err := os.ReadDir(fixtureDir)
			if err != nil {
				t.Fatalf("read fixture dir %s: %v", fixtureDir, err)
			}

			gotIgnoreTagged := false
			for _, e := range entries {
				name := e.Name()
				if e.IsDir() || !strings.HasSuffix(name, ".go") {
					continue
				}
				src, err := os.ReadFile(filepath.Join(fixtureDir, name))
				if err != nil {
					t.Fatalf("read %s: %v", name, err)
				}
				if hasBuildIgnoreTag(src) {
					gotIgnoreTagged = true
					break
				}
			}

			if df.TextOnly && !gotIgnoreTagged {
				t.Errorf("%s is TextOnly but no file in %s carries a `//go:build ignore` tag — it will be fed to go build/go vet", df.Doc, df.Fixture)
			}
			if !df.TextOnly && gotIgnoreTagged {
				t.Errorf("%s is not TextOnly but a file in %s carries a `//go:build ignore` tag — it is silently excluded from go build/go vet coverage", df.Doc, df.Fixture)
			}
		})
	}
}

// TestDocFencesMatchFixtures is the drift guard: it proves every fixture in
// docFixtures still says, verbatim (modulo whitespace), what its doc
// claims, and that every ```go fence in the covered docs has exactly one
// fixture pinned to it — neither more (an unlisted stray fence) nor fewer
// (a fence that gained no fixture).
func TestDocFencesMatchFixtures(t *testing.T) {
	root := repoRoot(t)

	byDoc := make(map[string][]docexamples.DocFixture)
	for _, f := range docFixtures {
		byDoc[f.Doc] = append(byDoc[f.Doc], f)
	}

	for doc, want := range byDoc {
		t.Run(doc, func(t *testing.T) {
			md, err := os.ReadFile(filepath.Join(root, doc))
			if err != nil {
				t.Fatalf("read %s: %v", doc, err)
			}
			fences := docexamples.ExtractGoFences(md)
			if len(fences) != len(want) {
				t.Fatalf("%s has %d ```go fences, docFixtures pins %d — update the fixture list", doc, len(fences), len(want))
			}

			for _, df := range want {
				fence := fences[df.FenceIndex]
				fixtureDir := filepath.Join(root, "mcp", "internal", "docexamples", df.Fixture)
				region, err := docexamples.FixtureRegion(fixtureDir)
				if err != nil {
					t.Fatalf("%s fence %d: %v", doc, df.FenceIndex, err)
				}

				gotNorm := docexamples.NormalizeIndent(region)
				wantNorm := docexamples.NormalizeIndent(fence.Content)
				if gotNorm != wantNorm {
					t.Errorf("%s fence %d (line %d) has drifted from %s:\n--- doc fence ---\n%s\n--- fixture region ---\n%s",
						doc, df.FenceIndex, fence.Line, df.Fixture, fence.Content, region)
				}
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test working directory")
		}
		dir = parent
	}
}

// TestCatalogGuidesHaveNoUntestedGoFences guards the other half of the MCP
// docs corpus (sections.List() serves the embedded docs above, plus every
// mcp/internal/agent/catalog guide as "guide/<id>" — see mcp/docs/mcp.md and
// mcp/internal/agent/sections/sections.go). Catalog guide bodies carry inline
// backtick snippets today, not ```go fences, so there is nothing to pin
// yet. If a guide ever grows a fenced Go block, this fails until it gets a
// docFixtures row of its own, so that half of the MCP surface cannot drift
// into aspirational syntax unnoticed either.
func TestCatalogGuidesHaveNoUntestedGoFences(t *testing.T) {
	for _, g := range catalog.All() {
		fences := docexamples.ExtractGoFences([]byte(g.Body))
		if len(fences) > 0 {
			t.Errorf("catalog guide %q has %d untested ```go fence(s) — add a docFixtures row (and a fixtures/ package) for each, the same way docs/*.md fences are pinned", g.ID, len(fences))
		}
	}
}
