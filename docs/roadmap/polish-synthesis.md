# Evident Output — Polish Synthesis

**Date:** 2026-07-28
**Status:** Working synthesis for refinement → next phase
**Scope:** Artistic DX, platform honesty, librarian adoption, multi-party critique
**Library tip (at writing):** `evident-output` **v0.2.8** (`dab378e`)
**Adoption tip (at writing):** `librarian` local commits `3edd911` → `8eae112` on evo **v0.2.8**

This document consolidates three conversational strands into one source of truth for polish and the next phase of work. It is intentionally complete: later refinement should cut and order, not rediscover.

---

## 1. Purpose of this document

We need a single artifact that:

1. States the **aesthetic bar** (jazz composition / marble subtraction) for Evo’s public syntax.
2. Records **what production feedback and zero-context reviews forced into the library** through v0.2.8.
3. Captures the **librarian adoption** as a real coexistence test (domain, concurrency, dry-run, machine contract).
4. Merges **three layers of assessment** (orchestrator write-up → adversarial DX → ChatGPT synthesis) into **adopted house rules**.
5. Separates **validated claims** from **unproven surface**.
6. Lists a **prioritized next phase** for Evo, librarian, docs, and case-study writing.

**Not the goal of this phase:** ship per-file Tasks into librarian, or invent pluralization / in-flight-only Task APIs.

---

## 2. The three conversational points (map)

| #     | Strand                                    | Core question                                                    | Primary outcome                                                                                                   |
| ----- | ----------------------------------------- | ---------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| **A** | Artistic DX / jazz spelling               | Is the public chart one lead sheet or many enharmonic spellings? | Ordinary ladder vs studio appendix; false notation is worse than sugar                                            |
| **B** | Platform honesty & release hardening      | Do public marks perform what they promise?                       | v0.2.6–v0.2.8: CSI Problems, Delay(0), Scope narrow, slog one conductor, Item.Capture, SIGWINCH, Actions Node 24  |
| **C** | Librarian adoption + multi-party critique | What does a real app teach?                                      | Presentation model becomes explicit; batch-summary validated; output taste bugs found; per-file pattern corrected |

These strands interact: (A) sets taste; (B) makes taste mechanically honest; (C) proves and stresses both against concurrent domain code and an existing machine contract.

---

## 3. Aesthetic bar — jazz composition for syntax

### 3.1 Principles (binding)

Derived from project standards (code-as-composition, subtraction, WHAT-not-HOW) and the explicit jazz metaphor used in review:

| Principle                                | Meaning for Evo                                                                                                              |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| **Same note, same spelling**             | One ordinary API per domain intent. No Eb three times and D# once.                                                           |
| **Different voicing is not duplication** | `Block` vs `BlockedBy`, `Progress` vs `Bytes`, `Item` vs `Task` preserve different information.                              |
| **Studio overdub**                       | Advanced APIs may exist (Options, Terminal, Progress64, session Capture) but must not compete with the lead sheet.           |
| **Marble subtraction**                   | Prefer removing alternate charts and vanity rows over adding motifs.                                                         |
| **Instinct plays the right notes**       | Defaults and examples make correct usage the path of least resistance.                                                       |
| **Hear your part in the band**           | Item = condition, Task = work, Plan/Changes = tense of effect, slog = infrastructure, ResultWriter/status = machine payload. |

### 3.2 Three tests before deleting or merging APIs

1. **Same-note test** — Same domain intent, difference is only mechanics/history → merge or remove one spelling.
   Examples of genuine enharmonics (historical): `For` vs `New(Config)`, `Progress` vs `Progress64`, `KeepLastLines` vs `CaptureLines`.

2. **Different-voicing test** — Shared cadence, different information → keep both.
   Examples: `Block(summary, opts…)` vs `BlockedBy(...Problem)`; `Progress(n,n)` vs `Bytes`; `Item` vs `Task`.

3. **Studio-overdub test** — Legitimate but not ordinary lead sheet → demote/document as advanced.
   Examples: `NewWithOptions`, custom Terminal, relative `Advance`, session `Output.Capture`, deterministic clocks.

