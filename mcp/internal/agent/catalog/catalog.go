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
			Concepts: []string{"Output", "Task", "Conclusion", "Main", "Run", "Sequence", "Group"},
			Rules: []string{
				"API-001", "DOM-007", "DOM-011", "CON-002",
				"API-035", "API-037", "API-038", "DOM-018", "DOM-019", "DOM-020", "TAX-002", "TXT-020", "TXT-021",
				"API-057", "API-060",
			},
			Body: `Adoption ladder (guess-driven defaults — the naive spelling is the correct one):
  1) os.Exit(evo.Main(evo.Config{Name, DryRun}, run)) in main; run is func(ctx, r *evo.Run) error —
     dry-run wording, empty-case, and exit codes are all owned; run takes a context.Context
     (wired to SIGINT/SIGTERM) and returns only error; Main returns the derived exit code and
     does not itself call os.Exit (evo.Execute returns the full Result instead, for a
     caller that needs the Conclusion and application error, not just the code).
  2) task.Define(func(ctx context.Context) error { return evo.Effect(ctx, evo.EffectSpec{Verb:
     evo.Delete, Object: "worktree", Quantity: n}, fn) }) for an opaque mutation (git ref,
     worktree, API change); evo.File(ctx, evo.FileSpec{...}) for file state. Config.DryRun picks
     [planned] vs [changed]; no call site ever flips its own tense or chooses Changed/Ready/Planned.
     A file write (os.WriteFile, os.Create, write-mode os.OpenFile) inside an Effect callback is
     never correct (API-057): Effect is the opaque-mutation escape hatch, not a second file-write
     API — route file state through evo.File, even when the new content derives from an existing
     file's contents (read the file first, then pass the derived result as FileSpec.Contents, or
     apply a unified diff with evo.Patch, which commits through File and fails with ErrStaleBasis
     rather than overwriting a changed source). A deletion is the Effect's own work: os.Remove or
     os.RemoveAll inside evo.Effect, with EffectSpec.Path naming the file (evo.File cannot state a
     deletion).
  3) worktrees := r.Group("worktrees"); for _, path := range paths { worktrees.Task(path).Define(...) }
     for independent collections; evo.Sequence for ordered ones (same one-Task-per-item shape;
     Group.Each/Sequence.Each were removed in 1.0); .Writer() as cmd.Stdout so a talkative
     child's last line becomes the live doing-text.
  4) return evo.Exclude(reason) from a Task's Define — taxonomy counted and summed, never a bare
     "skipped N". The item name is the Task name (a named Group/Sequence child).
  5) r.Confirm(question, ...) — owns the whole gate (prompt, quiesce, ⊘ declined resolution); a decline is evo.Refuse, exit 1.

Types: Task (work with Doing/Progress/mutations, resolved by what Define returns), Sequence (r.Sequence — named
children in dependency order, later siblings NotStarted on failure/cancel), Group (r.Group — independent
children; the scheduler may overlap eligible work; both offer nested .Sequence/.Group for recursive containers).
r.Task/r.Group/r.Sequence declare on the *evo.Run the application body receives.
Success resolves through Define (the callback returning nil); task.Summary(text) is the settled row's headline.
A mutation goes through evo.Effect (closed Verb set: Create, Update, Delete), information through task.Fact,
a file write through evo.File/evo.Patch.
Effect quantities tally and always render at Finish; a named row (evo.File "write <path>", evo.Exec "run <exe>")
streams the instant its owning task resolves, bounded by the same viewport cap and "… +N more (not shown)"
overflow the Finish ledger uses.

Severity: task.Problem(summary) = non-fatal finding (never ends the Task — call it any number of times before Define returns);
evo.Refuse = stop before mutation; evo.Fail = evaluation failed.
One Task, many Problems (1.1/ZYS-848): task.Problem(summary, opts...) appends one Problem without
ending the Task — call it once per finding instead of a Task per finding. The Task keeps the Outcome its
return gives: a nil return is Succeeded with the Problems attached, so a check that must fail returns evo.Fail.
Exit-code honesty (DOM-020): evo.Refuse and evo.Fail carry different exit codes (1 vs 2) so a caller can tell "you did
something wrong" from "something broke while checking". A usage or user mistake (missing flag, declined confirm,
protected-branch policy) returns evo.Refuse, never evo.Fail — routing it through Fail reports a user error as a system
failure.
evo.Fail/evo.Refuse are returned endings; Doing is printf-variadic; Summary takes one literal string; task.Writer() carries a child's output.
Never print a joined failure list yourself (CON-002): fmt.Fprintln(r.Text(), strings.Join(failures, "\n")) duplicates the
one summary Conclusion already owns and can drift from the glyphs/exit code the ledger shows. Resolve each
failure on its own Task and use evo.Next(evo.Action{Label: ...}) for follow-up guidance instead.`,
			TokenEstimate: 410,
		},
		{
			ID:       "tasks",
			Title:    "Tasks and progress",
			UseCases: []string{"progress", "collections", "phase", "bytes", "heartbeat", "loop", "retry", "skip"},
			Concepts: []string{"Task", "Group", "Sequence", "Progress", "Define", "Exclude"},
			Rules:    []string{"DOM-017", "BOUND-001", "API-030", "API-039", "API-045", "API-051"},
			Body: `Task is one independently schedulable promise whose outcome is independently meaningful to the user (ZYS-838) —
not a display row, not a subject label, not a container. A good Task name answers "what will this unit of work
accomplish or determine?" and usually reads as an action, verb + concrete object ("check file integrity", "format
Python", "stabilize Go source") — a strong heuristic, not a grammar validator: a concise contextual name can still
be clear, and review never rejects a short name on grammar alone. Four semantic tests decide, in order of what
actually matters: (1) does the Task's own name explain a failure without reading its children? (2) can it run/wait/
fail/satisfy independently? (3) would the user care about its independent outcome? (4) is it actual work, rather
than a category, a display heading, a fact, a verification dimension, or an implementation phase? "file integrity"
names a subject, not the work (API-045); "fix" organizes several independently meaningful operations under one row
instead of being one itself (API-045) — prefer a Group/Sequence such as Group("prepare staged files") with real
verb+object Tasks underneath. One Task may still make several internal observations — "check file integrity" can
inspect merge markers, path validity, symlinks, generated-file corruption — without turning each predicate into a
sibling Task: report them as Fact/Problem evidence under the one Task that answers the single user-meaningful
question, and only split one out into its own Task when it has an independently meaningful lifecycle/remediation
and can run on its own.

Do not hand-pick rows: a Group with no Summary renders no header of its own, a finished no-op child is hidden while
other content shows, [planned]/[changed] rows follow Task declaration order, and a cancelled run prints
"[cancelled]  subject  by user" plus an "!  already mutated: ..." line only if an Effect committed — never write those yourself.

Task is one atomic operation with optional Doing/Progress. Group/Sequence are collections that organize work — they
are never themselves fake work created just to earn a success row; state is
derived from children, so only a Task resolves or reports Progress; a Group or Sequence only settles. A Group of
exactly one explicit child is a lone Task (API-039): the live renderer collapses it to one line, and review flags the
Go shape so agents do not write a Group named run plus a single child. Group's children
are independent (the scheduler may overlap eligible work; concurrent Running children
expected); Sequence's children are an ordered dependency that stops later, still-unresolved siblings as
"-  not started" automatically once one fails or is cancelled (C13). Both offer nested .Sequence(name)/
.Group(name) for recursive containers — a failure three levels deep still surfaces at the root header. Task
itself has no .Task/.Group/.Sequence child constructors (ZYS-838 Decisions): only Run/Group/Sequence
declare children, so a Task cannot structurally grow containers of its own. The renderer, not the container's mere
presence, decides whether a container's own header row is visible or collapses into its one child — that decision
is independent of whether the children underneath are Tasks or further nested containers.

Heartbeat: any unresolved row (Running or Pending), and any unfinished container header, gains an elapsed
suffix ("pushing feat/a — 5s") 5s after it is first actually painted in the live region — monotonic, never reset
by Doing/Progress activity, so a stale spinner is never indistinguishable from progress and a queued row ages
honestly even if nothing ever touches it.

Loops: prefer one named Task per item under evo.Group(name) (or Sequence) over a hand-maintained counter
(Group.Each/Sequence.Each were removed in 1.0) — Define submits each item's work. On manual retry, set
Progress to the true completed count directly — there is no relative/delta counter to misuse (C7: Advance deleted).

Sealed-total invariant: indeterminate → determinate happens once; after a total is sealed it never changes, and
completed > total is unrepresentable.

Exclusion taxonomy: return evo.Exclude(reason) from the item Task's Define — evo counts, sums, and truncates the
reason partition (never a bare "skipped 6"); repeated calls with the same text merge into one taxonomy bucket, so an
inline evo.Exclude("protected") at every call site is correct as written; lifting it to a package-level var is a
style choice, never required for correctness.

Bounded narration (BOUND-001): a slice joined with strings.Join and handed straight to evo.Detail or Doing
reproduces the terminal flood the bounded ledger rows already fix — cap it (the first 8 names plus a count)
before it reaches either call.

Predeclare before fan-out (API-030): call Group.Task/Sequence.Task for every child in the
enclosing function, never inside a goroutine or g.Go closure. Declaration order is topology, so a Task
declared in a closure takes its row position and After edges from goroutine scheduling.
Prefer one named Task per item under Group/Sequence, submitted with Define, over caller
goroutines (Group.Each/Sequence.Each were removed in 1.0) — Evo's scheduler owns overlap.

One Task, many Problems (API-051, 1.1.0): a check that finds several independent issues owns one Task and
calls task.Problem(summary, opts...) once per finding — never one Task per finding (a Task(file) in a loop whose
callback only returns evo.Fail) and never every finding flattened into one errors.New(strings.Join(...)) string. Problem appends a
non-fatal finding without ending the Task; the Task still ends exactly once, by what Define returns (a check
that must fail returns evo.Fail). Every accumulated Problem survives in Snapshot/JSON/JSONL even when
human output bounds how many render inline.

Facts vs Tasks (v0.4.0/P8): discovered information ("repository /repo", "language go", "config loaded") is not
work — never fake a checkmark Task to display it. Use task.Fact(name, value) (attached to the Task that
discovered it) or r.Fact(name, value) (run-scoped) instead; both render as a durable dim "name  value" line,
never a lifecycle row, fire-and-forget. task.Problem(summary) is the
non-fatal annotation on the lifecycle, never a replacement for it (a Task ends only by what Define returns). Both flow through the same placement rule: inline on the row when it is the
only annotation, nested dim lines otherwise.`,
			TokenEstimate: 320,
		},
		{
			ID:       "evidence-provenance",
			Title:    "File, Verify, and provenance",
			UseCases: []string{"file", "idempotent", "provenance", "freshness", "cache", "manifest", "verify", "fingerprint"},
			Concepts: []string{"File", "FileSpec", "Verify", "FileSpec.Inputs", "FileSpec.Values"},
			Body: `Common file state goes through evo.File(ctx, evo.FileSpec{Path, Contents, Mode, Inputs, Values}) inside
task.Define — it creates, rewrites on drift, and no-ops when the desired state already matches; ctx must come
from a Task's Define callback. Do not teach a hand-rolled Evidence callback (a legacy named mutating registration)
as the normal way to make file work idempotent — evo.File already covers it.

Teach evo.Task("...").Define(fn) first, then Group/Sequence for collections, then evo.File for declarative
tracked file state, then FileSpec.Inputs / ExecSpec.Inputs and Values only when freshness
depends on external inputs File/Exec do not already track, then Task.Verify only for domains Evo cannot
track automatically. Never lead with manifest internals or renderer controls.

Teach the exact distinction — these are four different claims, not synonyms:
  - Task.Verify controls whole-Task fast skipping in 1.0: a pre-Define check that, when true, skips Define
    entirely and settles Satisfied.
  - Values (FileSpec.Values / ExecSpec.Values) name the non-file facts that decide freshness, such as a tool
    version; they are invalidation metadata, not positive Evidence by itself.
  - Evo-native operation identity (evo.File's own tracked record) controls operation-level no-op/recompilation
    once the current invocation is observed.
  - Inputs and Values explain operation freshness — they do not by themselves prove current state.
  - Evidence is a boolean current-state conclusion; only a true pre-Define Verify skips Define.
Never claim that discovering evo.File at the end of a callback can skip expensive arbitrary code that already
ran before it — only a pre-Define Verify can skip Define; File's own freshness check happens after Define starts.
Never invent an Input the source code does not actually read (EVO-PROVENANCE-001).`,
			TokenEstimate: 300,
		},
		{
			ID:       "streams",
			Title:    "Stdout and stderr contracts",
			UseCases: []string{"json", "data-command", "progress-stderr", "pipe", "color", "child", "exit-code", "signal"},
			Concepts: []string{"Output", "Plain", "JSON", "Config", "ResultWriter", "Main", "Writer"},
			Rules:    []string{"STREAM-003", "STREAM-004", "OUT-001", "OUT-003", "OUT-004", "API-031", "SIG-001", "SIG-002"},
			Body: `Human UI and logs must not contaminate structured stdout.
Ordinary dual-stream: os.Exit(evo.Main(evo.Config{}, run)) — a zero Config picks Plain off-TTY and honours NO_COLOR.
Run.ResultWriter carries the stdout domain payload; human presentation stays on stderr; a failed
data command emits no partial payload by default.

Exit codes come only from evo.Main's returned code (os.Exit(evo.Main(evo.Config{}, run))), or evo.Execute's returned Result.ExitCode() for a caller that needs the Conclusion and application error, not just the code (evo.MainWith was removed in 1.0): run(ctx) returns error, nothing else picks the code. Never hand-map an int
to os.Exit — that is exactly how a Blocked run (1) gets silently read as success, or a real
failure reads as blocked. SIGINT/SIGTERM already cancel the run's context through Main, so the
ledger shows the run Cancelled and the process exit code (130) can never disagree; a caller-written
signal.Notify handler that calls os.Exit itself bypasses that reconciliation (SIG-001). A host that wraps
evo.Main in its own signal.NotifyContext/signal.Notify for SIGINT/SIGTERM/os.Interrupt duplicates that same
lifecycle and can let the ledger and the process's real exit path diverge (SIG-002); delete the duplicate layer and
read cancellation from the ctx Main already passes into the run callback. Signal handling for anything
else (SIGHUP, SIGUSR1, ...) is unrelated application behavior and stays untouched.

Child processes: cmd.Stdout = task.Writer(); cmd.Stderr = task.Writer(); on error
inside Define return fmt.Errorf("...: %w", err), or return evo.Fail(summary, evo.Detail(text)) to attach
user-facing text.
Never implement your own io.Writer whose Write method calls Task.Doing (API-031): that
reimplements the exact adapter Writer already owns.
Task.Writer is task-owned: its ring always retains proof; Config.Debug.Level gates journal display.
Do not hand-thread DebugWriter for brew/git.
Run.ResultWriter() for the data payload and Config.Output (json or jsonl) for machines. Avoid fmt.Print during live UI — write to r.Text() (see interactive guide).`,
			TokenEstimate: 260,
		},
		{
			ID:       "security",
			Title:    "Terminal safety",
			UseCases: []string{"sanitize", "esc", "secrets"},
			Concepts: []string{"sanitize", "Detail", "Fail"},
			Rules:    []string{"SEC-001", "TXT-007", "SEC-006"},
			Body: `Untrusted text is sanitized. Detail is stable user-visible guidance text; Fail/Refuse
take evo.Detail(err.Error()) for the wrapped error's own text, also sanitized.
Never put raw ESC/CSI from user data into the terminal. Mark sensitive fields.`,
			TokenEstimate: 110,
		},
		{
			ID:       "evo-file-exec",
			Title:    "evo.File and evo.Exec: declarative tracked operations",
			UseCases: []string{"write", "chmod", "generate", "subprocess", "pipeline", "reconcile", "resource", "lock"},
			Concepts: []string{"File", "FileSpec", "Exec", "ExecSpec", "ExecSpec.Inputs", "ExecSpec.Outputs", "EffectSpec.Path", "EffectSpec.Resource", "Effect", "EffectSpec", "PartialEffect"},
			Rules:    []string{"EVO-FILE-001", "EVO-EXEC-001", "API-053", "API-054", "API-055", "API-058"},
			Body: `evo.File(ctx, evo.FileSpec{Path, Contents, Mode, Inputs, Values}) replaces hand-rolled os.WriteFile +
os.Chmod + a manual existence/hash check: it writes only on drift and no-ops when Path/Contents/Mode already
match, with dry-run safety the hand-rolled version never had (EVO-FILE-001).

File also claims write-side ownership of its own Path automatically — application code never wraps it in a
caller-managed sync.Mutex/RWMutex or lock file (API-055, ZYS-931/ZYS-840). Overlapping File and Effect claims
on the same or an ancestor path already wait on each other; a contended wait renders as "waiting for <path>" on
its own. A non-File operation that still needs exclusivity over a path claims it explicitly with
EffectSpec.Path (or EffectSpec.Resource, a logical name, for non-filesystem shared state) —
never a caller lock, and never a bare callback for tracked file state.

evo.Exec(ctx, evo.ExecSpec{Argv, Inputs, Values, Outputs}) is the same declarative shape for an external
process: it replaces os/exec.Command paired with a hand-written stat/hash/mtime freshness check, and no-ops
when the declared Outputs are already current against Inputs and Values (EVO-EXEC-001). Outputs are the paths the process
itself produces — never add them to Inputs, and never flag a literal Exec Arg that already names a declared
Output or the call's own Argv[0] as an omitted input (EVO-PROVENANCE-001, see the provenance guide).

evo.Exec returns (ExecResult, error): ExecResult carries Ran/ExitCode/Stdout/Stderr/Truncated, the same
sanitized/redacted, bounded capture Evo already retains as evidence — a linter or check can parse it to build
structured Problems/Facts without taking over process spawning, capture, liveness, or cancellation. Ordinary
callers that never inspect the result ignore it with "_, err := evo.Exec(ctx, spec)"; Exec still owns capture
either way — there is no second raw subprocess API to reach for.

Both share one freshness contract: Inputs (paths) and Values (named values) list every input
whose change should invalidate the current result — call it inside task.Define, from a ctx that Define supplies.

File claims its own path automatically; no caller manages a mutex, lock file, or unlock
lifecycle. For state Evo cannot model as desired file contents (a Git ref deletion, a worktree move, a remote
push), evo.Effect(ctx, evo.EffectSpec{Verb, Object, Quantity, Path, Resource}, fn) performs one opaque mutation and,
when Path or Resource is set, holds that claim for fn's duration. Generic
resource access holds at most one claim at a time: a second evo.File, evo.Patch, or claim-holding evo.Effect call
made with fn's own held ctx — directly, or through a helper fn hands that ctx to — is misuse and fails
instead of risking deadlock (API-053). Finish and return from the first
Effect/File before starting a second, or claim one coarser Path or Resource both mutations share.

An Effect fn that committed part of its Quantity before failing returns evo.PartialEffect(committed, err):
Effect records the committed subset as changed and still fails the Task with err (errors.Is/As intact). An
invalid count or nil err fails the Task and records no change.

evo.Patch(ctx, diff) derives the desired file states a unified diff describes — reading each source once under
its own read claim — and commits each through evo.File, so dry-run
planning, the stale-write guard (ErrStaleBasis), and already-satisfied all apply exactly as they do for a single
File call. Applying a patch through os/exec ("patch", "git apply", "git am") straight to the real workspace
bypasses that coverage outright and is never correct (API-058) — call evo.Patch(ctx, diff) instead.
Domain code that only parses or reads a patch's hunks, with no exec and no direct filesystem mutation, is not
this rule's target.`,
			TokenEstimate: 320,
		},
		{
			ID:       "provenance",
			Title:    "Input honesty and manifest-skip honesty",
			UseCases: []string{"inputs", "freshness", "manifest", "skip", "already-satisfied"},
			Concepts: []string{"FileSpec.Inputs", "FileSpec.Values", "Verify", "State"},
			Rules:    []string{"EVO-PROVENANCE-001", "EVO-PROVENANCE-002"},
			Body: `evo.File/evo.Exec no-op when Inputs, Values, and outputs are all current — every file or value a generator
visibly reads belongs in Inputs or Values, or the operation's freshness claim is false (EVO-PROVENANCE-001). A generator that
reads a config file, template, or environment value the call site never adds to Inputs or Values silently skips re-running
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
			Concepts: []string{"Terminal", "Run.Suspend", "Run.Confirm", "Run.Text"},
			Rules:    []string{"TERM-001", "TERM-006", "TERM-015", "CONFIRM-001", "CONFIRM-002", "LOG-001"},
			Body: `A Task that resolves before the visibility threshold must not flash a spinner.
Durable notes go through fmt.Fprintln(r.Text(), ...) — never the fmt print functions — while a live region is open: evo clears
the region, writes the line, redraws, atomically. fmt bypassing that path is how frames tear.
r.Confirm(question, ...) quiesces the live region for the whole ask-decide-resolve window before it prompts:
no spinner ever animates while waiting on a human, and the answer is a bool: true, or ⊘ declined / refused off a
terminal (return evo.Refuse; never Failed, never Cancelled) — see common-api for the call.
A severe question (delete/remove/trash/retire/force) always adds evo.Destructive() (CONFIRM-002): it renders the
prompt with an explicit "(destructive)" cue, so a user never approves a remote force-delete or trash mistaking it
for an ordinary yes/no.
Refused (⊘, exit 1) is a distinct terminal state from Failed (✗, exit 2): a human "n", a policy refusal (no
--yes off-TTY), and a protected-branch rule are all ⊘, never ✗.
Resize is a rerender: width is re-read every frame; a narrowed pane recomputes truncation and compact layout
live rather than leaving a wrapped remnant.
Use evo.Terminal with testkit.Screen.

Child processes go through cmd.Stdout = task.Writer() so the live row keeps moving.
Do not inherit os.Stdout (TERM-015). A child that must own the terminal, such as an interactive login, runs inside
r.Suspend(func() error { ... }), which pauses the live region around it.`,
			TokenEstimate: 260,
		},
		{
			ID:       "first-paint",
			Title:    "First paint and the heart contract",
			UseCases: []string{"startup", "latency", "blank", "streaming"},
			Concepts: []string{"Config", "Doing", "Progress"},
			Rules:    []string{"FP-001", "FP-002", "FP-003", "FP-004"},
			Body: `The user is always waiting for input, watching work, or reading a verdict — a blank terminal for the first
one to three seconds of a run is none of those, and reads as a hang.

evo.Main arms first paint before any I/O: declare the first Task as the first statement of run, before
config parsing, git walks, or a network dial. Main + the first declared entity must produce something honest on
screen within 100ms of process start (FP-001). Declaring a Task paints it running (a spinner) immediately —
nothing may hide that first running frame, so a fast bind cannot popup-complete.

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
