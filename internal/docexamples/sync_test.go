package docexamples_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
	"github.com/zachbornheimer/evident-output/internal/docexamples"
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
var docFixtures = []docexamples.DocFixture{
	{Doc: "README.md", FenceIndex: 0, Fixture: "fixtures/readme_quickstart"},

	{Doc: "docs/reference.md", FenceIndex: 0, Fixture: "fixtures/reference_1"},
	{Doc: "docs/reference.md", FenceIndex: 1, Fixture: "fixtures/reference_2"},
	{Doc: "docs/reference.md", FenceIndex: 2, Fixture: "fixtures/reference_3"},

	{Doc: "docs/development.md", FenceIndex: 0, Fixture: "fixtures/development_1"},
	{Doc: "docs/development.md", FenceIndex: 1, Fixture: "fixtures/development_2"},
	{Doc: "docs/development.md", FenceIndex: 2, Fixture: "fixtures/development_3"},

	{Doc: "docs/guides/teaching-ladder.md", FenceIndex: 0, Fixture: "fixtures/teaching_ladder_1"},
	{Doc: "docs/guides/teaching-ladder.md", FenceIndex: 1, Fixture: "fixtures/teaching_ladder_2"},
	{Doc: "docs/guides/teaching-ladder.md", FenceIndex: 2, Fixture: "fixtures/teaching_ladder_3"},
	{Doc: "docs/guides/teaching-ladder.md", FenceIndex: 3, Fixture: "fixtures/teaching_ladder_4"},
	{Doc: "docs/guides/teaching-ladder.md", FenceIndex: 4, Fixture: "fixtures/teaching_ladder_5"},
	{Doc: "docs/guides/teaching-ladder.md", FenceIndex: 5, Fixture: "fixtures/teaching_ladder_6"},

	{Doc: "docs/guides/exit-code-fidelity.md", FenceIndex: 0, Fixture: "fixtures/exit_code_fidelity_1"},
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
				fixtureDir := filepath.Join(root, "internal", "docexamples", df.Fixture)
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
// internal/agent/catalog guide as "guide/<id>" — see docs/mcp.md and
// internal/agent/sections/sections.go). Catalog guide bodies carry inline
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
