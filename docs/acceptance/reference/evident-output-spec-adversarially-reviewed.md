# Evident Output — consolidated execution, verification, UI, machine API, and MCP contract

Status: **adversarially reviewed implementation specification — 2026-09-15**

Repository: `github.com/zachbornheimer/evident-output`

This document is normative for the next implementation pass and incorporates an adversarial completeness review. It consolidates the desired developer API, scheduler semantics, verification model, human UI, JSON/JSONL contract, migration rules, testing requirements, and MCP guidance.

It intentionally keeps the beginner vocabulary small.

---

# 0. Adversarial implementation handoff

This revision is written to be implementable without the conversation that produced it.

The implementation MUST NOT guess at the following points; this document resolves them:

- **Task identity:** manifest reuse requires a stable Task key. Evo derives one automatically and provides an advanced explicit override.
- **Definition trust:** a callback-defined Task cannot be whole-Task skipped merely because yesterday's outputs still exist — even when the application binary is unchanged. Runtime closure values, arguments, branches, configuration, or other undeclared inputs may differ. The application fingerprint is a conservative definition-identity signal and invalidator, **not positive Evidence by itself**.
- **Operation freshness:** Evo-native operations such as `evo.File` and `evo.Exec` can independently avoid expensive work after Evo has entered `Define`, because their **current invocation** supplies the semantic specification that can be compared with tracked outputs and `Basis`.
- **Freshness vs scheduling:** provenance may delay a known consumer until a known producer has been revalidated, but first-run execution dependencies still come from `Sequence` / `After`. Manifest knowledge must never make a first-run race look safe.
- **Output projection:** output format and verbosity are top-level policy. Call sites record semantic truth; they never hand-build JSON, ANSI, glyphs, verbosity strings, or skip wording.

The safety rule for every optimization is:

> **A false invalidation is acceptable. A false "already satisfied" result is not.**

When Evo lacks proof, it runs or revalidates work. It never guesses that work is current.

The phrase **automatic** in this document means:

```text
application code records semantic intent once
→ Evo derives scheduling state, progress presentation, evidence, provenance,
  effects, human output, JSON/JSONL, and exit behavior from that same model
```

It does **not** mean Evo can infer undeclared semantic dependencies from arbitrary Go code.

---

# 1. Product thesis

Evident Output should let application code describe work once and get, from the same runtime truth:

- scheduling and dependency handling;
- lifecycle state and cancellation;
- declarative state establishment for common resources;
- automatic tracking of results that Evo establishes;
- provenance-aware reconciliation and safe automatic skipping;
- progress, activity, warnings, facts, effects, and partial-failure truth;
- automatic TTY/plain projection with row-budget-aware verbosity;
- automatic final JSON / streaming JSONL projection once a top-level format is selected;
- stable HTTP/API serialization from the same wire contract;
- stable exit behavior;
- debugging and optimization data.

The beginner vocabulary stays small:

```text
Task       one atomic unit of work
Group      independent Tasks
Sequence   ordered Tasks
After      exceptional explicit dependency
Define     the work that may need to run
File       declaratively establish and track file state
Exec       execute/track an external process when its outputs are declared
Basis      semantic inputs whose fingerprints determine freshness
Evidence   the boolean answer: is this Task already satisfied now?
Fact       information learned
Effect     what would change / did change
```

`Verify` is the advanced read-only escape hatch for domains Evo cannot track automatically. It is deliberately not part of the beginner vocabulary.

Evidence is not inherently a named attribute such as `contents` or `permissions`. Those may exist as verification details for reporting, but Task-level Evidence is the boolean conclusion Evo derives.

Do not add a public `TaskConfig` authoring path yet.

## 1.1 Runtime construction and top-level ownership

The package keeps the existing package-default convenience model, but the run boundary is explicit. Normative vNext signatures are:

```go
type RunFunc func(context.Context) error

func Init(config ...Config) *Output
func Run(ctx context.Context, run RunFunc) Result
func Main(run RunFunc) int
func (o *Output) Run(ctx context.Context, run RunFunc) Result
```

Minimum `Config` fields relevant to this specification are:

```go
type Config struct {
    Title     string
    Format    Format
    Verbosity Verbosity
    DryRun    bool

    AppID    string // advanced manifest namespace override
    StateDir string // advanced exact directory for Evo persisted state

    Stdout io.Writer
    Stderr io.Writer
    Isolated bool
}
```

Existing compatible configuration fields may remain; this list is the minimum normative behavior, not permission to delete unrelated compatible options.

`Init` constructs an `Output`; unless `Isolated` is true it also installs it as the package default used by package-level `Task`, `Group`, `Sequence`, `Fact`, `Run`, and related helpers. `Isolated` is for tests/embedders and never mutates package-default state.

CLI shape:

```go
func main() {
    evo.Init(evo.Config{Title: "ghost"})
    os.Exit(evo.Main(run))
}

func run(ctx context.Context) error {
    // declare work
    return nil
}
```

`Main` owns CLI signal-to-cancellation setup and returns the derived exit code; it does not itself call `os.Exit`. `Run` never exits the process. `Output.Run` is the isolated/embedder form.

Top-level output projection happens when the Run is finalized. Task code never calls a JSON renderer or conclusion renderer directly.

The Run itself has a renderer-owned active state. In interactive human mode, if the Run callback is still executing and no concrete Task/activity has become visible by the 80ms visibility delay, Evo paints a temporary fallback line such as:

```text
⠋ ghost  starting…
```

This is **not** a fake Task and is never retained as a completed ledger row. It exists solely so arbitrary initialization cannot leave an active Evo Run visually blank for more than 100ms. The moment a concrete Task/activity is available, the fallback disappears. Prefer concrete Task activity; `starting…` is the last-resort renderer fallback, never a replacement for modeling real work.

# 2. Evidence, tracking, definition trust, and provenance

Earlier drafts made Evidence a collection of named callbacks:

```go
task.Evidence("contents", contentsCurrent)
task.Evidence("permissions", permissionsCurrent)
```

That is superseded as the recommended model.

> **Evidence is Evo's boolean current-state conclusion when enough modeled proof exists. Before `Define`, true Evidence may skip the callback. Otherwise Evidence may become known during/after reconciliation and still drive reporting and machine output.**

There are three distinct layers. Do not collapse them:

```text
Task Evidence
    Is the modeled desired state currently satisfied?
    If proven before Define, the whole callback may be skipped.

Operation freshness
    Once Define is entered, can this particular Evo-native operation no-op
    without performing its expensive/mutating work?

Verification detail
    Which concrete subconditions were satisfied or failed, for diagnosis/reporting?
```

## 2.1 Critical soundness rule: prior runtime discovery is not pre-execution proof

A prior manifest created by operations discovered _inside an opaque callback_ is **not sufficient by itself to skip that callback on a later run**, even when the application fingerprint is unchanged. The same compiled callback may receive different closure values, CLI arguments, configuration, branch conditions, database state, or other semantic inputs that Evo cannot inspect before entry.

Therefore v1 whole-Task pre-`Define` skipping is allowed only from **current proof available before callback entry**, principally `Verify` (or a future fully declarative definition whose complete current semantic specification is available before execution).

The application fingerprint is still valuable, but only as definition identity / conservative invalidation metadata. It can prove that code definitely changed; it cannot prove that runtime intent definitely did not change.

This rule prevents the unsafe inference:

```text
same binary + yesterday's output still exists
    ≠
this invocation wants exactly yesterday's output
```

## 2.2 Automatic operation freshness still provides the practical skip

After Evo enters `Define`, `evo.File` and `evo.Exec` receive their **current invocation specification**. They compare that specification, current `Basis`, and current outputs against the manifest and may no-op precisely.

Example:

```text
Go binary changed or runtime configuration changed
→ enter callback (no unsafe whole-Task guess)
→ current evo.Exec invocation is observed
→ executable + args + Basis + outputs still match
→ Python process is not spawned
→ operation = current
→ callback returns successfully
```

This is the safe way to obtain targeted recompilation without pretending Evo can statically understand arbitrary Go code. The callback itself should keep expensive/mutating work behind Evo-native operations when that work needs automatic freshness.

Verification details may be named internally for machine output and human diagnosis:

```text
contents     already satisfied
permissions failed
```

They are not required to be public Evidence declarations.

The provenance rule is:

> **Specific semantic provenance controls operation freshness. The application fingerprint is a conservative definition-identity signal for opaque code, not a substitute for semantic Evidence.**

The tracking rule is:

> **Track semantic inputs, not ambient state.**

A `Basis` declaration is an assertion that the listed values are semantic inputs to that operation. Evo must not silently add arbitrary environment variables, hostname, temp paths, locale, or unrelated process state merely because they exist.

Conversely, Evo cannot protect against an undeclared semantic dependency. MCP review may flag obvious omissions, but runtime code must not invent them.

# 3. Task and stable identity

A `Task` is one atomic schedulable unit.

```go
task := evo.Task("check config")

task.Define(func(ctx context.Context) error {
    return checkConfig(ctx)
})
```

Creating the Task declares it. `Define` freezes scheduling configuration and submits it to Evo's scheduler. It does not mean “run synchronously now.”

A Task has no child Tasks. Multiplicity belongs to `Group` or `Sequence`.

## 3.1 Stable Task key

Manifest reuse requires identity that survives runtime-generated IDs. Do **not** depend on source-file/line metadata: trimpath/build systems can change it, and moving unrelated code should not invalidate persisted state.

The default stable key is derived only from the semantic declaration tree:

```text
entity kind
+ parent stable key
+ normalized entity name
```

The application/workspace manifest namespace already supplies application identity, so it is not duplicated into every key.

Properties:

- sibling `Task`, `Group`, or `Sequence` names must be unique within the same parent; duplicate default keys are a programmer error;
- loop declarations remain distinct when their names are distinct;
- moving source code without changing the declaration tree does not invalidate identity;
- renaming an entity or parent safely creates a cache miss for that entity/subtree;
- runtime UUID/sequence IDs are separate and are never manifest identity.

Advanced refactor/rename-stable override:

```go
task.Key("launch-agent.write-plist")
```

`Key` must be called before `Define`. An explicit key replaces the derived name-based identity for that entity within the application/workspace namespace. Duplicate explicit keys of the same entity kind are an error.

Human names are presentation, but the default key intentionally uses them because unique sibling names are already required for clear UI. Stable keys are reconciliation identity.

`Group` and `Sequence` use the same rule with their kind included. Their keys are always recorded because Task parent identity and manifest graph edges depend on them, even when the container itself has no persisted operation state.

# 4. Group

A `Group` contains independent child work.

Eligible children may execute concurrently.

```go
packages := evo.Group("packages")

for _, pkg := range required {
	task := packages.Task(pkg.Name)
	task.Define(func(ctx context.Context) error {
		return installPackage(ctx, pkg)
	})
}
```

