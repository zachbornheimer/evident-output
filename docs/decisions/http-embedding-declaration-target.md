# Decision: Embedded work declares on the Output it runs

**Status:** Accepted
**Date:** 2026-09-23
**IDs:** DEC-EMBED-001
**Ticket:** ZYS-946 (spec §53)
**Implementation:** `examples/launch-agent-http/agent.go` (`launchAgent`),
`internal/agent/review/review_isolated_facade.go` (API-062)

## Context

Spec §53 shows the shared model as `launchAgent(ctx, agent)`, declaring
work with package-level `evo.Task` / `evo.Group` / `evo.Sequence`, and
the HTTP handler calling it inside `out.Run(r.Context(), ...)` on an
Isolated Output.

Package-level declarations always reach the package default Output. An
Isolated Output's `Run` does not redirect them. Written as the sample
implies, every request's Tasks land on one shared default Output, and the
request's own `"evo.run"` document is empty. That breaks §53's own
requirement that concurrent requests share no runtime state.

## Decision

### DEC-EMBED-001: `launchAgent(out, agent)`, not `launchAgent(ctx, agent)`

Reusable work takes the `*evo.Output` that drives it and declares through
its methods (`out.Task`, `out.Sequence`, ...). The CLI passes
`evo.Default()`; the HTTP handler passes its per-request Output. `evo.File`,
`evo.Effect`, and `evo.Exec` already resolve the Output from the Define
callback's `ctx`, so they need no change.

No public API is added to route package-level declarations through a
`ctx`. That would be a second way to find the current Output, and §53
forbids expanding the public surface before one embedder proves it
necessary. `examples/launch-agent-http` is that embedder, and it needs
only the method form.

MCP rule API-062 flags the package-level spelling inside an Isolated
Output's `Run` callback, so the §53 sample shape is caught mechanically
for 1.2.0+ pins.

## Consequences

- The spec §53 sample is non-normative on this point: its `ctx`
  parameter carries cancellation, not the declaration target. The guide
  (`docs/guides/http-embedding.md`, "Declare on the Output you run")
  links here.
- If a later release adds ctx-scoped package-level declarations, this
  record is superseded and API-062 is retired with it.
