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
  text while retaining every byte in the task's `Capture` ring for
  `DetailTail` evidence. `Task.Capture()` is now get-or-create per task so
  `PhaseWriter` and a later `Capture()` call share one ring. A live `Phase`
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

| Old (v0.2.x)                                                | New (v0.3.0)                                                                                                   |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `evo.Main(out, run)` — two args                             | `evo.MainWith(out, run)` — same behavior, renamed                                                              |
| _(none)_                                                    | `evo.Init(cfg)` + `os.Exit(evo.Main(run))` — new front door, one arg                                           |
| `*evo.Task`                                                 | `*evo.TaskHandle`                                                                                              |
| `*evo.Item`                                                 | `*evo.ItemHandle`                                                                                              |
| `evo.Verbose` (Visibility constant)                         | `evo.VisibilityVerbose`                                                                                        |
| `out.Task(name)` / `out.Item(name)`                         | Unchanged (instance methods); or `evo.Task(name)` / `evo.Item(name)` against the default instance after `Init` |
| `out.Tasks(name)` for named children                        | `out.Tasks(name)` unchanged, or `evo.Group(name)` / `out.Group(name)` for auto-lifecycle NotStarted-on-failure |
| Hand-rolled `[y/N]` stdin prompt                            | `evo.Confirm(question, ...)`                                                                                   |
| Hand-built "skipped N" string                               | `task.Skipped(evo.Reason("..."), name)` / `task.Kept(...)`                                                     |
| Hand-maintained loop counter for `Progress`                 | `task.Each(items)` / `task.EachN(n)`                                                                           |
| Hand-rolled `io.Writer` calling `task.Phase`                | `task.PhaseWriter()`                                                                                           |
| `strings.Join(names, ", ")` into `Detail`/`Because`/`Phase` | `evo.TruncateNames(names, n)` first                                                                            |
| Manual `[planned]`/`[changed]` string picking               | `Config.DryRun` + `TaskHandle` mutation verbs (`Delete`/`Create`/…)                                            |

See [`docs/guides/teaching-ladder.md`](docs/guides/teaching-ladder.md) for the
full adoption order and [`README.md`](README.md) for the quick start.

## [0.2.16] and earlier

See git history prior to this file's introduction; `evo.PublishedRelease` in
[`release.go`](release.go) is the single source of truth for the currently
published tag.