The scheduler owns concurrency.

Application code does not create goroutines merely to make Evo parallel. Large or homogeneous groups do **not** require a special `Each` API for presentation; aggregation is renderer-owned and automatic.

---

# 5. Sequence

A `Sequence` contains ordered child work.

```go
setup := evo.Sequence("launch agent")

write := setup.Task("write plist")
register := setup.Task("register")
start := setup.Task("start")
```

The implied graph is:

```text
write plist → register → start
```

`Sequence` uses the same scheduler implementation as `Group`; it simply creates predecessor dependencies automatically.

---

# 6. After

`After` is the exceptional explicit DAG edge.

```go
database := setup.Task("database")
database.After(network)
database.Define(migrateDatabase)
```

Read as English:

> database after network
> define database as this work

Most application code should not need `After`.

Prefer structure first:

```text
Group      independent
Sequence   ordered
After      exceptional edge
```

---

# 7. Define is the scheduling and execution boundary

The common lifecycle is:

```text
Task(...)
    declare identity

.Key(...)
    optional advanced stable identity override

.After(...)
    configure exceptional dependencies when needed

.Verify(...)
    optional advanced current-state proof

.Define(...)
    freeze configuration
    submit to scheduler
    execute only when Task Evidence is not currently true
```

After `Define`:

- dependency and verification configuration is frozen;
- the scheduler may start the Task as soon as it is eligible;
- `Define` itself returns after submission; callback errors belong to the Task/Run result;
- `evo.Run` waits for all submitted work to settle before returning its `Result`;
- `evo.Main` waits for the same result and applies the derived process exit code.

A callback passed to `Define` is opaque Go code. Evo does not assume it is pure and does not assume all work inside it is represented by tracked operations.

Example:

```go
task.Define(func(ctx context.Context) error {
    data, err := generate(ctx)
    if err != nil {
        return err
    }

    return evo.File(ctx, evo.FileSpec{
        Path:     output,
        Contents: data,
        Mode:     0644,
        Basis: []evo.Fingerprint{
            evo.FSPath("schema/accounts.xlsx"),
            evo.FSPath("schema/users.xlsx"),
        },
    })
})
```

This is valid, but note the optimization boundary: `generate(ctx)` has already happened before Evo sees the `File` operation. If generation itself is expensive and should be provenance-skippable, express that work through an Evo-native operation such as `evo.Exec`, or otherwise place the expensive work behind an Evo-aware lazy/semantic adapter.

That distinction is mandatory documentation: **tracking an output after arbitrary work cannot retroactively skip the arbitrary work.**

## 7.1 Task-scoped operation context

The scheduler invokes every `Define` callback with a Task-scoped `context.Context`. `evo.File`, `evo.Exec`, and other package-level semantic operations use that context to attach operation truth to the currently Running Task.

Normative exported sentinel errors:

```go
var ErrNoTaskContext = errors.New("evo: tracked operation requires task context")
var ErrTaskClosed = errors.New("evo: task context is already closed")
```

Normative rules:

- calling a tracked semantic operation without an active Evo Task scope returns `ErrNoTaskContext`; it must never silently mutate without tracking;
- the Task scope closes when the `Define` callback returns; operations started after that point return `ErrTaskClosed`;
- child goroutines may use the passed `ctx` only if the callback waits for them before returning; Evo does not treat detached goroutines as part of a settled Task;
- callers must pass the callback's `ctx` (or a derived child context that preserves values), not replace it with `context.Background()`;
- cancellation of the Task context cancels `Exec` and causes `File` to stop before beginning a new mutation boundary where safely possible.

This context association is internal plumbing. Developers do not manually pass Task handles to `File`/`Exec`.

# 8. Declarative file operations

`evo.File(...)` is a declarative operation used inside `Define`. There is no `.Write`, `.Ensure`, or `.Apply` suffix.

Normative v1 shape:

Go cannot export both a type named `File` and a function named `File` in the same package namespace. The operation call is the important beginner spelling, so the function keeps the short name `File` and its data argument is `FileSpec`.

```go
type FileSpec struct {
    Path     string
    Contents []byte          // nil = unmanaged; []byte{} = manage as empty
    Mode     fs.FileMode     // 0 = unmanaged in the v1 shorthand
    Basis    []Fingerprint
}

func File(ctx context.Context, spec FileSpec) error
```

Example:

```go
return evo.File(ctx, evo.FileSpec{
    Path:     agent.PlistPath,
    Contents: agent.Plist,
    Mode:     agent.Mode,
})
```

means:

```text
establish this regular-file state
change only managed modeled attributes that differ
verify the resulting state
record output + Basis fingerprints
record verification details, Facts, and Effects automatically
```

Construction of a `FileSpec` performs no I/O. `evo.File` performs the operation when invoked.

## 8.1 Managed-state rules

- `Path` is required.
- relative `Path` values resolve against the Run's workspace directory captured once at Run start; changing process CWD later does not retarget an operation.
- paths are cleaned/canonicalized for identity without following the final symlink target; platform case-folding/volume rules are respected where available.
- `Contents == nil` means contents are not managed.
- non-nil empty contents mean the desired file is empty.
- `Mode == 0` means mode is unmanaged in the v1 shorthand. Managing literal mode `0000` is intentionally not supported by this shorthand; add a future explicit optional-mode representation rather than guessing.
- if the path does not exist and contents are unmanaged, Evo cannot invent contents merely to apply mode; return a typed usage/operation error.
- when creating a missing file with unmanaged `Mode`, use the platform's ordinary file-creation semantics (`0666` subject to umask on Unix-like systems) and do not subsequently claim mode as managed Evidence.
- regular files are supported in v1.
- an existing symlink at `Path` is rejected by default rather than followed implicitly.
- directories/devices/sockets at `Path` are a type mismatch, not a file to overwrite.

Unmanaged means Evo does not intentionally reconcile that modeled attribute. Atomic replacement may have platform metadata implications; the implementation must preserve existing unmanaged mode/ownership where supported and **fail rather than silently alter modeled metadata it promised to leave alone**.

## 8.2 Reconciliation algorithm

For one `evo.File` call:

```text
validate spec
→ fingerprint Basis
→ inspect existing path safely
→ compute verification details for every managed attribute
→ all managed attributes satisfied:
     record no-op/current operation state
     return nil
→ dry-run:
     record planned Effect
     perform no mutation
     do not commit new manifest state
     return nil
→ contents differ:
     write replacement in the same directory
     fsync/close
     preserve required unmanaged metadata
     atomically rename where the platform supports the contract
→ mode differs:
     chmod
→ re-inspect
→ verify every managed attribute
→ record committed Effect + tracked state
```

If contents succeed but chmod fails, safe post-error inspection may yield:

```text
✗ write plist  failed: permissions
  - contents  already satisfied
  ✗ permissions
    error  operation not permitted
```

The Task manifest is committed only after the Task itself finishes successfully; a partial failed operation never receives a success manifest record.

## 8.3 File fingerprints

Regular-file resource fingerprints use the same `FSPath` semantic content digest (type + bytes). Do not use mtime as freshness authority.

Managed mode is compared/stored as a separate verification attribute using permission bits relevant to the platform; a mode-only change does not automatically invalidate downstream content consumers unless they explicitly model mode as Basis.

Manifest records retain digests and modeled metadata, not file contents.

More than one producing operation claiming the same canonical file output in one Run is a conflict unless a future explicit shared-resource contract says otherwise. Detect conflicts as soon as the second claim is observed; fail safely and do not produce a success manifest for the conflicting Tasks.

## 8.4 External-process operation: `evo.Exec`

The primary Python/toolchain pipeline case needs an Evo-aware process operation; raw `exec.Command` cannot be provenance-skipped safely by Evo.

The existing package-level `Command(...)` name is already used for user next-action guidance, so execution sugar is named `Exec`.

Normative v1 shape:

```go
type ExecSpec struct {
    Executable string
    Args       []string
    Dir        string
    Env        map[string]string
    Basis      []Fingerprint
    Outputs    []string // regular-file outputs to fingerprint after success
}

func Exec(ctx context.Context, spec ExecSpec) error
```

Example:

```go
return evo.Exec(ctx, evo.ExecSpec{
    Executable: "python3",
    Args:       []string{"generate.py", "input.xlsx", "build/schema.bin"},
    Basis: []evo.Fingerprint{
        evo.FSPath("generate.py"),
        evo.FSPath("input.xlsx"),
    },
    Outputs: []string{"build/schema.bin"},
})
```

Path resolution rules for `Exec`:

- an empty `Dir` means the Run workspace directory; a relative `Dir` resolves against that workspace;
- relative `Outputs` resolve against the effective `Dir`;
- `Args` are literal strings — Evo does not guess which arguments are paths;
- `FSPath(...)` Basis values use the Run workspace path rules independently.

`Exec` **definition fingerprint** automatically includes:

```text
resolved executable fingerprint
argument vector in order
working directory
explicit Env entries sorted by key
Basis descriptors sorted by (kind,key)
declared output paths sorted canonically
```

Freshness then additionally requires the **current Basis digests** and **current tracked output digests** to match the prior successful record. Output digests are freshness state, not part of the definition fingerprint.

It does **not** fingerprint the entire inherited environment. By using provenance-based skipping, the caller asserts that any semantic environment dependency is represented explicitly through `Env`, `Basis`, or another adapter.

Skip protocol:

```text
prior matching Exec operation record exists
AND operation identity matches
AND every Basis matches
AND every declared output still matches
    → do not spawn process
    → operation is current
otherwise
    → spawn process
    → capture stdout/stderr through Evo
    → fingerprint declared outputs after exit 0
    → record new operation state
```

With no declared `Outputs`, `Exec` must not assume a successful process is reusable; it executes normally.

Declaring `Outputs` is a semantic assertion: those outputs plus `Basis`/operation identity must be sufficient to decide whether skipping the external process is correct. A command with unmodeled side effects (remote writes, database mutation, notifications, timestamps relied on elsewhere, etc.) must not be made reusable merely by naming one incidental output file. Use an explicit `Verify`, a richer domain adapter, or execute it normally.

After a spawned process exits 0, every declared output must exist as the declared supported resource type and be fingerprintable. Missing/inaccessible outputs are verification failure, not success. A nonzero exit fails the Task operation, retains a bounded redacted stdout/stderr tail, re-inspects declared outputs where safe for partial truth, and never commits a success operation record.

Dry-run never spawns `Exec`. If provenance proves it already current, report current. Otherwise record planned work without pretending to know outputs that were not generated.

Captured process output is sanitized/redacted **before it enters the runtime model** and retained in bounded form. By default, each complete non-empty stdout/stderr line becomes the operation's latest current activity; the live renderer coalesces rapid updates and shows only the newest line that fits the current frame. It is truncated by terminal-cell width and is not durable terminal output by default. This is what turns a dependency installer from vague `converging` into concrete activity such as `Collecting urllib3` without caller plumbing. On failure, a bounded retained tail becomes diagnostic detail. Empty lines and control-only progress frames do not replace a useful current activity line.

