# Evident Output

## Architecture, Product, Verification, Agent-Assistance, and Open-Source Specification

**Project name:** Evident Output
**Repository and Go module:** `github.com/<org>/evident-output`
**Go package identifier:** `evo`
**Companion MCP executable:** `evident-output-mcp`
**Document status:** Design candidate, revision 0.3
**Intended license:** Apache License 2.0, subject to maintainer approval
**Primary implementation language:** Go
**Audience:** maintainers, contributors, CLI authors, systems engineers, security reviewers, developer-experience teams, and AI-tool integrators

---

## 0. Document conventions

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** are normative requirements in this specification.

The product is named **Evident Output**. The repository and module use `evident-output`. The root Go package is named `evo` so call sites remain short and immediately recognizable:

```go
import "github.com/<org>/evident-output"

out := evo.For("bpp-csharp")
```

The package name is intentionally not `EvidentOutput`: Go package names are lowercase, concise, and are read together with exported identifiers. `evo.Output`, `evo.Item`, `evo.Task`, and `evo.Problem` are the canonical names used in documentation.

Evident Output defines **how a command-line application communicates**. It standardizes presentation semantics for current activity, reported conditions, progress, evidence, changes, plans, messages, actions, and conclusions. It does not define command parsing, business workflows, dependency injection, concurrency, cancellation, retries, or process lifetime.

Compatibility is a primary requirement. The library MUST embed cleanly into ordinary Go programs and coexist with Cobra, `urfave/cli`, Kong, the standard `flag` package, `slog`, arbitrary `io.Writer` consumers, Bubble Tea and other TUIs, CI systems, shell pipelines, JSON consumers, and AI tools.

The architecture is intentionally layered:

```text
application domain and control flow
             │
             │ facts and presentation updates
             ▼
        Evident Output
             │
             │ human, plain, JSON, JSONL, snapshot, MCP
             ▼
 terminal / pipe / file / host UI / agent
```

The application owns execution. Evo owns presentation.

### Document map

- **Parts I–II:** product definition, compatibility boundary, public vocabulary, ownership, inference, and state machines.
- **Parts III–IV:** Go API and internal synchronization/rendering architecture.
- **Parts V–VII:** output contracts, security, and Svelte-inspired MCP/agent assistance.
- **Part VIII:** verification strategy, executable conformance, and edge-case traceability.
- **Parts IX–X:** red-green delivery, SOLID/DRY guardrails, licensing, compatibility, and governance.
- **Part XI:** adversarial review with integrated resolutions.
- **Part XII:** milestones, release gates, and success metrics.
- **Appendices:** canonical examples, event taxonomy, declarative schemas, decisions, and red tests.

### Decisions intentionally left for pre-1.0 evidence

- final organization and domain ownership;
- final legal approval of Apache-2.0 versus MIT;
- whether renderer interfaces have sufficient external demand to be exported;
- exact minimum supported Go version;
- whether a hosted MCP endpoint is justified after the stdio implementation proves useful;
- whether `evo` is retained as the root package name after package and trademark clearance.

# Part I — Product definition

## 1. Executive summary

Evident Output is a Go library for making CLI output easier to understand than ad hoc `fmt.Printf` calls while remaining almost as easy to use for ordinary cases. It is a presentation library, not a command framework, scheduler, workflow engine, logger replacement, or full-screen TUI toolkit.

A CLI author should provide domain facts:

```go
out := evo.For("bpp-csharp")
defer out.Close()

workingTree := out.Item("working tree")
branches := out.Item("branches")
remotes := out.Item("remotes")

workingTree.OK()
branches.BlockedBy(problems...)
remotes.Warn(
    "origin was not reachable",
    evo.Detail("remote state is unverified"),
    evo.Cause(err),
)

return out.Finish()
```

Evo decides:

- transient versus final rendering;
- row and column alignment;
- color, Unicode, and terminal capability policy;
- progress-frame coalescing;
- stable declaration order;
- debug insertion above a live region;
- narrow-terminal degradation;
- aggregation of problems and actions;
- human, plain, JSON, JSONL, snapshot, and MCP projections;
- the final command conclusion when it can be inferred safely.

The library's primary vocabulary is deliberately small:

- `Output`: everything presented by one command invocation;
- `Item`: one named fact, condition, or result that remains meaningful in final output;
- `Task`: one operation that may have phases or progress;
- `Tasks`: a named collection of independently progressing tasks;
- `Problem`: structured evidence explaining an undesirable item or task;
- `Changes`: durable effects that already occurred;
- `Plan`: effects that would occur but have not occurred;
- `Action`: something the user can do next;
- `Conclusion`: the inferred meaning of the whole command.

The distinction between `Task` and `Tasks` is architectural, not cosmetic:

```text
Task   = one operation with one lifecycle
Tasks  = a collection whose state is derived from child tasks
```

A task is never both a leaf and a container. This prevents ambiguous parent progress, conflicting terminal states, and hidden composite behavior.

Progress updates favor absolute values:

```go
file.Progress(completed, total)
download.Bytes(written, totalBytes)
```

Incremental progress is explicit:

```go
file.Advance(1)
```

The application owns concurrency:

```go
var group errgroup.Group

group.Go(func() error {
    // application work
    task.Bytes(written, total)
    return nil
})
```

Evo is safe under concurrent updates but never decides when work runs, how many goroutines exist, whether failures cancel siblings, or whether an error should be returned.

The defining interaction is a live presentation that becomes durable evidence:

```text
⠋  working tree
⠋  branches
⠋  remotes
```

then:

```text
✓  working tree
✗  branches
   ├─ feat/sdk-full-consolidation  local-only (1)
   └─ fix/login-flow               ahead of origin (2)
!  remotes
   └─ origin was not reachable     remote state is unverified

[blocked]  bpp-csharp
  Resolve the branch problems before retiring this repository.

  repo-retire salvage --dry-run bpp-csharp
```

For many concurrent operations:

```text
⠋  dependencies  2/4 complete
   ✓  react      8.1 MB
   ⠋  esbuild   12.4/18.0 MB
   ⠋  sharp      verifying
   ○  zod
```

A successful final projection may collapse child detail:

```text
✓  dependencies  installed 4 packages

[changed]  dependencies
  added  4 packages
  wrote    app.lock
```

Debug records remain durable and cannot corrupt the live region:

```text
12:04:18.219 [DEBUG] package index loaded  packages=18

⠋  dependencies  2/4 complete
   ⠋  esbuild   12.4/18.0 MB
```

The MCP companion follows the strongest aspect of Svelte's agent tooling: authoritative task-oriented guidance, focused retrieval, deterministic review, repair-and-recheck loops, and inspectable previews. It does not merely expose documentation.

## 2. Product goals

### 2.1 Primary goals

The library SHALL:

1. make CLI state, progress, evidence, effects, and next actions understandable within one short screen for ordinary cases;
2. remain a presentation library compatible with existing command frameworks and application architectures;
3. make simple output nearly as easy as `fmt` and structured logging;
4. make multi-row progress, item evidence, debug interleaving, resizing, and final reports substantially easier than hand-written terminal code;
5. require only domain facts that cannot be inferred safely;
6. keep application execution, goroutines, cancellation, retries, transactions, and business errors outside the library;
7. preserve final static output independent of animation history;
8. make failures and blockers richer than successes;
9. provide deterministic human, plain, JSON, JSONL, snapshot, and event projections;
10. maintain strict stdout/stderr behavior;
11. remain useful without ANSI, Unicode, color, or cursor movement;
12. support many independently progressing tasks with bounded redraw cost and screen-budget policies;
13. expose a small, ordinary-language Go API;
14. prevent invalid state transitions without requiring an error branch after every presentation update;
15. make all inference deterministic and observable in snapshots;
16. provide executable conformance tests independent of a particular renderer or language implementation;
17. provide MCP tools that help agents discover guidance, review code/output, repair findings, and preview terminal profiles;
18. preserve source compatibility within a major version.

### 2.2 Secondary goals

The library SHOULD:

- require no configuration for common interactive use;
- avoid global mutable state;
- minimize dependencies;
- support Linux, macOS, and Windows terminals;
- permit projection-only and host-TUI integration;
- provide virtual terminals, deterministic clocks, fixtures, and assertions;
- make accessibility and plain-output behavior first-class;
- provide compile-tested examples and agent skills generated from canonical sources;
- keep root-package dependencies independent of MCP and command frameworks.

### 2.3 Explicit non-goals

Version 1 SHALL NOT be:

- a command parser or CLI application framework;
- a goroutine scheduler or workflow engine;
- a retry, timeout, or transaction library;
- a shell-command execution engine;
- a full-screen TUI framework;
- a general logging backend;
- an arbitrary widget tree or CSS-like layout engine;
- an interception layer for uncontrolled writes to process streams;
- a distributed progress protocol;
- a hosted AI automation service.

The library cannot make unrelated direct writes safe during live rendering. Applications MUST route visible output through Evo, suspend the live renderer, embed snapshots in another UI, or accept static degradation.

## 3. Product principles

### 3.1 Presentation-only ownership

Evo defines how a command communicates. It never decides how the command executes.

### 3.2 Domain facts before formatting

The caller supplies names, states, counts, evidence, paths, actions, and effects. The renderer owns spaces, colors, glyphs, wrapping, columns, and terminal control.

### 3.3 Ordinary-language vocabulary

Public names should be understandable without knowledge of terminal rendering internals. Terms such as frame, region, sink, scheduler, event bus, and widget remain internal.

### 3.4 Infer what is safe; require what is meaningful

Safe inference includes:

- a task starts on its first phase or progress update;
- an item can resolve directly without a visible running frame;
- an instant operation does not flash a spinner;
- a `Tasks` collection derives state from children;
- blocked items imply a blocked conclusion unless a stronger failure exists;
- terminal capabilities determine presentation defaults.

Unsafe inference is prohibited:

- inventing domain-specific remediation language;
- deciding whether an application error should cancel work;
- deciding concurrency or retry policy;
- turning unresolved work into success or skip;
- assuming a raw error is safe user-facing detail.

### 3.5 One owner for mutable presentation truth

`Output` is the aggregate root. Handles contain identity and delegate mutations to the owner. They do not maintain authoritative independent copies.

### 3.6 Stable result, disposable animation

The final report must be correct without the live frames that preceded it.

### 3.7 Success is quiet; failure is explanatory

Successful children may collapse under screen pressure. Failures, warnings, and unresolved states remain visible with evidence.

### 3.8 Color is redundant

Color reinforces text and glyphs but never carries the only meaning.

### 3.9 Machine output is a separate contract

Scripts and agents consume structured projections, not aligned human text.

### 3.10 Compatibility before control

Evo integrates with host systems. It does not require ownership of the application's architecture or process.

### 3.11 Conservative public API

New sugar is accepted only when it removes ceremony without hiding application decisions or introducing ambiguous state.

### 3.12 Convergent agent assistance

Agent tooling must help an implementation reach a demonstrably conforming result: discover, retrieve, implement, review, repair, recheck, preview.

## 4. Representative use cases

### 4.1 Repository inspection with items

```go
out := evo.For(repo)
defer out.Close()

workingTree := out.Item("working tree")
branches := out.Item("branches")
remotes := out.Item("remotes")

var group errgroup.Group

group.Go(func() error {
    if err := inspectWorkingTree(); err != nil {
        workingTree.Block(
            "unstashed changes",
            evo.Detail("commit or stash the changes before continuing"),
            evo.Cause(err),
        )
        return nil
    }

    workingTree.OK()
    return nil
})

group.Go(func() error {
    problems := compareBranches()
    if len(problems) > 0 {
        branches.BlockedBy(problems...).
            Because("Resolve the branch problems before retiring this repository.").
            NextCommand(
                "repo-retire",
                "salvage",
                "--dry-run",
                repo,
            )
        return nil
    }

    branches.OK()
    return nil
})

group.Go(func() error {
    if err := probeRemotes(); err != nil {
        remotes.Warn(
            "origin was not reachable",
            evo.Detail("remote state is unverified"),
            evo.Cause(err),
        )
        return nil
    }

    remotes.OK()
    return nil
})

if err := group.Wait(); err != nil {
    return err
}

return out.Finish()
```

Expected negative results update presentation and may still return `nil`. An ordinary Go error means the application could not complete its operation as designed. Evo never forces one control-flow policy.

### 4.2 One phased task

```go
dependencies := out.Task("dependencies")

dependencies.Phase("reading lockfile")
dependencies.Phase("resolving packages")
dependencies.Progress(downloaded, total)
dependencies.Donef("installed %d packages", installed)
```

`Phase` and `Progress` start a pending task implicitly. `Done` may resolve a task before any live frame appears.

### 4.3 Multiple progress rows

```go
dependencies := out.Tasks("dependencies")

var group errgroup.Group
var installed atomic.Int64

for _, pkg := range packages {
    pkg := pkg
    install := dependencies.Task(pkg.Name)

    group.Go(func() error {
        install.Phase("downloading")

        err := download(pkg, func(written int64) {
            install.Bytes(written, pkg.Size)
        })
        if err != nil {
            install.Fail("download failed", evo.Cause(err))
            return nil
        }

        install.Phase("verifying")
        if err := verify(pkg); err != nil {
            install.Fail("verification failed", evo.Cause(err))
            return nil
        }

        installed.Add(1)
        install.Done()
        return nil
    })
}

if err := group.Wait(); err != nil {
    return err
}

dependencies.Summaryf("installed %d packages", installed.Load())
```

`Tasks` owns no independent progress or terminal state. It derives aggregate state from children. Its summary is shown only when compatible with that derived state.

### 4.4 Structured changes and automatic alignment

```go
out.Changes("dependencies").
    Added(14, "packages").
    Updated(4, "packages").
    Reused(63, "cached packages").
    Wrote("app.lock")
```

The caller supplies no spaces. Evo may render:

```text
[changed]  dependencies
  added    14 packages
  updated   4 packages
  reused   63 cached packages
  wrote       app.lock
```

On narrow terminals it may render:

```text
[changed]  dependencies
  added 14 packages
  updated 4 packages
  reused 63 cached packages
  wrote app.lock
```

### 4.5 Plan output

```go
out.Plan("delete account acme").
    Delete(14, "projects").
    Revoke(7, "API keys").
    Remove(23, "users").
    Retain("audit records for 90 days")
```

A plan never implies that changes occurred.

### 4.6 Managed debug output

```go
out.Debug(
    "branch comparison completed",
    evo.Int("blockers", blockers),
    evo.Duration("duration", elapsed),
)
```

The debug line is durable. The terminal driver clears and redraws the transient region around it.

### 4.7 Logger integration

```go
logger := slog.New(out.SlogHandler(slog.LevelDebug))
logger.Debug("package index loaded", "packages", 18)
```

### 4.8 Host TUI integration

```go
out, err := evo.NewWithConfig(evo.Config{
    Projection: evo.ExternalProjection(),
})
if err != nil {
    return err
}

for snapshot := range out.Snapshots(ctx) {
    host.Update(snapshot)
}
```

In projection-only mode, Evo provides semantics and snapshots while the host owns rendering.

# Part II — Domain model and ownership

## 5. Bounded contexts and interoperability boundary

The design uses domain-driven thinking without pretending terminal presentation is the application's business domain. It defines a narrow **presentation domain** and keeps all application concerns outside it.

### 5.1 Application context — outside the library

The host application owns:

- command parsing and routing;
- domain entities and business rules;
- networking, storage, Git, builds, tests, downloads, and subprocesses;
- retries, transactions, timeouts, and cancellation;
- concurrency topology;
- process exit and signal policy;
- configuration and dependency injection.

Evident Output accepts facts about those activities. It SHALL NOT require application types to implement presentation interfaces or inherit from library-owned base types.

### 5.2 Presentation context

Owns:

- outputs;
- items;
- tasks;
- problems;
- inferred and explicit conclusions;
- next actions;
- plans and completed changes;
- durable user-facing lines;
- presentation state transitions;
- immutable snapshots.

It does not know ANSI, terminal width, file descriptors, MCP, command frameworks, or application business logic.

### 5.3 Journal context

Owns:

- monotonically ordered durable events;
- event sequence numbers;
- timestamps and clock injection;
- bounded retention and replay;
- event serialization contracts.

It does not decide layout and is not a permanent event-sourced database.

### 5.4 Rendering context

Owns:

- projection of snapshots and events into render trees;
- layout, wrapping, alignment, leaders, and truncation;
- semantic styling;
- frame generation;
- renderer-specific degradation;
- human, plain, JSON, JSONL, and preview projections.

It does not mutate the presentation domain.

### 5.5 Terminal context

Owns:

- TTY capability detection;
- width and height observation;
- cursor visibility;
- live-region erase and redraw;
- ANSI emission;
- signal-aware restoration hooks;
- serialized writes to managed terminal streams.

No other package may emit cursor-control sequences on behalf of the library.

### 5.6 Integration context

Owns adapters for:

- `slog`;
- `io.Writer` line capture;
- Cobra, `urfave/cli`, Kong, and standard `flag` examples/helpers;
- Bubble Tea and other host renderers through snapshots;
- JSON and JSON Lines;
- MCP;
- downstream test harnesses;
- optional custom capability detectors and redactors.

Integration packages depend inward on stable presentation contracts. The core never depends outward on MCP, a command framework, or a full-screen TUI.

### 5.7 Compatibility contract

Evident Output SHALL support three integration modes:

1. **Standalone rendering:** the output owns an inline terminal region and managed output.
2. **Cooperative rendering:** the output owns presentation state while an external system owns final rendering or selected streams.
3. **Projection-only:** callers construct and validate state, then consume snapshots, events, plain text, or structured output without terminal control.

The library SHALL NOT require exclusive process-wide stream ownership. It SHALL make the limits of cooperative use explicit and provide `Suspend`, managed writers, and no-op/external renderer modes.

## 6. Ubiquitous language

| Term                | Meaning                                                              | Ownership                     |
| ------------------- | -------------------------------------------------------------------- | ----------------------------- |
| `Output`            | One command invocation's presentation lifecycle                      | Aggregate root                |
| `Item`              | One named fact, condition, or result meaningful in final output      | Output-owned entity           |
| `Task`              | One operation with phases or progress                                | Output-owned leaf entity      |
| `Tasks`             | A named collection of independent child tasks                        | Output-owned aggregate entity |
| `Problem`           | Concrete evidence explaining warning, block, failure, or uncertainty | Immutable value               |
| `Changes`           | Durable effects that already occurred                                | Output-owned report section   |
| `Plan`              | Effects that would occur but have not occurred                       | Output-owned report section   |
| `Action`            | A recommended next step                                              | Immutable value               |
| `Command`           | Executable and argv used by an action                                | Immutable value               |
| `Conclusion`        | The inferred or explicitly overridden meaning of the whole command   | Immutable final value         |
| `Message`           | Durable user-facing or diagnostic text                               | Journaled value               |
| `Snapshot`          | Immutable complete presentation state at a version                   | Produced by Output            |
| `Event`             | Immutable state mutation or durable message                          | Produced by Output/journal    |
| `Projection`        | A rendering of semantic state for a consumer                         | Adapter                       |
| `TerminalDriver`    | Exclusive owner of terminal control sequences                        | Infrastructure                |
| `CapabilityProfile` | Terminal facts and feature policies                                  | Environment value             |

An `Item` is not an arbitrary output line. It is one named condition that remains useful in final output. Temporary actions belong in task phases or debug messages.

A `Task` is one operation. A `Tasks` collection is several operations. A `Task` never contains another task.

## 7. Aggregate ownership and inference

### 7.1 Output as aggregate root

`Output` owns:

- immutable IDs and deterministic declaration order;
- item, task, task-collection, changes, and plan registries;
- lifecycle validation and inferred activation;
- problem and action aggregation;
- conclusion inference and explicit overrides;
- durable messages and logs;
- the event journal and snapshot version;
- projection and terminal lifecycles;
- finish, flush, and cleanup semantics.

Handles contain only immutable identity plus a reference to their owning output facade.

### 7.2 Presentation facade over a strict transition engine

Common calls do not return errors:

```go
item.OK()
task.Phase("resolving packages")
task.Donef("installed %d packages", count)
```

Internally, every call uses the same strict transition engine used by tests and advanced APIs. Misuse:

