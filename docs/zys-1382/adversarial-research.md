# ZYS-1382 adversarial research

How mature tools implement and harden each ZYS-1382 primitive, what went wrong
for them, and which test in `conformance/zys1382/<primitive>/adversarial_test.go`
holds Evo to the hardened behavior. Each test is named for the weakness it
guards. Every primitive also has at least one deterministic structural
performance test (no wall-clock thresholds) and Go benchmarks for the shapes
zq's package work hits.

The tests are written against the ZYS-1382 target API (plain `evo.File` /
`evo.Tree` structs, `evo.Bytes`, `evo.Download`, `evo.Extract`, `evo.Find`,
`evo.Exec{...}.Run`, `task.Basis`, `evo.Patch(ctx, diff) error`). They fail to
compile until that API lands; that is the intended red state.

## Names

The tests follow `docs/zys-1382/contract-decisions.md` (File/Tree methods,
`Bytes`, `Download`, `Extract`, `Exclude`, `Find`, `Exec{...}.Run`,
`task.Basis`, `Patch(ctx, diff) error`, and the error names
`ErrExtractUnsafeEntry`, `ErrIntegrityMismatch`, `ErrDownloadFailed`,
`ErrPatchUnsafePath`, `ErrPatchDoesNotApply`, `ErrPatchMalformed`,
`ErrContentMissing`, `ErrPathMissing`, `ErrVerifyMismatch`,
`ErrFilePathTypeMismatch`, `ErrTreePathTypeMismatch`). One assumption goes
beyond that file: the "reads each file once" checksum test counts reads
through `evo.Config.FileFS`, so the checksum engine must read file content
through that facade's `ReadFile` (or the facade must grow a counted stream
method and the test follows it).

Package names are `<primitive>_test` (directories hold only `_test.go` files).
Helpers in the adversarial files are prefixed `adv` so they cannot collide
with `contract_test.go` helpers in the same package.

## File (atomic establish of one regular file)

| Weakness                                                                                                                                                                             | Source                                                                                                                         | Test                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------ |
| In-place `write()` exposes torn or empty content to concurrent readers; only temp-file + `rename(2)` in the same directory is atomic.                                                | google/renameio docs; natefinch/atomic; npm `write-file-atomic`                                                                | `TestAdversarial_TornWriteNeverVisibleToConcurrentReader`                                              |
| Writing through an existing hardlink mutates every other link to the inode. pnpm's content-addressable store hardlinks into `node_modules`, so an in-place write corrupts the store. | pnpm store design; renameio (rename replaces the dirent, never the inode)                                                      | `TestAdversarial_WriteDoesNotMutateHardlinkedInode`                                                    |
| Following a destination symlink lets a planted link redirect the write anywhere (classic `/tmp` symlink attack).                                                                     | CWE-59; GNU patch CVE-2019-13636 (same bug class)                                                                              | `TestAdversarial_WriteDoesNotFollowDestinationSymlink`                                                 |
| A directory at the destination gets clobbered or merged into.                                                                                                                        | renameio refuses non-regular targets                                                                                           | `TestAdversarial_WriteRefusesDirectoryAtPath`                                                          |
| Temp files are created 0600 (or umask-filtered); after rename the published mode is not the declared one.                                                                            | `write-file-atomic` mode/chown handling; renameio `WithPermissions`                                                            | `TestAdversarial_ModeIsExactRegardlessOfUmask`                                                         |
| Cancelled or failed writes leak `.tmp-*` siblings that later tools pick up.                                                                                                          | renameio `Cleanup()`; write-file-atomic temp cleanup on error                                                                  | `TestAdversarial_CanceledWritePublishesNothingAndLeaksNoTemp`                                          |
| Two processes doing read-modify-rename interleave; ad-hoc lockfiles survive a crash and wedge every later run. OS-backed `flock`/`fcntl` locks die with the process.                 | SQLite locking docs (POSIX advisory locks); LevelDB `LOCK` file uses `flock`                                                   | `TestAdversarial_ConcurrentProcessesNeverInterleave`, `TestAdversarial_CrashedWriterLeavesNoStaleLock` |
| In-process concurrent writers race the same rename.                                                                                                                                  | renameio (no coordination by design)                                                                                           | `TestAdversarial_ConcurrentInProcessWritersSerialize`                                                  |
| Post-commit drift is never noticed; "verification after commit" must compare bytes, not trust the write.                                                                             | SQLite/LevelDB fsync lessons: rename without directory fsync can be lost on power failure, so trust must come from observation | `TestAdversarial_VerifyDetectsOutOfBandEdit`                                                           |
| Rewriting already-current content churns inode and mtime, which invalidates mtime-based tools and wastes I/O.                                                                        | make/ninja mtime model; apenwarr "mtime comparison considered harmful"                                                         | `TestAdversarial_CurrentContentIsNotRewritten` (structural)                                            |