Generic Evo must not pretend it can parse package-manager totals from arbitrary text. Exact progress counts come from structured adapters, `Group`/Task progress, or domain-specific parsers. The live spinner still guarantees visible activity when the child is silent.

# 9. Evidence execution protocol

A Task may skip `Define` only when Evo has current proof before callback entry.

## 9.1 Advanced custom verifier

For domains Evo cannot track automatically:

```go
task.Verify(func(ctx context.Context) (bool, error) {
    return schemaVersionAtLeast(ctx, db, 18)
})
```

Normative rules:

- `Verify` is read-only.
- multiple verifiers are ANDed.
- verifiers run before `Define`; if `Define` executes successfully, they run again afterward.
- `true, nil` = satisfied.
- `false, nil` = not satisfied.
- non-nil error = observation failure, never drift.
- a verifier error prevents blind mutation and fails the Task unless an explicitly documented domain policy says otherwise.
- a current verifier may establish Evidence even when the application fingerprint changed, because the current program just observed the desired state directly;
- after a successful `Define`, any verifier returning `false` means postcondition failure and the Task fails with a stable verification-unsatisfied problem code;
- a post-`Define` verifier error is an observation failure and fails the Task; neither case commits a success Task record.

## 9.2 Task reconciliation algorithm

```text
Task becomes eligible
→ wait for explicit scheduler dependencies
→ wait for any known manifest freshness barriers that are needed before observation
→ Task = Running before any potentially blocking Verify/fingerprinting work
→ evaluate current Verify callbacks, if any

all configured Verify callbacks return true
    → Evidence = true (phase=before_definition)
    → do not enter Define
    → Task = Done
    → resolution = AlreadySatisfied

otherwise (Verify absent, false, or other proof insufficient)
    → pre-definition Evidence = false or unknown
    → enter Define
    → each Evo-native operation compares its CURRENT specification to manifest state
    → current operations no-op; stale operations execute/reconcile
    → on callback success, re-run Verify callbacks
    → verify every tracked operation that claims success
    → derive post-definition Evidence when modeled proof exists
    → commit successful operation/Task manifest truth atomically
    → Task = Done
    → resolution = Executed
```

`Executed` means the callback was entered. It does not claim every nested Evo-native operation mutated; machine output reports operation-level `current` / `executed` separately.

A prior manifest of operations discovered inside the callback is **not** a standalone whole-Task fast path. It accelerates the operations once their current invocation is known. This is deliberately conservative and prevents stale runtime intent from being mistaken for Evidence.

If a Task has no `Verify` and records no tracked operation/other observable proof, successful execution does not manufacture Evidence. The Task can still be Done/Executed; its Evidence remains unevaluated/unknown.

After callback error, Evo safely re-inspects tracked resources where possible so partial truth survives. No success Task reconciliation record is committed for the failed Task, though already-committed earlier Task records remain.

Unknown is never silently promoted to Evidence=true.

# 10. Why automatic tracking is useful

Without Evo-aware operations:

```go
task.Define(func(ctx context.Context) error {
    if err := os.WriteFile(path, contents, mode); err != nil {
        return err
    }
    return os.Chmod(path, mode)
})
```

Evo can reliably know only that opaque Go code ran and which error escaped.

With:

```go
task.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{
        Path:     path,
        Contents: contents,
        Mode:     mode,
    })
})
```

Evo automatically knows:

```text
resource path
contents fingerprint
managed mode
attribute-level verification
operation no-op vs mutation
partial state after failure
Effects and diagnostic Facts
manifest state for future reconciliation
```

For external generation, `evo.Exec` additionally allows Evo to avoid spawning an unchanged tool invocation when executable/args/Basis/outputs are all current.

This supports truthful failure output and automatic future Evidence without forcing developers to hand-name file checks.

# 11. Manifest, Basis, identity, and dependency graph

The manifest is persisted reconciliation truth, not merely “this callback succeeded last time.”

A conceptual record:

```json
{
  "schema_version": 1,
  "application": {
    "id": "ghost-controller",
    "fingerprint": "sha256:app-7d9a"
  },
  "task": {
    "key": "launch-agent.generate-flatbuffers",
    "definition": {
      "kind": "callback",
      "application_fingerprint": "sha256:app-7d9a"
    },
    "operations": [
      {
        "kind": "exec",
        "ordinal": 0,
        "definition_fingerprint": "sha256:exec-def-123",
        "basis": [
          { "kind": "fs_path", "path": "generate.py", "fingerprint": "sha256:111" },
          { "kind": "fs_path", "path": "input.xlsx", "fingerprint": "sha256:222" }
        ],
        "outputs": [{ "kind": "file", "path": "build/schema.bin", "fingerprint": "sha256:abc" }]
      }
    ]
  }
}
```

## 11.1 Fingerprint primitives

Normative advanced contract:

```go
type Fingerprint interface {
    Fingerprint(context.Context) (FingerprintValue, error)
}

type FingerprintValue struct {
    Kind   string   // stable machine kind; e.g. "fs_path", "value", "app"
    Key    string   // stable non-secret semantic identity
    Digest [32]byte // SHA-256
}

func FSPath(path string) Fingerprint
func Value(name string, value any) Fingerprint
func App() Fingerprint
```

`Fingerprint` implementations are observation-only. They must not mutate the world. `Kind` and `Key` are persisted and may appear in Debug/JSON output, so they must not contain secrets. Within one operation `Basis`, `(Kind, Key)` pairs must be unique; duplicates are a programmer error. Basis order is semantically irrelevant and is canonicalized by `(Kind, Key)` before hashing/serialization.

`FSPath` semantics intentionally avoid nitpicky metadata invalidation:

- regular file: SHA-256 of semantic type marker + bytes; ordinary permission/mtime/owner changes do not change the digest;
- directory: deterministic recursive Merkle fingerprint over sorted relative names, entry types, symlink targets, and file bytes; ordinary mtimes/permissions/ownership are excluded;
- symlink: fingerprint the link target text; do not silently follow it;
- missing path: stable “missing” digest, which differs from a present path;
- inaccessible/error: observation error, not “missing”.

This makes generated artifacts re-run for content/path-structure changes, not incidental chmod/touch noise. When metadata itself is semantically relevant, model it explicitly with a domain fingerprint or `Value`; do not silently make all metadata semantic. `evo.File` still verifies/manages its own `Mode` separately when requested.

`Value(name, value)` hashes a canonical typed scalar encoding; manifests store the digest and safe `name`, not secret/raw values. Supported v1 values are string, bool, signed/unsigned integers, finite IEEE-754 floats with canonical bit encoding, `time.Time` normalized to UTC nanoseconds, and `[]byte`. Unsupported values return a typed fingerprint error rather than using reflection/JSON with unstable semantics.

`App()` returns `(Kind="app", Key="application")` with the run's application fingerprint digest and is used only when the application's own implementation is a semantic input to an operation.

## 11.2 Application fingerprint

Compute once per run:

```text
SHA-256 of running executable bytes when readable
else stable Go build ID/build information when available
else unavailable
```

If the application fingerprint is unavailable, Evo simply lacks that definition-identity diagnostic. Opaque callback Tasks are not whole-Task skipped from manifest history alone either way. Evo-native operations still reconcile from their current invocation specification after callback entry. A caller may explicitly include `evo.App()` in an operation `Basis` when the application's own bytes are a semantic input to that result.

## 11.3 Manifest store

Default store is cache-like: loss must cause safe re-execution, never correctness loss.

Default location:

```text
os.UserCacheDir()/evident-output/<app-id>/<workspace-hash>/manifest-v1.json
```

Where:

- `app-id` defaults to Go main-module path + executable basename when build info is available, otherwise executable basename; advanced `Config.AppID` overrides it;
- workspace directory is the canonical working directory captured once when the Run begins;
- `workspace-hash` is SHA-256 of that canonical workspace directory, encoded/truncated only for filesystem-safe naming;
- if `Config.StateDir` is non-empty, it is the exact state directory and the manifest path is `<StateDir>/manifest-v1.json`; the default cache/app/workspace directory derivation is bypassed.

Security/consistency requirements:

- state directory permissions should be user-private where the platform supports it;
- schema/version/size limits are validated before use;
- corrupt/unknown manifest = cache miss + diagnostic warning, never Evidence=true;
- one Evo Run holds an exclusive per-manifest lock while reconciling that workspace;
- lock acquisition is context-cancellable;
- each successfully settled Task is committed with temp-file + fsync + atomic rename semantics where supported;
- cancellation/failure preserves already committed successful Task records;
- dry-run never commits speculative tracked state.

A prior success record left behind by a later failed run is harmless because current state/Basis is re-fingerprinted before reuse.

## 11.4 Basis and operation definition

A Basis says:

> **If any of these fingerprints changes, this operation may no longer be current.**

It does not claim “source”, “input”, or “derived from.”

Operation definition fingerprint includes the semantic operation itself. Examples:

```text
File: canonical path + managed contents digest + managed mode + sorted Basis descriptors
Exec: executable digest + argv + dir + sorted explicit Env + sorted Basis descriptors + sorted declared output paths
```

Changing the specification invalidates that operation record even if its prior outputs still exist.

For v1, a canonical tracked output resource has exactly one producing operation in a Run. Two `File` operations targeting the same canonical path, two `Exec` operations declaring the same output, or a `File` and `Exec` both claiming the same output are a programmer conflict. Fail safely rather than guessing producer ownership. Sequential consumption of an earlier output as a later operation's `Basis` is allowed.

Prior operation records are matched by current semantic definition fingerprint within the Task. No current match means stale/miss and reconciliation runs; the manifest never executes or reconstructs an operation merely from its old record.

## 11.5 Freshness/invalidation graph

The manifest records:

```text
Task → operation
operation → Basis resource/value
operation → tracked output resource
```

Invalidation follows semantic dependencies. Revalidation follows uncertainty.

If an upstream Task reruns/revalidates but produces a byte-identical output fingerprint, result invalidation stops at that resource.

## 11.6 Freshness graph vs scheduler graph

First-run correctness cannot depend on a dependency that is discovered only after callbacks execute.

Therefore:

```text
Sequence / After
    are the authority for first-run execution ordering.

Manifest provenance edges
    may add conservative freshness barriers on later runs,
    but never remove explicit scheduler dependencies.
```

When a prior manifest says Task B consumes an output produced by Task A, B must not observe/reconcile that resource from stale prior state while A is still being validated for the current run. Evo may conservatively delay B's callback or the relevant operation until A settles.

If A and B are concurrently eligible with no scheduler dependency, this prior-manifest freshness barrier prevents a known later-run race, but it does not make the authoring correct for a first run. MCP/conformance should flag a producer-consumer relationship that lacks `Sequence` / `After` when execution order matters.

