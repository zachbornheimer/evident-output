# ZYS-1382 contract decisions

Names and shapes the contract tests under `conformance/zys1382/*/contract_test.go`
depend on. Where the ticket left syntax open, the smallest shape consistent with
its examples was chosen and used identically across all ten primitives.
Implementers follow these names; a different spelling fails the contract.

Tests are external packages importing `evo "github.com/zachbornheimer/evident-output"`.
They do not compile until the surface below exists. That is expected.

## File and Tree

```go
type File struct {
    Path    string
    Content FileContent   // nil = no desired content declared; never "absent"
    Mode    fs.FileMode   // 0 = 0644 on create, preserve on an existing file
}

type Tree struct {
    Path    string
    Content TreeContent   // nil = no desired content declared; never "absent"
}
```

Plain structs; literals work; all methods have **value receivers** so
`evo.File{...}.Write(ctx)` and `tree.Write` method values compile.

Shared vocabulary (identical signatures on both):

| Method                                 | File                               | Tree                                                                                              |
| -------------------------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------------- |
| `Read(ctx)`                            | `([]byte, error)` observed bytes   | `([]File, error)` regular files, sorted by path, absolute `Path`, `Content == nil` (content-lazy) |
| `Write(ctx) error`                     | establish Content (+Mode)          | establish Content (replace the whole tree)                                                        |
| `Verify(ctx) error`                    | nil or `ErrVerifyMismatch`         | nil or `ErrVerifyMismatch`                                                                        |
| `Equal(ctx, other File) (bool, error)` | bytes only (mode ignored)          | `Equal(ctx, other Tree, opts ...ChecksumOption)`: structure + contents, honours `Exclude`         |
| `Remove(ctx) error`                    | delete; absent path is success     | delete recursively; absent path is success                                                        |
| `Checksum(ctx) (string, error)`        | lowercase hex SHA-256 of the bytes | `Checksum(ctx, opts ...ChecksumOption)`: Merkle digest, same hex form                             |

Semantics pinned by tests:

- Read/Checksum/Verify of a missing path: `fs.ErrNotExist` via `errors.Is`
  (Verify of a missing path with declared Content is `ErrVerifyMismatch`).
- Write with nil Content: `ErrContentMissing`, path untouched.
- `Bytes(nil)` / `Bytes("")` establish an **empty regular file**, never remove.
- Write replaces by atomic rename (new inode when content differs), leaves no
  siblings/temporaries in the destination's parent, and is a no-op (same
  inode) when already satisfied.
- Write verifies after commit; `Write` then `Verify` on the same value is nil.
- DryRun: Write and Remove mutate nothing and return nil (Patch still validates).
- Called outside a Task: `ErrNoTaskContext`, nothing mutated.
- Empty `Path`: `ErrPathMissing` (one error for File and Tree).
- Cardinality mismatch on Remove: `ErrFilePathTypeMismatch` (File on a dir),
  `ErrTreePathTypeMismatch` (Tree on a regular file); nothing deleted.
- Tree.Write replaces the tree wholesale: entries absent from the desired
  tree are gone afterwards.
- Concurrent Writes to one destination from sibling Tasks all succeed and the
  file holds exactly one writer's whole content; callers write no lock code.

## Content producers (sealed)

```go
type FileContent interface{ /* unexported marker */ }   // Bytes, Download
type TreeContent interface{ /* unexported marker */ }   // Extract

func Bytes[T ~string | ~[]byte](v T) FileContent          // Bytes("x") and Bytes([]byte) both compile

type Download struct {
    URL       string
    Integrity string   // SRI "sha512-<b64>" / "sha384-" / "sha256-", or plain hex sha256 / sha1
}

type Extract struct {
    File File     // the archive; its Content is irrelevant, Path is read
    Root string   // optional leading component to strip ("package" or "package/")
}
```

Interfaces are exported **names** whose only method is unexported, so callers
can hold a value (`var c evo.FileContent = evo.Bytes(...)`) but cannot
implement one. There is no public "File or Tree" superclass.

Download:

- Verified against Integrity before anything reaches the destination.
  Mismatch, unparseable, or empty Integrity: `ErrIntegrityMismatch`; the
  destination (and any previous content) is untouched; no temporaries left.
- Non-2xx response: `ErrDownloadFailed`, nothing published.
- Empty URL: `ErrDownloadURLMissing`.
- Destination already matching Integrity: no HTTP request at all.
- `Verify` checks Integrity on disk without fetching.
- DryRun: no request, no publish. Cancelled ctx: `context.Canceled`, no publish.