Not covered by a deterministic test: durability across power loss (fsync of
file and parent directory, `F_FULLFSYNC` on macOS). That needs fault
injection below the filesystem; the implementation must still do it.

Benchmarks: `BenchmarkFileWrite_4KiBChanged`, `BenchmarkFileWrite_AlreadyCurrent`.

## Tree (atomic establish of one directory tree)

| Weakness                                                                                                                              | Source                                                             | Test                                                        |
| ------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ | ----------------------------------------------------------- |
| A failure halfway through populating the destination leaves a mixed old/new tree. Stage the whole tree, then publish with one rename. | Docker layer apply; npm `pacote` extracts to a temp dir then moves | `TestAdversarial_FailedWritePreservesPreviousTree`          |
| Extracting over an existing tree merges, leaving stale files from the old version.                                                    | npm "reify" stale file bugs; Go module cache uses fresh dirs       | `TestAdversarial_ReplaceLeavesNoResidue`                    |
| A destination that is a symlink gets written through.                                                                                 | Docker CVE-2018-15664 (`docker cp` symlink race)                   | `TestAdversarial_DestinationSymlinkNotWrittenThrough`       |
| Concurrent writers interleave entries from two versions.                                                                              | pnpm/npm concurrent install corruption reports                     | `TestAdversarial_ConcurrentWritersPublishWholeTrees`        |
| Observers see a half-populated tree during the write.                                                                                 | same as above                                                      | `TestAdversarial_ObserverNeverSeesPartialTree` (structural) |
| Staging directories leak into the destination's parent on failure.                                                                    | renameio-style cleanup                                             | `TestAdversarial_FailedWriteLeavesNoStagingInParent`        |
| Drift (an added file) goes unnoticed.                                                                                                 | Nix store verification (`nix-store --verify`)                      | `TestAdversarial_VerifyDetectsAddedFile`                    |
| Cancellation still publishes.                                                                                                         | context cancellation contract                                      | `TestAdversarial_CanceledWritePublishesNothing`             |

Benchmark: `BenchmarkTreeWrite_2000Files`.

## Checksum (one engine, File and Tree)