1. preserves the first typed error;
2. records a diagnostic when safe;
3. preserves the last valid semantic state;
4. surfaces through `Output.Err`, `Finish`, `Close`, test assertions, and strict mode;
5. may panic only under an explicit test/development policy.

### 7.3 Identity and ordering

Every output entity has:

- an immutable generated ID;
- deterministic declaration order;
- an optional caller-provided external key in advanced APIs;
- a display name that need not be unique.

Display names are never identity. Concurrent declaration order is inherently scheduling-dependent, so callers SHOULD declare entities in intended display order before launching concurrent work. Advanced callers MAY provide explicit order keys.

### 7.4 Safe inference rules

1. `Item`, `Task`, and `Tasks.Task` declare entities immediately.
2. `Phase`, `Progress`, `Bytes`, and `Advance` start a pending task.
3. `OK`, `Warn`, `Block`, `BlockedBy`, `Fail`, `Unknown`, and `Skip` may resolve an item directly.
4. `Done`, `Warn`, `Fail`, `Cancel`, and `Skip` may resolve a task directly.
5. Instant terminal resolution before the visibility threshold produces no transient frame.
6. `Tasks` derives state from children and has no independent state mutation methods.
7. A collection success summary is displayed only when all relevant children complete successfully.
8. A collection failure or warning summary is generated from child state unless explicitly supplied through an advanced annotation.
9. Blocked items imply a blocked conclusion unless an application/output-level failure exists.
10. Failed items or tasks imply a failed conclusion.
11. Warnings imply warning only when no stronger state exists.
12. Changes set `Conclusion.Changed=true` but do not erase failure, block, warning, cancellation, or partial state.
13. Plans imply `planned`, not `changed`.
14. Child actions are promoted, deduplicated semantically, and remain ordered.
15. Inference depends only on final semantic state and explicit ordering, never goroutine completion order.
16. Evo never invents business-domain explanations or remediation.

### 7.5 Invariants

1. An entity belongs to exactly one `Output`.
2. IDs and explicit keys are unique.
3. The first terminal state wins.
4. A terminal state cannot return to active state.
5. Annotation calls such as `Because` and `NextCommand` remain legal after item resolution but before `Finish`.
6. All mutation is rejected after `Finish` begins.
7. `BlockedBy`, `WarnedBy`, and `FailedBy` require at least one problem.
8. `Detail` is user-visible text and accepts strings only.
9. `Cause` accepts errors and is diagnostic by default.
10. Progress values cannot be negative.
11. Completed progress cannot exceed total.
12. Totals may increase; they may not decrease below completed progress.
13. Progress regressions require an explicit reset/restart API.
14. Unresolved items or tasks at `Finish` are never silently converted to success, skip, or cancellation.
15. A `Tasks` collection cannot be manually completed or failed.
16. Renderer failure never corrupts semantic state.
17. Human, plain, JSON, JSONL, snapshot, and exit-code conclusions agree.
18. Caller-provided mutable collections are defensively copied.
19. Output close is idempotent and terminal modes are restored on every exit path.

## 8. State machines

### 8.1 Output state

```text
created → active → finishing → closed
              └→ degraded ───→ finishing → closed
```

`Finish` stops new semantic mutations, validates unresolved entities, computes the conclusion, emits final projections, and flushes. `Close` performs best-effort finish when needed and always restores terminal state.

### 8.2 Item state

```text
pending → running → ok
                  → warning
                  → blocked
                  → failed
                  → unknown
                  → skipped

pending ─────────→ ok | warning | blocked | failed | unknown | skipped
```

Items do not expose fractional progress. `Start` may exist in the advanced API when the caller explicitly wants an unresolved item to become visible.

Semantic distinction:

- `Block`: the item was evaluated successfully and found a condition preventing intended progress.
- `Fail`: the item itself could not be evaluated or established successfully.
- `Unknown`: the item could not be determined, but that uncertainty is distinct from an operational failure.

### 8.3 Task state

```text
pending → running → done
                  → warning
                  → failed
                  → cancelled
                  → skipped

pending ─────────→ done | warning | failed | cancelled | skipped
```

`Phase`, `Progress`, `Bytes`, and `Advance` activate a pending task. `Done` clears active phase text and retains safe completed measurements.

### 8.4 Tasks collection state

A collection is derived from child tasks:

```text
no children                   empty
any child running             running
any child failed              failed
otherwise any child warning   warning
otherwise any child cancelled cancelled or partial, by policy
all children done/skipped     done
any unresolved at Finish      incomplete + misuse error
```

The final `Conclusion` retains independent dimensions such as `Changed`, `Partial`, and `Cancelled`; the human headline remains singular.

### 8.5 Progress model

Primary methods use absolute progress:

```go
task.Progress(completed, total)
task.Bytes(completedBytes, totalBytes)
```

Incremental progress is explicit:

```go
task.Advance(delta)
```

Primary documentation SHALL NOT teach ambiguous stateful combinations such as `Total` plus `Add` or `Set`. Advanced APIs may expose measurement configuration where evidence shows a real need.

Validation rules:

- values use wide integer types;
- completed and total are non-negative;
- zero total is valid only with zero completed;
- completed cannot exceed total;
- totals may increase dynamically;
- totals may not decrease below completed;
- terminal tasks reject progress updates;
- overflow records misuse rather than wrapping;
- backward progress requires explicit `Restart` or `ResetProgress` and produces an observable event.

### 8.6 Conclusion

```go
type Conclusion struct {
    State      ConclusionState
    Subject    string
    Changed    bool
    Partial    bool
    Cancelled  bool
    Explanation string
    Items      []ItemSnapshot
    Tasks      []TaskSnapshot
    Collections []TasksSnapshot
    Changes    []ChangesSnapshot
    Plans      []PlanSnapshot
    Actions    []Action
    ExitCode   int
}
```

Default headline precedence:

```text
failed
blocked
warning
cancelled
changed
planned
ready
unchanged
```

Precedence selects the human headline; it does not discard orthogonal dimensions. A command may be `failed` and also `changed=true, partial=true`.

### 8.7 Visibility threshold

Creating an item or task records intent but does not immediately flash a live row. A terminal result before the threshold emits only final output. A phase or progress update makes state running immediately, while rendering remains subject to frame scheduling and visibility policy.

# Part III — Public API

## 9. Package topology

```text
/                         root `evo` package: common presentation API
/render                   render tree and narrow projection contracts
/terminal                 inline terminal projection
/plain                    stable human-readable projection
/jsonout                  JSON and JSON Lines projections
/logbridge                optional logger adapters
/testkit                  virtual terminal, clocks, assertions, fixtures
/schema                   versioned declarative and MCP schemas
/agent                    guidance catalog, review rules, analyzers
/mcpbridge                optional MCP adapter
/cmd/evident-output       review/preview/explain CLI
/cmd/evident-output-mcp   stdio MCP server
/internal/...             synchronization, layout, ANSI, width, scheduler
```

The root package SHALL import no MCP, HTTP, command-framework, or full-screen TUI dependencies.

### 9.1 Naming

- Product: **Evident Output**.
- Repository/module: `evident-output`.
- Go package: `evo`.
- CLI: `evident-output`.
- MCP server: `evident-output-mcp`.
- MCP namespace: `evident_output.*`.
- Resource URI scheme: `evident-output://`.

```go
import "github.com/<org>/evident-output"

out := evo.For("bpp-csharp")
```

### 9.2 Public-interface policy

Core domain types are concrete. Interfaces are exported only when callers provide behavior, such as clocks, capability detection, redaction, or a proven projection boundary. Interfaces are not exported merely for mocking.

### 9.3 Dependency direction

```text
command frameworks / loggers / TUIs / MCP
                    ↓
              integration adapters
                    ↓
       output state, snapshots, values
                    ↓
           invariants and semantics
```

## 10. Output construction and lifetime

### 10.1 Common constructors

```go
type Option interface { /* sealed */ }

func New(options ...Option) *Output
func For(subject string, options ...Option) *Output

func To(writer io.Writer) Option
func Diagnostics(writer io.Writer) Option
func Plain() Option
func NonInteractive() Option
func NoColor() Option
func Width(columns int) Option
func Terminal(driver Terminal) Option
func Clock(clock Clock) Option
func VisibilityDelay(delay time.Duration) Option
func MaxFrameRate(framesPerSecond int) Option
func Strict() Option
```

`New` creates output without a primary subject. `For` creates output for a subject.

```go
out := evo.For(repo)
defer out.Close()
```

Construction is lazy: no goroutine, cursor change, or terminal mode begins until presentation work requires it.

### 10.2 Lifecycle

```go
func (o *Output) Finish() error
func (o *Output) Close() error
func (o *Output) Err() error
func (o *Output) Snapshot() Snapshot
func (o *Output) Snapshots(ctx context.Context) <-chan Snapshot
func (o *Output) Events(ctx context.Context) <-chan Event
func (o *Output) Conclusion() Conclusion
```

`Finish`:

1. stops accepting semantic mutations;
2. validates unresolved entities;
3. computes the conclusion;
4. promotes and deduplicates actions;
5. emits final projections;
6. flushes writers and renderers;
7. returns accumulated misuse or rendering errors.

`Close` is idempotent, performs best-effort finish if needed, and always restores terminal state. Programs requiring strict delivery SHOULD return `Finish` and defer `Close` for cleanup:

```go
out := evo.For(repo)
defer out.Close()

// work

return out.Finish()
```

A helper may manage output lifetime only:

```go
func Run(subject string, fn func(*Output) error, options ...Option) error
```

`Run` SHALL NOT schedule tasks, create application goroutines, infer cancellation, or reinterpret ordinary Go errors.

### 10.3 Advanced construction

```go
type Config struct {
    Subject      string
    Output       OutputConfig
    Projection   ProjectionPolicy
    Theme        Theme
    Policy       Policy
    Clock        Clock
    Redactor     Redactor
    Capabilities *CapabilityProfile
}

func NewWithConfig(config Config) (*Output, error)
```

Advanced construction exists for deterministic tests, writer ownership, projection-only use, explicit capabilities, strict misuse policy, and embedding.

## 11. Item API

### 11.1 Common API

```go
func (o *Output) Item(name string) *Item

func (i *Item) OK() *Item
func (i *Item) Warn(summary string, options ...ProblemOption) *Item
func (i *Item) WarnedBy(problems ...Problem) *Item
func (i *Item) Block(summary string, options ...ProblemOption) *Item
func (i *Item) BlockedBy(problems ...Problem) *Item
func (i *Item) Fail(summary string, options ...ProblemOption) *Item
func (i *Item) FailedBy(problems ...Problem) *Item
func (i *Item) Unknown(summary string, options ...ProblemOption) *Item
func (i *Item) Skip(reason string) *Item
func (i *Item) Because(text string) *Item
func (i *Item) Next(actions ...Action) *Item
func (i *Item) NextCommand(executable string, args ...string) *Item
func (i *Item) Snapshot() ItemSnapshot
```

`Item` creates one named final-report entry. It is not a transient log event.

Simple evidence:

```go
workingTree.Block(
    "unstashed changes",
    evo.Detail("commit or stash the changes before continuing"),
    evo.Cause(err),
)
```

Structured evidence:

```go
branches.BlockedBy(problems...)
```

The grammar is intentional:

- `Block` means one simple reason;
- `BlockedBy` means structured evidence already exists;
- equivalent `WarnedBy` and `FailedBy` forms preserve symmetry.

### 11.2 Item lifecycle rules

- `OK`, `Warn`, `WarnedBy`, `Block`, `BlockedBy`, `Fail`, `FailedBy`, `Unknown`, and `Skip` are terminal state mutations.
- The first terminal state wins.
- `Because`, `Next`, and `NextCommand` are annotations and remain legal until `Finish` begins.
- A `...By` call with no problems records `ErrNoProblems` and leaves the item unresolved.
- An explicit advanced `Start` may make a slow unresolved item visible.

### 11.3 Advanced item specification

```go
type ItemSpec struct {
    Key         string
    Name        string
    Description string
    Order       int
    Hidden      bool
    ManualStart bool
}

func (o *Output) ItemWith(spec ItemSpec) (*Item, error)
```

## 12. Problem API

```go
type Problem struct {
    Code      string
    Subject   string
    Summary   string
    Detail    string
    Severity  Severity
    Count     int64
    Unit      string
    Location  *Location
    Evidence  []Evidence
    Actions   []Action
    Fields    []Field
    Cause     error
    Sensitive bool
}
```

Options are sealed typed values:

```go
func Detail(text string) ProblemOption
func Cause(err error) ProblemOption
func Code(value string) ProblemOption
func On(subject string) ProblemOption
func Count(value int64, unit ...string) ProblemOption
func At(path string, line, column int) ProblemOption
func Next(action Action) ProblemOption
func NextCommand(executable string, args ...string) ProblemOption
```

`Detail` accepts only strings because it is user-visible. `Cause` accepts errors and is diagnostic by default. Renderers MUST NOT expose raw causes in ordinary output unless explicit policy allows it.

Struct literals remain the preferred form for collections:

```go
problems := []evo.Problem{
    {
        Subject: "feat/sdk-full-consolidation",
        Summary: "local-only",
        Count:   1,
        Unit:    "commit",
    },
    {
        Subject: "fix/login-flow",
        Summary: "ahead of origin",
        Count:   2,
        Unit:    "commits",
    },
}
```

## 13. Task API

### 13.1 One task

```go
func (o *Output) Task(name string) *Task

func (t *Task) Phase(text string) *Task
func (t *Task) Progress(completed, total int64) *Task
func (t *Task) Bytes(completed, total int64) *Task
func (t *Task) Advance(delta int64) *Task
func (t *Task) Done() *Task
func (t *Task) Donef(format string, args ...any) *Task
func (t *Task) Warn(summary string, options ...ProblemOption) *Task
func (t *Task) Fail(summary string, options ...ProblemOption) *Task
func (t *Task) Cancel(reason string) *Task
func (t *Task) Skip(reason string) *Task
func (t *Task) Next(actions ...Action) *Task
func (t *Task) NextCommand(executable string, args ...string) *Task
func (t *Task) Snapshot() TaskSnapshot
```

`Phase`, `Progress`, `Bytes`, and `Advance` start a pending task. `Done` may resolve a task without a visible running frame.

Absolute methods are preferred because they are self-contained:

```go
download.Bytes(written, totalBytes)
files.Progress(completed, total)
```

`Advance` is explicitly incremental:

```go
files.Progress(0, int64(len(paths)))
for _, path := range paths {
    process(path)
    files.Advance(1)
}
```

`Done` clears active phase wording. It retains useful completed measurements but never converts an active phrase such as `verifying` into a completion summary.

### 13.2 Several tasks

```go
func (o *Output) Tasks(name string) *Tasks
func (g *Tasks) Task(name string) *Task
func (g *Tasks) Summary(text string) *Tasks
func (g *Tasks) Summaryf(format string, args ...any) *Tasks
func (g *Tasks) Snapshot() TasksSnapshot
```

`Tasks` is a collection, not a task with children. It has no `Done`, `Fail`, `Progress`, or `Phase` methods. Its state is derived from child tasks.

```go
dependencies := out.Tasks("dependencies")
react := dependencies.Task("react")
esbuild := dependencies.Task("esbuild")
```

A collection summary is success-oriented. Evo suppresses or qualifies it if child state conflicts. Failure and warning summaries default to deterministic child counts.

### 13.3 Collection screen policy

When all children fit, render declaration order. When they do not fit, choose rows by severity and activity while preserving declaration order within each class:

1. failed;
2. warning;
3. active;
4. pending;
5. successful.

The semantic snapshot always preserves canonical declaration order. The viewport selection MUST expose that rows are omitted:

```text
⠋  dependencies  37/120 complete
   ✗  sharp      checksum mismatch
   ⠋  esbuild    12.4/18.0 MB
   ⠋  zod        verifying
   …  117 not shown
```

### 13.4 Advanced task controls

Advanced APIs MAY expose:

- explicit `Start`;
- explicit IDs and order;
- measurement labels and precision;
- `Restart` or `ResetProgress` for legitimate regressions;
- hidden or final-only tasks;
- custom rate limiting.

They SHALL NOT expose execution helpers such as `RunAll`, `Map`, `Parallel`, `Retry`, `Timeout`, or worker limits in the core package.

## 14. Changes and plans

### 14.1 Changes

```go
func (o *Output) Changes(subject string) *Changes

func (c *Changes) Added(quantity int64, object string) *Changes
func (c *Changes) Created(object string) *Changes
func (c *Changes) Updated(quantity int64, object string) *Changes
func (c *Changes) Reused(quantity int64, object string) *Changes
func (c *Changes) Moved(source, destination string) *Changes
func (c *Changes) Removed(quantity int64, object string) *Changes
func (c *Changes) Wrote(object string) *Changes
func (c *Changes) Record(verb string, quantity int64, object string) *Changes
```

Changes represent effects that already occurred and set `Conclusion.Changed=true`.

### 14.2 Plans

```go
func (o *Output) Plan(subject string) *Plan

func (p *Plan) Add(quantity int64, object string) *Plan
func (p *Plan) Create(object string) *Plan
func (p *Plan) Update(quantity int64, object string) *Plan
func (p *Plan) Move(source, destination string) *Plan
func (p *Plan) Remove(quantity int64, object string) *Plan
func (p *Plan) Delete(quantity int64, object string) *Plan
func (p *Plan) Revoke(quantity int64, object string) *Plan
func (p *Plan) Write(object string) *Plan
func (p *Plan) Retain(description string) *Plan
func (p *Plan) Record(verb string, quantity int64, object string) *Plan
```

Plans describe future effects and never set `Changed`.

### 14.3 Alignment

Change and plan rows contain semantic fields:

```text
verb | optional quantity | object/source/destination
```

Renderers left-align verbs, right-align numeric quantities, left-align objects, and drop decorative padding when width is constrained. Callers never supply padding.

## 15. Messages, logs, and integrations

### 15.1 Durable user-facing lines

```go
func (o *Output) Line(message string)
func (o *Output) Linef(format string, args ...any)
func (o *Output) Info(message string, fields ...Field)
func (o *Output) WarnMessage(message string, fields ...Field)
func (o *Output) ErrorMessage(message string, fields ...Field)
```

`WarnMessage` and `ErrorMessage` avoid ambiguity with item/task terminal methods.

### 15.2 Diagnostic logs

```go
func (o *Output) Trace(message string, fields ...Field)
func (o *Output) Debug(message string, fields ...Field)
func (o *Output) Debugf(format string, args ...any)
func (o *Output) SlogHandler(minLevel slog.Leveler) slog.Handler
func (o *Output) DebugWriter() io.WriteCloser
```

Debug messages are durable and are inserted above a live region. `DebugWriter` buffers partial lines, validates UTF-8, bounds line length, sanitizes control bytes, and never writes directly to a managed terminal.

### 15.3 External output

```go
func (o *Output) Suspend(ctx context.Context, fn func() error) error
```

Evo cannot make arbitrary direct writes safe. Applications use managed methods, `Suspend`, projection-only integration, or static mode.

## 16. Actions, conclusion, and errors

### 16.1 Actions

```go
type Action struct {
    Label                string
    Command              *CommandSpec
    URL                  string
    File                 string
    Explanation          string
    RequiresConfirmation bool
    Destructive          bool
}

type CommandSpec struct {
    Executable string
    Args       []string
    WorkingDir string
}

func Command(executable string, args ...string) Action
func (o *Output) Next(actions ...Action)
func (o *Output) NextCommand(executable string, args ...string)
```

Commands are executable plus argv, not opaque shell strings. Evo never executes an action.

### 16.2 Explicit output failure and explanation

```go
func (o *Output) Fail(summary string, options ...ProblemOption)
func (o *Output) Cancel(reason string)
func (o *Output) Explain(text string)
```

`Output.Fail` means the command could not complete its job. It is distinct from `Item.Block`, which means the command successfully determined a blocking condition.

### 16.3 Error model

Common presentation calls record misuse rather than returning errors. Observable boundaries return errors.

