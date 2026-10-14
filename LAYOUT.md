# Layout

Every section of `CONTRACT.md` owns one root file, one internal package and
one guard. `layout_test.go` fails when a tracked file lives outside this
list or a listed path is missing. A bare path is one file. A path ending in
`/` is a directory that must exist and may hold anything beneath it.
`*_test.go` and `testdata/` are allowed beside any Go file.

```layout
CONTRACT.md
LAYOUT.md
README.md
CHANGELOG.md
LICENSE
CONTRIBUTING.md
CODE_OF_CONDUCT.md
GOVERNANCE.md
SECURITY.md
SUPPORT.md
AGENTS.md
go.mod
go.sum
go.work
mise.toml
.gitignore
.golangci.yml
.markdownlint.json
.prettierrc.json
.prettierignore
.github/
.trunk/
.grok/
evo.go
run.go
config.go
result.go
host.go
facades.go
task.go
compute.go
file.go
exec.go
patch.go
effect.go
report.go
problem.go
ending.go
confirm.go
output.go
errors.go
internal/graph/
internal/freshness/
internal/change/
internal/record/
internal/project/
internal/misuse/
internal/terminal/
internal/process/
internal/fs/
internal/clock/
mcp/
conformance/
examples/
schema/
testkit/
tools/
docs/migration/
docs/decisions/
```

## Root package `evo`

One file per contract section; the file holds that section's exported
surface plus the composition root, and nothing else. The composition root is
the `Run` aggregate (handles to graph, record, change, freshness, misuse and
project), its one constructor `openRun` and the signal seam in `run.go`;
`Config` resolved once into the private `config` in `config.go`; the env
seam in `output.go`; the `graph.Host`, `freshness.Narration` and
`change.Engine` adapter in `host.go`; Finish sequencing in `ending.go`; and
facade wiring in `facades.go`. Root writes no record truth and imports no
system package, except `os` for the `os.Signal` type `CleanStop` names.

| File         | Contract | Owns                                                                          |
| ------------ | -------- | ----------------------------------------------------------------------------- |
| `evo.go`     | thesis   | package doc only                                                              |
| `run.go`     | §1       | `Main`, `Execute`, `RunFunc`, the one constructor `openRun`, the signal seam  |
| `config.go`  | §1       | `Config`, `State`, and their resolution into the private `config`             |
| `result.go`  | §1       | `Result`, `Conclusion`                                                        |
| `host.go`    | §1       | the adapter graph, freshness and change call the `Run` through                |
| `facades.go` | §1       | `Facades`, `Clock`, `Terminal`, `Redactor`, `Process`, `FS`                   |
| `task.go`    | §2       | `Task`, `Group`, `Sequence`, `Container`, `DefineOption`, `CleanStop`         |
| `compute.go` | §2       | `Compute`, `Computed`                                                         |
| `file.go`    | §3       | `File`, `FileSpec`                                                            |
| `exec.go`    | §3       | `Exec`, `ExecSpec`, `ExecResult`                                              |
| `patch.go`   | §3       | `Patch`                                                                       |
| `effect.go`  | §3       | `Effect`, `EffectSpec`, `Verb`, `Create`, `Update`, `Delete`, `PartialEffect` |
| `report.go`  | §2       | `Doing`, `Progress`, `Writer`, `Fact`, `Summary` methods                      |
| `problem.go` | §4       | `Problem` method, `ProblemOption`, `Detail`, `Code`, `On`, `Next`, `Action`   |
| `ending.go`  | §4       | `Fail`, `Refuse`, `Exclude`, `Outcome` and its constants                      |
| `confirm.go` | §5       | `Confirm`, `Suspend`, `ConfirmOption`, `Destructive`, `NonInteractive`        |
| `output.go`  | §6       | `Output` and its constants, environment                                       |
| `errors.go`  | §7       | the five sentinels                                                            |

## Internal packages

