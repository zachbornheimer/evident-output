# Evident Output — Implementation Basis

**Date:** 2026-07-28
**Status:** Approved working basis for implementation planning
**Primary library baseline:** `evident-output` v0.2.8 (`dab378e`)
**Adoption baseline:** `librarian` local commits `3edd911` → `8eae112` (ahead of origin)
**Conversational pedigree:** `~/Downloads/polish.md` (synthesis); this file is the planning authority
**Repo check-in path (Phase 0):** `docs/roadmap/implementation-basis.md`
**Document role:** one authoritative basis from which implementation plans, design documents, issues, and release work are derived

---

## 1. Purpose

This document converts the artistic-DX review, platform-honesty work, Librarian adoption, and multi-party critique into one implementation-oriented source of truth.

It serves five purposes:

1. Define the design philosophy that implementation must preserve.
2. Record the decisions already made so they are not reopened casually.
3. Separate validated behavior from hypotheses and untested surfaces.
4. Define implementation workstreams, deliverables, dependencies, and acceptance criteria.
5. Authorize creation of smaller design-philosophy and decision documents as implementation deliverables.

This document is not itself the final task breakdown. It is the basis from which concrete issues, pull requests, and milestone plans are created.

**Code-grounding rule:** ordinary-ladder snippets in this document must match the exported API of the library baseline (or note “proposed”). Prefer symbols verified against `construct.go`, `run.go`, and `print.go`.

---

## 2. Executive directive

Evident Output should now be refined like a score, not expanded like a framework.

The next phase is primarily about:

- semantic honesty;
- projection polish;
- documentation of design philosophy;
- adoption guidance;
- bounded failure presentation;
- deterministic concurrent presentation;
- preserving one ordinary lead sheet and one clearly marked advanced appendix.

The next phase is not primarily about adding more public concepts.

The implementation north star is:

> Every public mark performs what it promises, each domain intent has one ordinary spelling, true semantic distinctions remain audible, and the correct call site is the easiest one to write by instinct.

---

## 3. Authority and document hierarchy

This document is the planning basis. It may direct creation of narrower documents with longer lifetimes.

### 3.1 Documents to be created from this basis

| Document                                   | Purpose                                                  | Expected lifetime          |
| ------------------------------------------ | -------------------------------------------------------- | -------------------------- |
| `docs/philosophy/jazz-syntax.md`           | Binding syntax and subtraction philosophy                | Long-lived                 |
| `docs/philosophy/presentation-boundary.md` | Presentation ≠ execution; human vs machine streams       | Long-lived                 |
| `docs/philosophy/domain-vocabulary.md`     | Item, Task, Tasks, Plan, Changes, Problem, Cause, Detail | Long-lived                 |
| `docs/decisions/conclusion-coalescing.md`  | Exact projection decision and edge cases                 | Until superseded by ADR    |
| `docs/decisions/progress-cardinality.md`   | Per-item vs aggregate progress model                     | Long-lived                 |
| `docs/adoption/librarian.md`               | Reproducible adoption case study                         | Historical and educational |
| `docs/roadmap/polish-phase.md`             | Concrete milestone/task sequencing                       | Temporary                  |
| `docs/guides/large-platform-adoption.md`   | Docker/npm/Homebrew-style platform integration           | Long-lived                 |

These documents are implementation deliverables, not competing sources of truth. They must trace back to decisions in this document.

### 3.2 Change discipline

A pull request that changes a binding decision in this document must:

1. identify the affected decision ID;
2. explain why the previous decision is insufficient;
3. update the relevant philosophy or ADR document;
4. add or update observable tests where behavior changes;
5. preserve the distinction between model changes and projection changes.

---

## 4. Binding design philosophy

### PHIL-001 — Same note, same spelling

One ordinary public API should exist for each domain intent.

Mechanical, historical, or type-system variants must not compete with the ordinary form.

Examples of genuine duplicate spelling:

```go
evo.For("install")
evo.New(evo.Config{Title: "install"})
```

```go
task.Progress(done, total)
task.Progress64(done64, total64)
```

```go
evo.KeepLastLines(100)
evo.CaptureLines(100)
```

Duplicate spellings should be removed, deprecated before v1, or moved to an explicitly advanced surface.

### PHIL-002 — Different voicing is not duplication

Related APIs remain separate when they preserve materially different domain information.

Examples:

```go
item.Block("local changes")
```

```go
item.BlockedBy(
    evo.Problem{Subject: "branch-a", Summary: "local-only"},
    evo.Problem{Subject: "branch-b", Summary: "ahead of origin"},
)
```