```go
var (
    ErrClosed             = errors.New("evo: output is closed")
    ErrAlreadyResolved    = errors.New("evo: entity is already resolved")
    ErrNoProblems         = errors.New("evo: structured resolution requires problems")
    ErrUnresolvedItem     = errors.New("evo: item has no final state")
    ErrUnresolvedTask     = errors.New("evo: task has no final state")
    ErrInvalidProgress    = errors.New("evo: invalid progress")
    ErrProgressRegression = errors.New("evo: progress moved backward")
    ErrDuplicateKey       = errors.New("evo: duplicate entity key")
    ErrInvalidConfig      = errors.New("evo: invalid configuration")
    ErrRenderer           = errors.New("evo: renderer failure")
)
```

Production default records misuse and preserves the last valid state. Strict test/development mode panics with deterministic context. Silent ignore is prohibited.

### 16.4 Unresolved entities

At `Finish`:

- strict mode returns or panics on unresolved items/tasks;
- normal mode renders them as incomplete/unknown, records typed misuse, and returns an error;
- Evo never fabricates success, skip, or cancellation.

### 16.5 Go errors versus presentation states

An expected negative result may update output and return `nil`:

```go
if len(problems) > 0 {
    branches.BlockedBy(problems...)
    return nil
}
```

An application error may update output and still return the error:

```go
if err := readRepository(); err != nil {
    out.Fail("repository inspection stopped", evo.Cause(err))
    return err
}
```

The application owns that decision.

# Part IV — Internal architecture

## 17. State, journal, and render scheduling

### 17.1 Authoritative state

The output maintains a private state object protected by a mutex. Every common facade call delegates to the same strict transition operation used by advanced APIs and tests.

A strict operation:

1. validates arguments outside the lock where safe;
2. acquires the output lock;
3. validates the state transition or inference refinement;
4. mutates authoritative state only when valid;
5. records activation provenance when activation is explicit, activity-driven, or threshold-driven;
6. assigns event sequence numbers;
7. appends durable events to the journal queue;
8. increments the snapshot version;
9. signals the render scheduler;
10. releases the lock.

If validation fails, the common facade records the typed error without applying the mutation. Under strict test policy it panics after recording diagnostic context. Advanced configuration/definition APIs return the error directly.

The mutex design is preferred over an actor-style synchronous command channel because it is simpler to reason about, avoids self-deadlock when callbacks log, and keeps high-frequency task updates inexpensive. Projection work SHALL occur outside the domain lock.

### 17.2 Immutable snapshots

Renderers receive immutable snapshots. Snapshot production SHALL:

- defensively copy mutable collections;
- preserve insertion order;
- include a monotonically increasing version;
- include only sanitized display text;
- expose both display labels and stable machine keys;
- avoid holding the output lock during layout or I/O.

The implementation MAY use copy-on-write or structural sharing internally after profiling, but version 1 correctness SHALL not depend on such optimization.

### 17.3 Durable journal versus transient invalidation

The architecture SHALL distinguish:

- **durable events**, which must be emitted or retained;
- **dirty signals**, which may be coalesced;
- **render frames**, which may be skipped.

A bounded dirty-signal channel of capacity one is sufficient because multiple mutations can collapse into the latest snapshot. The durable event queue MUST use a separate policy.

### 17.4 Journal backpressure

Default policy:

- log and terminal events are buffered up to a configurable limit;
- when the limit is reached, producers block briefly rather than silently drop durable records;
- after the configured timeout, the output enters a degraded state and emits a single structured overflow diagnostic if possible;
- high-frequency progress changes are never journaled one-for-one unless JSON Lines event mode explicitly requests them;
- even in event mode, progress events MAY be sampled with an explicit `sampled=true` field and documented policy.

No implementation may silently discard errors, final states, problems, conclusions, durable lines, or warnings.

### 17.5 Render scheduler

The scheduler SHALL:

- render immediately for terminal outcomes and conclusions;
- delay initial live UI until the configured visibility threshold, defaulting to approximately 150 ms;
- cap frame rate, defaulting to 20 frames per second;
- coalesce rapid progress changes;
- wake on resize;
- wake on durable log arrival;
- force flush on finish, suspend, and close;
- avoid rendering when the visible projection is unchanged.

Frame-rate and visibility thresholds SHALL be policy, not domain state.

---

## 18. Renderer contract

```go
type Renderer interface {
    ID() string
    Start(context.Context, RenderContext) error
    Render(context.Context, Frame) error
    Emit(context.Context, []Event) error
    Suspend(context.Context) error
    Resume(context.Context) error
    Close(context.Context, FinalFrame) error
}
```

The exact interface may be refined during implementation, but responsibilities SHALL remain separated:

- snapshots/frames represent current state;
- events represent durable history;
- renderer lifecycle is explicit;
- renderers do not mutate the output;
- renderer callbacks MUST NOT occur under the output lock.

A simpler internal interface MAY be used if public custom renderers prove uncommon. The public API review gate SHALL prefer not exporting this interface in v1 unless at least two independent external renderer implementations exist.

### 18.1 Renderer failure isolation

If one renderer fails:

- the output records the first failure;
- other renderers continue where safe;
- interactive rendering is disabled if terminal ownership is compromised;
- the library attempts to restore cursor and terminal modes;
- final structured output may continue if its stream remains healthy;
- `Finish` or `Close` returns the aggregated failure.

### 18.2 Determinism

Given the same snapshot, capability profile, width, height, theme, and clock-neutral options, a renderer SHALL produce byte-identical output.

---

## 19. Render tree

The rendering context SHALL use a semantic intermediate representation rather than concatenate ANSI strings throughout the codebase.

Illustrative nodes:

```go
type Node interface{ isNode() }

type Text struct {
    Value string
    Role  Role
}

type Row struct {
    Cells []Cell
}

type Stack struct {
    Children []Node
    Gap      int
}

type Indent struct {
    Width int
    Child Node
}

type Rule struct { /* ... */ }
```

Public domain objects SHALL not depend on these node types.

Roles include semantic concepts such as:

```text
status.success
status.failure
status.warning
status.active
log.debug
log.error
metadata
identifier
command
leader
normal
```

Themes map roles to terminal styles. Machine renderers ignore styles.

### 19.1 Text safety boundary

There SHALL be two internal text categories:

- `SafeText`: sanitized user/application text with control sequences neutralized;
- `TrustedControl`: internal terminal instructions inaccessible to ordinary callers.

No public API SHALL accept raw ANSI markup by default. An escape hatch, if ever added, MUST be visibly named `UnsafeANSI`, gated behind configuration, excluded from JSON output, and documented as incompatible with width guarantees.

---

## 20. Layout engine

### 20.1 Item rows

Primary item rows follow:

```text
<state>  <name>  <optional summary>
```

Examples:

```text
✓  working tree
!  remotes       remote state is unverified
✗  branches
```

The name column is left-aligned. A summary column may align across sibling items when width permits. Long names or narrow terminals degrade to stacked rows.

### 20.2 Structured problems

A blocked, warned, or failed item expands beneath itself:

```text
✗  branches
   ├─ feat/sdk-full-consolidation  local-only (1)
   └─ fix/login-flow               ahead of origin (2)
```

The renderer preserves problem order. Child layout prioritizes subject, summary, count, and concise detail. Deep or verbose evidence moves to a second indented line.

### 20.3 Task rows

One task follows:

```text
<state>  <name>  <phase/progress/summary>
```

A `Tasks` collection renders a parent summary plus selected children:

```text
⠋  dependencies  2/4 complete
   ✓  react      8.1 MB
   ⠋  esbuild   12.4/18.0 MB
   ⠋  sharp      verifying
   ○  zod
```

Successful children may collapse when the screen budget is constrained. Failed and warning children remain visible.

### 20.4 Change and plan rows

Semantic columns:

```text
verb | quantity | object
```

Default:

```text
[changed]  dependencies
  added    14 packages
  updated   4 packages
  reused   63 cached packages
  wrote       app.lock
```

Rules:

- verbs are left-aligned;
- numeric quantities are right-aligned;
- objects are left-aligned;
- rows without quantity align their object with quantity-bearing rows when space permits;
- no leading padding is added merely to right-align verbs;
- narrow layouts remove decorative alignment before truncating meaning.

### 20.5 Width, wrapping, and omission

- Width is measured in visible terminal cells after removing ANSI.
- Unicode width handling is best-effort and capability-dependent.
- Long values wrap under their semantic column.
- Omissions are explicit, for example `… 117 not shown`.
- Final static output preserves canonical declaration order.
- Interactive viewport selection may prioritize severity/activity while retaining canonical order within each priority class.
- Renderers never truncate stable IDs, error codes, or the only actionable command without an explicit continuation mechanism.

## 21. Terminal driver

### 21.1 Exclusive ownership

Only the terminal driver may:

- hide/show cursor;
- move cursor;
- erase lines;
- enter alternate-screen or raw mode;
- emit ANSI SGR styling;
- install resize listeners;
- coordinate live-region writes.

Version 1 SHALL NOT use alternate-screen or raw mode for ordinary inline output.

### 21.2 Live-region algorithm

For each update:

1. acquire the terminal write lock;
2. restore cursor to the known live-region origin;
3. erase the previous live region using whole-line operations;
4. append queued durable terminal log lines;
5. calculate the new live-region height;
6. write the complete frame;
7. position cursor after the frame;
8. flush when the writer supports flushing;
9. release the lock.

The implementation SHALL never attempt partial diff optimization before a correct full-region redraw exists and is tested. A future diff renderer may be added behind the same driver contract.

### 21.3 Debug output during live UI

Library-managed debug output is durable history. It SHALL appear above the live region or inside a dedicated pane. The default inline renderer uses append-above-and-redraw.

If logs and UI are directed to different terminal devices, each stream SHALL be managed independently. If they refer to the same terminal, writes SHALL be serialized by a shared coordinator where detection is reliable or explicitly configured.

### 21.4 Shared-stream limitation

The library cannot guarantee correct cursor placement when unrelated goroutines or subprocesses write directly to the same terminal. Applications MUST use:

- output logging methods;
- `LineWriter`;
- `SlogHandler`;
- `Suspend` around external output;
- or non-interactive/plain mode.

### 21.5 Suspend/resume

`Output.Suspend` SHALL:

1. stop scheduled frames;
2. flush durable logs;
3. erase the live region;
4. show the cursor;
5. invoke the callback without internal locks;
6. re-detect terminal size and liveness;
7. redraw current state if still interactive;
8. propagate callback and resume errors without losing either.

Nested suspension SHALL be rejected or reference-counted; version 1 SHOULD reject it for simplicity.

### 21.6 Terminal restoration

The driver SHALL register cleanup for normal return, panic recovery within library-owned goroutines, `SIGINT`, `SIGTERM`, and platform-equivalent console events where supported.

The documentation SHALL explicitly state that restoration cannot be guaranteed after `SIGKILL`, power loss, kernel termination, or process corruption.

---

## 22. Capability detection

```go
type CapabilityProfile struct {
    Interactive bool
    Color       ColorLevel
    Unicode     bool
    Width       int
    Height      int
    Dumb        bool
    Hyperlinks  bool
    Platform    Platform
}
```

Detection SHALL consider:

- whether the target writer is a terminal;
- `NO_COLOR`;
- explicit color flags;
- `TERM=dumb`;
- CI environment indicators only as hints, never as stronger evidence than writer capabilities;
- Windows console/ConPTY support;
- locale and encoding;
- terminal dimensions;
- explicit caller overrides.

Precedence:

1. explicit API/CLI override;
2. user-standard disable signals such as `NO_COLOR`;
3. detected writer and terminal capability;
4. conservative fallback.

Capabilities SHALL be injectable for deterministic tests.

---

# Part V — Output contracts

## 23. Streams and projection selection

The public common path does not ask callers to classify commands with abstract intent enums. The output selects a projection from semantic content, stream capabilities, and explicit writer configuration.

### 23.1 Human report projection

A output containing items, tasks, problems, plans, changes, or user-facing lines defaults to a human report. The final durable report normally belongs on stdout when stdout is itself the requested human result. Transient frames and diagnostic logs use a managed interactive stream, commonly stderr when stdout purity matters.

### 23.2 Data-producing commands

When an application writes requested data directly to stdout or selects a structured/data projection:

- stdout is reserved for requested data;
- progress, warnings, and logs use stderr;
- no terminal control bytes enter stdout;
- a broken data pipe follows configured policy;
- the output's final human conclusion is omitted from stdout unless explicitly requested.

Advanced configuration declares writer ownership; the library does not attempt to infer that arbitrary application writes are structured data.

### 23.3 Plans and changes

Plans and mutation results are primary human results and normally use stdout. Their semantics are inferred from added plan/change values or selected explicitly in advanced projection policy.

### 23.4 Structured projections

Selecting JSON or JSONL is explicit because machine contracts must never be guessed from TTY state. A caller may select them through configuration, a CLI adapter, or direct encoder APIs.

### 23.5 Writer identity

The output layer SHOULD determine whether configured writers refer to the same terminal device. Because this is not perfectly portable, callers MAY explicitly declare writer relationships. When uncertain, the renderer chooses a conservative strategy that avoids cross-stream cursor control.

### 23.6 Compatibility with host output

The library supports:

- application data written directly to stdout while UI uses stderr;
- `slog` routed through the output;
- external commands wrapped by `Suspend`;
- a host TUI consuming snapshots;
- final-only and projection-only operation;
- custom writers and files.

It does not promise safe cursor behavior around unmanaged writes to the same terminal.

## 24. Human output modes

### 24.1 Interactive

- cursor-managed live region;
- semantic color;
- Unicode when available;
- debug append-above;
- final static frame.

### 24.2 Plain

- no cursor movement;
- no animation;
- no color;
- stable line grammar;
- coarse phase events only when useful;
- grep-friendly output.

Example:

```text
[CHECK] working tree: ready
[CHECK] branches: blocked
[PROBLEM] branch=feat/sdk-full-consolidation state=local-only commits=1
[CHECK] remotes: ready
[RESULT] blocked repository=bpp-csharp
```

This event grammar SHALL not be treated as the formal machine API unless explicitly versioned. JSON Lines is preferred for automation.

### 24.3 Final-only

For consumers that want a human report without intermediate events, the plain renderer SHALL support final-only mode.

---

## 25. Structured output

### 25.1 Final JSON

```json
{
  "schema_version": "1.0",
  "output": {
    "id": "01J...",
    "subject": "bpp-csharp"
  },
  "conclusion": {
    "state": "blocked",
    "changed": false,
    "partial": false,
    "cancelled": false,
    "exit_code": 1
  },
  "items": [
    {
      "id": "item-1",
      "name": "working tree",
      "state": "ok",
      "problems": []
    },
    {
      "id": "item-2",
      "name": "branches",
      "state": "blocked",
      "problems": [
        {
          "subject": "feat/sdk-full-consolidation",
          "summary": "local-only",
          "count": 1,
          "unit": "commit"
        }
      ]
    }
  ],
  "task_collections": [
    {
      "id": "tasks-1",
      "name": "dependencies",
      "state": "failed",
      "children": ["task-1", "task-2"]
    }
  ],
  "tasks": [],
  "changes": [],
  "plans": [],
  "actions": []
}
```

Raw diagnostic causes are omitted by default. Structured diagnostic modes may include redacted cause metadata under explicit policy.

### 25.2 JSON Lines

```json
{"schema_version":"1.0","sequence":1,"type":"output.started","output_id":"01J..."}
{"schema_version":"1.0","sequence":2,"type":"item.declared","entity_id":"item-1","name":"working tree"}
{"schema_version":"1.0","sequence":3,"type":"item.ok","entity_id":"item-1","activation":"implicit"}
{"schema_version":"1.0","sequence":4,"type":"tasks.declared","entity_id":"tasks-1","name":"dependencies"}
{"schema_version":"1.0","sequence":5,"type":"task.progress","entity_id":"task-1","completed":12,"total":18,"unit":"bytes"}
{"schema_version":"1.0","sequence":6,"type":"output.finished","state":"blocked","source":"inferred"}
```

Events have strictly increasing sequence numbers within one output. Snapshots preserve canonical declaration order independent of completion order.

### 25.3 Schema evolution

- additive optional fields are backward-compatible;
- enum additions require consumers to handle unknown values;
- breaking changes require a new schema major version;
- human rendering may evolve without changing semantic schemas;
- IDs, state values, and event names are stable contracts.

### 25.4 Projection APIs

Pure projection functions accept immutable snapshots and do not require terminal ownership:

```go
func RenderPlain(snapshot Snapshot, options PlainOptions) ([]byte, error)
func EncodeJSON(snapshot Snapshot, options JSONOptions) ([]byte, error)
func EncodeJSONL(events []Event, options JSONLOptions) ([]byte, error)
```

## 26. Exit-code integration

The library does not call `os.Exit`. It recommends or validates exit codes.

Default mapping:

| Conclusion  |                         Recommended exit |
| ----------- | ---------------------------------------: |
| `ready`     |                                        0 |
| `changed`   |                                        0 |
| `unchanged` |                                        0 |
| `warning`   |               0 by default, configurable |
| `blocked`   |                                        1 |
| `failed`    |                                        2 |
| `cancelled` | 130 on interrupt, otherwise configurable |

Applications remain responsible for process exit. Structured output SHALL include the recommended code. Human conclusion, structured conclusion, and actual process exit SHOULD agree.

---

# Part VI — Security, privacy, and trust

## 27. Content security

### 27.1 Terminal escape injection

All caller-provided text SHALL have terminal control characters neutralized. At minimum:

- ESC;
- CSI and OSC introducers;
- carriage return;
- backspace;
- bell;
- non-printable C0/C1 controls;
- bidirectional text controls according to policy;
- invalid UTF-8.

Newlines are permitted only in fields explicitly documented as multiline. Ordinary names and summaries SHALL normalize newlines to visible escaped forms or spaces.

### 27.2 Hyperlinks

OSC 8 hyperlinks, if supported, SHALL be generated only by the trusted style layer. URLs SHALL be parsed and allowlisted by scheme. User text SHALL never become a hyperlink target through string concatenation.

### 27.3 Redaction

Fields and problems MAY be marked sensitive. A redactor SHALL run before journaling to any externally visible renderer. Redaction MUST occur before debug logs are queued, not only at final formatting.

The test suite SHALL include secrets split across writer chunks and structured nested values.

### 27.4 Resource exhaustion

The library SHALL cap or configure:

- entity count;
- problems per entity;
- evidence size;
- line length;
- log queue size;
- render-tree depth;
- frame size;
- MCP request body size;
- MCP execution duration.

Exceeding limits SHALL produce structured, bounded diagnostics rather than panic or allocate without bound.

### 27.5 Command display safety

Recommended commands are structured argv. Rendered shell strings SHALL use platform-appropriate quoting and SHALL mark destructive actions. AI-facing schemas SHALL return argv separately from display text and include `requires_confirmation` and `destructive` flags.

### 27.6 Dependency security

CI SHALL run:

- `govulncheck ./...`;
- dependency license items;
- reproducible module verification;
- static analysis;
- fuzz targets for parsers and sanitizers.

The project SHALL prefer small, maintained dependencies and document why each direct dependency exists.

---

# Part VII — MCP support

## 28. MCP and agent-assistance design

### 28.1 Product objective

The MCP companion exists to make AI agents materially better at designing and implementing understandable CLI output even when the model has little or inconsistent training data about Evident Output.

The design adopts the most valuable pattern demonstrated by Svelte's official agent tooling:

```text
discover authoritative guidance
        ↓
retrieve only task-relevant sections
        ↓
write or edit implementation
        ↓
review deterministically
        ↓
repair every applicable finding
        ↓
review again until clean
        ↓
produce an inspectable preview
```

The MCP server SHALL not merely expose documentation search. It SHALL create a convergent workflow with explicit stopping conditions and outputs that both agents and humans can inspect.

Version 1 agent support SHALL be:

- optional and isolated from the root package;
- stdio-first;
- deterministic;
- useful without model sampling;
- read-only with respect to caller source by default;
- non-executing with respect to shell commands and suggested actions;
- schema-driven;
- available through equivalent CLI commands where practical;
- capable of reviewing Go source, declarative scenes, snapshots, event streams, and captured terminal output;
- explicit about whether another review call is required.

### 28.2 Agent workflow contract

The shipped task prompt and skills SHALL direct agents to:

1. identify the command's user question and output job;
2. retrieve applicable guidance before designing unfamiliar output;
3. use the smallest Evident Output API that expresses the domain;
4. avoid configuration and lifecycle calls that the library can infer;
5. review the resulting source or scene;
6. correct every error and each applicable warning;
7. call review again while `recheck_required` is true;
8. generate a profile matrix preview;
9. report unresolved policy choices separately from correctness findings.

The server SHALL make this loop easy to follow through tool descriptions and structured fields. It SHALL not depend on a model remembering an unstated multi-call convention.

### 28.3 Transport

`evident-output-mcp` supports stdio in version 1. Under stdio:

- stdin and stdout are reserved exclusively for MCP JSON-RPC messages;
- all server logs use stderr;
- no human banner, spinner, ANSI frame, or startup advice is printed to stdout;
- protocol version and capabilities are negotiated during initialization;
- requests support deadlines and cancellation;
- local file paths are accepted only by tools whose schema explicitly permits them;
- path access is bounded by configured roots or the process working directory policy.

A remote/HTTP distribution, if later justified, SHALL use code/text payloads rather than arbitrary server-side file paths. It MUST bind locally by default, validate origin, authenticate non-local access, enforce limits, and follow the then-current MCP transport and authorization requirements.

### 28.4 Tool catalog

Tool names use `evident_output.<verb>`. Names SHALL remain 1–128 characters, case-sensitive, unique within the server, and limited to ASCII letters, digits, underscore, hyphen, and dot for broad client compatibility.

#### 28.4.1 `evident_output.list_guides`

Returns the authoritative guidance catalog with task-oriented metadata.

Each entry includes:

```json
{
  "id": "items.parallel",
  "title": "Parallel items",
  "summary": "Represent independent bounded validations as a stable evidence ledger.",
  "use_cases": [
    "concurrent validation",
    "repository safety items",
    "multiple independent pass/fail questions",
    "expandable failure evidence"
  ],
  "concepts": ["Output", "Item", "Problem", "Conclusion"],
  "rules": ["API-014", "TERM-004", "TERM-005", "CON-002"],
  "related": ["problems.multiple", "terminal.debug-interleaving"],
  "estimated_tokens": 720
}
```

The catalog SHALL expose `use_cases`, not only titles. This allows an agent to select guidance from the user's task rather than guessing from document names.

Optional filters:

- concept;
- use-case text;
- rule ID;
- output mode;
- language/API surface;
- maximum token estimate.

#### 28.4.2 `evident_output.get_guidance`

Retrieves one or more guidance sections in one call.

Input:

```json
{
  "ids": ["items.parallel", "problems.multiple", "terminal.debug-interleaving"],
  "detail": "compact",
  "token_budget": 5000
}
```

A guidance section includes:

- stable ID and version;
- when to use and when not to use it;
- domain explanation;
- minimal Go example;
- advanced Go example only when necessary;
- expected terminal output;
- invariants;
- counterexamples;
- related review rules;
- relevant edge-case tests;
- migration notes for deprecated APIs.

The tool supports exact IDs, aliases, normalized titles, and fuzzy suggestions. Partial success is allowed: valid sections are returned while unresolved identifiers receive alternatives. The tool SHALL respect the requested token budget and indicate omitted related sections.

#### 28.4.3 `evident_output.review`

Reviews a source artifact or output artifact against the specification and API rules.

Supported input kinds:

- Go source text;
- local Go file path in stdio mode;
- local package/directory path under configured roots in stdio mode;
- declarative presentation document;
- semantic snapshot;
- JSONL event stream;
- plain terminal transcript;
- ANSI terminal transcript with capability profile;
- terminal preview bundle generated by this project.

Example input:

```json
{
  "kind": "go_source",
  "path": "./cmd/item.go",
  "profiles": ["interactive-80x24", "plain", "ascii-40x12"],
  "ruleset": "recommended"
}
```

Example output:

```json
{
  "valid": false,
  "summary": {
    "errors": 1,
    "warnings": 2,
    "suggestions": 1
  },
  "findings": [
    {
      "rule": "API-006",
      "severity": "warning",
      "message": "Task.Start is unnecessary because Phase starts a declared task implicitly.",
      "location": {
        "file": "cmd/item.go",
        "line": 84,
        "column": 5
      },
      "why": "The common API should contain only domain information that cannot be inferred.",
      "suggested_change": "Remove dependencies.Start().",
      "guidance": ["tasks.lifecycle-inference"],
      "autofix_safe": true
    },
    {
      "rule": "STREAM-003",
      "severity": "error",
      "message": "Progress output is written to stdout while stdout contains JSON.",
      "location": {
        "file": "cmd/item.go",
        "line": 112,
        "column": 9
      },
      "why": "Machine output must remain free of transient frames and diagnostics.",
      "suggested_change": "Configure the output UI writer as stderr or use the data projection adapter.",
      "guidance": ["streams.data-output"],
      "autofix_safe": false
    }
  ],
  "recheck_required": true
}
```

`recheck_required` is true when:

- any error exists;
- a warning requires verifying a caller-applied change;
- an autofix or suggested change could alter subsequent analysis;
- profile-specific rendering has not yet been validated after source changes.

It is false only when no required correction remains and all requested analyses completed.

Review layers include:

1. **API clarity:** unnecessary config, redundant start calls, ambiguous fluent chains, use of advanced APIs where common APIs suffice.
2. **Domain ownership:** progress detached from tasks, application errors confused with failed items, commands represented as shell strings, output library owning business behavior.
3. **Lifecycle:** invalid or duplicate terminal transitions, unresolved children, progress overflow, updates after conclusion.
4. **Concurrency:** nondeterministic ordering assumptions, output misuse across goroutines, missing final synchronization where required by application logic.
5. **Streams:** stdout contamination, debug output bypass, cursor control in non-TTY paths.
6. **Human comprehension:** missing conclusion, color-only meaning, vague problems, count without evidence, missing next action where action is known.
7. **Layout:** ANSI-aware width, narrow fallback, overlong live region, broken multiple-problem hierarchy.
8. **Accessibility:** no-color and ASCII semantics, blink, unsafe contrast assumptions.
9. **Structured output:** schema, deterministic order, conclusion/exit mismatch, secrets.
10. **Security:** terminal injection, shell quoting, destructive command metadata, arbitrary path/network behavior.

The Go reviewer SHOULD use the Go parser, type information when available, vet-style printf analysis, and project-specific static rules. It SHALL not require executing the caller's package.

#### 28.4.4 `evident_output.preview`

Produces an inspectable compatibility matrix from a declarative scene, snapshot, or event stream.

Default profiles:

```text
interactive · 120×40 · Unicode · color
interactive · 80×24  · Unicode · color
interactive · 40×12  · Unicode · no color
interactive · 80×24  · ASCII   · no color
plain       · non-TTY
CI          · non-TTY
interactive · debug interleaving
JSON
JSONL
```

Output includes, as applicable:

- final plain snapshots;
- ANSI byte snapshots;
- virtual-terminal final screens;
- sampled frames;
- durable transcript;
- computed visible widths;
- wrap and truncation annotations;
- capability decisions;
- accessibility findings;
- conclusion and recommended exit code;
- sanitized HTML review artifact;
- recording data compatible with an external terminal recorder format when enabled.

The tool SHALL not execute arbitrary caller code. To preview Go code, the caller or agent supplies a declarative scene/event fixture, or uses generated test instrumentation to capture one.

#### 28.4.5 `evident_output.explain`

Explains a stable diagnostic or rule ID.

Input:

```json
{ "rule": "API-006" }
```

Output includes:

- protected invariant;
- why the rule exists;
- bad and good code examples;
- bad and good output examples where relevant;
- safe remedies;
- exceptions;
- related guidance;
- related verification IDs;
- first version and deprecation status.

#### 28.4.6 Optional future `evident_output.rewrite`

A source-mutating tool is excluded from the initial release. A future local-only rewrite tool may apply mechanically safe changes only after:

- review rules have mature false-positive data;
- every edit has a deterministic source transformation;
- edits are returned as patches before application;
- caller confirmation is explicit;
- no shell or arbitrary code execution is required.

Agent models are already capable of editing source. The library's first responsibility is to be the deterministic authority that explains and verifies those edits.

### 28.5 MCP resources

The server SHOULD expose immutable, versioned resources:

```text
evident-output://catalog/guides
evident-output://guides/items.parallel
evident-output://guides/tasks.lifecycle-inference
evident-output://guides/terminal.debug-interleaving
evident-output://schema/presentation/1.0
evident-output://schema/events/1.0
evident-output://schema/review-result/1.0
evident-output://examples/items/blocked
evident-output://examples/tasks/concurrent
evident-output://examples/logging/debug-live
evident-output://rules/API-006
```

Resources are generated from the same source as the documentation site, embedded into released binaries, and checksum-versioned. MCP, CLI, website, and repository guidance SHALL not drift independently.

### 28.6 MCP prompts

The server provides at least one user-invoked prompt:

#### `build_understandable_cli_output`

The prompt embeds or references the guide catalog and tells the agent to perform the full discover–implement–review–repair–preview loop.

Optional focused prompts:

- `review_existing_cli_output`;
- `migrate_fmt_to_evident_output`;
- `design_parallel_checks`;
- `design_task_progress`;
- `diagnose_terminal_corruption`.

Prompts produce guidance and review steps. They do not claim to execute shell commands or mutate repositories.

### 28.7 Skills and subagent distribution

The repository SHALL ship agent-neutral skill content and thin integrations for major coding agents:

```text
/skills/evident-output/SKILL.md
/agents/evident-output-engineer.md
/integrations/claude-code/
/integrations/codex/
/integrations/gemini/
/integrations/grok/
/integrations/opencode/
```

The specialized subagent instructions own repetitive guidance retrieval, file review, repair, re-review, and preview generation. The main agent receives a compact summary, changed files, verification results, and unresolved design choices.

### 28.8 CLI parity

Every deterministic MCP capability has a CLI equivalent:

```text
evident-output guides
evident-output guide items.parallel
evident-output review ./cmd/item.go
evident-output review --transcript output.ansi
evident-output preview scenario.json
evident-output explain API-006
```

This supports agents without MCP, CI, editors, pre-commit hooks, humans, and offline environments.

### 28.9 Local file access policy

In stdio mode:

- file-path inputs are opt-in per tool schema;
- paths resolve under configured roots;
- symlink traversal outside roots is rejected;
- maximum file and aggregate package size is enforced;
- binary files are rejected;
- source is read but never modified in v1;
- dependency loading may use Go tooling in analysis-only mode without running package initialization;
- caller may always provide source text instead of a path.

Remote transports do not accept server-local paths.

### 28.10 Security

- no shell execution;
- no execution of suggested commands;
- no source mutation in v1;
- no network access by default;
- input and output size limits;
- per-call deadlines;
- panic containment;
- structured audit logs on stderr;
- no authorization decision based only on tool annotations;
- no sampling, roots, or elicitation required for core v1 tools;
- conformance tests across supported protocol versions;
- redaction before findings or previews are journaled.

### 28.11 Installation

Recommended command:

```text
go install github.com/<org>/evident-output/cmd/evident-output-mcp@latest
```

Configuration generation:

```text
evident-output-mcp config --client claude-code
evident-output-mcp config --client codex
evident-output-mcp config --client gemini
evident-output-mcp config --client grok
```

These commands print configuration only. They do not edit user files unless a future explicit install command is separately designed and confirmed.

### 28.12 Agent-assistance success criteria

The MCP/skill surface is successful only if an agent with no prior Evident Output context can:

1. choose the common API rather than overconfigure it;
2. implement a representative item/task flow that compiles;
3. distinguish expected blocked items from Go errors;
4. preserve stdout purity;
5. repair all deterministic findings;
6. stop when `recheck_required` is false;
7. produce a profile matrix that reveals narrow, no-color, ASCII, CI, and debug behavior;
8. explain why the resulting output is understandable;
9. do so without repository mutation or command execution by the MCP server.

# Part VIII — Verification and quality engineering

## 29. Verification philosophy

Quality is defined as demonstrated behavior, not code coverage alone. Every normative requirement SHALL have at least one verification path:

- a deterministic unit test;
- a property test;
- a fuzz target;
- an integration test using a virtual or real terminal;
- a race-enabled concurrency test;
- a protocol conformance test;
- a manual exploratory test with documented evidence.

The project follows red-green-refactor discipline:

1. express the behavioral requirement as a failing test or executable fixture;
2. implement the smallest correct behavior;
3. make the test pass;
4. refactor behind the passing contract;
5. add adversarial and property cases before broadening the API.

No exported API is considered complete until its invalid-use behavior, concurrency behavior, structured representation, and documentation example are tested.

---

## 30. Test architecture

### 30.1 Unit tests

Unit tests cover:

- state transition tables;
- value validation;
- redaction;
- sanitization;
- shell display quoting;
- visible-cell width;
- line wrapping;
- theme mapping;
- event sequencing;
- JSON schema encoding;
- stream policy;
- capability precedence;
- scheduler decisions using a fake clock.

### 30.2 Property tests

Properties include:

- rendering never emits raw untrusted ESC bytes;
- final rendering is deterministic for equal inputs;
- lines never exceed width except for indivisible tokens explicitly marked unbreakable;
- sequence numbers are strictly increasing;
- completed progress remains within invariant bounds;
- terminal states never return to active states;
- sanitized text remains valid UTF-8;
- plain output contains no ANSI control sequences;
- JSON round-trips preserve semantic state;
- rendering an empty collection never panics;
- closing twice is observationally equivalent to closing once.

### 30.3 Fuzzing

Dedicated fuzz targets SHALL cover:

1. arbitrary UTF-8 and invalid byte sequences through sanitization;
2. ANSI/OSC injection strings;
3. grapheme width and truncation;
4. split writes into `LineWriter`;
5. JSON event decoding and replay;
6. declarative MCP document validation;
7. shell-display quoting for argv;
8. terminal resize sequences;
9. random valid and invalid state-transition sequences;
10. nested structured fields and redaction;
11. pathological numbers, overflows, and totals;
12. render-tree depth and size limits.

Fuzz failures SHALL be checked into the seed corpus.

### 30.4 Race testing

CI SHALL run representative tests with `go test -race ./...` on supported platforms. Race-focused tests SHALL include:

- hundreds of tasks advancing concurrently;
- item completion racing with output conclusion;
- log writes racing with redraw and resize;
- `Close` racing with mutations;
- cancellation racing with task completion;
- multiple `LineWriter` instances writing partial records;
- renderer failure racing with finish;
- duplicate key insertion racing across goroutines;
- snapshot reads during rapid mutation.

### 30.5 Virtual terminal

`testkit` SHALL provide a virtual terminal that records:

- bytes written;
- cursor location;
- cursor visibility;
- line contents;
- width and height;
- erase operations;
- resize events;
- stream identity;
- terminal capability profile.

It SHALL interpret only the control sequences emitted by the library. Tests compare final screen state and durable transcript separately.

### 30.6 Golden tests

Golden files SHALL exist for high-value scenarios, but golden tests SHALL not be the only verification mechanism. Each golden case records:

- capability profile;
- final screen;
- durable plain transcript;
- JSON snapshot;
- JSON Lines events;
- expected exit recommendation.

Golden updates require human review and a generated semantic diff.

### 30.7 Real-terminal integration

Release candidates SHALL be exercised on at least:

- macOS Terminal;
- iTerm2 or another widely used macOS terminal;
- Windows Terminal/ConPTY;
- a Linux terminal such as GNOME Terminal, Konsole, or equivalent;
- tmux;
- SSH output;
- a CI pseudo-terminal;
- redirected stdout;
- redirected stderr;
- both redirected;
- `TERM=dumb`;
- `NO_COLOR`.

The project SHOULD add automated PTY integration on Unix and Windows where stable.

### 30.8 Fault injection

Test adapters SHALL inject:

- short writes;
- write errors after N bytes;
- broken pipes;
- delayed writers;
- blocked writers;
- renderer panic;
- clock jumps;
- resize storms;
- invalid capability reports;
- context cancellation;
- journal saturation;
- MCP request cancellation;
- malformed JSON-RPC frames.

---

### 30.9 Agent effectiveness evaluation

The agent-assistance surface SHALL be evaluated as a product, not only protocol-tested.

A fixed, versioned scenario suite includes:

- migrate raw `fmt`/spinner output to the common API;
- implement three parallel items with multiple branch problems;
- implement phased and measured tasks;
- preserve JSON stdout while showing progress;
- repair debug/live-region corruption;
- diagnose narrow-terminal alignment;
- distinguish an expected blocked item from an application error;
- embed snapshots in a host TUI;
- review a deliberately overconfigured implementation;
- explain and remediate a security finding.

Each scenario is run in at least these conditions:

1. model receives only the user task and package name;
2. model receives the skill but no MCP;
3. model receives MCP tools and task prompt;
4. model receives an intentionally stale or misleading code example plus MCP review.

Metrics:

- compile success;
- semantic correctness;
- number of required review/repair cycles;
- percentage of required findings repaired;
- false-positive and false-negative rates;
- use of common versus advanced API;
- stdout/stderr contract correctness;
- profile preview completeness;
- token and tool-call cost;
- whether the agent stops only at a clean review;
- human reviewer rating of code and output clarity.

Release decisions SHALL not depend on one model vendor. The suite SHOULD cover multiple major agent families when access permits. Model variability is recorded, while deterministic tool correctness remains the hard release gate.

---

## 31. Requirements traceability and edge-case verification matrix

Each concern below is normative. An implementation is not release-ready until every row is automated or explicitly waived with a documented reason and owner.

### 31.1 Domain, inference, and lifecycle

