Status: **canonical implementation contract**

Repository: `github.com/zachbornheimer/evident-output`

Consumer/canary: **zq**

This document is the source of truth for the 1.x implementation. The checked-in API, implementation, tests, docs, examples, MCP guidance, JSON/JSONL schemas, terminal renderer, manifest/provenance behavior, and zq usage must agree with this contract.

This document defines evo as if designed from scratch. The **Vocabulary** section is closed: a name that is not in it is not part of evo. Release history and call-site migration live in the repository (`docs/migration/`, `CHANGELOG.md`), never in this contract.

## 1. Product thesis

Evident Output lets application code describe work once and derives, from the same typed runtime truth:

- scheduling and dependency handling;
- lifecycle state and cancellation;
- declarative state establishment for common resources;
- automatic tracking and verification;
- provenance-aware freshness and safe skipping;
- Facts, Effects, warnings, progress, and partial-failure truth;
- TTY/plain output;
- JSON/JSONL/HTTP projection;
- conclusion and process exit.

The application describes **domain intent**. Evo owns **execution presentation and derived reporting**.

The safety rule for every optimization is:

> **False invalidation is acceptable. False “already satisfied” is not.**

## 2. Beginner vocabulary

```text
Task       one independently meaningful schedulable promise
Group      independent child work
Sequence   ordered child work
After      exceptional explicit dependency
Define     submit the work a Task performs
Wait       await settlement
File       authoritative desired state for one file
Patch      Basis-derived transformation into desired file states; mutates nothing
Effect     one opaque mutation Evo cannot model as file state
Basis      semantic inputs whose fingerprints determine freshness
Evidence   proof answering: is the requested state already satisfied?
Fact       structured information learned
Problem    one structured diagnostic attached to real work (error by default, or a warning)
Summary    one concise non-terminal result headline
Doing      current transient activity
Progress   numeric progress through work
```

`Verify` is the advanced read-only way to state Evidence for opaque work. Evo-native operations (File, Exec, Patch) derive Evidence themselves. The complete closed set follows.

## Vocabulary

This is the whole of evo, frozen by the owner on 2026-09-25 (<issue id="ee833204-2a6e-4ba3-a715-56c84d64e281" href="https://linear.app/zysys/issue/ZYS-1180/eo-v12-design-shrink-the-vocabulary-and-move-pit-of-success-from-lint">ZYS-1180</issue>). The rule: **one word per semantic concept.** Helpers and formatting variants never become additional vocabulary. An export that cannot be explained as one of these concepts, or as trivial sugar around one, must justify its existence before the 1.1.0 release or be removed.

### Canonical vocabulary

| Area         | Word            | Exact meaning                                                                 |
| ------------ | --------------- | ----------------------------------------------------------------------------- |
| Execution    | **Run**         | One root invocation of Evo.                                                   |
| Execution    | **Task**        | One independently meaningful schedulable promise. Never a container.          |
| Structure    | **Group**       | Independent child work; eligible siblings may run concurrently.               |
| Structure    | **Sequence**    | Ordered child work.                                                           |
| Structure    | **After**       | Exceptional explicit semantic dependency. Never resource locking.             |
| Execution    | **Define**      | Defines/submits a Task's execution. Evo owns lifecycle from here.             |
| Execution    | **Wait**        | Await Task/Group/Sequence settlement.                                         |
| Freshness    | **Basis**       | Semantic inputs whose identities determine whether prior state remains valid. |
| Freshness    | **Fingerprint** | Stable identity of one Basis input.                                           |
| Satisfaction | **Verify**      | Explicit satisfaction check when Evo cannot derive satisfaction itself.       |
| Satisfaction | **Evidence**    | The semantic proof/result answering "is requested state already satisfied?"   |
| File state   | **File**        | Authoritative desired state for one file.                                     |
| File state   | **Patch**       | Basis-derived transformation into desired file states; never mutates.         |
| File state   | **Files**       | Commits a Patch-derived FileSet through File semantics.                       |
| Processes    | **Exec**        | Evo-managed external command with capture, cancellation, provenance, outputs. |
| Coordination | **Resource**    | Identity used only to decide what may safely overlap.                         |
| Mutation     | **Effect**      | Planned/committed opaque mutation Evo cannot model as declarative state.      |
| Information  | **Summary**     | One concise non-terminal result headline, e.g. "459 checked".                 |
| Information  | **Fact**        | Structured information learned.                                               |
| Diagnostics  | **Problem**     | One structured diagnostic attached to real work.                              |
| Remediation  | **Action**      | Recommended next action/remedy.                                               |
| Live UX      | **Doing**       | Current transient activity.                                                   |
| Live UX      | **Progress**    | Numeric progress through work.                                                |

### Canonical outcomes and run Conclusion

Outcomes are runtime semantics, never caller-painted strings:

| Outcome              | Meaning                                                 |
| -------------------- | ------------------------------------------------------- |
| **Succeeded**        | Ran/evaluated and succeeded.                            |
| **AlreadySatisfied** | Requested state already true.                           |
| **Skipped**          | Genuinely not applicable or intentionally not executed. |
| **Blocked**          | A prerequisite/condition prevented it.                  |
| **Failed**           | Ran/evaluated and failed.                               |
| **Cancelled**        | Interrupted.                                            |

The run-level **Conclusion** is only: **OK** · **Blocked** · **Failed** · **Cancelled**.

Canonical outcome calls on `TaskHandle`:

```go
func (t *TaskHandle) Block(summary string, opts ...ProblemOption)
func (t *TaskHandle) Fail(summary string, opts ...ProblemOption)
func (t *TaskHandle) Skipped(reason TaxonomyReason)

type ProblemSeverity string

const (
    SeverityError   ProblemSeverity = "error"
    SeverityWarning ProblemSeverity = "warning"
)

func Severity(value ProblemSeverity) ProblemOption
func (t *TaskHandle) Problem(summary string, opts ...ProblemOption) *TaskHandle
func (t *TaskHandle) Progress(completed, total int) *TaskHandle
```

- Block and Fail are semantically different; both remain. Block: a policy or precondition refused the work (Blocked, exit 1). Fail: evaluation or required I/O failed (Failed, exit 2).
- A Problem's default severity is `SeverityError`: `task.Problem("invalid configuration")`. A warning is a severity, not a second call: `task.Problem("tool version differs from manifest", evo.Severity(evo.SeverityWarning))`. An error Problem fails the owning Define if the callback otherwise returns nil; a warning Problem does not.
- Progress carries the count; the current-item text is orthogonal: `task.Progress(i, total).Doing(path)`.
- Summary = what did we learn or accomplish. It resolves nothing. Skipped = why didn't this work run. It is an actual resolution. "kept 383 branches" is domain information: `task.Fact("kept", "383")` or part of the Summary. A per-candidate Task intentionally not executed because policy excludes it is Skipped.

### Effect verbs

Effect is the only opaque-mutation concept. The verbs are a closed set:

| Constant          | Verb        |
| ----------------- | ----------- |
| `EffectAdd`       | `add`       |
| `EffectCreate`    | `create`    |
| `EffectDelete`    | `delete`    |
| `EffectInstall`   | `install`   |
| `EffectPush`      | `push`      |
| `EffectRemove`    | `remove`    |
| `EffectUninstall` | `uninstall` |
| `EffectUpdate`    | `update`    |

There is deliberately no `EffectWrite`: files go through File/Patch. `PartialEffect` is an advanced helper for truthful partial commit, not a conceptual noun.

### API helpers, not vocabulary

Each helper serves exactly one concept and is never taught as a word of its own.

| Helper                          | Concept                         |
| ------------------------------- | ------------------------------- |
| `FSPath`, `Value`, `App`        | Fingerprint                     |
| `FSResource`, `LogicalResource` | Resource                        |
| `FileSet`                       | Patch result                    |
| `ExecResult`                    | Exec result                     |
| `PartialEffect`                 | Effect error/result helper      |
| `ProblemOption`                 | Problem configuration           |
| `Next`                          | attaches Action                 |
| `NextCommand`                   | constructs/attaches Action      |
| `Key`                           | stable identity                 |
| `Context`                       | execution access                |
| `Snapshot`                      | runtime inspection              |
| `Writer`                        | advanced stream adapter         |
| `Bytes`                         | Progress formatting convenience |

### Not part of evo

These names **must not exist** in the 1.1 exported API. There is no compatibility window: they are removed in 1.1, the 1.x release. Docs, examples, and the MCP corpus never use them, and review carries a migration rule that rewrites every occurrence to its canonical form.

| Not part of evo                                                     | Canonical form                                                                                                                   |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `Done`                                                              | Define (success resolves only through Define or an Evo-native operation)                                                         |
| `Blockf`                                                            | `Block(summary, opts...)`                                                                                                        |
| `Failf` (on `TaskHandle` and `Output`)                              | `Fail(summary, opts...)`                                                                                                         |
| `Warn` (on `TaskHandle`, `Output`, and the package)                 | `Problem(summary, Severity(SeverityWarning))`                                                                                    |
| `Step`                                                              | `Progress(completed, total).Doing(item)`                                                                                         |
| `Kept`                                                              | `Skipped(evo.Reason(...))` for a policy-excluded per-candidate Task; a Fact or part of the Summary for counts such as "kept 383" |
| `Record`, `RecordLabel`, `RecordName`                               | Fact / Summary                                                                                                                   |
| `TaskHandle.Add`/`Create`/`Delete`/`Push`/`Remove`/`Update`/`Write` | Effect (or File for files)                                                                                                       |
| `Lock`/`Unlock`/`ReadLock`/`WriteLock`/`IsLocked`                   | Resource (derived, §26)                                                                                                          |
| `File.Patch`, `ApplyPatch`                                          | Patch + Files                                                                                                                    |
| `Converge`                                                          | File / Effect                                                                                                                    |
| `Finding`                                                           | Problem (there is no second diagnostic model)                                                                                    |
| `Plan`/`Changes` as caller-authored mutation concepts               | Effect (planned/committed rows are derived)                                                                                      |
| capture-meaning `Evidence*` names                                   | Capture (below)                                                                                                                  |

