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

## Procedure

1. **Implement** (or skip if hygiene-only).
2. **Bump** `PublishedRelease` in `release.go`.
3. **Sync** portable pins:

   ```bash
   go run ./scripts/sync-release-pins
   ```

4. **Gate**:

   ```bash
   go test . -run VersionDrift
   go test ./...
   ```

5. **Commit** with the release notes in the message.
6. **Tag** the commit (`git tag -a v0.2.N`). **Never** move a prior signed tag
   (if a tag shipped with a stale README, fix in the next patch — v0.2.9 → v0.2.10).
7. **Push** `main` and the new tag.

## What the drift gate forbids (class)

| Defect | Detection |
|--------|-----------|
| Skill/integration/README install pin ≠ `PublishedRelease` | `version_drift_test.go` |
| `go get/install/run …@latest` on this module | same |
| Personal clone paths (`Developer/Personal/…`, `/Users/…`) on portable surfaces | same |
| MCP config_client hardcoding a tag string | must use `evo.PublishedRelease` |
| New skill/integration md forgotten from a hand list | auto-walk of `skills/` + `integrations/` |

## What it does not do

- Does not rewrite historical ADRs or polish synthesis timelines.
- Does not inject ldflags into CI binaries (local `Version` may still be `dev`;
  fallback is `PublishedRelease`).
- Does not change library API or wire schema versions (`EventSchemaVersion` is separate).