| ID      | Concern                                          | Verification        | Pass criteria                                                          |
| ------- | ------------------------------------------------ | ------------------- | ---------------------------------------------------------------------- |
| DOM-001 | `Output.Item` construction                       | Unit                | Stable generated ID and declaration order; no error branch required    |
| DOM-002 | `Output.Task` construction                       | Unit                | Pending task with indeterminate measurement; no live frame yet         |
| DOM-003 | `Output.Tasks` construction                      | Unit                | Collection owns no independent terminal state or progress              |
| DOM-004 | Duplicate display names                          | Unit                | Allowed; generated IDs remain distinct                                 |
| DOM-005 | Duplicate explicit key                           | Unit + race         | At most one succeeds; `ErrDuplicateKey` is recorded                    |
| DOM-006 | `Item.OK` without explicit start                 | Table + golden      | Direct legal terminal transition; no spinner flash                     |
| DOM-007 | `Item.Block`                                     | Unit + golden       | One anonymous problem is created with summary/options                  |
| DOM-008 | `Item.BlockedBy`                                 | Unit + golden       | Structured problems preserved in caller order                          |
| DOM-009 | `BlockedBy()` with no problems                   | Unit + strict       | State unchanged; `ErrNoProblems`; strict mode panics deterministically |
| DOM-010 | `WarnedBy` and `FailedBy`                        | Table + golden      | Structured evidence attaches with correct terminal state               |
| DOM-011 | First terminal state wins                        | Unit + race         | Later terminal mutation rejected; original state preserved             |
| DOM-012 | Annotation after item resolution                 | Unit                | `Because` and actions accepted until `Finish` begins                   |
| DOM-013 | Mutation after `Finish` begins                   | Unit + race         | Rejected; no semantic change                                           |
| DOM-014 | `Detail(string)`                                 | Compile/API test    | Only user-visible strings accepted                                     |
| DOM-015 | `Cause(error)`                                   | Unit + security     | Cause retained diagnostically and hidden from default human output     |
| DOM-016 | `Task.Phase` without start                       | Unit + fake clock   | Task becomes running and indeterminate                                 |
| DOM-017 | `Task.Progress`                                  | Unit                | Absolute completed/total values stored exactly                         |
| DOM-018 | `Task.Bytes`                                     | Unit                | Absolute bytes stored and formatted by renderer                        |
| DOM-019 | `Task.Advance`                                   | Unit                | Explicit delta increments prior valid progress                         |
| DOM-020 | Progress exceeds total                           | Unit + fuzz         | Last valid state preserved; `ErrInvalidProgress` recorded              |
| DOM-021 | Negative progress                                | Unit + fuzz         | Rejected without arithmetic wrap                                       |
| DOM-022 | Progress moves backward                          | Unit                | Rejected with `ErrProgressRegression` unless explicit restart          |
| DOM-023 | Total increases dynamically                      | Unit                | Accepted when not below completed value                                |
| DOM-024 | Total decreases below completed                  | Unit                | Rejected and last valid state preserved                                |
| DOM-025 | `Task.Done` before visibility threshold          | Fake clock + golden | Final row only; zero transient frames                                  |
| DOM-026 | `Task.Done` after active phase                   | Unit + golden       | Active phase cleared; not reused as completion summary                 |
| DOM-027 | `Task.Donef`                                     | Vet + golden        | Standard Go formatting and durable completion summary                  |
| DOM-028 | Child task starts                                | Unit                | Owning `Tasks` collection derives running state                        |
| DOM-029 | One child fails                                  | Unit + golden       | Collection derives failed state regardless of summary text             |
| DOM-030 | One child warns                                  | Unit + golden       | Collection derives warning when no child fails                         |
| DOM-031 | All children complete                            | Unit + golden       | Collection derives done and may render success summary                 |
| DOM-032 | Success summary with failed child                | Unit + golden       | Summary suppressed or qualified; cannot contradict child state         |
| DOM-033 | Unresolved item at `Finish`                      | Unit + golden       | Rendered incomplete/unknown; `ErrUnresolvedItem` returned              |
| DOM-034 | Unresolved task at `Finish`                      | Unit + golden       | Rendered incomplete; `ErrUnresolvedTask` returned                      |
| DOM-035 | Unresolved child in collection                   | Unit + golden       | Collection incomplete; no fabricated success/skip                      |
| DOM-036 | Blocked item conclusion                          | Table               | Inferred `blocked` unless stronger failure exists                      |
| DOM-037 | Failed item/task conclusion                      | Table               | Inferred `failed`                                                      |
| DOM-038 | Warnings only                                    | Table               | Inferred `warning`                                                     |
| DOM-039 | Changes plus failure                             | Table + JSON        | Headline failed; `Changed=true`, `Partial` retained as applicable      |
| DOM-040 | Plan only                                        | Table               | `planned`; `Changed=false`                                             |
| DOM-041 | Actions promoted                                 | Unit + golden       | Semantic duplicates removed; insertion order preserved                 |
| DOM-042 | Explicit explanation                             | Unit                | Replaces generated generic text without deleting evidence              |
| DOM-043 | `Finish` twice                                   | Unit + race         | Same result; no duplicate final output                                 |
| DOM-044 | `Close` twice                                    | Unit + race         | Idempotent cleanup                                                     |
| DOM-045 | Empty output                                     | Unit + golden       | Neutral documented result; no fabricated subject                       |
| DOM-046 | Caller mutates input problem slice               | Unit                | Output state unchanged due defensive copy                              |
| DOM-047 | Inference independent of completion order        | Property + race     | Equal semantic state produces equal conclusion                         |
| DOM-048 | Expected blocked item inside `errgroup`          | Compile + review    | Presentation state may be negative while callback returns `nil`        |
| DOM-049 | Application error plus `Output.Fail`             | Unit                | Failed conclusion; original Go error remains host-owned                |
| DOM-050 | Common and advanced APIs share transition engine | Differential        | Equivalent actions yield identical snapshots/events                    |

### 31.2 Concurrency and ordering

| ID      | Concern                               | Verification              | Pass criteria                                                                 |
| ------- | ------------------------------------- | ------------------------- | ----------------------------------------------------------------------------- |
| CON-001 | Concurrent task updates               | Race + stress             | No races; final counts correct                                                |
| CON-002 | Out-of-order completion               | Golden + property         | Display order remains configured order                                        |
| CON-003 | Log while redraw occurs               | Virtual terminal + race   | No split live rows; log is durable above region                               |
| CON-004 | Resize while redraw occurs            | Virtual terminal + race   | Screen remains valid and next frame uses new width                            |
| CON-005 | Close during update storm             | Race + fault              | No deadlock; durable terminal states retained                                 |
| CON-006 | Renderer callback logs recursively    | Unit                      | No output-lock reentrancy deadlock                                            |
| CON-007 | Scheduler dirty channel saturation    | Stress                    | Latest state rendered; no unbounded goroutines                                |
| CON-008 | Journal queue saturation              | Fault injection           | Documented backpressure/degraded behavior; no silent loss of critical events  |
| CON-009 | Multiple renderers with one failure   | Fault injection           | Healthy renderers finish; error returned                                      |
| CON-010 | Cancellation races completion         | Race + repeated test      | One terminal state wins deterministically                                     |
| CON-011 | Sequence allocation under concurrency | Property + race           | Strictly increasing unique sequence numbers                                   |
| CON-012 | Concurrent duplicate keys             | Race                      | At most one entity owns key                                                   |
| CON-013 | Snapshot during mutation              | Race + property           | Snapshot internally consistent                                                |
| CON-014 | High-frequency progress               | Benchmark + stress        | Frame rate bounded; final value exact                                         |
| CON-015 | Goroutine leak after close            | Leak test                 | No library goroutines remain after deadline                                   |
| CON-016 | Child updates complete out of order   | Golden + race             | Collection rows retain declaration order                                      |
| CON-017 | Concurrent declarations               | Race + docs               | Safe but order reflects declaration scheduling unless explicit order supplied |
| CON-018 | Duplicate child names                 | Unit                      | Distinct internal IDs; structured output unambiguous                          |
| CON-019 | High-frequency child progress         | Stress + virtual terminal | Updates coalesced; final values exact                                         |

### 31.3 Terminal rendering

| ID       | Concern                                      | Verification        | Pass criteria                                                 |
| -------- | -------------------------------------------- | ------------------- | ------------------------------------------------------------- |
| TERM-001 | Initial operation completes before threshold | Fake clock          | No spinner flash; final output only                           |
| TERM-002 | Operation exceeds threshold                  | Fake clock + golden | Spinner appears once and resolves                             |
| TERM-003 | Sequential phases                            | Golden              | One transient row is replaced, not accumulated                |
| TERM-004 | Parallel items                               | Golden              | One row per item; independent resolution                      |
| TERM-005 | Failed row expands                           | Golden              | Problems appear beneath parent without misalignment           |
| TERM-006 | Debug line during live UI                    | Virtual terminal    | Live region erased, log appended, region redrawn              |
| TERM-007 | Partial writer failure during erase          | Fault               | Cursor restored if possible; interactivity disabled safely    |
| TERM-008 | Cursor hidden on start                       | Virtual terminal    | Cursor shown after close and signal cleanup                   |
| TERM-009 | SIGINT                                       | PTY integration     | Final cleanup occurs and exit behavior documented             |
| TERM-010 | SIGKILL                                      | Documentation test  | No false guarantee; recovery instructions documented          |
| TERM-011 | Terminal width becomes zero                  | Virtual terminal    | Switch to safe plain/event mode                               |
| TERM-012 | Terminal height too small                    | Virtual terminal    | Vertical budget collapses without scrolling corruption        |
| TERM-013 | Resize storm                                 | Stress              | Redraw rate bounded; latest size wins                         |
| TERM-014 | External unmanaged output                    | Integration fixture | Documented corruption detector or explicit unsupported result |
| TERM-015 | Suspend external subprocess                  | PTY integration     | UI removed before child output and restored after             |
| TERM-016 | Suspended callback fails                     | Unit + PTY          | Callback error returned; renderer restoration attempted       |
| TERM-017 | Nested suspend                               | Unit                | Rejected or handled according to documented policy            |
| TERM-018 | Many child tasks fit                         | Golden              | One row per child in declaration order                        |
| TERM-019 | Child tasks exceed height                    | Virtual terminal    | Failure/warning/active priority; omission count explicit      |
| TERM-020 | Completed child collapse                     | Fake clock + golden | Screen pressure may collapse success without hiding failure   |
| TERM-021 | Final collection output                      | Golden              | Canonical order restored or documented compact mode selected  |
| TERM-022 | stdout/stderr same terminal                  | PTY                 | Managed writes serialize without overlap                      |
| TERM-023 | stdout/stderr different targets              | Integration         | No cross-stream cursor assumptions                            |
| TERM-024 | Broken pipe                                  | Integration         | Configured clean failure; no panic or corrupted stderr        |

### 31.4 Layout, Unicode, and text

| ID      | Concern                          | Verification           | Pass criteria                                                |
| ------- | -------------------------------- | ---------------------- | ------------------------------------------------------------ |
| TXT-001 | ASCII text width                 | Unit                   | Width equals terminal cells                                  |
| TXT-002 | Combining marks                  | Unit + fuzz            | No split combining sequence at truncation                    |
| TXT-003 | CJK wide characters              | Unit + golden          | Alignment follows configured width algorithm                 |
| TXT-004 | Emoji with variation selector    | Corpus + real terminal | Conservative width; no semantic loss                         |
| TXT-005 | ZWJ sequence                     | Fuzz + corpus          | Valid UTF-8 and grapheme-safe truncation                     |
| TXT-006 | Invalid UTF-8                    | Fuzz                   | Replaced safely; renderer never panics                       |
| TXT-007 | Embedded ESC/CSI                 | Fuzz + security        | Neutralized; no injected terminal command                    |
| TXT-008 | Embedded OSC hyperlink           | Fuzz + security        | Neutralized unless generated internally                      |
| TXT-009 | Carriage return/backspace        | Fuzz                   | Cannot overwrite prior content                               |
| TXT-010 | Newline in item name             | Unit                   | Normalized or rejected according to field contract           |
| TXT-011 | Very long unbroken token         | Golden                 | Documented overflow/truncation policy; bounded memory        |
| TXT-012 | Long path                        | Golden                 | Configured middle/path-aware truncation works                |
| TXT-013 | ANSI styling and width           | Unit                   | Styled and unstyled visible widths match                     |
| TXT-014 | OSC 8 hyperlink and width        | Unit                   | Link controls count as zero cells                            |
| TXT-015 | Narrow stacked layout            | Golden                 | Detail remains associated with correct parent                |
| TXT-016 | Leader rendering                 | Golden                 | Leaders dim, bounded, and omitted when unnecessary           |
| TXT-017 | Duplicate names                  | Golden                 | Rows remain understandable via order/key-independent details |
| TXT-018 | Bidirectional control characters | Security test          | Escaped, marked, or rejected by policy                       |
| TXT-019 | Huge number of problems          | Limit test             | Bounded output with summary and full structured data policy  |
| TXT-020 | Empty strings                    | Unit                   | Required fields rejected; optional fields omitted cleanly    |

### 31.5 Color and accessibility

| ID       | Concern                        | Verification             | Pass criteria                                                  |
| -------- | ------------------------------ | ------------------------ | -------------------------------------------------------------- |
| A11Y-001 | `NO_COLOR`                     | Environment test         | No color even on TTY unless explicit documented override wins  |
| A11Y-002 | `--color=never` equivalent API | Unit                     | No SGR bytes                                                   |
| A11Y-003 | `--color=always` to non-TTY    | Unit                     | Color emitted only because caller explicitly forced it         |
| A11Y-004 | Meaning without color          | Golden review            | Every state has text/symbol equivalent                         |
| A11Y-005 | ASCII fallback                 | Golden                   | No Unicode; same semantic states                               |
| A11Y-006 | Light/dark themes              | Manual + contrast review | Semantic tokens remain legible                                 |
| A11Y-007 | Screen-reader plain mode       | Manual                   | Stable, non-rewriting transcript is understandable             |
| A11Y-008 | Blinking                       | Static analysis          | Library emits no blink by default                              |
| A11Y-009 | Color only smallest token      | Golden policy            | Paragraphs are not indiscriminately colored                    |
| A11Y-010 | Unknown terminal palette       | Unit                     | Uses named/portable colors or no color, not unsafe assumptions |

### 31.6 Logs and diagnostics

| ID      | Concern                         | Verification | Pass criteria                                                                         |
| ------- | ------------------------------- | ------------ | ------------------------------------------------------------------------------------- |
| LOG-001 | Uppercase bracketed level       | Golden       | `[DEBUG]`, `[WARN]`, etc. stable                                                      |
| LOG-002 | Timestamp injection             | Fake clock   | Deterministic exact timestamps                                                        |
| LOG-003 | Structured field order          | Unit         | Stable canonical order or documented insertion order                                  |
| LOG-004 | Sensitive field                 | Unit + fuzz  | Redacted before journal and every renderer                                            |
| LOG-005 | Split UTF-8 across writes       | Fuzz         | Reassembled or safely replaced                                                        |
| LOG-006 | Unterminated final line         | Unit         | Flushed once according to policy                                                      |
| LOG-007 | Maximum line length             | Limit test   | Bounded truncation with explicit marker                                               |
| LOG-008 | Concurrent line writers         | Race         | No record interleaving                                                                |
| LOG-009 | `slog` groups                   | Unit         | Preserved in structured form                                                          |
| LOG-010 | `slog` error values             | Unit         | Stable safe representation                                                            |
| LOG-011 | Recursive/cyclic values         | Unit         | Bounded representation; no stack overflow                                             |
| LOG-012 | Debug disabled                  | Unit         | Record omitted from human renderer according to level but policy for journal explicit |
| LOG-013 | Debug with final JSON           | Integration  | stdout JSON remains valid; logs stay off stdout                                       |
| LOG-014 | Warning log versus item warning | API test     | Distinct semantics and rendering                                                      |
| LOG-015 | Log burst                       | Stress       | Durable order preserved within configured limits                                      |

### 31.7 Streams, projection, and machine output

| ID      | Concern                                   | Verification          | Pass criteria                                                                 |
| ------- | ----------------------------------------- | --------------------- | ----------------------------------------------------------------------------- |
| OUT-001 | Human report defaults                     | Integration           | Final report uses intended human writer; transient output does not corrupt it |
| OUT-002 | Application data on stdout                | Integration           | UI/logs remain on configured diagnostic stream                                |
| OUT-003 | Progress to stderr in data projection     | Integration           | No progress bytes in stdout                                                   |
| OUT-004 | Plain mode                                | Byte assertion        | No ANSI/cursor controls                                                       |
| OUT-005 | Final JSON                                | Schema validation     | Conforms to embedded schema                                                   |
| OUT-006 | JSON Lines                                | Schema + parser       | One valid object per physical line                                            |
| OUT-007 | Deterministic ordering                    | Repeat test           | Byte-identical with fixed clock/IDs                                           |
| OUT-008 | Inference provenance                      | Schema test           | Explicit/implicit activation and conclusion source represented correctly      |
| OUT-009 | Unknown optional fields                   | Compatibility test    | Older reader ignores safely                                                   |
| OUT-010 | Unknown enum                              | Compatibility fixture | Behavior follows documented major-version rule                                |
| OUT-011 | Timestamp format                          | Unit                  | RFC 3339-compatible documented precision                                      |
| OUT-012 | Exit recommendation                       | Table test            | Matches inferred/explicit conclusion mapping                                  |
| OUT-013 | Actual exit mismatch                      | Integration helper    | Optional validator detects mismatch                                           |
| OUT-014 | JSON with secrets                         | Security              | Redaction policy applied                                                      |
| OUT-015 | Huge event stream                         | Streaming test        | Bounded memory and ordered output                                             |
| OUT-016 | Consumer closes early                     | Broken-pipe test      | Clean documented policy                                                       |
| OUT-017 | Final snapshot after sampled progress     | Replay test           | Exact final progress retained                                                 |
| OUT-018 | Direct pure encoder                       | Unit                  | Snapshot encodes without terminal/output ownership                            |
| OUT-019 | Host writes data while output active      | PTY/integration       | Managed streams remain coherent under documented ownership                    |
| OUT-020 | No subject supplied                       | Golden + JSON         | Human/structured result omits subject rather than guessing                    |
| OUT-021 | Structured projection selected explicitly | API test              | No TTY heuristic changes machine format                                       |
| OUT-022 | Plan/change inferred from values          | Unit + golden         | Correct projection without public intent enum                                 |
| OUT-023 | `Line` while live UI active               | Virtual terminal      | Durable line inserted above and live region redrawn                           |
| OUT-024 | `Linef` formatting                        | Vet + unit            | Standard `fmt` behavior and sanitization                                      |

### 31.8 MCP and agent assistance

| ID      | Concern                                       | Verification              | Pass criteria                                                              |
| ------- | --------------------------------------------- | ------------------------- | -------------------------------------------------------------------------- |
| MCP-001 | Initialization first                          | Protocol test             | Server rejects out-of-lifecycle calls as required                          |
| MCP-002 | Capability negotiation                        | Conformance               | Advertised capabilities match implementation                               |
| MCP-003 | stdout purity in stdio                        | Byte capture              | Every stdout message is valid MCP framing                                  |
| MCP-004 | stderr logging                                | Integration               | Logs never contaminate stdout                                              |
| MCP-005 | Tool list                                     | Schema test               | Stable names, descriptions, input/output schemas                           |
| MCP-006 | `evident_output.list_guides` catalog          | Tool + snapshot           | Every entry has ID, use cases, concepts, rules, token estimate             |
| MCP-007 | Guide filtering by use case                   | Tool test                 | Relevant sections found without exact title knowledge                      |
| MCP-008 | `evident_output.get_guidance` batch retrieval | Tool test                 | Exact sections returned in requested order within budget                   |
| MCP-009 | Unknown guidance ID                           | Tool test                 | Fuzzy alternatives and partial success returned                            |
| MCP-010 | Guidance source consistency                   | Generation test           | MCP, CLI, website, and embedded resources share checksum/source            |
| MCP-011 | `evident_output.review` valid Go common path  | Tool test                 | No unnecessary advanced-API findings                                       |
| MCP-012 | Review detects redundant `Start`              | Static fixture            | `API-006` finding with safe suggestion and recheck signal                  |
| MCP-013 | Review detects stdout contamination           | Static/transcript fixture | Error finding with stream guidance                                         |
| MCP-014 | Review expected blocked item vs Go error      | Static fixture            | Flags control-flow misuse without false positive on real application error |
| MCP-015 | Review source location                        | Tool test                 | File, line, and column accurate for Go AST findings                        |
| MCP-016 | Review without type information               | Tool test                 | Partial analysis clearly marked; no invented certainty                     |
| MCP-017 | Review with package/type information          | Integration               | Cross-file API use resolved without executing package code                 |
| MCP-018 | Review terminal transcript                    | Virtual terminal fixture  | ANSI corruption, width, and final-state findings detected                  |
| MCP-019 | Review structured document                    | Schema/property           | Domain and schema findings deterministic                                   |
| MCP-020 | `recheck_required=true`                       | Tool test                 | True for required repairs or analysis-invalidating changes                 |
| MCP-021 | `recheck_required=false`                      | Tool test                 | False only after all requested deterministic items complete cleanly        |
| MCP-022 | Iterative repair loop                         | End-to-end agent fixture  | Known-bad sample reaches clean state in bounded cycles                     |
| MCP-023 | `evident_output.preview` profile matrix       | Tool + golden             | All default profiles generated and labeled                                 |
| MCP-024 | Preview narrow/ASCII/no-color                 | Golden                    | Semantic equivalence across degraded profiles                              |
| MCP-025 | Preview debug interleaving                    | Virtual terminal          | Durable logs and live region remain coherent                               |
| MCP-026 | Preview refuses arbitrary Go execution        | Security                  | Requires scene/event input; no code execution path                         |
| MCP-027 | `evident_output.explain`                      | Tool test                 | Rule rationale, examples, remedies, and verification IDs returned          |
| MCP-028 | Rule stability                                | Compatibility             | Rule IDs and meanings obey version policy                                  |
| MCP-029 | Structured output schema                      | Schema test               | `structuredContent` matches advertised schema                              |
| MCP-030 | Text compatibility content                    | Tool test                 | Useful text accompanies structured result where client support varies      |
| MCP-031 | Oversized request                             | Security                  | Rejected within bounded memory                                             |
| MCP-032 | Deadline/cancellation                         | Integration               | Work stops and resources release                                           |
| MCP-033 | Malformed JSON-RPC                            | Conformance/fuzz          | Protocol error; server remains safe                                        |
| MCP-034 | Panic containment                             | Fault injection           | Bounded tool error; server continues where safe                            |
| MCP-035 | Local path outside roots                      | Security                  | Rejected including symlink traversal                                       |
| MCP-036 | Remote path input                             | Schema/security           | Unsupported; remote transport accepts content only                         |
| MCP-037 | Source mutation                               | Static/dynamic            | No v1 tool writes caller files                                             |
| MCP-038 | Network access                                | Sandboxed test            | No network calls by core v1 tools                                          |
| MCP-039 | Shell execution                               | Static/dynamic            | No execution path exists                                                   |
| MCP-040 | Resource URIs                                 | Protocol test             | Embedded versioned resources readable and immutable                        |
| MCP-041 | Protocol-version compatibility                | Matrix                    | Supported versions negotiate or fail explicitly                            |
| MCP-042 | Tool-name rules                               | Schema test               | Names use allowed characters and length                                    |
| MCP-043 | Unknown input fields                          | Tool test                 | Rejected unless documented extension point                                 |
| MCP-044 | Debug flag                                    | Integration               | Server debug goes only to stderr                                           |
| MCP-045 | HTTP disabled by default                      | CLI test                  | No listener in default build                                               |
| MCP-046 | CLI parity                                    | Differential test         | CLI and MCP deterministic operations return equivalent results             |
| MCP-047 | Agent prompt embeds loop                      | Prompt snapshot           | Discover–review–repair–recheck–preview steps explicit                      |
| MCP-048 | Agent skill common API                        | Scenario test             | Agent chooses inferred API over specs/config for ordinary task             |
| MCP-049 | Agent stopping condition                      | Scenario test             | Agent stops only when `recheck_required=false`                             |
| MCP-050 | Token-budget compliance                       | Tool test                 | Guidance truncation explicit and deterministic                             |

