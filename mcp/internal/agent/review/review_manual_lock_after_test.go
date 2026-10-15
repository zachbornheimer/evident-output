package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// API-055 (ZYS-931): a caller-managed sync.Mutex/RWMutex Lock/Unlock wrapped
// around an evo.File call — the exact caller lock ZYS-840's automatic
// resource coordination now owns (File claims its own path for writing with
// no caller code).

const manualMutexAroundFileWriteSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  mu   sync.Mutex
  path string
}
func (w *Writer) write(ctx context.Context, contents []byte) error {
  w.mu.Lock()
  defer w.mu.Unlock()
  return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}
`

func TestAPI055_ManualMutexAroundFileWrite_Fires(t *testing.T) {
	res := review.GoSource("writer.go", manualMutexAroundFileWriteSrc)
	f := findingByID(t, res, "API-055")
	if f.Severity != "warning" {
		t.Fatalf("API-055 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("API-055 suggestion does not name evo.File as the fix: %q", f.Suggestion)
	}
}

const noManualMutexAroundFileWriteSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  path string
}
func (w *Writer) write(ctx context.Context, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}
`

func TestAPI055_NoManualMutexAroundFileWrite_StaysSilent(t *testing.T) {
	res := review.GoSource("writer.go", noManualMutexAroundFileWriteSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("false positive API-055 when File already owns its own coordination: %+v", f)
		}
	}
}

const mutexUnrelatedToEvoFileSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Service struct {
  mu    sync.Mutex
  count int
}
func (s *Service) touch() {
  s.mu.Lock()
  defer s.mu.Unlock()
  s.count++
}
func (s *Service) write(ctx context.Context, path string, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI055_MutexGuardingUnrelatedDomainState_StaysSilent(t *testing.T) {
	res := review.GoSource("service.go", mutexUnrelatedToEvoFileSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("false positive API-055 on a mutex guarding unrelated domain state: %+v", f)
		}
	}
}

const rwMutexRLockAroundFileReadSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  mu   sync.RWMutex
  path string
}
func (w *Writer) read(ctx context.Context) ([]byte, error) {
  w.mu.RLock()
  defer w.mu.RUnlock()
  var out []byte
  err := evo.File(ctx, evo.FileSpec{Path: w.path, Contents: out})
  return out, err
}
`

func TestAPI055_RWMutexRLockAroundFileRead_Fires(t *testing.T) {
	res := review.GoSource("writer.go", rwMutexRLockAroundFileReadSrc)
	f := findingByID(t, res, "API-055")
	if f.Severity != "warning" {
		t.Fatalf("API-055 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "w.mu.RLock()/w.mu.RUnlock()") {
		t.Fatalf("API-055 suggestion does not name the actual RLock()/RUnlock() pair: %q", f.Suggestion)
	}
	if strings.Contains(f.Suggestion, "w.mu.Lock()/w.mu.Unlock()") {
		t.Fatalf("API-055 suggestion wrongly names Lock()/Unlock() for RWMutex RLock code: %q", f.Suggestion)
	}
}

const rwMutexRLockScopeNarrowsAtRealRUnlockSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Writer struct {
  mu   sync.RWMutex
  path string
}
func (w *Writer) read(ctx context.Context) ([]byte, error) {
  w.mu.RLock()
  var out []byte
  err := evo.File(ctx, evo.FileSpec{Path: w.path, Contents: out})
  w.mu.RUnlock()
  if err != nil {
    return nil, err
  }
  return out, nil
}
`

func TestAPI055_RWMutexRLockScopeNarrowsAtRealRUnlock_Fires(t *testing.T) {
	res := review.GoSource("writer.go", rwMutexRLockScopeNarrowsAtRealRUnlockSrc)
	f := findingByID(t, res, "API-055")
	if !strings.Contains(f.Suggestion, "RLock()/w.mu.RUnlock()") {
		t.Fatalf("API-055 suggestion does not name the actual RLock()/RUnlock() pair: %q", f.Suggestion)
	}
}

func TestAPI055_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("writer.go", manualMutexAroundFileWriteSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("API-055 fired for a pin older than 1.1.0 (automatic resource claiming did not exist yet): %+v", f)
		}
	}
}

func TestAPI055_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("writer.go", manualMutexAroundFileWriteSrc)
	findingByID(t, res, "API-055")

	after := review.GoSource("writer.go", noManualMutexAroundFileWriteSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-055" {
			t.Fatalf("API-055 still fires after remediation to plain evo.File: %+v", f)
		}
	}
}

// API-056: a .After(...) edge kept only to avoid a race on shared/same-file
// state — both Tasks already claim that same resource (evo.File's own
// Path, or an EffectSpec Path/Resource claim), so Evo's
// automatic resource coordination (ZYS-840) already serializes them without
// the edge (ZYS-936).

const afterResourceContentionSameFileSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  // same file — avoid concurrent write race
  cacheTask.After(configTask)
}
`

func TestAPI056_AfterOnlyForSameFileContention_Fires(t *testing.T) {
	res := review.GoSource("run.go", afterResourceContentionSameFileSrc)
	f := findingByID(t, res, "API-056")
	if f.Severity != "warning" {
		t.Fatalf("API-056 severity = %q, want warning", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "delete") || !strings.Contains(f.Suggestion, "cacheTask.After(configTask)") {
		t.Fatalf("API-056 suggestion does not name the edge to delete: %q", f.Suggestion)
	}
	if !strings.Contains(f.Message, "config.json") {
		t.Fatalf("API-056 message does not name the shared resource: %q", f.Message)
	}
}

// afterResourceContentionEffectPathSrc: sweepTask and gcTask both DELETE
// the identical Path. Deleting an already-deleted path is idempotent,
// so which Task runs first never changes the outcome — a genuine
// order-invariant overlap, safe to report.
const afterResourceContentionEffectPathSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(sweepTask, gcTask *evo.Task) {
  sweepTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "repo cache", Path: "/repo/cache"}, removeRepoCache)
  })
  gcTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "repo cache", Path: "/repo/cache"}, removeRepoCache)
  })
  // shared resource, exclusive access only
  gcTask.After(sweepTask)
}
func removeRepoCache(context.Context) error { return nil }
`

func TestAPI056_AfterOnlyForEffectPathContention_Fires(t *testing.T) {
	res := review.GoSource("run.go", afterResourceContentionEffectPathSrc)
	findingByID(t, res, "API-056")
}

// afterResourceConflictingVerbsSrc: lockTask UPDATEs and sweepTask DELETEs
// the identical Path. Delete-then-update recreates the resource;
// update-then-delete removes it — the outcome genuinely depends on order,
// so this is a real dependency the rule must not tell the caller to delete.
const afterResourceConflictingVerbsSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(lockTask, sweepTask *evo.Task) {
  lockTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Update, Object: "repo", Path: "/repo"}, refreshRepo)
  })
  sweepTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "repo", Path: "/repo"}, removeRepo)
  })
  // shared resource, exclusive access only
  sweepTask.After(lockTask)
}
func refreshRepo(context.Context) error { return nil }
func removeRepo(context.Context) error  { return nil }
`

func TestAPI056_ConflictingVerbsEffectPath_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterResourceConflictingVerbsSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 when conflicting Verbs (Update vs Delete) on the same resource make order a real dependency: %+v", f)
		}
	}
}

// afterResourceDifferingContentsSrc: configTask and cacheTask both write
// config.json, but with different Contents. Deleting the .After edge (this
// rule's own remediation) would leave config.json's final content
// nondeterministic across runs — a resource claim only coordinates the
// overlap, it does not decide which write wins.
const afterResourceDifferingContentsSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: warm})
  })
  // same file — avoid concurrent write race
  cacheTask.After(configTask)
}
`

func TestAPI056_DifferingFileContents_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterResourceDifferingContentsSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 when the two File writes carry different Contents, so deleting .After leaves config.json nondeterministic: %+v", f)
		}
	}
}

const afterRealDependencySrc = `package p
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(prepareTask, buildTask *evo.Task) {
  prepareTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  buildTask.Define(func(ctx context.Context) error {
    _, err := os.ReadFile("config.json")
    return err
  })
  // buildTask needs the config prepareTask writes
  buildTask.After(prepareTask)
}
`

func TestAPI056_RealSemanticDependency_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterRealDependencySrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 on a real producer/consumer dependency: %+v", f)
		}
	}
}

const afterNoCommentSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.After(configTask)
}
`

func TestAPI056_NoContentionComment_StaysSilent(t *testing.T) {
	res := review.GoSource("run.go", afterNoCommentSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("false positive API-056 with no comment naming exclusion as the reason: %+v", f)
		}
	}
}

func TestAPI056_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("run.go", afterResourceContentionSameFileSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("API-056 fired for a pin older than 1.1.0 (automatic resource claims did not exist yet): %+v", f)
		}
	}
}

// afterResourceContentionRemediatedSrc is afterResourceContentionSameFileSrc
// with this rule's own Suggestion applied: cacheTask.After(configTask) and
// its comment are deleted, keeping only the overlapping File claim.
const afterResourceContentionRemediatedSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(configTask, cacheTask *evo.Task) {
  configTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
  cacheTask.Define(func(ctx context.Context) error {
    return evo.File(ctx, evo.FileSpec{Path: "config.json"})
  })
}
`

