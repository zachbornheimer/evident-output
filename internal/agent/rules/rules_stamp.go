package rules

// stampRules is the EVO-STAMP-*/EVO-FACT-001/EVO-EFFECT-001 family: Done
// used as a print, a duplicate sibling label, Define followed by Done,
// a fake-success Fact, or a planned mutation narrated as if it already
// succeeded.
func stampRules() []Rule {
	return []Rule{
		{
			ID:        "EVO-STAMP-001",
			Category:  "STAMP",
			Severity:  "warning",
			Invariant: "Task.Done resolves already-submitted work; it is not a generic printf",
			Why:       "Task(...).Done(\"processed %s\", path) with no preceding Define/File uses the success stamp as a print statement. The row claims work finished that evo never scheduled, and the format string is unstructured text no renderer/JSON consumer can project.",
			BadCode: `out.Task("house-style").Done("processed %s", path)
`,
			GoodCode: `t := out.Task("write " + path)
t.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})
`,
			Remediation:     "Submit the work with Define (or evo.File inside Define). Do not call Done after Define — Define already resolves from the callback",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-STAMP-001"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-STAMP-002",
			Category:  "STAMP",
			Severity:  "error",
			Invariant: "each sibling Task under one parent has a distinct name",
			Why:       "Repeating Task(\"house-style\") in a loop (or twice in one function) is a duplicate sibling declaration: 1.0 creates a second Failed row and records ErrDuplicateSiblingName instead of merging, because a merged identity could later report a false already-satisfied.",
			BadCode: `for _, path := range paths {
  out.Task("house-style").Done("ok")
}
`,
			GoodCode: `files := evo.Group("house-style")
for _, path := range paths {
  files.Task(path).Define(func() error { return write(path) })
}
`,
			Remediation:     "Give each sibling a distinct name — one Group.Task per item (the loop variable), never the same literal every iteration",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-STAMP-002"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-STAMP-003",
			Category:  "STAMP",
			Severity:  "error",
			Invariant: "Define already resolves the Task from the callback outcome; Done after Define is double-resolution",
			Why:       "t.Define(fn) submits work and resolves the row from fn's error. A later t.Done() tries to resolve the same handle a second time. MCP suggestions that say Define then Done teach this bug.",
			BadCode: `t := out.Task("write config")
t.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})
t.Done()
`,
			GoodCode: `t := out.Task("write config")
t.Define(func(ctx context.Context) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: data})
})
`,
			Remediation:     "Delete Done after Define — Define already resolves from the callback. Do not add Done to close the row",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-STAMP-003"},
			Since:           "1.0.0",
			Certainty:       "deterministic",
		},
		{
			ID:        "EVO-FACT-001",
			Category:  "FACT",
			Severity:  "warning",
			Invariant: "discovered information is a Fact, never a fake Task success",
			Why:       "Done(\"mapped to /tmp/out\") paints a checkmark for information, not work. Facts are name/value observations the renderer already projects; a success stamp for them lies about a job that never ran.",
			BadCode:   `out.Task("mapping").Done("mapped to %s", dest)`,
			GoodCode: `t := out.Task("scan")
t.Fact("mapped to", dest)
`,
			Remediation:     "Replace Done(\"mapped to...\") with task.Fact(name, value) on the Task that discovered the information",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-FACT-001"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-STAMP-004",
			Category:  "STAMP",
			Severity:  "warning",
			Invariant: "Task names are verb+object, never a fake phase or a bare noun",
			Why:       "Task(\"resolve\"), Task(\"finalize\"), and Task(\"file integrity\") name a phase or a thing, not the work. The row cannot tell a reader what happened, and MCP/agents invent a second Task to \"finalize\" work Define already resolved.",
			BadCode: `out.Task("resolve")
out.Task("file integrity").Fail("checksum mismatch")
`,
			GoodCode: `t := out.Task("check file integrity")
t.Define(func(ctx context.Context) error { return check(path) })
`,
			Remediation:     "Rename the Task to a verb+object phrase for the actual work (e.g. Task(\"check file integrity\"))",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-STAMP-004"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-FACT-002",
			Category:  "FACT",
			Severity:  "warning",
			Invariant: "a file path is a Fact or problem attachment on the owning Task, never a Task name used to display the path",
			Why:       "Task(issue.File).Fail paints a row whose name is a path. The work that found the problem already has a Task; the path belongs as Fact(\"file\", issue.File) or a Fail on that owner.",
			BadCode:   `out.Task(issue.File).Fail("checksum mismatch")`,
			GoodCode: `t := out.Task("check file integrity")
t.Fact("file", issue.File)
t.Fail("checksum mismatch")
`,
			Remediation:     "Call owning.Fail(...) and owning.Fact(\"file\", issue.File) on the Task that owns the work; do not declare Task(issue.File)",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-FACT-002"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
		{
			ID:        "EVO-EFFECT-001",
			Category:  "EFFECT",
			Severity:  "warning",
			Invariant: "a planned mutation is a DryRun mutation verb or Record, never Done(\"would add\")",
			Why:       "Done(\"would add %s\", path) or Done(\"proposal only\") stamps the task successful for work that has not happened. Config.DryRun already picks [planned] vs [changed] on Create/Delete/Update; Record tallies the effect. Narrating the plan through Done makes a dry-run look like a completed success.",
			BadCode: `out.Task("proposal").Done("would add %s", path)
out.Task("plan").Done("proposal only")
`,
			GoodCode: `out := evo.Init(evo.Config{DryRun: true})
t := out.Task("house-style")
t.Create("config", func() error { return write(path) })
// or, for a tally with no callback: t.Record("add", 1, "config")
`,
			Remediation:     "Use a mutation verb under Config.DryRun, or Record, instead of Done(\"would ...\" / \"proposal only\")",
			RelatedGuidance: []string{"tasks", "common-api"},
			VerificationIDs: []string{"EVO-EFFECT-001"},
			Since:           "1.0.0",
			Certainty:       "heuristic",
		},
	}
}

func init() { registerFamily(stampRules()) }