Extract:

- Formats: tar.gz, tar, zip, detected from content (not extension).
- Malformed/empty archive: `ErrExtractMalformed`.
- Unsafe entries: `ErrExtractUnsafeEntry` for path traversal, absolute paths,
  entries outside `Root`, symlink/hardlink escapes (including writing through
  a symlinked dir), char/block devices, fifos. Nothing published on rejection.
- Owner execute bit preserved from the archive.
- Whole tree published atomically; parent holds only the destination.
- Missing archive file: `fs.ErrNotExist`.

## Checksum

```go
type ChecksumOption   // opaque
func Exclude(pattern string) ChecksumOption
```

- One internal engine; `File.Checksum` and the leaf digest inside
  `Tree.Checksum` are the same function (tests: equal leaves ⇒ equal trees,
  changed leaf ⇒ changed tree; rename/move with identical bytes changes the
  tree digest).
- Digest string form: lowercase hex, 64 chars (`sha256.Size*2`) for both.
- `Exclude` is a Go regexp matched (unanchored, `regexp.MatchString`) against
  `"/" + slash-separated path inside the tree`; directories carry a trailing
  `/`. The ticket's `.*\/\.git\/.*` therefore excludes a `.git` subtree at the
  root and at any depth. The tree's own root location is never matched.
- Several `Exclude` options combine (union).
- `File.Checksum` takes no options.

## Find

```go
func Find(ctx context.Context, root string, names ...string) ([]File, error)
```

- Matcher: exact **base-name** match (`"package.json"`, `"pnpm-lock.yaml"`);
  a name containing a separator matches nothing.
- Returns regular files only, absolute `Path`, sorted by path, `Content == nil`.
- No names: `ErrFindNamesMissing`. Missing root: `fs.ErrNotExist`.
- Does not follow symlinked directories.
- Honours `context.Canceled` / `context.DeadlineExceeded`.
- Never blocks on, or blocks, a concurrent `File.Write` under the same root.
- Identical concurrent searches agree on results (coalesced).

## Exec

```go
type Exec struct {
    Path    string
    Args    []string
    Dir     string
    Env     []string   // os/exec vocabulary: "KEY=VALUE"; nil inherits the parent
    Outputs Outputs
}
type Outputs []fsState           // sealed element type; literal evo.Outputs{evo.File{...}, evo.Tree{...}} compiles
func (x Exec) Run(ctx context.Context) (ExecResult, error)
```

`ExecResult` keeps `Ran, ExitCode, Stdout, Stderr, Truncated`.

- `Path` resolves through `PATH` when bare. Missing executable:
  `ErrExecExecutableNotFound`, `Ran == false`.
- Empty Path: `ErrExecPathMissing`. Outside a Task: `ErrNoTaskContext`.
- Nonzero exit: `ErrExecNonzeroExit`, result still carries captured output.
- Relative Output paths resolve against `Dir`. Declared Output missing after
  exit 0: `ErrExecOutputMissingAfterSuccess`. Output with declared Content is
  verified (`ErrVerifyMismatch` on difference).
- DryRun: not spawned, `Ran == false`, nil error.
- Captured stdout reaches the run's evidence (rendered at `VerbosityVerbose`).
- Cancelled ctx: `context.Canceled`, child ended promptly.

## Basis

```go
func (t *TaskHandle) Basis(inputs ...basisInput) *TaskHandle   // sealed: File, Tree, Fingerprint (Value/App)
```

- Chainable, before `Define`. After Define: misuse recorded as
  `ErrBasisAfterDefine` on `Output.Err()`, call ignored.
- Freshness: with a shared `StateDir`, an unchanged Basis makes the Task
  **current** (Define callback does not run); any changed identity reruns it.
  No prior record: runs.
- Identity is content: mode/mtime-only changes do not rerun.
- Not ordering: a Task whose Basis names a file does not wait for a Task that
  writes it. Not a lock: Tasks sharing a Basis file run concurrently.
- `evo.Value` and `evo.App` remain valid inputs. `FSPath` is gone (File/Tree
  are the filesystem identity).

## Patch

```go
func Patch(ctx context.Context, diff []byte) error
```

- Paths resolve against the working directory at Run start.
- All-or-nothing: applicability of every file is validated before any commit
  (`ErrPatchDoesNotApply` leaves every file untouched).
