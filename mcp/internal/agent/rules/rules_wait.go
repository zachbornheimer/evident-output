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
			Invariant:  "the container that owns child Task scheduling also owns deriving their aggregate outcome; a caller does not store child handles merely to loop over them and hand-count failures",
			Why:        "zq's runParallel (internal/app/app.go) keeps a []*evo.Task, checks each child, counts failed children, and builds its own \"N of N failed\" error; waitDefinedRunOperations (internal/app/run_execute.go) loops Tasks, special-cases a Task that never started, and returns the first remaining error. Both reimplement what a Group already does: its outcome folds its members', and work declared After(group) runs only once the members succeeded or were Excluded.",
			BadCode: `var handles []*evo.Task
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
			GoodCode: `jobs := r.Group("jobs")
for _, item := range items {
  jobs.Task(item.Name).Define(func(ctx context.Context) error { return run(item) })
}
r.Task("report").After(jobs).Define(report)`,
			Remediation:     "Delete the stored-handle slice, the loop and the hand-counted aggregate error; declare each child under a Group and order later work with After(group), or read a result with evo.Compute and Computed.Get, once every child is declared",
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
			Why:        "evo.Effect only claims EffectSpec.Resource for its fn callback's duration when it is set; a second evo.File or Resource-claiming evo.Effect call made with that same held context — moving a worktree's Effect whose fn also writes a marker File at the destination, say — is misuse (CONTRACT.md: holding two claims at once) and fails at apply time, even when the second resource is free, because holding at most one claim at a time is what makes deadlock impossible by construction. Catching it in review turns a runtime failure into a review finding before it ships.",
			BadCode: `spec := evo.EffectSpec{Object: "worktree", Verb: evo.Update, Path: from}
return evo.Effect(ctx, spec, func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: to, Contents: marker}) // nested: ctx already holds "from"
})`,
			GoodCode: `if err := evo.File(ctx, evo.FileSpec{Path: to, Contents: marker}); err != nil {
  return fmt.Errorf("write marker %s: %w", to, err)
}
spec := evo.EffectSpec{Object: "worktree", Verb: evo.Update, Quantity: 1, Path: from}
return evo.Effect(ctx, spec, func(ctx context.Context) error {
  return os.Rename(from, to)
})`,
			Remediation:     "Finish and return from the first evo.Effect/evo.File before starting a second, or claim one coarser Path or Resource name that both mutations share instead of nesting a second acquisition",
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
			Invariant:  "application code does not manage mutexes, lock files, or unlock lifecycle around an Evo-managed File path; File claims write-side ownership of its own path automatically, and overlapping File and Effect claims already wait on each other",
			Why:        "A caller-managed sync.Mutex/RWMutex wrapped around an evo.File call (ZYS-931) is invisible to Evo's own resource coordination (ZYS-840): it cannot see a contended wait, cannot render \"waiting for <path>\" the way a real resource claim does, and is pure redundancy once File already serializes writers on its own Path — or a false sense of safety if the two coordination layers ever disagree about ordering. Remove the lock and let File own the path; when the work is not itself a File write, claim the same path explicitly with EffectSpec.Path so it still overlaps File on that path.",
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
  return evo.File(ctx, evo.FileSpec{Path: w.path, Contents: contents})
}
// A non-File operation over the same path claims it explicitly instead:
func (w *Writer) archive(ctx context.Context) error {
  return evo.Effect(ctx, evo.EffectSpec{
    Verb: evo.Update, Object: "archive", Quantity: 1,
    Path: w.path,
  }, func(ctx context.Context) error { return archive(w.path) })
}`,
			Remediation:     "When the Lock()/Unlock() pair guards nothing but the evo.File call, drop the pair; File already claims its own path for writing. When the lock also guards other shared state, keep it and end the critical section before calling evo.File, because File can wait on its resource claim while the caller lock is held. Never delete a mutex field on this rule alone; other methods may depend on it. For an opaque (non-File) mutation over the same path, claim it with EffectSpec.Path instead of a caller lock — never a bare callback for tracked file state.",
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
			Invariant:  "a child.After(parent) edge exists to declare a real semantic dependency; it is never kept only to avoid a data race that File and Effect's own automatic path claim (ZYS-840) already serializes AND whose overlapping writes are order-invariant (identical writes, or an idempotent Verb like Delete) — a resource claim only coordinates the overlap, it never decides which write wins, so an edge guarding two writes with different outcomes stays",
			Why:        "Before ZYS-840, two Tasks writing the same file/shared state had no automatic exclusion, so pinning one After the other was the only way to avoid a race, and the reason usually shows up as a comment (\"same file\", \"avoid race\", \"exclusive access\") next to the edge. Now that File and Effect auto-claim and serialize any overlapping write, an edge guarding two IDENTICAL writes no longer does anything a resource claim doesn't already do — it only couples two Tasks' scheduling that would otherwise run concurrently, which costs wall-clock time and reads as a real dependency to the next person who touches the DAG. An edge guarding two DIFFERENT writes (different Contents, or conflicting Verbs like Update vs Delete) is not this case: the resource claim only prevents concurrent corruption, it does not pin which write is final, so deleting .After there would make the outcome nondeterministic across runs — that edge is a real dependency and must stay.",
			BadCode: `configTask.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
})
cacheWarmTask.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
})
// same file — avoid concurrent write race
cacheWarmTask.After(configTask)`,
			GoodCode: `configTask.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
})
cacheWarmTask.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: "config.json", Contents: cfg})
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