### 3.3 Canonical ordinary lead sheet (adopted)

```text
New(Config) → Main → Print*/Verbose → Item/Task(+ID) → Capture (Task|Item)
             → Plan | Changes → slog via SlogHandler() [level from Config.Debug]
             → ResultWriter / external machine contracts as needed
```

**Opening:**

```go
out := evo.New(evo.Config{Title: "install"})
os.Exit(evo.Main(out, run))
```

**Note on Main:** Returning an exit code from a helper is fine; documenting `os.Exit(Main(...))` as the _only_ recipe is slightly framework-shaped for a pure presentation library. Acceptable as ordinary sugar for tiny tools; hosts (Cobra, custom runners) still own process exit. Do not expand Main into a full runner.

### 3.4 What must not be flattened (armature)

- Item vs Task vs Tasks
- Warn vs Block vs Fail
- Plan vs Changes
- Detail vs Cause
- Capture retention vs debug visibility
- Application execution vs presentation lifecycle
- Raw result / status JSON vs human presentation

Aesthetic review makes these _more_ visible, not less.

---

## 4. Platform honesty track (v0.2.x → v0.2.8)

### 4.1 Release arc (condensed)

| Version | Theme                                                                                                                                     |
| ------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| v0.2.3  | Platform contracts: Redactor, ResultWriter, ID, Scope (early)                                                                             |
| v0.2.4  | Capture pending fragments; UTF-8 truncate; LevelUnset/Trace                                                                               |
| v0.2.5  | All slog levels journaled with structure                                                                                                  |
| v0.2.6  | Shared Problem CSI sanitize; VisibilityDelay honored; resize RefreshSize; schema 0.2; Main no double-Fail                                 |
| v0.2.7  | **Honesty-first:** Delay(0) expressible; remove false notation; narrow Scope; Item.Capture; slog one conductor; strip legacy constructors |
| v0.2.8  | Pedagogy examples; unix SIGWINCH watch; Actions checkout@v6 / setup-go@v6 (Node 24)                                                       |

### 4.2 Honesty rules (false notation is P0)

A public name that does not fully perform is worse than missing sugar.

| Symptom                                        | Rule                                                                                     |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------- |
| Config field cannot express intentional zero   | Distinguish unspecified vs explicit (e.g. `VisibilityDelay *time.Duration` + `Delay(0)`) |
| Method accepts fields and discards them        | Remove or implement (`Info`/`WarnMessage` with ignored fields → removed)                 |
| Struct advertises fields ItemWith ignores      | Remove or implement fully (`ItemSpec` → removed)                                         |
| Scope methods that do not scope                | Narrow Scope to Item/Task/Tasks only                                                     |
| Dual log thresholds (Config + SlogHandler min) | One conductor: `SlogHandler()` reads `Config.Debug.Level`                                |
| Silent Capture loses unterminated lines        | Observation methods snapshot pending fragments                                           |

### 4.3 CSI / Problem contract

Terminal safety is a **presentation invariant**, not Item-vs-Task quirk:

```text
applyProblemOptions → sanitizeProblem
storeProblems (clone + sanitize)  ← Item and Task both
```

Any future entity attaching `Problem` uses the same path. Capture sanitizes on its own retention boundary.

### 4.4 Capture ownership (ordinary ladder)

| Owner              | Use                                                              |
| ------------------ | ---------------------------------------------------------------- |
| `Task.Capture()`   | Work / subprocess for an operation                               |
| `Item.Capture()`   | Tool-backed **condition** (git status, docker info, brew doctor) |
| `Output.Capture()` | Advanced session sink only                                       |

Silent default; `Cause` diagnostic; `DetailTail` user-visible evidence. Success stays quiet.

### 4.5 Logging division of labor

```text
Human prose:     Print / Printf / Println / Verbose()
Semantic state:  Item / Task / Changes / Plan
Infrastructure:  slog.New(out.SlogHandler())  // level from Config.Debug only
```

User-actionable command failure **must not exist only in the debug journal**.

### 4.6 Still deferred / non-blockers (known limits)

