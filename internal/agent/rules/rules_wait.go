package rules

// waitRules is the Wait and Resource family (1.1): wait on containers, not
// hand-rolled loops (API-052); hold one resource at a time (API-053); let
// resource claims, not manual locks or After edges, serialize conflicting
// work (API-055, API-056).
func waitRules() []Rule {
	return []Rule{
		{
			ID:         "API-052",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "the container that owns child Task scheduling also owns waiting for its descendants and deriving their aggregate outcome; a caller does not store child handles merely to loop Wait, filter ErrNotStarted, Snapshot the container, and hand-count failures",
			Why:        "zq's runParallel (internal/app/app.go) keeps []*evo.TaskHandle, loops task.Wait(), Snapshots the Group, counts failed children, and builds its own \"N of N failed\" error; waitDefinedRunOperations (internal/app/run_execute.go) loops Tasks, special-cases evo.ErrNotStarted, and returns the first remaining error. Both reimplement exactly what GroupHandle.Wait()/SequenceHandle.Wait() (ZYS-849) now does natively, including ErrNotStarted-from-a-failed-predecessor suppression and declaration-order error joining.",
			BadCode: `var handles []*evo.TaskHandle
for _, item := range items {
  t := jobs.Task(item.Name)
  t.Define(func(ctx context.Context) error { return run(item) })
  handles = append(handles, t)
}
failed := 0
for _, h := range handles {
  if err := h.Wait(); err != nil {
    failed++
  }
}
if failed > 0 {
  return fmt.Errorf("%d of %d failed", failed, len(handles))
}`,
			GoodCode: `for _, item := range items {
  jobs.Task(item.Name).Define(func(ctx context.Context) error { return run(item) })
}
return jobs.Wait()`,
			Remediation:     "Delete the stored-handle slice, the Wait loop, the Snapshot, and the hand-counted aggregate error; call the owning GroupHandle/SequenceHandle's own Wait() after every child is declared",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-052"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-053",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityError,
			Invariant:  "generic resource access holds at most one Resource at a time (ZYS-840); code that already holds a Resource — directly, or through any helper it hands its context to — never asks for a second one",
			Why:        "evo.Effect only claims spec.Resource for its fn callback's duration when spec.Resource is set; a second evo.File or Resource-claiming evo.Effect call made with that same held context — moving a worktree's Effect whose fn also writes a marker File at the destination, say — fails deterministically with evo.ErrNestedResourceAcquisition at apply time, even when the second resource is free, because holding at most one Resource at a time is what makes deadlock impossible by construction. Catching it in review turns a runtime failure into a review finding before it ships.",
			BadCode: `spec := evo.EffectSpec{Object: "worktree", Verb: evo.EffectUpdate, Resource: evo.FSResource(from)}
return evo.Effect(ctx, spec, func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: to, Contents: marker}) // nested: ctx already holds "from"
})`,
			GoodCode: `if err := (evo.File{Path: to, Content: evo.Bytes(marker)}).Write(ctx); err != nil {
  return err
}
spec := evo.EffectSpec{Object: "worktree", Verb: evo.EffectUpdate, Resource: evo.FSResource(from)}
return evo.Effect(ctx, spec, func(ctx context.Context) error {
  return os.Rename(from, to)
})`,
			Remediation:     "Finish and return from the first evo.Effect/evo.File before starting a second, or claim one coarser Resource (e.g. evo.FSResource covering both paths) that both mutations share instead of nesting a second acquisition",
			RelatedGuidance: []string{"evo-file-exec", "common-api"},
			VerificationIDs: []string{"API-053"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-055",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "application code does not manage mutexes, lock files, or unlock lifecycle around an Evo-managed File path; File claims write-side ownership of its own path automatically, and overlapping File/Basis/Effect claims already wait on each other",
			Why:        "A caller-managed lock wrapped around an evo.File/Files/Patch/Effect call (ZYS-931, ZYS-831) is invisible to Evo's own resource coordination (ZYS-840). The lock may be a sync.Mutex/RWMutex, a flock(2) on a file descriptor (syscall.Flock, unix.Flock), an O_EXCL lock file removed afterwards, or a gofrs/flock value. It cannot see a contended wait, cannot render \"waiting for <path>\" the way a real resource claim does, and is pure redundancy once File already serializes writers on its own Path — or a false sense of safety if the two coordination layers ever disagree about ordering. Remove the lock and let File own the path; when the work is not itself a File write, claim the same path explicitly with evo.FSResource so it still overlaps File/Basis on that path.",
			BadCode: `type Writer struct {
  mu   sync.Mutex
  path string
}
func (w *Writer) write(ctx context.Context, contents []byte) error {
  w.mu.Lock()
  defer w.mu.Unlock()
  return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}`,
			GoodCode: `type Writer struct {
  path string
}
func (w *Writer) write(ctx context.Context, contents []byte) error {
  return evo.File{Path: w.path, Content: evo.Bytes(contents)}.Write(ctx)
}
// A non-File operation over the same path claims it explicitly instead:
func (w *Writer) archive(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{
    Verb: evo.EffectUpdate, Object: "archive", Quantity: 1,
    Resource: evo.FSResource(w.path),
  }, func(ctx context.Context) error { return archive(w.path) })
}`,
			Remediation:     "When the lock (Lock()/Unlock(), flock LOCK_EX/LOCK_UN, an O_EXCL lock file plus os.Remove, or gofrs/flock) guards nothing but the evo.File call, drop it; File already claims its own path for writing. When the lock also guards other shared state, keep it and end the critical section before calling evo.File, because File can wait on its resource claim while the caller lock is held. Never delete a mutex field on this rule alone; other methods may depend on it. For an opaque (non-File) mutation over the same path, claim it with evo.Effect's EffectSpec.Resource: evo.FSResource(path) instead of a caller lock — never a bare Write(func...) callback for tracked file state.",
			RelatedGuidance: []string{"evo-file-exec", "common-api"},
			VerificationIDs: []string{"API-055"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
		{
			ID:         "API-056",
			MinDialect: "1.1.0",
			Category:   "API",
			Severity:   SeverityWarning,
			Invariant:  "a child.After(parent) edge exists to declare a real semantic dependency; it is never kept only to avoid a data race that File/FSResource/LogicalResource's own automatic resource claim (ZYS-840) already serializes AND whose overlapping writes are order-invariant (identical writes, or an idempotent Verb like Delete) — a resource claim only coordinates the overlap, it never decides which write wins, so an edge guarding two writes with different outcomes stays",
			Why:        "Before ZYS-840, two Tasks writing the same file/shared state had no automatic exclusion, so pinning one After the other was the only way to avoid a race, and the reason usually shows up as a comment (\"same file\", \"avoid race\", \"exclusive access\") next to the edge. Now that File/FSResource/LogicalResource auto-claim and serialize any overlapping write, an edge guarding two IDENTICAL writes no longer does anything a resource claim doesn't already do — it only couples two Tasks' scheduling that would otherwise run concurrently, which costs wall-clock time and reads as a real dependency to the next person who touches the DAG. An edge guarding two DIFFERENT writes (different Contents, or conflicting Verbs like Update vs Delete) is not this case: the resource claim only prevents concurrent corruption, it does not pin which write is final, so deleting .After there would make the outcome nondeterministic across runs — that edge is a real dependency and must stay.",
			BadCode: `configTask.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
})
cacheWarmTask.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
})
// same file — avoid concurrent write race
cacheWarmTask.After(configTask)`,
			GoodCode: `configTask.Define(func(ctx context.Context) error {
  return evo.File{Path: "config.json", Content: evo.Bytes(cfg)}.Write(ctx)
})
cacheWarmTask.Define(func(ctx context.Context) error {
  return evo.File{Path: "config.json", Content: evo.Bytes(cfg)}.Write(ctx)
})
// no .After: both Tasks write the identical config.json, and File already
// claims the path and serializes the overlap`,
			Remediation:     "Delete the .After(...) edge only when the overlapping writes are order-invariant — identical Contents, or an idempotent Verb such as Delete on both sides. Keep .After when the writes differ (different Contents, or conflicting Verbs like Update vs Delete): the resource claim serializes them but does not decide which write wins, so order is still a real dependency there.",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"API-056"},
			Since:           "1.1.0",
			Certainty:       CertaintyHeuristic,
		},
	}
}

func init() { registerFamily(waitRules()) }