```go
task.Progress(3, 10)
task.Bytes(24<<20, 80<<20)
```

```go
out.Item("credentials")
out.Task("authenticate")
```

Aesthetic consistency must not flatten domain meaning.

### PHIL-003 — Studio overdub stays out of the lead sheet

Advanced capabilities may exist without appearing in ordinary examples.

Examples:

- `NewWithOptions`;
- custom terminal drivers;
- deterministic clocks;
- relative `Advance`;
- generic 64-bit count progress;
- session-level Capture;
- direct debug-record APIs;
- fixed terminal dimensions;
- testkit-only hooks.

Ordinary documentation must show the minimal canonical surface first.

### PHIL-004 — Marble subtraction

Before adding an API, ask:

1. Does it express a new domain concept?
2. Does it remove repeated real-world ceremony?
3. Does it prevent a common correctness failure?
4. Could the same result be achieved by documentation, projection policy, or an application adapter?

New symmetry alone is not justification.

### PHIL-005 — Instinct should play the right notes

Defaults, examples, and API ownership should make correct behavior the path of least resistance.

Examples:

- Capture is silent on success.
- `Cause` is diagnostic; `Detail` is user-facing.
- `Main` reconciles application errors before final rendering.
- stdout data contracts remain uncontaminated.
- concurrent Task declaration order is deterministic.
- a human failure cannot exist only in `slog`.

### PHIL-006 — Presentation does not own execution

Evo does not own:

- goroutine scheduling;
- retries;
- command routing;
- cancellation policy;
- subprocess execution;
- business logic;
- worker pools;
- domain architecture.

Evo owns presentation state, rendering, evidence attachment, stream safety, and final conclusion.

### PHIL-007 — Honest notation is mandatory

A public field or method must perform the behavior its syntax implies.

False notation is a release-blocking design defect.

Examples of forbidden dishonesty:

- a Config field that cannot represent explicit zero;
- a method accepting fields that are discarded;
- a Scope method that does not apply scope;
- a struct exposing members that construction ignores;
- a log handler advertising structure but flattening it.

---

## 5. Canonical ordinary lead sheet

The ordinary public learning ladder is:

```text
New(Config)
→ Main
→ Print / Verbose
→ Item / Task / Tasks
→ Capture on Item or Task
→ Plan / Changes
→ slog through SlogHandler
→ ResultWriter or an external machine contract
```

### 5.1 Construction

```go
out := evo.New()
```

```go
out := evo.New(evo.Config{
    Title: "install",
})
```

```go
cfg := evo.DefaultConfig()
cfg.Title = "install"
cfg.Verbosity = evo.VerbosityVerbose
out := evo.New(cfg)
```

### 5.2 Standalone process boundary

```go
func main() {
    out := evo.New(evo.Config{Title: "install"})
    os.Exit(evo.MainWith(out, run))
}
```

`Main` is **ordinary convenience for standalone tools**, not a second product category.

Lifecycle (authoritative, matches `run.go`):

```text
run → reconcile run error into Fail if !AnyFailed → Finish → Close → exit code
```

It must not grow into a command framework, flag parser, or router. Hosted apps (Cobra, custom `main`, tests) use §5.3 and own `os.Exit`.

### 5.3 Hosted command boundary

Frameworks and larger hosts own process death and may own lifecycle explicitly. **Always `Close`**, not only `Finish` — `Main` does both; hosted code must too.

```go
func runCommand(cmd *cobra.Command) error {
    out := evo.New(configFromCommand(cmd))
    defer func() { _ = out.Close() }()

    runErr := run(cmd.Context(), out)
    if runErr != nil && !out.AnyFailed() {
        out.Fail("command failed", evo.Cause(runErr))
    }
    finishErr := out.Finish()
    // Map Conclusion().ExitCode to ExitCoder / error if the host needs it.
    return errors.Join(runErr, finishErr)
}
```

Optional: map `out.Conclusion().ExitCode` through the host’s exit-code mechanism. Do not call `os.Exit` inside library-owned packages.

### 5.4 Human prose

```go
out.Println("Reading configuration")
out.Printf("Found %d packages\n", count)
out.Verbose().Printf("Cache: %s\n", cacheDir)
```

### 5.5 Conditions

```go
item := out.Item("working tree", evo.ID("repo.working-tree"))

item.OK()
item.Warn("contains ignored files", evo.Detail("2 files"))
item.Block("contains local changes", evo.Detail("stash or commit them"))
item.Fail("could not inspect working tree", evo.Cause(err))
```

Plural structured evidence is used only when it is actually plural:

```go
branches.BlockedBy(problems...)
```

