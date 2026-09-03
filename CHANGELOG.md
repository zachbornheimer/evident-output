# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project has not reached 1.0 — pre-1.0 API breaks are called out explicitly
below rather than deferred to a major version.

## [Unreleased] — v0.3.0

### The migration hazard: `Main` changed meaning

**If you call `evo.Main` today, read this first.** The old two-argument
`Main(*Output, func(*Output) error)` is now called **`MainWith`** — same
behavior, new name, no code changes needed beyond the rename. But **`Main`
itself now means something different**: it's a new, one-argument function
that runs against the package-level default instance instead of an explicit
`*Output`.

```go
// old (pre-v0.3.0)
out := evo.New(evo.Config{Title: "tool"})
os.Exit(evo.Main(out, run))            // two args: *Output, func(*Output) error

// new: same call, renamed
out := evo.New(evo.Config{Title: "tool"})
os.Exit(evo.MainWith(out, run))        // renamed, unchanged behavior

// new: the front door
evo.Init(evo.Config{Title: "tool"})    // first statement — arms first paint
os.Exit(evo.Main(run))                 // one arg: func() error
```

A mechanical `Main` → `MainWith` rename on every existing call site is
required before upgrading — the compiler will not catch the two-argument
call silently becoming a one-argument call, because the new `Main` also
compiles with a different (and incompatible) signature. There is no
mixed-arity overload; `go build` fails immediately on the old two-argument
call, which is the safe outcome, but grep for `evo.Main(` before upgrading
to confirm every call site was already migrated to `MainWith`.

### Added

- **New front door**: `evo.Init(Config)` installs a package-level default
  `*Output` (mirrors `slog.SetDefault`) and arms first paint before any I/O.
  Package-level `evo.Task`, `evo.Item`, `evo.Group`, `evo.Print`/`Printf`/
  `Println`/`Verbose`, `evo.Confirm`, and `evo.Reason` all delegate to it and
  never panic even if `Init` was skipped (a zero-`Config` instance is
  lazily created). `evo.Task(name)` and `evo.Item`/`evo.Group` are
  get-or-create: repeated calls with the same name return the same handle.
- **`evo.Group(name)`**: a thin front door over the existing `Tasks`
  collection with get-or-create children. Once a group child reaches
  `Failed` or `Cancelled`, every later-declared sibling still unresolved at
  `Finish` auto-resolves to `NotStarted` ("`-  <name>  not started`", dim,
  excluded from the Conclusion) — no caller code required. Explicit
  resolution before `Finish` always wins over the auto-resolution.
- **Mode-free mutation verbs**: `Config.DryRun` / `evo.DryRun()` declared
  once; `TaskHandle.Delete/Create/Update/Remove/Write/Push/Record/
RecordName` render into the task's `Plan` (dry-run) or `Changes` (applied)
  section automatically — the imperative verb is used as-is for `[planned]`
  rows and conjugated to past tense for `[changed]` rows (an irregulars
  table plus a default `+d`/`+ed` rule). No call site ever flips its own
  tense. A `DryRun`-configured `Output` writes an unmissable
  `[dry-run]  no changes will be made` banner immediately at construction,
  and the Conclusion headline reads `Planned` even when the run has no Plan
  section.
- **`evo.Confirm(question, ...)`**: owns the whole ask-decide-resolve
  confirmation gate — quiesces the live region, renders a durable
  `?  <question>  [y/N]` line, reads one line via an injectable
  `Config.Stdin` facade, and resolves to `OK` / `⊘ declined` / `⊘ blocked by
policy` (never a Go error, never `Failed`/`Cancelled`).
  `evo.Destructive()` marks a severe question (delete/remove/trash/retire/
  force) with an explicit "(destructive)" render cue.
- **Skip/keep taxonomy**: `evo.Reason("...")` (get-or-create, typo-safe once
  lifted to a var) plus `TaskHandle.Skipped(reason, name)` /
  `TaskHandle.Kept(reason, name)` accumulate records on the task model; the
  `!  skipped N  (a reasonA, b reasonB)` line is derived at render — never
  hand-assembled — so the reason partition mechanically sums to the
  headline count.
- **`TaskHandle.Each(items)` / `EachN(n)`**: owns absolute loop progress
  (`Progress(completed, total)`), sealed once discovery completes and never
  re-sealed or moved backward on manual retry.
- **`TaskHandle.PhaseWriter()`**: a line-buffered `io.Writer` that turns
  each completed line from a child process into the task's live `Phase`
  text while retaining every byte in the task's `Evidence` ring for
  `DetailTail` evidence. `Task.Evidence()` is get-or-create per task so
  `PhaseWriter` and a later `Evidence()` call share one ring. A live `Phase`
  left unrefreshed for ~10s auto-appends elapsed context (e.g.
  "pushing feat/a — 90s") so a stale spinner is never mistaken for silence.
