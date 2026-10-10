# Evident Output — usage contract

A Go program hands Evo values: what work exists, what it depends on, what
change it wants. Evo decides when the work runs, whether it needs to run at
all, what happened, and how that is shown. The output is always good: the
live frame changes within 100ms; file and process work is skipped or
regenerated from recorded status; Group and Sequence give parallelism without
goroutines; one runtime truth drives TTY, plain text, JSON and JSONL. The bar
is the default output library for Go.

Two idioms, and nothing else to learn:

- a **spec** (several required facts, no headline) is a struct: `Config`,
  `FileSpec`, `ExecSpec`, `EffectSpec`;
- a **message** (one headline, optional detail) is summary first, then
  options: `Problem`, `Confirm`, `Fail`, `Refuse`.

Work is always a function argument, never a struct field. A Task ends by
returning a value.

The tables below are the whole exported surface of package `evo`.
`contract_test.go` fails when the package exports a name that is not here or
lacks a name that is. Symbol forms: `Name` is a function, type, constant or
variable; `Type.Name` is a method or a struct field.

## 1. Run

```go
func main() { os.Exit(evo.Main(run)) }

func run(ctx context.Context, r *evo.Run) error { ... }
```

`Main` wires SIGINT/SIGTERM to cancellation, runs, finishes and returns the
exit code; it never exits. `Run` is the same for embedding: the caller owns
the context and the exit. A zero `Config` is complete: `Name` defaults to the
binary name and `Output` to Human on a TTY, Plain otherwise.

| Symbol                   | Kind   | Meaning                                                                    |
| ------------------------ | ------ | -------------------------------------------------------------------------- |
| `Main`                   | func   | `Main(run RunFunc, cfg ...Config) int` — run with signals, return exit     |
| `Run`                    | func   | `Run(ctx, cfg Config, run RunFunc) Result` — run without signals           |
| `RunFunc`                | type   | `func(ctx context.Context, r *Run) error`, the application body            |
| `Config`                 | type   | the run's settings; zero value is valid                                    |
| `Config.Name`            | field  | what ran and on what, e.g. `zq prune ~/repo`                               |
| `Config.DryRun`          | field  | plan every change, run no mutation                                         |
| `Config.Verbose`         | field  | show hidden Facts and per-reason items                                     |
| `Config.Output`          | field  | `Output` selector                                                          |
| `Config.Debug`           | field  | `slog.Level` of the debug journal                                          |
| `Config.MaxConcurrency`  | field  | bound on executing callbacks; zero means GOMAXPROCS                        |
| `Config.State`           | field  | where recorded status lives                                                |
| `Config.Facades`         | field  | system boundaries, replaceable in tests                                    |
| `State`                  | type   | manifest location                                                          |
| `State.Dir`              | field  | directory; zero means the platform state dir                               |
| `State.AppID`            | field  | namespace; zero means the binary name                                      |
| `Facades`                | type   | every system boundary Evo touches                                          |
| `Facades.Stdin`          | field  | `io.Reader`                                                                |
| `Facades.Stdout`         | field  | `io.Writer`                                                                |
| `Facades.Stderr`         | field  | `io.Writer`                                                                |
| `Facades.Clock`          | field  | `Clock`                                                                    |
| `Facades.Terminal`       | field  | `Terminal`                                                                 |
| `Facades.Redactor`       | field  | `Redactor`                                                                 |
| `Facades.Process`        | field  | `Process`                                                                  |
| `Facades.FS`             | field  | `FS`                                                                       |
| `Clock`                  | type   | time source                                                                |
| `Terminal`               | type   | terminal driver                                                            |
| `Redactor`               | type   | secret redaction over captured text                                        |
| `Process`                | type   | process spawner                                                            |
| `FS`                     | type   | filesystem                                                                 |
| `Run`                    | type   | the root of one invocation; a `Container`                                  |
| `Run.Task`               | method | declare a root Task                                                        |
| `Run.Group`              | method | declare a root Group                                                       |
| `Run.Sequence`           | method | declare a root Sequence                                                    |
| `Run.Fact`               | method | run-scoped structured information                                          |
| `Run.Summary`            | method | the one line the application contributes before the Conclusion             |
| `Run.Confirm`            | method | `Confirm(question string, opts ...ConfirmOption) bool`; decline is Refused |
| `Run.Suspend`            | method | `Suspend(fn func() error) error`; pause the live region around `fn`        |
| `Run.Text`               | method | `io.Writer` for free human text, routed with the presentation              |
| `Run.ResultWriter`       | method | `io.Writer` for the stdout data payload                                    |
| `Run.SlogHandler`        | method | `slog.Handler` that writes into the debug journal                          |
| `Result`                 | type   | what `Run` returns                                                         |
| `Result.Conclusion`      | field  | the verdict                                                                |
| `Result.Err`             | field  | the error the application body returned                                    |
| `Result.ExitCode`        | method | 0 OK · 1 Refused · 2 Failed · 130 Cancelled                                |
| `Conclusion`             | type   | one verdict for UI and exit                                                |
| `Conclusion.Outcome`     | field  | the worst Task outcome                                                     |
| `Conclusion.Changed`     | field  | any mutation committed                                                     |
| `Conclusion.DryRun`      | field  | planned tense                                                              |
| `Conclusion.Explanation` | field  | why, when Refused, Failed or Cancelled                                     |