Tool-backed condition:

```go
captured := item.Capture()
```

### 5.6 Work

```go
task := out.Task("download", evo.ID("install.download"))

task.Doing("resolving")
task.Progress(done, total)
task.Bytes(written, size)
task.Done("downloaded")
task.Fail("download failed", evo.Cause(err))
```

Evidence:

```go
captured := task.Capture()
```

### 5.7 Concurrent work

```go
packages := out.Tasks("packages")

react := packages.Task("react", evo.ID("react"))
esbuild := packages.Task("esbuild", evo.ID("esbuild"))
```

Collection state is derived from child state.

### 5.8 Effects

```go
out.Changes("dependencies").
    Added(14, "packages").
    Removed(3, "packages")
```

```go
out.Plan("dependencies").
    Add(14, "packages").
    Remove(3, "packages")
```

Use `Record` when the domain verb is not a built-in verb.

### 5.9 Logging

```go
logger := slog.New(out.SlogHandler())
logger.Debug("cache lookup completed", "hit", true)
```

There is one logging threshold, derived from Output configuration.

### 5.10 Machine output

```go
json.NewEncoder(out.ResultWriter()).Encode(result)
```

or preserve an existing application-owned machine contract.

Human presentation must never contaminate raw result output.

---

## 6. Architecture boundaries

### ARCH-001 — Application triangle

```text
domain / broker / facade
    owns work and neutral progress facts

command / presentation adapter
    owns Evo entities and call sites

-status / ResultWriter / application schemas
    owns machine contracts
```

Domain and reusable execution packages must not import Evo.

Presentation-aware command orchestration may hold Evo handles.

Preferred neutral domain callback:

```go
type PlaceCallbacks struct {
    OnDoing func(string)
    OnBytes func(completed, total int64)
}
```

Command adapter:

```go
callbacks := PlaceCallbacks{
    OnDoing: task.Doing,
    OnBytes: task.Bytes,
}
```

### ARCH-002 — Stream ownership

- Human final output: configured presentation stream.
- Live progress and diagnostics: configured diagnostic stream.
- Raw application data: ResultWriter or application-owned writer.
- MCP stdout: protocol only.
- Debug and launcher diagnostics: stderr.

### ARCH-003 — Machine identity is not display text

Display labels may change. Stable IDs must not.

Coalescing, plugin namespacing, structured consumers, and snapshot comparisons must prefer semantic identity over normalized strings.

### ARCH-004 — Viewport policy is not model policy

The renderer may cap visible rows. The application should not create and destroy semantic Tasks merely to fit terminal height.

---

## 7. Validated claims and unvalidated surfaces

### 7.1 Validated by Librarian batch-summary adoption

- Config construction.
- Main lifecycle.
- Plan versus Changes tense.
- Summary-only Item as a run condition.
- `slog` as diagnostics instead of human UI.
- Preservation of an existing machine JSON contract.
- Presentation centralized after worker completion.
- Domain packages remain free of Evo.
- Existing concurrent worker architecture can coexist with summary-only Evo presentation.

### 7.2 Not validated by Librarian

- Live concurrent Task updates under load.
- Thousands of Tasks.
- Per-file Capture.
- Progress callbacks in the facade.
- Terminal resize during a long Librarian run.
- Debug journal interleaving with live Librarian Tasks.
- Cancellation mid-flight.
- Partial-change plus failure as production UX.
- Evo JSON as Librarian’s external machine contract.
- Plugin-scale namespaced structured consumption.

Claims and case-study copy must preserve this boundary.

---

## 8. Adopted house rules

### RULE-001 — Domain verbs over generic verbs

Use built-in Changes/Plan verbs only when they are the real domain verb.

```go
// Wrong
changes.Added(1, "files placed")
```

```go
// Right
changes.Record("placed", n, noun(n, "file", "files"))
changes.Record("offloaded", n, noun(n, "source", "sources"))
```

Evo does not own English pluralization.

The object passed to a quantity-bearing record is the final grammatical object.

### RULE-002 — No vanity Items

Keep an Item when it expresses an independent condition or carries severity/evidence not represented elsewhere.

Remove an Item when it only:

- announces that a Plan exists;
- repeats successful Changes;
- restates a Tasks collection’s derived failure;
- says the command succeeded without adding a condition.

A summary Item may represent the aggregated condition of work that was intentionally not modeled as Tasks.

### RULE-003 — User-actionable failure cannot be slog-only

`slog` carries implementation diagnostics.

Item/Task Problems carry user-facing failure meaning.

