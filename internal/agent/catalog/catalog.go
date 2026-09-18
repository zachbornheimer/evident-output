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
			Concepts: []string{"Output", "Task", "Conclusion", "Main", "TaskHandle", "Sequence", "Group"},
			Rules: []string{
				"API-001", "API-006", "API-026", "API-028", "API-029", "DOM-006", "DOM-007", "DOM-011", "CON-002",
				"API-034", "API-035", "API-036", "API-037", "API-038", "DOM-018", "DOM-019", "DOM-020", "TAX-002", "TXT-020", "TXT-021",
			},
			Body: `Adoption ladder (guess-driven defaults — the naive spelling is the correct one):
  1) evo.Init(evo.Config{Title, DryRun}) once in main, before any I/O; os.Exit(evo.Main(run)) —
     dry-run wording, empty-case, and exit codes are all owned; run takes a context.Context
     (wired to SIGINT/SIGTERM) and returns only error; Main returns the derived exit code and
     does not itself call os.Exit (evo.Run/Output.Run return the full Result instead, for a
     caller that needs the Conclusion and application error, not just the code).
  2) task.Delete("worktree", fn, evo.Affected(n)) (also Add/Create/Update/Remove/Write/Push) — the
     callback is the work; Affected is optional quantity. Config.DryRun picks
     [planned] vs [changed]; no call site ever flips its own tense or chooses Changed/Ready/Planned.
  3) worktrees := evo.Group("worktrees"); for _, path := range paths { worktrees.Task(path).Define(...) }
     for independent collections; evo.Sequence for ordered ones (same one-Task-per-item shape;
     Group.Each/Sequence.Each were removed in 1.0); .Writer() as cmd.Stdout so a talkative
     child's last line becomes the live doing-text.
  4) evo.Task(name).Skipped(reason) / .Kept(reason) — taxonomy counted and summed, never a bare
     "skipped N". The item name is the Task name (a named Group/Sequence child).
  5) evo.Confirm(question, ...) — owns the whole gate (prompt, quiesce, ⊘/OK resolution, exit code).

Types: TaskHandle (work with Doing/Progress/mutations/taxonomy, or a fact-check gate resolved directly with no
Doing/Progress call), SequenceHandle (evo.Sequence — named children in dependency order, auto-lifecycle
NotStarted on failure/cancel), GroupHandle (evo.Group — independent children; the scheduler may overlap
eligible work; both offer nested .Sequence/.Group for recursive containers).
evo.Task/Sequence are get-or-create facades on the package-level default instance (see evo.Init/evo.SetDefault);
Record/RecordName/RecordLabel stay on TaskHandle for tooling call sites that need a raw ledger row, not a front
door of their own — Output.Changes/Output.Plan were removed (P1): every effect goes through a Task's mutation
verb now. Item/ItemHandle were removed v0.2.x shims over Task/TaskHandle — new code always uses Task.
Record/RecordLabel are quantity tallies and always render at Finish; RecordName names one item individually and
streams its row the instant its owning task resolves (Done/Fail/Block), bounded by the same viewport cap and
"… +N more (not shown)" overflow the Finish ledger uses.

Severity: Warn = non-terminal annotation (does not resolve the task — call it any number of times before Done/
Fail/Block); Block = stop before mutate; Fail = evaluation failed.
Exit-code honesty (DOM-020): Block and Fail carry different exit codes (1 vs 2) so a caller can tell "you did
something wrong" from "something broke while checking". A usage or user mistake (missing flag, declined confirm,
protected-branch policy) resolves Block, never Fail — routing it through Fail reports a user error as a system
failure.
Do not Start (API-006); no caller RunAll/Map/Retry on evo receivers (API-026 — Group/Sequence/Define/After are the scheduler; Group.Each/Sequence.Each were removed in 1.0); Failf/Blockf need % (API-028; Done/Warn/Task/Sequence/Reason
are printf-variadic themselves — there is no separate Donef/Warnf/Taskf/Reasonf); Capture not DebugWriter (API-029).
Never print a joined failure list yourself (CON-002): out.Println(strings.Join(failures, "\n")) duplicates the
one summary Conclusion already owns and can drift from the glyphs/exit code the ledger shows. Resolve each
failure on its own Task and use Next(evo.Label(...)) for follow-up guidance instead.`,
			TokenEstimate: 340,
		},
		{
			ID:       "tasks",
			Title:    "Tasks and progress",
			UseCases: []string{"progress", "collections", "phase", "bytes", "heartbeat", "loop", "retry", "skip"},
			Concepts: []string{"Task", "Group", "Sequence", "Progress", "Each", "Define", "Skipped", "Kept"},
			Rules:    []string{"API-027", "API-028", "DOM-016", "DOM-017", "BOUND-001", "API-030", "API-039", "EVO-STAMP-001", "EVO-STAMP-002", "EVO-FACT-001", "EVO-EFFECT-001", "EVO-UI-003"},
			Body: `Task is one atomic operation with optional Doing/Progress. Group/Sequence are collections whose state is
derived from children — never call Done/Fail/Progress on the collection itself (API-027). A Group of
exactly one explicit child is a lone Task (API-039): the live renderer collapses it to one line, and review flags the
Go shape so agents do not write a Group named run plus a single child. Group's children
are independent (the scheduler may overlap eligible work; concurrent Running children
expected); Sequence's children are an ordered dependency that stops later, still-unresolved siblings as
"-  not started" automatically once one fails or is cancelled (C13). Both offer nested .Sequence(name)/
.Group(name) for recursive containers — a failure three levels deep still surfaces at the root header.

Heartbeat: any unresolved row (Running or Pending), and any unfinished container header, gains an elapsed
suffix ("pushing feat/a — 5s") 5s after it is first actually painted in the live region — monotonic, never reset
by Doing/Progress activity, so a stale spinner is never indistinguishable from progress and a queued row ages
honestly even if nothing ever touches it.

Loops: prefer one named Task per item under evo.Group(name) (or Sequence) over a hand-maintained counter
(Group.Each/Sequence.Each were removed in 1.0) — Define/mutation verbs submit each item's work. On manual retry, set
Progress to the true completed count directly — there is no relative/delta counter to misuse (C7: Advance deleted).

Sealed-total invariant: indeterminate → determinate happens once; after a total is sealed it never changes, and
completed > total is unrepresentable.

Skip/keep taxonomy: task.Skipped(reason) / task.Kept(reason) — evo counts, sums, and truncates the
reason partition (never a bare "skipped 6"); reasons come from evo.Reason("protected") (get-or-create — repeated
calls with the same text merge into one taxonomy bucket, so inline evo.Reason("protected") at every call site is
correct as written; lifting it to a package-level var is a style choice, never required for correctness).

Bounded narration (BOUND-001): a slice joined with strings.Join and handed straight to Because/Detail/Doing
reproduces the same terminal flood evo.TruncateNames already fixed for Plan/Changes rows — wrap it:
evo.TruncateNames(names, 8) before it reaches any of those three calls.

Predeclare before fan-out (API-030): call out.Task/Group.Task for every child before starting any goroutine
or g.Go closure, then pass the handle in. Declaring the Task inside the closure races task creation with rendering
and produces the unordered multi-spinner defect Sequence's "one Running child" heart contract forbids.
Prefer one named Task per item under Group/Sequence, submitted with Define, over caller
goroutines (Group.Each/Sequence.Each were removed in 1.0) — Evo's scheduler owns overlap.

Facts vs Tasks (v0.4.0/P8): discovered information ("repository /repo", "language go", "config loaded") is not
work — never fake a checkmark Task to display it. Use task.Fact(name, value) (attached to the Task that
discovered it) or evo.Fact(name, value) (run-scoped) instead; both render as a durable dim "name  value" line,
never a lifecycle row, fire-and-forget. task.Warn(...)/evo.Warn(...) are the warning-severity sibling — an
annotation on the lifecycle, never a replacement for it (a warned-but-unresolved Task auto-resolves Done at
Finish). Both flow through the same placement rule: inline on the row when it is the only annotation, nested dim
lines otherwise.`,
			TokenEstimate: 320,
		},
		{
			ID:       "evidence-provenance",
			Title:    "File, Verify, and provenance (spec §55/§56)",
			UseCases: []string{"file", "idempotent", "provenance", "freshness", "cache", "manifest", "verify", "fingerprint"},
			Concepts: []string{"File", "FileSpec", "Verify", "Fingerprint", "FSPath", "Value", "App", "Basis"},
			Body: `Common file state goes through evo.File(ctx, evo.FileSpec{Path, Contents, Mode, Basis}) inside
task.Define — it creates, rewrites on drift, and no-ops when the desired state already matches; ctx must come
from a Task's Define callback. Do not teach a hand-rolled Evidence callback (a legacy named mutating registration)
as the normal way to make file work idempotent — evo.File already covers it.

Teach evo.Task("...").Define(fn) first, then Group/Sequence for collections, then evo.File for declarative
tracked file state, then a Task's Basis of Fingerprint values (evo.FSPath/evo.Value/evo.App) only when freshness
depends on semantic external inputs File/Exec do not already track, then Task.Verify only for domains Evo cannot
track automatically. Never lead with manifest internals or renderer controls.

Teach the exact distinction (spec §56) — these are four different claims, not synonyms:
  - Task.Verify controls whole-Task fast skipping in 1.0: a pre-Define check that, when true, skips Define
    entirely and resolves ResolutionAlreadySatisfied.
  - Application fingerprint (evo.App) is definition identity/invalidation metadata, not positive Evidence by
    itself.
  - Evo-native operation identity (evo.File's own tracked record) controls operation-level no-op/recompilation
    once the current invocation is observed.
  - Basis (a []Fingerprint on FileSpec) explains operation freshness — it does not by itself prove current state.
  - Evidence is a boolean current-state conclusion; only a true pre-Define Verify skips Define.
Never claim that discovering evo.File at the end of a callback can skip expensive arbitrary code that already
ran before it — only a pre-Define Verify can skip Define; File's own freshness check happens after Define starts.
Never invent a Basis entry the source code does not actually read (EVO-PROVENANCE-001).`,
			TokenEstimate: 300,
		},
		{
			ID:       "streams",
			Title:    "Stdout and stderr contracts",
			UseCases: []string{"json", "data-command", "progress-stderr", "pipe", "color", "child", "exit-code", "signal"},
			Concepts: []string{"Projection", "Plain", "JSON", "NoColor", "Config", "FormatData", "Main", "Writer"},
			Rules:    []string{"STREAM-003", "STREAM-004", "OUT-001", "OUT-003", "OUT-004", "API-031", "EV-001", "EVO-WIRE-001", "EVO-EXIT-001", "EVO-UI-004"},
			Body: `Human UI and logs must not contaminate structured stdout.
Ordinary dual-stream: evo.Init(evo.Config{Stdout: os.Stdout, Stderr: os.Stderr}) — Config auto-applies Plain/NoColor off-TTY.
FormatData reserves stdout for domain payload via ResultWriter; human presentation moves to stderr; a failed
data command emits no partial payload by default.

Exit codes come only from evo.Main's returned code (os.Exit(evo.Main(run))), or evo.Run/Output.Run's returned Result.ExitCode() for a caller that needs the Conclusion and application error, not just the code (evo.MainWith was removed in 1.0 — an Isolated *Output now calls its own Output.Run instead): run(ctx) returns error, nothing else picks the code. Never hand-map an int
to os.Exit — that is exactly how a Blocked run (1) gets silently read as success, or a real
failure reads as blocked. SIGINT/SIGTERM already route through Main into Cancel on the active task, so the
ledger's ■ and the process exit code (130) can never disagree; a caller-written signal.Notify handler that
calls os.Exit itself bypasses that reconciliation.

Child processes: cmd.Stdout = task.Writer(); cmd.Stderr = task.Writer(); on error
task.Failf("...: %w", err) (the trailing %w renders as an evidence line under the summary).
Never implement your own io.Writer whose Write method calls TaskHandle.Doing (API-031): that
reimplements the exact adapter Writer already owns.
Evidence is deduplicated for you: never embed capture text into a Failf/Blockf summary
(task.Failf("install failed: %s", capture.Text()) — EV-001) — auto-attach already renders that same
retained tail as its own evidence line underneath; embedding it in the summary too just repeats it.
Evidence is task-owned. Ring always retains proof; Config.Debug.Level gates journal display.
Do not hand-thread DebugWriter for brew/git.
EncodeJSON/EncodeJSONL for machines. Avoid fmt.Print during live UI — use evo.Println (see interactive guide).`,
			TokenEstimate: 260,
		},
		{
			ID:       "security",
			Title:    "Terminal safety",
			UseCases: []string{"sanitize", "esc", "secrets"},
			Concepts: []string{"sanitize", "Detail", "Failf"},
			Rules:    []string{"SEC-001", "TXT-007", "SEC-006"},
			Body: `Untrusted text is sanitized. Detail is stable user-visible guidance text; Failf/Blockf's
trailing %w renders the wrapped error's own text as a separate evidence line, also sanitized.
Never put raw ESC/CSI from user data into the terminal. Mark sensitive fields.`,
			TokenEstimate: 110,
		},
		{
			ID:       "evo-file-exec",
			Title:    "evo.File and evo.Exec: declarative tracked operations",
			UseCases: []string{"write", "chmod", "generate", "subprocess", "pipeline", "reconcile"},
			Concepts: []string{"File", "FileSpec", "Exec", "ExecSpec", "Fingerprint", "FSPath", "Outputs"},
			Rules:    []string{"EVO-FILE-001", "EVO-EXEC-001"},
			Body: `evo.File(ctx, evo.FileSpec{Path, Contents, Mode, Basis}) replaces hand-rolled os.WriteFile +
os.Chmod + a manual existence/hash check: it writes only on drift and no-ops when Path/Contents/Mode already
match, with dry-run safety the hand-rolled version never had (EVO-FILE-001).

evo.Exec(ctx, evo.ExecSpec{Executable, Args, Basis, Outputs}) is the same declarative shape for an external
process: it replaces os/exec.Command paired with a hand-written stat/hash/mtime freshness check, and no-ops
when the declared Outputs are already current against Basis (EVO-EXEC-001). Outputs are the paths the process
itself produces — never add them to Basis, and never flag a literal Exec Arg that already names a declared
Output or the call's own Executable as an omitted input (EVO-PROVENANCE-001, see the provenance guide).

Both share one freshness contract: Basis lists every additional Fingerprint input (evo.FSPath/evo.Value/evo.App)
whose change should invalidate the current result — call it inside task.Define, from a ctx that Define supplies.`,
			TokenEstimate: 220,
		},
		{
			ID:       "provenance",
			Title:    "Basis honesty and manifest-skip honesty",
			UseCases: []string{"basis", "freshness", "manifest", "skip", "already-satisfied"},
			Concepts: []string{"Basis", "FSPath", "Fingerprint", "Verify", "Manifest"},
			Rules:    []string{"EVO-PROVENANCE-001", "EVO-PROVENANCE-002"},
			Body: `evo.File/evo.Exec no-op when Basis, identity, and outputs are all current — every file or value a generator
visibly reads belongs in Basis, or the operation's freshness claim is false (EVO-PROVENANCE-001). A generator that
reads a config file, template, or environment value the call site never adds to Basis silently skips re-running
after that input changes.

The manifest is a record of what a past run did, not evidence about the current filesystem/process state. A
Task/operation may only report already-satisfied on proof this run observed, never on provenance an opaque
callback merely recorded on some earlier run (EVO-PROVENANCE-002) — replace a manifest-only skip with a live
pre-definition Verify (or a tracked evo.File/evo.Exec check) that proves the current state, not the recorded one.`,
			TokenEstimate: 160,
		},
		{
			ID:       "interactive",
			Title:    "Live region and debug",
			UseCases: []string{"spinner", "debug", "narrow", "confirm", "prompt", "resize", "suspend", "child-ui"},
			Concepts: []string{"LiveSurface", "VisibilityDelay", "Terminal", "Confirm", "Println"},
			Rules:    []string{"TERM-001", "TERM-006", "TERM-015", "CONFIRM-001", "CONFIRM-002", "LOG-001"},
			Body: `Instant Done before the visibility threshold must not flash a spinner.
Durable notes go through evo.Println/Print/Printf — never fmt.Print* — while a live region is open: evo clears
the region, writes the line, redraws, atomically. fmt bypassing that path is how frames tear.
evo.Confirm(question, ...) quiesces the live region for the whole ask-decide-resolve window before it prompts:
no spinner ever animates while waiting on a human, and the answer resolves to OK, ⊘ declined, or ⊘ blocked by
policy (never Failed, never Cancelled) — see common-api for the call.
A severe question (delete/remove/trash/retire/force) always adds evo.Destructive() (CONFIRM-002): it renders the
prompt with an explicit "(destructive)" cue, so a user never approves a remote force-delete or trash mistaking it
for an ordinary yes/no.
Blocked (⊘, exit 1) is a distinct terminal state from Failed (✗, exit 2): a human "n", a policy refusal (no
--yes off-TTY), and a protected-branch rule are all ⊘, never ✗.
Resize is a rerender: width is re-read every frame; a narrowed pane recomputes truncation and compact layout
live rather than leaving a wrapped remnant.
Use evo.Terminal with testkit.Screen or terminal.NewANSI.

Child processes go through cmd.Stdout = task.Writer() so the live row keeps moving.
Do not inherit os.Stdout, and do not clear the spinner around a child.`,
			TokenEstimate: 260,
		},
		{
			ID:       "first-paint",
			Title:    "First paint and the heart contract",
			UseCases: []string{"startup", "latency", "blank", "streaming"},
			Concepts: []string{"Init", "VisibilityDelay", "Doing", "Progress"},
			Rules:    []string{"FP-001", "FP-002", "FP-003", "FP-004", "FP-005"},
			Body: `The user is always waiting for input, watching work, or reading a verdict — a blank terminal for the first
one to three seconds of a run is none of those, and reads as a hang.

evo.Init(evo.Config{...}) arms first paint before any I/O: call it as the very first statement in main, before
config parsing, git walks, or a network dial. Init + the first declared entity must produce something honest on
screen within 100ms of process start (FP-001). Declaring a Task paints it running (a spinner) immediately —
VisibilityDelay must not hide that first running frame, so a fast bind cannot popup-complete (FP-005).

Declare before you compute (FP-002): the first Task/Group/Sequence declaration comes before the first read/open/dial
in main or run — a config load or repo walk that happens first, with the first evo.Task only after, is the exact
"blank screen for two seconds" bug this guide exists to catch.

Discovery streams into the same task's Doing, then Progress, as totals become known (FP-003) — never a separate
"startup" task, never a fake total invented early. Doing text that goes stale for more than ~10s without a refresh
is a defect (now auto-mitigated by the built-in heartbeat, which appends elapsed context automatically — see the
tasks guide — but the caller must still not go silent for minutes with no Doing/Progress calls at all when a
child process could be narrating through Writer instead).

A Doing string also names the object in motion, never a placeholder (FP-004): "working"/"running"/"please
wait"/"starting" reads identically on every frame, so the user cannot tell progress from a hang. Say what is
being worked on — Doing("scanning ~/Developer/Personal/zq"), not Doing("scanning").`,
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