`Bytes` stays as a helper (Progress formatting convenience) and is never taught.

### Collision decision: Evidence vs Capture

- **Evidence** = proof that requested state is satisfied. It means nothing else.
- **Capture** = retained stdout/stderr/process output.
- In 1.1 the API is `type Capture`, `type CaptureOption`, `type CaptureStream`. The capture-meaning names (`Evidence` as a capture type, `EvidenceOption`, `EvidenceStream`, `EvidenceStreamStdout`/`Stderr`/`Combined`) are renamed in 1.1 and do not remain. `KeepLastLines`, `MaxEvidenceBytes`, `MirrorToDebug`, and `MirrorToDiagnostics` become `CaptureOption`s only if they justify themselves (below); otherwise they are removed.

### One-screen mental model

```text
Run → Group/Sequence → Task → Define
  structure:      After, Wait
  state:          Basis, Fingerprint, Verify/Evidence, File, Patch/Files, Exec, Resource, Effect
  communication:  Summary, Fact, Problem, Action, Doing, Progress

Evo derives:
  Succeeded / AlreadySatisfied / Skipped / Blocked / Failed / Cancelled
    → OK / Blocked / Failed / Cancelled
    → TTY / plain / JSON / JSONL
```

### Reference: exported surface by concept

The rest of this section maps the exported API onto the vocabulary above. No name from "Not part of evo" appears here.

#### Run and Output

- `Output`: one run's runtime truth and every projection of it. The package-level default instance is the front door.
- `Init(Config) *Output`: the only constructor. It installs the default instance unless `Config.Isolated` is set.
- `Default()`, `SetDefault(out)`, `DefaultConfig()`: read or replace the default instance and its baseline Config.
- `Config`: ordinary wiring. `Title` names the run in the conclusion band. `Subject` is a durable header line (a repo path, a host). `Facts` are run-scoped Facts known at construction. `Stdout`, `Stderr`, `Result`, `Stdin` are the streams. `Verbosity`, `Color`, `Glyphs`, `Plain`, `Width` shape human output. `Format` routes streams and `Projection` selects encoding. `Debug` configures the debug journal. `DryRun` and `Preview` select planned tense. `Isolated` returns an independent Output. `MaxConcurrency` bounds executing callbacks (zero means GOMAXPROCS). `FailedExitCode` overrides exit 2 for Failed. `StateDir` and `AppID` place the manifest. Advanced: `Clock`, `Redactor`, `Terminal`, `Strict`, `VisibilityDelay` (built with `Delay(d)`), `MaxFrameRate`, `MaxEntities`, `MaxEvents`. `ProcessRunner` and `FileFS` are the injection points for process execution and the file system.
- `RunFunc`: `func(context.Context) error`, the application body.
- `Main(run) int`: runs `run` on the default instance with SIGINT/SIGTERM wired to cancellation, then Finish and Close, and returns the exit code. It never exits the process: `os.Exit(evo.Main(run))` does.
- `Run(ctx, run) Result` and `Output.Run(ctx, run) Result`: the non-exiting reconciler. `Result` carries the Conclusion and the error `run` returned; `Result.ExitCode()` is the derived code.
- `Output.Finish()`: validates, settles unresolved Tasks, computes the Conclusion, and writes the manifest. `Output.Close()`: idempotent cleanup that runs Finish when needed.
- `Output.Cancel(reason)`, `Output.Context()`, `Output.Err()`, `Output.Conclusion()`, `Output.Snapshot()`, `Output.Events()`: run-level cancellation, context, error, verdict, runtime snapshot, and event journal copy.
- Dry run (`Config.DryRun`): planned tense for every Effect, File, and Exec, announced by a `[dry-run] <Subject>` header. Mutation callbacks never run.
- Preview (`Config.Preview`): the same planned tense for a plan a confirm gate is about to act on, announced by the Subject header alone, with no `[dry-run]` tag.

#### Structure

- `Task(name) *TaskHandle`: one atomic, independently meaningful promise. A Task takes only a name and has no children.
- `Group(name) *GroupHandle`: independent children. The scheduler may overlap eligible siblings.
- `Sequence(name) *SequenceHandle`: ordered children with at most one Running child. A nested Group or Sequence is one step.
- `GroupHandle` and `SequenceHandle` declare children with `Task`, `Group`, and `Sequence`; set the container's own header text with `Summary`; and expose `Wait` and `Snapshot`. Only `Output`, `GroupHandle`, and `SequenceHandle` declare children.
- Own Task: a Group's child Task named for the Group itself owns the category's own work (classification, `Summary`, `Effect`). It is never folded as an item.
- `TaskHandle.After(preds...)`: an explicit dependency on Tasks, Groups, or Sequences.
- `TaskHandle.Key(key)`: stable identity, set before Define. It freezes once set; repeating the same key is idempotent.
- Sibling names are unique per parent across Task, Group, and Sequence (`ErrDuplicateSiblingName`, stable code `ProblemCodeDuplicateSiblingName`). Keys are unique per run (`ErrDuplicateKey`).

#### Define and Wait

- `TaskHandle.Define(fn) *TaskHandle`: submits the Task's work to the scheduler. It never runs `fn` inline. A nil return succeeds unless an error-severity Problem was recorded.
- `TaskHandle.Wait()`, `GroupHandle.Wait()`, `SequenceHandle.Wait()`: block until settled and return the aggregate error (§30).
- `TaskHandle.Context()`: the Task's scheduler-owned context. `TaskHandle.Cancel(reason)`: cancel this Task.
- Live activity, never terminal: `Doing(text string, args ...any)` names the current activity; `Progress(completed, total)` sets an absolute count, and the current item is `Progress(i, total).Doing(item)`. `Bytes(completed, total)` is Progress formatting sugar.
- `TaskHandle.Writer()`: an advanced stream adapter for a child process. Its last line becomes live activity, and a bounded, redacted tail is retained as Capture.

#### Outcomes

- `Block(summary, opts...)`: a policy or precondition refused the work. Stop before mutation. The Task concludes Blocked, exit 1.
- `Fail(summary, opts...)`: evaluation or required I/O failed. The Task concludes Failed, exit 2. Inside Define, return the error.
- `Problem(summary, opts...) *TaskHandle`: appends one diagnostic without resolving the Task. Severity defaults to `SeverityError`, which makes the Task settle Failed; `Severity(SeverityWarning)` sets `warned` and never fails the Task.
- `Skipped(reason)`: the work did not apply or was intentionally not executed. Renders `- skipped N (...)` and does not set `warned`. Skipped is never AlreadySatisfied.
- `Summary(text) *TaskHandle`: non-terminal result headline (§30).
- `Verify(fn) *TaskHandle`: a read-only Evidence check (§5).
- `Reason(name) TaxonomyReason`: a named, compile-time-stable reason for Skipped (`TaxonomyReason.Name()`). Each call records one `TaxonomyRecord`.
- `Failure`: the error a Block or Fail outcome carries; `Unwrap` exposes the cause. Actions attach to Block and Fail through one path only: `evo.Next(action)` or `evo.NextCommand(executable, args...)` passed as ProblemOptions to `Problem`, `Block`, or `Fail`. `TaskHandle.Next`, `TaskHandle.NextCommand`, `Output.Next`, `Output.NextCommand`, `Failure.Next`, and `Failure.NextCommand` do not exist.
- Run scope: `Output.Fail(summary, opts...)` states a run-level failure. A run-level warning is a Problem with `Severity(SeverityWarning)`. There is no `Output.Failf`, `Output.Warn`, or `evo.Warn`.
- `EntityState`: `Pending`, `Running`, `Done`, `Failed`, `Blocked`, `Cancelled`, `NotStarted`, `Skipped`, `Incomplete`, `Empty`. These are snapshot states that project the canonical outcomes.
- `Resolution` of a Done Task: `ResolutionExecuted` (Succeeded), `ResolutionAlreadySatisfied` (AlreadySatisfied), `ResolutionNoWork`.
- `Conclusion` and `ConclusionState`: `StateReady`, `StateChanged`, `StatePlanned`, `StateWarning`, `StateBlocked`, `StateFailed`, `StateCancelled`, plus the `warned`, `partial`, and `cancelled` modifiers. They project the run Conclusion (OK · Blocked · Failed · Cancelled).
- Exit codes: `ExitOK` 0, `ExitBlocked` 1, `ExitFailed` 2 (or `Config.FailedExitCode`), `ExitCancelled` 130.

#### Evidence and information

- Evidence: the Task-level answer to "is the requested state satisfied now?" (§5). `TaskEvidence` holds the before and after `EvidencePhase` observations.
- `Fact(name, value)` on `TaskHandle`, `Output`, and the package: structured information learned. Hidden in normal human output, always in machine output (§11). `FactRecord` is its recorded form.
- `Problem`: one structured diagnostic attached to real work. `ProblemOption`s: `Severity(value)`, `Detail(text)`, `Code(value)`, `On(subject)`, `Location(path, line, column)`, `Count(value, unit...)`, `Next(action)`, `NextCommand(executable, args...)`. `Attachment`, `Field`, and `SourceLocation` are its structured parts.
- `Action`, built by `Command(executable, args...)` or `Label(text)`: a recommended next step, attached only by the `Next` and `NextCommand` ProblemOptions. Actions on any Problem reach the run's next-steps output.
- Stable problem codes: `ProblemCodeVerificationUnsatisfied`, `ProblemCodeDuplicateSiblingName`.