```go
placement := out.Item("placement", evo.ID("run.placement"))

if len(summary.Failures) == 0 {
    placement.OK()
} else {
    placement.FailedBy(problemsFrom(summary.Failures)...)
}
```

### RULE-004 — Predeclare concurrent Tasks

Declare Tasks in deterministic semantic order before starting workers.

Workers update handles; they do not declare presentation order concurrently.

```go
jobs := out.Tasks("placement")
tracked := predeclarePlacementTasks(jobs, sortedFiles)
```

### RULE-005 — Scale model cardinality to product need

| Workload     | Model                                                     |
| ------------ | --------------------------------------------------------- |
| Small batch  | One predeclared Task per operation                        |
| Medium batch | Aggregate count Task plus selected active large transfers |
| Huge batch   | Aggregate progress plus bounded failures                  |
| Dry-run      | Plan, never simulated Tasks                               |
| Completion   | Changes for durable effects                               |

### RULE-006 — Capability does not imply product obligation

Librarian may remain summary-only.

Per-file progress is added only when users need confidence during sufficiently long operations.

### RULE-007 — Present failure once; propagate errors according to application architecture

Both are valid:

```go
task.Fail("tests failed", evo.Cause(err))
return err
```

```go
task.Fail("one expected operation failed", evo.Cause(err))
return nil
```

Evo must not force application error policy.

---

## 9. Target Librarian presentation

### 9.1 Dry-run

```text
Dry-run: no files will be written or deleted

[planned] librarian
  move    arr/radarr.tar.gz → …/backup/arr-config/radarr.tar.gz
  remove  1 source file after verified offload
```

Requirements:

- keep the safety statement;
- remove the `dry-run plan` vanity Item;
- use domain-accurate verbs;
- suppress a duplicate trailing `[planned] librarian` only when coalescing rules permit.

### 9.2 Live success

```text
[changed] librarian
  placed     1 file
  offloaded  1 source
```

No generic success Item unless a distinct condition is being reported.

### 9.3 Live partial failure

```text
✗  placement
   ├─ arr/radarr.tar.gz
   │  └─ destination was not writable
   ├─ sonarr/config.zip
   │  └─ offload verification failed
   └─ and 37 more failures

[changed] librarian
  placed     7 files
  offloaded  6 sources

[failed] librarian
  39 files could not be completed
```

Requirements:

- retain Changes because durable effects occurred;
- retain Conclusion because failure adds a distinct dimension;
- bound human failure rows;
- disclose omitted failure count;
- preserve full or broader failure detail only in bounded structured snapshots, status output, or diagnostics according to policy.

---

## 10. Conclusion coalescing decision basis

### DEC-COAL-001 — Projection only

Conclusion and Plan/Changes remain independent in the model and structured output.

Coalescing affects only human rendering.

### DEC-COAL-002 — Semantic identity

Coalescing must not rely only on normalized display strings.

The implementation must establish semantic subject identity, such as:

- explicit subject IDs;
- shared primary subject identity;
- an internal reference from section to Output subject.

### DEC-COAL-003 — Narrow suppression rule

Suppress the standalone trailing conclusion band only when all conditions hold:

1. exactly one visible Plan or Changes section exists;
2. the section refers to the same semantic subject as the Conclusion;
3. section state and conclusion state are compatible;
4. the Conclusion contributes no additional visible information;
5. no extra severity, warning, partial state, cancellation, problems, action, or next command exists;
6. no multi-section ambiguity exists.

### DEC-COAL-004 — Preserve conclusion when it adds dimension

Preserve it for:

- changed plus warning;
- changed plus failure;
- partial completion;
- cancellation;
- blocked state;
- multiple effect sections;
- additional next actions;
- title/subject mismatch;
- mixed Plan and Changes.

### DEC-COAL-005 — Compatibility matrix required

An ADR and tests must define compatible state pairs explicitly.

String matching is insufficient.

**Draft matrix (must be finalized in ADR; not implementation until then):**

| Visible section tag         | Conclusion mood                      | Coalesce trailing conclusion? |
| --------------------------- | ------------------------------------ | ----------------------------- |
| `[changed]` only            | ready / changed, no extras           | Yes                           |
| `[planned]` only            | planned / ready, no extras           | Yes                           |
| `[changed]`                 | failed / blocked / warning extras    | No                            |
| any section                 | NextCommand / problems on conclusion | No                            |
| multiple Plan/Changes       | any                                  | No                            |
| Plan + Changes both visible | any                                  | No                            |
| subject ID mismatch         | any                                  | No                            |

“No extras” means no additional visible severity, partial-completion copy, cancellation, or next-action chrome.

---

## 11. Failure cardinality and bounded evidence

