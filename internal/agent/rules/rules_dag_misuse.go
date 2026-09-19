package rules

func dagMisuseRules() []Rule {
	return []Rule{
		{
			ID:        "EVO-DAG-004",
			Category:  "EVO",
			Severity:  "warning",
			Invariant: "application code does not invent a lock table or //fix-schedule/ path around Evo",
			Why:       "writeLocks/readLocks and a synthetic //fix-schedule/ path reimplement scheduling Evo already owns. File/Patch serialize same-path work through process-local resource holds; Group already overlaps independent Tasks.",
			BadCode: `writeLocks := map[string]struct{}{path: {}}
//fix-schedule/serialize
t.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})
`,
			GoodCode: `t := out.Task("write config")
t.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})
`,
			Remediation:     "Delete the custom writeLocks/readLocks or //fix-schedule/ scheduler; declare work with Task.Define and evo.File",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-DAG-004"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-DAG-005",
			Category:  "EVO",
			Severity:  "warning",
			Invariant: "a caller-owned goroutine does not Wait on a Task merely to sequence Evo work",
			Why:       "go task.Wait() (or a goroutine whose body is Wait) is a second scheduler. Sequence already runs children in declaration order; After is the exceptional DAG edge.",
			BadCode: `go func() {
  _ = write.Wait()
}()
read.Define(func(ctx context.Context) error { return readAfter(ctx) })
`,
			GoodCode: `seq := evo.Sequence("apply greeting")
write := seq.Task("write config")
write.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})
read := seq.Task("read config")
read.Define(func(ctx context.Context) error { return readAfter(ctx) })
`,
			Remediation:     "Delete the goroutine and declare the work under evo.Sequence so ordering is a scheduler edge, not Task.Wait",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-DAG-005"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-DAG-006",
			Category:  "EVO",
			Severity:  "warning",
			Invariant: "After is a DAG edge, not a lock for same-path File contention",
			Why:       "Two Tasks that File the same path do not need After to avoid overlap. evo.File already serializes same-path work through process-local resource holds. After for that contention lies about a producer/consumer relationship that does not exist.",
			BadCode: `writeA.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: left})
})
writeB.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: right})
})
writeB.After(writeA)
`,
			GoodCode: `writeA := out.Task("write left config")
writeA.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: left})
})
writeB := out.Task("write right config")
writeB.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: right})
})
`,
			Remediation:     "Delete After used as a lock — evo.File already serializes same-path work; After is only for a real producer/consumer DAG edge",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-DAG-006"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
	}
}

func init() { registerFamily(dagMisuseRules()) }
