# Developing evident-output

Commands, examples, and the lower-level surfaces (CLI, machine output,
production ANSI driver, interactive testkit) for anyone changing the library
itself, not just consuming it.

## Develop

```bash
mise run setup    # go mod download
mise run test     # unit + roast
mise run test-race
mise run conformance
mise run traceability   # all §31 IDs present
mise run ci             # lint + test + scan + conformance + traceability
```

Trunk is configured **daemonless** (`--monitor=false`). Prefer `mise` over raw tools.

## Conformance (roast)

`conformance/` is the executable specification (Raku/`roast` model):

- `TRACEABILITY.md` — every §31 ID dispositioned (pass / waived with reason + owner for external/manual only; 0 untested)
- Go Appendix H tests (`reference_scenarios_test.go`) at the module root
- `goldens/` — spec-golden byte-for-byte render tests
- `gates/` — release-gate regression rounds

Contract: [`CONTRACT.md`](../../CONTRACT.md), the 2.0 surface every conformance test checks.

## Examples (adoption ladder)

```text
examples/print/              Run.Text: free human text through an io.Writer
examples/verbose/            Task.Fact; Config.Verbose (--verbose) shows Facts
examples/repo-status/        Task.Problem, evo.Refuse, evo.Next; blockers exit 1
examples/install-pipeline/   Sequence, Task.Doing, Task.Progress; a failed Task exits 2
examples/migrate/            Config.DryRun, evo.Effect, evo.File, After, evo.Fail
examples/doctor/             Task.Problem findings, --strict evo.Refuse, Output JSON
examples/data-command/       Run.ResultWriter on stdout, rows and bands on stderr
examples/named-tasks/        root Tasks named for their work
examples/live-progress/      ordinary multi-progress: Task.Doing and Task.Progress
examples/debug-history/      Config.Debug and Run.SlogHandler: the debug journal
examples/debug-pane/         a Sequence stopped by evo.Refuse; later Tasks NotStarted
examples/terminal-driver/    advanced: a custom Facades.Terminal logging frames
examples/generate-pipeline/  evo.Exec with Inputs and Outputs; the second run is Satisfied
examples/launch-agent-file/  evo.File and Task.Verify; the second run is Satisfied
examples/quiet-writer/       Task.Writer: a silent child keeps the live row changing
examples/many-tasks/         one Group of N Tasks: live-region scale (--tasks N)
```

```bash
mise run examples          # non-interactive batch
go run ./examples/print/
go run ./examples/verbose/ --verbose
```

Every example's documented flags have a pinned exit code
(`examples/flag_behaviour_test.go`): 0 Succeeded, Satisfied or Excluded, 1
Refused, 2 Failed.

| Example            | Pattern                                                                                  | Exit codes                             |
| ------------------ | ---------------------------------------------------------------------------------------- | -------------------------------------- |
| `repo-status`      | Parallel **Tasks**; a blocker is `evo.Refuse` with a Next step, a finding is a `Problem` | 0 with `--clean`, 1 otherwise          |
| `install-pipeline` | A **Sequence** of Tasks reporting `Task.Progress`; a failing test returns the error      | 0, 2 with `--fail-tests`               |
| `migrate`          | Plans by default (`Config.DryRun` is `!--apply`); the schema runs After the backup       | 0, 2 with `--apply --fail`             |
| `doctor`           | Findings are Problems; `--strict` returns `evo.Refuse`; `--json` is `Output` `JSON`      | 0, 1 with `--strict`                   |
| `data-command`     | JSON payload from `Run.ResultWriter` to **stdout**, human report to **stderr**           | 0, 2 with `--fail-link` (empty stdout) |
| `live-progress`    | **Live multi-progress**: bars and indeterminate phases (ANSI on stderr)                  | 0                                      |
| `debug-history`    | **Debug journal**: `slog` records from `Run.SlogHandler`, kept by `Config.Debug`         | 0                                      |
| `debug-pane`       | A **Sequence** that `evo.Refuse` stops: the later Task is not started                    | 0, 1 with `--fail`                     |

```bash
mise run examples                          # all, back-to-back with headers
go run ./examples/repo-status/ --name my-app
go run ./examples/install-pipeline/
go run ./examples/migrate/                 # dry-run plan
go run ./examples/migrate/ --apply
go run ./examples/doctor/ --json | jq .outcome
go run ./examples/data-command/ 2>/dev/null | jq .
go run ./examples/live-progress/           # in-place ANSI live region (real TTY)
go run ./examples/terminal-driver/ --frames  # numbered frames you can scroll
go run ./examples/debug-history/           # debug journal beside the rows
go run ./examples/debug-pane/              # three Tasks, all Succeeded
go run ./examples/debug-pane/ --fail       # compare refuses; exit 1
```

## CLI

```bash
go run ./mcp/cmd/evident-output review path/to/file.go   # JSON findings (exit 1 if recheck_required)
go run ./mcp/cmd/evident-output preview --item=status --state=ok
go run ./mcp/cmd/evident-output explain STREAM-003
go run ./mcp/cmd/evident-output version
```

## Machine output

```go
var buf bytes.Buffer
_ = evo.Execute(ctx, evo.Config{Name: "tool", Output: evo.Plain, Facades: evo.Facades{Stderr: &buf}}, run)
plain := buf.String() // rows and bands; Facades.Stdout receives only Run.ResultWriter
```

Schemas: `../schema/run.v3.json`, `../schema/event.v2.json`.

## Interactive (testkit / virtual terminal)

```go
screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
clock := testkit.NewClock()
_ = evo.Execute(ctx, evo.Config{
    Facades: evo.Facades{Terminal: screen, Clock: clock},
}, run)
// Doing and Progress draw a live region; a Task that resolves before the threshold does not flash.
```
