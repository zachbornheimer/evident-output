# 002. Conformance scenario schema v1

Date: 2026-07-26

## Status

Accepted

## Context

Parallel roast and domain work must share one executable fixture dialect
(Raku roast model).

## Decision

Check in `conformance/schema/scenario.v1.json` as the sole declarative
scenario dialect. The Go runner loads `conformance/scenarios/*.json` and
rejects missing id/title. Domain unit tests may use shared assert helpers
but must not invent a second scenario language.

## Consequences

- Schema changes require an ADR and version bump if breaking.
- Appendix H Go tests remain first-class alongside JSON scenarios.
