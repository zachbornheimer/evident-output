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
func main() { os.Exit(evo.Main(evo.Config{}, run)) }

func run(ctx context.Context, r *evo.Run) error { ... }
```

`Main` wires SIGINT/SIGTERM to cancellation, runs, finishes and returns the
exit code; it never exits. It honours both signals until the run has
finished, including while declared work still runs after the body returned,
and a signal ends the run Cancelled with exit 130. `Execute` runs without
signals, for embedding: the caller owns the context and the exit. `Execute` binds that
context for the whole run, including while declared work still runs after
the body returned: once it is done, every running callback's context is
done and the run ends Cancelled with exit 130. A zero `Config` is complete: `Name` defaults to the
binary name and `Output` to Human on a TTY, Plain otherwise. `Config.Name`
titles the run in Human and Plain and is `name` in `evo.run` and in the
`run.started` event; `Config.Verbose` shows Facts in Human and Plain exactly
as `EVO_VERBOSE=1` does.

Every row's leading Go is the declaration, checked by
`TestContractSignaturesTypeCheck`. Every exported field, parameter and result
names only exported `evo` or standard-library types, and `Clock`,
`Terminal`, `Redactor`, `Process` and `FS` are interfaces with only exported
methods, so a test can implement each one.

When the run is Refused, Failed or Cancelled, `Conclusion.Explanation` is the
deciding ending: the summary or error of the Task (or run body) whose Outcome
became the Conclusion's, or what interrupted the run. When several Tasks end
the run's way, the earliest-settled ending decides, whatever the declaration
order. `evo.run` and
`run.finished` carry the same text as `explanation`. It is empty for
Succeeded, Satisfied and Excluded.

| Symbol                   | Kind   | Meaning                                                                                     |
| ------------------------ | ------ | ------------------------------------------------------------------------------------------- |
| `Main`                   | func   | `Main(cfg Config, run RunFunc) int` — run with signals, return exit                         |
| `Execute`                | func   | `Execute(ctx context.Context, cfg Config, run RunFunc) Result` — run without signals        |
| `RunFunc`                | type   | `func(ctx context.Context, r *Run) error`, the application body                             |
| `Config`                 | type   | the run's settings; zero value is valid                                                     |
| `Config.Name`            | field  | `string`; what ran and on what, e.g. `zq prune ~/repo`                                      |
| `Config.DryRun`          | field  | `bool`; plan every change, run no mutation                                                  |
| `Config.Verbose`         | field  | `bool`; show hidden Facts and per-reason items                                              |
| `Config.Output`          | field  | `Output` selector                                                                           |
| `Config.Debug`           | field  | `slog.Leveler`; the lowest level the debug journal keeps; nil keeps it off                  |
| `Config.MaxConcurrency`  | field  | `int`; bound on executing callbacks; zero means GOMAXPROCS                                  |
| `Config.State`           | field  | `State`; where recorded status lives                                                        |
| `Config.Facades`         | field  | `Facades`; system boundaries, replaceable in tests                                          |
| `State`                  | type   | manifest location                                                                           |
| `State.Dir`              | field  | `string`; directory; zero means the platform state dir                                      |
| `State.AppID`            | field  | `string`; namespace; zero means the binary name                                             |
| `Facades`                | type   | every system boundary Evo touches                                                           |
| `Facades.Stdin`          | field  | `io.Reader`                                                                                 |
| `Facades.Stdout`         | field  | `io.Writer`                                                                                 |
| `Facades.Stderr`         | field  | `io.Writer`                                                                                 |
| `Facades.Clock`          | field  | `Clock`                                                                                     |
| `Facades.Terminal`       | field  | `Terminal`                                                                                  |
| `Facades.Redactor`       | field  | `Redactor`                                                                                  |
| `Facades.Process`        | field  | `Process`                                                                                   |
| `Facades.FS`             | field  | `FS`                                                                                        |
| `Clock`                  | type   | time source                                                                                 |
| `Terminal`               | type   | terminal driver                                                                             |
| `Redactor`               | type   | secret redaction over captured text                                                         |
| `Process`                | type   | process spawner                                                                             |
| `FS`                     | type   | filesystem                                                                                  |
| `Run`                    | type   | the root of one invocation; a `Container`                                                   |
| `Run.Task`               | method | `Task(name string) *Task`; declare a root Task                                              |
| `Run.Group`              | method | `Group(name string) *Group`; declare a root Group                                           |
| `Run.Sequence`           | method | `Sequence(name string) *Sequence`; declare a root Sequence                                  |
| `Run.Fact`               | method | `Fact(name, value string) *Run`; run-scoped structured information                          |
| `Run.Summary`            | method | `Summary(text string) *Run`; the application's one line, printed before every row           |
| `Run.Confirm`            | method | `Confirm(question string, opts ...ConfirmOption) bool`; decline is Refused                  |
| `Run.Suspend`            | method | `Suspend(fn func() error) error`; pause the live region around `fn`                         |
| `Run.Text`               | method | `Text() io.Writer`; free human text, routed with the presentation                           |
| `Run.ResultWriter`       | method | `ResultWriter() io.Writer`; the stdout data payload                                         |
| `Run.SlogHandler`        | method | `SlogHandler() slog.Handler`; writes into the debug journal                                 |
| `Result`                 | type   | what `Execute` returns                                                                      |
| `Result.Conclusion`      | field  | `Conclusion`; the verdict                                                                   |
| `Result.Err`             | field  | `error`; the error the application body returned, joined with the first misuse              |
| `Result.ExitCode`        | method | `ExitCode() int`; 0 Succeeded, Satisfied or Excluded · 1 Refused · 2 Failed · 130 Cancelled |
| `Conclusion`             | type   | one verdict for UI and exit                                                                 |
| `Conclusion.Outcome`     | field  | `Outcome`; the worst Task outcome                                                           |
| `Conclusion.Changed`     | field  | `bool`; any mutation committed                                                              |
| `Conclusion.DryRun`      | field  | `bool`; planned tense                                                                       |
| `Conclusion.Explanation` | field  | `string`; the deciding ending when Refused, Failed or Cancelled; empty otherwise            |

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

`After` takes `Predecessor`, a sealed interface that only `*Task`, `*Group`,
`*Sequence` and `*Computed[T]` implement, so passing anything else does not
compile. `After(group)` waits for the members declared before the `After` call;
members declared after it never gate the edge. Naming a Group that has no
members yet waits for every member it later gets. Sibling names are unique per parent. Evo owns goroutines, bounded
concurrency, dependency readiness and cancellation. `Define` submits and never
runs inline; a nil return succeeds. `Verify` true before `Define` resolves
Satisfied and the callback never runs; false after a successful callback
fails the Task. A Failed, Refused, Excluded or Cancelled predecessor leaves
its dependents NotStarted. A NotStarted Task's summary names the Task that
stopped it. `After(group)` and `After(sequence)` wait on the container's own
outcome, which folds an Excluded member into success, so only a direct
`After(task)` on an Excluded Task leaves its dependent NotStarted. `Get` is valid only from work ordered after the
producer by Sequence position or `After`; an unordered `Get` fails the run
with `ErrComputedUnordered` in every mode. A goroutine a callback started is
the callback's hands: its `Get` is judged as the callback's, as
far as the runtime names who started it. Attribution ends at a goroutine
that exited before anything asked about it, and at one first seen after the
run's bounded allowance of lookups (eight for each callback or container builder
running, shared by those running together), so a `Get` through such a
goroutine is not caught. A goroutine that a finished nested Task left running
is found by a change in the count of the process's running goroutines, so it
can be missed when as many other goroutines end in the same moment.
`Compute` on a Task that has a
`Verify` is `ErrInvalidConfig` at declaration (section 7): a satisfied `Verify` skips the
callback, so a value would exist only when the world says so. A `Build` runs
once when its container becomes eligible, declares topology only, and closes
the container's declaration phase; declaring work from inside a Task's
callback is misuse. A signal named in `CleanStop`, arriving while that Task
runs, cancels only that Task's context and ends it Succeeded; it does not
cancel the run, so a run with nothing else left exits 0. A signal it does not
name still cancels the run.

| Symbol              | Kind   | Meaning                                                                        |
| ------------------- | ------ | ------------------------------------------------------------------------------ |
| `Task`              | type   | one independently meaningful schedulable promise                               |
| `Predecessor`       | type   | sealed interface After accepts: Task, Group, Sequence or Computed              |
| `Task.After`        | method | `After(preds ...Predecessor) *Task`; a Task, Group, Sequence or Computed       |
| `Task.Verify`       | method | `Verify(fn func(context.Context) (bool, error)) *Task`; read-only              |
| `Task.Define`       | method | `Define(fn func(context.Context) error, opts ...DefineOption) *Task`           |
| `Task.Doing`        | method | `Doing(text string, args ...any) *Task`; current item, live only               |
| `Task.Progress`     | method | `Progress(completed, total int) *Task`                                         |
| `Task.Writer`       | method | `Writer() io.Writer`; its lines become the live tail and Capture               |
| `Task.Fact`         | method | `Fact(name, value string) *Task`; structured information learned               |
| `Task.Summary`      | method | `Summary(text string) *Task`; the settled row's headline                       |
| `Task.Problem`      | method | `Problem(summary string, opts ...ProblemOption) *Task`; non-fatal              |
| `Group`             | type   | independent children; eligible siblings may run concurrently                   |
| `Group.Task`        | method | `Task(name string) *Task`; declare a child Task                                |
| `Group.Group`       | method | `Group(name string) *Group`; declare a child Group                             |
| `Group.Sequence`    | method | `Sequence(name string) *Sequence`; declare a child Sequence                    |
| `Group.After`       | method | `After(preds ...Predecessor) *Group`                                           |
| `Group.Build`       | method | `Build(fn func(*Group))`; topology once eligible                               |
| `Group.Summary`     | method | `Summary(text string) *Group`; the container row's headline                    |
| `Sequence`          | type   | ordered children; one Running child                                            |
| `Sequence.Task`     | method | `Task(name string) *Task`; declare a child Task                                |
| `Sequence.Group`    | method | `Group(name string) *Group`; declare a child Group                             |
| `Sequence.Sequence` | method | `Sequence(name string) *Sequence`; declare a child Sequence                    |
| `Sequence.After`    | method | `After(preds ...Predecessor) *Sequence`                                        |
| `Sequence.Build`    | method | `Build(fn func(*Sequence))`                                                    |
| `Sequence.Summary`  | method | `Summary(text string) *Sequence`; the container row's headline                 |
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
every Run. An Exec with no Outputs runs every Run and is recorded as ran in
`operations`, never as a change: it leaves `Conclusion.Changed` and the band
untouched. A Task whose File and Exec calls were all current, and which
committed no Effect, settles Satisfied, not Succeeded, with no `Verify`. File and Exec fingerprint the content of every Input and every
Value, so a changed Input byte or Value runs the operation again on the next
Run, even when a File's Contents is unchanged. An Output whose bytes no
longer match what the last Run recorded has drifted, and its producer runs
again to restore it. Cascade is by content: a step whose Input is another
step's Output runs again only when those bytes changed, not because its
producer ran. Dry run plans every mutation and runs no callback. Each mutation
appears once in the ledger as `[planned]` or `[changed]`; per-item mutations
with one Verb and Object fold into one counted row. `EffectSpec.Path` claims
that path for the Effect's callback, so Effects whose Paths are equal or one
contains the other wait for each other; two Resources overlap only by equal
name; an `EffectSpec` that sets both `Path` and `Resource` is refused
(section 7) and its callback never runs; holding two claims at once is
misuse. Two Tasks that declare the same Output both fail, but the conflict
is found at the second claim of that Output path, so the first Task's command
may already have run and rewritten it; the next Run restores it. A Patch whose
source changed since it was read fails with
`ErrStaleBasis` instead of overwriting. Uncertain or missing recorded status
re-executes; it never reports Satisfied.

| Symbol                 | Kind  | Meaning                                                                                                     |
| ---------------------- | ----- | ----------------------------------------------------------------------------------------------------------- |
| `File`                 | func  | `File(ctx context.Context, spec FileSpec) error`; desired state of one file                                 |
| `FileSpec`             | type  |                                                                                                             |
| `FileSpec.Path`        | field | `string`; relative paths resolve against the workspace                                                      |
| `FileSpec.Contents`    | field | `[]byte`; nil leaves contents unmanaged                                                                     |
| `FileSpec.Mode`        | field | `fs.FileMode`; zero leaves mode unmanaged                                                                   |
| `FileSpec.Inputs`      | field | `[]string`; paths whose content decides freshness                                                           |
| `FileSpec.Values`      | field | `map[string]string`; named values that decide freshness                                                     |
| `Exec`                 | func  | `Exec(ctx context.Context, spec ExecSpec) (ExecResult, error)`                                              |
| `ExecSpec`             | type  |                                                                                                             |
| `ExecSpec.Argv`        | field | `[]string`; executable and arguments, no shell                                                              |
| `ExecSpec.Dir`         | field | `string`; working directory; relative `Outputs` resolve against it, relative `Inputs` against the workspace |
| `ExecSpec.Env`         | field | `map[string]string`; explicit entries only; they enter freshness                                            |
| `ExecSpec.Inputs`      | field | `[]string`; paths whose content decides freshness                                                           |
| `ExecSpec.Values`      | field | `map[string]string`; named values that decide freshness                                                     |
| `ExecSpec.Outputs`     | field | `[]string`; declared outputs; none means always run                                                         |
| `ExecResult`           | type  |                                                                                                             |
| `ExecResult.Ran`       | field | `bool`; false on Satisfied, dry run, or spawn failure                                                       |
| `ExecResult.ExitCode`  | field | `int`                                                                                                       |
| `ExecResult.Stdout`    | field | `string`; bounded, redacted tail                                                                            |
| `ExecResult.Stderr`    | field | `string`; bounded, redacted tail                                                                            |
| `ExecResult.Truncated` | field | `bool`                                                                                                      |
| `Patch`                | func  | `Patch(ctx context.Context, diff []byte) error`; derive from a unified diff and commit through File         |
| `Effect`               | func  | `Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error`                        |
| `EffectSpec`           | type  |                                                                                                             |
| `EffectSpec.Verb`      | field | `Verb`                                                                                                      |
| `EffectSpec.Object`    | field | `string`; singular noun; Evo pluralizes                                                                     |
| `EffectSpec.Quantity`  | field | `int`; > 0; zero is refused (section 7)                                                                     |
| `EffectSpec.Path`      | field | `string`; filesystem claim, overlaps by ancestry; optional                                                  |
| `EffectSpec.Resource`  | field | `string`; logical resource name, overlaps by equal name; optional                                           |
| `Verb`                 | type  | closed set: no string converts to a Verb, and any value but these three is refused                          |
| `Create`               | const | `Create Verb`                                                                                               |
| `Update`               | const | `Update Verb`                                                                                               |
| `Delete`               | const | `Delete Verb`                                                                                               |
| `PartialEffect`        | func  | `PartialEffect(committed int, err error) error`; part committed                                             |

## 4. Endings and findings

```go
return err                                              // Failed
return evo.Fail("lint failed", evo.Detail(out))         // Failed, with structure
return evo.Refuse("no remote configured", evo.Next(evo.Action{Label: "add a remote"}))
return evo.Exclude("protected")                         // Excluded, folds into the tally
t.Problem("protected ref", evo.On("origin/main"), evo.Code("protected"))  // non-fatal
```

A Task ends exactly once, by returning. Problems never end a Task: a Task
that records Problems keeps the Outcome its return gives, and the Problems
stay attached, so a Problem and a nil return are Succeeded with exit 0. The
value a Define callback or the run body returns settles its Outcome, wrapped
or not: nil is Succeeded, `Refuse` Refused, `Exclude` Excluded, and `Fail` or
any other error Failed. The exit code follows the Conclusion: 0 for
Succeeded, Satisfied and Excluded, 1 for Refused, 2 for Failed, 130 for
Cancelled. `Fail`, `Refuse` and `Exclude` record every `ProblemOption` on the
Problem their ending creates.

`Outcome` is the only verdict type. Its six values appear unchanged as each
Task's outcome, as `Conclusion.Outcome`, and as `outcome` in `evo.run` and
`evo.event`; Human and Plain name every Outcome but Succeeded in its own
word. A NotStarted Task has no Outcome, so both machine projections omit its
`outcome`. No other verdict spelling exists.

Human and Plain end with one band, `[<headline>]  <name>`. The headline is the
Conclusion's Outcome, except that a Succeeded run reads `changed` when it
committed a mutation and `planned` in a dry run. The band adds `· partial`
when some declared Task never reached an Outcome: a predecessor or an earlier
Sequence step left it NotStarted, or the run ended while it was still pending
or running. `· partial` qualifies the headline and never replaces it, so
`[failed · partial]` is still exit 2. A Cancelled band omits it, since
Cancelled already says the run stopped short.

| Symbol           | Kind  | Meaning                                                                                          |
| ---------------- | ----- | ------------------------------------------------------------------------------------------------ |
| `Fail`           | func  | `Fail(summary string, opts ...ProblemOption) error`; ran and broke                               |
| `Refuse`         | func  | `Refuse(summary string, opts ...ProblemOption) error`; a precondition stopped it before mutation |
| `Exclude`        | func  | `Exclude(reason string, opts ...ProblemOption) error`; policy chose not to run it                |
| `ProblemOption`  | type  | sealed interface                                                                                 |
| `Detail`         | func  | `Detail(text string) interface{ ProblemOption; ConfirmOption }`; longer explanation              |
| `Code`           | func  | `Code(value string) ProblemOption`; stable machine code                                          |
| `On`             | func  | `On(subject string) ProblemOption`; the subject: a name, or `path:line:col`                      |
| `Next`           | func  | `Next(a Action) ProblemOption`; recommended next step                                            |
| `Action`         | type  |                                                                                                  |
| `Action.Command` | field | `[]string`; argv a machine can run                                                               |
| `Action.Label`   | field | `string`; text only a human can act on                                                           |
| `Outcome`        | type  | closed set; the only verdict type                                                                |
| `Succeeded`      | const | `Succeeded Outcome`; ran and succeeded; exit 0                                                   |
| `Satisfied`      | const | `Satisfied Outcome`; requested state already true; nothing ran; exit 0                           |
| `Excluded`       | const | `Excluded Outcome`; policy chose not to run it; exit 0                                           |
| `Refused`        | const | `Refused Outcome`; a precondition stopped it; exit 1                                             |
| `Failed`         | const | `Failed Outcome`; ran and failed; exit 2                                                         |
| `Cancelled`      | const | `Cancelled Outcome`; interrupted; exit 130                                                       |

## 5. Confirm

```go
if !r.Confirm("delete 40 branches?", evo.Destructive(), evo.NonInteractive("--yes")) {
    return evo.Refuse("declined")
}
```

| Symbol           | Kind | Meaning                                                             |
| ---------------- | ---- | ------------------------------------------------------------------- |
| `ConfirmOption`  | type | sealed interface                                                    |
| `Destructive`    | func | `Destructive() ConfirmOption`; default answer is no                 |
| `NonInteractive` | func | `NonInteractive(flag string) ConfirmOption`; how to pass unattended |

`Confirm` asks whenever stdin is a terminal, whatever the `Output`. When stdin
is not a terminal nobody can answer, and the row is Refused
"no terminal to answer" (never "blocked by policy", which is `Exclude`'s word).
A character device that is not a terminal, such as `/dev/null`, is not a
terminal. `NonInteractive` does not change the wording; it only names the
flag the Next step points at. A test that must not depend on the process's
stdin sets `Facades.Stdin` to a reader that is not a terminal.

`Confirm` may be asked from the run body or from inside a `Define` callback.
Either way it settles a row of its own named by the question: Succeeded on
yes, and Refused when declined or when nobody can answer, with the
non-interactive flag as the next step. Asking from a callback is not a
declaration, so it is never misuse. `Detail` is also a `ConfirmOption`.

## 6. Output

| Symbol   | Kind  | Meaning                                                                           |
| -------- | ----- | --------------------------------------------------------------------------------- |
| `Output` | type  | encoding selector                                                                 |
| `Human`  | const | `Human Output`; live TTY presentation                                             |
| `Plain`  | const | `Plain Output`; durable text, one line per event, no live region                  |
| `JSON`   | const | `JSON Output`; one `evo.run` document on stdout at Finish, presentation on stderr |
| `JSONL`  | const | `JSONL Output`; `evo.event` lines on stdout as they occur                         |

`Run.Summary` prints once, as the first line of Human and Plain, before every
row, wherever the body calls it. `Run.ResultWriter` is the only writer to stdout in Human and Plain: rows,
bands and `Run.Text` go to `Facades.Stderr`, so a pipe on stdout receives
exactly the payload. In JSON the payload is the string `result` of the `evo.run`
document, and in JSONL each write is one `result.written` event whose
`payload.text` is the bytes written. Bytes that are not valid UTF-8 travel as
`result_base64` and `payload.base64` instead of text, and Human and Plain pass
every byte through unchanged.

Environment, applied only where `Config` is zero:

| Variable    | Values                            | Effect                                                                                          |
| ----------- | --------------------------------- | ----------------------------------------------------------------------------------------------- |
| EVO_OUTPUT  | `human`, `plain`, `json`, `jsonl` | selects `Output`                                                                                |
| EVO_VERBOSE | `1`                               | same as `Config.Verbose`                                                                        |
| EVO_DEBUG   | `debug`, `info`, `warn`, `error`  | the lowest `slog.Level` the journal keeps when `Config.Debug` is nil; a named level beats it    |
| NO_COLOR    | any non-empty value               | no color; wins over `FORCE_COLOR`                                                               |
| FORCE_COLOR | any value except empty and `0`    | color in Human even when stdout is not a terminal                                               |
| LC_ALL      | a locale name                     | glyph selection on a live terminal; the first non-empty of `LC_ALL`, `LC_CTYPE`, `LANG` decides |
| LC_CTYPE    | a locale name                     | the same lookup, consulted when `LC_ALL` is empty                                               |
| LANG        | a locale name                     | the same lookup, consulted when `LC_ALL` and `LC_CTYPE` are empty                               |

These are the only variables Evo reads, and a value outside its list is
ignored. Machine output is never inferred from a piped stdout. A locale
containing `UTF-8` or `utf8` draws Unicode glyphs; any other locale draws ASCII
glyphs in the live status column. With none of the three set, Evo assumes UTF-8.

A JSONL run emits exactly one `task.finished` event for every Task it
declared, including a Task that never started because a predecessor failed
or the run ended first. The `seq` of a JSONL run's lines is 1, 2, 3 ... with
no gap, so a missing number means a lost line.

## 7. Errors

| Symbol                 | Meaning                                        |
| ---------------------- | ---------------------------------------------- |
| `ErrExecNonzeroExit`   | wrapped by Exec on a nonzero exit              |
| `ErrExecOutputMissing` | a declared Output is absent after exit 0       |
| `ErrStaleBasis`        | a Patch source changed between read and commit |
| `ErrPatchDoesNotApply` | the diff does not match the source             |
| `ErrComputedUnordered` | `Get` from work not ordered after the producer |
| `ErrInvalidConfig`     | a declaration or spec Evo refuses              |

Every declaration or spec Evo refuses is refused one way: the error wraps
`ErrInvalidConfig`, is joined into `Result.Err`, and `Conclusion.Explanation`
states what was refused. That covers a duplicate sibling name, a Task, Group or Sequence
declared from inside a `Define` callback, a nil `Define` callback, `Compute` on
a Task with a `Verify`, an `EffectSpec` that sets both `Path` and `Resource`,
an `EffectSpec` whose `Quantity` is not above 0, a `FileSpec` with no `Path`,
and an `ExecSpec` with no `Argv`. The refused call's callback never runs. Other sections say "refused" and mean this rule.

## 8. What Evo promises

- The live frame changes within 100ms of entering Running and at least every 100ms after, with a stable parent row and the current item beneath it.
- One runtime truth drives Human, Plain, `evo.run` and `evo.event`; no projection invents state another lacks.
- `evo.run` states only what happened: `definition_executed` is true for every Define callback that ran, including one that failed; `timing`, when present, is measured; `operations` and `basis` list what a Task's File and Exec calls did and read.
- Facts are hidden at normal verbosity and always in machine output; collapse folds routine detail away but never a container's own row: a Group or Sequence whose members all succeed keeps its row with the members indented beneath it, in Human and Plain as `evo.run` nests them; passing verification is silent, failing verification is shown.
- Per-item Excluded Tasks fold into one tally; per-item Effects with one Verb and Object fold into one counted row; JSON keeps every item.
- Cancellation preserves committed truth and never implies rollback.
- Misuse is recorded with a remedy, appears in `Result` as `Result.Err` and `Conclusion.Explanation`, and never crashes the program.
- Recorded status commits atomically; uncertainty re-executes and never reports Satisfied.