Do not silently infer a permanent scheduling DAG from provenance in v1.

# 12. Canonical launchd example

The file case needs no explicit Evidence declaration. Non-file current-state checks use the advanced read-only verifier.

```go
func launchAgent(ctx context.Context, agent Agent) error {
    seq := evo.Sequence("launch agent")

    write := seq.Task("write plist")
    write.Define(func(ctx context.Context) error {
        return evo.File(ctx, evo.FileSpec{
            Path:     agent.PlistPath,
            Contents: agent.Plist,
            Mode:     agent.Mode,
        })
    })

    register := seq.Task("register")
    register.Verify(func(ctx context.Context) (bool, error) {
        return launchAgentRegistered(ctx, agent.Label)
    })
    register.Define(func(ctx context.Context) error {
        return bootstrapLaunchAgent(ctx, agent.PlistPath)
    })

    start := seq.Task("start")
    start.Verify(func(ctx context.Context) (bool, error) {
        return launchAgentRunning(ctx, agent.Label)
    })
    start.Define(func(ctx context.Context) error {
        return startLaunchAgent(ctx, agent.Label)
    })

    return nil
}
```

On an unchanged run, `register` and `start` may resolve `AlreadySatisfied` because their current `Verify` callbacks prove the state before `Define`. `write plist` still enters its cheap callback, but `evo.File` detects that the current file specification is already satisfied and performs no mutation.

If contents are already correct but chmod fails:

```text
✗ write plist  failed: permissions
  - contents  already satisfied
  ✗ permissions
    error  operation not permitted
    path   ~/Library/LaunchAgents/com.example.agent.plist
```

No caller manually assembled those verification rows.

# 13. Facts

A Fact is structured information learned by the run.

```go
task.Fact("path", agent.PlistPath)
evo.Fact("repository", repoPath)
```

Facts are not lifecycle, scheduling authority, Evidence, or Effects.

Fact values are normalized into stable scalar machine values where possible. Human rendering is a projection decision; recording a Fact does **not** mean it earns a terminal row.

Evo-native operations automatically emit relevant Facts such as file path, mode, executable, exit status, and bounded diagnostic details. Callers should not duplicate those as manual prose.

Every Fact has a scope/link in the runtime model:

```text
run-scoped       evo.Fact(...)
task-scoped      task.Fact(...)
operation-scoped emitted by File/Exec or an operation adapter
problem-linked   referenced by the Problem that makes it diagnostic
```

Normal failure rendering automatically surfaces only Facts linked to the failing Problem/operation. A generic Task-scoped or run-scoped Fact is **not** promoted merely because some Task failed; otherwise routine configuration floods failure output. Verbose/debug may show the broader scoped Facts.

Default visibility is automatic:

```text
normal successful Task    hide ordinary Facts
warning/failure            surface directly related diagnostic Facts
verbose                    show Task/run Facts subject to row budget
debug                      also show verification/provenance diagnostics
JSON/JSONL                 retain structured Facts regardless of human verbosity
```

If a success value is the actual user-facing result rather than diagnostic context, put it in the Task's structured summary/domain result rather than relying on a routine Fact to become visible.

# 14. Problems and Facts are separate typed records

Problems often _describe facts_ such as permission denied or authentication expired, but the implementation must not collapse Problems into the Fact type.

`Problem` remains a specialized typed record because it carries:

- severity;
- stable machine code;
- lifecycle consequence;
- remediation/next-action context;
- related diagnostic Facts/verification details.

`Fact` remains consequence-free structured information.

This separation prevents a junior implementation from accidentally deriving failure from arbitrary information or losing stable problem codes in generic key/value data.

# 15. Effects remain separate and are normally automatic

Effects deserve separate runtime semantics because they describe mutation:

```text
planned
committed
partial
suppressed by dry-run
```

Callers should not hand-compose effect strings. Evo-native mutation boundaries create typed Effects automatically:

- `evo.File(...)` records the file attributes it actually establishes, or the corresponding planned change in dry-run;
- `evo.Exec(...)` may record a typed operation effect when its contract declares outputs or another concrete mutation result;
- existing typed mutation verbs such as Create/Update/Delete remain the custom-domain escape hatch and are dry-run boundaries;
- a raw opaque `Define` callback has no automatic mutation semantics beyond the Evo-native operations it invokes.

Examples:

```text
[planned] branches  delete 8 local tips
[changed] branches  deleted 8 local tips
```

A committed Effect must never disappear from the model if later work fails. Routine one-to-one Effects may be hidden from Normal human output when the Task verdict already communicates the same fact, but they remain in JSON/JSONL.

There is no public free-form `Effect("...")` string API in the recommended surface. Effects are structured truth, not presentation text.

---

# 16. Human UI thesis

The runtime may know much more than the terminal should show.

The renderer must rank information.

The critical rule is:

> **Rows are scarce. Do not render something merely because the model knows it.**

Dimming ten unnecessary rows still produces a bad ten-row UI.

Hide routine information entirely by default.

---

# 17. Normative UI hierarchy and automatic verbosity

The renderer uses semantic priority plus a row budget; callers do not choose glyphs/colors/indentation per row.

Priority from highest to lowest:

```text
Prompt / failure / blocked / cancellation
Warning requiring attention
Running Task
Current activity (one indented line when useful)
Determinate progress exact count (bar is decoration)
Committed/planned Effect that is not redundant
Completed Task landmark
Directly related diagnostic Fact
Pending/NotStarted
Routine Fact
Passing verification detail
Debug/provenance detail
```

Intensity:

```text
Task names                    full
Current activity              full
Failure/warning message       full semantic color
Useful Fact label             muted
Useful Fact value             full
Timer                         muted
Already-satisfied suffix      muted
Pending/NotStarted            muted
```

Avoid excessive bold.

## 17.1 Human verbosity levels

Preserve existing zero-value compatibility:

```text
VerbosityQuiet   = -1
VerbosityNormal  = 0   default
VerbosityVerbose = 1
VerbosityDebug   = 2
```

Behavior:

- **Quiet:** while running, show the minimum truthful active line; at rest show conclusion + failures/blocked reasons. Do not hide errors.
- **Normal:** default hierarchy above. Routine Facts and passing verification are hidden.
- **Verbose:** expand Facts, additional active children, skipped/current reasoning, and effect detail while respecting terminal height.
- **Debug:** additionally expose bounded verification/provenance/manifest diagnostics and captured-process tails.

Machine projections are not filtered by human verbosity.

## 17.2 Row budget

On a live terminal, derive available rows from terminal height. Reserve rows for prompt/conclusion/failure detail, then fit highest-priority live information. Aggregate Groups when needed instead of letting hundreds of children scroll the terminal.

Row scarcity is automatic projection policy, not an application concern.

Terminal examples in docs must contain only terminal output. Explanatory prose belongs outside the terminal block.

# 18. Normal success

Routine verification and Facts stay hidden in normal mode:

```text
✓ launch agent
  ✓ write plist
  ✓ register
  ✓ start
```

Do not print contents digests, mode checks, application fingerprints, or manifest hits merely because the runtime knows them.

A running Task uses the stable parent/current-activity shape:

```text
⠋ install dependencies  [████        ]  14/40  — 7s
  ⠋ urllib3
```

The child activity line is allowed to change/truncate independently without moving the timer.

# 19. Already satisfied

When Evidence is true before entering `Define`:

```text
✓ write launch agent  already satisfied
```

The suffix is muted.

The Task remains a full-intensity successful landmark because the requested state is satisfied. Do not present the entire Task as a failure-like skip.

Larger example:

```text
✓ deploy production
  ✓ discover
  ✓ prepare hosts       already satisfied
  ✓ services            already satisfied
  ✓ write launch agent  already satisfied
    path  ~/Library/LaunchAgents/com.acme.prod.agent.plist
  ✓ cleanup             nothing to do
```

Passing verification details remain hidden.

Do not apply the `already satisfied` Task suffix merely because an `evo.File` or `evo.Exec` operation no-op'd **after** `Define` was entered. That is operation freshness, not pre-definition Task resolution. In Normal mode the successful Task can simply remain:

```text
✓ write plist
```

Verbose/debug or JSON may expose `file current` / `operation.current=true`.

---

# 20. Partial failure

Relevant verification detail surfaces only when it explains what actually happened:

```text
✗ write launch agent  failed: permissions
  - contents  already satisfied
  ✗ permissions
    error  operation not permitted
    path   ~/Library/LaunchAgents/com.acme.prod.agent.plist
    mode   0644
```

Use a plain, widely-rendered `-` for an already-satisfied/skipped detail. Do not introduce a decorative skip glyph that is likely to render inconsistently across terminals.

The parent Task names the failed component. The concrete error becomes a Fact beneath that component rather than being jammed into the Task headline.

The `contents` detail appears because it materially narrows the diagnosis: the bytes were already correct; permissions are what failed.

---

# 21. Facts in human output

Normal successful Tasks do not show routine Facts by default.

Verbose example:

```text
✓ write launch agent
  path  ~/Library/LaunchAgents/com.example.agent.plist
```

Styling:

```text
label    muted
value    full text
```

Failure-related Facts surface automatically when they explain the failure:

```text
✗ permissions
  error  operation not permitted
  path   ~/Library/LaunchAgents/com.example.agent.plist
```

The operation that failed owns those Facts; the caller should not have to manually “promote” them.

Unrelated configuration Facts stay hidden in normal mode even if the Task fails, unless the row budget has room in verbose/debug mode.

# 22. Warnings

Warnings must not visually disappear.

```text
⠋ services  [████        ]  14/40  — 8s
  ⠋ payments-api
  ! audit-stream rollout slower than baseline
```

Both:

```text
!
warning message
```

use the warning color.

Warnings annotate; they do not automatically resolve lifecycle.

---

# 23. Progress and visible activity

Evo-native operations and collection state should populate progress/activity automatically when they know it. For custom work, retain one small semantic fallback API:

```go
task.Doing("urllib3")        // current concrete activity; presentation is renderer-owned
task.Progress(done, total)  // absolute item counts, never deltas
```

`Doing` and `Progress` are thread-safe Task observations; they do not resolve lifecycle and do not reset elapsed time. `Progress` rejects negative values, `completed > total`, regression, and a changed total after the first nonzero total is sealed. `Doing("")` clears explicit activity. Callers never include spinners, indentation, timers, bars, or `N/M` text themselves.

A future byte helper may exist, but the model stores byte progress distinctly from item counts; callers must not pass rendered `"5.2 MB"` strings as numeric progress.

Determinate progress uses a stable parent line plus current activity:

```text
⠋ prepare hosts  [████        ]  31/100  — 8s
  ⠋ host-031

⠋ services       [█████       ]  14/40   — 8s
  ⠋ payments-api
```

The exact numeric count is authoritative. The bar is decorative. Empty cells are literal spaces, never shaded/outline glyphs:

```text
[████        ]
```

Current activity is the most concrete useful item Evo knows: package, file, host, branch, subprocess line, etc. Avoid abstract text such as `converging` when concrete activity exists.

On narrow terminals, drop decoration before information:

```text
⠋ services  14/40  — 8s
  ⠋ payments-api
```

Never invent a denominator:

```text
⠋ services  discovering…
```

Progress invariants:

```text
completed never decreases
completed never exceeds a sealed total
indeterminate → determinate is allowed once
a sealed determinate total never changes
bytes use exact byte counts internally; rendered units are consistent
retry does not reset absolute completed progress
```

## 23.1 First paint and live heartbeat

`Init` should happen before application I/O that the user could reasonably wait on. When a `Run` is active in interactive `FormatHuman`, Evo arms the live surface immediately. If no concrete Task exists yet, the temporary Run-level `starting…` fallback guarantees first paint. A Task that remains active beyond the visibility delay must become visible no later than 100ms after its active observation begins; the default visibility delay is 80ms to avoid flashing Tasks that complete instantly.

A potentially blocking `Verify` is **active work**: once its Task is eligible, the Task transitions to Running before the verifier executes. If there is no more specific activity, Evo may render the automatic fallback `checking current state`. A Task can therefore briefly be Running and still finish with resolution `AlreadySatisfied`; that resolution means `Define` was skipped, not that no observation work occurred.

Default live-motion contract:

> **After a Task enters Running, Evo emits a visibly different live frame within 100ms and at least every 100ms thereafter while any live Task remains Running.**

Implementation: one renderer-owned animation clock shared by all spinners; target period 80ms, never slower than 100ms under normal scheduling. The animation tick is a render invalidation source independent of semantic model version: frame coalescing/deduplication must not suppress a Running repaint merely because no Task field changed. Consecutive frames use different spinner glyphs, so a successful repaint is visibly different.

Semantic progress is not required every 100ms; spinner motion is sufficient to prove the UI is alive. If terminal writes fail or block, Evo reports/propagates the I/O failure according to its output error policy rather than claiming the visual guarantee was met.

Intentional exceptions:

- prompt/quiesce windows waiting for human input;
- explicit motion-disabled accessibility/configuration mode;
- plain/non-TTY output, which uses durable milestone/heartbeat policy instead of animation.

The guarantee begins when Evo owns a Running live region, not at arbitrary OS process start before Evo is initialized.

# 24. Timers

Elapsed time appears automatically after 5 seconds of actual Running time.

```text
⠋ services  [█████       ]  14/40  — 8s
  ⠋ payments-api
```

Timer styling is muted.

The timer stays on the stable parent Task line. Current activity must not push it horizontally or move it between frames.

Doing/progress updates never reset the timer.

Pending work does not accumulate running time.

The timer itself does not need to tick every 100ms; the shared live-render heartbeat keeps the frame visibly alive through the spinner.

---

# 25. Large Groups and collections

Groups can contain many Tasks. Rendering every child is not a correctness requirement; retaining every child in the model is.

Normal TTY projection uses row budget and aggregation automatically:

```text
⠋ worktrees  [████        ]  31/100  — 8s
  ⠋ ../worktrees/app-031
⠋ branches   [███         ]  14/50   — 8s
  ⠋ feat/cleanup
```

A failing/attention child surfaces:

```text
✗ worktrees  99/100 checked · 1 failed
  ✗ ../bpp2.0  git status failed
```

Verbose mode may show more active children. Structured output retains all children.

Do not require a special `Each` authoring API merely to make the renderer scalable; aggregation is a renderer concern. Collection helpers may exist for ergonomics/progress derivation, but they are not required by this specification.

# 26. Effects ledger

Execution truth and mutation truth should not compete in the same visual grammar.

```text
✓ branches  50 checked
  ! kept 13 (8 protected, 5 unpushed)

[changed] branches  deleted 8 local tips
```

Dry-run:

```text
[planned] branches  delete 8 local tips
```

Do not color an entire ledger block as if it were lifecycle.

The tag carries the semantic accent.

---

# 27. Dry-run

Dry-run is automatic at **Evo-native mutation boundaries**. Evo may still enter a `Define` callback so the callback can perform read-only discovery and declare the operations that would be required.

In dry-run mode:

- `evo.File(...)` may inspect current state and compute a plan, but it does not write, replace, chmod, chown, or otherwise mutate the filesystem;
- `evo.Exec(...)` does not spawn the external process; if its recorded contract is stale, it records the operation as planned rather than executing it;
- typed mutation verbs do not invoke their mutation callback; they record planned Effects;
- no committed Effect is recorded;
- no speculative manifest state is committed as if work occurred;
- human and machine output use planned, never committed, tense;
- Evo never prompts merely to authorize work that dry-run will not perform.

An arbitrary Go callback is opaque. Evo cannot intercept `os.WriteFile`, a direct database mutation, a raw `exec.Command`, or another side effect performed directly by application code. Therefore **code that promises Evo dry-run safety must express mutation through Evo-native mutation boundaries**. Performing raw mutation directly inside such a `Define` callback is misuse; the MCP/reviewer must flag it.

Read-only discovery inside `Define` is allowed in dry-run. This is what lets the same program produce an accurate plan without duplicating a separate planning implementation.

```text
[dry-run] repo  ~/Developer/zq

✓ worktrees  100 checked
  ! kept 6 (4 dirty, 2 unpushed)
✓ branches  50 checked
  ! kept 13 (8 protected, 5 unpushed)

[planned] worktrees  remove 6 worktrees
[planned] branches   delete 8 local tips
```

---

# 28. Prompt UI

Prompting quiesces live animation:

```text
? apply 14 changes?  destructive  [y/N]
›
```

Styling:

```text
?            prompt accent
question     full text
destructive  warning color
[y/N]        secondary
typed input  full text
```

Human decline → Blocked.

Interrupt → Cancelled.

Non-TTY execution must not hang waiting for input.

---

# 29. Lifecycle

Typed lifecycle:

```text
Pending → Running → Done | Failed | Blocked | Cancelled
Pending → NotStarted
```

Warning is an annotation, not a terminal lifecycle state.

`AlreadySatisfied` is not lifecycle. It is a successful resolution reason.

Forbidden transitions include terminal reopening, progress regression, and a Sequence child starting before its predecessor is successfully settled.

A Group may have multiple Running children.

# 30. Resolution and Evidence reporting

Machine output must distinguish at minimum:

```text
Executed
    Define callback was entered and returned successfully.

AlreadySatisfied
    Define callback was not entered because current pre-definition Evidence was true.

NoWork
    Task was explicitly resolved successfully without a Define callback.
```

Operation-level no-ops are reported separately; they do not change an `Executed` Task into `AlreadySatisfied` after the callback already ran.

Evidence reporting preserves both observation phases when they occurred:

```text
before.evaluated=false
    no modeled pre-definition proof was available.

before.evaluated=true, satisfied=true
    current proof allowed Define to be skipped.

before.evaluated=true, satisfied=false
    current proof showed the desired state was not yet satisfied.

after.evaluated=true, satisfied=true
    reconciliation/post-verification proved the modeled desired state after callback entry.
```

A Task can therefore be `resolution=executed` while `evidence.after.satisfied=true`: the callback ran, but its tracked operations may all have been current/no-op. Preserve the before-false/after-true transition rather than overwriting it with one final boolean.

This avoids abusing `null`, lifecycle states, or manifest history to mean “Evidence available.”

# 31. Exit codes

One Conclusion owns both UI and process exit.

Recommended mapping:

```text
OK         0
Blocked    1
Failed     2
Cancelled  130
```

A printed conclusion and process exit code must never disagree.

`evo.Run` returns a Result and never calls `os.Exit`.

`evo.Main` is the CLI convenience that applies the derived process exit code.

---

# 32. Existing public `Action` name

The current repository already exposes `Action` for a **recommended next step for the user**.

Do not introduce another public execution `Action` type.

Use **definition** or **Define callback** for executable Task work in implementation/docs. `Define` may contain arbitrary application work plus Evo-native declarative operations, so calling it an `Action` is both ambiguous and unnecessarily narrow.

Public developer syntax remains `Define`, avoiding the existing naming collision.

---

# 32.1 Automatic projection selection

“Automatic JSON output” means application code never manually marshals Evo runtime structs. One top-level format selection chooses the projection.

Extend the existing `Format` enum without renumbering existing values:

```text
FormatHuman     existing default; live TTY when possible, durable plain otherwise
FormatData      existing domain-payload mode; stdout app-owned, Evo presentation stderr
FormatExternal  existing externally-rendered/snapshot mode
FormatJSON      new final Evo run JSON document
FormatJSONL     new streaming Evo event protocol
```

Normative public additions:

```go
type Format int
const (
    // Existing FormatHuman / FormatData / FormatExternal keep their current values.
    FormatJSON Format = /* next unused value */
    FormatJSONL
)

func ParseFormat(s string) (Format, error)
```

`ParseFormat` accepts exactly `human`, `data`, `external`, `json`, and `jsonl` (case-insensitive, surrounding whitespace ignored) and rejects unknown values. A host CLI may expose `--json` as shorthand for selecting `FormatJSON`; Evo does not parse process arguments itself.

`FormatHuman` automatically chooses live vs plain from terminal capability. **Do not infer JSON merely because stdout is a pipe.** Machine format is explicit top-level policy because a pipe may mean logs, not JSON.

Structured stream rules:

```text
FormatJSON
    stdout: exactly one final JSON document + newline
    stderr: human live/plain presentation according to Verbosity/capability, never mixed into stdout
    failure/cancel still emits a final JSON document when serialization is possible

FormatJSONL
    stdout: JSON event lines only
    stderr: human presentation according to Verbosity/capability
    final run.finished event emitted on normal failure/cancel paths

FormatData
    stdout: application domain payload
    stderr: Evo presentation
```

Human verbosity never removes semantic fields from JSON/JSONL. Applications that want machine stdout with no human stderr set `Stderr: io.Discard`; no separate per-Task switch exists.

## 32.2 Projection write failures

Projection failures are real Run failures, not ignored logging errors.

- Final JSON is encoded into an internal bounded buffer first. An encode failure writes no JSON payload, records an output/encoding Problem, and makes `Main` return Failed/2.
- A destination `Write` error is recorded as an output Problem and makes `Main` return Failed/2. The library cannot retract bytes already accepted by a broken writer, so byte-perfect atomicity across an arbitrary `io.Writer` is not promised.
- JSONL emits each event as one fully encoded line under the output lock. Earlier valid lines remain valid if a later write fails; the Run then fails and best-effort emits no fabricated success event.
- Human terminal write failures use the same output-error path; the 100ms visual guarantee applies only while the terminal accepts writes.
- `Result` retains the semantic work outcome plus output Problems so embedders can distinguish work failure from presentation/transport failure; process exit severity is the maximum of the two.