### DEC-FAIL-001 — Human rendering is bounded

Human output must not print unbounded Problem lists.

The renderer should prioritize and cap visible failures, preserving declaration or semantic order as specified.

### DEC-FAIL-002 — Omission is explicit

When failures are hidden:

```text
└─ and 37 more failures
```

The omitted count must be accurate.

### DEC-FAIL-003 — Structured retention is separately bounded

Structured snapshots may retain more failures than human output, but still require explicit limits to protect memory, privacy, and payload size.

### DEC-FAIL-004 — Redaction precedes retention

Failure subjects, details, Capture tails, diagnostics, and structured snapshots must pass through configured redaction before retention or rendering.

---

## 12. Implementation workstreams

## WS-1 — Design philosophy documents

**Goal:** make the aesthetic and architectural rules durable, concise, and reusable by humans and agents.

### Deliverables

1. `docs/philosophy/jazz-syntax.md`
2. `docs/philosophy/presentation-boundary.md`
3. `docs/philosophy/domain-vocabulary.md`
4. updates to the Agent Skill and MCP guidance referencing these documents

### Required content

`jazz-syntax.md`:

- same-note test;
- different-voicing test;
- studio-overdub test;
- marble subtraction;
- examples of accepted and rejected aliases;
- rule for adding sugar.

`presentation-boundary.md`:

- presentation ≠ execution;
- stdout/stderr/result ownership;
- Main versus hosted lifecycle;
- domain adapter boundaries;
- human versus machine contracts.

`domain-vocabulary.md`:

- Item, Task, Tasks;
- Warn, Block, Fail;
- Problem, Detail, Cause;
- Plan versus Changes;
- Capture ownership;
- aggregate summary Items;
- examples of domain-correct and domain-wrong usage.

### Acceptance criteria

- documents are checked into the repository;
- ordinary examples link to the relevant philosophy;
- Skill/MCP guidance references canonical sections rather than duplicating prose;
- a drift test or generated index verifies references remain valid.

---

## WS-2 — Librarian presentation polish

**Goal:** correct the real adoption output before using it as a case study.

### Tasks

1. Replace forced `Added`/`Removed` wording with domain `Record` verbs.
2. Add application-owned singular/plural helper.
3. Remove the dry-run vanity Item.
4. Reconsider the live `placement.OK()` Item; retain only if it represents an independent condition.
5. Accumulate structured failure records.
6. Convert failure summaries into Problems with path Subjects.
7. Bound visible failure rows.
8. Keep `-status` JSON unchanged.
9. Add deterministic golden outputs for:
   - dry-run;
   - live success;
   - live partial failure;
   - no-op/unchanged run;
   - fatal pre-run error.

### Acceptance criteria

- observed output matches target layouts in Section 9 **except** trailing conclusion-band coalescing (that is WS-3);
- vanity Items and forced domain verbs are gone (no `✓ dry-run plan`, no `added … files placed`);
- domain packages still do not import Evo;
- machine contract is byte-for-byte or schema-compatible according to existing tests;
- user-actionable failure appears in the final report (not slog-only);
- until WS-3 ships, a trailing `[planned]/`/`[changed]` conclusion band may still duplicate the Plan/Changes title — golden fixtures record current truth and mark the post-coalesce expectation.

---

## WS-3 — Conclusion coalescing

**Goal:** remove redundant human conclusion chrome without collapsing the model.

### Tasks

1. Create `docs/decisions/conclusion-coalescing.md`.
2. Define semantic subject identity.
3. Define the compatibility matrix.
4. Implement projection-only suppression.
5. Add fixtures for:
   - one matching Changes section;
   - one matching Plan section;
   - changed plus warning;
   - changed plus failure;
   - changed plus cancellation;
   - multiple Changes sections;
   - Plan plus Changes;
   - NextCommand present;
   - different subject IDs;
   - same label but different subject IDs;
   - no title.

### Acceptance criteria

- human duplicate bands are removed only in the narrow case;
- snapshots retain separate Conclusion and section objects;
- JSON/JSONL/event schemas do not collapse model objects;
- no string-normalization-only comparison is used as semantic identity;
- all fixtures pass across color/no-color, Unicode/ASCII, TTY/non-TTY profiles.

---

## WS-4 — Documentation and teaching ladder

**Goal:** make ordinary usage boringly correct.

### Tasks

1. Preserve the teaching order:
   - print;
   - verbose;
   - Items;
   - Task Capture;
   - Plan/Changes;
   - data output;
   - scopes;
   - live progress;
   - debug views;
   - advanced terminal driver.