| Weakness                                                                                                                                              | Source                                                                                                                 | Test                                                                          |
| ----------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| Naive concatenation of names and contents collides (`a`+`bc` vs `ab`+`c`). Every field needs length-prefixed or typed framing.                        | Git tree objects (`<mode> <name>\0<oid>`); Nix NAR length-prefixed strings                                             | `TestAdversarial_FramingDoesNotCollide`                                       |
| Empty directories and file-vs-directory type are part of structure; Git drops empty dirs, NAR and Bazel do not. The SPEC says "structure + contents". | Nix NAR spec; Bazel REAPI `Directory` messages                                                                         | `TestAdversarial_EmptyDirectoryAndTypeAreStructure`                           |
| Directory listing order (inode/creation order) leaks into the digest. Sort entries by bytes.                                                          | Git sorts tree entries; NAR sorts; OCI tar layers are order-sensitive, which is why their digests are not reproducible | `TestAdversarial_CreationOrderIrrelevant`                                     |
| Including mtime/atime makes identical trees hash differently.                                                                                         | NAR omits timestamps; Bazel omits mtime; `tar --mtime` reproducible-builds work                                        | `TestAdversarial_TimestampsIgnored`                                           |
| Including the absolute root path makes copies of a tree hash differently, defeating cache hits.                                                       | Bazel action-cache relocatability                                                                                      | `TestAdversarial_RootLocationIgnored`                                         |
| Following symlinks reads outside the tree and loops forever. Hash the link target text instead.                                                       | NAR stores symlink target; Git stores link text as a blob                                                              | `TestAdversarial_SymlinkNotFollowed`, `TestAdversarial_SymlinkLoopTerminates` |
| Opening a FIFO blocks forever.                                                                                                                        | ripgrep/fd skip non-regular files; GNU tar warns on sockets                                                            | `TestAdversarial_FIFODoesNotBlock`                                            |
| Unreadable files silently skipped or reported as missing produce a false "unchanged".                                                                 | Evo `FSPath` contract; Bazel errors on unreadable inputs                                                               | `TestAdversarial_UnreadableFileIsError`                                       |
| Exclusion anchored on the absolute path excludes the whole tree when the tree itself lives under a `.git` directory.                                  | `.gitignore` patterns are relative to the repo; SPEC: match the path inside the tree                                   | `TestAdversarial_ExcludeMatchesInsidePathOnly`                                |
| An invalid exclusion regex silently matches nothing.                                                                                                  | fail-closed principle                                                                                                  | `TestAdversarial_InvalidExcludeIsError`                                       |
| Excluded subtrees are still read (cost, and errors from unreadable `.git/objects`).                                                                   | Bazel/Buck skip excluded inputs entirely                                                                               | `TestAdversarial_ExcludedSubtreeIsNotRead` (structural)                       |
| Files are read more than once (stat-then-read-then-verify).                                                                                           | Bazel digest cache; Buck2 file digest dedupe                                                                           | `TestAdversarial_TreeChecksumReadsEachFileOnce` (structural)                  |
| Whole-file buffering blows memory on large blobs.                                                                                                     | Go `io.Copy` into a hash; NAR streaming                                                                                | `TestAdversarial_FileChecksumStreams` (structural)                            |

Benchmarks: `BenchmarkTreeChecksum_3000SmallFiles`, `BenchmarkFileChecksum_16MiB`.

## Download (File content producer with integrity)