- **`evo.TruncateNames(names, n)`**: bounds a slice-derived string before it
  reaches `Because`/`Detail`/`Phase`, so a large `strings.Join` can't flood
  the terminal the way unbounded Plan/Changes rows once did.
- **Glyph capability profiles** (`Config.Glyphs` / `evo.Glyphs(...)`,
  `auto|unicode|ascii`): the state-glyph vocabulary (including `⊘` Blocked
  and `■` Cancelled) downgrades to a tightened ASCII table on a non-UTF-8
  interactive terminal; non-interactive output keeps Unicode glyphs
  unconditionally. Selection measures terminal cell width, not rune count.
- **Two-axis conclusion algebra**: `Outcome` is
  `OK | Blocked | Failed | Cancelled`; a warning is not one of those four —
  it renders as its own `!` row and never overrides an otherwise-OK
  headline. It becomes the headline only when a warning is the only content
  in the run.
- **`out.Suspend(func() error)`**: clears the live region, holds it
  invisible for the whole call, and redraws after — for a child process
  that paints its own UI directly on the shared terminal (tty-passthrough).
  Captured or `PhaseWriter`-wired children never need it.
- **Printf-variadic entity names**: `evo.Task`, `Output.Task`, `Tasks.Task`,
  `evo.Group`/`Output.Group`, `Output.Item`, `Scope.Task`/`Scope.Item`, and
  `GroupHandle.Task` accept trailing `args ...any` — with args present, name
  is `fmt.Sprintf(name, args...)`; with none, name passes through unchanged
  (a literal `%` in a plain call site never misfires through `Sprintf`). An
  `evo.ID(...)` (or any other `EntityOption`) may sit anywhere among the
  args and still applies; get-or-create identity keys on the formatted name.
- **MCP review detectors API-032/API-033**: `API-032` catches every
  superseded spelling (`evo.New`/`MainWith` inside `main`, `evo.Cause`,
  `Capture`) with a derived one-line fix using the actual call site's
  identifiers, not just a pointer at the rule. `API-033` flags an entity's
  own name reused verbatim as its skip/verb argument (`out.Item(note).Skip(note)`)
  — the second occurrence carries no information the count/reason partition
  doesn't already show.
- **`TaskHandle.Run(cmd *exec.Cmd) error`**: the subprocess facade —
  sets the task's `Phase` to the command's basename if none is set yet,
  wires `cmd.Stdout`/`cmd.Stderr` through the same `Evidence`/`PhaseWriter`
  plumbing (retained, redacted, live phase per line, `DetailTail` evidence
  on failure), tees rather than replaces any writer the caller already
  wired, and returns the subprocess error verbatim — `Run` never resolves
  the task; the caller still chooses `Done`/`Fail`. `Run` doesn't touch
  `cmd.Stdin` and doesn't `Suspend`; tty passthrough stays the explicit
  `Suspend` path.

### Changed

- **`Task` → `TaskHandle`, `Item` → `ItemHandle`** (pre-1.0 API break): the
  old type names are freed for the new package-level `Task`/`Item`
  functions. Update any type reference (e.g. `func f(t *evo.Task)` →
  `func f(t *evo.TaskHandle)`); method calls on the returned value are
  unaffected.
- **`Visibility` constant `Verbose` → `VisibilityVerbose`** (pre-1.0 API
  break): frees the `Verbose` identifier for the new package-level
  `evo.Verbose()` function. `evo.Print*` visibility gating is otherwise
  unchanged.