- Standard forms applied: modify, create (parent dirs created), delete,
  rename (with or without edit), mode change (with or without edit).
- `ErrPatchMalformed` (not a diff), `ErrPatchDoesNotApply` (hunk mismatch,
  missing file, create over existing, delete with other content, rename onto
  an occupied path), `ErrPatchUnsupported` (binary).
- `ErrPatchUnsafePath`: `..` traversal, absolute paths, paths through a
  symlinked directory. Nothing written.
- Concurrent edit between read and commit: `ErrPatchStale` (or
  `ErrPatchDoesNotApply`); the concurrent edit is never overwritten.
- Commits through File: atomic replacement (new inode), no temporaries, the
  result verifies as `File{Path, Content: Bytes(result)}`.
- Already applied: nil. DryRun: nothing mutated but applicability still
  validated. Outside a Task: `ErrNoTaskContext`.

## Error names introduced

`ErrPathMissing`, `ErrContentMissing`, `ErrVerifyMismatch`,
`ErrTreePathTypeMismatch`, `ErrIntegrityMismatch`, `ErrDownloadFailed`,
`ErrDownloadURLMissing`, `ErrExtractMalformed`, `ErrExtractUnsafeEntry`,
`ErrFindNamesMissing`, `ErrExecPathMissing`, `ErrBasisAfterDefine`,
`ErrPatchUnsafePath`, `ErrPatchStale`.

Kept: `ErrNoTaskContext`, `ErrFilePathTypeMismatch`, `ErrExecExecutableNotFound`,
`ErrExecNonzeroExit`, `ErrExecOutputMissingAfterSuccess`, `ErrPatchMalformed`,
`ErrPatchDoesNotApply`, `ErrPatchUnsupported`.

## Test conventions

- Each package defines its own small `contractRun(t, cfg, fn)` helper: an
  `Isolated`, `Plain` Output with a temp `StateDir`, one Task, fn inside
  Define, `Wait` + `Finish`. No shared test package, so each directory stands
  alone when implementation lands piecemeal.
- Patch tests use `t.Chdir` (Go 1.24+) for the workspace, so they are not
  parallel.
- Exec tests run real `/bin/sh` processes rather than the ProcessRunner
  facade: the contract is about observable process behavior.

## Disputes

### 2026-10-01 download: gaps the contract leaves open (not changed, flagged)

- Content-Encoding: the contract does not say whether Integrity covers wire
  bytes or decoded bytes (adversarial-research.md flags it). Proposal: Integrity
  covers the bytes as served (no transparent decoding), matching npm's stored
  tarball integrity. No test pins this yet.
- Retry/resume: Download makes no stated retry or Range-resume promise. Tests
  only pin the safety property (a failed attempt never publishes and never
  corrupts a later attempt). Pin retry counts only if the ticket wants them.
