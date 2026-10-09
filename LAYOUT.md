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
mise.toml
.gitignore
.golangci.yml
.prettierrc.json
.prettierignore
.github/
.trunk/
.grok/
evo.go
run.go
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
mcp/
conformance/
examples/
schema/
testkit/
tools/
docs/migration/
docs/decisions/
docs/history/
```

## Root package `evo`

One file per contract section; the file holds that section's exported
surface and nothing else.

| File         | Contract | Owns                                                                            |
| ------------ | -------- | ------------------------------------------------------------------------------- |
| `evo.go`     | thesis   | package doc only                                                                |
| `run.go`     | §1       | `Main`, `Run`, `RunFunc`, `Config`, `State`, `Run` type, `Result`, `Conclusion` |
| `facades.go` | §1       | `Facades`, `Clock`, `Terminal`, `Redactor`, `Process`, `FS`                     |
| `task.go`    | §2       | `Task`, `Group`, `Sequence`, `Container`, `DefineOption`, `CleanStop`           |
| `compute.go` | §2       | `Compute`, `Computed`                                                           |
| `file.go`    | §3       | `File`, `FileSpec`                                                              |
| `exec.go`    | §3       | `Exec`, `ExecSpec`, `ExecResult`                                                |
| `patch.go`   | §3       | `Patch`                                                                         |
| `effect.go`  | §3       | `Effect`, `EffectSpec`, `Verb`, `Create`, `Update`, `Delete`, `PartialEffect`   |
| `report.go`  | §2       | `Doing`, `Progress`, `Writer`, `Fact`, `Summary` methods                        |
| `problem.go` | §4       | `Problem` method, `ProblemOption`, `Detail`, `Code`, `On`, `Next`, `Action`     |
| `ending.go`  | §4       | `Fail`, `Refuse`, `Exclude`, `Outcome` and its constants                        |
| `confirm.go` | §5       | `Confirm`, `Suspend`, `ConfirmOption`, `Destructive`, `NonInteractive`          |
| `output.go`  | §6       | `Output` and its constants, environment                                         |
| `errors.go`  | §7       | the five sentinels                                                              |

## Internal packages

Four packages are the four promises, in the order a run executes them.
`record` is the only package with mutable run state: `graph`, `freshness`
and `change` append to it; `project` reads it; nothing else touches it. The
three facades are the only packages that import `os`, `os/exec`, `syscall`
or a terminal library.

| Package              | Decides | Owns                                                                                                             |
| -------------------- | ------- | ---------------------------------------------------------------------------------------------------------------- |
| `internal/graph`     | when    | nodes, edges, eligibility, bounded scheduler, cancellation, Build, Computed                                      |
| `internal/freshness` | whether | Inputs, Values, manifest, satisfied-or-stale, Verify                                                             |
| `internal/change`    | what    | File, Exec, Patch, Effect reconciliation; atomic publish; claims                                                 |
| `internal/record`    | truth   | the event journal, the run document, outcomes, Conclusion                                                        |
| `internal/project`   | how     | shared row model; `live/` TTY frame, heartbeat, tail, fold; `plain/` durable text; `wire/` evo.run and evo.event |
| `internal/misuse`    |         | misuse codes, remedy text, recording                                                                             |
| `internal/terminal`  | facade  | ANSI driver, size, char-device                                                                                   |
| `internal/process`   | facade  | spawn, capture, redaction                                                                                        |
| `internal/fs`        | facade  | filesystem, locks, canonical paths                                                                               |

## Other trees

| Tree              | Holds                                                                                                                                  |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `mcp/`            | separate Go module: the MCP server, the CLI, review rules, docs corpus, host integrations, agent skills; imports only the root package |
| `conformance/`    | spec registry keyed to `CONTRACT.md` headings, ratchet, goldens, scenarios                                                             |
| `examples/`       | one directory per contract section, compile-tested                                                                                     |
| `schema/`         | `run.v3.json`, `event.v2.json`                                                                                                         |
| `testkit/`        | clock, screen, fake fs and process for consumers' tests                                                                                |
| `tools/`          | gate tooling: spec-score, spec-guard, traceability, bisectability                                                                      |
| `docs/migration/` | one file per breaking release                                                                                                          |
| `docs/decisions/` | dated rulings; never normative                                                                                                         |
| `docs/history/`   | earlier specs and philosophy; read-only                                                                                                |

## Guards

| Test                       | Fails on                                                                                                                               |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `contract_test.go`         | an exported name not in `CONTRACT.md`, or a contract name not exported                                                                 |
| `layout_test.go`           | a tracked file outside the layout, or a layout path missing                                                                            |
| `import_direction_test.go` | `project` importing `change`; a non-producer writing `record`; a non-facade importing `os`, `os/exec`, `syscall`; root importing `mcp` |
| `examples_test.go`         | an example that fails to compile, or a contract section with no example                                                                |
| `spec_ratchet`             | a requirement that passed once failing now                                                                                             |
