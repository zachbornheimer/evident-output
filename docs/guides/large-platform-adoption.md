# Large-platform adoption

Guidance for Docker-/npm-/Homebrew-scale CLIs integrating Evident Output.

## Architecture triangle

```text
domain / facade     → work + neutral progress callbacks (no evo import)
command layer       → evo entities and presentation
machine contract    → existing -status JSON / ResultWriter / schemas
```

## What to adopt first

1. `New(Config{Title})` + `Main` or hosted Finish+Close
2. Plan vs Changes for dry-run vs live
3. Item for real gates; FailedBy for path-scoped evidence
4. Capture on Task/Item for subprocesses
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
