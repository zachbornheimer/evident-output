# Cutting a release (pin maintenance class)

Install pins are a **maintenance class**, not one-off README edits.

## Source of truth

```go
// release.go
const PublishedRelease = "v0.2.N"
```

Everything portable (README, skills, integrations, MCP config-generator fallback)
must recommend that tag. Historical design docs under `docs/roadmap/` and
`docs/architecture/` may narrate older versions; they are not install surfaces.

## Procedure (automated)

After implementation is committed on `main` and tests are green:

```bash
mise run test && mise run cut-release
# or: mise run release
# or pin explicitly: VERSION=v0.2.12 mise run cut-release
```

`tools/scripts/cut-release.sh` will:

1. Auto patch-bump `PublishedRelease` (or use `VERSION=vX.Y.Z`)
2. Run `sync-release-pins`
3. Run VersionDrift tests
4. Commit pin files if needed
5. Create annotated tag (refuses if tag already exists — **never moves tags**)
6. Push `HEAD` and the new tag (`CUT_RELEASE_PUSH=0` to skip push)

### Manual procedure (same steps)

1. **Implement** and commit.
2. **Bump** `PublishedRelease` in `release.go`.
3. **Sync** portable pins: `mise run sync-release-pins`
4. **Gate**: `go test . -run VersionDrift` and `go test ./...`
5. **Commit** pin updates; **tag** `vX.Y.Z`; **push** main + tag.

Never move a prior tag (v0.2.9 → fix in v0.2.10 style).

## What the drift gate forbids (class)

| Defect                                                                         | Detection                                |
| ------------------------------------------------------------------------------ | ---------------------------------------- |
| Skill/integration/README install pin ≠ `PublishedRelease`                      | `version_drift_test.go`                  |
| `go get/install/run …@latest` on this module                                   | same                                     |
| Personal clone paths (`Developer/Personal/…`, `/Users/…`) on portable surfaces | same                                     |
| MCP config_client hardcoding a tag string                                      | must use `evo.PublishedRelease`          |
| New skill/integration md forgotten from a hand list                            | auto-walk of `skills/` + `integrations/` |

## What it does not do

- Does not rewrite historical ADRs or polish synthesis timelines.
- Does not inject ldflags into CI binaries (local `Version` may still be `dev`;
  fallback is `PublishedRelease`).
- Does not change library API or wire schema versions (`EventSchemaVersion` is separate).