## 2. Work

```go
repo := r.Group("repository")                // children may overlap
seq  := r.Sequence("consolidate packages")   // children run in declaration order
t    := repo.Task("prune landed branches")   // one atomic promise; never a container

t.After(other).Verify(isDone).Define(func(ctx context.Context) error { ... })

inv := evo.Compute(seq.Task("discover packages"), discover)   // a Task with a value
seq.Group("centralize packages").After(inv).Build(func(g *evo.Group) {
    for _, p := range inv.Get().Packages {
        g.Task("centralize " + p.Name).Define(p.Centralize)
    }
})
```

Sibling names are unique per parent. Evo owns goroutines, bounded
concurrency, dependency readiness and cancellation. `Define` submits and never
runs inline; a nil return succeeds. `Verify` true before `Define` resolves
Satisfied and the callback never runs; false after a successful callback
fails the Task. A Failed, Refused, Excluded or Cancelled predecessor leaves
its dependents NotStarted. `Get` is valid only from work ordered after the
producer by Sequence position or `After`; an unordered `Get` fails the run
with `ErrComputedUnordered` in every mode. `Compute` on a Task that has a
`Verify` is `ErrInvalidConfig` at declaration: a satisfied `Verify` skips the
callback, so a value would exist only when the world says so. A `Build` runs
once when its container becomes eligible, declares topology only, and closes
the container's declaration phase; declaring work from inside a Task's
callback is misuse.

| Symbol              | Kind   | Meaning                                                                        |
| ------------------- | ------ | ------------------------------------------------------------------------------ |
| `Task`              | type   | one independently meaningful schedulable promise                               |
| `Task.After`        | method | `After(preds ...any) *Task`; a Task, Group, Sequence or Computed               |
| `Task.Verify`       | method | `Verify(fn func(context.Context) (bool, error)) *Task`; read-only              |
| `Task.Define`       | method | `Define(fn func(context.Context) error, opts ...DefineOption) *Task`           |
| `Task.Doing`        | method | current item, live only                                                        |
| `Task.Progress`     | method | `Progress(completed, total int) *Task`                                         |
| `Task.Writer`       | method | `io.Writer` whose lines become the live tail and Capture                       |
| `Task.Fact`         | method | structured information learned                                                 |
| `Task.Summary`      | method | the settled row's headline                                                     |
| `Task.Problem`      | method | `Problem(summary string, opts ...ProblemOption) *Task`; non-fatal              |
| `Group`             | type   | independent children; eligible siblings may run concurrently                   |
| `Group.Task`        | method | declare a child Task                                                           |
| `Group.Group`       | method | declare a child Group                                                          |
| `Group.Sequence`    | method | declare a child Sequence                                                       |
| `Group.After`       | method | `After(preds ...any) *Group`                                                   |
| `Group.Build`       | method | `Build(fn func(*Group))`; topology once eligible                               |
| `Group.Summary`     | method | the container row's headline                                                   |
| `Sequence`          | type   | ordered children; one Running child                                            |
| `Sequence.Task`     | method | declare a child Task                                                           |
| `Sequence.Group`    | method | declare a child Group                                                          |
| `Sequence.Sequence` | method | declare a child Sequence                                                       |
| `Sequence.After`    | method | `After(preds ...any) *Sequence`                                                |
| `Sequence.Build`    | method | `Build(fn func(*Sequence))`                                                    |
| `Sequence.Summary`  | method | the container row's headline                                                   |
| `Container`         | type   | interface `{Task; Group; Sequence}` satisfied by Run, Group, Sequence          |
| `Compute`           | func   | `Compute[T any](task *Task, fn func(context.Context) (T, error)) *Computed[T]` |
| `Computed`          | type   | the value a successful Task produced                                           |
| `Computed.Get`      | method | `Get() T`                                                                      |
| `DefineOption`      | type   | option for `Define`                                                            |
| `CleanStop`         | func   | `CleanStop(signals ...os.Signal) DefineOption`; these end the Task Succeeded   |