#### Effect

- `Effect(ctx, EffectSpec, fn) error`: the one opaque mutation Evo cannot model as file state (a Git ref, a worktree, a remote push, an API change). Called inside Define.
- `EffectSpec`: `Verb`, `Object` (singular noun; Evo pluralizes it), `Quantity` (one aggregate count, > 0), `Resource` (optional, at most one).
- `EffectVerb`: the closed set above. There is no free-text verb. Planned rows use the imperative (`delete`, `install`); changed rows use the past tense (`deleted`, `installed`).
- `PartialEffect(committed, err) error`: returned from an Effect callback that committed part of its aggregate before failing (§30).
- `EffectRecord`: one recorded planned or changed Effect.
- Errors: `ErrEffectVerbInvalid`, `ErrEffectObjectMissing`, `ErrEffectQuantityNotPositive`, `ErrEffectCallbackMissing`, `ErrInvalidPartialEffect`.

#### File, Patch, Files

- `File(ctx, FileSpec) error`: establish one file's desired state (§4).
- `FileSpec`: `Path`, `Contents` (nil means unmanaged), `Mode` (0 means unmanaged), `Basis`.
- `FileFS`: the injectable filesystem facade File reconciles through.
- `Patch(ctx, diff) (FileSet, error)`: derive desired file states from a unified text diff; mutates nothing (§27).
- `FileSet`: the opaque result of Patch. It carries each desired state bound to its source Basis.
- `Files(ctx, FileSet) error`: commit each desired state through File, revalidating its source Basis first.
- Basis inputs (`Fingerprint`, `FingerprintValue`): `FSPath(path)` for file content, `Value(name, v)` for an explicit value, `App()` for application identity as a deliberate semantic input.
- Errors: `ErrFileSpecMissingPath`, `ErrFileUnmanagedContentsMissing`, `ErrFilePathIsSymlink`, `ErrFilePathTypeMismatch`, `ErrPatchMalformed`, `ErrPatchDoesNotApply`, `ErrPatchUnsupported`, `ErrPatchDeleteUnsupported`, `ErrPatchRenameUnsupported`, `ErrPatchBinaryUnsupported`, `ErrStaleBasis`.

#### Exec

- `Exec(ctx, ExecSpec) (ExecResult, error)`: run one child process with Evo-owned spawning, capture, live activity, cancellation, redaction, provenance, and output verification.
- `ExecSpec`: `Executable`, `Args` (literal, no shell), `Dir`, `Env` (explicit entries only enter the fingerprint), `Basis`, `Outputs` (declared outputs; none means always run).
- `ExecResult`: `Ran`, `ExitCode`, `Stdout`, `Stderr`, `Truncated`. The streams are the bounded, redacted Capture tail, not a data channel.
- `ProcessRunner`, `ProcessCommand`, and `ProcessOutcome`: the injectable process facade, set as `Config.ProcessRunner`.
- Errors: `ErrExecSpecMissingExecutable`, `ErrExecExecutableNotFound`, `ErrExecNonzeroExit`, `ErrExecOutputMissingAfterSuccess`.

#### Resource

- `Resource`: a sealed identity type callers cannot implement. It decides only what may safely overlap.
- `FSResource(path)`: a filesystem identity, hierarchical after canonicalization. `LogicalResource(name)`: a named identity for state with no truthful path.
- Errors: `ErrInvalidResource`, `ErrNestedResourceAcquisition` (§26).

#### Human output and interaction

- Messages: `Print`, `Printf`, `Println`, and `Verbose()` returning a `Printer`. `Output.Writer()` is the human stream; `Output.Suspend(fn)` pauses the live region around `fn`; `Output.ResultWriter()` is the domain-payload stream.
- `Confirm(question, opts...) bool` with `ConfirmOption`s `AssumeYes(v)`, `Destructive()`, `ConfirmDetail(lines...)`, `PolicyFlag(flag)`, `PolicyHint(command, args...)`. It owns the ask-decide-resolve gate. A decline concludes Blocked, exit 1.
- Presentation types: `ColorMode` (`ColorAuto`, `ColorAlways`, `ColorNever`), `GlyphProfile` (`GlyphsAuto`, `GlyphsUnicode`, `GlyphsASCII`), `Verbosity` (`VerbosityNormal`, `VerbosityVerbose`), `Visibility` (`VisibilityNormal`, `VisibilityVerbose`), `TerminalDriver`, `LiveSurface`, `TimeSource` (`SystemClock`, `FixedClock`), `Redactor` (`NoopRedactor`).
- Text helpers: `Pluralize(quantity, singular)`, `TruncateNames(names, visible)` with `DefaultVisibleNames`, `IsCharDevice(w)`.
- Debug journal: `DebugConfig`, `LogLevel` (`LevelUnset`, `LevelTrace`, `LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError`), `LogRecord`, `SlogHandler()`, `DebugPresentation` (`DebugPresentationHistory`, `DebugPresentationPane`), and `DebugPaneOption`s `NewestFirst()`, `OldestFirst()`, `PaneHeight(lines)`, `PreserveDebugTail()`.
- Glyphs, one meaning each: `✓` done, `✗` failed, `⊘` blocked, `!` warning, `■` cancelled, `-` not started or skipped detail, `○` pending, `◐` running (a spinner on a live TTY), `?` waiting for human input, `→` next action, `…` overflow.

#### Machine output

- `Format` routes streams: `FormatHuman`, `FormatData` (domain payload on stdout, presentation on stderr), `FormatExternal` (host renders from `Snapshot`), `FormatJSON`, `FormatJSONL`. `ParseFormat(s)` parses a host CLI flag value.
- `Projection` selects encoding: `ProjectionHuman`, `ProjectionPlain`, `ProjectionJSON`, `ProjectionJSONL`, `ProjectionStreamJSON`.
- Environment, applied only when the matching Config field is zero: `EVO_OUTPUT`, `EVO_COLOR`, `EVO_VERBOSE`, `EVO_DEBUG` (§16).
- `evo.run`: the final JSON document (`schema/run.v2.json`, `schema_version` `2.0`). `WriteJSON(w, result)` writes it without owning stdout.
- `evo.event`: the JSONL event stream (`schema_version` `1.0`), ending with `run.finished` on every terminal path.
- Snapshot model (runtime inspection): `Snapshot`, `TaskSnapshot`, `TasksSnapshot`, `MessageSnapshot`, `PlanSnapshot`, `ChangesSnapshot`, `Progress`, `ProgressKind` (`Determinate`, `Indeterminate`, `BytesKind`), `Event`, `RenderPlain(s, PlainOptions)`.
- `PublishedRelease`: the release this build reports.

#### Exports that must justify themselves before 1.1.0

Under the freeze rule, these exports are neither canonical words nor helpers of one. Each must justify its existence (a real consumer and one concept it serves) before the 1.1.0 release or be removed in 1.1. They are never taught.

- The capture-oriented `Evidence*` names are covered by the Capture decision above.

#### Misuse and lifecycle errors

`ErrAlreadyResolved`, `ErrTaskClosed`, `ErrClosed`, `ErrUnresolvedTask`, `ErrInvalidConfig`, `ErrKeyAfterDefine`, `ErrDryRunDeclaredLate`, `ErrNoTaskContext`, `ErrConcurrentRunning`, `ErrInvalidProgress`, `ErrProgressRegression`, `ErrLimitExceeded`, `ErrRenderer`, `ErrTerminalWithoutSink`, `ErrNotStarted`, `ErrWaitDeadlock`. Misuse is recorded as a run-level misuse line with a remedy hint; under `Config.Strict` it panics.

## 3. Scheduler semantics

### Task

A Task is one independently meaningful schedulable promise. It does not contain child Tasks.

A Task should normally name the action it performs, usually as a verb + concrete object such as `check file integrity`, `format Python`, `stabilize Go source`, or `lint Go`. This is a semantic rule, not a grammar linter: if the Task fails, its name should normally explain what failed without requiring the reader to inspect its children.

A category, heading, implementation phase, or collection of child work is not a Task merely because the renderer might show a row for it. Group and Sequence organize Tasks; the renderer decides whether their container labels deserve visible rows.

### Group

A Group contains independent children. Eligible siblings may run concurrently. Declaration order must not silently serialize them.

### Sequence

A Sequence contains ordered children. Prefer it to chains of manual `After` dependencies when the dependency is simply predecessor ordering.

### After

`After` is the advanced explicit dependency escape hatch.

### Scheduler ownership

Evo owns goroutines, bounded concurrency, cancellation propagation, dependency readiness, lifecycle transitions, and failure propagation. Caller-owned goroutines should not be needed just to make Evo work concurrent.

## 4. Define and declarative operations

`Define` is the execution/scheduling boundary.

Application code should not manually:

- print lifecycle rows;
- print “wrote …”;
- print “would add …”;
- print “nothing to write”;
- manually choose success/warning glyphs;
- manually compare a resource and narrate “already satisfied” when Evo can own that state.

The common file shape should be approximately:

```go
task.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{
        Path:     path,
        Contents: contents,
        Mode:     0o644,
    })
})
```

Declaring the file establishes the requested file state. The declaration is the whole call.

Unspecified file attributes are unmanaged. `Contents: nil` leaves contents alone and cannot create a missing file (`ErrFileUnmanagedContentsMissing`). `Mode: 0` keeps an existing file's permissions, and a new file gets 0666 less the umask. `Basis` adds semantic inputs to the file's freshness. File refuses a symlink at the path (`ErrFilePathIsSymlink`) and a path of the wrong type (`ErrFilePathTypeMismatch`).