| Weakness                                                                                                                                    | Source                                                                                                  | Test                                                                                                     |
| ------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Publishing before integrity is checked exposes attacker bytes.                                                                              | npm/pnpm `ssri` verifies while streaming to cache tmp; Go module proxy + sumdb verify before extracting | `TestAdversarial_IntegrityMismatchNeverPublishes`                                                        |
| A truncated transfer that happens to match a prefix hash, or an early EOF treated as success.                                               | Go `net/http` `io.ErrUnexpectedEOF`; cargo checksum on whole crate                                      | `TestAdversarial_TruncatedBodyRejected`                                                                  |
| Slow-retrieval and endless-data attacks hang or fill disk.                                                                                  | TUF threat model ("slow retrieval", "endless data")                                                     | `TestAdversarial_StalledServerHonorsDeadline`, `TestAdversarial_EndlessBodyHonorsDeadlineAndLeaksNoTemp` |
| Missing integrity silently means "trust anything".                                                                                          | npm `package-lock` integrity, cargo `Cargo.lock` checksum are mandatory for registry deps               | `TestAdversarial_MissingIntegrityRejected`                                                               |
| Malformed integrity strings parsed as "no constraint". `ssri` ignores unknown tokens, which is fine only when a known strong token remains. | W3C SRI spec, `ssri`                                                                                    | `TestAdversarial_MalformedIntegrityRejected`                                                             |
| Weak algorithms (md5) accepted as integrity.                                                                                                | W3C SRI allows only sha256/384/512                                                                      | `TestAdversarial_WeakAlgorithmRejected`                                                                  |
| Downgrade: a weak matching hash wins over a strong mismatching one. SRI: the strongest algorithm present must match.                        | W3C SRI "getPrioritizedHashFunction"; `ssri.checkData` picks strongest                                  | `TestAdversarial_StrongestAlgorithmWins`                                                                 |
| Plain hex digests of the wrong length are truncated-compared.                                                                               | cargo/Go sumdb fixed-length digests                                                                     | `TestAdversarial_HexDigestLengthEnforced`                                                                |
| An error page (404/500) whose bytes match the integrity is published. Status must be 2xx.                                                   | npm-registry-fetch status checks                                                                        | `TestAdversarial_NonSuccessStatusNeverPublishes`                                                         |
| Redirect loops.                                                                                                                             | Go `http.Client` stops after 10 redirects                                                               | `TestAdversarial_RedirectLoopBounded`                                                                    |
| Credentials in the URL leak into errors and logs.                                                                                           | Go `url.URL.Redacted`; npm `npm-registry-fetch` cleans auth from errors                                 | `TestAdversarial_URLCredentialsRedactedInErrors`                                                         |
| Concurrent downloads to one destination interleave or leak temp files.                                                                      | pnpm store concurrent writers; cacache uses tmp + move                                                  | `TestAdversarial_ConcurrentSameDestinationPublishesOnce`                                                 |
| Re-downloading when the destination already has the declared integrity.                                                                     | pnpm store hit; Go module cache                                                                         | `TestAdversarial_CurrentDestinationMakesNoRequest` (structural)                                          |
| Unbounded parallel fetches exhaust sockets.                                                                                                 | pnpm `network-concurrency`; cargo `http.multiplexing`                                                   | `TestAdversarial_ParallelDownloadsAreBounded` (structural)                                               |

Not covered: `Content-Encoding` transparent decompression (whether integrity
covers wire bytes or decoded bytes). npm integrity is over the stored tarball;
Go's transport only auto-decodes when it set `Accept-Encoding` itself. The
contract should decide; flagged for contract-decisions.md.

Benchmark: `BenchmarkDownload_Parallel32x64KiB`.

## Extract (Tree content producer from an archive)

| Weakness                                                                                      | Source                                                                                            | Test                                                                                              |
| --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| `../` entry names escape the destination (zip-slip).                                          | Snyk "Zip Slip" (2018); Go 1.20 `GODEBUG=zipinsecurepath`/`tarinsecurepath`; Python CVE-2007-4559 | `TestAdversarial_ParentTraversalRejected` (tar and zip)                                           |
| Absolute entry names write anywhere.                                                          | node-tar CVE-2021-32804                                                                           | `TestAdversarial_AbsolutePathRejected`                                                            |
| A symlink entry pointing outside, followed by a file written through it.                      | node-tar CVE-2021-32803; Python PEP 706 `data` filter                                             | `TestAdversarial_SymlinkEscapeThenWriteThrough`, `TestAdversarial_SymlinkPointingOutsideRejected` |
| A directory entry later replaced by a same-name symlink, bypassing a "known directory" cache. | node-tar CVE-2021-32803 / CVE-2021-37701                                                          | `TestAdversarial_DirectoryReplacedBySymlink`                                                      |
| Backslash treated as a separator by the safety check but not by the filesystem.               | node-tar CVE-2021-37701                                                                           | `TestAdversarial_BackslashSeparatorConfusion`                                                     |
| Case-insensitive filesystems alias `Foo` (symlink) and `foo/` (dir).                          | Git CVE-2021-21300; node-tar CVE-2021-37712                                                       | `TestAdversarial_CaseInsensitiveSymlinkCollision`                                                 |
| Hardlink entries targeting files outside the destination.                                     | Python PEP 706 `data` filter; GNU tar hardlink checks                                             | `TestAdversarial_HardlinkEscapeRejected`                                                          |
| Device/FIFO entries.                                                                          | PEP 706 `data` filter refuses special files                                                       | `TestAdversarial_SpecialFilesRejected`                                                            |
| setuid/setgid bits preserved from untrusted archives.                                         | PEP 706 `data` filter clears them; GNU tar `--no-same-permissions` default for non-root           | `TestAdversarial_SetuidAndSetgidStripped`                                                         |
| Root stripping by string prefix: `packagex/evil` treated as under `package`.                  | npm `tar --strip`; common string-prefix path bugs                                                 | `TestAdversarial_RootPrefixIsPathSegment`                                                         |
| `Root/../..` survives stripping.                                                              | same                                                                                              | `TestAdversarial_RootTraversalRejected`                                                           |
| A malicious entry late in the archive leaves earlier entries published.                       | PEP 706 notes partial extraction on error                                                         | `TestAdversarial_LateFailurePublishesNothing`                                                     |
| Cancellation ignored on big archives.                                                         | context contract                                                                                  | `TestAdversarial_CanceledExtractPublishesNothing`                                                 |
| Re-extracting an archive over an identical tree rewrites everything.                          | pnpm store "already extracted" check                                                              | `TestAdversarial_CurrentTreeIsNotRewritten` (structural)                                          |