CLI frameworks bind `ParseFormat` to their own `--format` / `--json` flags; Evo itself does not globally parse `os.Args` behind the host CLI framework's back.

---

# 33. Machine output principle

TTY, plain, JSON, JSONL, and HTTP are projections of one runtime truth.

No projection invents lifecycle, Evidence, Effects, or Facts.

No public HTTP/JSON encoder marshals internal Go structs directly. Wire types are explicit contracts with explicit schema versions.

All structured output is emitted by Evo once the top-level format is selected; Task call sites never maintain a parallel “JSON version” of their logic.

# 34. Current repository compatibility note

As of the inspected `main` branch on 2026-09-14:

- the README advertises version `v0.5.0`;
- final JSON uses `JSONSchemaVersion = "1.0"`;
- JSONL events use `EventSchemaVersion = "0.2"`;
- final JSON currently exposes `items`, `task_collections`, `tasks`, `changes`, `plans`, `messages`, and `actions`;
- the current `Config.Format` surface distinguishes human/data/external projection; adding JSON/JSONL must extend that policy rather than creating a parallel per-callsite encoder path;
- the existing public `Action` type is user next-action guidance;
- the repository already contains MCP documentation and MCP-related implementation surfaces.

Therefore:

> Do not silently replace the published JSON 1.0 shape with an incompatible document under the same version.

If the desired new envelope is incompatible, introduce a new final JSON schema version.

---

# 35. Recommended final JSON v2 envelope

If the consolidated model ships as an incompatible final document, publish schema `2.0` rather than modifying `1.0` in place.

Required top-level shape:

```json
{
  "object": "evo.run",
  "schema_version": "2.0",
  "evo_version": "0.6.0",
  "run_id": "run_123",
  "started_at": "2026-09-15T20:00:00Z",
  "finished_at": "2026-09-15T20:00:02Z",
  "duration_ms": 2147,
  "mode": "apply",
  "outcome": "ok",
  "exit_code": 0,
  "data": {
    "collections": [],
    "tasks": [],
    "facts": [],
    "problems": [],
    "effects": [],
    "actions": []
  }
}
```

Required enums:

```text
mode:     apply | dry_run
outcome:  ok | blocked | failed | cancelled
```

Schema v1 remains stable until intentionally deprecated. Do not repurpose `1.0`.

The v2 encoder must have a published JSON Schema fixture and golden examples for success, already-satisfied, failure, blocked, cancelled, dry-run, provenance revalidation, and partial Effects.

# 36. Task JSON

A Task retains information suppressed by normal TTY.

Normative semantic shape:

```json
{
  "id": "task_runtime_17",
  "key": "launch-agent.write-plist",
  "parent_id": "seq_runtime_3",
  "name": "write plist",
  "state": "done",
  "resolution": "executed",
  "definition_executed": true,
  "evidence": {
    "before": {
      "evaluated": false
    },
    "after": {
      "evaluated": true,
      "satisfied": true,
      "source": "operations"
    }
  },
  "progress": null,
  "activity": null,
  "timing": {
    "queued_ms": 0,
    "running_ms": 0,
    "total_ms": 3
  },
  "verification": [
    { "name": "contents", "status": "satisfied" },
    { "name": "permissions", "status": "satisfied" }
  ],
  "tracked_resources": [
    {
      "kind": "file",
      "path": "~/Library/LaunchAgents/com.example.agent.plist",
      "fingerprint": "sha256:file-abc",
      "mode": "0644"
    }
  ],
  "basis": [],
  "facts": [],
  "problems": [],
  "operations": [
    {
      "kind": "file",
      "executed": false,
      "current": true
    }
  ]
}
```

Required Task enums:

```text
state:       pending | running | done | failed | blocked | cancelled | not_started
resolution:  executed | already_satisfied | no_work
verification.status: satisfied | unsatisfied | error | unknown
evidence.before.source: verify
evidence.after.source:  verify | operations | mixed
```

Each Evidence phase contains `evaluated`. `satisfied` and `source` are present only when `evaluated=true`. For a Task with no modeled proof in a phase, emit `{"evaluated":false}` for that phase. If `before.satisfied=true`, `after` is omitted because `Define` did not run.

Semantic separation is mandatory:

```text
Evidence             boolean current-state conclusion when modeled proof exists
verification detail  diagnostic sub-results
tracked resources    observable state Evo can fingerprint
Basis                 semantic operation dependencies
Definition identity   why a prior callback declaration is trusted
Operation state       whether nested Evo-native work executed or no-op'd
```

Collections include `kind: group|sequence`, child IDs, derived progress, and state. Effects/Facts/Problems use their own wire records rather than being flattened into human strings.

# 37. Problems and stable codes

Machine consumers must not parse human messages.

Example:

```json
{
  "code": "launchd.plist.permissions",
  "message": "failed to set plist permissions"
}
```

Stable codes are contract.

Messages may evolve.

Typed Go constants are appropriate:

```go
const (
	ErrWritePlist evo.ErrorCode = "launchd.plist.write"
	ErrSetPerms   evo.ErrorCode = "launchd.plist.permissions"
)
```

---

# 38. JSONL

JSONL is a durable event protocol and follows the same redaction/security rules as final JSON.

Envelope:

```json
{
  "object": "evo.event",
  "schema_version": "1.0",
  "run_id": "run_123",
  "seq": 17,
  "at": "2026-09-15T20:00:01.241Z",
  "type": "evidence.evaluated",
  "entity_id": "task_runtime_17",
  "payload": {
    "phase": "after_definition",
    "evaluated": true,
    "satisfied": true,
    "source": "operations"
  }
}
```

`seq` is strictly monotonic within a run and is the ordering authority. Timestamp is informational.

Required event families:

```text
run.started
collection.declared
task.declared
task.eligible
evidence.evaluated
task.started
definition.started
operation.started
operation.skipped_current
tracked_resource.observed
basis.fingerprinted
verification.observed
fact.recorded
warning.recorded
effect.planned
effect.committed
manifest.task_committed
operation.finished
definition.finished
task.finished
run.finished
```

Events must distinguish:

```text
whole Task skipped from current pre-definition Verify Evidence
callback entered because no current pre-definition proof existed
nested operation skipped as current
Basis drift
tracked output drift
upstream revalidated with identical output
manifest corruption/cache miss
```

JSONL is never filtered by human verbosity. Bounded raw captured streams remain diagnostic attachments/tails, not unbounded event spam by default.

# 39. Optimization data

The runtime event model should make optimization measurable without a second instrumentation system.

Useful derived metrics:

```text
declaration → eligible wait
eligible → started scheduler wait
Define duration
Evidence evaluation duration
operation-manifest hit / no-op rate
Verify-driven already-satisfied rate
opaque callback entry/reconciliation rate
Basis invalidation rate
tracked-result change rate
propagation stopped by identical output
group concurrency
critical-path duration
```

This allows Evo to explain whether time is spent:

```text
waiting on dependencies
waiting on scheduler capacity
executing work
revalidating uncertain definitions
checking provenance
verifying tracked state
```

---

# 40. Plain / non-TTY projection

Plain mode uses durable lines only: no cursor movement, ANSI animation, or braille frame cycling.

Progress streams milestones rather than every update:

```text
• install dependencies  0/40
• install dependencies  4/40
  requests
• install dependencies  14/40
  urllib3
✓ install dependencies  40/40
```

Thinning target for determinate progress is roughly ten useful milestones, always including first and final progress and every meaningful activity change.

Silent long-running work gets an automatic durable heartbeat no more often than every 30s:

```text
• generate schema  — 30s
• generate schema  — 60s
```

This is intentionally much slower than the live-TTY 100ms animation contract to avoid log spam.

# 41. Color, glyphs, and motion

Text/glyph semantics survive ANSI stripping. Color only reinforces.