### 31.9 Interoperability and API ergonomics

| ID      | Concern                              | Verification        | Pass criteria                                                                      |
| ------- | ------------------------------------ | ------------------- | ---------------------------------------------------------------------------------- |
| API-001 | Minimal item example                 | Compile fixture     | `For`, `Item`, `OK`/`Block`, `Finish` compile without config structs               |
| API-002 | Minimal task example                 | Compile fixture     | `Task`, `Phase`, `Donef` compile and render correctly                              |
| API-003 | Multiple progress example            | Compile fixture     | `Tasks`, child `Task`, `Bytes`, and derived summary compile without scheduler APIs |
| API-004 | Common path clarity                  | API review          | Calls read as output facts rather than renderer mechanics                          |
| API-005 | No public intent enum in quick start | Static docs         | No `IntentReport`/title ceremony                                                   |
| API-006 | Explicit start not required          | Differential        | Direct terminal resolution and phase-driven start remain valid                     |
| API-007 | Advanced API availability            | Compile             | Stable keys, fixed capabilities, manual start, external projection available       |
| API-008 | Common/advanced parity               | Differential        | Same semantic input produces same snapshots                                        |
| API-009 | No runtime `...any` problem overload | API lint            | `Block` and `BlockedBy` statically distinguish shapes                              |
| API-010 | Standard formatting language         | Vet                 | `*f` methods use Go formatting and are vet-compatible                              |
| API-011 | Cobra integration                    | Compile/integration | No base class, command router, or execution takeover                               |
| API-012 | Standard `flag` integration          | Compile             | Ordinary Go command embeds `Output` directly                                       |
| API-013 | `urfave/cli` and Kong examples       | Compile matrix      | No core dependency changes                                                         |
| API-014 | `slog` bridge                        | Unit/integration    | Attributes preserved; live region remains coherent                                 |
| API-015 | Arbitrary logger writer              | Integration         | Partial lines, UTF-8, close, and limits handled safely                             |
| API-016 | Bubble Tea/external renderer         | Compile + snapshot  | Host consumes snapshots without terminal ownership                                 |
| API-017 | Pure projection                      | Unit                | Snapshot renders plain/JSON without active output                                  |
| API-018 | No `os.Exit`                         | Static analysis     | Library never exits host process                                                   |
| API-019 | No global mutable state              | Static + race       | Multiple outputs remain independent                                                |
| API-020 | External subprocess suspend          | PTY                 | Cooperative output works without interception                                      |
| API-021 | Generated docs compile               | CI                  | Every Go example type-checks and passes vet                                        |
| API-022 | Discoverability                      | Usability study     | New user finds `Item`, `Task`, and `Tasks` before advanced specs                   |
| API-023 | Simple output versus `fmt`           | Usability           | One durable line is one method call after output creation                          |
| API-024 | Complex-output advantage             | Comparison fixture  | Safe multi-progress/debug implementation materially smaller than ad hoc code       |
| API-025 | Import-name convention               | Static docs         | Package is lowercase `evo`; examples do not use `EvidentOutput`                    |
| API-026 | Execution boundary                   | Static/API review   | No `RunAll`, `Map`, `Retry`, `Timeout`, worker-pool, or shell execution in core    |
| API-027 | Singular/plural distinction          | Compile/docs        | `Task` cannot contain children; `Tasks` cannot expose leaf lifecycle methods       |
| API-028 | Absolute progress semantics          | API review          | Primary docs use `Progress(done,total)` and `Bytes(done,total)`                    |
| API-029 | Incremental progress semantics       | API review          | Only `Advance(delta)` communicates a delta                                         |
| API-030 | Compatibility promise                | Integration matrix  | Cobra, flag, slog, io.Writer, TUI, CI, pipe, JSON, and MCP paths pass              |

### 31.10 Security and limits

| ID      | Concern                    | Verification    | Pass criteria                                                  |
| ------- | -------------------------- | --------------- | -------------------------------------------------------------- |
| SEC-001 | Terminal injection         | Fuzz + PTY      | Untrusted text cannot control terminal                         |
| SEC-002 | Secret split across chunks | Fuzz            | Redactor detects according to documented capability            |
| SEC-003 | Oversized entity count     | Limit           | Controlled error/degradation; no unbounded allocation          |
| SEC-004 | Oversized render tree      | Limit           | Bounded traversal and error                                    |
| SEC-005 | Integer overflow           | Fuzz            | Rejected before arithmetic wrap                                |
| SEC-006 | Malicious shell argument   | Unit            | Display quoting preserves argv boundaries                      |
| SEC-007 | Destructive action         | Unit + JSON     | Marked explicitly in human and machine representations         |
| SEC-008 | Dependency vulnerability   | CI              | `govulncheck` passes or waiver is documented                   |
| SEC-009 | Dependency license         | CI              | Compatible approved license list                               |
| SEC-010 | Panic from caller renderer | Fault           | Output cleanup and error propagation                           |
| SEC-011 | Bidi spoofing              | Security corpus | Neutralized or visibly escaped                                 |
| SEC-012 | Path disclosure            | Policy test     | Sensitive paths can be redacted consistently                   |
| SEC-013 | Log forging with newline   | Unit            | One caller record cannot forge multiple level-prefixed records |
| SEC-014 | Resource URI traversal     | MCP test        | Embedded resource resolver rejects traversal                   |
| SEC-015 | Untrusted MCP annotations  | Review          | No authorization decision depends on annotations               |

### 31.11 Portability and compatibility

| ID       | Concern                     | Verification       | Pass criteria                                           |
| -------- | --------------------------- | ------------------ | ------------------------------------------------------- |
| PORT-001 | Linux TTY                   | Integration        | Correct interactive and cleanup behavior                |
| PORT-002 | macOS TTY                   | Integration        | Correct interactive and cleanup behavior                |
| PORT-003 | Windows ConPTY              | Integration        | Correct Unicode/color/cleanup or documented degradation |
| PORT-004 | tmux                        | Manual/PTY         | No region corruption                                    |
| PORT-005 | SSH                         | Manual/PTY         | Capability detection remains conservative               |
| PORT-006 | `TERM=dumb`                 | Environment test   | No cursor controls; plain output                        |
| PORT-007 | No locale UTF-8             | Environment test   | ASCII fallback or safe UTF-8 policy                     |
| PORT-008 | Width unavailable           | Unit               | Plain/event fallback                                    |
| PORT-009 | Height unavailable          | Unit               | Bounded inline policy                                   |
| PORT-010 | Go supported-release matrix | CI                 | Builds and tests on policy-defined Go versions          |
| PORT-011 | 32-bit integer environment  | Cross-build/fuzz   | Explicit int64 arithmetic remains safe                  |
| PORT-012 | Big-endian architecture     | Cross-compile      | No encoding assumption                                  |
| PORT-013 | Public API compatibility    | `apidiff`-style CI | No unapproved breaking change in major v1               |
| PORT-014 | Schema compatibility        | Fixture suite      | Old fixtures decode under current reader                |
| PORT-015 | Reproducible build metadata | Release CI         | Version and schema hashes deterministic                 |

---

## 32. Benchmarks and performance targets

Performance targets are guardrails, not marketing claims. They SHALL be measured on documented hardware and Go versions.

### 32.1 Targets

- `Task.Advance` without forced render: p50 under 5 µs and allocation-free after warm-up where feasible;
- item terminal transition: p50 under 20 µs excluding renderer I/O;
- snapshot of 100 entities: under 1 ms target;
- plain final render of 100 entities: under 5 ms target;
- idle output: no periodic wakeups except spinner tick while visible;
- default frame rate: at most 20 fps;
- memory for 100 active entities and 1,000 bounded log records: under a documented low-single-digit MB target;
- MCP pure tool request for an 80x24 preview: under 100 ms target excluding process startup;
- no goroutine growth proportional to update count.

These values SHALL be revised based on evidence before 1.0. Correctness and bounded behavior take precedence over microbenchmarks.

### 32.2 Benchmark scenarios

- one spinner;
- 10 parallel items;
- 1,000 items final-only;
- 100 tasks advancing at 1 kHz aggregate;
- 10,000 debug records with bounded queue;
- Unicode-heavy wrapping;
- narrow terminal;
- JSON Lines streaming;
- MCP validate/render/replay.

Benchmarks SHALL report allocations, not only time.

---

# Part IX — Development process

## 33. Red-green implementation sequence

The prose specification explains intent. The conformance suite defines observable behavior, following the executable-specification model used by the Perl 6/Raku `roast` suite.

Every feature cycle SHALL follow:

```text
RED
  Add the smallest failing unit, conformance, or transcript test.
  Confirm it fails for the intended reason.

GREEN
  Implement the smallest semantic behavior that passes.

REFACTOR
  Improve ownership, naming, and duplication without changing observable behavior.

VERIFY
  Run unit, conformance, race, fuzz, golden, and relevant PTY tests.
```

Canonical command set:

```bash
go test ./...
go test -race ./...
go test ./conformance/...
go test ./... -run Fuzz -fuzztime=30s
```

### Phase 0 — API comprehension and compile-red tests

Begin with examples that intentionally do not compile until the public API exists:

- `evo.For`, `Output.Item`, `Item.Block`, and `Item.BlockedBy`;
- `Output.Task`, `Task.Phase`, `Task.Progress`, `Task.Bytes`, and `Task.Done`;
- `Output.Tasks` and `Tasks.Task`;
- `Output.Changes` and aligned structured effects;
- `Output.Plan` without changed inference;
- `Output.Finish` and idempotent `Close`.

### Phase 1 — Strict semantic core

Implement:

- immutable IDs and declaration order;
- item and task state machines;
- first-terminal-state-wins;
- problem defensive copies;
- progress validation;
- unresolved-entity detection;
- derived task-collection state;
- deterministic conclusion dimensions.

### Phase 2 — Plain and structured projections

Implement plain, JSON, and JSONL from immutable snapshots before ANSI rendering. This proves that semantics are independent of the terminal.

### Phase 3 — Interactive terminal projection

Implement visibility delay, frame coalescing, terminal-width response, task-collection budgeting, and durable final output using the virtual terminal before real PTY integration.

### Phase 4 — Durable messages and logger bridges

Implement debug insertion, `slog`, managed writers, sanitization, redaction, and suspend/resume.

### Phase 5 — Guidance and review engine

Generate task-oriented guidance from canonical rules. Implement deterministic review of Go source, snapshots, transcripts, stream use, and structured output.

### Phase 6 — MCP adapter

Expose discovery, guidance retrieval, review, preview, and explanation through stdio MCP with stdout reserved for protocol traffic.

### Phase 7 — Hardening

Complete race, fuzz, PTY, Windows, Unicode, resource-limit, security, compatibility, and agent-effectiveness gates.

The complete red tests are normative examples in **Appendix H**.

## 34. Definition of done for a feature

A feature is complete only when:

- behavior is described in domain language;
- ownership is unambiguous;
- valid and invalid transitions are tested;
- concurrent use is tested if applicable;
- plain output is defined;
- interactive output is defined if applicable;
- JSON/schema impact is defined;
- MCP impact is explicitly “none” or implemented;
- security and redaction impact is reviewed;
- narrow/ASCII/no-color behavior is tested;
- documentation contains a realistic example;
- benchmark impact is measured for hot paths;
- no new public abstraction duplicates an existing concept.

---

## 35. SOLID and DRY application

### 35.1 Single Responsibility

- `Output` owns one command's presentation lifecycle and invariants, not ANSI layout or application work.
- `Item` owns one named condition or result that remains meaningful in final output, not task progress.
- `Task` owns its lifecycle and measurement, not a free-floating progress object.
- `Projection` renders state, not business operations.
- `TerminalDriver` owns terminal control, not semantic decisions.
- `CapabilityDetector` observes environment, not style policy.
- `Theme` maps semantic roles, not text content.
- Agent review tools analyze and explain; they do not execute or mutate caller applications.

### 35.2 Open/Closed

New projections can consume stable snapshots and events. New themes can map existing semantic roles. New review rules register against stable analysis inputs. Domain invariants are not bypassed by extensions.

### 35.3 Liskov Substitution

Any exported renderer or projection contract defines ordering, error, lifecycle, and concurrency semantics precisely enough that implementations substitute without changing output behavior.

### 35.4 Interface Segregation

Do not expose one large backend interface. Detection, rendering, clocks, redaction, snapshots, and event sinks remain separate boundaries.

### 35.5 Dependency Inversion

Infrastructure depends on domain snapshots and events. The root package does not import terminal, JSON-RPC, MCP, command-parser, logger implementation, or TUI packages.

### 35.6 DRY without premature unification

Share:

- strict transition validation;
- common/advanced facade implementation;
- text sanitization;
- width computation;
- semantic roles;
- schemas and rule metadata;
- guidance source content.

Do not:

- merge items and tasks into a generic public `Activity` merely because fields overlap;
- merge user lines and debug logs;
- create one ambiguous `Update(...any)` method;
- duplicate inference logic in renderers, MCP, and CLI tools;
- maintain separate prose copies for website, MCP resources, and skills.

### 35.7 Domain-oriented naming

Public calls should read as statements a person might make:

```go
branches := out.Item("branches")
branches.BlockedBy(problems...)

dependencies := out.Task("dependencies")
dependencies.Phase("resolving packages")
dependencies.Donef("installed %d packages", count)
```

Names such as `AddProgressNode`, `RenderItem`, `ActivityController`, or `SetLifecycleState` are rejected for the common API because they expose implementation mechanics rather than user-domain meaning.

# Part X — Open-source architecture and governance

## 36. License decision

### 36.1 Recommendation

Use **Apache License 2.0** for the initial release.

Rationale:

- permissive commercial and open-source use;
- explicit contributor patent grant and termination terms;
- familiar to enterprise consumers;
- clear redistribution and notice requirements;
- aligned with the official MCP Go SDK's contemporary licensing direction.

MIT remains a reasonable alternative if the maintainers prioritize the shortest possible license text and do not consider an explicit patent grant material. Do not dual-license at launch without a concrete downstream requirement; dual licensing adds contributor and compliance complexity without automatically improving adoption.

This is an engineering recommendation, not legal advice.

### 36.2 Repository files

The repository SHALL contain:

- `LICENSE`;
- `NOTICE` if required by dependencies or branding decisions;
- SPDX identifiers in source headers only if the project chooses header-level notices;
- `CONTRIBUTING.md`;
- `CODE_OF_CONDUCT.md`;
- `SECURITY.md`;
- `GOVERNANCE.md`;
- `SUPPORT.md`;
- dependency license report in releases or CI artifacts.

### 36.3 Contributions

The project SHOULD use a Developer Certificate of Origin sign-off rather than a custom CLA unless legal counsel or a sponsoring organization requires a CLA.

Contribution requirements:

- tests for behavior changes;
- API review for exported identifiers;
- schema review for machine contracts;
- security review for control sequences, file/network access, and MCP changes;
- changelog entry for user-visible changes;
- benchmark evidence for hot-path regressions.

---

## 37. Versioning and compatibility

### 37.1 Go module versioning

- semantic versioning;
- no breaking exported API changes within major version 1 after 1.0;
- pre-1.0 APIs may change but require migration notes;
- experimental packages use an explicit `experimental` path or build tag;
- deprecations remain for at least one minor release and include replacements.

### 37.2 Supported Go versions

At each release, support the Go versions defined by the project's documented support policy, preferably the current stable release and the immediately preceding stable release. Avoid hardcoding a stale version in architecture documents.

### 37.3 Terminal compatibility

Terminal behavior is best-effort enhancement. Plain and structured modes form the stable cross-environment contract.

### 37.4 MCP compatibility

The MCP server SHALL advertise only capabilities it implements and maintain a tested protocol-version matrix. Unsupported versions SHALL fail clearly during initialization.

---

## 38. Repository architecture

```text
/
├── agent/
│   ├── catalog/
│   ├── review/
│   ├── rules/
│   └── preview/
├── agents/
│   └── evident-output-engineer.md
├── cmd/
│   ├── evident-output/
│   └── evident-output-mcp/
├── examples/
│   ├── repository-item/
│   ├── dependency-task/
│   ├── download/
│   ├── data-command/
│   ├── external-renderer/
│   └── debug-live/
├── integrations/
│   ├── claude-code/
│   ├── codex/
│   ├── gemini/
│   ├── grok/
│   └── opencode/
├── internal/
│   ├── ansi/
│   ├── capability/
│   ├── journal/
│   ├── layout/
│   ├── scheduler/
│   ├── sanitize/
│   ├── termdriver/
│   └── width/
├── jsonout/
├── logbridge/
├── mcpbridge/
├── plain/
├── render/
├── schema/
├── skills/
│   └── evident-output/
│       └── SKILL.md
├── terminal/
├── testdata/
│   ├── agent/
│   ├── corpus/
│   ├── events/
│   ├── golden/
│   ├── review/
│   └── schemas/
├── testkit/
├── action.go
├── item.go
├── conclusion.go
├── event.go
├── evo.go
├── problem.go
├── output.go
└── task.go
```

Internal packages remain implementation details. The root package SHALL not become a re-export facade for every subpackage. Generated agent assets SHALL be checked for drift against the canonical guidance/rule source.

## 39. Documentation deliverables

Before 1.0:

1. README with a common-path quick start under 30 lines;
2. “Why Evident Output” compatibility and scope statement;
3. common API guide emphasizing inference;
4. advanced API guide for explicit lifecycle, keys, renderers, and projections;
5. state diagrams and conclusion precedence;
6. terminal compatibility guide;
7. stdout/stderr and exit-code guide;
8. logging and `slog` integration guide;
9. embedding guide for Cobra, `flag`, `urfave/cli`, Kong, and host TUIs;
10. JSON and JSON Lines schemas;
11. MCP and agent-workflow setup for major clients;
12. guide catalog with task-oriented use cases;
13. migration guide from `fmt.Printf`, ad hoc spinners, and progress libraries;
14. testkit and preview-matrix guide;
15. security model;
16. performance and limits guide;
17. contributing, rule-authoring, and API-review guide;
18. generated reference for every stable review rule and diagnostic.

Every Go example SHALL compile in CI. Every `*f` example SHALL pass printf/vet analysis. Every guide section used by MCP SHALL be generated from or checksum-matched to the canonical source.

# Part XI — Adversarial architecture review

## 40. Review method

The review assumes long-term public API stability, hostile concurrency schedules, broken writers, narrow terminals, untrusted text, partial failures, AI-generated misuse, and future alternate-language implementations. Objections below are integrated design constraints, not commentary appended after the fact.

## 41. Counterpoint: “This is a formatting library pretending to be a domain system.”

### Objection

`Output`, `Item`, `Task`, `Tasks`, `Problem`, `Changes`, and `Plan` may appear excessive for terminal formatting.

