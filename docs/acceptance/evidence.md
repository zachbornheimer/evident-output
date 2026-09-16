# v0.6/1.0 acceptance command evidence

Baseline commit: `7e501d2995bd65df8fc9bc718fbc389587717193` (`v0.5.2`). The
shipped version for this work is **1.0.0** (breaking release; see
`v0.6.md`'s Owner decisions note for what that changes).

## Commands (increment 1, this worktree)

| Command                                                             | Result                                                                     | Purpose                                                                                                                                                                                                                                          |
| ------------------------------------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `go build ./...`                                                    | exit 0                                                                     | whole repo compiles                                                                                                                                                                                                                              |
| `gofmt -l .`                                                        | no output                                                                  | whole repo formatted                                                                                                                                                                                                                             |
| `go vet ./...`                                                      | exit 0                                                                     | whole repo vets clean                                                                                                                                                                                                                            |
| `go test ./...`                                                     | exit 0, all `ok`                                                           | full ordinary suite green, including the legacy conformance suite (`conformance/gates`, `conformance/goldens`) and the MCP self-documentation packages                                                                                           |
| `go test -race ./...`                                               | exit 0, all `ok`                                                           | full suite race-clean                                                                                                                                                                                                                            |
| `mise run ci`                                                       | exit 0                                                                     | lint + test + scan + conformance + traceability (trunk's own golangci-lint/gitleaks tool installs fail on this host's Go toolchain skew — a pre-existing, unrelated condition mise.toml itself tolerates with `\|\| echo 'trunk check skipped'`) |
| `mise run test-race`                                                | exit 0                                                                     | same race-clean suite via the mise task                                                                                                                                                                                                          |
| `go test -tags v06acceptance ./conformance/future/api`              | exit 0, `ok`                                                               | increment-1 API compile contract                                                                                                                                                                                                                 |
| `go test -tags v06acceptance ./conformance/future/behavior`         | exit 0, `ok`                                                               | increment-1 Verify behavioral fixtures                                                                                                                                                                                                           |
| `go test -tags v06acceptance ./conformance/future/api/pending`      | exit 1, `FAIL [build failed]` (expected)                                   | increment 2+ (File/Exec/AppID/StateDir) still red-by-compile                                                                                                                                                                                     |
| `go test -tags v06acceptance ./conformance/future/behavior/pending` | exit 1, `FAIL [build failed]` (expected)                                   | increment 2+ (File/dry-run/cancellation/manifest re-entry) still red-by-compile                                                                                                                                                                  |
| `go test -tags v06acceptance ./conformance/future/...`              | exit 1 overall, per-package: 2 `ok`, 2 `FAIL [build failed]`, none skipped | proves increment-1 and increment-2+ packages are compile-isolated in one invocation                                                                                                                                                              |

## Red-then-green evidence per section (roast discipline)

- **§3.1 identity reversal**: before this change, `evo.Task("same")` called
  twice returned the identical `*TaskHandle` (get-or-create); the reversal
  is a genuine behavior change, not a missing symbol, so its tests were
  authored against the new contract and shown green after
  `internal/engine/identity.go`/`taskScoped`/`declareGroupTask`/
  `declareChildContainerLocked`/`Output.Group`/`Output.Sequence` were
  rewritten. Full `go test ./...` was green both before and after purely by
  coincidence of the pre-existing suite's coverage gaps — the _new_
  duplicate-rejection assertions (`identity_duplicate_sibling_test.go` etc.)
  did not exist before this change, so "red" here is "did not exist/would
  have asserted the opposite," which the rewritten `TestDOM004_*`,
  `TestCON018_*`, `TestSequence_PackageLevelRepeatIsDuplicateSibling`,
  `TestAPISugar_TaskDuplicateFormattedNameIsRejected` tests make explicit in
  their own doc comments (each states the prior get-or-create behavior they
  replace).
- **§7/§9.1 Define+Verify**: `TaskHandle.Verify` and context-aware `Define`
  did not exist before this increment — `conformance/future/behavior`'s
  fixtures failed to compile (`task.Verify undefined`) against the
  pre-increment tree; after `internal/engine/verify.go` and the `Define`
  signature change, the same file compiles and every `TestV06Verify*`/
  `TestV06PostVerify*` test passes, confirmed via
  `go test -tags v06acceptance ./conformance/future/behavior -v`.
- **§7.1 task context**: `internal/engine/taskscope_internal_test.go` calls
  the unexported `taskScope` directly; before `taskscope.go` existed, this
  package failed to compile (`undefined: taskScope`). After: all three
  cases (no scope / inside callback / after callback) pass.
- **§29–31 result model**: `TaskSnapshot.Resolution`/`.Evidence` did not
  exist before this increment (compile-red: `snap.Resolution undefined` in
  `result_model_test.go`). After `internal/core/resolution.go` and the
  `taskState`/`runDefine` wiring, all five `result_model_test.go` cases
  pass.
- **§46 API golden**: proven to catch a real regression, not just to pass vacuously — a temporary `func MainWith() {}` (MainWith was removed in 1.0) reintroduced into `api.go` failed `TestAPIGolden_PublicSurfaceMatchesCommittedGolden` with `retired API name "MainWith" reappeared in the public surface` (removed in 1.0), then the revert restored green.

## Compatibility note

The legacy JSON 0.4 and JSONL 0.3 wire encoders and their goldens
(`conformance/gates`, `conformance/goldens`) are unchanged and remain green
throughout. No production wire-format code was touched by this increment.
