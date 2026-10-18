# Tier 1.1 reports no waivers

On 2026-10-15 the last 42 tier 1.1 waivers left the spec registry, and the ratchet lost the 39 of them it held.
Each waiver excused a 1.x requirement whose 2.0 successor is a sentence of `CONTRACT.md`.
Where a C40 to C48 requirement already quotes that sentence and binds its tests, the 1.x entry only duplicated it, so it left.
Four successors had no requirement yet and gained one: C42-020 (`Container` row, which also states that a Task declares no children), C42-021 (a container's `Summary`), C44-010 (`Fail`, `Refuse` and `Exclude` record every `ProblemOption`) and C44-011 (`Outcome` is the only verdict type).
Some 1.x sentences have no 2.0 counterpart. They left with nothing to bind: nil-handle safety, the bounded Human problem list, Summary narration review (API-060), Evidence phases, and an Effect callback that settles its own Task.

| Removed 1.x entries                                                                                  | 2.0 successor and its requirement                    |
| ---------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| C02-003, C02-013, C02-014 (`Block`, `Next`)                                                          | `Refuse` and `Next`: C44-002, C44-007                |
| C02-005, C02-038, C30-003, C30-007, C30-037, C30-038, C30-039 (severity and `Warn`, removed in 1.1)  | Problems never end a Task: C44-001; options: C44-010 |
| C02-008, C02-012, C05-008, C05-010, C13-001, C13-003, C17-008, C18-006, C30-005, C30-009 (`Skipped`) | `Exclude`: C44-006, C48-006                          |
| C02-016 (`Init`, removed in 2.0)                                                                     | `Main` and `Execute`: C41-001, C41-002, C41-003      |
| C02-022, C30-001, C30-010 (handles)                                                                  | `Container`: C42-020                                 |
| C02-032, C02-033 (`EntityState`, `Resolution`)                                                       | `Outcome` and `state`: C44-009, C44-011              |
| C02-036, C21-011 (`Evidence`)                                                                        | `Task.Writer` and Facts: C42-013, C42-014            |
| C05-005, C05-007 (`Done`, `Preview`)                                                                 | `Verify` and `Config.DryRun`: C42-004, C41-005       |
| C11-001, C13-006 (hidden Facts and no-op rows)                                                       | C42-014, C48-002                                     |
| C21-006 (`Done` narration)                                                                           | no 2.0 sentence                                      |
| C26-003 (`File`)                                                                                     | C43-002, C43-013                                     |
| C30-008 (undefined Task)                                                                             | C44-008, C47-006                                     |
| C30-011, C30-014 (`Summary`)                                                                         | C42-015, C42-021                                     |
| C30-016 (Summary narration)                                                                          | no 2.0 sentence                                      |
| C30-026, C30-031 (`Wait`)                                                                            | `After` and Group overlap: C42-002                   |
| C30-041 (bounded problem list)                                                                       | no 2.0 sentence                                      |
| C30-048 (Effect callback verdicts)                                                                   | no 2.0 sentence                                      |
