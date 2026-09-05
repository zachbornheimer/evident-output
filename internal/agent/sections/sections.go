// Package sections is the full-docs counterpart to catalog: catalog holds
// hand-curated, token-budgeted guidance snippets; sections serves the whole
// authoritative doc corpus (reference, development, MCP wiring, the
// adoption ladder, and the per-concept guides catalog already owns) through
// one list/get pair, mirroring the Svelte MCP's list-sections /
// get-documentation shape. The embedded copies under embedded/ are
// generated from docs/*.md (go:generate ./...; see tools/gensections) —
// docs/*.md stays the only place that prose is authored.
package sections

import (
	"embed"
	"sort"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/agent/catalog"
)

//go:generate go run ../../../tools/gensections

//go:embed embedded/*.md
var embedded embed.FS

// Section is one servable unit of documentation.
type Section struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Source   string   `json:"source"`
	Concepts []string `json:"concepts,omitempty"`
	Body     string   `json:"body"`
}

// docSection describes one embedded doc file's presentation.
type docSection struct {
	id       string
	title    string
	file     string
	source   string
	concepts []string
}

var docSections = []docSection{
	{id: "reference", title: "API reference", file: "reference.md", source: "docs/reference.md", concepts: []string{"api", "types", "signatures"}},
	{id: "development", title: "Development guide", file: "development.md", source: "docs/development.md", concepts: []string{"contributing", "testing", "ci"}},
	{id: "mcp", title: "MCP server wiring", file: "mcp.md", source: "docs/mcp.md", concepts: []string{"mcp", "wire", "tools", "hosts"}},
	{id: "adoption-ladder", title: "Teaching / adoption ladder", file: "adoption-ladder.md", source: "docs/guides/teaching-ladder.md", concepts: []string{"adoption", "ladder", "onboarding"}},
	{id: "exit-code-fidelity", title: "Exit-code fidelity", file: "exit-code-fidelity.md", source: "docs/guides/exit-code-fidelity.md", concepts: []string{"exit-code", "os.Exit", "lifecycle", "run"}},
}

// List returns every section the server can return via Get, sorted by ID —
// doc-backed sections first, then the per-concept catalog guides.
func List() []Section {
	out := make([]Section, 0, len(docSections)+len(catalog.All()))
	for _, d := range docSections {
		body, err := embedded.ReadFile("embedded/" + d.file)
		if err != nil {
			// A missing embedded copy is a build-time defect (stale
			// tools/gensections output), never a runtime-reachable state —
			// surface it loudly rather than silently short a section.
			panic("sections: embedded copy missing for " + d.file + ": " + err.Error())
		}
		out = append(out, Section{
			ID:       d.id,
			Title:    d.title,
			Source:   d.source,
			Concepts: d.concepts,
			Body:     string(body),
		})
	}
	for _, g := range catalog.All() {
		out = append(out, Section{
			ID:       "guide/" + g.ID,
			Title:    g.Title,
			Source:   "internal/agent/catalog",
			Concepts: g.Concepts,
			Body:     g.Body,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns the section with the given ID, if any.
func Get(id string) (Section, bool) {
	for _, s := range List() {
		if s.ID == id {
			return s, true
		}
	}
	return Section{}, false
}

// Filter returns sections whose ID, title, or concepts match the query
// (case-insensitive substring), or every section when query is empty.
func Filter(query string) []Section {
	if query == "" {
		return List()
	}
	q := strings.ToLower(query)
	var out []Section
	for _, s := range List() {
		if strings.Contains(strings.ToLower(s.ID), q) || strings.Contains(strings.ToLower(s.Title), q) {
			out = append(out, s)
			continue
		}
		for _, c := range s.Concepts {
			if strings.Contains(strings.ToLower(c), q) {
				out = append(out, s)
				break
			}
		}
	}
	return out
}