- **`Main` re-signature — see "The migration hazard" above.**
- **Verb-return redesign: `Fail`/`Block` are statement-form; `Failf`/`Blockf`
  return a `%w`-wrapped error** (pre-1.0 API break, supersedes the
  error-returning `Fail`/`Block` from the previous unreleased iteration):
  `TaskHandle.Fail`, `TaskHandle.Block`, `ItemHandle.Fail`, and
  `ItemHandle.Block` now return nothing, so a bare `task.Fail("summary")` is
  errcheck-clean and no longer needs `_ =`. `Failf`/`Blockf` use `fmt.Errorf`
  semantics: `return task.Failf("validate policy manifest: %w", err)`
  builds and returns the error in one line — a trailing `": %w"`/`", %w"`
  splits the formatted text into the rendered summary (the text before) and
  an evidence line (the wrapped error's text); without a trailing `%w` the
  whole text is the summary and a `%w` placed elsewhere still feeds
  evidence. `TaskHandle` gains `Block`/`Blockf` (a task can now terminate
  `Blocked`, exit 1, same as `Item`). Both `Fail`/`Block` and their `f`
  variants stay nil-receiver safe.
- **`evo.Cause` is deprecated**: it no longer affects the returned error
  since `Fail`/`Block` are statement-form. Kept as a `ProblemOption` shim so
  existing call sites still compile; migrate to `Failf`/`Blockf`'s trailing
  `%w`.
- **`Capture` renamed to `Evidence`** (`Task.Evidence`/`Item.Evidence`/
  `Output.Evidence`): "Stdout" would lie as a name since it also takes
  stderr and combined writes. `Capture` (type and the three accessor
  methods) is kept as a type alias / thin shim since it shipped in v0.2.16.
  `TaskHandle.Run` and `PhaseWriter` now build on `Evidence` internally.
- **`Problem`'s label/value attachment type renamed `Evidence` → `Attachment`**
  (pre-1.0 API break, forced by the rename above): `Problem.Evidence` keeps
  its field name; the element type is now `evo.Attachment`. This type had
  zero real producers (no `ProblemOption` ever constructed one), so there is
  no deprecated shim — a genuine collision with the process-output sink
  claiming the more central `Evidence` name.
- **`TaskHandle.Skipped`/`Kept` accept a trailing `errs ...error`**: causes
  render as one bounded evidence line under the count row (first cause +
  `"(+N more)"`), full list under `Verbose`; the `(reason, name)`
  aggregation key is untouched. Zero `errs` (every existing call site)
  renders no evidence line.
- **`evo.Reasonf(format, args...)`**: printf-formatted `Reason` name,
  same get-or-create identity rule as `Reason`.
- **Printf-name symmetry**: `Scope.Task`, `Scope.Item`, and
  `GroupHandle.Task` now accept trailing `args ...any` like `Output.Task`/
  `Item` and the package-level facades.

### Fixed

- `writeCollection` renders every resolved child (including `Done`) with
  its summary instead of collapsing to one parent line — group evidence no
  longer disappears once it scrolls out of the live region.
- Plan/Changes effect rows are bounded to a configurable maximum with a dim
  overflow line; the snapshot still retains every record.
- The in-flight `Running` phase/current item is no longer dimmed in plain
  and live projection; dim stays reserved for pending/not-started/evidence/
  overflow rows.
- `already mutated: ...` now renders on `Cancelled`/`Failed` conclusions,
  derived from the Changes ledger (never caller-assembled), with a `none`
  fallback for an empty ledger.

## Migration guide (v0.2.x → v0.3.0)

| Old (v0.2.x)                                                 | New (v0.3.0)                                                                                                   |
| ------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| `evo.Main(out, run)` — two args                              | `evo.MainWith(out, run)` — same behavior, renamed                                                              |
| _(none)_                                                     | `evo.Init(cfg)` + `os.Exit(evo.Main(run))` — new front door, one arg                                           |
| `*evo.Task`                                                  | `*evo.TaskHandle`                                                                                              |
| `*evo.Item`                                                  | `*evo.ItemHandle`                                                                                              |
| `evo.Verbose` (Visibility constant)                          | `evo.VisibilityVerbose`                                                                                        |
| `out.Task(name)` / `out.Item(name)`                          | Unchanged (instance methods); or `evo.Task(name)` / `evo.Item(name)` against the default instance after `Init` |
| `out.Tasks(name)` for named children                         | `out.Tasks(name)` unchanged, or `evo.Group(name)` / `out.Group(name)` for auto-lifecycle NotStarted-on-failure |
| Hand-rolled `[y/N]` stdin prompt                             | `evo.Confirm(question, ...)`                                                                                   |
| Hand-built "skipped N" string                                | `task.Skipped(evo.Reason("..."), name)` / `task.Kept(...)`                                                     |
| Hand-maintained loop counter for `Progress`                  | `task.Each(items)` / `task.EachN(n)`                                                                           |
| Hand-rolled `io.Writer` calling `task.Phase`                 | `task.PhaseWriter()`                                                                                           |
| `strings.Join(names, ", ")` into `Detail`/`Because`/`Phase`  | `evo.TruncateNames(names, n)` first                                                                            |
| Manual `[planned]`/`[changed]` string picking                | `Config.DryRun` + `TaskHandle` mutation verbs (`Delete`/`Create`/…)                                            |
| `task.Fail(summary, evo.Cause(err))` — returned an `error`   | `return task.Failf("summary: %w", err)`                                                                        |
| `item.Block(summary, evo.Cause(err))` — returned an `error`  | `return item.Blockf("summary: %w", err)`                                                                       |
| `task.Capture()` / `item.Capture()` / `out.Capture()`        | `task.Evidence()` / `item.Evidence()` / `out.Evidence()`                                                       |
| `evo.New(cfg)` + `os.Exit(evo.MainWith(out, run))` in `main` | `evo.Init(cfg)` + `os.Exit(evo.Main(run))` — `New`/`MainWith` stay for a genuinely hosted instance             |

See [`docs/guides/teaching-ladder.md`](docs/guides/teaching-ladder.md) for the
full adoption order and [`README.md`](README.md) for the quick start.

## [0.2.16] and earlier

See git history prior to this file's introduction; `evo.PublishedRelease` in
[`release.go`](release.go) is the single source of truth for the currently
published tag.