2. Update Changes documentation with the domain-verb rule.
3. Document summary Item versus Tasks collection.
4. Document predeclared Tasks and deterministic ordering.
5. Document model-by-scale progress cardinality.
6. Document user-facing failure versus `slog`.
7. Document standalone `Main` and hosted explicit lifecycle separately.
8. Ensure examples never configure the same policy twice.
9. Add a “small combo versus platform” guide:
   - IDs optional for small human-only commands;
   - IDs required by platform policy for plugins and structured consumers.

### Acceptance criteria

- every documented snippet compiles;
- generated or tested snippets cannot drift;
- no deprecated API appears in ordinary docs;
- every advanced API is labeled as advanced;
- examples demonstrate behavior that is actually visible under their default flags.

---

## WS-5 — Librarian adoption case study

**Goal:** publish a forensic, reproducible adoption account.

### Prerequisites

- Librarian polish completed.
- Before and after commits are pushed or retained in an accessible patch.
- Golden outputs are committed.
- Evo version is pinned.

### Required sections

1. Application shape and existing architecture.
2. Before: slog-as-UI.
3. After: explicit presentation boundary.
4. What changed mechanically.
5. What required product/design judgment.
6. LOC and why it is not the primary metric.
7. Mistakes discovered:
   - forced verbs;
   - vanity Items;
   - duplicate conclusion;
   - thin failure evidence.
8. What was validated.
9. What was not validated.
10. Why per-file progress was deferred.
11. Final architecture.
12. Commands/tests used to reproduce results.

### Acceptance criteria

- all source links are repository-relative or commit URLs;
- no workstation-specific paths;
- claims are reproducible;
- tone is forensic, not promotional;
- no claim implies validation of untested live progress surfaces.

---

## WS-6 — Concurrent progress guidance and future validation

**Goal:** prepare correct architecture before any product adopts per-file live progress.

### Tasks

1. Document predeclaration pattern.
2. Provide neutral callback examples.
3. Add a deterministic concurrent Task fixture.
4. Add cardinality scenarios:
   - 10 Tasks;
   - 100 Tasks;
   - 10,000 operations represented as aggregate progress.
5. Verify viewport selection does not reorder semantic snapshots.
6. Verify debug logging during active progress.
7. Verify terminal resize during active progress.
8. Verify partial failures plus durable Changes.
9. Verify cancellation leaves explicit terminal states.

### Acceptance criteria

- concurrent declaration is not taught;
- task order is stable across repeated runs;
- domain packages in examples do not import Evo;
- renderer remains bounded;
- semantic model is independent of viewport selection.

This workstream does not require Librarian to adopt per-file progress.

---

## WS-7 — API hierarchy and pre-v1 subtraction

**Goal:** ensure the ordinary chart remains singular.

### Audit categories

1. constructors;
2. progress aliases;
3. Capture aliases;
4. direct debug methods;
5. legacy writer adapters;
6. formatted variants;
7. plural outcome methods;
8. advanced Options.

### Decision criteria

For each exported symbol:

- ordinary;
- advanced;
- compatibility-only;
- remove before v1;
- retain because it preserves distinct domain information.

### Acceptance criteria

- a generated/exported API inventory records category and rationale;
- ordinary docs use only ordinary symbols;
- compatibility-only symbols have a removal milestone;
- no public field or parameter is ignored;
- every plural or formatted variant has demonstrated call sites and tests.

---

## WS-8 — Quality gates and conformance

**Goal:** make design rules executable.

### Required tests

- conclusion-coalescing fixtures;
- bounded Problem rendering;
- semantic ID versus same-label tests;
- domain-record grammar goldens;
- deterministic concurrent Task order;
- human and structured model equivalence;
- TTY/non-TTY output;
- Unicode/ASCII;
- narrow widths;
- redaction-before-retention;
- capture and debug interleaving;
- no duplicate conclusion;
- no duplicate aggregate Item where collection already expresses failure;
- Skill/MCP guidance drift.

### CI

```text
go test ./...
go test -race ./...
go vet ./...
conformance suite
traceability checks
example smoke tests
generated-document drift checks
cross-platform builds
```

---

## 13. Implementation sequencing

### Phase 0 — Freeze and issue creation

1. Check in this document as `docs/roadmap/implementation-basis.md`.
2. Check in `polish.md` (or a slim link) as conversational pedigree under `docs/roadmap/` if retained.
3. Create issue labels: philosophy · projection · adoption · docs · conformance · advanced-api · librarian.
4. Create one tracking issue per workstream (WS-1…WS-8).
5. Record decision IDs (PHIL/RULE/DEC/OPEN) in issue descriptions.
6. Push or tag Librarian commits so WS-5 is not workstation-bound.

