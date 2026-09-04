# Presentation boundary

Binding rules for what Evident Output owns versus what the application owns.
Source: `docs/roadmap/implementation-basis.md` §4 (PHIL-006), §5.2–5.3, §6 (ARCH-001…004), §15.

Cross-links: [jazz-syntax.md](./jazz-syntax.md) · [domain-vocabulary.md](./domain-vocabulary.md)

---

## PHIL-006 — Presentation does not own execution

Evo does **not** own:

- goroutine scheduling
- retries
- command routing
- cancellation policy
- subprocess execution
- business logic
- worker pools
- domain architecture

Evo **does** own:

- presentation state
- rendering
- evidence attachment
- stream safety
- final conclusion

**Test:** If a proposed API schedules work, routes commands, or encodes domain policy, it belongs in the application — not in `evo`.

§15 non-goals reinforce this: no execution helpers such as `RunAll` / `Parallel`; do not turn `Main` into Cobra or a command router; do not import Evo into reusable domain packages.

---

## Application triangle (ARCH-001)

```text
domain / broker / facade
    owns work and neutral progress facts

command / presentation adapter
    owns Evo entities and call sites

-status / ResultWriter / application schemas
    owns machine contracts
```

Domain and reusable execution packages **must not import Evo**.

Presentation-aware command orchestration **may** hold Evo handles.

### Preferred neutral domain callback

```go
type PlaceCallbacks struct {
    OnPhase func(string)
    OnBytes func(completed, total int64)
}
```

### Command adapter wires Evo

```go
callbacks := PlaceCallbacks{
    OnPhase: task.Phase,
    OnBytes: task.Bytes,
}
```

Domain emits facts. Adapter maps facts onto Items/Tasks. Machine contracts stay on their own writers.

---

## Stream ownership (ARCH-002)

| Stream                        | Owns                                       |
| ----------------------------- | ------------------------------------------ |
| Human final output            | Configured **presentation** stream         |
| Live progress and diagnostics | Configured **diagnostic** stream           |
| Raw application data          | `ResultWriter` or application-owned writer |
| MCP stdout                    | Protocol only                              |
| Debug / launcher diagnostics  | stderr                                     |

Human presentation must never contaminate raw result output.

```go
json.NewEncoder(out.ResultWriter()).Encode(result)
// or preserve an existing application-owned machine contract
```

---

## Main vs hosted Finish+Close

### Standalone process — `Main` (ordinary convenience)

```go
func main() {
    out := evo.New(evo.Config{Title: "install"})
    os.Exit(evo.MainWith(out, run))
}
```

`Main` is ordinary convenience for standalone tools, **not** a second product category and **not** a framework.

Lifecycle (authoritative; matches `run.go`):

```text
run → reconcile run error into Fail if !AnyFailed → Finish → Close → exit code
```

### Hosted command boundary

Frameworks and larger hosts own process death. They may own lifecycle explicitly.

**Always `Close`, not only `Finish`.** `Main` does both; hosted code must too.

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

Do **not** call `os.Exit` inside library-owned packages. Optional: map `out.Conclusion().ExitCode` through the host’s exit-code mechanism.

---

## Human vs machine contracts

| Concern          | Surface                                                                |
| ---------------- | ---------------------------------------------------------------------- |
| Human prose      | `Print` / `Verbose` / rendered Items, Tasks, Plan, Changes, conclusion |
| Diagnostics      | `slog` via `SlogHandler` (implementation detail, not human UI)         |
| Machine identity | Stable IDs — not display labels (ARCH-003)                             |
| Machine payload  | `ResultWriter` or app-owned schema; not the human render               |

Display labels may change. Stable IDs must not. Coalescing, plugin namespacing, structured consumers, and snapshot comparisons prefer **semantic identity** over normalized strings (ARCH-003). §15: no string-only semantic identity.

### Viewport is not model (ARCH-004)

The renderer may cap visible rows. The application must not create and destroy semantic Tasks merely to fit terminal height.

---

## Domain adapter boundary — checklist

| Question                                              | If yes                                        |
| ----------------------------------------------------- | --------------------------------------------- |
| Does this package run work reusable outside this CLI? | No `evo` import                               |
| Does this package decide how humans see state?        | Presentation adapter; Evo OK                  |
| Is this JSON for tools/agents?                        | ResultWriter / app schema; never human chrome |
| Am I about to add retries inside Evo?                 | Stop — application owns execution (PHIL-006)  |

---

## Accepted vs rejected

| Pattern                                                                               | Verdict                                   |
| ------------------------------------------------------------------------------------- | ----------------------------------------- |
| Domain facade with `OnPhase` / `OnBytes`; command maps to `task.Phase` / `task.Bytes` | Accepted                                  |
| Domain package imports `evo` and calls `out.Task`                                     | Rejected                                  |
| `Main` for a tiny standalone binary                                                   | Accepted                                  |
| Growing `Main` into flag parsing / subcommand routing                                 | Rejected (§15)                            |
| Hosted: `defer Close` + `Finish` + map exit code                                      | Accepted                                  |
| Hosted: only `Finish`, leak resources / skip Close                                    | Rejected                                  |
| Human failure only in `slog`, no Item/Task Problem                                    | Rejected (see domain-vocabulary RULE-003) |
| Machine JSON on `ResultWriter`, human summary on presentation stream                  | Accepted                                  |
| Mixing progress chrome into machine stdout                                            | Rejected                                  |
