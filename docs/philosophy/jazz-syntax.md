# Jazz syntax

Binding rules for Evident Output’s public API surface.
Source: `docs/roadmap/implementation-basis.md` §4 (PHIL-001…005, PHIL-007), §15.

These rules govern what ships on the ordinary lead sheet. Agents and humans both treat them as constraints, not taste notes.

---

## PHIL-001 — Same note, same spelling

One ordinary public API per domain intent.

Mechanical, historical, or type-system variants must not compete with the ordinary form.

**Test:** If two names express the same intent to a caller, one is sugar or debt. Remove, deprecate before v1, or quarantine on an advanced surface.

### Rejected (duplicate spelling)

```go
evo.For("install")
evo.New(evo.Config{Title: "install"})
```

```go
task.Progress(done, total)
task.Progress64(done64, total64) // same intent, second spelling
```

```go
evo.KeepLastLines(100)
evo.CaptureLines(100) // same intent, second spelling
```

### Accepted (one ordinary form)

```go
out := evo.New(evo.Config{Title: "install"})
task.Progress(done, total)
// Capture lines via the ordinary Capture surface, not a parallel alias
```

---

## PHIL-002 — Different voicing is not duplication

Related APIs stay separate when they preserve materially different domain information.

**Test:** If collapsing two APIs loses a real domain distinction a product needs, keep both.

### Accepted (different voicing)

```go
item.Block("local changes")
item.BlockedBy(
    evo.Problem{Subject: "branch-a", Summary: "local-only"},
    evo.Problem{Subject: "branch-b", Summary: "ahead of origin"},
)
```

```go
task.Progress(3, 10)   // unit count
task.Bytes(24<<20, 80<<20) // byte progress — different measure
```

```go
out.Item("credentials")  // condition
out.Task("authenticate") // work
```

### Rejected (false consolidation)

Flattening `Block` into `BlockedBy` with a single synthetic Problem “for consistency,” or forcing every progress call through one generic `ProgressAny` that erases unit vs bytes.

Aesthetic consistency must not flatten domain meaning.

---

## PHIL-003 — Studio overdub stays out of the lead sheet

Advanced capabilities may exist without appearing in ordinary examples.

**Test:** Ordinary docs and the learning ladder show the minimal canonical surface first. Studio knobs live in an appendix / advanced package, not the first page.

### Studio (allowed, not ordinary)

- `NewWithOptions`
- custom terminal drivers
- deterministic clocks
- relative `Advance`
- generic 64-bit count progress
- session-level Capture
- direct debug-record APIs
- fixed terminal dimensions
- testkit-only hooks

### Lead sheet (ordinary)

```text
New(Config) → Main → Print/Verbose → Item/Task/Tasks
→ Capture on Item or Task → Plan/Changes → slog → ResultWriter
```

---

## PHIL-004 — Marble subtraction

Before adding an API, answer yes to at least one:

1. Does it express a **new domain concept**?
2. Does it **remove repeated real-world ceremony**?
3. Does it **prevent a common correctness failure**?
4. Could the same result be achieved by **documentation, projection policy, or an application adapter**? (If yes → do that instead.)

New symmetry alone is not justification. Adding a method “because its sibling has one” is rejected (see §15 non-goal: sugar merely for method symmetry).

---

## Sugar rule (adding aliases)

Sugar is allowed only when it:

- is the **one ordinary spelling** of an intent (not a second spelling of the same note), **or**
- is explicitly **advanced / testkit** and never shown as ordinary, **and**
- passes marble subtraction (PHIL-004).

Sugar is forbidden when it:

- competes with an existing ordinary form (PHIL-001),
- pretends to do more than it does (PHIL-007),
- exists only for method-table symmetry (§15).

| Candidate | Verdict |
|---|---|
| `New(Config{Title})` as sole constructor on the lead sheet | Accepted |
| `For(title)` alias of `New` | Rejected — same note, second spelling |
| `BlockedBy(...Problem)` beside singular `Block` | Accepted — plural evidence voicing |
| `Progress64` beside `Progress` for the same count intent | Rejected — type-system twin |
| `Record(verb, n, object)` for domain verbs | Accepted — real domain concept (see domain-vocabulary) |

---

## PHIL-005 — Instinct should play the right notes

Defaults, examples, and ownership make correct behavior the path of least resistance.

- Capture is silent on success.
- `Cause` is diagnostic; `Detail` is user-facing.
- `Main` reconciles application errors before final rendering.
- stdout data contracts remain uncontaminated.
- concurrent Task declaration order is deterministic.
- a human failure cannot exist only in `slog`.

If the “easy” call path produces dishonest or contaminated output, the API is wrong — not the caller.

---

## PHIL-007 — Honest notation is mandatory

A public field or method must perform the behavior its syntax implies.

False notation is a **release-blocking** design defect.

### Forbidden dishonesty

- a Config field that cannot represent explicit zero
- a method accepting fields that are discarded
- a Scope method that does not apply scope
- a struct exposing members that construction ignores
- a log handler advertising structure but flattening it

**Test:** Read the signature aloud. If a reasonable caller would expect behavior the implementation drops, fix the API or remove the field — never document around the lie.