- Non-HTTP schemes (file://, ftp://) and multi-token SRI strings
  ("sha256-... sha512-...", strongest wins) are untested/extension respectively;
  strongest-wins is kept as W3C SRI semantics, scheme policy is undecided.

### 2026-10-01 find: gaps and over-reach flagged (not changed)

- Liveness on slow searches (SPEC) has no deterministic test: Find has no
  directory-read facade or clock to slow down. Proposal: a `ReadDir` method on
  the FileFS facade (or a `DirFS` facade) so tests can inject latency and a
  counter. The same counter would pin coalescing (one traversal for N callers)
  and "bounded parallelism" exactly instead of via rlimit and goroutine peaks.
- Unreadable root returns `fs.ErrPermission` (test
  `TestFindOfAnUnreadableRootIsPermissionDenied`); the decisions only pin a
  missing root. Unreadable subdirectories may be skipped or reported (the test
  accepts either, error must name the directory). Decide and pin one.
- Unspecified, untested: Find outside a Task (File methods return
  `ErrNoTaskContext`), root that is a file or a symlink to a directory, and
  DryRun behaviour (Find is read-only, so proposal: unaffected).

### 2026-10-01 checksum: the engine's seam needs a streaming read (not changed, flagged)

- The only injectable seam for the checksum engine is `Config.FileFS`, whose
  content method is `ReadFile` (whole-file buffer). `TestAdversarial_FileChecksumStreams`
  requires streaming, and the routing tests (`forgingFS`, `countingFS`,
  `failingFS`) already supply an `Open(path string) (fs.File, error)` method;
  earlier test comments cited this Disputes entry before it was written.
  Proposal: the engine reads content through an optional interface
  `interface{ Open(string) (fs.File, error) }` when the FileFS implements it,
  falling back to `ReadFile`. Adding `Open` to `FileFS` itself would break
  consumer FileFS implementations (see `internal/engine/file_mode_unmanaged_test.go`
  `v10FileFS`). Routing tests pass under either choice.
- Tree walking has no seam: FileFS has no `ReadDir`, so the routing tests prove
  leaf content routing only, not directory enumeration. Same proposal as the
  find dispute (a `ReadDir` facade method) would let a test count traversals.
- Unspecified, untested: `File.Checksum`/`Tree.Checksum` outside a Task (the
  write rule is `ErrNoTaskContext`; for a read-only method, proposal: allowed,
  with the default FileFS), `File.Checksum` of a symlink to a regular file, and
  `Tree.Checksum` whose `Path` is a symlink to a directory.
- Pinned consequences of the decided Exclude rule (directories carry a trailing
  `/`, paths start with `/`): a regular file named `.git` (a linked worktree's
  gitfile) is NOT excluded by `.*\/\.git\/.*`; excluding every file in a
  directory leaves that directory as an empty directory in the digest. If
  either is unwanted, the rule needs changing, not the tests.

## Rulings (2026-10-01, step 3.0a skeleton)

Each dispute above is settled here. Implementers follow these; a test that
disagrees is changed in the same commit as the behavior.

- **`Bytes(nil)` does not compile.** `Bytes[T ~string | ~[]byte]` has no core
  type, so an untyped `nil` cannot infer `T`. The generic signature stays (the
  shape tests pin it). The three test call sites now spell the nil case
  `evo.Bytes([]byte(nil))`; the semantics are unchanged (empty regular file).
- **Download, Content-Encoding:** Integrity covers the bytes as served. No
  transparent decoding.
- **Download, retry/resume:** no retry or Range-resume promise. Only the
  safety property is contract: a failed attempt never publishes and never
  corrupts a later attempt.
- **Download, schemes:** `http` and `https` only. Any other scheme fails with
  `ErrDownloadFailed` before any I/O. Multi-token SRI keeps W3C semantics
  (strongest algorithm wins).
- **Directory enumeration seam (Find and Tree.Checksum):** the engine reads
  directories through an optional `interface{ ReadDir(string) ([]fs.DirEntry, error) }`
  on the configured `FileFS`, falling back to the OS. `FileFS` itself does not
  grow a method (that would break consumer implementations). Tests may then
  inject latency and count traversals for liveness and coalescing.
- **Checksum streaming seam:** accepted as proposed. The engine streams through
  an optional `interface{ Open(string) (fs.File, error) }` on `FileFS`,
  falling back to `ReadFile`.
- **Find, unreadable root:** `fs.ErrPermission` (as tested). **Unreadable
  subdirectory:** Find fails with an error that wraps `fs.ErrPermission` and
  names the directory. It never skips silently.
- **Find, root that is a regular file:** `ErrTreePathTypeMismatch`. **Root that
  is a symlink to a directory:** the root is resolved; symlinks below it are
  never followed. **DryRun:** unaffected (read-only).
- **Read-only methods outside a Task** (`Find`, `File.Read`, `File.Checksum`,
  `File.Equal`, `Tree.Read`, `Tree.Checksum`, `Tree.Equal`): allowed, using the
  default `FileFS`. Methods that establish or check declared state (`Write`,
  `Verify`, `Remove`, `Exec.Run`, `Patch`) return `ErrNoTaskContext`.
- **Symlinks at a File's Path:** `ErrFilePathIsSymlink` for every File method,
  `Checksum` included. **Tree.Path that is a symlink to a directory:** resolved,
  like a Find root; entries below are never followed.
- **Exclude consequences** (a `.git` gitfile is not excluded by `.*\/\.git\/.*`;
  excluding every file leaves the empty directory in the digest): accepted as
  pinned.
- **Fingerprint:** now a struct built only by `Value` and `App`, so it can
  satisfy the sealed Basis input. Caller-implemented fingerprints are no longer
  Basis inputs; File and Tree are the filesystem identities.
- **Release:** the removed 1.1 shapes are retired in 1.2
  (`rules.RetiredRelease1_2`), matching the CHANGELOG's "Added (1.2)" section.