Four packages are the four promises, in the order a run executes them.
`record` is the only package with mutable run state: `graph`, `freshness`
and `change` append to it; `project` reads it; nothing else touches it. The
four facades are the only packages that import `os`, `os/exec`, `syscall`
or a terminal library.

| Package              | Decides | Owns                                                                                                                                                                                   |
| -------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/graph`     | when    | nodes, edges, eligibility, bounded scheduler, cancellation, Build, Computed; per-Task callback bookkeeping; the run root node, which carries run-scoped annotations and the Conclusion |
| `internal/freshness` | whether | Inputs, Values, manifest, satisfied-or-stale, Verify                                                                                                                                   |
| `internal/change`    | what    | File, Exec, Patch, Effect reconciliation; atomic publish; claims                                                                                                                       |
| `internal/record`    | truth   | the event journal, the run document, outcomes, Conclusion                                                                                                                              |
| `internal/project`   | how     | shared row model and record follower; `live/` TTY frame, heartbeat, tail, fold; `plain/` durable text; `wire/` evo.run and evo.event; `present/` the one presenter the root holds      |
| `internal/misuse`    |         | misuse codes, remedy text, recording                                                                                                                                                   |
| `internal/terminal`  | facade  | ANSI driver, size, char-device                                                                                                                                                         |
| `internal/process`   | facade  | spawn, capture, redaction                                                                                                                                                              |
| `internal/fs`        | facade  | filesystem, locks, canonical paths                                                                                                                                                     |
| `internal/clock`     | facade  | time source, timers, sleep                                                                                                                                                             |

## Other trees

| Tree              | Holds                                                                                                                                                                                                       |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `mcp/`            | separate Go module: the MCP server, the CLI, review rules, docs corpus, host integrations, agent skills, API surface walker, go.mod pin tooling, compile-tested doc examples; imports only the root package |
| `conformance/`    | spec registry keyed to `CONTRACT.md` headings, ratchet, goldens, scenarios, architecture tests                                                                                                              |
| `examples/`       | 15 runnable demo programs, one directory each (`examples/<name>/main.go`), plus `examples/internal/` shared demo helpers and `examples/examples_test.go`, which builds and runs every program               |
| `schema/`         | `run.v3.json`, `event.v2.json`                                                                                                                                                                              |
| `testkit/`        | clock, screen, fake fs and process for consumers' tests; scale and wire-schema helpers                                                                                                                      |
| `tools/`          | gate tooling: spec-score, spec-guard, traceability, bisectability, red-proof, gitenv                                                                                                                        |
| `docs/migration/` | one file per breaking release                                                                                                                                                                               |
| `docs/decisions/` | dated rulings; never normative                                                                                                                                                                              |

## Guards

| Test                        | Fails on                                                                                                                                                                                                                                        |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `contract_test.go`          | an exported name not in `CONTRACT.md`, or a contract name not exported                                                                                                                                                                          |
| `layout_test.go`            | a tracked file outside the layout, or a layout path missing; root over 4,000 non-test code lines or holding a `folded from` seam                                                                                                                |
| `import_direction_test.go`  | `project` importing `change`; a non-producer writing `record`; a non-misuse package owning the first-misuse store; a non-facade importing `os`, `os/exec`, `syscall`; root importing `mcp`. The record-write and system-import rules cover root |
| `examples/examples_test.go` | an example program that fails to build or run                                                                                                                                                                                                   |
| `spec_ratchet`              | a requirement that passed once failing now                                                                                                                                                                                                      |

`import_direction_test.go` also fails on `change` importing anything but the standard library, `record`, `graph`, `freshness` and the facades; on `misuse` importing anything but the standard library, `record` and the facades; on `graph` importing anything but `record`, `misuse` and the facades; on any package outside `change` holding a claim; and on any import of the packages `change` absorbed (`internal/patch`, `internal/publish`, `internal/resource`).
