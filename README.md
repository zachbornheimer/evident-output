# Evident Output

Go presentation library for CLI **state, progress, evidence, changes, plans, and
conclusions**. Application code owns execution; package `evo` owns presentation —
so the same call sites render correctly whether stdout is a real terminal or a
log file, and the exit code always matches what the screen just said.

## Install

```bash
go get github.com/zachbornheimer/evident-output@v0.3.1
```

Requires **Go 1.25+**. License: **Apache-2.0**.

## Quickstart

```go
import (
    "os"
    evo "github.com/zachbornheimer/evident-output"
)

func main() {
    evo.Init(evo.Config{Title: "bpp-csharp"}) // first statement — arms first paint before any I/O
    os.Exit(evo.Main(run))
}

func run() error {
    // Start as casually as fmt — then promote to structure when useful.
    evo.Println("Reading configuration")
    evo.Printf("Found %d packages\n", 18)

    evo.Task("working tree").Done()
    evo.Task("branches").Block(
        "local-only branch",
        evo.Detail("commit or stash before continuing"),
    )

    evo.Task("cleanup").Delete(2, "stale local branch") // singular object, ledger renders "2 stale local branches"
    for pkg := range evo.Task("install").Each(packages) {
        install(pkg)
    }
    return nil // Block is a presentation outcome, not a Go error
}
```

On a real terminal, `install` draws an in-place, colored, animated progress
line while it runs — no extra code. Piped (`prog > log.txt`, CI, an agent
harness), the same call sites fall back to plain, durable lines:

```text
Reading configuration
Found 18 packages
✓  working tree
⊘  branches  local-only branch
   └─ commit or stash before continuing
◐  install  0/2
◐  install  1/2  a
◐  install  2/2  b
✓  cleanup
✓  install
[changed]  cleanup
  deleted  2 stale local branches

[blocked]  bpp-csharp
```

Exit code `1` — see the table below. Try the live, colored version:
`go run ./examples/repo-status/`.

## Conclusion band → exit code

The trailing `[state]` band and the process exit code always agree — never read
one without checking the other. `· partial` and `· warned` are modifiers on
the state, not a state of their own.

| Band                           | Exit code | Meaning                                                                  |
| ------------------------------ | --------- | ------------------------------------------------------------------------ |
| `[changed]`                    | `0`       | A mutation verb (`Delete`/`Create`/…) recorded outside `DryRun`          |
| `[planned]`                    | `0`       | A mutation verb recorded under `Config.DryRun` (would, not did)          |
| `[ready]`                      | `0`       | Every task resolved `Done`; no mutation verb recorded                    |
| `[unchanged]`                  | `0`       | Every task resolved `Done.Unchanged`; nothing needed to change           |
| `[blocked]`                    | `1`       | At least one `Block`, and nothing `Fail`ed                               |
| `[failed]`                     | `2`       | At least one `Fail`, or a caller-supplied misuse                         |
| `[cancelled]`                  | `130`     | `Cancel` or an interrupt ended the run early                             |
| any of the above + `· partial` | unchanged | The run also left an unresolved task — same exit code as the state above |
| any of the above + `· warned`  | unchanged | At least one `Warn` resolved without otherwise changing the headline     |

## Pick the entity

| Shape     | Use when                                                                                                                                                                                                    |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Task**  | Everything — a check/gate resolved directly (`Done`/`Warn`/`Block`/`Fail`/`Skip`, no `Phase`/`Progress`) renders as a fact row; work with phases, progress, or mutation verbs shows a spinner while running |
| **Tasks** | Collection of independent tasks (state is **derived**)                                                                                                                                                      |

## Learn more

- [`docs/reference.md`](docs/reference.md) — construction, config, lifecycle, severity dialect, evidence capture, platform adapters, vocabulary
- [`docs/development.md`](docs/development.md) — mise commands, conformance suite, examples ladder, CLI, machine output, production ANSI driver, testkit
- [`docs/mcp.md`](docs/mcp.md) — the `evident-output-mcp` stdio server (Grok, Claude Code, Codex, …)
- [`docs/roadmap/implementation-basis.md`](docs/roadmap/implementation-basis.md), [`docs/philosophy/`](docs/philosophy/) — design philosophy
- [`docs/architecture/COMPLETENESS_MATRIX.md`](docs/architecture/COMPLETENESS_MATRIX.md) — §31 requirement coverage
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — DCO sign-off, red test → green → refactor, small conventional commits