`File`, `Files`, `Patch`, `Exec`, and `Effect` must be called with the context a Define callback received (`ErrNoTaskContext` otherwise).

`evo.File` should automatically:

- inspect relevant current state;
- avoid unnecessary mutation;
- mutate only managed attributes;
- verify the resulting state;
- record tracked state/fingerprints;
- record useful Facts;
- emit planned/changed Effects;
- preserve partial truth on failure.

## 5. Evidence

Evidence is the Task-level boolean conclusion:

> **Is the requested state currently satisfied?**

Evidence is not inherently a list of public named attributes such as “contents” or “permissions.” Those may exist internally as verification dimensions for reporting.

For common Evo-native operations, Evidence should be derived automatically.

Ordinary success should remain compact:

```text
✓ write plist
```

not:

```text
✓ write plist
  ✓ contents
  ✓ permissions
```

Verification details surface when they explain partial truth, skipped work, or failure:

```text
✗ write launch agent  failed: permissions
  - contents  already satisfied
  ✗ permissions
    error  operation not permitted
    path   ~/Library/LaunchAgents/com.acme.prod.agent.plist
    mode   0644
```

Already satisfied is a successful resolution, e.g.:

```text
Outcome: Done
Resolution: AlreadySatisfied
```

### Verify

`TaskHandle.Verify(fn)` states Evidence for opaque work. It is read-only and must observe state; a constant answer is misuse (review API-063). Checks registered before Define are ANDed.

- Before the callback: all checks true resolves the Task Done with `ResolutionAlreadySatisfied`, and the callback never runs.
- After a successful callback: any false check fails the Task with `ProblemCodeVerificationUnsatisfied` (`postcondition not satisfied`, exit 2).
- Dry run and Preview skip the after-check only for a Task that recorded a planned row, because that mutation never ran. A Task whose callback planned nothing is checked exactly as a real run checks it.
- A callback that resolves its own Task (`Skipped`, `Block`) chose not to converge, so the after-check is skipped. If the callback committed an Effect before resolving itself, state changed and the after-check runs.
- A callback that calls `Skipped` and then returns an error fails with that error. The self-resolution was only a proposal, so no misuse line prints.

## 6. Operation freshness vs whole-Task skipping

These are different.

An Evo-native operation encountered inside `Define` may no-op based on its **current invocation**.

But a resource discovered only after entering `Define` cannot retroactively justify skipping arbitrary work that already started.

Example:

```go
task.Define(func(ctx context.Context) error {
    data := expensiveGeneration()
    return evo.File(ctx, ...)
})
```

By the time `evo.File` is reached, `expensiveGeneration` already ran.

Therefore:

- **operation freshness** may make `evo.File` itself a no-op;
- **whole-Task Evidence** requires sufficient proof before `Define` starts.

Never whole-Task skip opaque code based only on yesterday’s dynamically discovered manifest.

## 7. Provenance and Basis

Tracked results may depend on a `Basis`.

Basis means:

> These semantic fingerprints participate in whether this result remains valid.

It does not imply only “source files” or “generated from.”

A Basis may mean:

- generated from;
- compiled against;
- configured by;
- validated against;
- tool/script version;
- schema version;
- command arguments;
- input path;
- another tracked artifact;
- explicitly relevant environment/config value.

**Track semantic inputs, not ambient state.**

Do not fingerprint the entire machine or environment by default.

## 8. Provenance precision hierarchy

Prefer the most specific proof available:

```text
precise semantic provenance
    >
task/definition provenance
    >
application fingerprint fallback
    >
no proof
```

Known external work should support precise provenance. Example: a Go binary invokes `python generate.py input.xlsx` and creates `schema.bin`. If the script, input, relevant arguments, and explicitly relevant environment are unchanged, unrelated Go application edits should not necessarily force regeneration.

For opaque Go callbacks where Evo cannot know semantic dependencies, the application/build fingerprint is the conservative fallback.

An application fingerprint means “previously discovered semantics may need revalidation,” not “every downstream result definitely changed.”

## 9. Freshness graph vs scheduler graph

These are separate graphs.

Scheduler graph:

```text
Group / Sequence / After
→ when work is eligible
```

Freshness graph:

```text
tracked resources / Basis / fingerprints / definition identity
→ whether previous results remain valid
```

Do not conflate them.

Freshness propagation follows actual changed resources, not merely the fact that an upstream Task executed.

Example:

```text
source.xlsx
   ↓
normalize
   ↓
normalized.json
   ↓
compile
   ↓
schema.bin
   ↓
package
```

If `normalize` must be revalidated but produces the same `normalized.json` fingerprint, downstream work should remain satisfied where safe.

Principle:

> **Revalidation follows uncertainty. Invalidation follows actual dependency changes.**

## 10. Manifest requirements

Persist enough information to validate tracked outputs and their Basis on later runs.

Use stable Task/resource identities, not human display labels alone.

Define safe behavior for:

- missing manifest;
- corrupt manifest;
- concurrent writers;
- atomic updates;
- interrupted writes;
- schema upgrades;
- application/definition changes;
- missing resources;
- fingerprint algorithm changes.

Uncertainty or corruption causes safe revalidation/re-execution, never false satisfaction.

Successful tracked-state updates commit atomically.

## 11. Facts

Facts are structured observations, not print statements.

Ordinary Facts (`TaskHandle.Fact`, `Output.Fact`, `evo.Fact`) are hidden in normal human output on every Task outcome: success, warned, and failed alike. Verbose output shows them. JSON and JSONL always carry them (`facts` on each `evo.run` task, `fact.recorded` events), whatever the human verbosity. `Config.Facts` are run-scoped Facts known at construction; they render with the header.

Shown Facts render with muted labels and normal-intensity values:

```text
path  ~/Library/LaunchAgents/com.acme.prod.agent.plist
```

A failure is explained by its Problems and by the failing operation's verification detail, which carries its own facts and renders with it. A Task that fails does not promote its unrelated ordinary Facts.

The application does not choose glyphs, colors, indentation, or terminal verbosity.

## 12. Effects and dry-run

Effects are separate from Tasks and Facts.

Planned:

```text
[planned] branches         delete 40 local tips
[planned] .prettierrc.json create
```

Committed:

```text
[changed] remote-tracking  deleted 4 stale origin/*
```

A content-free Effect such as:

```text
[changed] zq
```

is invalid. `Effect` refuses an empty `Object`, a non-positive `Quantity`, an unknown `Verb`, or a nil callback, and records nothing.

Dry-run must be structural. The same workflow model should produce:

```text
dry-run:
    [planned] .prettierrc.json  create

apply:
    [changed] .prettierrc.json  created

unchanged:
    ✓ .prettierrc.json  already satisfied
```

Applications do not maintain separate reporting paths for these modes.

Ledger rules:

- An aggregate Effect row reads `<subject>  <verb> <quantity> <pluralized object>`. Its subject is the owning Task's name.
- A ledger section belongs to its owning Task, not to its name. Two same-named Tasks in different containers get separate rows, qualified by container path (`alpha › prune`) when the bare name is ambiguous.
- Rows print in Task declaration order, whatever order the Tasks finished in.
- Aggregate Effect rows tally and render at Finish. Named rows (File's `write <path>`, Exec's `run <executable>`) stream when their owning Task resolves. Both share one viewport cap with a `… +N more (not shown)` overflow line.
- Subject columns align to the widest subject in the block, followed by exactly two spaces (§30).

## 13. Automatic verbosity and terminal projection

Rows are scarce. Do not render data merely because Evo knows it.

Default projection hierarchy:

```text
Task                    full-text landmark
current activity        one indented row
determinate progress    bar + exact count
indeterminate progress  spinner + description
timer                   muted
routine Fact            hidden
useful Fact label       muted
useful Fact value       full text
passing Evidence        hidden
failing verification    shown
warning                 strong warning glyph + message
failure                 failure glyph + message
Effect                  separate ledger
not-started             muted
```

Implementation phases such as `classify` should collapse automatically unless independently meaningful.

Row rules the renderer owns:

- A Task's settled row shows one headline: its Summary (with the count it reached if it failed mid-loop), else its first Problem.
- A Group without a Summary has no header row; its children render as siblings. A live header stays while work runs, because its `N/M complete` count is the Group's own progress. A Sequence keeps its header.
- A header-less child whose name another visible row also shows is named by its container path (`g › build`).
- A finished no-op child (Done, no work or already satisfied, carrying nothing) is hidden when other content is visible and the run did not fail, block, or cancel. A root Task that simply finishes stays a landmark.
- A warning renders its `On` subject on every row (`✓ check jobs  ! job  x`).
- Problems nest under their own row. Human output shows at most 5 inline, then `and N more failures`; machine output keeps every one.
- JSON and JSONL keep every Group, Task, Fact, and Problem regardless of these rules.

### Skipped fold

Skipped is the per-item disposition. The item is the Task: declare one child Task per candidate under the category's Group. A candidate that policy excludes is intentionally not executed, so it calls `Skipped(evo.Reason(...))` at most once. Counts such as "kept 383" are domain information: a Fact or part of the Summary, never a separate outcome.

