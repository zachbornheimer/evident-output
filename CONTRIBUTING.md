# Contributing to Evident Output

## Developer Certificate of Origin

By contributing, you certify the [DCO 1.1](https://developercertificate.org/).
Sign off commits with:

```bash
git commit -s -m "feat: your change"
```

## Process

1. Express behavior as a failing conformance or unit test (red).
2. Implement the smallest correct change (green).
3. Refactor behind the green bar.
4. Run `mise run ci`.
5. Small conventional commits (`feat:`, `fix:`, `test:`, `docs:`, `chore:`).

## Conformance

The roast suite under `conformance/` is the executable specification.
Every §31 requirement ID lives in `conformance/TRACEABILITY.md`.
Do not silently drop requirement IDs.

## Scope

Evident Output is a presentation library. Do not add command frameworks,
schedulers, `RunAll`/`Map`/`Retry`, or shell execution to the core package.

## Release pins (maintenance class)

Install version strings in README, skills, and integrations are **one class of
defect**. Do not edit them ad hoc.

1. Change `PublishedRelease` in `release.go`.
2. `mise run sync-release-pins` (or `go run ./tools/scripts/sync-release-pins`).
3. `go test . -run VersionDrift` (also part of `mise run doctor`).

Full procedure: [`docs/guides/cutting-a-release.md`](docs/guides/cutting-a-release.md).
Never move a previously published git tag to fix a stale README — ship a patch.