Not covered: decompression bombs (a tiny gzip expanding to many GiB). A
deterministic test needs a size-limit policy the SPEC does not define yet;
cancellation is covered. Flagged for contract-decisions.md.

Benchmarks: `BenchmarkExtract_5000FileTarGz`, `BenchmarkExtract_2000FileZip`.

## Find (discovery)

| Weakness                                              | Source                                                                  | Test                                                                                                            |
| ----------------------------------------------------- | ----------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| Following directory symlinks loops forever.           | `walkdir` loop detection (used by ripgrep/fd); `fastwalk` `Follow` docs | `TestAdversarial_SymlinkLoopTerminates`                                                                         |
| Following symlinks escapes the root.                  | fd/ripgrep default to not following                                     | `TestAdversarial_SymlinkedDirectoryOutsideRootNotFollowed`                                                      |
| Name matching as substring or glob (`.` as wildcard). | fd regex-by-default surprises; SPEC says exact names                    | `TestAdversarial_ExactBasenameNotSubstringOrGlob`                                                               |
| Reading file content during discovery.                | SPEC "content-lazy"                                                     | `TestAdversarial_UnreadableMatchesStillFound` (structural: zero reads), `TestAdversarial_ResultsCarryNoContent` |
| Opening a FIFO named like a target blocks.            | fd/ripgrep skip special files                                           | `TestAdversarial_FIFONamedLikeTargetDoesNotBlock`                                                               |
| Cancellation not honored on wide trees.               | ripgrep stops walkers on quit                                           | `TestAdversarial_CanceledContextStopsWalk`                                                                      |
| An unreadable subdirectory aborts or hangs the walk.  | ripgrep/fd report and continue                                          | `TestAdversarial_UnreadableDirectoryDoesNotHang`                                                                |
| Parallel walkers return nondeterministic order.       | fd `--threads` order is unstable unless sorted                          | `TestAdversarial_ResultOrderIsDeterministic`                                                                    |
| Concurrent identical searches disagree.               | SPEC coalescing                                                         | `TestAdversarial_ConcurrentIdenticalSearchesAgree`                                                              |
| A missing root is silently "no matches".              | fd/ripgrep exit with an error                                           | `TestAdversarial_MissingRootIsError`                                                                            |

Not covered deterministically: coalescing count (needs a directory-read
counter on the facade) and liveness rendering when slow.

Benchmark: `BenchmarkFind_WideTree`.

## Exec (process execution)