- Per-item Skipped child Tasks aggregate into one tally under the Group's row, with per-reason counts: `- skipped N (283 checked out, 135 unpushed, 1 protected)`. This happens when the Group's own row names their subject (its own Task or its own Summary) or when no sibling finished work of its own. Beside a sibling that did work, a Skipped child is a peer category and keeps its named row.
- A lone skipped item folds into its Group's tally like any other count (`✓ branches  2 checked` then `  - skipped 1 (protected)`); it never renders as its own success row.
- Verbose lists the items under each reason, bounded (`a, b, c … +N more`); items carrying Facts get their own row, at most three.
- Skipped uses the `-` glyph and never sets `warned`. A fold of policy exclusions produces no warned band.
- JSON/JSONL keep every child, with `dispositions` (`{disposition, reason, name, causes}`) on each `evo.run` task and `disposition.recorded` events.

## 14. Live activity invariant

While work is Running on a live TTY, the visible frame must change within 100ms of entering Running and at least every 100ms thereafter.

Evo owns a shared animation heartbeat independent of application event frequency.

Intentional waiting for human input is the exception.

Stable parent row:

```text
⠋ install dependencies  [████        ]  14/40  — 7s
  ⠋ urllib3
```

The parent owns progress and timer. The changing concrete item appears beneath it.

Prefer concrete items such as package/file/host/ref names over abstract status such as “converging.”

## 15. Cancellation

Cancellation preserves truth.

Never imply rollback unless rollback actually occurred.

Example:

```text
✓ remote-tracking  4/4
■ worktrees        interrupted
- branches         not started

[changed] remote-tracking  deleted 4 stale origin/*

[cancelled] prune  by user
  ! partial changes were applied before cancellation
```

Distinguish not-started, interrupted, completed, and committed effects.

Do not duplicate the effects ledger in a second giant cancellation sentence.

A cancelled run ends `[cancelled] <subject>  by user`, the cause also recorded as `Conclusion.Explanation`. `! partial changes were applied before cancellation` appears only when an Effect committed. Work Defined after the interrupt settles NotStarted.

Conclusion maps cancellation to exit 130.

## 16. Machine output

One typed runtime truth drives:

- TTY;
- plain text;
- final JSON;
- JSONL;
- HTTP/wire serialization.

No projection invents state unavailable to the others.

Applications do not hand-build separate JSON reporting models.

Preserve wire compatibility deliberately. Every wire document carries `schema_version`: `evo.run` is `2.0` (`schema/run.v2.json`) and `evo.event` is `1.0`. Additive optional fields are compatible. An incompatible change requires a new major schema.

Selection:

- `Config.Format: FormatJSON` writes exactly one `evo.run` document to stdout at Finish. `FormatJSONL` streams `evo.event` lines to stdout as they occur (`seq` strictly monotonic and the only ordering authority), ending with `run.finished` on every terminal path, including failure and cancellation. Human presentation goes to stderr in both.
- With no `Format` chosen, `EVO_OUTPUT=json` selects `FormatJSON` and `EVO_OUTPUT=jsonl` selects `FormatJSONL`. `EVO_OUTPUT=human|plain|stream-json`, `EVO_COLOR=auto|always|never`, `EVO_VERBOSE=1`, and `EVO_DEBUG=info|debug|trace` select the rest. Explicit Config wins over the environment; the environment wins over TTY inference.
- Evo never infers machine output because stdout is a pipe. The caller states it.

The `evo.run` task entry carries `id`, `key`, `parent_id`, `name`, `state`, `resolution`, `definition_executed`, `evidence`, `progress`, `activity`, `timing`, `verification`, `tracked_resources`, `basis`, `facts`, `dispositions`, `problems`, `warnings`, `operations`, and `summary`. Collections carry `kind`, `state`, `summary`, `progress`, and `children`. Top-level `data` carries `tasks`, `collections`, `effects` (each tagged `planned` or `changed`), `facts`, `problems`, and `actions`.

Machine output should preserve, where applicable:

- run identity;
- timestamps/duration;
- mode;
- conclusion/outcome;
- Task hierarchy and stable identity;
- resolution (`Executed`, `AlreadySatisfied`, `NoWork`);
- action/mutation executed;
- Facts;
- warnings/problems with stable codes;
- Effects;
- tracked resources/provenance;
- cancellation;
- durations.

Machine consumers use stable structured fields/codes, not human terminal prose.

## 17. Conclusion and exit

One Conclusion drives UI + exit:

```text
OK         0
Blocked    1
Failed     2
Cancelled  130
```

`evo.Run` returns a Result and never exits.

`evo.Main` returns the exit code and never exits. The program applies it: `os.Exit(evo.Main(run))`. A bare `evo.Main(run)` statement exits 0 after a failed run (review EVO-EXIT-002).

Mapping rules:

- A Task Blocked anywhere, including inside a Group or Sequence, concludes `[blocked]`, exit 1.
- A Confirm decline concludes Blocked, exit 1.
- Failed exits 2 unless `Config.FailedExitCode` overrides it.
- `warned` (any warning-severity Problem) and `partial` (work that never started) are modifiers on the headline (`[planned · warned]`, `[ready · warned]`); they never change the exit code. Skipped does not warn.
- A non-nil error from `run` is recorded as a failure only when nothing already failed.

## 18. zq prune acceptance fixture

Independent prune categories belong in a Group:

```text
zq prune  ~/repo

⠋ branches         [████        ]  120/459  — 2s
  ⠋ feat/style-contract
⠋ worktrees        [███         ]   70/294  — 2s
  ⠋ eapp-system-style-contract-heading
⠋ remote-tracking  [███         ]    1/4    — 2s
  ⠋ origin/old-style
```

`branches`, `worktrees`, and `remote-tracking` become eligible together.

Internal `classify` rows should not clutter ordinary output.

Dry-run should separate discovery from planned Effects:

```text
[dry-run] zq prune  ~/repo

✓ branches         459 checked
  - skipped 419 (283 checked out, 135 unpushed, 1 protected)
✓ worktrees        294 checked
  - skipped 292 (163 dirty, 89 unpushed, 40 ignored files)
✓ remote-tracking  4 stale refs

[planned] branches         delete 40 local tips
[planned] worktrees        remove 1 worktree
[planned] remote-tracking  delete 4 stale origin/*
```

The band drops the title because the `[dry-run]` Subject header already named the run. A pure `[planned]` verdict under that header prints no band at all. Policy-excluded items are Skipped and never warn, so this run concludes `[planned]` and prints no band. Ordinary Facts such as `on disk  23.1 MB` appear only under verbose, where each skip reason also lists its items.

Each category is a Group whose own Task (`branches`) owns the classification, the Summary, and the Effect; each policy-excluded candidate is a per-item Task that calls `Skipped(evo.Reason(...))` (`evo.Reason("checked out")`, `evo.Reason("unpushed")`, `evo.Reason("protected")`, `evo.Reason("dirty")`, `evo.Reason("ignored files")`, `evo.Reason("in use")`, ...), and those Tasks fold into the category's `- skipped N (...)` tally (§13).

No-op output should remain concise and suppress zero-information `classify` rows.

## 19. zq adopt acceptance fixture

zq adopt describes the repository state it wants. Evo derives the rest:

- a mapping zq discovers is a Fact or domain data;
- an unsupported mapping is a Problem (error or warning severity) with a stable Code;
- each house-style file is one `evo.File` declaration;
- dry run produces planned Effects; apply produces changed Effects;
- an unchanged file resolves AlreadySatisfied and stays quiet;
- a file failure shows its verification detail;
- zq writes no per-mode narration and no compare/write/report code for files.

Acceptance: the dry-run, apply, and unchanged runs of zq adopt come from one declaration per file, with no application-authored mutation or already-satisfied prose.

## 20. zq as canonical consumer

Every zq output call site is exactly one of:

- actual Task;
- Fact;
- Problem (error or warning severity);
- Skipped (a policy-excluded candidate, with an `evo.Reason`);
- planned Effect;
- changed Effect;
- automatic resource operation;
- automatic Evidence/already-satisfied;
- summary/conclusion.

If the same awkward boilerplate appears repeatedly, determine whether the missing abstraction belongs in Evident.

Do not add zq-specific sugar when a general Evo primitive would make all consumers safer/simpler.

Shared-state coordination for parallel work is scheduler-owned and declarative (§26).

## 21. MCP

MCP guidance must match the exact installed/target Evo version.

Teaching order:

1. Task + Define
2. Group / Sequence
3. declarative resource sugar
4. automatic Evidence/provenance
5. After for exceptional edges
6. Facts
7. Effects / dry-run
8. advanced verification/capture/prompts

MCP should detect and explain at least:

- success or result claimed without work (Summary carrying mutation, dry-run, or already-satisfied narration; a Verify that returns a constant; a Task never Defined or resolved);
- duplicate sibling Task labels;
- information represented as fake Task success;
- planned mutation narrated through Done;
- any name listed under Vocabulary "Not part of evo", each with a migration rule to its canonical form (Blockf → Block, Failf → Fail, Warn → Problem + Severity, Step → Progress + Doing, Kept → Skipped with an `evo.Reason` or a Fact/Summary count, capture-meaning Evidence\* → Capture);
- manual file compare/write/report code that should use `evo.File`;
- caller goroutines used only for Evo concurrency;
- After chains better represented by Sequence;
- routine Facts/passing Evidence manually printed;
- caller-chosen glyph/color/status;
- direct terminal output competing with the live renderer;
- breaking wire-schema edits;
- `os.Exit` bypassing Evo conclusion handling.

Findings should include severity, file/line, violated rule, why it matters, smallest migration, corrected syntax, and required target version.

The review loop is mandatory: review, apply every suggestion, review the same source again, and stop only when findings are empty and `recheck_required=false`. Tools: `evident_output_list_sections`, `evident_output_get_documentation`, `evident_output_adopt_plan`, `evident_output_review`, `evident_output_explain` (argument `rule_id`), `evident_output_preview`, `evident_output_update`.