| Meaning                  | Unicode                  | ASCII                         |
| ------------------------ | ------------------------ | ----------------------------- |
| Running                  | braille spinner          | non-semantic spinner alphabet |
| Done                     | `✓`                      | `[ok]`                        |
| Failed                   | `✗`                      | `[x]`                         |
| Blocked                  | `⊘`                      | `[blocked]`                   |
| Cancelled                | `■`                      | `[cancel]`                    |
| Already-satisfied detail | `-`                      | `-`                           |
| Not started              | `-` + text `not started` | `[-]`                         |
| Pending                  | `○`                      | `[.]`                         |
| Warning                  | `!`                      | `[!]`                         |
| Prompt                   | `?`                      | `[?]`                         |
| Next action              | `→`                      | `>`                           |
| Detail                   | `└─`                     | `` `- ``                      |

The plain dash is intentionally reused only where accompanying text disambiguates `already satisfied` vs `not started`; never rely on the dash alone.

Measure terminal cells, not rune counts.

Expose color/glyph/motion policy independently. Explicit motion-disabled mode is the only live-TTY configuration allowed to violate the 100ms visible-change animation rule.

# 42. Security and sensitive provenance

All externally derived text is sanitized before entering durable presentation or machine output.

Examples:

```text
subprocess stdout/stderr
file/repository names
remote messages
Facts
verification details
tracked-resource / Basis labels
problem detail
```

Captured secrets are redacted before storage in the runtime model, not merely at render time, so JSON/JSONL cannot bypass human redaction.

Manifest rules:

- store digests, not file contents;
- `Value` stores a safe label + digest, not the raw value;
- reject oversized/deep/corrupt manifest data before allocation-heavy processing;
- manifest files are user-private where supported;
- never execute a command from a manifest; manifests describe prior truth only.

A manifest is optimization state, not authorization state.

# 43. Cancellation

SIGINT/SIGTERM flow through the same runtime truth.

Example:

```text
✓ scan
■ venv     interrupted
- install  not started
```

Exit `130`.

Completed truth remains.

Committed effects remain.

---

# 44. Developer adoption ladder

Teach in this order:

```text
1. Task + Define
2. Group / Sequence
3. evo.File for declarative file state
4. evo.Exec for external work with declared outputs
5. Basis when operation freshness depends on semantic external inputs
6. After for exceptional execution dependencies
7. Facts / warnings / Effects / dry-run
8. Verify only for domains Evo cannot track automatically
9. top-level Format/Verbosity when the host CLI needs machine/verbose output
```

Do not require named Evidence declarations for common resources.

Opaque work remains valid:

```go
task := evo.Task("send notification")
task.Define(sendNotification)
```

It executes on each eligible run unless a current `Verify` proves it already satisfied. Evo does not pretend an opaque callback is cacheable merely because it succeeded before.

# 45. No TaskConfig for now

Do not add:

```go
evo.TaskConfig{...}
```

as a recommended public authoring path.

Task intent remains fluent:

```go
task := group.Task("database")
task.After(network)
task.Define(migrate)
```

Structs are appropriate for data-shaped desired state and persisted/wire representation. A file specification is therefore compatible with this rule even though a Task configuration struct is not.

The division is:

```text
Task authoring             fluent
Desired resource state     data/struct where useful
Persisted manifest         struct
Wire/schema data           struct
Internal normalized spec   struct
```

---

# 46. Public API drift test

Keep one exported-surface golden.

It should fail if stale concepts reappear unintentionally.

Examples to forbid unless explicitly retained for compatibility:

```text
Task.Run
Task.Go
Task.Each (legacy/deprecated; must not reappear)
DisplayGroup
Group.Done
Sequence.Fail
```

The test should make intentional additions such as declarative file/provenance primitives visible in review.

Do not treat named `Task.Evidence(...)` as required beginner surface merely because older drafts used it.

---

# 47. Evidence, identity, tracking, provenance, and manifest tests

Required deterministic tests:

```text
stable Task key is deterministic for same kind/parent/name
moving source code alone preserves default key
renaming default-key entity/parent causes safe manifest miss
duplicate sibling names are rejected
explicit Key survives refactor and duplicate Key is rejected
first run without proof → Define executes
current Verify true → Define not called even with no manifest
Verify false → Define called
Verify error → no blind mutation
post-Define Verify false/error → Task fails and no success record commits
same app fingerprint + prior tracked operations + no Verify → callback still entered
changed runtime closure/config value + same app fingerprint → callback still entered
app fingerprint changed + no current Verify → callback entered
unchanged/current evo.File invocation → File performs no mutation
unchanged/current evo.Exec identity/Basis/outputs → process not spawned
changed current Exec args with same app fingerprint → process executes
changed current File Contents with same app fingerprint → File reconciles new contents
explicit Basis evo.App() + app fingerprint changed → relevant operation becomes stale
tracked output drift → operation/Task reconciliation runs
Basis drift → relevant operation runs
unknown/inaccessible observation never becomes Evidence=true
successful callback + verified tracked state → task manifest committed
failed callback → no success task-manifest commit
contents already correct + chmod failure → partial verification retained
manifest write is atomic
corrupt/unknown manifest → safe cache miss
concurrent Runs serialize on manifest lock
cancelled Run preserves prior committed successful Task records
dry-run never writes manifest success state
symlink File target rejected by default
same canonical output claimed by conflicting File/Exec producers is rejected
upstream reruns but output digest unchanged → result invalidation stops
changed upstream output invalidates semantic descendants
known manifest producer delays consumer Evidence evaluation until producer revalidated
first-run producer/consumer ordering still requires Sequence/After
FSPath directory fingerprint is deterministic and ignores mtime
Value stores digest, not raw secret
```

Use injected filesystem/clock/process runner/fingerprinter where needed. No sleep-based freshness tests.

# 48. Scheduler tests

Use deterministic gates.

No sleeps.

Required:

```text
Group eligible children may overlap
Sequence children start in order
After blocks until dependency completes
configured concurrency ceiling is never exceeded
failed Sequence predecessor makes later dependents NotStarted
independent Group siblings retain truthful eligibility
```

Run under:

```text
go test -race ./...
```

---

# 49. UI and automatic-verbosity golden matrix

Golden-test the projection matrix.

| Runtime truth                       | Normal live TTY                            | Verbose/debug                 | Plain                      | JSON/JSONL                  |
| ----------------------------------- | ------------------------------------------ | ----------------------------- | -------------------------- | --------------------------- |
| Pre-definition Evidence true        | `✓ … already satisfied`                    | may show why                  | same durable verdict       | full before-Evidence record |
| Passing verification                | hidden                                     | debug may show                | hidden                     | retained                    |
| Diagnostic already-satisfied detail | shown only when explanatory                | shown                         | shown                      | retained                    |
| Failing verification                | shown                                      | shown                         | shown                      | retained                    |
| Routine Fact on success             | hidden                                     | shown by verbosity/row budget | hidden normal              | retained                    |
| Failure-related Fact                | shown automatically                        | shown                         | shown                      | retained                    |
| Warning                             | shown                                      | shown                         | durable warning            | retained                    |
| Running determinate                 | parent bar/count/timer + activity child    | may expand active children    | milestones                 | progress fields/events      |
| Timer                               | after 5s                                   | same                          | periodic heartbeat context | timing fields               |
| Large Group                         | aggregate by row budget                    | expand when space/verbosity   | milestones                 | all children                |
| Evo-native no-op                    | normally invisible beneath successful task | may show current reason       | normally invisible         | operation retained          |
| Effects                             | separate/nonredundant ledger               | expanded                      | durable ledger             | structured                  |

Additional golden requirements:

```text
ANSI-stripped output retains meaning
narrow terminal drops bar before count
current activity does not move timer column
empty progress cells are spaces, never ░/outline glyphs
terminal mockup blocks contain no explanatory prose
quiet mode never hides failure reason
```

This table is normative.

# 50. First-paint and live-heartbeat tests

Use a fake clock and fake terminal; no wall-clock sleeps.

Required:

```text
Run active with no declared Task beyond visibility delay → temporary `starting…` frame by 100ms
first concrete Task appears → temporary Run fallback disappears
Run/Task active beyond visibility delay → first live frame no later than 100ms
slow Verify → Task is Running and heartbeat remains visible
2s with no semantic progress → spinner frame changes at ≤100ms intervals
all Running rows share one animation clock
current activity changes → timer column remains stable
prompt begins → live animation quiesces
prompt ends with work remaining → heartbeat resumes
motion-disabled mode → no animation requirement
plain mode → no animation; 30s durable heartbeat policy
```

The clock starts from Evo's Running/live-region transition, not arbitrary process birth before Evo initialization.

# 51. Dry-run tests

Behavioral assertions matter more than golden text. Required cases:

```text
Define callback may run for read-only discovery
evo.File does not write/chmod/replace anything
evo.Exec does not spawn a process
typed mutation callback is not called
planned Effect exists when stale work would be needed
committed Effect does not exist
manifest is not advanced to speculative post-mutation state
human output uses planned tense
JSON reports dry_run mode and planned operation/effect state
```

Add a static-review/MCP fixture showing that direct `os.WriteFile`, database mutation, or raw mutating `exec.Command` inside a dry-run-capable `Define` is rejected as an unsafe opaque side effect. The runtime cannot reliably intercept arbitrary Go side effects.

---

# 52. API and wire compatibility tests

Current public compatibility remains a first-class constraint.

Required:

```text
existing JSON 1.0 fixtures remain semantically/byte compatible where promised
new JSON 2.0 validates against a committed JSON Schema
JSON mode stdout contains JSON only
JSONL mode stdout contains event lines only
human presentation in structured modes uses stderr only
failure/cancel still emit structured terminal record/document
human Verbosity does not remove JSON semantic fields
FormatHuman off-TTY becomes plain, not surprise-JSON
FormatData remains distinct from Evo run JSON
no internal Go field rename changes wire shape accidentally
```

Public API surface golden must show intentional additions (`Verify`, `File`, `Exec`, `Key`, `WriteJSON`, task-context errors, format/verbosity additions) and fail on accidental reintroduction of deprecated concepts.

# 53. CLI + HTTP use the same model

Application execution is reusable:

```go
func launchAgent(ctx context.Context, agent Agent) error {
    // declare Task / Group / Sequence and definitions
    return nil
}
```

CLI human mode configures `FormatHuman` once. CLI JSON/JSONL configures the corresponding format once; Tasks do not change.

HTTP / embedding uses an isolated Output so concurrent requests do not share package-default runtime state:

```go
out := evo.Init(evo.Config{
    Isolated: true,
    Format:   evo.FormatExternal,
    Stdout:   io.Discard,
    Stderr:   io.Discard,
})

result := out.Run(r.Context(), func(ctx context.Context) error {
    return launchAgent(ctx, defaultAgent())
})

if err := evo.WriteJSON(w, result); err != nil {
    // embedding application owns HTTP transport error handling
}
```

Normative encoder:

```go
func WriteJSON(w io.Writer, result Result) error
```

`WriteJSON` serializes the stable v2 `evo.run` wire document plus one trailing newline. It never serializes internal snapshots directly and applies the same redaction as CLI structured output. HTTP status is owned by the embedding application; it must not be derived by parsing a human message. Evo outcome/exit semantics remain present in the body.

# 54. MCP mission

The MCP is not only documentation search.

It should actively make correct adoption and upgrades easier.

The MCP should know:

- the current recommended beginner API;
- removed/deprecated concepts;
- scheduler semantics;
- declarative tracked operations;
- Evidence-as-boolean semantics;
- Basis/provenance and manifest semantics;
- dependency-graph invalidation rules;
- UI projection rules, including the 100ms heartbeat;
- machine schema versions;
- migration paths and anti-patterns;
- conformance tests.

It should guide developers toward the smallest correct change.

---

# 55. MCP guidance: common API

When asked how to use Evo, teach:

```go
task := evo.Task("name")
task.Define(fn)
```

then:

```text
Group for independent work
Sequence for ordered work
evo.File for declarative tracked file state
evo.Exec for a provenance-aware external process with declared outputs
Basis for semantic inputs that determine freshness
After only for exceptional scheduler edges
Facts for information
```

Teach `Verify` only when Evo cannot derive Evidence automatically from tracked state. Teach output format and verbosity once at the application boundary, never per Task.

Do not lead with advanced config, manifest internals, or renderer controls.

Do not teach named Evidence callbacks as the normal way to make file work idempotent.

---

# 56. MCP guidance: Evidence, provenance, and automatic operations

The MCP must distinguish stale and recommended semantics.

Legacy mutating Evidence:

```go
task.Evidence("write", func() error { return os.WriteFile(...) })
```

Flag it.

Intermediate named read-only checks may remain compatibility syntax, but the recommended advanced escape hatch is one boolean verifier:

```go
task.Verify(func(ctx context.Context) (bool, error) { ... })
```

Common file state:

```go
task.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{
        Path: path, Contents: contents, Mode: mode, Basis: basis,
    })
})
```

External generated artifact:

```go
task.Define(func(ctx context.Context) error {
    return evo.Exec(ctx, evo.ExecSpec{
        Executable: "python3",
        Args: []string{"generate.py", "input.xlsx", "build/out.bin"},
        Basis: []evo.Fingerprint{
            evo.FSPath("generate.py"),
            evo.FSPath("input.xlsx"),
        },
        Outputs: []string{"build/out.bin"},
    })
})
```

Teach the exact distinction:

```text
Current pre-Define Verify controls whole-Task fast skipping in v1.
Application fingerprint is definition identity/invalidation metadata, not positive Evidence by itself.
Evo-native operation identity controls operation-level no-op/recompilation once the current invocation is observed.
Basis explains operation freshness.
Evidence is a boolean current-state conclusion; only before-definition true Evidence skips Define.
```

Never claim that discovering `evo.File` at the end of a callback can skip expensive arbitrary code that already ran before it.

# 57. MCP review detections

Required rules:

```text
EVO-EVIDENCE-001
    legacy Evidence callback mutates state.

