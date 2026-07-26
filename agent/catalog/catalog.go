// Package catalog is the task-oriented guidance catalog for agent assistance.
package catalog

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
			UseCases: []string{"items", "finish", "block", "ok"},
			Concepts: []string{"Output", "Item", "Conclusion"},
			Rules:    []string{"API-001", "DOM-006", "DOM-007"},
			Body: `Use evo.For(subject), declare Items, resolve with OK/Block/Fail, then Finish.
Do not configure advanced specs for ordinary report commands.
Blocked means evaluation succeeded and found a blocker; Fail means evaluation failed.`,
			TokenEstimate: 120,
		},
		{
			ID:       "tasks",
			Title:    "Tasks and progress",
			UseCases: []string{"progress", "collections", "phase", "bytes"},
			Concepts: []string{"Task", "Tasks", "Progress"},
			Rules:    []string{"API-027", "API-028", "DOM-016", "DOM-017"},
			Body: `Task is one operation. Tasks is a collection whose state is derived from children.
Prefer Progress(completed,total) and Bytes(completed,total). Use Advance for deltas only.
Never call Done/Fail on a Tasks collection.`,
			TokenEstimate: 140,
		},
		{
			ID:       "streams",
			Title:    "Stdout and stderr contracts",
			UseCases: []string{"json", "data-command", "progress-stderr"},
			Concepts: []string{"Projection", "Plain", "JSON"},
			Rules:    []string{"STREAM-003", "OUT-001", "OUT-003", "OUT-004"},
			Body: `Human UI and logs must not contaminate structured stdout.
Use Plain/NonInteractive for CI. EncodeJSON/EncodeJSONL for machines.
Avoid fmt.Print during live UI; use out.Line, Debug, or SlogHandler.`,
			TokenEstimate: 130,
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

func stringsToLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsFold(hay, needle string) bool {
	return stringsContains(stringsToLower(hay), stringsToLower(needle))
}

func stringsContains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