- Windows ConPTY / full PTY matrix (external)
- Resize: SIGWINCH + on-redraw RefreshSize (unix); not a perfect multi-driver matrix
- Scope is **not** a security sandbox (documented)
- NoopRedactor default (secrets need opt-in Redactor)
- schema_version wire format honesty vs full version-manifest generator

---

## 5. Librarian adoption — what happened

### 5.1 Context

**librarian** is a manifest-driven file-routing broker (NAS cross-zone writer). Domain lives in `internal/broker`, `facade`, `router`, `manifest`. CLI is thin.

**Before (pre-evo, e.g. `5e1be73`):**

- `slog.NewJSONHandler(os.Stdout)` as de facto UI
- Per-file `logger.Info("placed file"…)` from concurrent workers
- Summary counts only in a final log line and optional `-status` JSON
- Hard fail: `fmt.Fprintln(stderr); os.Exit(1)`

**After (local `8eae112`, evo v0.2.8):**

- `evo.New(Config{Title, Debug.LevelWarn})` + `Main`
- Workers: slog WARN/ERROR only via `out.SlogHandler()`
- After `wg.Wait()`: `presentRun` → Plan (dry-run) or Changes + Item (live)
- `-status` JSON unchanged (machine contract preserved)
- Broker/facade/router **not** importing evo

### 5.2 Code size (cmd/librarian)

| File         |  Pre-evo | After v0.2.8 |        Δ |
| ------------ | -------: | -----------: | -------: |
| main.go      |      119 |          129 |      +10 |
| run.go       |      215 |          183 |      −32 |
| present.go   |        — |         ~101 |     +101 |
| main_test.go |      125 |          141 |      +16 |
| **Total**    | **~459** |     **~554** | **~+95** |

Vs pre-evo (cmd + go.mod/sum): roughly **~173 insertions, ~69 deletions**.

**Do not apologize for +95 lines.** The migration made an implicit UI explicit. Right metrics:

```text
presentation decisions centralized
domain packages unchanged
machine contract preserved
worker log noise removed
failure / dry-run tense made explicit
```

### 5.3 What adoption proved

> Evo caused the application to **discover and name its presentation model**.

Boundary validated:

```text
broker/facade/router  = domain and execution
present.go            = human presentation
-status JSON          = existing machine contract
```

Also validated for **batch-summary mode**:

- Config construction
- Main lifecycle
- Plan vs Changes tense
- Item severity for run gate (summary-only)
- slog as diagnostics, not UI
- Coexistence with concurrency (if present only after wait)
- Tests with discarded Output

### 5.4 What adoption did **not** prove

- Live concurrent Task updates under load
- Thousands of Tasks / viewport policy
- Per-file Capture
- Progress callback ergonomics in facade
- Terminal resize during multi-minute runs (library has hooks; not exercised by librarian)
- Debug journal interleaved with active Tasks
- Partial-change + failed conclusions as product UX
- Cancellation mid-flight
- Evo JSON as an **external** contract (librarian keeps its own status schema)

**Do not present librarian as validating the complete Evo surface.**

### 5.5 Observed output problems (adoption bugs / taste debt)

These appeared in the real dry-run/live demo and became evo+app lessons:

#### Problem 1 — Duplicated conclusion cadence

```text
[changed]  librarian
  ...
[changed]  librarian
```

Plan/Changes section + standalone conclusion with same subject — not sweet when conclusion adds nothing.

#### Problem 2 — Wrong effect language

```text
added    1 files placed
removed  1 sources offloaded
```

- Grammar: “1 files”
- Domain: operation was **placed** / **offloaded**, not “added files placed”

#### Problem 3 — Dry-run over-orchestrated

```text
Dry-run: no files will be written or deleted
✓  dry-run plan
[planned]  librarian dry-run
...
[planned]  librarian
```

Four statements that mostly say “valid dry-run plan.” Vanity Item + duplicate conclusion.

#### Problem 4 — Failure evidence thin if only slog + count Fail

User-actionable failures must surface as structured Problems on an Item (or Tasks), not only in the journal.

---

## 6. Merged critique — three assessment layers