| Weakness                                                                     | Source                                                                      | Test                                                     |
| ---------------------------------------------------------------------------- | --------------------------------------------------------------------------- | -------------------------------------------------------- |
| Arguments passed through a shell get interpreted.                            | Python `subprocess(shell=True)` injection; Rust CVE-2024-24576 (BatBadBut)  | `TestAdversarial_ArgsNeverInterpretedByShell`            |
| PATH lookup resolving to the current directory.                              | Go blog "Command PATH security in Go" (Go 1.19 `exec.ErrDot`)               | `TestAdversarial_RelativePathEntryNotResolved`           |
| Cancellation kills the child but not its grandchildren. Use a process group. | Go issue 22485-style reports; Python `start_new_session`; `exec.Cmd.Cancel` | `TestAdversarial_CancellationKillsProcessGroup`          |
| A grandchild holding stdout open makes `Wait` hang forever.                  | Go 1.20 `exec.Cmd.WaitDelay` (issue 23019)                                  | `TestAdversarial_GrandchildHoldingPipesDoesNotHang`      |
| Unbounded capture of a chatty child exhausts memory.                         | Python `communicate()` buffering; Go `bytes.Buffer` capture                 | `TestAdversarial_OutputFloodIsBounded` (structural)      |
| Inheriting an interactive stdin blocks on terminal input.                    | `os/exec` defaults Stdin to /dev/null                                       | `TestAdversarial_StdinIsNotInherited`                    |
| Death by signal reported as success or as exit code 0.                       | `ExitError.ExitCode()` returns -1 on signal                                 | `TestAdversarial_SignalDeathIsFailure`                   |
| Deadline expiry misclassified as an ordinary failure.                        | context contract                                                            | `TestAdversarial_DeadlineIsClassifiedAsDeadline`         |
| Duplicate env keys: first-wins vs last-wins confusion.                       | Go 1.9+ `os/exec` dedupes, last wins                                        | `TestAdversarial_DuplicateEnvLastWins`                   |
| Secrets leak into captured output and rendered evidence.                     | GitHub Actions secret masking; Evo `Redactor`                               | `TestAdversarial_SecretsRedactedInEvidence`              |
| A missing executable classified as a nonzero exit.                           | `exec.ErrNotFound` vs `ExitError`                                           | `TestAdversarial_MissingExecutableIsNotAnExit`           |
| `Dir` implemented with `os.Chdir`, racing every goroutine.                   | Go `os.Chdir` is process-wide                                               | `TestAdversarial_DirDoesNotChangeParentWorkingDirectory` |
| More than one spawn per call (retry/probe).                                  | SPEC "Evo owns lifetime"                                                    | `TestAdversarial_OneSpawnPerCall` (structural)           |
| Unbounded parallel spawns.                                                   | make `-j`, ninja pools                                                      | `TestAdversarial_ConcurrentExecsAreBounded` (structural) |

Benchmark: `BenchmarkExec_True`.

## Basis (Task freshness)

| Weakness                                                                                           | Source                                                                  | Test                                                                 |
| -------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | -------------------------------------------------------------------- |
| mtime/size stamps miss a same-size edit within timestamp granularity ("racy git").                 | Git `racy-git.txt`; apenwarr "mtime comparison considered harmful"      | `TestAdversarial_ContentChangeWithRestoredStampsReruns`              |
| Touch without content change forces rebuilds.                                                      | make; ninja `restat`                                                    | `TestAdversarial_TouchWithoutContentChangeDoesNotRerun` (structural) |
| An input edited while the action runs is recorded as current. Bazel checks inputs after execution. | Bazel "file modified during build" detection                            | `TestAdversarial_EditDuringRunForcesRerun`                           |
| Missing-then-present inputs not tracked.                                                           | ninja does not track new files matching a glob; Bazel globs are tracked | `TestAdversarial_MissingBasisAppearingReruns`                        |
| New files in a directory input ignored (glob staleness).                                           | make/ninja directory deps; Bazel Merkle input root                      | `TestAdversarial_TreeBasisNewFileReruns`                             |
| Renames with identical content ignored.                                                            | content-only hashing without names                                      | `TestAdversarial_TreeBasisRenameReruns`                              |
| Symlink retargeting ignored.                                                                       | NAR hashes link text                                                    | `TestAdversarial_SymlinkTargetChangeReruns`                          |
| A failed run recorded as current.                                                                  | Bazel never caches failed actions                                       | `TestAdversarial_FailedRunIsNeverCurrent`                            |
| Two tasks sharing a Basis share freshness. Action keys must include identity.                      | Bazel action key = command + inputs + env                               | `TestAdversarial_FreshnessIsPerTask`                                 |
| Unreadable input treated as unchanged.                                                             | Evo `FSPath`: unreadable is an error                                    | `TestAdversarial_UnreadableBasisNeverCurrent`                        |