### Integrated resolution

The public vocabulary is limited to distinctions visible to an ordinary CLI user. No public widget tree, region, frame, event bus, or layout DSL is required. Internal DDD exists only to keep lifecycle, concurrency, and projection semantics coherent.

## 42. Counterpoint: “Output will become a god object.”

### Objection

The aggregate appears to own entities, logs, renderers, streams, events, conclusion, and cleanup.

### Integrated resolution

`Output` is a facade and aggregate root, not the implementation of every responsibility. It delegates to state storage, transition validation, journal, scheduler, projection pipeline, and terminal driver. The public facade owns orchestration because one command needs one coherent presentation lifecycle.

## 43. Counterpoint: “Item is too generic.”

### Objection

Users may turn every implementation event into an item and recreate noisy logs.

### Integrated resolution

An item is normatively defined as a named condition or result that remains meaningful in final output. Temporary actions belong in task phases or debug messages. MCP review and documentation flag item names such as `opening file`, `calling API`, or `waiting` when used as implementation narration.

## 44. Counterpoint: “Block versus BlockedBy is needless duplication.”

### Objection

Two methods represent one blocked state.

### Integrated resolution

Go cannot overload `Block(string, ...ProblemOption)` and `Block(...Problem)` safely. `Block` constructs one simple anonymous problem. `BlockedBy` accepts structured evidence. The grammar is clear and avoids `...any`. Symmetric `WarnedBy` and `FailedBy` are available where structured evidence is needed.

## 45. Counterpoint: “OK is inconsistent with Warn, Block, and Fail.”

### Objection

`OK` is not a verb like the other methods.

### Integrated resolution

Alternatives are worse: `Pass` implies testing, `Succeed` implies an operation, `Ready` is not universal, and `Clear` is ambiguous. `OK` expresses that the named condition is satisfactory and is the single canonical positive term.

## 46. Counterpoint: “Fail and Block will be confused.”

### Objection

Both look negative and can produce red output.

### Integrated resolution

The semantic distinction is normative and affects conclusion state:

- `Block`: evaluation succeeded and found a condition preventing intended progress.
- `Fail`: evaluation or operation itself could not be completed.

Review rules and tests require examples for both.

## 47. Counterpoint: “A task with subtasks would be simpler than Tasks.”

### Objection

`out.Tasks("dependencies").Task("react")` adds a noun.

### Integrated resolution

A parent task with child tasks creates conflicting progress, phases, and terminal states. `Task` is one operation. `Tasks` is a collection whose state is derived from children. This preserves SRP and makes invalid combinations unrepresentable.

## 48. Counterpoint: “Tasks is awkward English and visually close to Task.”

### Objection

Singular/plural names can be missed in code review.

### Integrated resolution

The distinction reads naturally at call sites and avoids an additional abstraction such as `Group` or `Batch`, whose semantics are less clear. Documentation always introduces them together. Static analysis can flag attempts to call collection-only or task-only methods.

## 49. Counterpoint: “Absolute progress callbacks are verbose.”

### Objection

`Bytes(written, total)` repeats the total on every update.

### Integrated resolution

Absolute updates are self-contained, replayable, and safe across callback conventions. `Add` is ambiguous because callbacks may report deltas or cumulative totals. `Advance(delta)` remains for explicitly incremental loops. Advanced measurement handles may be considered only after evidence.

## 50. Counterpoint: “Progress may legitimately move backward.”

### Objection

Retries and restarts can reset transfer progress.

### Integrated resolution

Backward movement is rejected by default because it usually signals misuse. Explicit `Restart` or `ResetProgress` records intent and produces an observable event. Silent regression is prohibited.

## 51. Counterpoint: “Collection summaries can lie.”

### Objection

A caller may write `installed 4 packages` while one child failed.

### Integrated resolution

`Tasks.Summary` is success-oriented and displayed only when compatible with derived child state. Failed or warning collections use deterministic aggregate summaries unless an advanced explicit annotation is supplied.

## 52. Counterpoint: “The library will become a scheduler.”

### Objection

A task collection tempts APIs such as `RunAll`, `Map`, `Retry`, `Parallel`, and `Timeout`.

### Integrated resolution

These methods are explicitly prohibited from the core package. The application owns all execution. Evo only records and presents state.

## 53. Counterpoint: “Concurrent declaration cannot be deterministic.”

### Objection

If goroutines create tasks concurrently, lock acquisition decides display order.

### Integrated resolution

Concurrency safety is guaranteed; semantic ordering from concurrent declarations is not. Callers declare in intended order before starting work or use explicit advanced order keys. Snapshots state the resulting order plainly.

## 54. Counterpoint: “Duplicate names break identity.”

### Objection

Two tasks may legitimately have the same display name.

### Integrated resolution

Every entity has an immutable internal ID. Names are presentation only. JSON, events, MCP, and snapshots reference IDs.

## 55. Counterpoint: “Unresolved work at Finish should be ignored.”

### Objection

Commands may exit early and leave pending tasks.

### Integrated resolution

Ignoring unresolved work hides bugs. Evo renders unresolved entities as incomplete/unknown, records typed misuse, and returns an error. Callers explicitly cancel or skip intentional non-completion.

## 56. Counterpoint: “Done without a summary is unclear.”

### Objection

A task ending after `Phase("verifying")` could retain active wording.

### Integrated resolution

`Done` clears active phase text. It retains useful final measurement but does not transform active tense into completion text. `Donef` supplies a durable summary.

## 57. Counterpoint: “Detail(error) is convenient.”

### Objection

Forcing callers to separate detail and cause adds ceremony.

### Integrated resolution

Errors can expose secrets, internal paths, server responses, or unstable implementation text. `Detail(string)` is intentionally user-visible. `Cause(error)` is diagnostic and redacted by policy. Security and output quality outweigh one shorter call.

## 58. Counterpoint: “First terminal state wins hides later truth.”

### Objection

A task may be marked done and later discover failure.

### Integrated resolution

Allowing last-write-wins makes concurrency nondeterministic and rewrites durable history. A later terminal mutation records misuse. Correct application code must resolve only after final state is known.

## 59. Counterpoint: “Annotation after resolution violates immutability.”

### Objection

Chaining `BlockedBy(...).Because(...).NextCommand(...)` mutates a resolved item.

### Integrated resolution

State and annotation are separate. Terminal state is immutable after resolution; annotations may be added until `Finish` begins. This preserves fluent clarity without reopening lifecycle.

## 60. Counterpoint: “Conclusion precedence loses information.”

### Objection

A command may fail after making partial changes.

### Integrated resolution

The headline uses deterministic precedence, while `Conclusion` retains orthogonal `Changed`, `Partial`, and `Cancelled` dimensions. Structured output never collapses these facts into one enum.

## 61. Counterpoint: “Output.Close hides writer failures.”

### Objection

A deferred return value is easily ignored.

### Integrated resolution

`Finish` is the strict boundary and returns errors. `Close` is idempotent cleanup and best-effort finish. Documentation shows both `defer out.Close()` and `return out.Finish()` for strict programs.

## 62. Counterpoint: “Automatic conclusion is magic.”

### Objection

Users may not understand why a headline became blocked or warning.

### Integrated resolution

Inference rules are deterministic, documented, included in snapshots, and reviewable through MCP. Explicit overrides exist. Evo infers only presentation semantics, not domain-specific explanations.

## 63. Counterpoint: “The library cannot safely mix stdout and stderr.”

### Objection

Separate file descriptors can interleave unpredictably.

### Integrated resolution

Evo documents stream ownership, uses managed writers, supports shared-stream and split-stream profiles, and provides projection-only/static degradation. It never claims to coordinate arbitrary external writes.

## 64. Counterpoint: “Append-above debug logging will flicker.”

### Objection

Frequent durable logs force clears and redraws.

### Integrated resolution

The scheduler coalesces frames, batches queued messages, bounds redraw rate, and recommends file or pane logging for very noisy debug modes. Correctness takes precedence over animation smoothness.

## 65. Counterpoint: “Many progress bars will overwhelm the terminal.”

### Objection

Hundreds of children cannot fit and rapid callbacks can saturate rendering.

### Integrated resolution

Semantic updates are cheap and independent from redraw frequency. The scheduler coalesces latest state at a bounded frame rate. The viewport selects failed, warning, active, pending, then successful rows; omissions are explicit. Final structured output retains every child.

## 65A. Counterpoint: “MCP guidance will impose one aesthetic.”

### Objection

Agent tooling may reject valid CLI designs that differ visually.

### Integrated resolution

Review rules distinguish normative semantics from advisory style. Stream purity, state consistency, terminal safety, accessibility, and machine contracts may be errors. Spacing, glyphs, and compactness are profile-dependent suggestions. Rule findings cite the invariant they protect.

## 65B. Counterpoint: “The spec is too broad for v1.”

### Objection

Items, tasks, collections, changes, plans, terminal rendering, JSON, testkit, and MCP may exceed a viable initial release.

### Integrated resolution

Milestones stage delivery. The v1 core is semantics, plain/JSON projections, one inline renderer, task collections, debug-safe output, testkit, and local review tooling. Hosted services, arbitrary renderer plugins, deep task hierarchies, and workflow helpers remain out of scope.

## 66. Milestones

### 66.1 v0.1 — Semantic core and simple projections

Deliver:

- `New`, `For`, `Item`, `Task`, `Line`, and conclusion inference;
- strict transition engine with recorded common facade;
- plain final projection;
- JSON final projection;
- state-machine, inference, and API comprehension tests;
- schemas;
- minimal README.

### 66.2 v0.2 — Interactive ledger

Deliver:

- terminal capability detection;
- delayed visibility and spinner threshold;
- parallel items;
- phased and measured tasks;
- failed-row expansion;
- resize and narrow layout;
- cursor restoration.

### 66.3 v0.3 — Lines, logs, and external integration

Deliver:

- durable `Line`/`Linef`;
- append-above logs;
- `slog` handler;
- line writer;
- suspend/resume;
- snapshot/external-renderer integration;
- redaction.

### 66.4 v0.4 — Testkit and portability

Deliver:

- virtual terminal;
- PTY tests;
- Windows coverage;
- fuzz corpus;
- race stress suite;
- compatibility fixtures;
- direct pure projection APIs.

### 66.5 v0.5 — Guidance, review, and preview CLI

Deliver:

- task-oriented guidance catalog;
- stable rule registry;
- Go source review;
- scene/transcript review;
- profile matrix preview;
- `evident-output` CLI parity;
- agent scenario fixtures.

### 66.6 v0.6 — MCP and agent integrations

Deliver:

- `evident-output-mcp` stdio server;
- `list_guides`, `get_guidance`, `review`, `preview`, and `explain` tools;
- resources and prompts;
- skills and specialized subagent;
- conformance and security tests;
- client configuration examples.

### 66.7 v0.9 — Compatibility and security candidate

Deliver:

- external pilot feedback;
- API freeze candidate;
- schema and rule-ID compatibility review;
- protocol-version matrix;
- dependency and terminal security review;
- documentation freeze candidate.

### 66.8 v1.0 — Stable public contract

Requires:

- all critical traceability rows passing;
- no unresolved high-severity security findings;
- at least two external pilot CLIs;
- public API review demonstrating common-path simplicity;
- schema, guidance, and rule compatibility review;
- benchmark baseline;
- documentation complete;
- license and governance finalized;
- no known terminal-corruption issue in supported environments.

## 67. Release gates

A release candidate SHALL fail if:

- race detector finds a race;
- fuzz corpus has a reproducible crash;
- plain or JSON output contains ANSI;
- stdio MCP stdout contains non-protocol bytes;
- cursor remains hidden after any handled exit path;
- final state differs between human and machine renderers;
- a critical durable event can be silently lost;
- a known terminal escape injection is possible;
- a breaking API/schema change lacks a major-version plan;
- dependency vulnerability review is incomplete;
- examples do not compile;
- generated guidance, skills, MCP resources, or rule documentation drift from the canonical source;
- a required agent scenario cannot reach `recheck_required=false` because of deterministic tool defects;
- the common-path examples require advanced configuration without an approved API change.

---

## 68. Success metrics

Technical:

- zero known terminal-corruption defects at 1.0;
- no data races in stress suite;
- deterministic structured fixtures;
- bounded memory under documented limits;
- API compatibility CI clean;
- MCP conformance clean for supported versions.

Adoption:

- downstream CLI can replace ad hoc progress/log code with fewer than 50 lines of integration;
- common item command can be implemented with no custom ANSI;
- AI tool can discover guidance, review Go source, iterate to a clean result, and preview output without executing the caller project;
- issue reports include replayable event fixtures rather than screenshots alone.

Usability:

- ordinary success fits one short screen;
- failures identify subject, condition, consequence, and next action;
- no-color and plain modes remain fully understandable;
- debug mode does not corrupt the live ledger.

---

# Appendices

## Appendix A — Canonical API examples

### A.1 Blocked repository inspection

```go
func inspectRepository(ctx context.Context, repo string) error {
    out := evo.For(repo)
    defer out.Close()

    workingTree := out.Item("working tree")
    branches := out.Item("branches")
    remotes := out.Item("remotes")

    var group errgroup.Group

    group.Go(func() error {
        if err := inspectWorkingTree(); err != nil {
            workingTree.Block(
                "unstashed changes",
                evo.Detail("commit or stash the changes before continuing"),
                evo.Cause(err),
            )
            return nil
        }
        workingTree.OK()
        return nil
    })

    group.Go(func() error {
        problems := compareBranches()
        if len(problems) > 0 {
            branches.BlockedBy(problems...).
                Because("Resolve the branch problems before retiring this repository.").
                NextCommand(
                    "repo-retire",
                    "salvage",
                    "--dry-run",
                    repo,
                )
            return nil
        }
        branches.OK()
        return nil
    })

    group.Go(func() error {
        if err := probeRemotes(); err != nil {
            remotes.Warn(
                "origin was not reachable",
                evo.Detail("remote state is unverified"),
                evo.Cause(err),
            )
            return nil
        }
        remotes.OK()
        return nil
    })

    if err := group.Wait(); err != nil {
        return err
    }

    return out.Finish()
}
```

### A.2 One phased task

```go
func installDependencies(out *evo.Output, installed int64) {
    dependencies := out.Task("dependencies")
    dependencies.Phase("reading lockfile")
    dependencies.Phase("resolving packages")
    dependencies.Phase("downloading packages")
    dependencies.Donef("installed %d packages", installed)
}
```

### A.3 Multiple progress rows

```go
func installPackages(out *evo.Output, packages []Package) error {
    dependencies := out.Tasks("dependencies")

    var group errgroup.Group
    var installed atomic.Int64

    for _, pkg := range packages {
        pkg := pkg
        task := dependencies.Task(pkg.Name)

        group.Go(func() error {
            task.Phase("downloading")
            if err := download(pkg, func(written int64) {
                task.Bytes(written, pkg.Size)
            }); err != nil {
                task.Fail("download failed", evo.Cause(err))
                return nil
            }

            task.Phase("verifying")
            if err := verify(pkg); err != nil {
                task.Fail("verification failed", evo.Cause(err))
                return nil
            }

            installed.Add(1)
            task.Done()
            return nil
        })
    }

    if err := group.Wait(); err != nil {
        return err
    }

    dependencies.Summaryf("installed %d packages", installed.Load())
    return nil
}
```

### A.4 Changes

```go
out.Changes("dependencies").
    Added(14, "packages").
    Updated(4, "packages").
    Reused(63, "cached packages").
    Wrote("app.lock")
```

### A.5 Plan

```go
out.Plan("delete account acme").
    Delete(14, "projects").
    Revoke(7, "API keys").
    Remove(23, "users").
    Retain("audit records for 90 days")
```

### A.6 Data command with stdout purity

```go
out, err := evo.NewWithConfig(evo.Config{
    Projection: evo.DataProjection(),
    Output: evo.OutputConfig{
        Primary:    os.Stdout,
        Diagnostic: os.Stderr,
    },
})
if err != nil {
    return err
}
defer out.Close()

scan := out.Task("repository scan")
scan.Phase("walking files")

if err := encodeResults(os.Stdout); err != nil {
    out.Fail("could not produce repository data", evo.Cause(err))
    return err
}

scan.Donef("scanned %d files", count)
return out.Finish()
```

### A.7 `slog` during live rendering

```go
out := evo.New()
defer out.Close()

logger := slog.New(out.SlogHandler(slog.LevelDebug))
task := out.Task("index")
task.Phase("reading documents")
logger.Debug("batch loaded", "documents", 200)
task.Donef("indexed %d documents", 200)
```

### A.8 Strict downstream test

```go
func TestBranchOutput(t *testing.T) {
    out := testkit.NewOutput(t, testkit.Strict())
    branches := out.Item("branches")
    branches.Block("local-only branch")

    testkit.RequireConclusion(t, out, evo.StateBlocked)
    testkit.RequireClean(t, out)
}
```

### A.9 Host TUI projection-only integration

```go
out, err := evo.NewWithConfig(evo.Config{
    Projection: evo.ExternalProjection(),
})
if err != nil {
    return err
}
defer out.Close()

for snapshot := range out.Snapshots(ctx) {
    host.Update(snapshot)
}
```

## Appendix B — Event taxonomy

```text
output.started
output.degraded
output.suspended
output.resumed
output.finishing
output.finished
output.closed
output.line_emitted
output.failed
output.cancelled

item.declared
item.started
item.ok
item.warned
item.blocked
item.failed
item.unknown
item.skipped
item.annotated

tasks.declared
tasks.summary_set

task.declared
task.started
task.phase_changed
task.progress_changed
task.restarted
task.done
task.warned
task.failed
task.cancelled
task.skipped

problem.attached
action.attached
changes.declared
change.recorded
plan.declared
plan.recorded
log.emitted
renderer.degraded
terminal.resized
terminal.suspended
terminal.resumed
```

Every durable event includes:

- schema version;
- output ID;
- sequence;
- timestamp from the injected clock;
- entity ID when applicable;
- event type;
- redacted structured payload;
- inference source when a transition was implicit.

## Appendix C — Validation diagnostic and rule taxonomy

Stable rule namespaces:

```text
API-*      common/advanced API clarity and misuse
DOM-*      domain ownership and conclusion inference
STATE-*    lifecycle and transition correctness
LAYOUT-*   width, wrapping, hierarchy, and vertical budget
TEXT-*     sanitization, Unicode, and control characters
COLOR-*    semantic styling and no-color behavior
STREAM-*   stdout/stderr and projection contracts
TERM-*     cursor, resize, suspend, and transient region
PROGRESS-* measurement and progress semantics
LOG-*      durable lines and diagnostic logging
SCHEMA-*   JSON/JSONL compatibility
MCP-*      protocol and agent workflow
SEC-*      security and privacy
COMPAT-*   framework, platform, and version compatibility
```

Examples:

```text
API-006    redundant explicit Start before Phase
API-014    direct logger bypasses output during live UI
DOM-011    expected blocked item returned as application error
LAYOUT-004 detail column starved at requested width
STREAM-003 progress contaminates structured stdout
TERM-012   live region exceeds terminal height
MCP-021    review stopped while recheck_required is true
SEC-007    destructive action lacks confirmation metadata
```

Every stable rule contains:

- ID and version introduced;
- category and severity class;
- protected invariant;
- detection certainty;
- accepted alternatives;
- good/bad examples;
- safe fix guidance;
- related guide IDs;
- related verification IDs;
- deprecation/replacement metadata.

## Appendix D — Declarative presentation model

```json
{
  "$schema": "https://evident-output.dev/schema/v1/presentation.json",
  "subject": "bpp-csharp",
  "items": [
    {
      "id": "working-tree",
      "name": "working tree",
      "state": "ok"
    },
    {
      "id": "branches",
      "name": "branches",
      "state": "blocked",
      "problems": [
        {
          "subject": "feat/sdk-full-consolidation",
          "summary": "local-only",
          "count": 1,
          "unit": "commit"
        }
      ]
    }
  ],
  "task_collections": [
    {
      "id": "dependencies",
      "name": "dependencies",
      "summary": "installed 4 packages",
      "tasks": [
        {
          "id": "react",
          "name": "react",
          "state": "done",
          "progress": {
            "kind": "bytes",
            "completed": 8100000,
            "total": 8100000
          }
        }
      ]
    }
  ],
  "changes": [],
  "plans": [],
  "actions": [
    {
      "command": {
        "executable": "repo-retire",
        "args": ["salvage", "--dry-run", "bpp-csharp"]
      }
    }
  ]
}
```

