# Layout

One page that tells a reader where a thing lives. Tests enforce the root package map below.

## Principles

- Public package: one obvious home per canonical concept family.
- Canonical vocabulary: semantic concepts only.
- Helpers: live beside the concept.
- Internal packages: one owner per invariant.
- Tests: named by concept or contract behavior, never by implementation phase or ticket.

## Root package map (`evo`)

`vocabulary_guard_test.go` enforces this table. `testdata/api_vocabulary.txt` is the concept source: each exported symbol's vocabulary concept decides its file.

| File          | Concept family                                  | Representative symbols                                                 |
| ------------- | ----------------------------------------------- | ---------------------------------------------------------------------- |
| `run.go`      | Run                                             | `Init`, `Run`, `Main`, `Config`, `Output`, `Result`, `Delay`           |
| `task.go`     | Task, Define, Wait, Summary, Doing, Progress    | `Task`, `TaskHandle`, `Progress`                                       |
| `group.go`    | Group, Sequence, After                          | `Group`, `Sequence`, `GroupHandle`, `SequenceHandle`                   |
| `outcome.go`  | Skipped, Blocked, Failed, Cancelled, Conclusion | `Conclusion`, `EntityState`, `Reason`, `Resolution`                    |
| `problem.go`  | Problem                                         | `Problem`, `Failure`, `Severity`, `Detail`, `Code`                     |
| `action.go`   | Action                                          | `Action`, `Command`, `Label`                                           |
| `fact.go`     | Fact                                            | `Fact`, `FactRecord`                                                   |
| `verify.go`   | Verify, Evidence                                | `EvidencePhase`, `TaskEvidence`                                        |
| `basis.go`    | Fingerprint                                     | `Fingerprint`, `FSPath`, `Value`                                       |
| `file.go`     | File                                            | `File`, `FileSpec`, `FileFS`                                           |
| `patch.go`    | Patch, Files                                    | `Patch`, `Files`, `FileSet`                                            |
| `exec.go`     | Exec                                            | `Exec`, `ExecSpec`, `ExecResult`                                       |
| `capture.go`  | Capture                                         | `Capture`, `CaptureOption`                                             |
| `effect.go`   | Effect                                          | `Effect`, `EffectSpec`, `EffectRecord`                                 |
| `resource.go` | Resource                                        | `Resource`, `FSResource`, `LogicalResource`                            |
| `snapshot.go` | Snapshot                                        | `Snapshot`, `Event`, `RenderPlain`, `PlainOptions`                     |
| `format.go`   | Machine output                                  | `Format`, `Projection`, `ParseFormat`, `WriteJSON`, `PublishedRelease` |
| `human.go`    | Human output                                    | `Print`, `Confirm`, `Printer`, `GlyphProfile`, `Visibility`            |
| `debug.go`    | Debug journal                                   | `DebugConfig`, `LogLevel`, `SlogHandler`                               |
| `misuse.go`   | Misuse                                          | `ErrClosed`, `ErrDuplicateKey`, `ErrWaitDeadlock`                      |
| `doc.go`      | package doc only                                | no exported declarations                                               |

Three override rules take precedence over the concept column:

- Receiver rule: every method of `GroupHandle` and `SequenceHandle` lives in `group.go`.
- `Output.Context` lives in `run.go`, because it is the run-scoped context.
- Package-level `Next` and `NextCommand` live in `problem.go`, because they return `ProblemOption`.

Until series R finishes, the catch-all files `api.go`, `facade.go`, `glyph.go`, `jsonout.go`, `output.go`, `printer.go`, `release.go`, `state.go` and `types.go` still exist; the guard tolerates them and each slice that empties one deletes it.

## Internal packages

- `internal/core`: domain types (Snapshot, Problem, Action, Event, state, conclusion) and the closed Effect verb set
- `internal/engine`: presentation engine (Output, tasks, confirm, evidence, print, run); `capture/`, `ledger/`, `schedule/`, `lifecycle/` as they land
- `internal/render`: shared row model for human, JSON, and live projection; `render/live` and `render/plain` as they land
- `internal/text`: glyphs, sanitize, width, conjugate, name truncation
- `internal/wire`, `internal/wireschema`: wire documents and JSON schema validation
- `internal/apisurface`: public API golden/required/retired contract walk and declaration-file lookup
- `internal/agent/*`: MCP tools (adopt, review, catalog, preview, sections, harness, rules incl. the retired-symbol table); `vocabulary` for classification and layout
- `internal/fingerprint`, `internal/patch`, `internal/resource`, `internal/manifest`, `internal/modpin`, `internal/architecture`, `internal/docexamples`: one owner per invariant, named by the package

## Tests

- Root `*_test.go` files are named by concept (`effect_test.go`) or contract family (`concurrency_test.go` for CON-xxx).
- `conformance/` holds release gates, goldens and scenarios.
- Internal packages use `*_internal_test.go`.
- Requirement IDs stay in test names; `conformance/TRACEABILITY.md` maps them.

## Frozen files

`.evor/baseline.sha256` lists the frozen files (conformance goldens, two scenarios, `testdata/api_golden.txt`, `testdata/api_golden_released.txt`). Check them with `bash .evor/check-frozen.sh`. Frozen files keep their names even when they do not follow the test naming principle.

## Other trees

- `terminal`: ANSI terminal driver
- `testkit`: test helpers (clock, screen, output assertions)
- `cmd/evident-output`: CLI (review, adopt, explain, contract)
- `cmd/evident-output-mcp`: MCP server
- `examples`: runnable demos (repo-status, doctor, data-command, ...)
- `conformance`: release gates, goldens, TRACEABILITY
- `schema`: wire JSON schemas (event.v1, output.v1)
- `docs`: guides, architecture, API inventory, ADRs
- `integrations`: editor/agent install notes
- `skills`: agent skill docs
- `tools/agents`: engineer prompt (not MCP; that is internal/agent)
- `tools/scripts`: release, traceability, usage-audit, pin sync
- `tools/gensections`: embed docs into the MCP server