Benchmark: `BenchmarkBasisCurrentCheck_Tree2000Files`.

## Remove

| Weakness                                                                     | Source                                                                                                            | Test                                                                                         |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Recursive delete follows a symlinked directory and deletes outside content.  | Go `os.RemoveAll` is safe; many `rm -rf` reimplementations were not (CVE-2022-21658 Rust `remove_dir_all` TOCTOU) | `TestAdversarial_SymlinkedDirectoryInsideTreeNotFollowed`                                    |
| Removing a symlinked root deletes the target's contents.                     | Rust CVE-2022-21658                                                                                               | `TestAdversarial_SymlinkedRootNotFollowed`                                                   |
| File.Remove on a symlink deletes the target.                                 | CWE-59                                                                                                            | `TestAdversarial_FileRemoveOnSymlinkRemovesLinkOnly`                                         |
| Type confusion: File.Remove deletes a directory, Tree.Remove deletes a file. | SPEC                                                                                                              | `TestAdversarial_FileRemoveRefusesDirectory`, `TestAdversarial_TreeRemoveRefusesRegularFile` |
| Read-only directories (Go module cache is 0555) make removal fail.           | `go clean -modcache` needs chmod; Go issue 27161                                                                  | `TestAdversarial_ReadOnlyDirectoriesRemoved`                                                 |
| Empty path resolves to the working directory and deletes it.                 | Steam `rm -rf "$STEAMROOT/"*` incident (2015)                                                                     | `TestAdversarial_EmptyPathNeverMeansWorkingDirectory`                                        |
| Nil content treated as "make absent".                                        | SPEC                                                                                                              | `TestAdversarial_NilContentWriteNeverDeletes`                                                |
| Missing target is an error. Removal must be idempotent.                      | desired-state semantics                                                                                           | `TestAdversarial_RemoveMissingIsIdempotent`                                                  |
| Cancellation still deletes.                                                  | context contract                                                                                                  | `TestAdversarial_CanceledRemoveLeavesTargetIntact`                                           |
| Removal reads contents (slow, fails on unreadable files).                    | `unlink` needs only directory write permission                                                                    | `TestAdversarial_RemoveNeverReadsContents` (structural)                                      |

Benchmark: `BenchmarkTreeRemove_3000Files`.

## Patch

| Weakness                                                                    | Source                                                                        | Test                                                |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------- | --------------------------------------------------- |
| `../` paths write outside the workspace.                                    | GNU patch CVE-2010-4651; git apply rejects `..`                               | `TestAdversarial_ParentTraversalRejected`           |
| Absolute paths.                                                             | GNU patch strips only with `-p`; git apply rejects                            | `TestAdversarial_AbsolutePathRejected`              |
| Writing beyond a symlinked directory.                                       | Git CVE-2023-23946; GNU patch CVE-2019-13636                                  | `TestAdversarial_PathBeyondSymlinkRejected`         |
| Paths inside `.git` (and case variants on case-insensitive FS) plant hooks. | Git CVE-2014-9390                                                             | `TestAdversarial_GitDirectoryRejected`              |
| Partial application across files.                                           | git apply is all-or-nothing by default; GNU patch writes `.rej` and continues | `TestAdversarial_AllOrNothingAcrossFiles`           |
| Fuzzy matching silently applies to the wrong lines.                         | GNU patch fuzz factor 2 default; git apply refuses fuzz                       | `TestAdversarial_NoFuzzyApplication`                |
| Ed-style scripts execute commands.                                          | GNU patch CVE-2018-1000156, CVE-2019-13638                                    | `TestAdversarial_EdScriptRejectedNotExecuted`       |
| Header line counts that disagree with the hunk body.                        | git apply "corrupt patch" check                                               | `TestAdversarial_HunkCountMismatchRejected`         |
| Create over an existing file silently overwrites.                           | git apply "already exists in working directory"                               | `TestAdversarial_CreateOverExistingRejected`        |
| Delete of a file whose content no longer matches.                           | git apply verifies preimage on delete                                         | `TestAdversarial_DeleteRequiresExactPreimage`       |
| Rename to an out-of-scope path.                                             | Git CVE-2023-23946 exploit used renames                                       | `TestAdversarial_RenameOutOfScopeRejected`          |
| setuid modes from a diff header.                                            | git only honors 100644/100755                                                 | `TestAdversarial_SetuidModeNeverApplied`            |
| Re-applying an applied patch rewrites files.                                | git apply `--check` reports already applied                                   | `TestAdversarial_AlreadyAppliedIsNoOp` (structural) |

