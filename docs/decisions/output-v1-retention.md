# Decision: output.v1 (`EncodeJSON`/`EncodeJSONL`/`EncodeEventJSON`) is kept in 1.1

**Status:** Accepted
**Date:** 2026-09-25
**IDs:** E-122 (lane F3)
**Reference:** ZYS-946 (FormatExternal hosts keep a documented machine format)

## Context

1.1 introduced the versioned wire vocabulary `evo.run` (`schema/run.v2.json`,
`wire.EncodeRun`) and `evo.event` (`schema/event.v2.json`, `wire.EncodeEvent`),
wired automatically into `FormatJSON`/`FormatJSONL` (`docs/migration/1.1.md`
§"`EVO_OUTPUT=json` and `jsonl` write the evo.run document"). That raised the
question of whether the older output.v1 machine projection —
`evo.JSONDocument` (`schema/output.v1.json`, `schema_version: "0.4"`),
`evo.EncodeJSON`, `evo.EncodeJSONL`, `evo.EncodeEventJSON` — is now dead
vocabulary that should be removed for 1.1, per the ZYS-1180 vocabulary freeze.

## Decision

**Keep** `JSONDocument`/`EncodeJSON`/`EncodeJSONL`/`EncodeEventJSON` as public
API. They are not legacy: they are the documented machine projection for a
host that owns its own stdout payload.

- `Format: FormatData` and `Format: FormatExternal` both promise the host
  exclusive use of stdout (`docs/reference.md` "Platform adapters" table);
  Evo never writes `evo.run`/`evo.event` there.
- Per `docs/migration/1.1.md`, such a host that still sets `EVO_OUTPUT=json`
  or `jsonl` gets the output.v1 projection on **stderr**, not stdout — the
  only shape a `FormatData`/`FormatExternal` host can consume that projection
  in.
- A host that calls `out.Snapshot()`/`out.Events()` directly (the
  `FormatExternal` pattern) needs a stable encoder for that snapshot/event
  data independent of `Format`. `EncodeJSON`/`EncodeJSONL`/`EncodeEventJSON`
  are that encoder.

`evo.run`/`evo.event` do not serve this case: they are Result-shaped
(`wire.EncodeRun(result, version)`, spec §35/§38), produced only at Finish
for `FormatJSON`/`FormatJSONL`, and never exposed as a raw Snapshot/Event
encoder a host can call mid-run.

## Classification (api_vocabulary)

Encoders and the pinned schema version:

| Symbol                             | Class            | Rationale                                                    |
| ---------------------------------- | ---------------- | ------------------------------------------------------------ |
| `evo.JSONDocument`                 | Kept, documented | output.v1 root shape for `FormatData`/`FormatExternal` hosts |
| `evo.EncodeJSON`                   | Kept, documented | Snapshot -> output.v1, called directly by such hosts         |
| `evo.EncodeJSONL`                  | Kept, documented | Events -> output.v1 JSONL, same hosts                        |
| `evo.EncodeEventJSON`              | Kept, documented | single-event output.v1 encoder, same hosts                   |
| `evo.JSONSchemaVersion` ("0.4")    | Kept, documented | pinned output.v1 schema_version                              |
| `evo.EventSchemaVersion` ("0.3")   | Kept, documented | pinned durable-event schema_version stamped into `EventJSON` |
| `wire.EncodeRun` / `evo.WriteJSON` | Canonical        | `evo.run`, spec §35, `FormatJSON` default                    |
| `wire.EncodeEvent`                 | Canonical        | `evo.event`, spec §38, `FormatJSONL` default                 |

`JSONDocument`'s field type family below is reachable only through a
`JSONDocument` or `EventJSON` value, never constructed standalone by a
host, so it is kept for the same reason `JSONDocument` itself is kept:

| Symbol                 | Class            | Rationale                                                  |
| ---------------------- | ---------------- | ---------------------------------------------------------- |
| `evo.EventJSON`        | Kept, documented | output.v1 JSONL row shape, `EncodeJSONL`/`EncodeEventJSON` |
| `evo.ConclusionJSON`   | Kept, documented | `JSONDocument.Conclusion` field type                       |
| `evo.JSONMessage`      | Kept, documented | message row inside `JSONDocument`/`ConclusionJSON`         |
| `evo.JSONOutputMeta`   | Kept, documented | `JSONDocument` output-instance identity block              |
| `evo.JSONProblem`      | Kept, documented | problem row inside `JSONDocument`                          |
| `evo.JSONTask`         | Kept, documented | task row inside `JSONDocument`'s task tree                 |
| `evo.JSONProgress`     | Kept, documented | progress field type on `JSONTask`                          |
| `evo.JSONCollection`   | Kept, documented | task-collection-with-children shape on `JSONTask`          |
| `evo.JSONChanges`      | Kept, documented | changes field type on `JSONTask`                           |
| `evo.JSONPlan`         | Kept, documented | plan field type on `JSONTask`/`JSONChanges`                |
| `evo.JSONEffectRecord` | Kept, documented | change/plan row inside `JSONPlan`/`JSONChanges`            |
| `evo.JSONAction`       | Kept, documented | action field type on `JSONTask`                            |
| `evo.JSONCommand`      | Kept, documented | argv-for-display field type on `JSONAction`                |

Neither vocabulary is a compatibility alias for the other: they serve
different `Format` choices and are documented separately in
`docs/migration/1.1.md` and `docs/reference.md`.

## Consequences

- No removal, no MCP migration rule, no golden/doc migration for these
  symbols in 1.1. `testdata/api_vocabulary.txt` classes all 19 (the encoders,
  the two pinned schema versions, and the whole `JSONDocument` field family
  above) `helper`, not `removed` — they are documented sugar for exactly one
  concept ("Machine output"), never taught as canonical vocabulary on their
  own.
- `docs/development.md`'s "Machine output" section is corrected to name both
  projections and when each applies, instead of presenting output.v1 as the
  only machine output. `docs/migration/1.1.md` and `CHANGELOG.md` point at
  this decision from the `EVO_OUTPUT` sections that could otherwise read as
  output.v1 going away.
- Revisit only if `FormatData`/`FormatExternal` hosts gain a documented way
  to receive `evo.run`/`evo.event` on stderr — no such request exists today.
