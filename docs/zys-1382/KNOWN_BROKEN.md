# ZYS-1382 known-broken list

ZYS-1382 replaced the 1.1 shapes (`FileSpec` + `File(ctx, spec)`, `ExecSpec` +
`Exec(ctx, spec)`, `FileSet` + `Files`, operation-level `Basis`, `FSPath`) with
plain `File` / `Tree` / `Exec` structs, `TaskHandle.Basis`, and
`Patch(ctx, diff) error`. Code that used the old shapes no longer compiles.
None of it was deleted or skipped: each file below carries
`//go:build evo_pre1382` as its first line and nothing else changed, so

```sh
go test -tags evo_pre1382 ./...
```

brings every one back and shows it failing. `TestKnownBroken_ListMatchesQuarantine`
(root package) fails when this list and the tagged files disagree. Remove a
file's tag and its line here in the same change that migrates it.

## Quarantined files

- `example_define_test.go`: uses `evo.File(ctx, evo.FileSpec{...})`.
- `example_exec_test.go`: uses `evo.ExecSpec`, `evo.Exec(ctx, spec)`, and `evo.FSPath`.
- `example_file_test.go`: uses `evo.FileSpec` and `evo.File(ctx, spec)`.
- `example_fingerprint_test.go`: uses `evo.FSPath`.
- `example_patch_test.go`: uses `evo.FileSet`, `evo.Files`, the two-result `evo.Patch`, `evo.ErrStaleBasis`, and `evo.ErrPatchDeleteUnsupported`.
- `file_named_row_test.go`: uses `evo.File(ctx, evo.FileSpec{...})`.
- `named_row_streaming_test.go`: uses `evo.File(ctx, evo.FileSpec{...})`.
- `patch_test.go`: uses `evo.FileSet`, the two-result `evo.Patch`, `evo.ErrPatchDeleteUnsupported`, and `evo.ErrPatchRenameUnsupported`.
- `patch_verification_test.go`: uses `evo.Files` and the two-result `evo.Patch`.
- `scenario_test.go`: uses `evo.FileSpec` and `evo.File(ctx, spec)`. Its helper `firstRune` is served by `first_rune_unquarantined_test.go` (tagged `!evo_pre1382`) while it is quarantined.
- `wire_schema_test.go`: uses `evo.File(ctx, evo.FileSpec{...})`.
- `internal/docexamples/fixtures/migration_1_0_file_spec/main.go`: compiles the frozen `docs/migration/1.0.md` fence, which teaches `evo.FileSpec` with `Basis: []evo.Fingerprint{evo.FSPath(...)}`. The fence is history, so the fixture cannot be migrated without rewriting the 1.0 guide.

## Failing without quarantine (compiles, fails visibly)

These compile against the new surface and fail at run time. They are not
tagged; they stay red in `go test ./...` until their owner migrates them.

- `internal/agent/review`: `TestMigration1_1EveryRemovedNameHasDirtyRewriteCleanFixture`
  wants a dirty/rewrite/clean migration fixture for each name ZYS-1382 retired
  (`FileSpec`, `ExecSpec`, `FSPath`, `FileSet`, `Files`, `ErrStaleBasis`,
  `ErrFileSpecMissingPath`, `ErrFileUnmanagedContentsMissing`,
  `ErrExecSpecMissingExecutable`, `ErrPatchDeleteUnsupported`,
  `ErrPatchRenameUnsupported`). The MCP autofixer does not rewrite them yet.
- `internal/docexamples`: `TestRuleGoodCodeTypeChecks` type-checks MCP rule
  GoodCode snippets (API-054, API-057, API-058, API-059, EVO-FILE-001,
  EVO-EXEC-001, and others) that still teach the 1.1 shapes.
