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
			Concepts: []string{"Output", "Item", "Task", "Conclusion", "Main", "TaskHandle", "ItemHandle", "GroupHandle"},
			Rules:    []string{"API-001", "API-006", "API-026", "API-028", "API-029", "DOM-006", "DOM-007", "DOM-011"},
			Body: `Adoption ladder (guess-driven defaults — the naive spelling is the correct one):
  1) evo.Init(evo.Config{Title, DryRun}) once in main, before any I/O; os.Exit(evo.Main(run)) — dry-run wording,
     empty-case, and exit codes are all owned; run returns only error.
  2) evo.Task(subject).Delete(n, obj) (also Create/Update/Remove/Write/Push/Record/RecordName) — the verb picks
     [planned] vs [changed] from Config.DryRun; no call site ever flips its own tense.
  3) evo.Task(name).Each(items) for loop progress (absolute, never double-counted); .PhaseWriter() as
     cmd.Stdout so a talkative child's last line becomes the live Phase.
  4) evo.Task(name).Skipped(reason, name) / .Kept(reason, name) — taxonomy counted and summed, never a bare
     "skipped N".
  5) evo.Confirm(question, ...) — owns the whole gate (prompt, quiesce, ⊘/OK resolution, exit code).

Types: TaskHandle (work with Phase/Progress/mutations/taxonomy), ItemHandle (pass-fail gate/verdict),
GroupHandle (evo.Group — named children, auto-lifecycle NotStarted on failure/cancel). evo.Task/Item/Group are
get-or-create facades on the package-level default instance (see evo.Init/evo.SetDefault); Plan/Changes/Record
stay on the instance API for tooling call sites, not the front door above.

Severity: Warn = soft/optional; Block = stop before mutate; Fail = evaluation failed.
Do not Start (API-006); no RunAll/Map (API-026); Donef needs % (API-028); Capture not DebugWriter (API-029).`,
			TokenEstimate: 320,
		},
		{
			ID:       "tasks",
			Title:    "Tasks and progress",
			UseCases: []string{"progress", "collections", "phase", "bytes", "heartbeat", "loop", "retry", "skip"},
			Concepts: []string{"Task", "Tasks", "Group", "Progress", "Each", "Skipped", "Kept"},
			Rules:    []string{"API-027", "API-028", "DOM-016", "DOM-017"},
			Body: `Task is one operation with optional Phase/Progress. Tasks/Group are collections whose state is derived from
children — never call Done/Fail/Progress on the collection itself (API-027); a Group stops later children as
"-  not started" automatically after a failed child.

Heartbeat: a Phase left unrefreshed for ~10s auto-appends elapsed context ("pushing feat/a — 90s") so a stale
spinner is never indistinguishable from progress — no manual timer required.

Loops: prefer evo.Task(name).Each(items) (or EachN(n)) over a hand-maintained counter — it owns the absolute
Progress(completed,total) so a retry can never double-count or move the bar backwards. On manual retry, set
Progress to the true completed count, not Advance(1) again for the same item.

Sealed-total invariant: indeterminate → determinate happens once; after a total is sealed it never changes, and
completed > total is unrepresentable.

Skip/keep taxonomy: task.Skipped(reason, name) / task.Kept(reason, name) — evo counts, sums, and truncates the
reason partition (never a bare "skipped 6"); reasons come from evo.Reason("protected") (get-or-create, typo-safe
once lifted to a var).`,
			TokenEstimate: 260,
		},
		{
			ID:       "streams",
			Title:    "Stdout and stderr contracts",
			UseCases: []string{"json", "data-command", "progress-stderr", "pipe", "color", "child", "exit-code", "signal"},
			Concepts: []string{"Projection", "Plain", "JSON", "NoColor", "Config", "FormatData", "Main", "PhaseWriter"},
			Rules:    []string{"STREAM-003", "OUT-001", "OUT-003", "OUT-004"},
			Body: `Human UI and logs must not contaminate structured stdout.
Ordinary dual-stream: evo.New(evo.Config{Stdout: os.Stdout, Stderr: os.Stderr}) — Config auto-applies Plain/NoColor off-TTY.
FormatData reserves stdout for domain payload via ResultWriter; human presentation moves to stderr; a failed
data command emits no partial payload by default.

Exit codes come only from evo.Main/evo.MainWith: run returns error, nothing else picks 0/1/2/130. Never
hand-map an int to os.Exit — that is exactly how a Blocked run (1) gets silently read as success, or a real
failure reads as blocked. SIGINT/SIGTERM already route through Main into Cancel on the active task, so the
ledger's ■ and the process exit code (130) can never disagree; a caller-written signal.Notify handler that
calls os.Exit itself bypasses that reconciliation.

Child processes: output := task.Capture(); run.Run(ctx, name, args, output); on error task.Fail(...,
evo.Cause(err), output.DetailTail()). For live narration wire cmd.Stdout = task.PhaseWriter() instead of a
hand-rolled line-splitting writer — every line becomes the current Phase and is retained for DetailTail.
Tool-backed gates: item.Capture() on the Item evaluating the condition.
Capture is entity-owned (Task or Item). Ring always retains evidence; Config.Debug.Level gates journal display.
Do not hand-thread DebugWriter for brew/git.
EncodeJSON/EncodeJSONL for machines. Avoid fmt.Print during live UI — use evo.Println (see interactive guide).`,
			TokenEstimate: 260,
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
			UseCases: []string{"spinner", "debug", "narrow", "confirm", "prompt", "resize", "suspend", "child-ui"},
			Concepts: []string{"LiveSurface", "VisibilityDelay", "Terminal", "Confirm", "Println", "Suspend"},
			Rules:    []string{"TERM-001", "TERM-006", "TERM-015", "CONFIRM-001", "LOG-001"},
			Body: `Instant Done before the visibility threshold must not flash a spinner.
Durable notes go through evo.Println/Print/Printf — never fmt.Print* — while a live region is open: evo clears
the region, writes the line, redraws, atomically. fmt bypassing that path is how frames tear.
evo.Confirm(question, ...) quiesces the live region for the whole ask-decide-resolve window before it prompts:
no spinner ever animates while waiting on a human, and the answer resolves to OK, ⊘ declined, or ⊘ blocked by
policy (never Failed, never Cancelled) — see common-api for the call.
Blocked (⊘, exit 1) is a distinct terminal state from Failed (✗, exit 2): a human "n", a policy refusal (no
--yes off-TTY), and a protected-branch rule are all ⊘, never ✗.
Resize is a rerender: width is re-read every frame; a narrowed pane recomputes truncation and compact layout
live rather than leaving a wrapped remnant.
Use evo.Terminal with testkit.Screen or terminal.NewANSI.

Handing the tty to a child: out.Suspend(func() error { ... }) clears the live region, holds it invisible for the
whole call, and redraws after — required only when a child paints its own UI on the shared terminal (its Stdout/
Stderr are the process's own, tty-passthrough); otherwise the parent's spinner and the child's first line glue
together. Captured or PhaseWriter-wired children (task.Capture(), cmd.Stdout = task.PhaseWriter()) never need
Suspend — their output already flows through evo's own render loop.`,
			TokenEstimate: 260,
		},
		{
			ID:       "first-paint",
			Title:    "First paint and the heart contract",
			UseCases: []string{"startup", "latency", "blank", "streaming"},
			Concepts: []string{"Init", "VisibilityDelay", "Phase", "Progress"},
			Rules:    []string{"FP-001", "FP-002", "FP-003"},
			Body: `The user is always waiting for input, watching work, or reading a verdict — a blank terminal for the first
one to three seconds of a run is none of those, and reads as a hang.

evo.Init(evo.Config{...}) arms first paint before any I/O: call it as the very first statement in main, before
config parsing, git walks, or a network dial. Init + the first declared entity must produce something honest on
screen within 100ms of process start (FP-001) — VisibilityDelay (default 80ms) only suppresses spinner flash on
work that finishes instantly; it never excuses a blank window before that.

Declare before you compute (FP-002): the first Task/Item/Group declaration comes before the first read/open/dial
in main or run — a config load or repo walk that happens first, with the first evo.Task only after, is the exact
"blank screen for two seconds" bug this guide exists to catch.

Discovery streams into the same task's Phase, then Progress, as totals become known (FP-003) — never a separate
"startup" task, never a fake total invented early. A Phase that goes stale for more than ~10s without a refresh
is a defect (now auto-mitigated by the built-in heartbeat, which appends elapsed context automatically — see the
tasks guide — but the caller must still not go silent for minutes with no Phase/Progress calls at all when a
child process could be narrating through PhaseWriter instead).`,
			TokenEstimate: 260,
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
