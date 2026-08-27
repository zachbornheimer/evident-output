# Exported API surface inventory (WS-7)

**Baseline:** post-coalescing polish phase
**Categories:** ordinary · advanced · compatibility · remove-before-v1 · retain (distinct domain)

Classification is intentional spelling policy (PHIL-001–003), not a freeze of every symbol.

## Ordinary (lead sheet)

| Symbol                                               | Notes                            |
| ---------------------------------------------------- | -------------------------------- |
| `New`, `Config`, `DefaultConfig`                     | Construction                     |
| `Main`                                               | Standalone lifecycle convenience |
| `Finish`, `Close`, `Conclusion`, `Snapshot`          | Lifecycle / model                |
| `Print`, `Printf`, `Println`, `Verbose`              | Human prose                      |
| `Item`, `Task`, `Tasks`                              | Entities                         |
| `ID`, `Scope`                                        | Machine keys / namespace         |
| `Plan`, `Changes` + built-in effect verbs + `Record` | Effects                          |
| `Capture` on Task/Item                               | Evidence                         |
| `Cause`, `Detail`, Problem options                   | Evidence attachment              |
| `OK`/`Warn`/`Block`/`Fail` + `*By` plurals           | Outcomes (PHIL-002 voicing)      |
| `Phase`, `Progress`, `Bytes`, `Done`                 | Work                             |
| `SlogHandler`                                        | Logging bridge                   |
| `ResultWriter`, `FormatData`                         | Machine purity                   |
| `Redactor` / `Config.Redactor`                       | Pre-retention scrub              |

## Advanced (studio)

| Symbol                                      | Notes                      |
| ------------------------------------------- | -------------------------- |
| `NewWithOptions`, `Option` helpers          | Tests / injection          |
| `Progress64`, `Advance`                     | 64-bit / relative progress |
| `Debug`, debug pane options                 | Diagnostics                |
| Terminal drivers, `VisibilityDelay`/`Delay` | Live region                |
| `Item.Start`                                | Prefer omit on happy path  |
| Session-level `Output.Capture`              | Prefer Task/Item ownership |

## Compatibility / historical

| Symbol                                | Notes                                        |
| ------------------------------------- | -------------------------------------------- |
| Deprecated constructors (`For`, etc.) | Removed in honesty pass — do not reintroduce |

## Remove-before-v1 candidates

| Symbol                          | Disposition                                   |
| ------------------------------- | --------------------------------------------- |
| Remaining unused export aliases | Audit per OPEN-008                            |
| `Progress64`                    | OPEN-009: retain advanced until proven unused |

## Retain as distinct domain (not aliases)

`Block` vs `BlockedBy`, `Progress` vs `Bytes`, `Item` vs `Task`, `Plan` vs `Changes`.

## Honesty rule

No public field/parameter may be ignored (PHIL-007). Config zero-value holes are defects.