EVO-VERIFY-001
    Verify callback contains obvious mutation; verifier must be read-only.

EVO-FILE-001
    manual ordinary file reconciliation can be simplified with evo.File.

EVO-EXEC-001
    raw exec produces declared/generated files and manual freshness logic duplicates evo.Exec.

EVO-DRYRUN-001
    raw mutation occurs inside a Define path that promises Evo dry-run safety; route mutation through evo.File, evo.Exec, or a typed mutation boundary.

EVO-PROVENANCE-001
    generated artifact visibly reads semantic files/values omitted from Basis.

EVO-PROVENANCE-002
    docs/code claim prior manifest provenance discovered inside an opaque callback can skip that callback on a later run without current pre-definition proof.

EVO-DAG-001
    goroutine exists merely to make Evo Tasks parallel.

EVO-DAG-002
    After chain duplicates a Sequence.

EVO-DAG-003
    producer/consumer resource relationship is visible but first-run scheduler ordering is missing.

EVO-UI-001
    routine Facts manually printed as durable lines.

EVO-UI-002
    passing verification manually printed on normal success.

EVO-UI-003
    caller hand-builds collection/progress/status text Evo already models.

EVO-UI-004
    caller chooses glyph/color/free-form status state.

EVO-WIRE-001
    internal Snapshot/Result is marshaled directly as public API.

EVO-WIRE-002
    breaking JSON 1.0 edit without schema bump.

EVO-WIRE-003
    JSON/JSONL stdout is mixed with human presentation.

EVO-EXIT-001
    os.Exit bypasses Evo-derived conclusion.

EVO-LIVE-001
    fmt.Print* competes with Evo live rendering.
```

Findings include rule, severity, file/line, why, smallest migration, corrected syntax, and required target version. Do not invent Basis entries that source code does not justify.

# 58. MCP upgrade assistance

The MCP should be version-aware.

Given a consumer repository, it should:

```text
1. detect imported Evo version
2. inspect Evo usage patterns
3. compare them with the recommended API for the target version
4. separate mechanical migrations from semantic tracking/provenance decisions
5. produce a bounded migration plan
6. update call sites incrementally
7. run the consumer tests
8. report unresolved provenance/intent instead of inventing it
```

Do not perform broad rewrites when a mechanical migration suffices.

Do not infer a generated artifact's semantic Basis when the code does not make that relationship clear.

---

# 59. MCP version knowledge

Maintain machine-readable or easily parsable guidance for transitions such as:

```text
legacy presentation-only Task usage
→ scheduled Task/Group/Sequence

mutating Evidence wrapper
→ mutation in Define

manual os.WriteFile + chmod + read-only Evidence
→ evo.File declarative tracked operation where semantics match

named Evidence used only for common file state
→ derived Evidence from tracked file state

manual counters
→ Group/Sequence derived progress

manual JSON struct marshal
→ versioned stable encoder

hand-built skip text
→ Done resolution / AlreadySatisfied
```

The MCP must not recommend APIs that only exist in future/unreleased versions without stating the required target version.

---

# 60. MCP conformance tool

Add or extend an MCP review/conformance operation that can answer:

```text
Is this Evo usage current?
What is stale?
What is semantically unsafe?
What can be mechanically upgraded?
What provenance is explicit?
What provenance remains opaque?
What requires developer intent?
```

Output should be structured enough for an agent to act on.

Suggested fields:

```json
{
  "target_version": "next",
  "findings": [
    {
      "rule": "EVO-FILE-001",
      "severity": "suggestion",
      "file": "launchd.go",
      "line": 42,
      "summary": "manual file reconciliation can use evo.File",
      "migration": "Replace write/chmod/check boilerplate with one declarative tracked file operation"
    }
  ]
}
```

---

# 61. MCP spec linkage

The MCP's teaching and review rules must be tested against the same normative examples as the library.

Do not maintain separate contradictory prose.

Prefer:

```text
spec fixture
    ↓
library golden test
    ↓
MCP guidance fixture
```

When the recommended API or provenance semantics change, a failing MCP fixture should force the guidance to be upgraded too.

---

# 62. MCP regression tests

Required fixtures:

```text
beginner Task + Define → no finding
Group independent / Sequence ordered → no finding
After exceptional edge → no finding
evo.File desired state → no finding
evo.Exec generated output + Basis → no finding
Verify read-only custom state → no finding
mutating legacy Evidence → finding
mutating Verify → finding
manual common file reconciliation → simplification finding
raw exec + manual output hashes → evo.Exec suggestion
expensive work before trailing evo.File → warning that tracking cannot retroactively skip it
opaque callback → no false claim of precise provenance
same app fingerprint + prior manifest → no false whole-Task skip claim
producer/consumer with no first-run ordering → DAG finding
routine Facts printed manually → UI finding
direct os.Exit → finding
direct internal JSON marshal → finding
human text mixed into JSON stdout → finding
legacy API example → versioned migration guidance
```

MCP tests must verify version-specific advice and never fabricate semantic dependencies.

# 63. Repository implementation order

Implement in dependency order:

```text
1. stable Task/Group/Sequence identity + scheduler semantics
2. Result lifecycle/resolution model + Verify
3. application fingerprint service
4. fingerprint primitives (FSPath, Value, App)
5. manifest store, locking, corruption handling, atomic per-Task commit
6. evo.File reconciliation + verification/Facts/Effects
7. evo.Exec reconciliation + capture/activity + declared outputs
8. Verify-driven pre-definition Evidence fast path
9. freshness graph and known-producer barriers
10. Facts/Problems/Effects projection model
11. row-budget/verbosity renderer + 80ms shared heartbeat
12. plain milestone/30s heartbeat projection
13. FormatJSON / FormatJSONL automatic encoders + schema v2
14. HTTP stable writer
15. dry-run integration for File/Exec/effects/manifest
16. MCP guidance/review rules
17. migration docs + conformance matrix
```

Do not start by rewriting every example. Build one runtime truth and one deterministic test harness first.

# 64. Developer UX acceptance tests

## Static file

```go
task := seq.Task("write plist")
task.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{
        Path:     agent.PlistPath,
        Contents: agent.Plist,
        Mode:     agent.Mode,
    })
})
```

A junior developer should read this as:

> this Task establishes this file state; Evo avoids unnecessary file mutation and tracks what it established.

## External generated file

```go
task := seq.Task("generate flatbuffers")
task.Define(func(ctx context.Context) error {
    return evo.Exec(ctx, evo.ExecSpec{
        Executable: "python3",
        Args: []string{"generate.py", "schema.xlsx", "build/schema.bin"},
        Basis: []evo.Fingerprint{
            evo.FSPath("generate.py"),
            evo.FSPath("schema.xlsx"),
        },
        Outputs: []string{"build/schema.bin"},
    })
})
```

Read as:

> reconcile this generator invocation; spawn it only when its current semantic inputs/operation/output are no longer current.

Evo enters the Task callback on each run unless pre-definition Evidence (for example `Verify`) skips it. `evo.Exec` itself does not spawn when the **current invocation** has the same semantic identity/Basis/output. Unrelated Go application changes therefore do not force the Python generator to rerun unless `evo.App()` is explicitly part of its Basis.

## Multi-stage generated-artifact pipeline

```go
build := evo.Sequence("build artifacts")

normalize := build.Task("normalize schema")
normalize.Define(func(ctx context.Context) error {
    return evo.Exec(ctx, evo.ExecSpec{
        Executable: "python3",
        Args:       []string{"normalize.py", "schema.xlsx", "build/schema.json"},
        Basis: []evo.Fingerprint{
            evo.FSPath("normalize.py"),
            evo.FSPath("schema.xlsx"),
        },
        Outputs: []string{"build/schema.json"},
    })
})

compile := build.Task("compile flatbuffers")
compile.Define(func(ctx context.Context) error {
    return evo.Exec(ctx, evo.ExecSpec{
        Executable: "python3",
        Args:       []string{"compile.py", "build/schema.json", "build/schema.bin"},
        Basis: []evo.Fingerprint{
            evo.FSPath("compile.py"),
            evo.FSPath("build/schema.json"),
        },
        Outputs: []string{"build/schema.bin"},
    })
})
```

Required behavior:

```text
unchanged run
    both Define callbacks enter
    both Exec operations are current
    neither Python process spawns

schema.xlsx changes
    normalize.py runs
    if build/schema.json digest changes → compile.py runs
    if build/schema.json digest is byte-identical → invalidation stops; compile.py does not run

compile.py changes
    normalize.py stays current
    compile.py runs

unrelated Go binary changes
    callbacks enter
    both Python processes remain skipped/current
```

The `Sequence` is still required for first-run execution order. The provenance graph explains freshness and stops unnecessary downstream recompilation; it does not replace the scheduler graph.

## Custom observable state

```go
task := seq.Task("upgrade schema to 18")
task.Verify(func(ctx context.Context) (bool, error) {
    v, err := schemaVersion(ctx)
    return v >= 18, err
})
task.Define(upgradeTo18)
```

Read as:

> if the current world already proves this state, skip the definition.

No explanatory comments are required at the call site.

# 65. Final design thesis

The intended path is:

```text
declare Tasks and real execution dependencies
perform arbitrary work inside Define when needed
prefer Evo-native File/Exec operations for common reconcilable work
let those operations track outputs and semantic Basis
let current Verify handle custom observable state
let Evo persist safe manifest truth
let Evo derive pre-definition Evidence only from current proof that is actually available before callback entry
let Evo re-enter opaque callbacks conservatively instead of trusting prior runtime discovery
let Evo-native operations still no-op precisely from their current invocation inside those callbacks
let unchanged output fingerprints stop result invalidation downstream
let Facts/Problems/Effects remain structured
let automatic verbosity decide what humans need to see
let top-level Format project the same model as TTY/plain/JSON/JSONL/HTTP
```

The smallest durable distinctions are:

```text
Define performs opaque application work.
File / Exec are semantic operations Evo can reconcile.
Tracked resources remember observable results.
Basis explains operation freshness.
Verify proves custom current state.
Evidence says whether modeled desired state is currently satisfied; only pre-definition true Evidence skips Define.
Facts describe.
Problems carry consequence.
Effects record mutation.
```

Provenance rule:

```text
specific semantic operation proof
    → precise operation freshness

application fingerprint
    → conservative definition identity / invalidation signal

no current proof
    → execute or reconcile; never guess satisfied
```

Renderer rule:

```text
stable parent line
current activity underneath
normal mode spends rows only on useful truth
no unchanged default live-TTY frame for more than 100ms while Running
machine output retains the truth hidden from compact human output
```

Safety rule:

```text
false invalidation is acceptable
false already-satisfied is not
```

That is the pit of success.