The declarative model is semantic. It contains no caller-supplied spaces, ANSI, cursor operations, spinner frames, or column widths.

## Appendix E — Review checklist for new abstractions

Before adding a public noun:

1. Would a normal CLI user use this noun when describing the command?
2. Does it have lifecycle or invariants distinct from an existing noun?
3. Who owns it?
4. Can it exist independently?
5. Is it mutable or a value?
6. What are its valid transitions?
7. How does it appear in plain and structured output?
8. Does it create a new schema/version obligation?
9. Can composition solve the need without a new type?
10. Are two independent downstream use cases known?

Before adding a verb:

1. Is the receiver the logical owner of the action?
2. Is the verb unambiguous in ordinary language?
3. Is the operation idempotent?
4. Is it safe concurrently?
5. What error or terminal state results from invalid use?
6. Does it emit a durable event?
7. Can it be retried?
8. Does it require context or cancellation?

---

## Appendix F — Source and standards basis

This architecture is informed by:

- the Model Context Protocol specification, including lifecycle negotiation, tools, structured tool output, stdio and Streamable HTTP transport requirements, and security guidance;
- Svelte's official AI tooling pattern of task-oriented documentation discovery, focused retrieval, deterministic source review/autofix feedback, iterative rechecking, previews, skills, and specialized agent context;
- the official MCP Go SDK and its conformance direction;
- Go's official race detector, fuzzing, and vulnerability-management tooling;
- JSON Schema 2020-12 usage as adopted by MCP tool schemas;
- SPDX license identifiers and the Apache License 2.0 patent and redistribution terms.

The implementation SHALL pin exact protocol/schema versions in code and tests rather than depend on an unversioned “latest” document.

---

## Appendix G — Final architectural decisions

| Concern               | Chosen design                                   | Rejected design                               |
| --------------------- | ----------------------------------------------- | --------------------------------------------- |
| Product               | Presentation library                            | CLI framework or workflow engine              |
| Package               | `evo`                                           | `EvidentOutput` identifier                    |
| Root ownership        | `Output` aggregate facade                       | Global renderer singleton                     |
| Reported condition    | `Item`                                          | Check, Verification, Status, Result, LineItem |
| One operation         | `Task`                                          | Generic activity/widget                       |
| Multiple operations   | `Tasks` collection                              | Parent task with subtasks                     |
| Simple block          | `Item.Block`                                    | Ambiguous overloaded `Block(...any)`          |
| Structured block      | `Item.BlockedBy`                                | `BlockAll`                                    |
| Positive item state   | `OK`                                            | Pass, Succeed, Ready, Clear                   |
| Progress              | Absolute `Progress`/`Bytes`; explicit `Advance` | Ambiguous `Total` + `Add` + `Set` common path |
| Application execution | Host-owned                                      | Library-created goroutines/scheduler          |
| Problem detail        | `Detail(string)` and `Cause(error)`             | Raw error as user-visible detail              |
| Terminal resolution   | First terminal state wins                       | Last-write-wins                               |
| Unresolved work       | Explicit incomplete/error                       | Silent success/skip                           |
| Effects               | Structured `Changes`                            | Caller-spaced text                            |
| Future effects        | Structured `Plan`                               | Reusing changes with flags                    |
| Final result          | Multidimensional `Conclusion`                   | One enum discarding partial/changed facts     |
| Machine output        | Separate stable schemas                         | Scraping human output                         |
| Agent support         | Guidance → review → repair → recheck → preview  | Documentation dump                            |

## Appendix H — Normative red tests

These tests are intentionally written before implementation. During the red phase, some fail to compile. The missing API is part of the specification.

```go
package evo_test

import (
    "bytes"
    "errors"
    "fmt"
    "io"
    "reflect"
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/example/evident-output"
    "github.com/example/evident-output/testkit"
)
```

### H.1 A phase starts a pending task

```go
func TestTask_PhaseStartsPendingTask(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Task("dependencies")
    dependencies.Phase("reading lockfile")

    got := dependencies.Snapshot()

    if got.State != evo.Running {
        t.Fatalf("state = %q, want %q", got.State, evo.Running)
    }
    if got.Phase != "reading lockfile" {
        t.Fatalf("phase = %q, want reading lockfile", got.Phase)
    }
    if got.Progress.Kind != evo.Indeterminate {
        t.Fatalf("kind = %q, want %q", got.Progress.Kind, evo.Indeterminate)
    }
}
```

### H.2 Instant completion does not flash a spinner

```go
func TestTask_InstantCompletionDoesNotFlashSpinner(t *testing.T) {
    screen := testkit.NewScreen(
        testkit.Interactive(),
        testkit.Width(80),
        testkit.NoColor(),
    )
    clock := testkit.NewClock()

    out := evo.New(
        evo.Terminal(screen),
        evo.Clock(clock),
        evo.VisibilityDelay(150*time.Millisecond),
    )
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Task("dependencies")
    dependencies.Donef("installed %d packages", 18)

    if err := out.Finish(); err != nil {
        t.Fatal(err)
    }

    if got := screen.LiveFrameCount(); got != 0 {
        t.Fatalf("live frames = %d, want 0", got)
    }
    if strings.ContainsAny(screen.FinalText(), "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
        t.Fatalf("final output contains spinner:\n%s", screen.FinalText())
    }
}
```

### H.3 Absolute progress starts a determinate task

```go
func TestTask_ProgressStartsDeterminateTask(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Task("dependencies")
    dependencies.Progress(12, 18)

    got := dependencies.Snapshot()
    if got.State != evo.Running {
        t.Fatalf("state = %q, want %q", got.State, evo.Running)
    }
    if got.Progress.Completed != 12 || got.Progress.Total != 18 {
        t.Fatalf("progress = %d/%d, want 12/18", got.Progress.Completed, got.Progress.Total)
    }
}
```

### H.4 Invalid progress preserves the last valid state

```go
func TestTask_InvalidProgressIsRecordedWithoutCorruption(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    task := out.Task("download")
    task.Progress(4, 10)
    task.Progress(11, 10)

    got := task.Snapshot()
    if got.Progress.Completed != 4 || got.Progress.Total != 10 {
        t.Fatalf("last valid progress was not preserved: %#v", got.Progress)
    }
    if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
        t.Fatalf("error = %v, want ErrInvalidProgress", out.Err())
    }
}
```

### H.5 Backward progress requires explicit restart

```go
func TestTask_BackwardProgressIsRejected(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    task := out.Task("download")
    task.Bytes(500, 1_000)
    task.Bytes(400, 1_000)

    if !errors.Is(out.Err(), evo.ErrProgressRegression) {
        t.Fatalf("error = %v, want ErrProgressRegression", out.Err())
    }
}
```

### H.6 Block creates one simple problem

```go
func TestItem_BlockCreatesSingleProblem(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    workingTree := out.Item("working tree")
    workingTree.Block(
        "unstashed changes",
        evo.Detail("index contains modified files"),
    )

    got := workingTree.Snapshot()
    if got.State != evo.Blocked {
        t.Fatalf("state = %q, want %q", got.State, evo.Blocked)
    }
    if len(got.Problems) != 1 {
        t.Fatalf("problems = %d, want 1", len(got.Problems))
    }
    if got.Problems[0].Summary != "unstashed changes" {
        t.Fatalf("summary = %q", got.Problems[0].Summary)
    }
}
```

### H.7 BlockedBy preserves structured evidence

```go
func TestItem_BlockedByPreservesProblems(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    branches := out.Item("branches")
    problems := []evo.Problem{
        {Subject: "feat/sdk-full-consolidation", Summary: "local-only", Count: 1},
        {Subject: "fix/login-flow", Summary: "ahead of origin", Count: 2},
    }

    branches.BlockedBy(problems...)

    if got := branches.Snapshot().Problems; !reflect.DeepEqual(got, problems) {
        t.Fatalf("problems differ\ngot: %#v\nwant: %#v", got, problems)
    }
}
```

### H.8 BlockedBy requires evidence

```go
func TestItem_BlockedByWithoutProblemsRecordsMisuse(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    branches := out.Item("branches")
    branches.BlockedBy()

    if !errors.Is(out.Err(), evo.ErrNoProblems) {
        t.Fatalf("error = %v, want ErrNoProblems", out.Err())
    }
    if got := branches.Snapshot().State; got != evo.Pending {
        t.Fatalf("state = %q, want %q", got, evo.Pending)
    }
}
```

### H.9 First terminal state wins

```go
func TestItem_FirstTerminalStateWins(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    item := out.Item("working tree")
    item.OK()
    item.Block("unstashed changes")

    if got := item.Snapshot().State; got != evo.OK {
        t.Fatalf("state = %q, want %q", got, evo.OK)
    }
    if !errors.Is(out.Err(), evo.ErrAlreadyResolved) {
        t.Fatalf("error = %v, want ErrAlreadyResolved", out.Err())
    }
}
```

### H.10 Concurrent item resolution preserves declaration order

```go
func TestItem_ConcurrentResolutionPreservesDeclarationOrder(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    workingTree := out.Item("working tree")
    branches := out.Item("branches")
    remotes := out.Item("remotes")

    remoteResolved := make(chan struct{})
    branchResolved := make(chan struct{})

    var group sync.WaitGroup
    group.Go(func() { remotes.OK(); close(remoteResolved) })
    group.Go(func() { <-remoteResolved; branches.Warn("unreachable"); close(branchResolved) })
    group.Go(func() { <-branchResolved; workingTree.OK() })
    group.Wait()

    if err := out.Finish(); err != nil {
        t.Fatal(err)
    }
    conclusion := out.Conclusion()
    got := []string{
        conclusion.Items[0].Name,
        conclusion.Items[1].Name,
        conclusion.Items[2].Name,
    }
    want := []string{"working tree", "branches", "remotes"}
    if !reflect.DeepEqual(got, want) {
        t.Fatalf("order = %q, want %q", got, want)
    }
}
```

### H.11 A Tasks collection derives state from children

```go
func TestTasks_StateIsDerivedFromChildren(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Tasks("dependencies")
    react := dependencies.Task("react")
    sharp := dependencies.Task("sharp")

    react.Done()
    sharp.Fail("checksum mismatch")

    got := dependencies.Snapshot()
    if got.State != evo.Failed {
        t.Fatalf("state = %q, want %q", got.State, evo.Failed)
    }
}
```

### H.12 A collection success summary cannot hide failure

```go
func TestTasks_SuccessSummaryIsSuppressedOnFailure(t *testing.T) {
    var output bytes.Buffer
    out := evo.For("dependencies", evo.To(&output), evo.Plain(), evo.NoColor())
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Tasks("dependencies")
    dependencies.Summary("installed 2 packages")
    dependencies.Task("react").Done()
    dependencies.Task("sharp").Fail("checksum mismatch")

    _ = out.Finish()

    if strings.Contains(output.String(), "installed 2 packages") {
        t.Fatalf("success summary was rendered for failed collection:\n%s", output.String())
    }
}
```

### H.13 Unresolved tasks fail Finish

```go
func TestOutput_FinishReportsUnresolvedTask(t *testing.T) {
    out := evo.New(evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Tasks("dependencies")
    dependencies.Task("react").Done()
    dependencies.Task("esbuild")

    err := out.Finish()
    if !errors.Is(err, evo.ErrUnresolvedTask) {
        t.Fatalf("error = %v, want ErrUnresolvedTask", err)
    }
}
```

### H.14 Changes align semantic columns

```go
func TestChanges_AlignVerbQuantityAndObject(t *testing.T) {
    var output bytes.Buffer
    out := evo.For(
        "dependencies",
        evo.To(&output),
        evo.Plain(),
        evo.NoColor(),
        evo.Width(80),
    )
    t.Cleanup(func() { _ = out.Close() })

    out.Changes("dependencies").
        Added(14, "packages").
        Updated(4, "packages").
        Reused(63, "cached packages").
        Wrote("app.lock")

    if err := out.Finish(); err != nil {
        t.Fatal(err)
    }

    want := `[changed]  dependencies
  added    14 packages
  updated   4 packages
  reused   63 cached packages
  wrote       app.lock
`
    if got := output.String(); got != want {
        t.Fatalf("output mismatch\ngot:\n%swant:\n%s", got, want)
    }
}
```

### H.15 Narrow changes use compact layout

```go
func TestChanges_NarrowOutputUsesCompactLayout(t *testing.T) {
    var output bytes.Buffer
    out := evo.For(
        "dependencies",
        evo.To(&output),
        evo.Plain(),
        evo.NoColor(),
        evo.Width(30),
    )
    t.Cleanup(func() { _ = out.Close() })

    out.Changes("dependencies").
        Added(14, "packages").
        Updated(4, "packages").
        Wrote("app.lock")

    if err := out.Finish(); err != nil {
        t.Fatal(err)
    }

    want := `[changed]  dependencies
  added 14 packages
  updated 4 packages
  wrote app.lock
`
    if got := output.String(); got != want {
        t.Fatalf("output mismatch\ngot:\n%swant:\n%s", got, want)
    }
}
```

### H.16 A plan does not imply changes

```go
func TestPlan_DoesNotInferChangedConclusion(t *testing.T) {
    out := evo.For("account acme", evo.To(io.Discard))
    t.Cleanup(func() { _ = out.Close() })

    out.Plan("delete account acme").
        Delete(14, "projects").
        Revoke(7, "API keys")

    if err := out.Finish(); err != nil {
        t.Fatal(err)
    }

    got := out.Conclusion()
    if got.State != evo.Planned || got.Changed {
        t.Fatalf("conclusion = %#v, want planned and unchanged", got)
    }
}
```

### H.17 Debug messages do not corrupt the live region

```go
func TestDebug_MessageIsInsertedAboveLiveRegion(t *testing.T) {
    screen := testkit.NewScreen(
        testkit.Interactive(),
        testkit.Width(80),
        testkit.NoColor(),
    )

    out := evo.New(
        evo.Terminal(screen),
        evo.DebugLevel(evo.Debug),
    )
    t.Cleanup(func() { _ = out.Close() })

    task := out.Task("dependencies")
    task.Phase("resolving packages")
    out.Debug("package index loaded", evo.Int("packages", 18))
    task.Donef("installed %d packages", 18)
    _ = out.Finish()

    want := []testkit.Operation{
        testkit.DrawLive("⠋  dependencies  resolving packages"),
        testkit.ClearLive(),
        testkit.WriteDurable("[DEBUG] package index loaded  packages=18"),
        testkit.DrawLive("⠋  dependencies  resolving packages"),
        testkit.ClearLive(),
        testkit.WriteFinal("✓  dependencies  installed 18 packages"),
    }

    if diff := testkit.DiffOperations(want, screen.Operations()); diff != "" {
        t.Fatalf("terminal operations differ (-want +got):\n%s", diff)
    }
}
```

### H.18 Non-interactive output contains no terminal controls

```go
func TestOutput_NonInteractiveContainsNoTerminalControls(t *testing.T) {
    var output bytes.Buffer
    out := evo.New(
        evo.To(&output),
        evo.NonInteractive(),
        evo.NoColor(),
    )
    t.Cleanup(func() { _ = out.Close() })

    task := out.Task("dependencies")
    task.Phase("reading lockfile")
    task.Phase("resolving packages")
    task.Donef("installed %d packages", 18)
    _ = out.Finish()

    got := output.String()
    for _, forbidden := range []string{"\x1b[", "\r", "⠋", "⠙", "⠹"} {
        if strings.Contains(got, forbidden) {
            t.Fatalf("non-interactive output contains %q:\n%s", forbidden, got)
        }
    }
}
```

### H.19 Human and JSON projections preserve meaning

```go
func TestOutput_HumanAndJSONPreserveMeaning(t *testing.T) {
    scenario := testkit.Scenario(
        evo.Subject("bpp-csharp"),
        func(out *evo.Output) {
            out.Item("working tree").OK()
            out.Item("branches").BlockedBy(evo.Problem{
                Subject: "feat/sdk-full-consolidation",
                Summary: "local-only",
                Count:   1,
            })
            out.Item("remotes").OK()
        },
    )

    human := scenario.Render(evo.PlainFormat)
    machine := scenario.Render(evo.JSONFormat)

    if human.Conclusion.State != machine.Conclusion.State {
        t.Fatalf("human = %q, machine = %q", human.Conclusion.State, machine.Conclusion.State)
    }
    if human.ExitCode != machine.ExitCode {
        t.Fatalf("human exit = %d, machine exit = %d", human.ExitCode, machine.ExitCode)
    }
}
```

### H.20 Multiple progress rows preserve declaration order

```go
func TestTasks_MultipleProgressRowsPreserveDeclarationOrder(t *testing.T) {
    screen := testkit.NewScreen(
        testkit.Interactive(),
        testkit.Width(80),
        testkit.NoColor(),
    )

    out := evo.New(evo.Terminal(screen))
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Tasks("dependencies")
    react := dependencies.Task("react")
    esbuild := dependencies.Task("esbuild")
    sharp := dependencies.Task("sharp")

    // Update and resolve in a different order than declaration.
    sharp.Phase("verifying")
    esbuild.Bytes(12_400_000, 18_000_000)
    react.Bytes(8_100_000, 8_100_000)
    react.Done()

    got := screen.LatestLiveText()
    want := `⠋  dependencies  1/3 complete
   ✓  react      8.1 MB
   ⠋  esbuild   12.4/18.0 MB
   ⠋  sharp      verifying`

    if got != want {
        t.Fatalf("live output mismatch\ngot:\n%s\nwant:\n%s", got, want)
    }
}
```

### H.21 Screen budgeting favors failures and active tasks

```go
func TestTasks_ScreenBudgetSelectsImportantRowsAndReportsOmission(t *testing.T) {
    screen := testkit.NewScreen(
        testkit.Interactive(),
        testkit.Width(80),
        testkit.Height(8),
        testkit.NoColor(),
    )

    out := evo.New(evo.Terminal(screen))
    t.Cleanup(func() { _ = out.Close() })

    dependencies := out.Tasks("dependencies")
    for n := 0; n < 120; n++ {
        task := dependencies.Task(fmt.Sprintf("package-%03d", n))
        switch n {
        case 7:
            task.Fail("checksum mismatch")
        case 12, 18:
            task.Phase("downloading")
        case 20:
            task.Warn("using cached fallback")
        default:
            task.Done()
        }
    }

    got := screen.LatestLiveText()
    for _, required := range []string{
        "package-007",
        "checksum mismatch",
        "package-020",
        "package-012",
        "not shown",
    } {
        if !strings.Contains(got, required) {
            t.Fatalf("live output omitted %q:\n%s", required, got)
        }
    }
}
```

### H.22 High-frequency progress is coalesced but final state is exact

```go
func TestTask_HighFrequencyProgressIsCoalesced(t *testing.T) {
    screen := testkit.NewScreen(
        testkit.Interactive(),
        testkit.Width(80),
        testkit.NoColor(),
    )
    clock := testkit.NewClock()

    out := evo.New(
        evo.Terminal(screen),
        evo.Clock(clock),
        evo.MaxFrameRate(30),
    )
    t.Cleanup(func() { _ = out.Close() })

    download := out.Task("download")
    for completed := int64(0); completed <= 10_000; completed++ {
        download.Progress(completed, 10_000)
    }
    download.Done()

    if err := out.Finish(); err != nil {
        t.Fatal(err)
    }

    if got := download.Snapshot().Progress.Completed; got != 10_000 {
        t.Fatalf("completed = %d, want 10000", got)
    }
    if frames := screen.LiveFrameCount(); frames >= 10_000 {
        t.Fatalf("frames = %d, progress updates were not coalesced", frames)
    }
}
```

### H.23 Shared conformance runner

```go
func TestConformanceV1(t *testing.T) {
    testkit.RunConformanceDirectory(
        t,
        "../../spec/v1",
        testkit.GoImplementation(),
    )
}
```

The same behaviors SHALL be represented as language-neutral JSON scenarios so future renderers and implementations can prove conformance without reproducing the Go API.

# End of specification
