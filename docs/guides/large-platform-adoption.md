# Large-platform adoption

Guidance for Docker-/npm-/Homebrew-scale CLIs integrating Evident Output.

## Architecture triangle

```text
domain / facade     → work + neutral progress callbacks (no evo import)
command layer       → evo entities and presentation
machine contract    → existing -status JSON / ResultWriter / schemas
```

## What to adopt first

1. `Init(Config{Title})` + `Main` or hosted `Output.Run`
2. Mutation verbs (`Delete`/`Create`/`Update`/…) for dry-run vs live — `Config.DryRun` picks
   `[planned]` vs `[changed]` at the same call site
3. Task for real gates; `FailedBy`/`BlockedBy` for path-scoped evidence
4. Capture on Task for subprocesses
5. `ID` / `Scope` when structured consumers exist
6. `Config.Redactor` before Capture/debug retention of secrets

## What not to force

- Per-file Tasks for huge batches — use aggregate Progress (RULE-005)
- Evo as command router or scheduler
- Replacing a stable machine JSON contract with evo human chrome
- Pluralization or localization inside evo

## Concurrent work

Predeclare Tasks in semantic order; workers only update handles. See
`docs/guides/concurrent-progress.md`.

## Case study

`docs/adoption/librarian.md` — batch-summary adoption, mistakes, and limits.
