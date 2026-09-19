# Evident Output

Go presentation library for CLI **state, progress, evidence, changes, plans, and
conclusions**. Application code owns execution; package `evo` owns presentation —
so the same call sites render correctly whether stdout is a real terminal or a
log file, and the exit code always matches what the screen just said.

## Install

```bash
go get github.com/zachbornheimer/evident-output@v1.0.0
```

Requires **Go 1.25+**. License: **Apache-2.0**.

## Quickstart

Beginner path: **Task + Define**, then Group/Sequence, File, Basis, Patch.
Facts/problems, Effects/dry-run, and Exec come next. After is an advanced
DAG edge. `Println` / `Task.Done` still exist for compatibility; they are
not how ordinary work is submitted.

```go
import (
    "context"
    "os"

    evo "github.com/zachbornheimer/evident-output"
)

func main() {
    evo.Init(evo.Config{Title: "bpp-csharp"}) // first statement — arms first paint before any I/O
    os.Exit(evo.Main(run)) // exits the process itself; evo.Run(ctx, run) if you need the Result without exiting
}

func run(ctx context.Context) error {
    write := evo.Task("write config")
    write.Define(func(ctx context.Context) error {
        return evo.File(ctx, evo.FileSpec{
            Path:     "config.json",
            Contents: []byte(`{"ok":true}`),
            Basis:    []evo.Fingerprint{evo.FSPath("config.in")},
        })
    })

    installs := evo.Group("install")
    for _, pkg := range packages {
        pkg := pkg
        installs.Task("install "+pkg).Define(func(ctx context.Context) error {
            return install(pkg)
        })
    }
    return nil
}
```

Define already resolves from the callback — do not call `Done` after it.
Patch derives FileSpecs from a unified diff; pass `result.Files` through to
`evo.File` (copying only Path/Contents drops Basis).

On a real terminal, `install` draws an in-place, colored, animated progress
line while it runs — no extra code. Piped (`prog > log.txt`, CI, an agent
harness), the same call sites fall back to plain, durable lines:

```text
✓ write config
◐ install  0/2
◐ install  1/2  install a
◐ install  2/2  install b
✓ install

[changed] write config  wrote config.json

[changed]  bpp-csharp
```

Exit code `0` — see the table below. Try the live, colored version:
`go run ./examples/repo-status/`.

## Conclusion band → exit code

The trailing `[state]` band and the process exit code always agree — never read
one without checking the other. `· partial` and `· warned` are modifiers on
the state, not a state of their own.

| Band                           | Exit code | Meaning                                                                                                                                                  |
| ------------------------------ | --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `[changed]`                    | `0`       | A mutation verb (`Delete`/`Create`/…) recorded outside `DryRun`                                                                                          |
| `[planned]`                    | `0`       | A mutation verb recorded under `Config.DryRun` (would, not did)                                                                                          |
| `[ready]`                      | `0`       | Every task resolved `Done`; no mutation verb recorded                                                                                                    |
| `[blocked]`                    | `1`       | At least one `Block`, and nothing `Fail`ed                                                                                                               |
| `[failed]`                     | `2`       | At least one `Fail`, or a caller-supplied misuse                                                                                                         |
| `[cancelled]`                  | `130`     | `Cancel` or an interrupt ended the run early                                                                                                             |
| any of the above + `· partial` | unchanged | The run also left an unresolved task — same exit code as the state above                                                                                 |
| any of the above + `· warned`  | unchanged | At least one `Warn` annotated a task without otherwise changing the headline — `Warn` never resolves the task itself; `Done`/`Fail`/`Block`/… still must |

## Pick the entity

| Shape        | Use when                                                                                                                                                                      |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**     | One atomic unit submitted with `Define` (verb+object name, e.g. `write config`). Direct `Done`/`Block`/`Fail` remain for gates, not ordinary work                             |
| **Group**    | Independent collection of atomic tasks (state is **derived**); the scheduler may overlap eligible children; one `group.Task(name).Define(...)` per item for homogeneous items |
| **Sequence** | Ordered dependency of tasks (state is **derived**); a failed child auto-resolves later siblings to NotStarted; both nest via `.Sequence`/`.Group`                             |
| **File**     | Declarative managed-state file content, called from `Define`. Optional `Basis` of `evo.FSPath`/`evo.Value`/`evo.App`                                                          |
| **Patch**    | Derive FileSpecs from a unified diff; pass `result.Files` to `File`. Copying Path/Contents into a new FileSpec drops Basis                                                    |
| **Exec**     | External work with declared outputs, called from `Define`                                                                                                                     |
| **After**    | Advanced: a DAG edge Sequence cannot express. Not a lock, not for same-path File contention                                                                                   |

## Learn more

- [`docs/migration/1.0.md`](docs/migration/1.0.md) — upgrading from 0.5: every breaking change with before/after code
- [`docs/reference.md`](docs/reference.md) — construction, config, lifecycle, severity dialect, evidence capture, platform adapters, vocabulary
- [`docs/development.md`](docs/development.md) — mise commands, conformance suite, examples ladder, CLI, machine output, production ANSI driver, testkit
- [`docs/mcp.md`](docs/mcp.md) — the `evident-output-mcp` stdio server (Grok, Claude Code, Codex, …)
- [`docs/guides/teaching-ladder.md`](docs/guides/teaching-ladder.md) — the ordinary-surface learning order
- [`docs/guides/large-platform-adoption.md`](docs/guides/large-platform-adoption.md) — guidance for Docker-/npm-/Homebrew-scale CLIs
- [`docs/adoption/librarian.md`](docs/adoption/librarian.md) — a real adoption case study, with what was and wasn't validated
- [`docs/roadmap/implementation-basis.md`](docs/roadmap/implementation-basis.md), [`docs/philosophy/`](docs/philosophy/) — design philosophy
- [`docs/architecture/COMPLETENESS_MATRIX.md`](docs/architecture/COMPLETENESS_MATRIX.md) — §31 requirement coverage
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — DCO sign-off, red test → green → refactor, small conventional commits