### 6.1 Layer map

| Layer                                   | Role                                                                                                        |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| **Orchestrator (Grok) adoption report** | Measured ease, LOC, architecture win; first per-file progress sketch                                        |
| **Adversarial / multi-model DX**        | Exposed false notation earlier; later refined same-note vs voicing; corrected over-deletion instincts       |
| **ChatGPT synthesis**                   | Froze architecture triangle, coalescing rule, summary-only Item rule, backlog order; partial-failure layout |

**Adopted stance:** Combined critique is stronger than any single pass. Grok updated rather than defending the first per-file sketch; that update is part of the learning.

### 6.2 Consensus judgments

1. **Librarian validates batch-summary architecture**, not live concurrent progress.
2. **+LOC is acceptable** when presentation becomes explicit.
3. **Domain verbs** for Changes via `Record` when stock verbs are wrong.
4. **No evo English pluralization API** from one adoption.
5. **Vanity success Items** should go; condition Items stay.
6. **Summary-only failure Item is not redundant** without Tasks; becomes redundant if Tasks already carry the same condition/evidence.
7. **Conclusion coalescing** is human-projection only; model keeps separate objects.
8. **Per-file Tasks:** predeclare, adapter/callbacks, scale model by cardinality; product decision, not next implementation default.
9. **Next work:** taste + docs + coalescing design **before** Tasks-in-librarian.

---

## 7. House rules (adopted)

### 7.1 Architecture triangle (binding)

```text
broker / facade     → work and neutral progress facts (callbacks OK)
command / present   → Evo presentation only
-status / ResultWriter / app schemas → machine contracts
```

**Never** pass `*evo.Output` or `*evo.Task` into domain packages.

Preferred adapter shapes:

```go
type FileProgress interface {
    Phase(string)
    Bytes(completed, total int64)
    Done(summary string)
    Fail(summary string, err error)
}
// or PlaceCallbacks{ OnPhase, OnBytes, ... }
```

### 7.2 Changes verb house rule

```text
Added     — something was added
Removed   — something was removed
Moved     — something was moved
Updated   — something was updated
Reused    — something was reused as-is
Record    — use the actual domain verb otherwise
```

**Do not** force domain language into stock verbs:

```go
// Wrong
changes.Added(1, "files placed")

// Right (caller owns grammar)
changes.Record("placed", n, noun(n, "file", "files"))
changes.Record("offloaded", n, noun(n, "source", "sources"))
```

**Documentation detail:** the object string on a quantity-bearing record is the **final grammatical object**. Evo does not pluralize or append words.

Local helper (app-owned, not evo):

```go
func noun(n int, one, many string) string {
    if n == 1 {
        return one
    }
    return many
}
```

### 7.3 When an Item is warranted

| Keep Item when…                                                                             | Drop Item when…                                                        |
| ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| It names an independent **condition** (manifest valid, dest writable, offload verification) | It only announces that a Plan/Changes section exists                   |
| It carries severity + structured evidence not expressed elsewhere                           | It only repeats “command succeeded” after a successful Changes section |
| Summary-only mode needs a run gate + Fail/FailedBy                                          | Per-file Tasks collection already expresses the same failures          |

**Rule:** _An Item is redundant only when another model element already expresses the same condition and evidence._

### 7.4 Target librarian presentation (after polish)

**Dry-run (preferred):**

```text
Dry-run: no files will be written or deleted

[planned] librarian
  move    arr/radarr.tar.gz → …/backup/arr-config/radarr.tar.gz
  remove  1 source file after verified offload
```

- Keep safety prose line (product is safety-critical).
- No `✓ dry-run plan` vanity Item.
- No second `[planned] librarian` if coalescing applies.

**Live success (preferred):**

```text
[changed] librarian
  placed     1 file
  offloaded  1 source
```

**Live partial failure (preferred sketch):**

```text
✗  placement
   ├─ arr/radarr.tar.gz
   │  └─ destination was not writable
   └─ sonarr/config.zip
      └─ offload verification failed

[changed] librarian
  placed     7 files
  offloaded  6 sources

[failed] librarian
  2 files could not be completed
```