Not covered deterministically: a concurrent edit between Patch's validation
and commit (needs a hook inside the lock). The SPEC requires it; the contract
tests should cover the revalidation path through a facade.

Benchmark: `BenchmarkPatch_100Files`.

## Sources

- google/renameio: https://pkg.go.dev/github.com/google/renameio
- natefinch/atomic: https://github.com/natefinch/atomic
- npm write-file-atomic: https://github.com/npm/write-file-atomic
- SQLite file locking and atomic commit: https://www.sqlite.org/lockingv3.html, https://www.sqlite.org/atomiccommit.html
- Git racy-git: https://git-scm.com/docs/racy-git
- apenwarr, "mtime comparison considered harmful": https://apenwarr.ca/log/20181113
- Nix NAR format (Eelco Dolstra thesis, ch. 5.2.1): https://edolstra.github.io/pubs/phd-thesis.pdf
- Bazel remote execution API (Merkle Directory): https://github.com/bazelbuild/remote-apis
- OCI image spec, descriptors and digests: https://github.com/opencontainers/image-spec/blob/main/descriptor.md
- W3C Subresource Integrity: https://www.w3.org/TR/SRI/
- npm ssri: https://github.com/npm/ssri
- Go module checksum database: https://go.dev/ref/mod#checksum-database
- TUF security model: https://theupdateframework.io/security/
- Snyk Zip Slip: https://security.snyk.io/research/zip-slip-vulnerability
- node-tar CVE-2021-32803, CVE-2021-32804, CVE-2021-37701: https://github.blog/security/vulnerability-research/github-security-update-vulnerabilities-tar-npmcli-arborist/, https://nvd.nist.gov/vuln/detail/CVE-2021-37701
- Python PEP 706 extraction filters: https://peps.python.org/pep-0706/
- Go 1.20 insecure path GODEBUGs: https://go.dev/doc/godebug
- Docker CVE-2018-15664: https://nvd.nist.gov/vuln/detail/CVE-2018-15664
- Git CVE-2021-21300: https://github.com/git/git/security/advisories/GHSA-8prw-h3cq-mghm
- Git CVE-2023-23946: https://github.com/git/git/security/advisories/GHSA-r87m-v37r-cwfh
- Git CVE-2014-9390: https://nvd.nist.gov/vuln/detail/CVE-2014-9390
- GNU patch CVE-2018-1000156, CVE-2019-13636, CVE-2019-13638: https://github.com/irsl/gnu-patch-vulnerabilities
- Go "Command PATH security in Go": https://go.dev/blog/path-security
- Go exec.Cmd.WaitDelay (issue 23019): https://github.com/golang/go/issues/23019
- Rust CVE-2024-24576 (BatBadBut): https://blog.rust-lang.org/2024/04/09/cve-2024-24576.html
- Rust CVE-2022-21658 (remove_dir_all TOCTOU): https://blog.rust-lang.org/2022/01/20/cve-2022-21658.html
- walkdir loop detection: https://docs.rs/walkdir/latest/walkdir/struct.WalkDir.html#method.follow_links
- fastwalk: https://github.com/charlievieth/fastwalk
- Bazel action cache and input modification checks: https://bazel.build/remote/caching