## 3. Change

```go
evo.File(ctx, evo.FileSpec{Path: p, Contents: b, Mode: 0o644, Inputs: []string{"gen.py"}})
res, err := evo.Exec(ctx, evo.ExecSpec{Argv: []string{"go", "build"}, Outputs: []string{"bin/x"}})
evo.Patch(ctx, unifiedDiff)
evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "branch", Quantity: 40}, del)
return evo.PartialEffect(committed, err)
```

Every call takes the Define `ctx`. File and Exec are Satisfied when their
Inputs, Values and recorded outputs are current; opaque Define work runs
every Run. Dry run plans every mutation and runs no callback. Each mutation
appears once in the ledger as `[planned]` or `[changed]`; per-item mutations
with one Verb and Object fold into one counted row. Overlapping Paths wait
for each other; two Resources overlap only by equal name; holding two claims
at once is misuse. A Patch whose source changed since it was read fails with
`ErrStaleBasis` instead of overwriting. Uncertain or missing recorded status
re-executes; it never reports Satisfied.

| Symbol                 | Kind  | Meaning                                                                             |
| ---------------------- | ----- | ----------------------------------------------------------------------------------- |
| `File`                 | func  | `File(ctx, FileSpec) error`; desired state of one file                              |
| `FileSpec`             | type  |                                                                                     |
| `FileSpec.Path`        | field |                                                                                     |
| `FileSpec.Contents`    | field | nil leaves contents unmanaged                                                       |
| `FileSpec.Mode`        | field | zero leaves mode unmanaged                                                          |
| `FileSpec.Inputs`      | field | paths whose content decides freshness                                               |
| `FileSpec.Values`      | field | named values that decide freshness                                                  |
| `Exec`                 | func  | `Exec(ctx, ExecSpec) (ExecResult, error)`                                           |
| `ExecSpec`             | type  |                                                                                     |
| `ExecSpec.Argv`        | field | executable and arguments, no shell                                                  |
| `ExecSpec.Dir`         | field |                                                                                     |
| `ExecSpec.Env`         | field | explicit entries only; they enter freshness                                         |
| `ExecSpec.Inputs`      | field |                                                                                     |
| `ExecSpec.Values`      | field |                                                                                     |
| `ExecSpec.Outputs`     | field | declared outputs; none means always run                                             |
| `ExecResult`           | type  |                                                                                     |
| `ExecResult.Ran`       | field | false on Satisfied, dry run, or spawn failure                                       |
| `ExecResult.ExitCode`  | field |                                                                                     |
| `ExecResult.Stdout`    | field | bounded, redacted tail                                                              |
| `ExecResult.Stderr`    | field | bounded, redacted tail                                                              |
| `ExecResult.Truncated` | field |                                                                                     |
| `Patch`                | func  | `Patch(ctx, diff []byte) error`; derive from a unified diff and commit through File |
| `Effect`               | func  | `Effect(ctx, EffectSpec, fn func(context.Context) error) error`                     |
| `EffectSpec`           | type  |                                                                                     |
| `EffectSpec.Verb`      | field |                                                                                     |
| `EffectSpec.Object`    | field | singular noun; Evo pluralizes                                                       |
| `EffectSpec.Quantity`  | field | > 0                                                                                 |
| `EffectSpec.Path`      | field | filesystem claim, overlaps by ancestry; optional                                    |
| `EffectSpec.Resource`  | field | logical claim, overlaps by equal name; optional                                     |
| `Verb`                 | type  | closed set                                                                          |
| `Create`               | const |                                                                                     |
| `Update`               | const |                                                                                     |
| `Delete`               | const |                                                                                     |
| `PartialEffect`        | func  | `PartialEffect(committed int, err error) error`; part committed                     |

## 4. Endings and findings