Here conclusion **must remain**: durable changes occurred **and** overall failure is a different dimension. Strong test case for multidimensional conclusions.

### 7.5 Conclusion coalescing (Evo projection — design rule)

**Not** model collapse. Snapshot retains independent Conclusion and Plan/Changes.

**Human rule (draft for implementation):**

> Suppress the standalone human conclusion band when **exactly one** visible Plan or Changes section has the **same normalized subject** and a **compatible state** as the conclusion, **and** the conclusion contributes **no additional visible information** (no extra severity, problems, next actions, or secondary tags).

**Compatible examples (suppress trailing duplicate):**

```text
[changed] librarian
  placed     1 file
  offloaded  1 source
```

(no second bare `[changed] librarian`)

**Must preserve conclusion when it adds dimension:**

```text
[changed] librarian
  placed  18 files

[warning] librarian
  2 source files could not be removed
```

```text
[changed] librarian
  placed  17 files

[failed] librarian
  placement partially completed   # severity dimension; may map to existing failed state + copy
```

```text
[changed] files
[changed] manifest

[changed] librarian   # multi-section — do not collapse away the run conclusion without care
```

**Implementation notes for next phase:**

- Coalesce only the **trailing title band**, never the Changes/Plan body.
- Specify a state-compatibility matrix in tests (ready/changed/planned/blocked/failed/warn).
- Multi-section runs: default **no** aggressive coalesce.
- Do not invent a new conclusion enum value lightly; prefer existing states + clear copy.

### 7.6 slog vs human failure

```text
slog          → implementation diagnostics (paths, tiers, raw errors)
Item/Task     → user-facing condition + structured Problems
Detail        → stable guidance / tool tail
Cause         → diagnostic error attachment (not primary human prose by default)
```

Summary-only failure pattern (direction):

```go
placement := out.Item("placement", evo.ID("run.placement"))
if len(summary.Failures) == 0 {
    placement.OK()
} else {
    placement.FailedBy(problemsFrom(summary.Failures)...)
}
// Changes only for effects that actually occurred
```

---

## 8. Per-file progress — corrected pattern (product optional)

### 8.1 Vocabulary (correct)

```text
Tasks   = collection of independent work units
Task    = one file operation
Bytes   = absolute copy progress
Progress = absolute count progress
Phase   = hardlink / non-metered work
Plan    = dry-run only (no simulated Tasks)
Changes = final durable effects
```

### 8.2 Hard rules if product ever needs it

1. **Predeclare Tasks** in semantic order **before** workers start.
   Concurrent `jobs.Task(...)` inside workers ⇒ nondeterministic declaration order (live rows, snapshots, tests).

2. **Workers only mutate** existing handles (`Phase` / `Bytes` / `Done` / `Fail`).

3. **Adapter boundary** — domain gets interfaces/callbacks, not evo types.

4. **Hardlink** — Phase + Done; do not fake multi-step Bytes.

5. **Copy** — Bytes only when the facade can report progress; else start/end absolute updates.

6. **Scale the model with cardinality** — do not use “in-flight-only Tasks” as the semantic model to save rows; viewport policy is the renderer’s job.

| Workload     | Appropriate model                                      |
| ------------ | ------------------------------------------------------ |
| Small batch  | One predeclared Task per file                          |
| Medium batch | Aggregate count Task + optional active large transfers |
| Huge batch   | Aggregate Progress + bounded failure evidence          |
| Dry-run      | Plan, not Tasks                                        |
| Completion   | Changes of durable effects                             |

7. **Do not duplicate** Tasks-collection failure with a second Item named “placement” unless the Item is an **independent** post-condition (e.g. offload verification).

8. **Capability ≠ obligation.** Librarian may correctly stay summary-only if placements are fast/hardlink-heavy.

### 8.3 Corrected sketch (reference only — not current librarian implementation)