MCP also owns **pit-of-success escalation**. A normal misuse with an existing canonical rewrite is a review finding. If the target Evo version cannot express the consumer's semantic requirement cleanly without caller-owned scheduling/state/presentation/typed plumbing, or the same awkward workaround recurs and indicates a missing general primitive, MCP emits a structured product-gap event, keeps `recheck_required=true`, and must not invent public API. The event carries a stable dedupe signature, consumer/version/source evidence, violated invariant, smallest reproducer, why existing primitives are insufficient, and the desired semantic capability. When a Linear sink is available, the harness opens or updates one deduplicated Evident Output ticket and returns its identifier; without a sink it returns `ticket_required=true` plus a paste-ready issue payload. An unresolved product-gap event means the review is not clean. Implementation: <issue id="980cc99a-1eb1-443f-9af9-eb8c6415f2da" href="https://linear.app/zysys/issue/ZYS-1369/eo-mcp-pit-of-success-gap-event-deduped-linear-ticket">ZYS-1369</issue>.

When extracted Tasks are referenced later, MCP should recommend typed variables and `var (...)` when it reduces typo/duplicate-label risk:

```go
var (
    branches  = prune.Task("branches")
    worktrees = prune.Task("worktrees")
    remote    = prune.Task("remote-tracking")
)
```

Do not mechanically require this for trivial one-off Tasks.

## 22. Tests

Behavioral/golden/integration tests must prove:

- Group sibling concurrency;
- Sequence ordering;
- After semantics;
- bounded scheduling;
- cancellation and partial Effects;
- determinate/indeterminate progress;
- <=100ms visible Running heartbeat;
- stable timer/current-item layout;
- automatic Fact verbosity;
- warnings/failures/partial truth;
- planned vs changed Effects;
- already satisfied;
- declarative file create/update/no-op/mode-only/partial failure;
- provenance invalidation;
- precise script/input provenance;
- application fingerprint fallback;
- unchanged upstream output stopping downstream invalidation;
- dry-run never mutating;
- manifest missing/corrupt/concurrent/atomic behavior;
- TTY/plain/JSON/JSONL semantic consistency;
- stable problem codes;
- schema compatibility;
- exit codes.

Public examples and MCP-recommended snippets should compile against the actual target version so documentation cannot drift.

## 23. Junior-developer pit of success

A junior developer should not need comments explaining how to produce acceptable output.

Correct API use should automatically produce:

- sensible concurrency;
- correct ordering;
- useful live activity;
- automatic verbosity;
- automatic Facts;
- automatic plans/changes;
- automatic JSON/JSONL;
- automatic Evidence;
- provenance-aware safe skipping/recompilation;
- correct cancellation;
- useful failure detail;
- centralized exit behavior.

Prefer compiler/type-system constraints over documentation saying “do not do this.”

## 24. Linear execution rule

This document contains the durable product contract.

Split implementation work into issues only when a unit has its own independently testable invariant and review boundary. Do not scatter product semantics across many tickets such that an implementer must reconstruct the design from issue archaeology.

A good issue contains:

- the invariant it proves;
- relevant contract section;
- concrete before/after fixture;
- acceptance tests;
- dependencies/blockers.

Milestones represent coherent proof points, not directory/component buckets.

## 25. Final acceptance question

The system is ready when a reviewer can answer **yes** to:

> Does zq describe what it wants to accomplish, while Evident automatically handles how that work is scheduled, verified, skipped, tracked, planned, mutated, reported, serialized, cancelled, and rendered?

If the answer still requires application-specific terminal prose, duplicated state logic, manual already-satisfied checks, manual JSON construction, caller-managed concurrency, or comments explaining correct usage, 1.x is not complete.

## 26. Resource access is derived, not manually locked

Evo must make file/resource coordination difficult to misuse.

The application does **not** manage mutexes, lock files, lock ordering, or unlock lifecycle. Resource access has no caller-visible lock lifecycle, and no mutation path bypasses Evo's state model.

The runtime owns resource access.

For file-backed state:

```text
Basis(FSPath(path))
    → safely observe/fingerprint path
    → read-side coordination while observing

File(Path: path, ...)
    → establish desired file state
    → write-side coordination while committing
```

The existence of a lock is an implementation detail. Uncontended access should be invisible. If contention matters to the user, the renderer may show that work is waiting; the application does not poll or ask whether a resource is locked.

### One-resource rule

Generic resource access may hold **at most one resource at a time**.

While one resource is held, nested acquisition of another resource is invalid. This rule must apply even through helper calls.

The goal is to make deadlock construction impossible through the public common path rather than teaching lock ordering.

A Resource may be a file or, for an opaque operation that genuinely has coarser semantics, a directory/module/workspace/logical shared resource. Prefer the narrowest truthful resource because narrower resources preserve concurrency.

Operations that inherently require multi-resource atomicity are implemented as Evo-native operations rather than exposing arbitrary multi-lock acquisition to callers.

### Read/Write are semantics, not necessarily beginner nouns

The public vocabulary has no mutex terms.

`Basis` is the normal read-side semantic declaration because it says both that the resource is observed and that its fingerprint participates in freshness.

`File` is the normal write-side semantic declaration because it says what state should be established, enabling dry-run, Evidence, Effects, verification, and stale-write protection.

Resource exclusion alone cannot express desired state, dry run, Evidence, or Effects, so no API grants exclusion without one of those meanings. Opaque mutation claims its one Resource through `EffectSpec.Resource`.

## 27. Patch is a Basis-derived transformation, not a mutation

`Patch` is pure with respect to the real workspace.

A patch operation:

1. identifies the source file states it refers to;
2. establishes their Basis/fingerprints;
3. derives the desired resulting file states;
4. retains the Basis relationship with those desired states;
5. mutates nothing.

Conceptually:

```text
Basis
   ↓
Patch
   ↓
desired file states
   ↓
File
```

A multi-file patch is sugar over multiple desired file states. It does not introduce another mutation engine.

The returned value should preserve provenance strongly enough that callers cannot accidentally discard the source fingerprints and then commit stale derived contents as though they were fresh. `FileSet` is opaque for exactly this reason.

Before committing a Patch-derived file state, Evo revalidates the Basis used to derive it. If the source state changed, Evo must not overwrite the newer state. That file fails with `ErrStaleBasis`.

Principle:

> **Patches are derived via Basis. File owns mutation.**

### File remains authoritative

`File` is the one-file authoritative state primitive.

For existing-file rewrites, Patch-derived desired contents ultimately reduce to ordinary File establishment. Dry-run, already-satisfied behavior, verification, planned/changed Effects, and manifest updates come from File rather than from a second patch-specific mutation path.

`Patch → Files → File` expresses every rewrite. File is the only file-mutation engine.

### Multi-file patch

A unified/multi-file patch may derive several desired file states. Evo may provide sugar to establish the returned set, but the semantics remain per-file File establishment.

Do not expose caller-managed multi-file locks. If several files need mutation, Evo commits them through its file-state machinery while preserving the patch Basis and stale-write checks.

Patch supports modification and file creation. It rejects deletion, rename, and binary forms, because File cannot represent desired absence or rename, and approximating them would be false truth (§30).

## 28. In-place external fixers should produce state, not own mutation

For commands such as formatters/fixers that rewrite existing files, prefer:

```text
native diff/check mode
    → Patch
    → Basis-derived desired file states
    → File

otherwise:
isolated workspace
    → run opaque mutating command there
    → derive filesystem diff
    → Patch/file states
    → File in the real workspace
```

If a tool provides a trustworthy diff mode, prefer it to copying the workspace.

For example, a `go fix -diff <target>` style flow may derive a patch without allowing the tool to mutate the real workspace. If the fixer requires multiple passes until stable, that convergence remains one user-meaningful Task such as `stabilize Go source`; individual passes are implementation activity, not Tasks.

The isolated-workspace fallback is an implementation strategy, not a beginner API. The application should not manually copy files to temporary directories merely to obtain dry-run safety.

## 29. Three separate relationships

Keep these concerns distinct:

```text
Scheduler graph
    Group / Sequence / After
    → semantic work eligibility

Resource access
    Basis/read-side observation + File/write-side establishment
    → what may safely overlap

Freshness graph
    Basis / tracked resources / fingerprints / definition identity
    → whether prior results remain valid
```

A resource conflict is not automatically a semantic dependency edge.

A Basis relationship is not merely a lock.

A File write is not merely exclusive access.

The common path declares semantic truth once and lets Evo derive the scheduling, safety, freshness, dry-run, Evidence, Effects, and rendering behavior from it.

## 30. Settled semantics (1.1)

This section states, as normative behavior, the 1.1 API-freeze decisions (2026-09-23), the zq canary decisions (2026-09-23b), and the release-gate rulings.

### Task lifecycle

- Success resolves only through Define (the callback returns nil) or an Evo-native operation.
- Terminal verbs: `Block`, `Fail`, `Skipped`, `Cancel`. `Doing`, `Progress`, `Writer`, `Fact`, `Problem`, `Summary`, `Next`, and `NextCommand` annotate and never resolve. `Bytes` is Progress sugar and annotates.
- A Task resolves exactly once. A Task holding an error-severity Problem never settles Done or Skipped: a nil return, a `Skipped` call, or Finish settling it unresolved all settle it Failed.
- A Task nobody Defines or resolves is misuse (hint: call Define, Fail, Block, or Skipped on this task). A Wait on it settles it NotStarted.
- An After cycle settles each member Blocked with one `dependency cycle: a → b → a` line.
- `Skipped` means the work never applied. Current desired state is `ResolutionAlreadySatisfied`, never Skipped.
- `TaskHandle` has no child constructors. Whether a name denotes independently meaningful work is taught by docs and review (API-045, API-050), not rejected by grammar.
- Every `TaskHandle` method is a safe no-op on a nil handle.