### Phase 1 — Philosophy and Librarian polish in parallel

**Track A:** WS-1 design philosophy documents.
**Track B:** WS-2 Librarian presentation polish.

Reason: philosophy stabilizes language while adoption fixes provide real examples and goldens (minus coalescing).

### Phase 2 — Coalescing design before code

1. Write the ADR (`docs/decisions/conclusion-coalescing.md`).
2. Resolve OPEN-001 and OPEN-002 (subject identity + compatibility matrix).
3. Write red fixtures first.
4. Implement projection-only suppression.
5. Run full profile matrix (TTY/non-TTY, color, Unicode/ASCII, narrow).

Coalescing must not begin as a renderer-only string patch.

### Phase 3 — Documentation and case study

1. Update teaching ladder (WS-4), incorporating WS-3 behavior.
2. Publish polished Librarian adoption case study (WS-5).
3. Add large-platform guide.
4. Update Skill and MCP guidance.

### Phase 4 — Concurrency validation

WS-6 fixtures and guidance for live concurrent progress without forcing a Librarian product change.

### Phase 5 — Pre-v1 surface audit

WS-7 + WS-8: classify/remove alternate charts; wire conformance and drift checks.

### Recommended first PR order (execution)

```text
0. Check in this basis (+ labels/issues)
1. librarian present.go polish (WS-2)          // fast product truth
2. philosophy docs (WS-1)                       // parallel with 1
3. coalescing ADR + red fixtures (WS-3 design)
4. coalescing implementation + tests (WS-3 code)
5. teaching ladder + case study (WS-4, WS-5)
6. concurrency fixtures (WS-6)
7. API inventory / subtraction (WS-7)
8. conformance gates (WS-8, continuous from 3–7)
```

---

## 14. Dependencies

| Workstream                | Depends on                                 |
| ------------------------- | ------------------------------------------ |
| WS-1 Philosophy docs      | This document                              |
| WS-2 Librarian polish     | Current v0.2.8 API                         |
| WS-3 Coalescing           | Subject identity decision; golden fixtures |
| WS-4 Teaching ladder      | WS-1 decisions; WS-3 behavior              |
| WS-5 Case study           | WS-2 completed and commits accessible      |
| WS-6 Concurrency guidance | Existing Task/Tasks renderer and tests     |
| WS-7 API subtraction      | WS-1 philosophy; exported API inventory    |
| WS-8 Conformance          | Every behavioral workstream                |

---

## 15. Explicit non-goals

The following are not part of this phase:

- Evo-owned English pluralization;
- in-flight-only semantic Task models;
- execution helpers such as RunAll or Parallel;
- forcing Librarian to use per-file Tasks;
- replacing Librarian’s existing status JSON;
- adding a new partial-success conclusion enum without exhausting existing state dimensions;
- aggressive multi-section conclusion collapse;
- string-only semantic identity;
- importing Evo into reusable domain packages;
- turning Main into Cobra or a command router;
- adding sugar merely for method symmetry.

---

## 16. Open decisions

These require explicit resolution or deferral. **Working recommendations** are non-binding until ADR or issue close.

| ID       | Decision                                 | Working recommendation                                                                                 |
| -------- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| OPEN-001 | Semantic subject identity for coalescing | Prefer Output subject / primary section ID linkage; never display-string alone                         |
| OPEN-002 | Conclusion compatibility matrix          | Tabulate Plan/Changes tag × Conclusion mood; only pure same-mood pairs coalesce                        |
| OPEN-003 | Human failure-row default limit          | Start at **5** visible Problems + “and N more” (tunable Config later if needed)                        |
| OPEN-004 | Structured Problem retention limit       | Higher than human (e.g. **50**); always redacted; document cap                                         |
| OPEN-005 | Partial-failure copy                     | Use existing **failed** (or blocked) conclusion + Changes of what happened; **no new enum** this phase |
| OPEN-006 | Dry-run safety prose                     | Application-specific; evo docs may show as optional pattern, not required chrome                       |
| OPEN-007 | Cause in human summary                   | Default: Cause stays diagnostic; Detail is human; tools may expose Cause in verbose/debug              |
| OPEN-008 | Advanced exports for v1                  | Inventory first (WS-7); no removals without replacement path                                           |
| OPEN-009 | `Progress64`                             | **Advanced / retain** until proven unused; ordinary path is `Progress(int)` + `Bytes`                  |
| OPEN-010 | Plural outcome variants                  | Keep `BlockedBy` / `FailedBy` / `WarnedBy` as genuine plural voicing (PHIL-002); audit others          |
| OPEN-011 | Case-study permanent URLs                | Push Librarian polish branch; link commit SHAs in evo docs                                             |
| OPEN-012 | Scope expansion                          | **Entity-only for this phase** (Item/Task/Tasks); no scoped slog/Writer                                |