```go
jobs := out.Tasks("placement")
tracked := predeclarePlacementTasks(jobs, sortedFiles)

var wg sync.WaitGroup
for _, entry := range tracked {
    entry := entry
    wg.Add(1)
    go func() {
        defer wg.Done()
        placeFile(entry.File, entry.Callbacks) // domain-facing callbacks
    }()
}
wg.Wait()

changes := out.Changes("librarian")
changes.Record("placed", summary.Placed, noun(summary.Placed, "file", "files"))
changes.Record("offloaded", summary.Offloaded, noun(summary.Offloaded, "source", "sources"))
// Independent post-gate Item only if needed
```

---

## 9. Evo ordinary surface vs studio appendix

### 9.1 Ordinary (teach first)

- `New` / `New(Config)` / `DefaultConfig` / `Delay`
- `Main`
- `Print` / `Printf` / `Println` / `Verbose`
- `Item` / `Task` / `Tasks` / `Changes` / `Plan`
- Problem options: `Detail`, `Cause`, `ID`, …
- `Task.Capture` / `Item.Capture`
- `SlogHandler()`
- `ResultWriter` under `FormatData`
- Narrow `Scope` for namespaced IDs

### 9.2 Studio / advanced (do not lead with)

- `NewWithOptions`, `Title` option, custom Terminal, Clock
- `Progress64`, `Advance`
- Session `Output.Capture`
- Direct debug-record APIs, DebugWriter as integration fallback
- Fixed dimensions / testkit surfaces

### 9.3 Removed or intentionally gone (pre-v1 honesty)

Examples: `For`, `NewWithConfig`, `WriterOptions`, `Line`/`Linef`, field-discarding Info/WarnMessage/ErrorMessage, `ItemSpec`/`ItemWith`, Capture alias pairs, Scope.Writer/SlogHandler/Capture pass-throughs.

---

## 10. Documentation & pedagogy lessons

### 10.1 Examples ladder (direction)

Keep boring, correct examples:

1. print → verbose
2. repo-status (Items, BlockedBy when truly plural)
3. install-pipeline (Task.Capture, Cause, DetailTail)
4. migrate (Cause raw + Detail guidance)
5. doctor (verbose flag, Item.Capture for tool-backed gate)
6. data-command (ResultWriter purity)
7. scope-plugin (keys visible; Scope not isolation)
8. live-progress / debug-\* (one log conductor)

### 10.2 Case study requirement (next phase artifact)

After librarian presentation polish, write a **source-grounded adoption case study** in Evo docs:

- Before: slog-as-UI
- After: present boundary
- Mistakes: stock verbs, vanity Items, duplicate conclusions, slog-only failures
- Explicit non-claims (what was not validated)
- Metrics that matter (not raw LOC apology)

Tone: forensic, not praise.

### 10.3 Main / process ownership (ecosystem note)

Libraries generally return status; apps call `os.Exit`. `Main` is convenient sugar. Document dual recipes: tiny tools use Main; embedders use Finish + host exit. Do not grow Main into Cobra.

---

## 11. Prioritized next phase

Ordered for leverage and honesty.

### Phase P0 — Librarian presentation polish (app)

1. Replace `Added`/`Removed` shoehorns with domain `Record` + local `noun`.
2. Remove vanity success Items (`dry-run plan`; reconsider bare `placement` OK).
3. On failure: accumulate structured failures → `FailedBy` / Problem subjects (paths).
4. Keep safety dry-run prose line.
5. Keep `-status` JSON contract unchanged.
6. Re-run dry-run/live demos; capture goldens if useful.

### Phase P1 — Evo projection coalescing (library)

1. Spec the narrow rule (subject match, state matrix, no extra conclusion info).
2. Implement human-only suppression of trailing duplicate conclusion band.
3. Tests: single Changes ok; partial fail keep conclusion; multi Changes; Plan dry-run; NextCommand present.
4. Do not change snapshot schema to “merge” objects.

### Phase P2 — Evo documentation

1. Changes verb house rule + Record guidance.
2. Concurrent Tasks: predeclare; adapter boundary.
3. Failure not slog-only.
4. Scope honesty (already partially documented).
5. Librarian adoption case study (after P0).
6. Model-by-scale table for progress cardinality.

### Phase P3 — Optional product (librarian)

Only if operators need confidence during long copies:

- Aggregate `Task.Progress(filesDone, filesTotal)` first.
- Per-file Tasks only for small batches or active large transfers.
- Facade progress callbacks; no evo import in broker.

### Phase P4 — Explicitly not now

- Evo-owned English pluralization
- In-flight-only semantic Task model
- RunAll / execution framework features
- Forcing per-file UI on hardlink-heavy librarian

---

## 12. Measurement rubric for “successful adoption”

Use this checklist instead of “diff size”:

| Criterion                               | Librarian?                          |
| --------------------------------------- | ----------------------------------- |
| Presentation decisions centralized      | Yes (`present.go`)                  |
| Domain packages free of evo             | Yes                                 |
| Machine contract preserved              | Yes (`-status`)                     |
| Worker happy-path noise removed         | Yes                                 |
| Dry-run tense explicit (Plan)           | Yes (needs taste polish)            |
| Live durable effects explicit (Changes) | Yes (needs domain verbs)            |
| Failure severity explicit               | Partial (needs structured evidence) |
| Concurrent live Tasks                   | Not attempted (correct)             |

---

## 13. Open questions for refinement phase

Refinement should resolve or park these deliberately:

1. **Coalescing matrix** — exact conclusion states that are “compatible” with `[changed]` / `[planned]`.
2. **Partial success copy** — “partially completed” as failed vs ready+warn vs new state (prefer existing states).
3. **Whether dry-run prose is universal evo guidance** or app-specific (recommend: app/product).
4. **Default visibility of Cause** in human projection for summary tools.
5. **How many failure Problems** to show before “and N more” truncation.
6. **Case study location** — `docs/adoption/librarian.md` vs README section vs external.
7. **Whether Main remains the taught opening** for library-only consumers embedding under other runners.
8. **Snapshot consumers** — any external tools already keying off conclusion chrome that coalescing would break? (None known for librarian.)

---

## 14. One-page executive summary

**Evo** is a presentation library: small ordinary ladder, strict honesty (marks must perform), jazz spelling (one ordinary glyph per intent; keep true voicings). Through **v0.2.8**, honesty and capture/slog/config/resize work landed; sugar sprawl and false APIs were cut.

**Librarian** proved the batch-summary adoption story: domain stayed pure, status JSON stayed, presentation became a named layer. That is more valuable than LOC reduction (net ~+95 lines of _owned_ UI). It did **not** prove live concurrent progress.

**Critique synthesis** corrected intermediate mistakes: stock Changes verbs misused; vanity Items; duplicate conclusions; slog-only failures; concurrent Task declaration; conflating viewport with model.

**Next phase:** polish librarian present.go (verbs, Items, FailedBy) → design/test narrow conclusion coalescing in evo → document house rules and an honest adoption case study → only then consider aggregate/per-file progress if product demands it.

---

## 15. Source anchors (for refinement authors)

| Topic                       | Where                                                                      |
| --------------------------- | -------------------------------------------------------------------------- |
| Evo library                 | `~/Developer/Personal/evident-output` @ v0.2.8                             |
| Librarian                   | `~/Developer/Personal/librarian` @ `8eae112` (local)                       |
| Construct / Config / Delay  | `construct.go`                                                             |
| Capture / Item.Capture      | `capture.go`                                                               |
| Scope                       | `entity.go`                                                                |
| slog                        | `slog.go`                                                                  |
| SIGWINCH                    | `terminal/resize_unix.go`, `live.go`                                       |
| Librarian present           | `cmd/librarian/present.go`                                                 |
| Librarian place             | `cmd/librarian/run.go`                                                     |
| Prior adversarial DX review | `~/Downloads/evident-output-adversarial-dx-review-v0.2.6.md` (if retained) |

---

## 16. Document history

| Date       | Event                                                                                                                                                                                             |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-07-28 | Initial comprehensive synthesis from three conversational strands (artistic DX, platform honesty through v0.2.8, librarian adoption + multi-party critique). Written for refinement → next phase. |

---

_End of polish synthesis. Refine in place or branch a `polish-v2.md`; do not scatter decisions across chat logs._