### Task Summary

`Summary(text string) *TaskHandle` is one sanitized single-line result field. The last call wins; empty clears. It never resolves lifecycle and is not live activity. It renders after the Task name on the settled row (`✓ branches  459 checked`). `GroupHandle.Summary` and `SequenceHandle.Summary` share its sanitization and projection. `TaskSnapshot`, `evo.run`, and `evo.event` expose it as `summary`. Summary never carries mutation, dry-run, or already-satisfied narration; those belong to Effect, File, Evidence, and Facts (review API-060).

### Scheduling and MaxConcurrency

- Group siblings become eligible together and may overlap. A Sequence runs one step at a time in declaration order; a nested Group or Sequence is one step, and a failed step leaves every later step NotStarted.
- `After(g)` on a collection that is empty when named stays open until `g.Wait()`, a Wait on the dependent, or the end of the run closes it, and waits for every child declared before then; closed while empty, it counts as done. A collection already populated when named is taken as declared: a child declared later never gates that edge.
- A Task declared into a Sequence step the Sequence has already passed runs after the Sequence's latest step.
- `Config.MaxConcurrency` bounds every executing callback, including work a waiting goroutine runs itself. A waiting callback lends its own slot to the work it awaits, so nested Define+Wait completes at MaxConcurrency 1. A goroutine outside any callback runs work only in a free slot.
- Callers never start goroutines to make Evo work concurrent (EVO-DAG-001, API-041). A goroutine a callback started that Waits while that callback blocks on it with every slot held gets `ErrWaitDeadlock` naming API-041.

### Wait

- `TaskHandle.Wait()` returns nil on success; the callback's error; `ErrNotStarted` when the work never ran (a failed predecessor, a run that drained first, a Task never Defined, or a refused declaration, wrapping the refusal); the cancellation, `errors.Is`-compatible; `ErrWaitDeadlock` when nothing can ever reach it; `ErrNestedResourceAcquisition` when called while holding a resource claim.
- `GroupHandle.Wait()` and `SequenceHandle.Wait()` wait for every descendant without serializing eligible siblings. They return nil only when every descendant ran and succeeded. Meaningful child errors join in declaration order (`errors.Join`). An `ErrNotStarted` whose predecessor cause is already in the joined error is omitted; when the cause is outside the container, the container returns `ErrNotStarted`. There is no container result type: per-child detail is `Snapshot()`.
- A collection with a Blocked child snapshots Blocked.
- A Wait's answer never depends on which Waits ran before it.

### Problems and warnings

`Problem(summary, opts...) *TaskHandle` appends one Problem without resolving; many may accumulate in one Define. Severity defaults to `SeverityError`: an error Problem fails the owning Define if the callback otherwise returns nil. `Severity(SeverityWarning)` makes it a warning, which does not fail Define and sets `warned`. There is no Warn call. A warning's `On(subject)` reaches every human row and machine output (`warnings` on the `evo.run` task). Human projection bounds inline detail; Snapshot, JSON, and JSONL retain every Problem, and the count stays authoritative. There is no second finding type.

### Effect

```go
type EffectVerb string

const (
    EffectAdd       EffectVerb = "add"
    EffectCreate    EffectVerb = "create"
    EffectDelete    EffectVerb = "delete"
    EffectInstall   EffectVerb = "install"
    EffectPush      EffectVerb = "push"
    EffectRemove    EffectVerb = "remove"
    EffectUninstall EffectVerb = "uninstall"
    EffectUpdate    EffectVerb = "update"
)

type EffectSpec struct {
    Verb     EffectVerb
    Object   string
    Quantity int
    Resource Resource
}

func Effect(ctx context.Context, spec EffectSpec, fn func(context.Context) error) error
```

- The verb set is closed. File state is never an Effect: it is File or Patch.
- `Quantity` is one aggregate count and must be positive; with nothing to mutate, do not call Effect. `Object` is singular.
- Dry run and Preview record the planned Effect and never invoke `fn`. Apply invokes `fn` with the scheduler-owned context and records the changed Effect only after `fn` succeeds.
- A callback that resolves its own Task Skipped or Failed records no ledger row.
- At most one `Resource` is claimed, for writing, while `fn` runs.

### PartialEffect

```go
func PartialEffect(committed int, err error) error
var ErrInvalidPartialEffect error
```

Returned from an Effect callback, PartialEffect keeps committed mutation truth while the Task still fails.

- `err` must be non-nil and `0 <= committed <= EffectSpec.Quantity`; otherwise Effect returns `ErrInvalidPartialEffect` and records nothing.
- `committed > 0` records one changed Effect with the original Verb and Object and `Quantity: committed`. `committed == 0` records no changed row.
- The returned error keeps `err` reachable through `errors.Is`/`errors.As`.
- It binds to the innermost Effect whose callback returned it. An outer Effect that passes the error up records nothing for it, and an outer callback's own PartialEffect joined beside an inner one still counts.
- Dry run never invokes the callback, so it plans the full Quantity.
- PartialEffect implies no rollback and no retry.

### Plain Effect alignment

The §18 plain fixture is normative. Subject columns pad to the widest display-cell width in the Effect block, then exactly two spaces precede the Effect phrase. Planned and changed bands use the same rule.

### Exec result

```go
type ExecResult struct {
    Ran       bool
    ExitCode  int
    Stdout    string
    Stderr    string
    Truncated bool
}

func Exec(ctx context.Context, spec ExecSpec) (ExecResult, error)
```

- `Ran` is false when Exec did not spawn (a current manifest hit, a dry-run plan) and alongside a spawn, cancellation, or configuration error.
- `Stdout`/`Stderr` are the sanitized, redacted Capture tail (at most 200 completed lines, about 256 KiB total); `Truncated` reports loss. They are for line-oriented diagnostics, not a data channel: a tool's complete machine output is read from a file it writes.
- A nonzero exit returns the result and an error wrapping `ErrExecNonzeroExit`. A declared Output missing after exit 0 wraps `ErrExecOutputMissingAfterSuccess`.
- Parsing is domain code over ExecResult. Cancellation is context-based, never string matching.

### Patch and Files

```go
func Patch(ctx context.Context, diff []byte) (FileSet, error)
func Files(ctx context.Context, files FileSet) error
```

- Patch accepts unified text diff bytes, observes each source as Basis under the one-resource rule, derives desired contents, and mutates nothing.
- Patch applies the diff forward first, as `patch` and `git apply` do. Applying the same diff again is already satisfied. When a source matches both sides of the diff, only this Task's own recorded result from its last Run counts as already applied.
- Patch never creates directories: a file in a missing directory fails `ErrPatchDoesNotApply` at Patch time, before anything commits. A path beyond a symlinked directory fails `ErrPatchUnsupported`. Deletion, rename, and binary forms fail `ErrPatchDeleteUnsupported`, `ErrPatchRenameUnsupported`, and `ErrPatchBinaryUnsupported`.
- Files commits each desired state through File, revalidating that state's source Basis immediately before its commit (`ErrStaleBasis`). Files is not a transaction: already-committed Effects stay truthful when a later file fails.
- Callers that already know desired bytes call File directly.

### Application fingerprint fallback

Opaque Define work automatically records application identity as its conservative definition provenance, with zero caller code, and that fallback never enters the visible Basis list. It never whole-Task skips an opaque callback: opaque work runs every Run. `evo.App()` is for callers who make application identity a deliberate semantic Basis of a precise File or Exec. Precise File/Exec/Patch Basis is not invalidated by unrelated application changes.

### Resources

- No API accepts more than one resource claim. Acquiring a second while one is held, directly or through a helper, fails `ErrNestedResourceAcquisition`.
- File, file-backed Basis, and `FSResource` share one canonical filesystem namespace. Two path claims overlap when the canonical paths are equal or one is an ancestor directory of the other. `LogicalResource` claims overlap only by equal normalized name.
- Read/read overlaps. Any overlapping pair with a write waits. A waiting Task shows `waiting for <resource>`; an uncontended claim renders nothing.
- A claim coordinates overlap only. It never creates an After edge and never enters freshness.
- Scope: claims coordinate every Output in one process. Across processes, the first File or Exec in a Run takes the manifest's exclusive lock and holds it until Close, so two processes in the same manifest namespace (same `StateDir`, or same `AppID` and workspace) never interleave tracked File/Exec state. Different namespaces do not coordinate. Effect claims are process-local.

### Manifest persistence

Finish writes the manifest; one background writer folds pending commits into one atomic write. A failed save renders a `manifest not saved: <reason>` warning and `Close` returns the error. A write that failed is retried on the next flush.

## 31. Composition: capabilities, topology, values (v1.2 direction)

Principle: **callers declare work, topology builders declare structure, Evo owns execution.** Typed dataflow, scheduler topology, and executable work are separate concerns.

### Task results: compiler-checked `Compute[T]`

A Task that produces a value for later Evo work uses the package-level generic:

```go
func Compute[T any](
    task *TaskHandle,
    fn func(context.Context) (T, error),
) *Computed[T]

func (c *Computed[T]) Get() T
```

`Compute[T]` is the typed-value form of defining exactly one Task. It internally submits one Task `Define` callback.

Rules:

- no runtime type injection into callbacks;
- no `.Into(&x)` result plumbing;
- no outer mutable result variable as the recommended handoff;
- `Get()` is valid only after the producing Task reached Succeeded or AlreadySatisfied; early access is deterministic misuse;
- `After(computed)` means After the producing Task, so ordering and data dependency are not declared twice;
- Sequence ordering is already sufficient when the consumer is a later Sequence step;
- an upstream Failed/Blocked/Cancelled producer prevents dependent work from starting.