```go
return err                                              // Failed
return evo.Fail("lint failed", evo.Detail(out))         // Failed, with structure
return evo.Refuse("no remote configured", evo.Next(evo.Action{Label: "add a remote"}))
return evo.Exclude("protected")                         // Excluded, folds into the tally
t.Problem("protected ref", evo.On("origin/main"), evo.Code("protected"))  // non-fatal
```

A Task ends exactly once, by returning. Problems never end a Task.

| Symbol           | Kind  | Meaning                                                                                          |
| ---------------- | ----- | ------------------------------------------------------------------------------------------------ |
| `Fail`           | func  | `Fail(summary string, opts ...ProblemOption) error`; ran and broke                               |
| `Refuse`         | func  | `Refuse(summary string, opts ...ProblemOption) error`; a precondition stopped it before mutation |
| `Exclude`        | func  | `Exclude(reason string) error`; policy chose not to run it                                       |
| `ProblemOption`  | type  |                                                                                                  |
| `Detail`         | func  | `Detail(text string) Detail`; longer explanation                                                 |
| `Code`           | func  | stable machine code                                                                              |
| `On`             | func  | the subject: a name, or `path:line:col`                                                          |
| `Next`           | func  | `Next(a Action) ProblemOption`; recommended next step                                            |
| `Action`         | type  |                                                                                                  |
| `Action.Command` | field | argv a machine can run                                                                           |
| `Action.Label`   | field | text only a human can act on                                                                     |
| `Outcome`        | type  | closed set                                                                                       |
| `Succeeded`      | const | ran and succeeded                                                                                |
| `Satisfied`      | const | requested state already true; nothing ran                                                        |
| `Excluded`       | const | policy chose not to run it                                                                       |
| `Refused`        | const | a precondition stopped it; exit 1                                                                |
| `Failed`         | const | ran and failed; exit 2                                                                           |
| `Cancelled`      | const | interrupted; exit 130                                                                            |

## 5. Confirm

```go
if !r.Confirm("delete 40 branches?", evo.Destructive(), evo.NonInteractive("--yes")) {
    return evo.Refuse("declined")
}
```

| Symbol           | Kind | Meaning                                                  |
| ---------------- | ---- | -------------------------------------------------------- |
| `ConfirmOption`  | type |                                                          |
| `Destructive`    | func | default answer is no                                     |
| `NonInteractive` | func | `NonInteractive(argv ...string)`; how to pass unattended |

`Detail` is also a `ConfirmOption`.

## 6. Output

| Symbol   | Kind  | Meaning                                                            |
| -------- | ----- | ------------------------------------------------------------------ |
| `Output` | type  | encoding selector                                                  |
| `Human`  | const | live TTY presentation                                              |
| `Plain`  | const | durable text, one line per event, no live region                   |
| `JSON`   | const | one `evo.run` document on stdout at Finish, presentation on stderr |
| `JSONL`  | const | `evo.event` lines on stdout as they occur                          |

Environment, applied only where `Config` is zero: `EVO_OUTPUT=human|plain|json|jsonl`,
`EVO_VERBOSE=1`, `EVO_DEBUG=debug|info|warn|error`, `NO_COLOR`, `FORCE_COLOR`.
Machine output is never inferred from a piped stdout.

## 7. Errors

| Symbol                 | Meaning                                        |
| ---------------------- | ---------------------------------------------- |
| `ErrExecNonzeroExit`   | wrapped by Exec on a nonzero exit              |
| `ErrExecOutputMissing` | a declared Output is absent after exit 0       |
| `ErrStaleBasis`        | a Patch source changed between read and commit |
| `ErrPatchDoesNotApply` | the diff does not match the source             |
| `ErrComputedUnordered` | `Get` from work not ordered after the producer |

## 8. What Evo promises

- The live frame changes within 100ms of entering Running and at least every 100ms after, with a stable parent row and the current item beneath it.
- One runtime truth drives Human, Plain, `evo.run` and `evo.event`; no projection invents state another lacks.
- Facts are hidden at normal verbosity and always in machine output; routine rows collapse; passing verification is silent, failing verification is shown.
- Per-item Excluded Tasks fold into one tally; per-item Effects with one Verb and Object fold into one counted row; JSON keeps every item.
- Cancellation preserves committed truth and never implies rollback.
- Misuse is recorded with a remedy, appears in `Result`, and never crashes the program.
- Recorded status commits atomically; uncertainty re-executes and never reports Satisfied.