func TestAPI056_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("run.go", afterResourceContentionSameFileSrc)
	findingByID(t, res, "API-056")

	after := review.GoSource("run.go", afterResourceContentionRemediatedSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-056" {
			t.Fatalf("API-056 still fires after applying its own prescribed remediation (deleting the .After(...) edge): %+v", f)
		}
	}
}

// A mutex that guards real shared state (a map) as well as the File call
// must never be deleted: the suggestion narrows the critical section
// instead, and never tells the agent to remove the lock or its field.
const mutexGuardingMapAndFileSrc = `package p
import (
  "context"
  "sync"
  eo "github.com/zachbornheimer/evident-output/v2"
)
type Cache struct {
  mu   sync.Mutex
  seen map[string]int
}
func (c *Cache) store(ctx context.Context, path string, contents []byte) error {
  c.mu.Lock()
  defer c.mu.Unlock()
  c.seen[path]++
  return eo.File(ctx, eo.FileSpec{Path: path, Contents: contents})
}
`

func TestAPI055_MutexGuardingSharedStateAndFile_NeverSuggestsDelete(t *testing.T) {
	f := findingByID(t, review.GoSource("cache.go", mutexGuardingMapAndFileSrc), "API-055")
	if f.Severity != "warning" {
		t.Fatalf("API-055 severity = %q, want warning", f.Severity)
	}
	for _, banned := range []string{"delete", "remove", "field"} {
		if strings.Contains(strings.ToLower(f.Suggestion), banned) {
			t.Fatalf("API-055 suggestion would drop a lock that guards shared state (%q): %q", banned, f.Suggestion)
		}
	}
	if !strings.Contains(f.Suggestion, "c.mu.Unlock()") {
		t.Fatalf("API-055 suggestion does not name the lock to narrow: %q", f.Suggestion)
	}
}

// A lock-shaped call in a comment or string, or an evo.File mention in a
// string, is not code: the AST detector stays silent.
const lockAndFileOnlyInTextSrc = `package p
import (
  "context"
  "sync"
  evo "github.com/zachbornheimer/evident-output/v2"
)
type Doc struct {
  mu sync.Mutex
}
func (d *Doc) help(ctx context.Context) string {
  // d.mu.Lock() then evo.File(ctx, spec) was the old shape
  return "d.mu.Lock(); evo.File(ctx, evo.FileSpec{})"
}
`

func TestAPI055_LockAndFileOnlyInCommentsAndStrings_StaysSilent(t *testing.T) {
	assertNoRule(t, review.GoSource("doc.go", lockAndFileOnlyInTextSrc), "API-055")
}

// The File-only region keeps the removal suggestion but never mentions
// deleting the mutex field, which may guard other methods.
func TestAPI055_FileOnlyRegion_DoesNotSuggestDeletingField(t *testing.T) {
	f := findingByID(t, review.GoSource("writer.go", manualMutexAroundFileWriteSrc), "API-055")
	if strings.Contains(f.Suggestion, "field") {
		t.Fatalf("API-055 suggestion tells the agent to delete the mutex field: %q", f.Suggestion)
	}
	if !strings.Contains(f.Suggestion, "w.mu.Lock()/w.mu.Unlock()") {
		t.Fatalf("API-055 suggestion does not name the lock pair: %q", f.Suggestion)
	}
}

// A logical Resource name and a filesystem Path never overlap, even when
// their text is equal, so the edge between them is not redundant.
const afterPathVersusResourceNameSrc = `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(sweepTask, gcTask *evo.Task) {
  sweepTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "cache", Quantity: 1, Path: "cache"}, removeCache)
  })
  gcTask.Define(func(ctx context.Context) error {
    return evo.Effect(ctx, evo.EffectSpec{Verb: evo.Delete, Object: "cache", Quantity: 1, Resource: "cache"}, removeCache)
  })
  // shared resource, exclusive access only
  gcTask.After(sweepTask)
}
func removeCache(context.Context) error { return nil }
`

func TestAPI056_PathVersusResourceName_StaysSilent(t *testing.T) {
	assertNoRule(t, review.GoSource("run.go", afterPathVersusResourceNameSrc), "API-056")
}

func TestAPI053_NestedPatchInsideEffectClaim_Fires(t *testing.T) {
	src := `package p
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func moveTree(ctx context.Context, root string, diff []byte) error {
  spec := evo.EffectSpec{Object: "tree", Verb: evo.Update, Quantity: 1, Resource: "tree"}
  return evo.Effect(ctx, spec, func(ctx context.Context) error {
    return evo.Patch(ctx, diff)
  })
}
`
	findingByID(t, review.GoSource("tree.go", src), "API-053")
}