Example:

```go
inventory := evo.Compute(
    packages.Task("discover installed packages"),
    func(ctx context.Context) (Inventory, error) {
        return discoverPackages(ctx)
    },
)
```

### Containers: structure, never hidden work

`TaskHandle` is never a container and never declares children.

The shared topology-builder helper is:

```go
type Container interface {
    Task(name string) *TaskHandle
    Group(name string) *GroupHandle
    Sequence(name string) *SequenceHandle
}
```

Only `*Output`, `*GroupHandle`, and `*SequenceHandle` satisfy `Container`. A Task does not.

A topology builder that already knows its shape accepts an `evo.Container` and mounts Tasks/Groups/Sequences onto it. It never starts a second scheduler or waits.

### Dynamic topology: the container owns the fan-out

When the shape of later work depends on a computed value, predeclare the Group or Sequence in the correct place and let **that container** define its children after its predecessor is ready.

Target shape:

```go
inventory := evo.Compute(
    packages.Task("discover installed packages"),
    discoverPackages,
)

centralize := packages.Group("centralize packages").After(inventory)

centralize.Define(func(group *evo.GroupHandle) {
    for _, pkg := range inventory.Get().Packages {
        pkg := pkg

        group.
            Task("centralize " + pkg.Name).
            Define(func(ctx context.Context) error {
                return pkg.Centralize(ctx)
            })
    }
})
```

For ordered composition, a later Sequence child already has the required predecessor relationship:

```go
packages := run.Sequence("consolidate packages")

inventory := evo.Compute(
    packages.Task("discover installed packages"),
    discoverPackages,
)

centralize := packages.Group("centralize packages")
centralize.Define(func(group *evo.GroupHandle) {
    for _, pkg := range inventory.Get().Packages {
        pkg := pkg
        group.Task("centralize " + pkg.Name).Define(func(ctx context.Context) error {
            return pkg.Centralize(ctx)
        })
    }
})
```

`GroupHandle.Define` and `SequenceHandle.Define` are **topology-only builders**, not work callbacks:

- the callback receives only its container handle;
- it has no `context.Context`;
- it returns no error;
- it may only declare Tasks, Groups, and Sequences beneath that container;
- it may read `Computed[T]` values whose producers are proven settled by Sequence ordering or `After(computed)`;
- it must not perform I/O, mutation, `Exec`, `File`, `Effect`, `Wait`, sleep/polling, or start goroutines.

The restrictive callback shape is intentional: executable work must live in independently meaningful Tasks, normally named as verb + concrete object. A container builder describes topology only.

A dynamic builder runs exactly once when the container becomes eligible, then the container closes its declaration phase and owns the lifecycle of every child it declared. If a predecessor Failed/Blocked/Cancelled, the dependent container does not build children and remains NotStarted under ordinary dependency semantics.

Calling package-level constructors or declaring child topology from inside a Task's `Define` is misuse. A Task never owns a subtree.

### Composition pit of success

The intended zq shape is therefore:

```text
prune
├─ repository                         Group
│  ├─ prune landed branches           Task -> BranchPruneResult
│  ├─ prune unused worktrees          Task -> WorktreePruneResult
│  ├─ prune stale remote-tracking refs Task
│  └─ prune deleted remote branches   Task <- branches
└─ consolidate packages               Sequence
   ├─ detect package managers         Task -> []Manager
   ├─ discover installed packages     Task -> Inventory
   └─ centralize packages             Group, dynamically defined
      ├─ npm                           Group
      │  ├─ centralize debug@4.3.4    Task
      │  └─ centralize lodash@4.17.21 Task
      └─ composer                      Group
         └─ centralize psr/log@3.0.0  Task
```

Noun/category names belong naturally on Group/Sequence. Tasks are independently meaningful promises; the strong authoring heuristic remains verb + concrete object.

MCP enforces this composition model (<issue id="2948e4e5-3e19-4810-9034-3145bd0ce472" href="https://linear.app/zysys/issue/ZYS-1368/eo-mcp-v12-enforce-typed-dataflow-container-owned-dynamic-topology">ZYS-1368</issue>). If a real consumer cannot express the clean shape in its target Evo version, MCP emits the pit-of-success product-gap event (<issue id="980cc99a-1eb1-443f-9af9-eb8c6415f2da" href="https://linear.app/zysys/issue/ZYS-1369/eo-mcp-pit-of-success-gap-event-deduped-linear-ticket">ZYS-1369</issue>), does not invent local Evo syntax, and keeps `recheck_required=true`.

### Not yet in Evo

| Need                                                                                  | Ticket                                                                                                                                                                                  |
| ------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Compiler-checked Task values via `Compute[T]`                                         | <issue id="1f0ad2a6-dd1b-4a9b-a4ab-c530a318b4d4" href="https://linear.app/zysys/issue/ZYS-1198/eo-v12-typed-values-between-tasks-via-computet">ZYS-1198</issue>                         |
| Container-owned dynamic topology after typed predecessors                             | <issue id="7d9233f2-6fd4-4bdc-bf93-847421762efa" href="https://linear.app/zysys/issue/ZYS-1199/eo-v12-container-owned-dynamic-topology-after-typed-predecessors">ZYS-1199</issue>       |
| Exported `evo.Container` for Output/Group/Sequence topology builders                  | <issue id="7b720d61-3fb2-4b54-bc2c-c0873a635bdb" href="https://linear.app/zysys/issue/ZYS-1203/eo-v12-export-evocontainer-for-rungroupsequence-topology-builders">ZYS-1203</issue>      |
| MCP enforcement of typed dataflow + container-owned topology                          | <issue id="2948e4e5-3e19-4810-9034-3145bd0ce472" href="https://linear.app/zysys/issue/ZYS-1368/eo-mcp-v12-enforce-typed-dataflow-container-owned-dynamic-topology">ZYS-1368</issue>     |
| MCP pit-of-success gap event -> deduped Linear ticket                                 | <issue id="980cc99a-1eb1-443f-9af9-eb8c6415f2da" href="https://linear.app/zysys/issue/ZYS-1369/eo-mcp-pit-of-success-gap-event-deduped-linear-ticket">ZYS-1369</issue>                  |
| Attempts: retry a unit until Verify passes; a failed attempt does not fail the parent | <issue id="74c2c01f-a71c-48be-a86c-f71fb8bc91ac" href="https://linear.app/zysys/issue/ZYS-1204/eo-v12-attempts-a-container-that-retries-a-unit-until-it-converges">ZYS-1204</issue>     |
| yaegi symbols package shipped by Evo                                                  | <issue id="5ca4a33f-251e-4389-8079-1329dff27556" href="https://linear.app/zysys/issue/ZYS-1202/eo-v12-ship-a-yaegi-symbols-package-so-interpreted-code-can-import-evo">ZYS-1202</issue> |

### Rendering note

In plain output, a named Group under a Sequence renders no header line; its children appear directly under the Sequence. This follows §3 ("the renderer decides whether container labels deserve visible rows"). It hides a meaningful label such as `validate plan`, so it is open for review in v1.2.

## 32. One elapsed format

Elapsed and quiet durations render in one compact form everywhere: live, plain, and durable output. The form is `2s` under a minute, `4m12s` under an hour, `3h04m` under a day, and `2d3h` beyond. Minutes after an hour are zero-padded. A duration never renders without a unit, and never in the Go `Duration.String` form (`1h2m3s`, `4h12m0s`).

## 33. Long-running Tasks (render-only lifecycle)

Evo renders caller-supplied lifecycle state and never supervises processes.

A `Writer()`-backed Running Task shows a bounded live tail of 6 lines under its row, with the footer `… N lines in evidence`. After 60 seconds without output the row carries a `· quiet <elapsed>` suffix in the one elapsed format of section 32. Memory stays bounded for any uptime. The live frame changes at least every 100ms.

Daemon extensions (1.2): `task.Ready(detail)` marks readiness. `task.Restarted(reason, attempt, nextBackoff)` renders a restart. `task.StoppedBy(signals...)` reclassifies the listed signals as a clean stop. SIGTERM and SIGINT are clean stops by default for a Task that opts in. A clean stop concludes with exit code 0 and the `[stopped]` band. Plain (non-TTY) output streams start, ready, log lines, and stop one line per event, without glyphs. JSONL carries every log line as an event. Log lines classified INFO and above appear in the tail; DEBUG lines appear only under `EVO_VERBOSE=1`.

## 34. Ledger fold

Sibling per-item mutating Tasks under one Group whose Effects share a verb and object fold into one aggregated human row, counted per item (for example `deleted 3 branches`). The fold keys on owner Task identity, never on display name. `[planned]` and `[changed]` Effects both fold. A failed item stays visible under the aggregated row. JSON and JSONL keep every per-item Effect. The fold needs no API.

## 35. Pit-of-success eval

A deterministic replay, with no model and no network, proves that every reference answer for each non-blocked eval task compiles, reviews clean with `recheck_required=false`, and runs to its expected topology. Every trap answer is rejected by a detector or recorded as expected-pending.

The driver that calls a model needs an explicit key, a positive `--max-usd`, and `--confirm-spend`. It aborts when accumulated cost reaches the cap. It exposes only the MCP tools plus `go_build` and `submit`, with at most 40 tool turns per sample. It places the prompt-cache breakpoint on the last system block and the last tool. It reports held-out tasks as aggregates only.

A hillclimb step is rejected if it touches a frozen path, edits Go beyond string literals and comments, or contains a distinctive task noun. It is accepted only at +1 or more training samples with no task down 2 or more.