Each open decision must be closed by an ADR, an explicit deferral, or removal from scope.

---

## 17. Measurement rubric

Successful adoption is not measured primarily by diff size.

### Architecture

| Criterion                                     |
| --------------------------------------------- |
| Presentation decisions centralized            |
| Reusable domain packages remain free of Evo   |
| Existing machine contracts preserved          |
| Execution policy remains application-owned    |
| Diagnostics separated from human presentation |

### User-facing quality

| Criterion                                                         |
| ----------------------------------------------------------------- |
| Failure can be identified without filtering logs                  |
| Output does not repeat the same conclusion                        |
| Displayed verbs match the domain action                           |
| Success, warning, block, partial change, and failure are scanable |
| Non-TTY output is complete and free of terminal controls          |
| Output order is deterministic                                     |
| Failure evidence is bounded and actionable                        |
| Dry-run clearly communicates non-mutation                         |
| Machine output remains uncontaminated                             |

### Library quality

| Criterion                                                |
| -------------------------------------------------------- |
| Ordinary API has one spelling per intent                 |
| Advanced APIs do not compete in ordinary docs            |
| Every public mark performs                               |
| Model and projection are not conflated                   |
| Human and structured outputs preserve equivalent meaning |
| Tests enforce design decisions                           |

---

## 18. Definition of done for this phase

This polish phase is complete when:

1. Design-philosophy documents are checked in and referenced by Skill/MCP guidance.
2. Librarian output uses domain verbs, no vanity Items, and structured bounded failure evidence.
3. Librarian before/after commits and golden outputs are accessible.
4. Conclusion coalescing has an ADR, semantic subject identity, tests, and a human-only implementation.
5. Structured snapshots retain independent sections and Conclusion.
6. Documentation teaches predeclared Tasks and model-by-scale cardinality.
7. A forensic Librarian case study is published.
8. Exported API inventory classifies ordinary, advanced, compatibility, and removal candidates.
9. CI runs all new behavior and drift checks.
10. No phase non-goal has been added accidentally.

---

## 19. Immediate next actions

1. Check in this document as the implementation basis.
2. Create the three design-philosophy documents from WS-1.
3. Open the Librarian polish branch and implement WS-2.
4. Push or otherwise preserve Librarian before/after commits.
5. Draft the conclusion-coalescing ADR with red fixtures.
6. Add golden outputs for the target Librarian presentations.
7. Create the exported API inventory.
8. Convert each workstream into milestone issues.

---

## 20. Source baselines

Use repository-relative paths and immutable commit links in implementation artifacts.

Current planning baselines:

```text
Evident Output: github.com/zachbornheimer/evident-output @ dab378e
Librarian:      local 3edd911 → 8eae112 until pushed
```

Workstation-specific paths must not appear in published documentation.

---

## 21. Document history

| Date       | Event                                                                                                                                                   |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-07-28 | Initial implementation-basis rewrite from the comprehensive polish synthesis                                                                            |
| 2026-07-28 | Added design-philosophy documents as an explicit implementation workstream                                                                              |
| 2026-07-28 | Revision A: code-ground Verbosity/Main/Close; WS-2 vs WS-3 acceptance split; OPEN working recommendations; execution PR order; pedigree + check-in path |

---

## 22. Revision A change log (this edit)

| Fix                                                | Why                                               |
| -------------------------------------------------- | ------------------------------------------------- |
| `VerbosityVerbose` not `Verbose`                   | Matches exported API in `construct.go`            |
| Hosted lifecycle includes Fail reconcile + `Close` | Parity with `Main` (`run.go`); Close was missing  |
| Main lifecycle spelled out                         | Prevents “Finish-only” hosted recipes             |
| WS-2 acceptance does not require coalescing        | Avoids blocking librarian polish on WS-3          |
| OPEN-001…012 working recommendations               | Unblock implementers without silent re-litigation |
| Explicit first-PR order                            | Turns sequencing into an executable queue         |
| Pedigree + `docs/roadmap/implementation-basis.md`  | Clear authority and check-in location             |
| Code-grounding rule for snippets                   | Stops basis drift from becoming false notation    |

---

_This document is deliberately comprehensive because it is the planning basis. Implementation should produce smaller philosophy, decision, adoption, and roadmap artifacts while preserving traceability back to this source._
