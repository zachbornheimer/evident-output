// Package catalog is the task-oriented guidance catalog for agent assistance.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Guide is one catalog entry.
type Guide struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	UseCases      []string `json:"use_cases"`
	Concepts      []string `json:"concepts"`
	Rules         []string `json:"rules"`
	Body          string   `json:"body"`
	TokenEstimate int      `json:"token_estimate"`
}

// All returns the built-in guidance catalog.
func All() []Guide {
	return []Guide{
		{
			ID:       "common-api",
			Title:    "Common API path",
			UseCases: []string{"items", "finish", "block", "ok", "main", "entity", "severity"},
			Concepts: []string{"Output", "Item", "Conclusion", "Main"},
			Rules:    []string{"API-001", "API-006", "API-026", "API-028", "API-029", "DOM-006", "DOM-007", "DOM-011"},
			Body: `Adoption ladder:
  1) evo.New() / New(Config{Title})
  2) Print/Printf/Println — replace human-facing fmt
  3) Verbose() — optional domain detail (not slog debug)
  4) Item / Task / Tasks / Changes / Plan
  5) task.Capture() + DetailTail for subprocess evidence
  6) slog.New(out.SlogHandler(...)) for implementation diagnostics
  7) os.Exit(evo.Main(out, run))

Pick the entity:
  Item     — check / gate / verdict unit (pass-fail)
  Task     — work with phases or progress
  Tasks    — collection of independent tasks (state derived)
  Changes  — past-tense durable effects that happened
  Plan     — dry-run would-happen effects

Severity: Warn = soft/optional; Block = stop before mutate; Fail = evaluation failed.
Do not Start (API-006); no RunAll/Map (API-026); Donef needs % (API-028); Capture not DebugWriter (API-029).`,
			TokenEstimate: 260,
		},
		{
			ID:       "tasks",
			Title:    "Tasks and progress",
			UseCases: []string{"progress", "collections", "phase", "bytes"},
			Concepts: []string{"Task", "Tasks", "Progress"},
			Rules:    []string{"API-027", "API-028", "DOM-016", "DOM-017"},
			Body: `Task is one operation with optional Phase/Progress. Tasks is a collection whose state is derived from children.
Prefer Item for sequential pass/fail gates; Task when the user should see progress/phase.
Prefer Progress(completed,total) and Bytes(completed,total). Use Advance for deltas only.
Never call Done/Fail on a Tasks collection. Long-running work: call Phase periodically (no automatic heartbeat yet).`,
			TokenEstimate: 160,
		},
		{
			ID:       "streams",
			Title:    "Stdout and stderr contracts",
			UseCases: []string{"json", "data-command", "progress-stderr", "pipe", "color", "child"},
			Concepts: []string{"Projection", "Plain", "JSON", "NoColor", "WriterOptions"},
			Rules:    []string{"STREAM-003", "OUT-001", "OUT-003", "OUT-004"},
			Body: `Human UI and logs must not contaminate structured stdout.
Use evo.WriterOptions(os.Stdout, evo.Diagnostics(os.Stderr)) for dual-stream CLIs.
WriterOptions applies Plain+NoColor for non-TTY *os.File (pipes). Diagnostics receives Debug and Capture mirrors.
Child processes: output := task.Capture(); run.Run(ctx, name, args, output); on error task.Fail(..., evo.Cause(err), output.DetailTail()).
Capture is task-owned (not session, not context). Ring always retains evidence; DebugLevel only controls display.
Do not hand-thread DebugWriter for brew/git.
EncodeJSON/EncodeJSONL for machines. Avoid fmt.Print during live UI.`,
			TokenEstimate: 180,
		},
		{
			ID:       "security",
			Title:    "Terminal safety",
			UseCases: []string{"sanitize", "esc", "secrets"},
			Concepts: []string{"sanitize", "Detail", "Cause"},
			Rules:    []string{"SEC-001", "TXT-007", "SEC-006"},
			Body: `Untrusted text is sanitized. Detail is user-visible strings only; Cause is diagnostic.
Never put raw ESC/CSI from user data into the terminal. Mark sensitive fields.`,
			TokenEstimate: 110,
		},
		{
			ID:       "interactive",
			Title:    "Live region and debug",
			UseCases: []string{"spinner", "debug", "narrow"},
			Concepts: []string{"LiveSurface", "VisibilityDelay", "Terminal"},
			Rules:    []string{"TERM-001", "TERM-006", "LOG-001"},
			Body: `Instant Done before the visibility threshold must not flash a spinner.
Debug lines are durable: clear live region, write line, redraw.
Use evo.Terminal with testkit.Screen or terminal.NewANSI.`,
			TokenEstimate: 120,
		},
	}
}

// Filter returns guides matching any use case or concept keyword (case-insensitive).
func Filter(useCase string) []Guide {
	if useCase == "" {
		return All()
	}
	q := stringsToLower(useCase)
	var out []Guide
	for _, g := range All() {
		if containsFold(g.ID, q) || containsFold(g.Title, q) || containsFold(g.Body, q) {
			out = append(out, g)
			continue
		}
		for _, u := range g.UseCases {
			if containsFold(u, q) {
				out = append(out, g)
				break
			}
		}
	}
	return out
}

// Get returns guides by ID in order; missing IDs are reported in missing.
func Get(ids []string) (found []Guide, missing []string) {
	idx := map[string]Guide{}
	for _, g := range All() {
		idx[g.ID] = g
	}
	for _, id := range ids {
		if g, ok := idx[id]; ok {
			found = append(found, g)
		} else {
			missing = append(missing, id)
		}
	}
	return found, missing
}

// Checksum returns a stable SHA-256 of the catalog body used by MCP, CLI,
// embedded resources, and website mirrors (MCP-010).
func Checksum() string {
	guides := All()
	ids := make([]string, len(guides))
	for i, g := range guides {
		ids[i] = g.ID
	}
	sort.Strings(ids)
	h := sha256.New()
	for _, id := range ids {
		for _, g := range guides {
			if g.ID != id {
				continue
			}
			_, _ = fmt.Fprintf(h, "%s\n%s\n%s\n", g.ID, g.Title, g.Body)
			break
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ApplyTokenBudget truncates guide bodies so estimated tokens stay within budget.
// Truncation is deterministic and marked explicitly (MCP-050).
func ApplyTokenBudget(guides []Guide, maxTokens int) (out []Guide, truncated bool) {
	if maxTokens <= 0 {
		return guides, false
	}
	remaining := maxTokens
	out = make([]Guide, 0, len(guides))
	for _, g := range guides {
		est := g.TokenEstimate
		if est <= 0 {
			est = len(g.Body)/4 + 1
		}
		if est <= remaining {
			out = append(out, g)
			remaining -= est
			continue
		}
		// Keep a stub with explicit truncation marker.
		budget := remaining
		if budget < 20 {
			truncated = true
			break
		}
		// Rough: 4 runes ≈ 1 token
		maxRunes := budget * 4
		body := g.Body
		if len([]rune(body)) > maxRunes {
			r := []rune(body)
			if maxRunes > 3 {
				body = string(r[:maxRunes-3]) + "…"
			} else {
				body = "…"
			}
			body += fmt.Sprintf("\n[truncated, token_budget=%d]", maxTokens)
			truncated = true
		}
		g.Body = body
		g.TokenEstimate = budget
		out = append(out, g)
		remaining = 0
		truncated = true
	}
	return out, truncated
}

func stringsToLower(s string) string {
	return strings.ToLower(s)
}

func containsFold(hay, needle string) bool {
	return strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
}
