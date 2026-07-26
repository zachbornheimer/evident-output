# 001. Repository creation

Date: 2026-07-26

## Status

Accepted

## Context

Evident Output needs a public Go module implementing the presentation library
defined in architecture specification v0.3.

## Decision

Create repository `evident-output` with module path
`github.com/zachbornheimer/evident-output`, root package name `evo`,
Apache-2.0 license, mise task contract, trunk in daemonless mode, and a
roast-style conformance suite as the behavioral source of truth.

## Consequences

- Implementation follows red-green against conformance IDs.
- Tooling is mise + trunk (no daemon); CI is `mise run ci`.
- Pre-1.0 APIs may change with migration notes.
